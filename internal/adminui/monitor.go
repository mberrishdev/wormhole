package adminui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mberrishdev/wormhole/internal/config"
	"github.com/mberrishdev/wormhole/internal/server"
	"sort"
	"strings"
	"time"
)

type statusResult struct {
	status server.Status
	err    error
}
type poll struct{}
type monitor struct {
	cfg                      config.Server
	status                   server.Status
	err                      error
	width, height, page, row int
	loading                  bool
}

func (m monitor) fetch() tea.Cmd {
	return func() tea.Msg {
		s, err := server.StatusAt(m.cfg)
		sort.Slice(s.Tunnels, func(i, j int) bool { return s.Tunnels[i].ID < s.Tunnels[j].ID })
		return statusResult{s, err}
	}
}
func (m monitor) Init() tea.Cmd { return tea.Batch(m.fetch(), refreshTerminal()) }
func (m monitor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case terminalCheck:
		return m, refreshTerminal()
	case tea.WindowSizeMsg:
		changed := m.width != msg.Width || m.height != msg.Height
		m.width, m.height = msg.Width, msg.Height
		if changed {
			return m, tea.ClearScreen
		}
	case statusResult:
		m.loading = false
		m.status, m.err = msg.status, msg.err
		if m.row >= len(m.status.Tunnels) {
			m.row = 0
		}
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return poll{} })
	case poll:
		return m, m.fetch()
	case tea.KeyMsg:
		k := msg.String()
		switch k {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.page = 0
		case "2":
			m.page = 1
		case "3":
			m.page = 2
		case "4":
			m.page = 3
		case "tab", "right":
			m.page = (m.page + 1) % 4
		case "shift+tab", "left":
			m.page = (m.page + 3) % 4
		case "up", "k":
			if len(m.status.Tunnels) > 0 {
				m.row = (m.row + len(m.status.Tunnels) - 1) % len(m.status.Tunnels)
			}
		case "down", "j":
			if len(m.status.Tunnels) > 0 {
				m.row = (m.row + 1) % len(m.status.Tunnels)
			}
		}
	}
	return m, nil
}
func (m monitor) View() string {
	if m.width < 32 || m.height < 12 {
		return "Monitor needs 32 columns, 12 rows.\nq / Ctrl+C to detach.\n"
	}
	if m.width < 90 || m.height < 32 {
		return m.compactView()
	}
	tabs := []string{"1 Overview", "2 Tunnels", "3 Events", "4 Settings"}
	for i, s := range tabs {
		if i == m.page {
			tabs[i] = selected.Render(" " + s + " ")
		} else {
			tabs[i] = muted.Render(" " + s + " ")
		}
	}
	body := strings.Join(tabs, " ") + "\n\n"
	if m.loading {
		body += "Connecting to management API..."
	} else if m.err != nil {
		body += "DISCONNECTED - retrying every 2s\n\n" + m.err.Error() + "\n\nStart the server using `wormhole-server serve`."
	} else {
		switch m.page {
		case 0:
			active := 0
			for _, t := range m.status.Tunnels {
				active += t.Connections
			}
			body += accent.Render("RUNNING / "+m.status.Name) + fmt.Sprintf("\n\n%d clients   %d tunnels   %d active connections\n\n", m.status.Clients, len(m.status.Tunnels), active)
			body += "Client endpoint  " + m.status.SSHAddress + "\nRouting          " + m.status.Mode + "\nStarted          " + m.status.Started.Format(time.RFC3339) + "\n\nHost fingerprint\n" + m.status.Fingerprint
		case 1:
			if len(m.status.Tunnels) == 0 {
				body += "No active tunnels.\n\nConnect a developer client using the generated client.json."
				break
			}
			count := max(1, min(6, m.height-23))
			start := m.row / count * count
			end := min(start+count, len(m.status.Tunnels))
			body += fmt.Sprintf("ACTIVE TUNNELS (%d-%d of %d)\n\n", start+1, end, len(m.status.Tunnels))
			for i := start; i < end; i++ {
				t := m.status.Tunnels[i]
				line := fmt.Sprintf("  %-22s %d connections", t.ID, t.Connections)
				if i == m.row {
					line = selected.Render(">" + line[1:])
				}
				body += line + "\n"
			}
			t := m.status.Tunnels[m.row]
			body += "\nClient   " + t.Owner + "\nLocal    " + t.Local + "\nURL      " + t.URL
		case 2:
			body += "RECENT EVENTS\n\n"
			start := max(0, len(m.status.Events)-8)
			for _, event := range m.status.Events[start:] {
				body += event.Time.Format("15:04:05") + "  " + event.Message + "\n"
			}
		case 3:
			body += fmt.Sprintf("CONFIGURATION / READ ONLY\n\nDeployment  %s\nMode        %s\nSSH port    %d\nPort range  %d-%d\nHTTP port   %d\nDomain      %s\nManagement  %s\n\nEdit the config (or deployment source), validate, then restart.\nCredentials are never shown here.", m.cfg.Deployment, m.cfg.Mode, m.cfg.SSHPort, m.cfg.PortStart, m.cfg.PortEnd, m.cfg.HTTPPort, m.cfg.Domain, m.cfg.AdminListen)
		}
	}
	body += "\n\n" + muted.Render("Tab views  |  up/down select  |  q detach (server stays running)")
	return frame("WORMHOLE / SERVER", body, m.width)
}

func (m monitor) compactView() string {
	line := func(s string) string { return ansi.Truncate(s, m.width-1, "…") }
	lines := []string{accent.Render("WORMHOLE / SERVER"), "1 Overview 2 Tunnels", "3 Events   4 Settings"}
	if m.loading {
		lines = append(lines, "Connecting...")
	} else if m.err != nil {
		lines = append(lines, "DISCONNECTED - retrying", line(m.err.Error()))
	} else {
		switch m.page {
		case 0:
			active := 0
			for _, t := range m.status.Tunnels {
				active += t.Connections
			}
			lines = append(lines, line("RUNNING / "+m.status.Name), fmt.Sprintf("%d clients / %d tunnels", m.status.Clients, len(m.status.Tunnels)), fmt.Sprintf("%d active connections", active), line(m.status.SSHAddress))
		case 1:
			if len(m.status.Tunnels) == 0 {
				lines = append(lines, "No active tunnels.")
				break
			}
			t := m.status.Tunnels[m.row]
			lines = append(lines, line(fmt.Sprintf("%d/%d > %s", m.row+1, len(m.status.Tunnels), t.ID)), line(t.URL), line("Client: "+t.Owner), fmt.Sprintf("%d connections", t.Connections))
		case 2:
			count := max(1, min(8, m.height-7))
			start := max(0, len(m.status.Events)-count)
			for _, event := range m.status.Events[start:] {
				lines = append(lines, line(event.Time.Format("15:04:05")+" "+event.Message))
			}
		case 3:
			lines = append(lines, "Settings (read only)", line("Deployment: "+m.cfg.Deployment), line("Mode: "+m.cfg.Mode), fmt.Sprintf("SSH port: %d", m.cfg.SSHPort))
			if m.cfg.Mode == "ports" {
				lines = append(lines, fmt.Sprintf("Ports: %d-%d", m.cfg.PortStart, m.cfg.PortEnd))
			} else {
				lines = append(lines, line("Domain: "+m.cfg.Domain))
			}
		}
	}
	lines = append(lines, "", "Tab: views  j/k: tunnels", "q: detach (server stays up)")
	return strings.Join(lines, "\n") + "\n"
}
func Monitor(c config.Server) error {
	_, err := tea.NewProgram(monitor{cfg: c, width: 90, height: 35, loading: true}, tea.WithAltScreen()).Run()
	return err
}
