package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
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
	mode         viewMode
	config       appConfig
	configField  int
	configEditing bool
	configInput  textinput.Model
	tagPath      string
	tagList      []string
	tagSelected  int
	tagEditing   bool
	tagInput     textinput.Model
}

type filesLoadedMsg []string
type searchDoneMsg []search.Result

type viewMode int

const (
	modeBrowse viewMode = iota
	modeConfig
	modeTags
)

type appConfig struct {
	DefaultAction string
	TerminalCmd   string
	ExplorerCmd   string
	EditorCmd     string
}

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

func buildSearchList(db *sql.DB, root string) []string {
	// Build history + tags list
	recentHistory, _ := store.GetRecentHistory(db, 100)
	tagged, _ := store.GetAllTaggedPaths(db)
	pathSet := make(map[string]bool)
	var historyFiles []string
	for _, h := range recentHistory {
		if !pathSet[h.Path] {
			pathSet[h.Path] = true
			historyFiles = append(historyFiles, h.Path)
		}
	}
	for _, p := range tagged {
		if !pathSet[p] {
			pathSet[p] = true
			historyFiles = append(historyFiles, p)
		}
	}

	currentFiles, _ := search.Walk(root)
	return combineFiles(historyFiles, currentFiles)
}

func defaultConfig() appConfig {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nvim"
	}
	terminal := os.Getenv("TERMINAL")
	if terminal == "" {
		terminal = "xterm"
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	return appConfig{
		DefaultAction: "explorer",
		TerminalCmd:   fmt.Sprintf(`%s -e bash -lc 'cd "%s"; exec %s'`, terminal, "{path}", shell),
		ExplorerCmd:   `xdg-open "{path}"`,
		EditorCmd:     fmt.Sprintf(`%s "{path}"`, editor),
	}
}

func loadConfig(db *sql.DB) appConfig {
	cfg := defaultConfig()
	if v, _ := store.GetSetting(db, "default_action"); v != "" {
		cfg.DefaultAction = v
	}
	if v, _ := store.GetSetting(db, "terminal_cmd"); v != "" {
		cfg.TerminalCmd = v
	}
	if v, _ := store.GetSetting(db, "explorer_cmd"); v != "" {
		cfg.ExplorerCmd = v
	}
	if v, _ := store.GetSetting(db, "editor_cmd"); v != "" {
		cfg.EditorCmd = v
	}
	return cfg
}

func saveConfig(db *sql.DB, cfg appConfig) {
	_ = store.SetSetting(db, "default_action", cfg.DefaultAction)
	_ = store.SetSetting(db, "terminal_cmd", cfg.TerminalCmd)
	_ = store.SetSetting(db, "explorer_cmd", cfg.ExplorerCmd)
	_ = store.SetSetting(db, "editor_cmd", cfg.EditorCmd)
}

func runCommandTemplate(cmdTemplate, path string) error {
	cmdStr := strings.ReplaceAll(cmdTemplate, "{path}", path)
	cmd := exec.Command("bash", "-lc", cmdStr)
	return cmd.Start()
}

func copyToClipboard(path string) error {
	if _, err := exec.LookPath("wl-copy"); err == nil {
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(path)
		return cmd.Run()
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(path)
		return cmd.Run()
	}
	return fmt.Errorf("clipboard tool not found (need wl-copy or xclip)")
}

func pasteToFocusedInput(text string) {
	if _, err := exec.LookPath("wtype"); err == nil {
		cmd := exec.Command("wtype", "-d", "60", "--", text)
		_ = cmd.Start()
		return
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		cmd := exec.Command("xdotool", "type", "--delay", "1", "--clearmodifiers", text)
		_ = cmd.Start()
		return
	}
}

func performAction(cfg appConfig, selectedPath string) {
	absPath, err := filepath.Abs(selectedPath)
	if err != nil {
		absPath = selectedPath
	}
	path := absPath
	info, err := os.Stat(selectedPath)
	if err == nil && !info.IsDir() {
		path = filepath.Dir(absPath)
	}

	switch cfg.DefaultAction {
	case "terminal":
		_ = runCommandTemplate(cfg.TerminalCmd, path)
	case "explorer":
		_ = runCommandTemplate(cfg.ExplorerCmd, path)
	case "editor":
		_ = runCommandTemplate(cfg.EditorCmd, absPath)
	case "copy":
		if err := copyToClipboard(absPath); err == nil {
			pasteToFocusedInput(absPath)
		}
	default:
		_ = runCommandTemplate(cfg.TerminalCmd, path)
	}
}

func resolveSelectedPath(selectedPath, baseDir string) string {
	if selectedPath == "" {
		return ""
	}
	if filepath.IsAbs(selectedPath) {
		return selectedPath
	}
	cleaned := filepath.Clean(selectedPath)
	baseClean := filepath.Clean(baseDir)
	if baseClean != "" {
		// If cleaned already looks like an absolute path without leading slash, fix it.
		baseNoSlash := strings.TrimPrefix(baseClean, string(filepath.Separator))
		if strings.HasPrefix(cleaned, baseNoSlash) {
			return string(filepath.Separator) + cleaned
		}
	}
	if baseClean == "" {
		return cleaned
	}
	return filepath.Join(baseClean, cleaned)
}

func initialModel(db *sql.DB, cfg appConfig) model {
	ti := textinput.New()
	ti.Placeholder = "Search... (use @tag for scopes)"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	ti.Width = 20

	wd, _ := os.Getwd()

	// Init empty tree
	tm := ui.NewTreeModel([]string{}, 80, 20, make(map[string]bool))
	configInput := textinput.New()
	configInput.Placeholder = "Value"
	configInput.CharLimit = 256
	configInput.Width = 40

	tagInput := textinput.New()
	tagInput.Placeholder = "Tag"
	tagInput.CharLimit = 64
	tagInput.Width = 30
	tagInput.Blur()

	return model{
		db:           db,
		input:        ti,
		tree:         tm,
		currentDir:   wd,
		historyPaths: make(map[string]bool),
		isInitialLoad: true,
		currentDirLoaded: false,
		mode:         modeBrowse,
		config:       cfg,
		configField:  0,
		configEditing: false,
		configInput:  configInput,
		tagEditing:   false,
		tagInput:     tagInput,
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
		if m.mode == modeConfig {
			if m.configEditing {
				switch msg.String() {
				case "esc":
					m.configEditing = false
					m.configInput.SetValue("")
				case "enter":
					m.configEditing = false
					val := m.configInput.Value()
					switch m.configField {
					case 1:
						m.config.TerminalCmd = val
					case 2:
						m.config.ExplorerCmd = val
					case 3:
						m.config.EditorCmd = val
					}
					saveConfig(m.db, m.config)
				default:
					m.configInput, cmd = m.configInput.Update(msg)
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}

			switch msg.String() {
			case "esc":
				m.mode = modeBrowse
				m.configEditing = false
				m.configInput.SetValue("")
				return m, nil
			case "up":
				if m.configField > 0 {
					m.configField--
				}
			case "down":
				if m.configField < 3 {
					m.configField++
				}
			case "left", "right", " ":
				if m.configField == 0 {
					actions := []string{"terminal", "explorer", "editor", "copy"}
					idx := 0
					for i, a := range actions {
						if a == m.config.DefaultAction {
							idx = i
							break
						}
					}
					if msg.String() == "left" {
						idx = (idx + len(actions) - 1) % len(actions)
					} else {
						idx = (idx + 1) % len(actions)
					}
					m.config.DefaultAction = actions[idx]
					saveConfig(m.db, m.config)
				}
			case "enter":
				if m.configField == 0 {
					return m, nil
				}
				m.configEditing = true
				switch m.configField {
				case 1:
					m.configInput.SetValue(m.config.TerminalCmd)
				case 2:
					m.configInput.SetValue(m.config.ExplorerCmd)
				case 3:
					m.configInput.SetValue(m.config.EditorCmd)
				}
				m.configInput.CursorEnd()
			}
			return m, tea.Batch(cmds...)
		}

		if m.mode == modeTags {
			if m.tagEditing {
				switch msg.String() {
				case "esc":
					m.tagEditing = false
					m.tagInput.Blur()
					m.tagInput.SetValue("")
				case "enter":
					tag := strings.TrimSpace(m.tagInput.Value())
					if tag != "" {
						_ = store.AddPathToTag(m.db, tag, m.tagPath)
						m.tagList, _ = store.GetTagsForPath(m.db, m.tagPath)
					}
					m.tagEditing = false
					m.tagInput.Blur()
					m.tagInput.SetValue("")
				default:
					m.tagInput, cmd = m.tagInput.Update(msg)
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}

			switch msg.String() {
			case "esc":
				m.mode = modeBrowse
				m.tagEditing = false
				m.tagInput.SetValue("")
				return m, nil
			case "up":
				if m.tagSelected > 0 {
					m.tagSelected--
				}
			case "down":
				if m.tagSelected < len(m.tagList)-1 {
					m.tagSelected++
				}
			case "a":
				m.tagEditing = true
				m.tagInput.SetValue("")
				m.tagInput.Focus()
				m.tagInput.CursorEnd()
			case "d":
				if len(m.tagList) > 0 && m.tagSelected >= 0 && m.tagSelected < len(m.tagList) {
					_ = store.RemovePathFromTag(m.db, m.tagList[m.tagSelected], m.tagPath)
					m.tagList, _ = store.GetTagsForPath(m.db, m.tagPath)
					if m.tagSelected >= len(m.tagList) {
						m.tagSelected = len(m.tagList) - 1
					}
				}
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+o":
			m.mode = modeConfig
			return m, nil
		case "ctrl+t":
			selectedPath := m.tree.SelectedPath()
			if selectedPath == "" {
				selectedPath = m.currentDir
			}
			if info, err := os.Stat(selectedPath); err == nil && !info.IsDir() {
				selectedPath = filepath.Dir(selectedPath)
			}
			selectedPath = resolveSelectedPath(selectedPath, m.currentDir)
			if absPath, err := filepath.Abs(selectedPath); err == nil {
				selectedPath = absPath
			}
			m.tagPath = selectedPath
			m.tagList, _ = store.GetTagsForPath(m.db, m.tagPath)
			m.tagSelected = 0
			m.tagEditing = false
			m.tagInput.SetValue("")
			m.mode = modeTags
			return m, nil
		case "ctrl+d":
			// Drill down into directory without triggering action
			selectedPath := m.tree.SelectedPath()
			if selectedPath == "" {
				return m, nil
			}
			if info, err := os.Stat(resolveSelectedPath(selectedPath, m.currentDir)); err == nil && info.IsDir() {
				_ = store.UpdateFrecency(m.db, resolveSelectedPath(selectedPath, m.currentDir))
				m.historyPaths[selectedPath] = true
				m.currentDir = resolveSelectedPath(selectedPath, m.currentDir)
				m.input.SetValue("")
				m.activeTag = ""
				m.currentDirLoaded = false
				cmds = append(cmds, loadFiles(m.db, m.currentDir))
			}
			return m, tea.Batch(cmds...)
		case "enter":
			// Handle Selection form Tree
			selectedPath := m.tree.SelectedPath()
			if selectedPath == "" {
				return m, nil
			}

			resolvedPath := resolveSelectedPath(selectedPath, m.currentDir)
			if absPath, err := filepath.Abs(resolvedPath); err == nil {
				resolvedPath = absPath
			}
			// Update History
			_ = store.UpdateFrecency(m.db, resolvedPath)
			// Mark as history (use tree path for highlighting)
			m.historyPaths[selectedPath] = true
			m.selectedPath = resolvedPath
			performAction(m.config, resolvedPath)
			return m, tea.Quit

		case "tab":
			actions := []string{"terminal", "explorer", "editor", "copy"}
			idx := 0
			for i, a := range actions {
				if a == m.config.DefaultAction {
					idx = i
					break
				}
			}
			idx = (idx + 1) % len(actions)
			m.config.DefaultAction = actions[idx]
			saveConfig(m.db, m.config)
		case "shift+tab":
			actions := []string{"terminal", "explorer", "editor", "copy"}
			idx := 0
			for i, a := range actions {
				if a == m.config.DefaultAction {
					idx = i
					break
				}
			}
			idx = (idx + len(actions) - 1) % len(actions)
			m.config.DefaultAction = actions[idx]
			saveConfig(m.db, m.config)

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
	if m.mode == modeConfig {
		return m.configView()
	}
	if m.mode == modeTags {
		return m.tagsView()
	}

	header := m.input.View()
	if m.activeTag != "" {
		tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
		header = fmt.Sprintf("%s %s", tagStyle.Render("[@"+m.activeTag+"]"), m.input.View())
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.tree.View(),
		m.actionTabsView(),
	)
}

func (m model) actionTabsView() string {
	actions := []string{"terminal", "explorer", "editor", "copy"}
	var tabs []string
	for _, a := range actions {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		if a == m.config.DefaultAction {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
		}
		tabs = append(tabs, style.Render("["+a+"]"))
	}
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Tab: cycle action")
	return lipgloss.JoinHorizontal(lipgloss.Left, strings.Join(tabs, " "), "  ", help)
}

func (m model) configView() string {
	title := lipgloss.NewStyle().Bold(true).Render("Config")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Esc: back • Enter: edit/save • Left/Right: cycle default")

	fields := []string{
		fmt.Sprintf("Default action: %s", m.config.DefaultAction),
		fmt.Sprintf("Terminal cmd: %s", m.config.TerminalCmd),
		fmt.Sprintf("Explorer cmd: %s", m.config.ExplorerCmd),
		fmt.Sprintf("Editor cmd: %s", m.config.EditorCmd),
	}

	var lines []string
	for i, f := range fields {
		prefix := "  "
		if i == m.configField {
			prefix = "> "
		}
		lines = append(lines, prefix+f)
	}

	editLine := ""
	if m.configEditing {
		editLine = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render("Edit: ") + m.configInput.View()
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		help,
		strings.Join(lines, "\n"),
		editLine,
	)
}

func (m model) tagsView() string {
	title := lipgloss.NewStyle().Bold(true).Render("Tags")
	pathLine := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(m.tagPath)
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("A: add • D: delete • Esc: back • Enter: save tag")

	var lines []string
	if len(m.tagList) == 0 {
		lines = append(lines, "(no tags)")
	} else {
		for i, t := range m.tagList {
			prefix := "  "
			if i == m.tagSelected {
				prefix = "> "
			}
			lines = append(lines, prefix+t)
		}
	}

	editLine := ""
	if m.tagEditing {
		editLine = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render("Add tag: ") + m.tagInput.View()
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		pathLine,
		help,
		strings.Join(lines, "\n"),
		editLine,
	)
}

func main() {
	// CLI Flags
	addTag := flag.String("add", "", "Add current directory to a tag")
	startAction := flag.String("action", "", "Start with action: terminal|explorer|editor|copy")
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

	// Non-interactive: if args provided, return best match and exit
	if args := flag.Args(); len(args) > 0 {
		query := strings.Join(args, " ")
		cwd, _ := os.Getwd()
		files := buildSearchList(db, cwd)
		results := search.FuzzyHierarchical(files, query)
		if len(results) == 0 {
			os.Exit(1)
		}
		best := resolveSelectedPath(results[0].Path, cwd)
		if absPath, err := filepath.Abs(best); err == nil {
			best = absPath
		}
		fmt.Println(best)
		return
	}

	cfg := loadConfig(db)
	if *startAction != "" {
		switch *startAction {
		case "terminal", "explorer", "editor", "copy":
			cfg.DefaultAction = *startAction
		default:
			fmt.Fprintf(os.Stderr, "invalid action: %s\n", *startAction)
			os.Exit(1)
		}
	}

	p := tea.NewProgram(initialModel(db, cfg), tea.WithAltScreen())
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
