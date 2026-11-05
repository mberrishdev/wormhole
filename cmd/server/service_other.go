//go:build !windows

package main

import "fmt"

func serviceCommand(_ []string) error {
	return fmt.Errorf("the service command is for Windows Server; on Linux use the generated wormhole.service, or run serve directly")
}
