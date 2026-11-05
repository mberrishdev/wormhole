package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mberrishdev/wormhole/internal/client"
	"github.com/mberrishdev/wormhole/internal/config"
	"github.com/mberrishdev/wormhole/internal/protocol"
)

func main() {
	path := flag.String("config", "client.json", "client configuration from your administrator")
	endpoint := flag.String("server", "", "override server host:port")
	local := flag.String("local", "", "local app host:port")
	name := flag.String("name", "", "optional stable tunnel name")
	once := flag.Bool("once", false, "exit after disconnect instead of reconnecting")
	flag.Parse()
	var c config.Client
	if err := config.Read(*path, &c); err != nil {
		log.Fatal(err)
	}
	if *endpoint != "" {
		c.Server = *endpoint
	}
	if *local != "" {
		c.Local = *local
	}
	if *name != "" {
		c.Name = *name
	}
	if err := client.Validate(c); err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	delay := time.Second
	for {
		connected := false
		err := client.Session(ctx, c, func(r protocol.Registration) {
			connected = true
			delay = time.Second
			fmt.Printf("Tunnel ready: %s → %s\n", r.URL, c.Local)
		})
		if ctx.Err() != nil {
			return
		}
		var rejected client.Rejected
		if errors.As(err, &rejected) {
			log.Fatal(err)
		}
		if *once {
			if err != nil {
				log.Fatal(err)
			}
			return
		}
		if connected {
			log.Printf("tunnel disconnected: %v", err)
		} else {
			log.Printf("server unavailable: %v", err)
		}
		log.Printf("reconnecting in %s", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, 10*time.Second)
	}
}
