package main

import (
	"fmt"
	"io"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"roundtable/internal/ui"
)

func main() {
	// Silence log output during TUI operation - it corrupts the display
	log.SetOutput(io.Discard)

	m := ui.New()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Set the program reference for async model response handling
	ui.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
