package adminui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mberrishdev/wormhole/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func key(w wizard, s string) (wizard, tea.Cmd) {
	var k tea.KeyMsg
	if s == "enter" {
		k.Type = tea.KeyEnter
	} else if s == "down" {
		k.Type = tea.KeyDown
	} else if s == "tab" {
		k.Type = tea.KeyTab
	} else {
		k.Type = tea.KeyRunes
		k.Runes = []rune(s)
	}
	m, cmd := w.Update(k)
	return m.(wizard), cmd
}

func TestCompactWizardFitsTerminalAcrossSteps(t *testing.T) {
	for _, size := range [][2]int{{32, 12}, {40, 16}, {60, 20}, {80, 24}, {100, 40}} {
		for stage := 0; stage <= 4; stage++ {
			t.Run(fmt.Sprintf("%dx%d/step%d", size[0], size[1], stage), func(t *testing.T) {
				w := newWizard("wormhole-setup", config.Defaults())
				w.width, w.height = size[0], size[1]
				w.stage = stage
				w.field = 7
				w.values[7] = strings.Repeat("long-path-界", 20)
				w.buffer = w.values[7]
				w.editing = stage == 2
				w.message = "A long validation error: " + strings.Repeat("details ", 20)
				if size[0] >= 90 {
					w.message = "Validation failed"
				}
				view := w.View()
				if strings.Contains(view, "Resize") {
					t.Fatal("normal terminal incorrectly blocked")
				}
				if width := lipgloss.Width(view); width > w.width {
					t.Fatalf("view width %d exceeds %d", width, w.width)
				}
				if height := lipgloss.Height(view); height > w.height {
					t.Fatalf("view height %d exceeds %d\n%s", height, w.height, view)
				}
			})
		}
	}
}

func TestWizardKeyboardAlternativesAndReviewPaging(t *testing.T) {
	w := newWizard("wormhole-setup", config.Defaults())
	w.width, w.height = 32, 12
	w, _ = key(w, "j")
	if w.choice != 1 {
		t.Fatal("j did not select native")
	}
	w, _ = key(w, "k")
	if w.choice != 0 {
		t.Fatal("k did not select compose")
	}
	w, _ = key(w, "tab")
	if w.choice != 1 {
		t.Fatal("Tab did not select native")
	}
	w, _ = key(w, "3")
	w, _ = key(w, "enter")
	if w.cfg.Deployment != "kubernetes" {
		t.Fatal("numeric deployment selection failed")
	}
	w, _ = key(w, "enter")
	w, _ = key(w, "r")
	if w.stage != 3 {
		t.Fatal("review unavailable")
	}
	first := w.View()
	w, _ = key(w, "tab")
	if w.View() == first {
		t.Fatal("compact review did not paginate")
	}
	if !strings.Contains(w.View(), "Generate setup files") {
		t.Fatal("confirmation hidden in compact view")
	}
}

func TestCompactMonitorFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{32, 12}, {40, 16}, {80, 24}} {
		for page := 0; page < 4; page++ {
			m := monitor{cfg: config.Defaults(), width: size[0], height: size[1], page: page}
			view := m.View()
			if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
				t.Fatalf("monitor page %d does not fit %dx%d", page, m.width, m.height)
			}
		}
	}
}

func TestResizePreservesWizardState(t *testing.T) {
	w := newWizard("wormhole-setup", config.Defaults())
	w.stage = 2
	w.field = 7
	w.editing = true
	w.buffer = "draft-path"
	for _, size := range [][2]int{{120, 40}, {80, 24}, {32, 12}, {120, 40}} {
		m, _ := w.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		w = m.(wizard)
		if w.stage != 2 || w.field != 7 || !w.editing || w.buffer != "draft-path" {
			t.Fatal("resize lost setup state")
		}
		if lipgloss.Width(w.View()) > w.width || lipgloss.Height(w.View()) > w.height {
			t.Fatalf("resize did not adapt to %dx%d", size[0], size[1])
		}
	}
	_, cmd := w.Update(terminalCheck{})
	if cmd == nil {
		t.Fatal("periodic size query was not rescheduled")
	}
}
func TestSetupRequiresReviewAndExplicitConfirmation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "setup")
	w := newWizard(dir, config.Defaults())
	w, _ = key(w, "down")
	w, _ = key(w, "down")
	w, _ = key(w, "enter")
	if w.cfg.Deployment != "kubernetes" || w.stage != 1 {
		t.Fatal("deployment selection did not advance")
	}
	w, _ = key(w, "enter")
	if w.cfg.Mode != "hostname" || w.stage != 2 {
		t.Fatal("access mode did not advance")
	}
	w, _ = key(w, "r")
	if w.stage != 3 {
		t.Fatal("review not reachable", w.message)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("files created before confirmation")
	}
	w, cmd := key(w, "g")
	if cmd == nil || !w.busy {
		t.Fatal("confirmation did not generate setup")
	}
	result := cmd()
	m, _ := w.Update(result)
	w = m.(wizard)
	if w.stage != 4 {
		t.Fatal("generation failed:", w.message)
	}
	if _, err := os.Stat(filepath.Join(dir, "kubernetes.json")); err != nil {
		t.Fatal(err)
	}
}
