// server-tui previews the proposed admin experience without running a server.
package adminui

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	accent   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "29", Dark: "79"})
	muted    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	selected = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "22", Dark: "121"}).Background(lipgloss.AdaptiveColor{Light: "194", Dark: "236"})
)

type tunnel struct{ name, owner, local, port, connections string }

var demoTunnels = []tunnel{
	{"frontend", "mariam-laptop", "localhost:3000", "8081", "4"},
	{"api", "giorgi-workstation", "localhost:8000", "8082", "2"},
	{"storybook", "design-mac", "localhost:6006", "8083", "1"},
}

type model struct {
	width, height    int
	page, row, field int
	editing, review  bool
	values           [3]string
	buffer, message  string
}

func initialModel() model {
	return model{width: 90, height: 32, values: [3]string{"lan-relay", "192.168.1.100", "2222"}}
}

func (m model) Init() tea.Cmd { return refreshTerminal() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case terminalCheck:
		return m, refreshTerminal()
	case tea.WindowSizeMsg:
		changed := m.width != msg.Width || m.height != msg.Height
		m.width, m.height = msg.Width, msg.Height
		if changed {
			return m, tea.ClearScreen
		}
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.editing {
			switch key {
			case "esc":
				m.editing = false
				m.message = "Edit cancelled."
			case "enter":
				if err := validateField(m.field, m.buffer); err != nil {
					m.message = err.Error()
					return m, nil
				}
				m.values[m.field] = m.buffer
				m.editing = false
				m.message = "Draft updated. Nothing written to disk."
			case "backspace", "ctrl+h":
				runes := []rune(m.buffer)
				if len(runes) > 0 {
					m.buffer = string(runes[:len(runes)-1])
				}
			case "ctrl+u":
				m.buffer = ""
			default:
				if msg.Type == tea.KeyRunes && len([]rune(m.buffer))+len(msg.Runes) <= 120 {
					for _, r := range msg.Runes {
						if unicode.IsPrint(r) {
							m.buffer += string(r)
						}
					}
				}
			}
			return m, nil
		}
		switch key {
		case "q", "esc":
			if m.review {
				m.review = false
			} else {
				return m, tea.Quit
			}
		case "1", "2", "3":
			m.page, _ = strconv.Atoi(key)
			m.page--
			m.review = false
			m.message = ""
		case "tab", "right":
			m.page = (m.page + 1) % 3
			m.review = false
			m.message = ""
		case "shift+tab", "left":
			m.page = (m.page + 2) % 3
			m.review = false
			m.message = ""
		case "down", "j":
			if m.page == 1 {
				m.row = (m.row + 1) % len(demoTunnels)
			}
			if m.page == 2 && !m.review {
				m.field = (m.field + 1) % 3
			}
		case "up", "k":
			if m.page == 1 {
				m.row = (m.row + len(demoTunnels) - 1) % len(demoTunnels)
			}
			if m.page == 2 && !m.review {
				m.field = (m.field + 2) % 3
			}
		case "enter":
			if m.page == 2 && !m.review {
				m.editing = true
				m.buffer = m.values[m.field]
				m.message = "Type to edit; Ctrl+U clears the field."
			}
		case "r":
			if m.page == 2 {
				m.review = !m.review
				m.message = ""
			}
		}
	}
	return m, nil
}

func validateField(field int, value string) error {
	if value == "" {
		return fmt.Errorf("Value cannot be empty.")
	}
	switch field {
	case 0:
		for _, r := range value {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return fmt.Errorf("Use letters, numbers, and hyphens for the name.")
			}
		}
	case 1:
		ip := net.ParseIP(value)
		if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || !ip.IsPrivate() {
			return fmt.Errorf("Enter a private LAN IP, for example 192.168.1.100.")
		}
	case 2:
		port, err := strconv.Atoi(value)
		if err != nil || port < 1024 || port > 65535 {
			return fmt.Errorf("Use an unprivileged port from 1024 to 65535.")
		}
		if port >= 8081 && port <= 8090 {
			return fmt.Errorf("SSH port conflicts with tunnel range 8081-8090.")
		}
	}
	return nil
}

func (m model) overview() string {
	return accent.Render("3 clients       3 / 10 tunnels       7 connections") + "\n\n" +
		muted.Render("SERVER / SAMPLE DATA") + "\n" +
		"Client listener   0.0.0.0:2222 (SSH)\n" +
		"Tunnel endpoints  192.168.1.100:8081-8090\n" +
		"Management        127.0.0.1:4041 (local only)\n\n" +
		muted.Render("RECENT EVENTS") + "\n" +
		"14:32:08  CONNECTED  mariam-laptop\n" +
		"14:32:09  PUBLISHED  frontend -> :8081\n" +
		"14:35:41  PUBLISHED  api -> :8082\n\n" +
		muted.Render("Sample server state is fixed. Setup edits a separate draft.")
}

func (m model) tunnels() string {
	var b strings.Builder
	b.WriteString(muted.Render("ACTIVE TUNNELS / SAMPLE DATA") + "\n\n")
	for i, t := range demoTunnels {
		line := fmt.Sprintf("  %-11s :%s  %s connections", t.name, t.port, t.connections)
		if i == m.row {
			line = selected.Render(">" + line[1:])
		}
		b.WriteString(line + "\n")
	}
	t := demoTunnels[m.row]
	b.WriteString("\n" + muted.Render("SELECTED TUNNEL") + "\n")
	b.WriteString(fmt.Sprintf("Client     %s\nLocal app  %s\nLAN URL    http://%s\n\n", t.owner, t.local, net.JoinHostPort("192.168.1.100", t.port)))
	b.WriteString(muted.Render("Visitors: anyone who can reach the tunnel port."))
	return b.String()
}

func (m model) setup() string {
	if m.review {
		return muted.Render("REVIEW / PROPOSED CONFIGURATION") + "\n\n" +
			fmt.Sprintf("name: %s\nssh_listen: %s\nadvertised_host: %s\ntunnel_ports: 8081-8090\ndeployment: docker-compose\n", m.values[0], net.JoinHostPort("0.0.0.0", m.values[2]), m.values[1]) + "\n" +
			accent.Render("Draft only. No files or credentials generated.") + "\n" +
			muted.Render("The existing server cannot load this configuration yet.") + "\n\n" +
			"r / Esc  Back to setup"
	}
	var b strings.Builder
	b.WriteString(muted.Render("SETUP / DOCKER COMPOSE ON A LAN SERVER") + "\n\n")
	labels := []string{"Server name", "LAN IP", "SSH port"}
	for i, label := range labels {
		value := m.values[i]
		if m.editing && i == m.field {
			value = m.buffer + "█"
		}
		line := fmt.Sprintf("  %-13s [%s]", label, value)
		if i == m.field {
			line = selected.Render(">" + line[1:])
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n  Deployment     Docker Compose\n  Publishing     HTTP, ports 8081-8090\n  Credentials    Generation planned; not in this demo\n\n")
	b.WriteString(accent.Render("Enter  Edit selected field    r  Review draft"))
	if m.message != "" {
		b.WriteString("\n\n" + m.message)
	}
	return b.String()
}

func (m model) View() string {
	if m.width < 58 || m.height < 27 {
		return fmt.Sprintf("WORMHOLE / TUI DEMO\n\nResize to at least 58 columns x 27 rows.\nCurrent: %d x %d\n\nq / Ctrl+C  Quit\n", m.width, m.height)
	}
	var tabs []string
	for i, label := range []string{"1 Overview", "2 Tunnels", "3 Setup"} {
		if m.page == i {
			tabs = append(tabs, selected.Render(" "+label+" "))
		} else {
			tabs = append(tabs, muted.Render(" "+label+" "))
		}
	}
	body := m.overview()
	if m.page == 1 {
		body = m.tunnels()
	}
	if m.page == 2 {
		body = m.setup()
	}
	width := min(m.width-6, 94)
	content := accent.Bold(true).Render("WORMHOLE / SERVER") + "\n" +
		muted.Render("INTERACTIVE DEMO - no server connection") + "\n\n" +
		strings.Join(tabs, "  ") + "\n\n" + body
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "245", Dark: "240"}).Padding(1, 2).Width(width).Render(lipgloss.NewStyle().Width(width - 4).Render(content))
	help := "1-3 / Tab views  |  up/down select  |  q quit"
	if m.editing {
		help = "Enter apply  |  Esc cancel  |  Ctrl+U clear  |  Ctrl+C quit"
	}
	return box + "\n" + muted.Render(help) + "\n"
}

func Demo() error {
	_, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run()
	return err
}
