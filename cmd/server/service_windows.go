//go:build windows

package main

import (
	"flag"
	"fmt"
	"github.com/mberrishdev/wormhole/internal/config"
	"github.com/mberrishdev/wormhole/internal/server"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"os"
	"path/filepath"
	"time"
)

const serviceName = "Wormhole"

type windowsService struct{ cfg config.Server }

func (w windowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	s, err := server.New(w.cfg)
	if err != nil {
		return true, 1
	}
	if err = s.Start(); err != nil {
		return true, 2
	}
	defer s.Close()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				return false, 0
			}
		case <-tick.C:
			if _, err := server.StatusAt(w.cfg); err != nil {
				return true, 3
			}
		}
	}
}
func serviceCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use service install|start|stop|uninstall --config <path>")
	}
	action := args[0]
	f := flag.NewFlagSet("service", flag.ContinueOnError)
	path := f.String("config", "wormhole-setup/server.json", "server configuration")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if action == "run" {
		isService, err := svc.IsWindowsService()
		if err != nil {
			return err
		}
		if !isService {
			return fmt.Errorf("service run must be started by Windows Service Control Manager")
		}
		c, err := config.Load(*path)
		if err != nil {
			return err
		}
		return svc.Run(serviceName, windowsService{c})
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if action == "install" {
		c, err := config.Load(*path)
		if err != nil {
			return err
		}
		if _, err = server.New(c); err != nil {
			return err
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		abs, err := filepath.Abs(*path)
		if err != nil {
			return err
		}
		s, err := m.CreateService(serviceName, exe, mgr.Config{DisplayName: "Wormhole LAN Tunnel Server", StartType: mgr.StartAutomatic}, "service", "run", "--config", abs)
		if err != nil {
			return err
		}
		defer s.Close()
		fmt.Println("Wormhole service installed. Run: wormhole-server.exe service start")
		return nil
	}
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	switch action {
	case "start":
		return s.Start()
	case "stop":
		_, err = s.Control(svc.Stop)
		return err
	case "uninstall":
		return s.Delete()
	default:
		return fmt.Errorf("unknown service action %q", action)
	}
}
