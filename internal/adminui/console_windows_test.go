//go:build windows

package adminui

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/mberrishdev/wormhole/internal/config"
	"golang.org/x/sys/windows"
)

// Unlike model-only tests, this drives the real Windows console input reader
// through ConPTY, including Enter, character keys, arrows, resize and Ctrl+C.
func TestWindowsConsoleWizard(t *testing.T) {
	for _, native := range []bool{false, true} {
		name := "redirected-handles"
		if native {
			name = "native-handles"
		}
		t.Run(name, func(t *testing.T) { testWindowsConsole(t, native) })
	}
}

func testWindowsConsole(t *testing.T, native bool) {
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer inputRead.Close()
	defer inputWrite.Close()
	outputRead, outputWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer outputRead.Close()
	defer outputWrite.Close()
	var console windows.Handle
	if err = windows.CreatePseudoConsole(windows.Coord{X: 80, Y: 24}, windows.Handle(inputRead.Fd()), windows.Handle(outputWrite.Fd()), 0, &console); err != nil {
		t.Fatal(err)
	}
	defer windows.ClosePseudoConsole(console)
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatal(err)
	}
	defer attributes.Delete()
	// The pseudoconsole attribute takes the HANDLE value itself, not a pointer
	// to it (unlike ordinary attributes). Pass it as a native integer argument.
	update := windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")
	if result, _, callErr := update.Call(uintptr(unsafe.Pointer(attributes.List())), 0, windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(console), unsafe.Sizeof(console), 0, 0); result == 0 {
		t.Fatal(callErr)
	}
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attributes.List()}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{exe, "-test.run=^TestWindowsConsoleHelper$", "-test.timeout=20s"}))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORMHOLE_CONSOLE_HELPER", "1")
	if native {
		t.Setenv("WORMHOLE_NATIVE_HANDLES", "1")
	} else {
		t.Setenv("WORMHOLE_NATIVE_HANDLES", "0")
	}
	t.Setenv("GOTRACEBACK", "all")
	var process windows.ProcessInformation
	if err = windows.CreateProcess(nil, command, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &process); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(process.Process)
	defer windows.CloseHandle(process.Thread)
	defer windows.TerminateProcess(process.Process, 1)
	inputRead.Close()
	outputWrite.Close()
	var mu sync.Mutex
	var output bytes.Buffer
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := outputRead.Read(b)
			mu.Lock()
			output.Write(b[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	await := func(marker string) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			text := output.String()
			mu.Unlock()
			if strings.Contains(text, marker) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		mu.Lock()
		text := output.String()
		mu.Unlock()
		t.Fatalf("Windows console did not reach %q. Output:\n%s", marker, text)
	}
	write := func(s string) {
		t.Helper()
		if _, err := inputWrite.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	await("1/4  Deployment")
	if native {
		write("\x1b[B")
	} else {
		write("j")
	}
	write("\r")
	await("2/4  Access mode")
	write("\r")
	await("3/4  Field")
	write("r")
	await("4/4  Review")
	await("Deployment: native")
	if err = windows.ResizePseudoConsole(console, windows.Coord{X: 110, Y: 40}); err != nil {
		t.Fatal(err)
	}
	await("4 / 4  REVIEW & CONFIRM")
	write("\x03")
	result, err := windows.WaitForSingleObject(process.Process, 3000)
	if err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("Ctrl+C did not exit the wizard: result %d, %v", result, err)
	}
	var code uint32
	if err = windows.GetExitCodeProcess(process.Process, &code); err != nil || code != 0 {
		t.Fatalf("wizard exit code %d, %v", code, err)
	}
}

func TestWindowsConsoleHelper(t *testing.T) {
	if os.Getenv("WORMHOLE_CONSOLE_HELPER") != "1" {
		return
	}
	// Native PowerShell/cmd launches already have console handles; embedded
	// hosts may supply pipes. Exercise both through the production runner.
	if os.Getenv("WORMHOLE_NATIVE_HANDLES") == "1" {
		// GitHub's runner gives its test process redirected standard handles.
		// Attaching to ConPTY does not replace those inherited handles; attach
		// both Go's streams and the Win32 standard handles to the child console.
		input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err = windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(input.Fd())); err != nil {
			t.Fatal(err)
		}
		if err = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(output.Fd())); err != nil {
			t.Fatal(err)
		}
		if err = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(output.Fd())); err != nil {
			t.Fatal(err)
		}
		os.Stdin, os.Stdout, os.Stderr = input, output, output
	}
	watchdog := time.AfterFunc(8*time.Second, func() { panic("Windows TUI watchdog: console stopped responding") })
	defer watchdog.Stop()
	if err := Setup("unused-console-test-output", config.Defaults()); err != nil {
		t.Fatal(err)
	}
}
