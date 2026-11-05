//go:build !windows

package adminui

import tea "github.com/charmbracelet/bubbletea"

func runProgram(model tea.Model) error {
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
