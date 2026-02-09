package ui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Node represents a file or directory in the tree.
type Node struct {
	Name     string
	Path     string
	Children []*Node
	Parent   *Node
	IsDir    bool
	IsHistory bool // True if this path is from history

	// Layout coordinates
	X, Y int
}

// TreeModel handles the tree visualization and navigation.
type TreeModel struct {
	Root         *Node
	SelectedNode *Node
	Width        int
	Height       int

	// ScrollOffset handles vertical scrolling if the tree is taller than the screen
	ScrollOffset int
	
	// Dynamic Column Widths (Depth -> Max Width)
	ColWidths map[int]int
}

// NewTreeModel creates a new tree model from a list of paths.
// historyPaths is a set of paths that are from history (for visual distinction).
func NewTreeModel(paths []string, width, height int, historyPaths map[string]bool) TreeModel {
	root := buildTree(paths, historyPaths)
	compressTree(root)
	tm := TreeModel{
		Root:      root,
		Width:     width,
		Height:    height,
		ColWidths: make(map[int]int),
	}
	// Default selection: Best Match (paths[0])
	if len(paths) > 0 {
		bestMatch := findNode(root, paths[0])
		if bestMatch != nil {
			tm.SelectedNode = bestMatch
		} else if len(root.Children) > 0 {
			tm.SelectedNode = root.Children[0]
		} else {
			tm.SelectedNode = root
		}
	} else if len(root.Children) > 0 {
		tm.SelectedNode = root.Children[0]
	} else {
		tm.SelectedNode = root
	}
	return tm
}

func findNode(root *Node, targetPath string) *Node {
	// BFS or DFS to find node with Path == targetPath
	// Since compression updates Path, this works.
	// Clean targetPath partials?
	// buildTree uses filepath.Clean via Join?
	// Let's assume exact match slightly.

	if root.Path == targetPath {
		return root
	}
	// Also check if targetPath is "inside" the root path? (for directories)
	// But we typically want the exact file match.
	
	// Actually targetPath from search is clean.
	// root.Path is clean.
	// Simple DFS.
	stack := []*Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if n.Path == targetPath {
			return n
		}
		// Also check relative?
		// if path == "./" + targetPath?
		// buildTree uses "." as root.
		// if targetPath is "src/main.go", root is ".", child is "src".
		// child.Path is "src".
		// Eventually "src/main.go".
		
		stack = append(stack, n.Children...)
	}
	return nil
}

func buildTree(paths []string, historyPaths map[string]bool) *Node {
	root := &Node{Name: "ROOT", IsDir: true, Path: "."}
	for _, path := range paths {
		parts := strings.Split(path, string(filepath.Separator))
		current := root
		for i, part := range parts {
			var child *Node
			for _, c := range current.Children {
				if c.Name == part {
					child = c
					break
				}
			}
			if child == nil {
				isDir := i < len(parts)-1
				childPath := filepath.Join(current.Path, part)
				// Check if this path or any parent is in history
				isHistory := historyPaths[path] || historyPaths[childPath]
				child = &Node{
					Name:      part,
					Path:      childPath,
					Parent:    current,
					IsDir:     isDir,
					IsHistory: isHistory,
				}
				current.Children = append(current.Children, child)
				// Sort removed to preserve search relevance order
				/*
				sort.Slice(current.Children, func(i, j int) bool {
					return current.Children[i].Name < current.Children[j].Name
				})
				*/
			} else {
				// If this path is in history, mark the existing node too
				if historyPaths[path] || historyPaths[child.Path] {
					child.IsHistory = true
				}
			}
			current = child
			current.IsDir = true
		}
		// Mark the final node as history if the path is in history
		if historyPaths[path] {
			current.IsHistory = true
		}
	}
	return root
}

func compressTree(node *Node) {
	if !node.IsDir || len(node.Children) == 0 {
		return
	}

	// Compress children first
	for _, child := range node.Children {
		compressTree(child)
	}

	// If single child, merge (skip Root)
	// We don't merge ROOT with its child usually, but here ROOT is hidden/special?
	// The View renders ROOT's children.
	// If ROOT has 1 child "src", and "src" has "main", we want "src/main".
	// ROOT shouldn't be part of the name.
	if node.Name == "ROOT" {
		return
	}

	if len(node.Children) == 1 {
		child := node.Children[0]
		
		// Conditional Compression
		// User Rule: If dir contains children that add height > 3-5 rows, don't cut off with "..." (don't make wide leaf).
		// Heuristic:
		// 1. Calculate potential new name length.
		// 2. If length > ColWidth (approx 18), it would be shortened.
		// 3. If so, check if the child has significant height (e.g. > 4 leaves).
		// 4. If Height > 4, Do NOT compress. Keep vertical structure to avoid horizontal truncation.
		
		potentialNameLen := len(node.Name) + 1 + len(child.Name)
		if potentialNameLen > 18 { // ColWidth 20 - padding
			leaves := countLeaves(child)
			if leaves > 4 {
				return // Skip merge
			}
		}

		// Merge child into current node
		node.Name = filepath.Join(node.Name, child.Name)
		node.Path = child.Path
		node.IsDir = child.IsDir
		node.Children = child.Children
		
		// CRITICAL: Update Parent pointers for grandchildren!
		for _, grandChild := range node.Children {
			grandChild.Parent = node
		}
		
		// We need to re-compress this node in case we just brought up another single child
		compressTree(node)
	}
}

func countLeaves(n *Node) int {
	if len(n.Children) == 0 {
		return 1
	}
	count := 0
	for _, c := range n.Children {
		count += countLeaves(c)
	}
	return count
}

func (m TreeModel) Init() tea.Cmd {
	return nil
}

func (m TreeModel) Update(msg tea.Msg) (TreeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.moveSelection(-1)
		case "down", "j":
			m.moveSelection(1)
		case "right", "l":
			m.enterDirectory()
		case "left", "h":
			m.leaveDirectory()
		}
	}

	// Adjust scroll to keep selection in view
	if m.SelectedNode != nil {
		// We need the layout to know the Y position.
		// Since Update happens before View, we might be using stale Y?
		// Actually, Y is deterministic based on the tree structure.
		// We can re-run layout calculation here or just ensure View handles it.
		// For now, let's strictly handle scrolling in View or after layout update.
		// However, standard Bubble Tea practice is to update state in Update.
		// Let's implement a helper to calculate Y for the selected node.
		// Or better: Layout is fast, run it on Update if needed, or View handles scrolling.
		// Simple approach: View calculates layout, then applies scroll.
		// But Update needs to change ScrollOffset.

		// Let's defer scroll calculation to View for now,
		// but we need to ensure m.ScrollOffset is persistent.
		// Actually, if we re-layout in View, we can't easily "scroll into view" in Update.

		// Strategy: Run a layout pass in Update to get SelectedNode.Y
		// Then adjust m.ScrollOffset.
		// Optimization: Only do this if structure changed or selection moved.
		expanded := m.getExpandedMap()
		m.layoutRoot(m.Root, expanded)

		// Scroll logic
		// If SelectedNode.Y < m.ScrollOffset -> m.ScrollOffset = SelectedNode.Y
		// If SelectedNode.Y >= m.ScrollOffset + m.Height -> m.ScrollOffset = SelectedNode.Y - m.Height + 1
		if m.SelectedNode.Y < m.ScrollOffset {
			m.ScrollOffset = m.SelectedNode.Y
		}
		if m.SelectedNode.Y >= m.ScrollOffset+m.Height {
			m.ScrollOffset = m.SelectedNode.Y - m.Height + 1
		}
	}

	return m, nil
}

func (m *TreeModel) moveSelection(delta int) {
	if m.SelectedNode == nil || m.SelectedNode.Parent == nil {
		return
	}
	siblings := m.SelectedNode.Parent.Children
	for i, node := range siblings {
		if node == m.SelectedNode {
			next := i + delta
			if next >= 0 && next < len(siblings) {
				m.SelectedNode = siblings[next]
			}
			return
		}
	}
}

func (m *TreeModel) enterDirectory() {
	if m.SelectedNode != nil && m.SelectedNode.IsDir && len(m.SelectedNode.Children) > 0 {
		m.SelectedNode = m.SelectedNode.Children[0]
	}
}

func (m *TreeModel) leaveDirectory() {
	if m.SelectedNode != nil && m.SelectedNode.Parent != nil && m.SelectedNode.Parent.Parent != nil {
		m.SelectedNode = m.SelectedNode.Parent
	}
}

func (m TreeModel) getExpandedMap() map[*Node]bool {
	expanded := make(map[*Node]bool)
	if m.Root != nil {
		expanded[m.Root] = true
	}
	curr := m.SelectedNode
	for curr != nil {
		expanded[curr] = true
		curr = curr.Parent
	}
	return expanded
}

// getVisibleChildren returns the list of children that should be rendered/laid out.
// Logic:
// 1. If node is SelectedNode or Parent of SelectedNode: Return ALL children (siblings/current dir).
// 2. If node is an Ancestor (Grandparent+), return ONLY the child on the path to SelectedNode.
// 3. Otherwise (Sibling of Ancestor, etc): Follow expanded map (usually collapsed).
func (m TreeModel) getVisibleChildren(node *Node, expanded map[*Node]bool) []*Node {
	// If collapsed (and not forced visible by isolation logic? No, isolation works within expansion)
	// If not expanded, return nil (or empty)
	if !expanded[node] {
		return nil
	}
	
	// Root is always expanded, but we apply isolation to it too.
	
	// Check if this node is an ancestor of SelectedNode
	// And specifically, is it the Parent?
	if m.SelectedNode != nil {
		if node == m.SelectedNode || node == m.SelectedNode.Parent {
			return node.Children
		}
		
		// If it is an ancestor (but not parent), we find the child on the path.
		// We can walk up from SelectedNode until we find the child whose parent is `node`.
		curr := m.SelectedNode
		for curr != nil {
			if curr.Parent == node {
				// Found the child on the path
				return []*Node{curr}
			}
			curr = curr.Parent
		}
	}
	
	// If we are here, node is either:
	// a) SelectedNode (handled)
	// b) Parent (handled)
	// c) Ancestor (handled - returned single child)
	// d) Sibling/Cousin (not on path).
	//    If Expanded map says true (e.g. manually expanded?), show children.
	//    Our current expanded logic ONLY expands path.
	//    So cousins are collapsed.
	
	return node.Children
}

// layoutRoot calculates X, Y for all nodes
func (m *TreeModel) layoutRoot(root *Node, expanded map[*Node]bool) {
	// 1. Assign Y to leaves
	// 2. Assign Y to parents (center of children)
	// 3. Assign X based on depth
	// Miller Columns: Y is assigned per depth index.
	// We need a counter for each depth.
	yCounters := make(map[int]int)
	// Build ColWidths anew
	m.ColWidths = make(map[int]int)
	
	// Start layout
	layoutAssign(root, 0, yCounters, expanded, m)
}

func layoutAssign(node *Node, depth int, yCounters map[int]int, expanded map[*Node]bool, m *TreeModel) {
    // 1. Assign X
    node.X = depth
    
    // 2. Assign Y
    // Use the counter for this depth
    node.Y = yCounters[depth]
    yCounters[depth]++
    
    // 3. Update Column Width
    // Calculate display width
    name := node.Name
    // Logic from RenderNode regarding truncation/formatting
    // "Interactive Contraction"
    // Just use raw length? No, we need formatted length.
    // Duplicate logic or simplify?
    // Let's approximate: len(node.Name) + 2 (cursor) + 1 (slash)
    w := len(name) + 4 // Cursor "> " + "/" + padding
    
    // If compressed, name might change visually (…/parent/child).
    if node != m.SelectedNode && strings.Contains(name, string(filepath.Separator)) {
        parts := strings.Split(name, string(filepath.Separator))
        if len(parts) > 2 {
            w = len("…/"+parts[len(parts)-2]+"/"+parts[len(parts)-1]) + 4
        }
    }
    
    // Max Width Cap?
    if w > 40 { w = 40 } // Hard cap to prevent visual explosion
    if w < 20 { w = 20 } // Min width for stability?
    
    if w > m.ColWidths[depth] {
        m.ColWidths[depth] = w
    }
    
    // 4. Recurse to Visible Children
    children := m.getVisibleChildren(node, expanded)
    
    // For Miller Columns, children start at Y=0 of next depth?
    // Or do they align with parent?
    // Typically they start at top of their column.
    
    // HOWEVER, if we restart Y=0 for every "folder's children", we get overlap if multiple folders expanded in same col?
    // In our logic ("Ancestor Isolation" + "One Path"), only ONE folder is expanded per column.
    // EXCEPT for the "Current Level" where we see siblings?
    // No, siblings are in the SAME column.
    // If we select a sibling, it expands into NEXT column.
    // Since only ONE path is followed, there is only ONE block of content in the Next Column.
    // So resetting Y counter for next depth?
    // Wait, yCounters is global for the map?
    // If we use global yCounters[depth], then children of Sibling A and children of Sibling B would stack.
    // But we only show children of Selected Node!
    // So yes, stacking is fine (actually there's only one set).
    // So `yCounters` works perfectly.
    
    // Actually, we want children to start at Y=0 relative to the screen?
    // Or align with parent?
    // User probably wants them at the top (Y=0).
    // Let's reset yCounters for deeper levels?
    // No, if we use a single map, it accumulates.
    // But we traverse DFS.
    
    // Issue: If `isolation` is on, we only descend ONE path.
    // So there is only ONE list at depth D.
    // So yCounters[depth] will count 0, 1, 2... for that list.
    // This is exactly what we want.
    
    for _, child := range children {
        layoutAssign(child, depth+1, yCounters, expanded, m)
    }
}

func (m TreeModel) View() string {
	if m.Root == nil {
		return ""
	}

	// 1. Calculate Layout
	// We want to hide "Common Ancestors" if they are just single-child containers.
	// Find the "Display Root": The first node that has > 1 child OR is a leaf?
	// Or simply, descend while 1 child.
	
	displayRoot := m.Root
	for len(displayRoot.Children) == 1 && displayRoot.Children[0].IsDir {
		displayRoot = displayRoot.Children[0]
	}
	// Exception: If DisplayRoot is the selected node?
	// No, we want to show its siblings/children.
	
	expanded := m.getExpandedMap()
	
	// Layout:
	// If displayRoot != Root, we want displayRoot to be at X = -1 (Hidden Parent), 
	// and its children to be at X = 0.
	// But `layoutRoot` starts at 0.
	// We can modify `layoutRoot` to accept a starting depth?
	// Or just layout normally and shift X in Render?
	
	m.layoutRoot(m.Root, expanded)
	
	// Shift DisplayRoot and descendants so that DisplayRoot.Children are at X=0.
	// displayRoot.X should be -1.
	// Currently `layoutRoot` assigns X based on depth from Root.
	// If Root -> A -> B -> [C, D]
	// Root=0, A=1, B=2, C=3.
	// displayRoot = B.
	// We want C=0.
	// So shift = - (B.X + 1) = -3
	// B.X + shift = -1.
	
	shiftX := 0
	if displayRoot != m.Root {
		shiftX = -(displayRoot.X + 1)
	} else {
	    // If Root has multiple children, Root is X=0. Children X=1.
	    // User said "no need for ROOT columns".
	    // So Root should be X=-1. Children X=0.
	    shiftX = -1
	}
	
	// Apply Shift
	m.applyXShift(m.Root, shiftX)

	// 2. Determine Column Widths
	// Calculated in layoutRoot -> layoutAssign.
	// No fixed colWidth.
	
	// 3. Horizontal Scroll
	// Determine where SelectedNode is efficiently.
	// We need to sum widths up to SelectedNode.X?
	// Or just count columns?
	// If columns are variable width, we need to know "Page Size" in columns?
	// Harder with variable width.
	// Let's stick to simple "Column Index" scrolling for now?
	// Yes, strict column scrolling.
	// But we need to verify total width fits?
	
	// For variable width, "Scrolling" means shifting the starting column index.
	// scrollX is the index of the first visible column.
	
	// Calculate total used width for visible range [scrollX, ...]
	// We want SelectedNode to be visible.
	// Simple strategy: Keep SelectedNode in the right-most or center?
	// "Steps to the left slide off-screen".
	// Let's try to fit as many columns from SelectedNode backwards as possible.
	
	targetCol := m.SelectedNode.X
	
	// Find minimal scrollX such that column 'targetCol' is visible.
	// Iterate backwards from targetCol, summing widths, until > m.Width.
	// The last fitting column is the start?
	// No, we want as much context to the left as possible.
	// So Start = targetCol - K.
	
	// Actually, easier:
	// Start with scrollX = 0.
	// Calculate if targetCol is within screen.
	// Sum widths 0..targetCol. If > Width, increment scrollX.
	
	// Let's implement a loop to find valid scrollX.
	// Optimization: If targetCol is huge, just jump.
	
	scrollX := 0
	// Heuristic: If we are deep, maybe start closer.
	if targetCol > 0 {
	    // Accumulate width from 0
	    currentW := 0
	    for i := 0; i <= targetCol; i++ {
	        currentW += m.ColWidths[i]
	    }
	    
	    // While total width > m.Width, remove from left (increment scrollX)
	    // AND ensure scrollX <= targetCol (always show selected)
	    for currentW > m.Width && scrollX < targetCol {
	        currentW -= m.ColWidths[scrollX]
	        scrollX++
	    }
	}
	
	// 4. Render Canvas
	// We only render the visible window (m.ScrollOffset to m.ScrollOffset + m.Height)
	// But we need to know where lines go.

	canvas := make([][]string, m.Height)
	for y := 0; y < m.Height; y++ {
		line := make([]string, m.Width)
		for x := 0; x < m.Width; x++ {
			line[x] = " "
		}
		canvas[y] = line
	}

	// Helper to draw string on canvas
	drawString := func(x, y int, s string, style lipgloss.Style) {
		if y < m.ScrollOffset || y >= m.ScrollOffset+m.Height {
			return
		}
		screenY := y - m.ScrollOffset

		// If x is outside width, skip
		// Check bounds strictly
		if x >= m.Width {
			return
		}

		runes := []rune(s)
		for i, r := range runes {
			if x+i >= 0 && x+i < m.Width {
				canvas[screenY][x+i] = style.Render(string(r))
			}
		}
	}

	// Recursive render
	var renderNode func(n *Node)
	renderNode = func(n *Node) {
		// Calculate Screen X
		// n.X is logical column.
		// offset is sum of widths from scrollX to n.X - 1.
		
		effX := n.X - scrollX
		
		if effX < 0 {
		    // Off-screen left?
		    // We might need to handle connectors traversing from left?
		    // For now skip rendering text.
		}
		
		// Calculate screenX
		screenX := 0
		if effX >= 0 {
		    for i := scrollX; i < n.X; i++ {
		        screenX += m.ColWidths[i]
		    }
		} else {
		    screenX = -100 // Hidden
		}
		
		colWidth := m.ColWidths[n.X] // Use actual width of this column
		
		// Draw Node (Only if visible and meaningful)
		if effX >= 0 && screenX < m.Width {
			// Style
			style := lipgloss.NewStyle()
			cursor := ""
			if n == m.SelectedNode {
				style = style.Foreground(lipgloss.Color("205")).Bold(true)
				cursor = "> "
			} else {
				// Check if it's an ancestor of selected
				isAncestor := false
				curr := m.SelectedNode
				for curr != nil {
					if curr == n {
						isAncestor = true
						break
					}
					curr = curr.Parent
				}
				if isAncestor {
					style = style.Foreground(lipgloss.Color("62")) // Blurple
				} else if n.IsHistory {
					// History items get a distinct color (cyan/blue)
					style = style.Foreground(lipgloss.Color("39")) // Bright cyan
				} else {
					style = style.Foreground(lipgloss.Color("240")) // Grey
				}
			}
	
			name := n.Name
			
			// Interactive Contraction
			// If compressed (has separators) AND NOT selected, show "…/parent/end"
			if n != m.SelectedNode && strings.Contains(n.Name, string(filepath.Separator)) {
				parts := strings.Split(n.Name, string(filepath.Separator))
				if len(parts) > 2 {
					name = filepath.Join("…", parts[len(parts)-2], parts[len(parts)-1])
				} else if len(parts) == 2 {
					name = n.Name
				}
			}
	
			if n.IsDir {
				name += "/"
			}
			
			// Truncate to column length (safe guard)
			if len(name) > colWidth-2 {
				name = name[:colWidth-2] + ".."
			} 
	
			drawString(screenX, n.Y, cursor+name, style)
		}
		
		// Draw Connectors to Children
		children := m.getVisibleChildren(n, expanded)
		if len(children) > 0 {
			// vx is at the RIGHT EDGE of the current column.
			// vx = screenX + colWidth - 2 ?
			// Wait, screenX is start.
			// Next column starts at screenX + colWidth.
			// Connector line should be at screenX + colWidth - 2 presumably?
			// Or just outside text?
			
			// Let's put it at the end of the calculated column width.
			
			// Re-calc screenX for safety (closure capture issue?)
			parentScreenX := 0
			if n.X >= scrollX {
    		    for i := scrollX; i < n.X; i++ {
    		        parentScreenX += m.ColWidths[i]
    		    }
			    
			    vx := parentScreenX + colWidth - 2
			    
			    if vx < m.Width { // valid x
        			minY := children[0].Y
        			maxY := children[len(children)-1].Y
        			
    				for y := minY; y <= maxY; y++ {
    					char := "│"
    					isChildY := false
    					for _, c := range children {
    						if c.Y == y {
    							isChildY = true
    							break
    						}
    					}
    					if isChildY {
    						if y == minY {
    							if len(children) > 1 { char = "┬" } else { char = "─" }
    						} else if y == maxY {
    							char = "└"
    						} else {
    							char = "├"
    						}
    					} else {
    						char = "│"
    					}
    					drawString(vx, y, char, lipgloss.NewStyle().Faint(true))
    					// Horizontal Dash
    					if isChildY {
    					    drawString(vx+1, y, "─", lipgloss.NewStyle().Faint(true))
    					}
    				}
    				
    				// Connect Parent to Vertical Line
    				// Assume Parent Name ends before vx (since colWidth includes padding)
    				// ...
			    }
			}
			
			// Recurse
			for _, child := range children {
				renderNode(child)
			}
		}
	}

	// Start Render
	renderNode(m.Root)

	// 4. Flatten Canvas to String
	var s strings.Builder
	for i, line := range canvas {
		s.WriteString(strings.Join(line, ""))
		if i < m.Height-1 {
			s.WriteRune('\n')
		}
	}

	return s.String()
}

// applyXShift recursively shifts X
func (m *TreeModel) applyXShift(node *Node, shift int) {
	node.X += shift
	for _, child := range node.Children {
		m.applyXShift(child, shift)
	}
}

func (m TreeModel) SelectedPath() string {
	if m.SelectedNode != nil {
		return strings.TrimPrefix(m.SelectedNode.Path, "ROOT/") // Clean prefix if exists
	}
	return ""
}
