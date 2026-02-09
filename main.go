package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "github.com/mattn/go-sqlite3"
	"github.com/montrey/navi/search"
	"github.com/montrey/navi/store"
	"github.com/montrey/navi/ui"
)

type model struct {
	db           *sql.DB
	input        textinput.Model
	tree         ui.TreeModel
	currentDir   string
	allFiles     []string // Cache of all files in current loop (local or tag)
	historyFiles []string // Files from history/tags (initial load)
	currentDirFiles []string // Files from current directory
	historyPaths map[string]bool // Set of paths that are from history
	activeTag    string   // Currently active tag (empty if local)
	selectedPath string
	width        int
	height       int
	err          error
	isInitialLoad bool // Track if this is the initial load
	currentDirLoaded bool // Track if current directory files have been loaded
}

type filesLoadedMsg []string
type searchDoneMsg []search.Result

// loadInitialFiles loads recent history + tagged paths for initial app load
func loadInitialFiles(db *sql.DB) tea.Cmd {
	return func() tea.Msg {
		// Get recent history (last 100 items) and tagged paths
		recentHistory, _ := store.GetRecentHistory(db, 100)
		tagged, _ := store.GetAllTaggedPaths(db)

		// Combine recent history paths and tagged paths
		pathSet := make(map[string]bool)
		var files []string

		// Add recent history paths
		for _, h := range recentHistory {
			if !pathSet[h.Path] {
				pathSet[h.Path] = true
				files = append(files, h.Path)
			}
		}

		// Add tagged paths
		for _, p := range tagged {
			if !pathSet[p] {
				pathSet[p] = true
				files = append(files, p)
			}
		}

		// Sort: Tagged > Recent > Alpha
		taggedSet := make(map[string]bool)
		for _, p := range tagged {
			taggedSet[p] = true
		}

		recency := make(map[string]int)
		for _, h := range recentHistory {
			recency[h.Path] = int(h.LastVisited.Unix())
		}

		sort.SliceStable(files, func(i, j int) bool {
			p1 := files[i]
			p2 := files[j]

			// 1. Tagged?
			t1 := taggedSet[p1]
			t2 := taggedSet[p2]
			if t1 && !t2 {
				return true
			}
			if !t1 && t2 {
				return false
			}

			// 2. Recent?
			r1 := recency[p1]
			r2 := recency[p2]
			if r1 != r2 {
				return r1 > r2 // Standard desc timestamp
			}

			// 3. Alphabetical
			return p1 < p2
		})

		return filesLoadedMsg(files)
	}
}

func loadFiles(db *sql.DB, root string) tea.Cmd {
	return func() tea.Msg {
		files, err := search.Walk(root)
		if err != nil {
			return filesLoadedMsg(nil)
		}

		// Default Prioritization
		tagged, _ := store.GetAllTaggedPaths(db)
		history, _ := store.GetHistory(db)

		// Sort: Tagged > Recent > Current Dir (lower priority) > Alpha
		// 1. Create Lookup Maps
		isTagged := make(map[string]bool)
		for _, p := range tagged {
			isTagged[p] = true
		}

		recency := make(map[string]int) // Path -> LastVisited (Unix) or Rank
		for _, h := range history {
			// Higher index = Lower rank. We want high priority first.
			// Let's use LastVisited.Unix()
			recency[h.Path] = int(h.LastVisited.Unix())

			// If we just want "Is Recent", we can check existence.
			// But user said "recent... on top".
			// Let's prioritize by Recency Score.
		}

		// Mark paths from current directory (give them lower priority)
		isCurrentDir := make(map[string]bool)
		for _, f := range files {
			if strings.HasPrefix(f, root+string(filepath.Separator)) || f == root {
				isCurrentDir[f] = true
			}
		}

		sort.SliceStable(files, func(i, j int) bool {
			p1 := files[i]
			p2 := files[j]

			// 1. Tagged?
			t1 := isTagged[p1]
			t2 := isTagged[p2]
			if t1 && !t2 {
				return true
			}
			if !t1 && t2 {
				return false
			}

			// 2. Recent?
			r1 := recency[p1]
			r2 := recency[p2]
			if r1 != r2 {
				return r1 > r2 // Standard desc timestamp
			}

			// 3. Current directory files get lower priority (after recent)
			cd1 := isCurrentDir[p1]
			cd2 := isCurrentDir[p2]
			if cd1 && !cd2 {
				return false // Current dir files come after non-current dir
			}
			if !cd1 && cd2 {
				return true
			}

			// 4. Alphabetical
			return p1 < p2
		})

		return filesLoadedMsg(files)
	}
}

func loadTagFiles(db *sql.DB, tag string) tea.Cmd {
	return func() tea.Msg {
		paths, err := store.GetPathsForTag(db, tag)
		if err != nil {
			return filesLoadedMsg(nil)
		}
		return filesLoadedMsg(paths)
	}
}

// combineFiles merges history files with current directory files, removing duplicates
// History files come first (higher priority)
func combineFiles(historyFiles, currentDirFiles []string) []string {
	pathSet := make(map[string]bool)
	var combined []string
	
	// Add history files first (higher priority)
	for _, path := range historyFiles {
		if !pathSet[path] {
			pathSet[path] = true
			combined = append(combined, path)
		}
	}
	
	// Add current directory files (lower priority, no duplicates)
	for _, path := range currentDirFiles {
		if !pathSet[path] {
			pathSet[path] = true
			combined = append(combined, path)
		}
	}
	
	return combined
}

func performSearch(files []string, query string) tea.Cmd {
	return func() tea.Msg {
		results := search.FuzzyHierarchical(files, query)
		return searchDoneMsg(results)
	}
}

func initialModel(db *sql.DB) model {
	ti := textinput.New()
	ti.Placeholder = "Search... (use @tag for scopes)"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	ti.Width = 20

	wd, _ := os.Getwd()

	// Init empty tree
	tm := ui.NewTreeModel([]string{}, 80, 20, make(map[string]bool))

	return model{
		db:           db,
		input:        ti,
		tree:         tm,
		currentDir:   wd,
		historyPaths: make(map[string]bool),
		isInitialLoad: true,
		currentDirLoaded: false,
	}
}

func (m model) Init() tea.Cmd {
	// On initial load, show recent history + tagged paths only
	return tea.Batch(textinput.Blink, loadInitialFiles(m.db))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case filesLoadedMsg:
		// Determine if this is history/tags load or current directory load
		if m.isInitialLoad {
			// Initial load: history + tags
			m.historyFiles = msg
			m.isInitialLoad = false
			// Mark all paths as history
			m.historyPaths = make(map[string]bool)
			for _, path := range msg {
				m.historyPaths[path] = true
			}
			// Combine with current directory files if already loaded
			if m.currentDirLoaded {
				m.allFiles = combineFiles(m.historyFiles, m.currentDirFiles)
			} else {
				m.allFiles = m.historyFiles
			}
		} else if m.activeTag != "" {
			// Tag load
			m.allFiles = msg
			m.historyPaths = make(map[string]bool)
			for _, path := range msg {
				m.historyPaths[path] = true
			}
		} else {
			// Current directory load
			m.currentDirFiles = msg
			m.currentDirLoaded = true
			// Update historyPaths for paths actually in history
			history, _ := store.GetHistory(m.db)
			historySet := make(map[string]bool)
			for _, h := range history {
				historySet[h.Path] = true
			}
			for _, path := range msg {
				if historySet[path] {
					m.historyPaths[path] = true
				}
			}
			// Combine with history files
			m.allFiles = combineFiles(m.historyFiles, m.currentDirFiles)
		}
		// Trigger search
		parsedQuery := m.input.Value()
		if m.activeTag != "" {
			parsedQuery = strings.TrimPrefix(parsedQuery, "@"+m.activeTag)
			parsedQuery = strings.TrimPrefix(parsedQuery, " ")
		}
		cmds = append(cmds, performSearch(m.allFiles, parsedQuery))

	case searchDoneMsg:
		var paths []string
		for _, res := range msg {
			paths = append(paths, res.Path)
		}
		// Rebuild tree with new paths
		// Use window dimensions if available, otherwise use existing tree dimensions
		treeWidth := m.width
		treeHeight := m.height - 3
		if treeWidth == 0 || treeHeight <= 0 {
			// Window size not set yet, use existing tree dimensions or defaults
			if m.tree.Width > 0 {
				treeWidth = m.tree.Width
			} else {
				treeWidth = 80 // Default width
			}
			if m.tree.Height > 0 {
				treeHeight = m.tree.Height
			} else {
				treeHeight = 20 // Default height
			}
		}
		// Pass history paths to tree for visual distinction
		m.tree = ui.NewTreeModel(paths, treeWidth, treeHeight, m.historyPaths)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			// Handle Selection form Tree
			selectedPath := m.tree.SelectedPath()
			if selectedPath == "" {
				return m, nil
			}

			// If Directory (check via os.Stat)
			info, err := os.Stat(selectedPath)
			if err == nil && info.IsDir() {
				// Update History for Dir
				_ = store.UpdateFrecency(m.db, selectedPath)
				// Mark as history
				m.historyPaths[selectedPath] = true
				// Drill down
				m.currentDir = selectedPath
				m.input.SetValue("")
				m.activeTag = ""
				m.currentDirLoaded = false // Reset since we're changing directory
				cmds = append(cmds, loadFiles(m.db, m.currentDir))
			} else {
				// File -> Update History, Select and Quit
				_ = store.UpdateFrecency(m.db, selectedPath)
				// Mark as history
				m.historyPaths[selectedPath] = true
				m.selectedPath = selectedPath
				return m, tea.Quit
			}

		case "up", "down", "left", "right":
			// Pass to tree
			var treeCmd tea.Cmd
			m.tree, treeCmd = m.tree.Update(msg)
			cmds = append(cmds, treeCmd)
		default:
			oldValue := m.input.Value()
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)

			newValue := m.input.Value()
			if newValue != oldValue {
				// Parsing Logic for Tags
				if strings.HasPrefix(newValue, "@") && strings.Contains(newValue, " ") {
					parts := strings.SplitN(newValue, " ", 2)
					potentialTag := strings.TrimPrefix(parts[0], "@")

					if potentialTag != m.activeTag {
						m.activeTag = potentialTag
						// Load files for tag
						cmds = append(cmds, loadTagFiles(m.db, m.activeTag))
						// Return to wait for filesLoadedMsg
						return m, tea.Batch(cmds...)
					}

					// Tag active, search with rest
					query := ""
					if len(parts) > 1 {
						query = parts[1]
					}
					// Use combined files if current dir is loaded
					searchFiles := m.allFiles
					if m.currentDirLoaded && len(m.currentDirFiles) > 0 {
						searchFiles = combineFiles(m.historyFiles, m.currentDirFiles)
					}
					cmds = append(cmds, performSearch(searchFiles, query))
				} else if strings.HasPrefix(newValue, "@") {
					// Typing tag... search in current files
					searchFiles := m.allFiles
					if m.currentDirLoaded && len(m.currentDirFiles) > 0 {
						searchFiles = combineFiles(m.historyFiles, m.currentDirFiles)
					}
					cmds = append(cmds, performSearch(searchFiles, newValue))
				} else {
					// Standard local search
					if m.activeTag != "" {
						// Backspaced out of tag?
						m.activeTag = ""
						cmds = append(cmds, loadFiles(m.db, m.currentDir))
					} else {
						// If user starts typing and current directory not loaded yet, load it
						if !m.currentDirLoaded && newValue != "" {
							m.currentDirLoaded = true
							cmds = append(cmds, loadFiles(m.db, m.currentDir))
						} else {
							// Search in combined files (history + current dir if loaded)
							searchFiles := m.allFiles
							if m.currentDirLoaded && len(m.currentDirFiles) > 0 {
								// Recombine to ensure we have latest
								searchFiles = combineFiles(m.historyFiles, m.currentDirFiles)
							}
							cmds = append(cmds, performSearch(searchFiles, newValue))
						}
					}
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputHeight := 3
		listHeight := msg.Height - inputHeight
		if listHeight > 0 {
			m.tree.Width = msg.Width
			m.tree.Height = listHeight
			// If we have files loaded, rebuild tree with new dimensions
			if len(m.allFiles) > 0 {
				parsedQuery := m.input.Value()
				if m.activeTag != "" {
					parsedQuery = strings.TrimPrefix(parsedQuery, "@"+m.activeTag)
					parsedQuery = strings.TrimPrefix(parsedQuery, " ")
				}
				cmds = append(cmds, performSearch(m.allFiles, parsedQuery))
			}
		}
		m.input.Width = msg.Width
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	header := m.input.View()
	if m.activeTag != "" {
		tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
		header = fmt.Sprintf("%s %s", tagStyle.Render("[@"+m.activeTag+"]"), m.input.View())
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.tree.View(),
	)
}

func main() {
	// CLI Flags
	addTag := flag.String("add", "", "Add current directory to a tag")
	flag.Parse()

	// Init DB
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".local", "share", "navi", "navi.db")
	db, err := store.InitDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Handle CLI Commands
	if *addTag != "" {
		cwd, _ := os.Getwd()
		err := store.AddPathToTag(db, *addTag, cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to add to tag: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added %s to tag @%s\n", cwd, *addTag)
		return
	}

	p := tea.NewProgram(initialModel(db), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}

	// Output Selection
	if m, ok := finalModel.(model); ok && m.selectedPath != "" {
		fmt.Println(m.selectedPath)
	}
}
