package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"roundtable/internal/ui"
)

func main() {
	m := ui.New()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Set the program reference for async model response handling
	ui.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
