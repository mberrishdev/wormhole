//go:build windows

package adminui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/sys/windows"
)

// A console application can be attached to ConPTY while inheriting redirected
// standard streams. Use the attached console explicitly for interactive UI;
// noninteractive CLI commands keep their ordinary stdin/stdout behavior.
func runProgram(model tea.Model) error {
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("interactive setup needs an attached Windows console: %w", err)
	}
	defer input.Close()
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("cannot open Windows console output: %w", err)
	}
	defer output.Close()
	oldIn, oldOut := os.Stdin, os.Stdout
	oldInput, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return err
	}
	oldOutput, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return err
	}
	defer func() {
		os.Stdin, os.Stdout = oldIn, oldOut
		windows.SetStdHandle(windows.STD_INPUT_HANDLE, oldInput)
		windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, oldOutput)
	}()
	if err = windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(input.Fd())); err != nil {
		return err
	}
	if err = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(output.Fd())); err != nil {
		return err
	}
	os.Stdin, os.Stdout = input, output
	_, err = tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen()).Run()
	return err
}
