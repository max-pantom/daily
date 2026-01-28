package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Run launches the TUI dashboard.
func Run(statePath string) error {
	p := tea.NewProgram(newModel(statePath), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
