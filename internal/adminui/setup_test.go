package adminui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mberrishdev/wormhole/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func key(w wizard, s string) (wizard, tea.Cmd) {
	var k tea.KeyMsg
	if s == "enter" {
		k.Type = tea.KeyEnter
	} else if s == "down" {
		k.Type = tea.KeyDown
	} else {
		k.Type = tea.KeyRunes
		k.Runes = []rune(s)
	}
	m, cmd := w.Update(k)
	return m.(wizard), cmd
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
