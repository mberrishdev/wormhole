package main

import (
	"flag"
	"fmt"
	"github.com/mberrishdev/wormhole/internal/adminui"
	"github.com/mberrishdev/wormhole/internal/config"
	"os"
)

func main() {
	demo := flag.Bool("demo", false, "show the read-only sample UX")
	path := flag.String("config", "wormhole-setup/server.json", "server configuration")
	flag.Parse()
	var err error
	if *demo {
		err = adminui.Demo()
	} else {
		var c config.Server
		c, err = config.Load(*path)
		if err == nil {
			err = adminui.Monitor(c)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
