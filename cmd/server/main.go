package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mberrishdev/wormhole/internal/adminui"
	"github.com/mberrishdev/wormhole/internal/config"
	"github.com/mberrishdev/wormhole/internal/deploy"
	"github.com/mberrishdev/wormhole/internal/server"
	"golang.org/x/crypto/ssh"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wormhole-server:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: wormhole-server <init|serve|tui|status|doctor|health|config validate|service> [options]")
		return nil
	}
	command := args[0]
	args = args[1:]
	if command == "service" {
		return serviceCommand(args)
	}
	if command == "init" {
		c := config.Defaults()
		c.AdvertiseHost = adminui.LANHost()
		f := flag.NewFlagSet("init", flag.ContinueOnError)
		dir := f.String("output", "wormhole-setup", "new output directory (never overwritten)")
		noninteractive := f.Bool("non-interactive", false, "generate from flags without the wizard")
		f.StringVar(&c.Deployment, "deployment", c.Deployment, "compose, native, or kubernetes")
		f.StringVar(&c.Name, "name", c.Name, "server name")
		f.StringVar(&c.ListenIP, "listen-ip", c.ListenIP, "server bind IP; use 127.0.0.1 for local-only testing")
		f.StringVar(&c.AdvertiseHost, "host", c.AdvertiseHost, "existing server LAN IP or reachable SSH hostname")
		f.StringVar(&c.Mode, "mode", c.Mode, "ports or hostname")
		f.IntVar(&c.SSHPort, "ssh-port", c.SSHPort, "SSH port")
		f.IntVar(&c.PortStart, "port-start", c.PortStart, "first tunnel port")
		f.IntVar(&c.PortEnd, "port-end", c.PortEnd, "last tunnel port")
		f.IntVar(&c.HTTPPort, "http-port", c.HTTPPort, "hostname routing HTTP backend port")
		f.StringVar(&c.Domain, "domain", "", "internal wildcard DNS suffix")
		f.StringVar(&c.Scheme, "scheme", "http", "advertised scheme; https requires external TLS termination")
		if err := f.Parse(args); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		if !*noninteractive {
			return adminui.Setup(*dir, c)
		}
		if err := deploy.Generate(*dir, c); err != nil {
			return err
		}
		fmt.Print(deploy.Instructions(*dir, c))
		return nil
	}
	if command == "config" {
		if len(args) == 0 || args[0] != "validate" {
			return fmt.Errorf("use config validate --config <path>")
		}
		command = "validate"
		args = args[1:]
	}
	switch command {
	case "serve", "tui", "status", "doctor", "health", "validate":
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	path := f.String("config", "wormhole-setup/server.json", "server config file")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	c, err := config.Load(*path)
	if err != nil {
		return err
	}
	switch command {
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return server.Run(ctx, c)
	case "tui":
		return adminui.Monitor(c)
	case "status":
		st, err := server.StatusAt(c)
		if err != nil {
			return err
		}
		fmt.Print(string(config.JSON(st)))
	case "validate", "doctor":
		if _, err := config.Secret(c.TokenFile); err != nil {
			return err
		}
		if _, err := config.Secret(c.AdminTokenFile); err != nil {
			return err
		}
		signer, err := config.Signer(c.HostKeyFile)
		if err != nil {
			return err
		}
		fmt.Println("Configuration and credential files are valid.")
		fmt.Println("Host fingerprint:", ssh.FingerprintSHA256(signer.PublicKey()))
		if command == "doctor" {
			st, err := server.StatusAt(c)
			if err != nil {
				return fmt.Errorf("config is valid, but management is unreachable: %w; start serve, then check from a second LAN machine", err)
			}
			fmt.Printf("Server running: %s, %d clients, %d tunnels\n", st.Name, st.Clients, len(st.Tunnels))
			fmt.Println("Client endpoint:", net.JoinHostPort(c.AdvertiseHost, strconv.Itoa(c.SSHPort)))
			fmt.Println("LAN reachability and firewall isolation require testing from another machine.")
		}
	case "health":
		client := http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + c.AdminListen + "/healthz")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("unhealthy: HTTP %d", resp.StatusCode)
		}
		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	}
	return nil
}
