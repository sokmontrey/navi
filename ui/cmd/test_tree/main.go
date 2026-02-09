package main

import (
	"fmt"

	"github.com/montrey/navi/ui"
)

func main() {
	paths := []string{
		"src/main.go",
		"src/utils/helper.go", 
		"src/utils/string.go",
		"README.md",
		"go.mod",
		"go.sum",
	}
	// Note: We expect Input Order to be preserved.
	// If "src/main.go" is first, "src" should be at top.

	
	// Create Tree
	tm := ui.NewTreeModel(paths, 80, 20, make(map[string]bool))
	
	// Print View
	fmt.Println("=== Tree Visualization Test ===")
	view := tm.View()
	fmt.Println(view)
	fmt.Println("===============================")
	
	// Test Layout with deeper paths
	paths2 := []string{
		"a/b/c/d/e/f.go",
		"a/b/c/g/h.go",
		"a/x/y.go",
	}
	tm2 := ui.NewTreeModel(paths2, 80, 20, make(map[string]bool))
	fmt.Println("\n=== Deep Tree Test ===")
	fmt.Println(tm2.View())
	// Test Compression
	paths3 := []string{
		"src/main/java/com/example/App.java",
		"src/main/java/com/example/Utils.java",
	}
	// "src" -> "main" -> "java" -> "com" -> "example" -> [App, Utils]
	// Should become: "src/main/java/com/example" -> [App, Utils]
	
	tm3 := ui.NewTreeModel(paths3, 80, 20, make(map[string]bool))
	fmt.Println("\n=== Compression Test ===")
	fmt.Println(tm3.View())
}
