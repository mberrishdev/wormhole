package client

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mberrishdev/wormhole/internal/config"
	"github.com/mberrishdev/wormhole/internal/protocol"
	"github.com/mberrishdev/wormhole/internal/tunnel"
	"golang.org/x/crypto/ssh"
)

func Validate(c config.Client) error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported client configuration version")
	}
	if _, _, err := net.SplitHostPort(c.Server); err != nil {
		return fmt.Errorf("server must be host:port")
	}
	if _, _, err := net.SplitHostPort(c.Local); err != nil {
		return fmt.Errorf("local must be host:port")
	}
	if len(c.Token) < 32 {
		return fmt.Errorf("client token must contain at least 32 characters")
	}
	if !strings.HasPrefix(c.Fingerprint, "SHA256:") {
		return fmt.Errorf("host_fingerprint is required; obtain client.json from your administrator")
	}
	if c.Name != "" && !config.ValidName(c.Name) {
		return fmt.Errorf("name must be 1-48 lowercase letters, numbers or hyphens")
	}
	return nil
}

type Rejected struct{ Reason string }

func (e Rejected) Error() string { return e.Reason }

// Session runs one tunnel until it disconnects or the context is cancelled.
func Session(ctx context.Context, c config.Client, ready func(protocol.Registration)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := Validate(c); err != nil {
		return err
	}
	owner, _ := os.Hostname()
	if len(owner) > 64 {
		owner = owner[:64]
	}
	if owner == "" {
		owner = "developer"
	}
	sshConfig := &ssh.ClientConfig{User: owner, Auth: []ssh.AuthMethod{ssh.Password(c.Token)}, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if subtle.ConstantTimeCompare([]byte(ssh.FingerprintSHA256(key)), []byte(c.Fingerprint)) != 1 {
			return Rejected{"server host key does not match the configured fingerprint"}
		}
		return nil
	}}
	raw, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", c.Server)
	if err != nil {
		return err
	}
	defer raw.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			raw.Close()
		case <-done:
		}
	}()
	raw.SetDeadline(time.Now().Add(10 * time.Second))
	cc, chs, reqs, err := ssh.NewClientConn(raw, c.Server, sshConfig)
	if err != nil {
		var rejected Rejected
		if errors.As(err, &rejected) || strings.Contains(err.Error(), "unable to authenticate") {
			return Rejected{fmt.Sprintf("SSH handshake failed: %v", err)}
		}
		return err
	}
	conn := ssh.NewClient(cc, chs, reqs)
	defer conn.Close()
	raw.SetDeadline(time.Time{})
	channels := conn.HandleChannelOpen("tunnel")
	registration, err := json.Marshal(protocol.Request{Name: c.Name, Local: c.Local})
	if err != nil {
		return err
	}
	// Bound registration even when a peer accepts SSH but does not implement Wormhole.
	raw.SetDeadline(time.Now().Add(10 * time.Second))
	ok, reply, err := conn.SendRequest(protocol.Forward, true, registration)
	if err != nil {
		return err
	}
	raw.SetDeadline(time.Time{})
	if !ok {
		return Rejected{"tunnel rejected: " + string(reply)}
	}
	var reg protocol.Registration
	if json.Unmarshal(reply, &reg) != nil || reg.Version != 1 || reg.ID == "" || reg.URL == "" {
		return Rejected{"invalid registration reply; update both server and client"}
	}
	if ready != nil {
		ready(reg)
	}
	go heartbeat(conn, done)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ch, ok := <-channels:
			if !ok {
				return io.EOF
			}
			go forward(ctx, ch, c.Local)
		}
	}
}
func heartbeat(conn *ssh.Client, done <-chan struct{}) {
	timer := time.NewTicker(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-done:
			return
		case <-timer.C:
			result := make(chan bool, 1)
			go func() { ok, _, err := conn.SendRequest("keepalive@wormhole", true, nil); result <- ok && err == nil }()
			timeout := time.NewTimer(10 * time.Second)
			select {
			case <-done:
				timeout.Stop()
				return
			case ok := <-result:
				timeout.Stop()
				if !ok {
					conn.Close()
					return
				}
			case <-timeout.C:
				conn.Close()
				return
			}
		}
	}
}
func forward(ctx context.Context, in ssh.NewChannel, local string) {
	upstream, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", local)
	if err != nil {
		in.Reject(ssh.ConnectionFailed, "local application is unavailable")
		log.Printf("cannot reach local app %s: %v", local, err)
		return
	}
	defer upstream.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			upstream.Close()
		case <-done:
		}
	}()
	ch, requests, err := in.Accept()
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	tunnel.Pipe(ch, upstream)
}
