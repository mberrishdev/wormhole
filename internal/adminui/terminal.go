package adminui

import (
	tea "github.com/charmbracelet/bubbletea"
	"time"
)

// Some embedded terminals update the PTY size without delivering SIGWINCH.
// Query periodically as well as relying on Bubble Tea's native resize events.
type terminalCheck struct{}

func nextTerminalCheck() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return terminalCheck{} })
}

func refreshTerminal() tea.Cmd {
	return tea.Batch(tea.WindowSize(), nextTerminalCheck())
}
