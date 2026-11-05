package adminui

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mberrishdev/wormhole/internal/config"
	"github.com/mberrishdev/wormhole/internal/deploy"
)

type generated struct{ err error }
type wizard struct {
	stage, choice, field, width, height int
	cfg                                 config.Server
	dir                                 string
	values                              []string
	editing, busy                       bool
	buffer, message                     string
}

func LANHost() string {
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ip, _, _ := net.ParseCIDR(a.String())
			if ip.To4() != nil && ip.IsPrivate() {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}
func newWizard(dir string, c config.Server) wizard {
	return wizard{dir: dir, cfg: c, width: 90, height: 35, values: []string{c.Name, c.AdvertiseHost, strconv.Itoa(c.SSHPort), strconv.Itoa(c.PortStart), strconv.Itoa(c.PortEnd), strconv.Itoa(c.HTTPPort), c.Domain, dir}}
}
func (w wizard) Init() tea.Cmd { return refreshTerminal() }
func (w wizard) configuration() (config.Server, error) {
	c := w.cfg
	c.Name = w.values[0]
	c.AdvertiseHost = w.values[1]
	nums := []*int{&c.SSHPort, &c.PortStart, &c.PortEnd, &c.HTTPPort}
	for i, n := range nums {
		v, err := strconv.Atoi(w.values[i+2])
		if err != nil {
			return c, fmt.Errorf("ports must be numbers")
		}
		*n = v
	}
	c.Domain = w.values[6]
	return c, c.Validate()
}
func (w wizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case terminalCheck:
		return w, refreshTerminal()
	case tea.WindowSizeMsg:
		changed := w.width != msg.Width || w.height != msg.Height
		w.width, w.height = msg.Width, msg.Height
		if changed {
			return w, tea.ClearScreen
		}
	case generated:
		w.busy = false
		if msg.err != nil {
			w.message = msg.err.Error()
		} else {
			w.stage = 4
			w.message = "Setup files generated successfully."
		}
	case tea.KeyMsg:
		k := msg.String()
		if k == "ctrl+c" {
			return w, tea.Quit
		}
		if w.width < 32 || w.height < 12 {
			if k == "q" {
				return w, tea.Quit
			}
			return w, nil
		}
		if w.busy {
			return w, nil
		}
		if w.editing {
			switch k {
			case "esc":
				w.editing = false
			case "enter":
				w.values[w.field] = strings.TrimSpace(w.buffer)
				w.editing = false
				w.message = ""
			case "ctrl+u":
				w.buffer = ""
			case "backspace":
				r := []rune(w.buffer)
				if len(r) > 0 {
					w.buffer = string(r[:len(r)-1])
				}
			default:
				if msg.Type == tea.KeyRunes && len(w.buffer) < 240 {
					for _, r := range msg.Runes {
						if unicode.IsPrint(r) {
							w.buffer += string(r)
						}
					}
				}
			}
			return w, nil
		}
		if k == "q" {
			return w, tea.Quit
		}
		if k == "esc" {
			if w.stage > 0 && w.stage < 4 {
				w.stage--
				w.choice = 0
				w.message = ""
			} else {
				return w, tea.Quit
			}
			return w, nil
		}
		switch w.stage {
		case 0, 1:
			count := 3
			if w.stage == 1 {
				count = 2
			}
			if k == "up" || k == "k" || k == "shift+tab" {
				w.choice = (w.choice + count - 1) % count
			}
			if k == "down" || k == "j" || k == "tab" {
				w.choice = (w.choice + 1) % count
			}
			if len(k) == 1 && k[0] >= '1' && int(k[0]-'1') < count {
				w.choice = int(k[0] - '1')
			}
			if k == "enter" {
				if w.stage == 0 {
					w.cfg.Deployment = []string{"compose", "native", "kubernetes"}[w.choice]
					if w.cfg.Deployment == "kubernetes" {
						w.choice = 1
					} else {
						w.choice = 0
					}
				} else {
					w.cfg.Mode = []string{"ports", "hostname"}[w.choice]
					if w.cfg.Mode == "hostname" && w.values[6] == "" {
						w.values[6] = "tunnels.company.internal"
					}
					w.choice = 0
				}
				w.stage++
			}
		case 2:
			if k == "up" || k == "k" {
				w.field = (w.field + 7) % 8
			}
			if k == "down" || k == "j" || k == "tab" {
				w.field = (w.field + 1) % 8
			}
			if k == "enter" {
				w.buffer = w.values[w.field]
				w.editing = true
			}
			if k == "r" {
				c, err := w.configuration()
				if err != nil {
					w.message = err.Error()
				} else if w.values[7] == "" {
					w.message = "Output directory is required"
				} else if c.Deployment == "compose" && net.ParseIP(c.AdvertiseHost) == nil {
					w.message = "Compose needs the server's LAN IP for port binding"
				} else {
					w.cfg = c
					w.dir = w.values[7]
					w.stage = 3
					w.message = ""
				}
			}
		case 3:
			if k == "tab" || k == "down" || k == "j" || k == "up" || k == "k" {
				rows := 6
				if w.cfg.Mode == "hostname" {
					rows = 7
				}
				count := max(1, w.height-9)
				w.choice = (w.choice + 1) % ((rows + count - 1) / count)
			}
			if k == "g" || k == "enter" {
				w.busy = true
				c, dir := w.cfg, w.dir
				return w, func() tea.Msg { return generated{deploy.Generate(dir, c)} }
			}
		case 4:
			if k == "enter" {
				return w, tea.Quit
			}
		}
	}
	return w, nil
}
func (w wizard) View() string {
	if w.width < 32 || w.height < 12 {
		return "Setup needs 32 columns, 12 rows.\nCtrl+C to exit.\n"
	}
	if w.width < 90 || w.height < 32 {
		return w.compactView()
	}
	var body string
	switch w.stage {
	case 0, 1:
		title := "1 / 4  CHOOSE DEPLOYMENT"
		options := []string{"Docker Compose - recommended for a LAN server", "Native - foreground, Linux systemd or Windows Service", "Kubernetes - manifests for your company cluster"}
		if w.stage == 1 {
			title = "2 / 4  CHOOSE ACCESS MODE"
			options = []string{"LAN IP + ports - no DNS or certificates required", "Internal domain - requires wildcard DNS and HTTP proxy"}
		}
		body = title + "\n\n"
		for i, s := range options {
			if i == w.choice {
				body += selected.Render("> " + s)
			} else {
				body += "  " + s
			}
			body += "\n\n"
		}
		body += "Arrows / j,k / Tab select   Enter continue   Esc back"
	case 2:
		body = "3 / 4  NETWORK & FILES\n\n"
		labels := []string{"Server name", "LAN IP / SSH host", "SSH port", "First tunnel port", "Last tunnel port", "HTTP backend port", "Domain (hostname mode)", "Output directory"}
		for i, label := range labels {
			v := w.values[i]
			if i == w.field && w.editing {
				v = w.buffer + "█"
			}
			if v == "" {
				v = "(unused)"
			}
			line := fmt.Sprintf("  %-23s %s", label, v)
			if i == w.field {
				line = selected.Render(">" + line[1:])
			}
			body += line + "\n"
		}
		body += "\nIP is the server's EXISTING address; this does not assign an IP.\nHTTP backend port/domain apply only to hostname mode.\n\nEnter edit   Ctrl+U clear   r Review   Esc back"
	case 3:
		c := w.cfg
		body = fmt.Sprintf("4 / 4  REVIEW & CONFIRM\n\nDeployment      %s\nServer name     %s\nClient endpoint %s\nAccess mode     %s\nTunnel ports    %d-%d\nHTTP backend    %d\nDomain          %s\nOutput          %s\n\n", c.Deployment, c.Name, net.JoinHostPort(c.AdvertiseHost, strconv.Itoa(c.SSHPort)), c.Mode, c.PortStart, c.PortEnd, c.HTTPPort, c.Domain, w.dir)
		body += "Will generate config, SSH identity, credentials, client profile,\nand deployment files. Existing directories are never overwritten.\n\n" + accent.Render("[ Generate setup files ]  Enter / g") + "\nEsc back   q cancel"
	case 4:
		abs, _ := filepath.Abs(w.dir)
		body = accent.Render("SETUP GENERATED") + "\n\n" + abs + "\n\nRead NEXT-STEPS.txt for deployment and firewall instructions.\nShare only client.json with authorized developers.\n\n"
		switch w.cfg.Deployment {
		case "compose":
			body += "cd " + abs + "\ndocker compose up -d --build\n\nAttach the TUI inside the container (see NEXT-STEPS.txt)."
		case "native":
			body += "wormhole-server serve --config " + filepath.Join(abs, "server.json") + "\n\nSee NEXT-STEPS.txt for service installation."
		case "kubernetes":
			body += "Set your registry image in kubernetes.json, then:\nkubectl apply -f " + filepath.Join(abs, "kubernetes.json") + "\n\nConfigure private TCP access, DNS and Ingress in your cluster."
		}
		body += "\n\nNo service has been started. Enter / q to exit."
	}
	if w.message != "" {
		body += "\n\n" + w.message
	}
	if w.busy {
		body += "\n\nGenerating files..."
	}
	return frame("WORMHOLE / SETUP", body, w.width)
}

// compactView keeps every step usable in a standard 80x24 terminal and on
// smaller SSH screens. Setup edits one field at a time; review is paginated.
func (w wizard) compactView() string {
	width := w.width - 1
	line := func(s string) string { return ansi.Truncate(s, width, "…") }
	var lines []string
	lines = append(lines, accent.Render("WORMHOLE / SETUP"))
	switch w.stage {
	case 0, 1:
		title := "1/4  Deployment"
		options := []string{"[1] Docker Compose", "[2] Native", "[3] Kubernetes"}
		if w.stage == 1 {
			title = "2/4  Access mode"
			options = []string{"[1] LAN IP + ports", "[2] Internal domain"}
		}
		lines = append(lines, title, "")
		for i, option := range options {
			if i == w.choice {
				lines = append(lines, selected.Render("> "+option))
			} else {
				lines = append(lines, "  "+option)
			}
		}
		lines = append(lines, "", "j/k, arrows or Tab: select", "Enter: continue  Esc: back", "q: quit")
	case 2:
		labels := []string{"Server name", "LAN IP / SSH host", "SSH port", "First tunnel port", "Last tunnel port", "HTTP backend port", "Domain (hostname mode)", "Output directory"}
		value := w.values[w.field]
		if w.editing {
			value = w.buffer + "█"
		}
		if value == "" {
			value = "(unused)"
		}
		if w.editing && ansi.StringWidth(value) > width {
			value = ansi.Cut(value, ansi.StringWidth(value)-width+1, ansi.StringWidth(value))
			value = "…" + value
		}
		lines = append(lines, fmt.Sprintf("3/4  Field %d of %d", w.field+1, len(labels)), "", labels[w.field], line(value), "")
		if w.editing {
			lines = append(lines, "Enter: save  Esc: cancel", "Ctrl+U: clear")
		} else {
			lines = append(lines, "j/k or Tab: previous/next field", "Enter: edit  r: review", "Esc: back  q: quit")
		}
		if w.message != "" {
			lines = append(lines, line(w.message))
		}
	case 3:
		c := w.cfg
		rows := []string{
			"Deployment: " + c.Deployment, "Name: " + c.Name,
			"Server: " + net.JoinHostPort(c.AdvertiseHost, strconv.Itoa(c.SSHPort)),
			"Mode: " + c.Mode,
		}
		if c.Mode == "ports" {
			rows = append(rows, fmt.Sprintf("Ports: %d-%d", c.PortStart, c.PortEnd))
		} else {
			rows = append(rows, "Domain: "+c.Domain, fmt.Sprintf("HTTP port: %d", c.HTTPPort))
		}
		rows = append(rows, "Output: "+w.dir)
		// Reserve room for the title, confirmation and keyboard hints.
		count := max(1, w.height-9)
		start := min(w.choice, (len(rows)-1)/count) * count
		end := min(start+count, len(rows))
		lines = append(lines, fmt.Sprintf("4/4  Review (%d/%d)", start/count+1, (len(rows)+count-1)/count), "")
		for _, row := range rows[start:end] {
			lines = append(lines, line(row))
		}
		lines = append(lines, accent.Render("Enter/g: Generate setup files"))
		if len(rows) > count {
			lines = append(lines, "Tab: more settings")
		}
		lines = append(lines, "Esc: back  q: cancel")
		if w.message != "" {
			lines = append(lines, line(w.message))
		}
	case 4:
		lines = append(lines, accent.Render("SETUP GENERATED"), "", line(w.dir), "", "Read NEXT-STEPS.txt to start.", "Keep credentials private.", "", "Enter/q: exit")
	}
	if w.busy {
		lines = append(lines, "Generating files...")
	}
	return strings.Join(lines, "\n") + "\n"
}
func frame(title, body string, width int) string {
	width = min(width-4, 100)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width).Render(accent.Bold(true).Render(title)+"\n\n"+lipgloss.NewStyle().Width(width-4).Render(body)) + "\n"
}
func Setup(dir string, c config.Server) error {
	_, err := tea.NewProgram(newWizard(dir, c), tea.WithAltScreen()).Run()
	return err
}
