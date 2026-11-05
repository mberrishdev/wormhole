package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mberrishdev/wormhole/internal/client"
	"github.com/mberrishdev/wormhole/internal/config"
	"github.com/mberrishdev/wormhole/internal/deploy"
	"github.com/mberrishdev/wormhole/internal/protocol"
	"github.com/mberrishdev/wormhole/internal/server"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func setup(t *testing.T, mode string) (*server.Server, config.Server, config.Client) {
	t.Helper()
	c := config.Defaults()
	c.Deployment = "native"
	c.ListenIP = "127.0.0.1"
	c.AdvertiseHost = "127.0.0.1"
	c.SSHPort = freePort(t)
	c.HTTPPort = freePort(t)
	c.PortStart = freePort(t)
	c.PortEnd = c.PortStart
	c.AdminListen = fmt.Sprintf("127.0.0.1:%d", freePort(t))
	c.Mode = mode
	c.Domain = "tunnels.company.internal"
	dir := filepath.Join(t.TempDir(), "setup")
	if err := deploy.Generate(dir, c); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(filepath.Join(dir, "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cc config.Client
	if err = config.Read(filepath.Join(dir, "client.json"), &cc); err != nil {
		t.Fatal(err)
	}
	s, err := server.New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, c, cc
}
func connect(t *testing.T, c config.Client) (protocol.Registration, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan protocol.Registration, 1)
	done := make(chan error, 1)
	go func() { done <- client.Session(ctx, c, func(r protocol.Registration) { ready <- r }) }()
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("client did not stop")
		}
	}
	select {
	case r := <-ready:
		t.Cleanup(stop)
		return r, stop
	case err := <-done:
		cancel()
		t.Fatalf("registration failed: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("registration timed out")
	}
	return protocol.Registration{}, nil
}
func waitFor(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func TestLANPortTunnelLifecycle(t *testing.T) {
	s, c, cc := setup(t, "ports")
	payload := strings.Repeat("wormhole-data-", 16000)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Connection", "close")
		w.Write(b)
	}))
	defer app.Close()
	cc.Local = strings.TrimPrefix(app.URL, "http://")
	cc.Name = "frontend"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan protocol.Registration, 1)
	done := make(chan error, 1)
	go func() { done <- client.Session(ctx, cc, func(r protocol.Registration) { ready <- r }) }()
	var reg protocol.Registration
	select {
	case reg = <-ready:
	case err := <-done:
		t.Fatalf("client failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("registration timeout")
	}
	visitor := http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 3; i++ {
		resp, err := visitor.Post(reg.URL, "text/plain", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || string(b) != payload {
			t.Fatalf("response mismatch: %d bytes, error %v", len(b), err)
		}
	}
	st, err := server.StatusAt(c)
	if err != nil || len(st.Tunnels) != 1 || st.Clients != 1 {
		t.Fatalf("unexpected live status: %+v, %v", st, err)
	}
	resp, err := visitor.Get("http://" + c.AdminListen + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated status: %d", resp.StatusCode)
	}
	// A second client cannot exceed the configured port capacity.
	extra := cc
	extra.Name = "other"
	rejectCtx, rejectCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer rejectCancel()
	if err = client.Session(rejectCtx, extra, nil); err == nil || !strings.Contains(err.Error(), "no available tunnel ports") {
		t.Fatalf("expected capacity error, got %v", err)
	}
	idle, err := net.Dial("tcp", strings.TrimPrefix(reg.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	waitFor(t, func() bool { st := s.Snapshot(); return len(st.Tunnels) == 1 && st.Tunnels[0].Connections > 0 })
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("client did not stop")
	}
	waitFor(t, func() bool { return len(s.Snapshot().Tunnels) == 0 })
	idle.SetReadDeadline(time.Now().Add(time.Second))
	var one [1]byte
	if _, err := idle.Read(one[:]); err == nil {
		t.Fatal("idle visitor was not closed on disconnect")
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatal("idle visitor remained open after disconnect")
	}
	// Closed tunnel ports can be allocated again, preserving the server identity.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	ready2 := make(chan protocol.Registration, 1)
	done2 := make(chan error, 1)
	go func() { done2 <- client.Session(ctx2, cc, func(r protocol.Registration) { ready2 <- r }) }()
	select {
	case r := <-ready2:
		if r.URL != reg.URL {
			t.Errorf("port not reused: %s", r.URL)
		}
	case err := <-done2:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("port was not reusable")
	}
	cancel2()
	select {
	case <-done2:
	case <-time.After(3 * time.Second):
		t.Fatal("reconnected client did not stop")
	}
}

func TestHostnameRoutingAndAuthentication(t *testing.T) {
	_, c, cc := setup(t, "hostname")
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "local-app:"+r.URL.Path) }))
	defer app.Close()
	cc.Local = strings.TrimPrefix(app.URL, "http://")
	cc.Name = "api"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan protocol.Registration, 1)
	done := make(chan error, 1)
	go func() { done <- client.Session(ctx, cc, func(r protocol.Registration) { ready <- r }) }()
	select {
	case reg := <-ready:
		if reg.URL != "http://api.tunnels.company.internal" {
			t.Fatal(reg.URL)
		}
	case err := <-done:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("client timeout")
	}
	visitor := http.Client{Timeout: 3 * time.Second}
	request, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/hello", c.HTTPPort), nil)
	request.Host = "api.tunnels.company.internal"
	resp, err := visitor.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "local-app:/hello" {
		t.Fatalf("wrong hostname route: %s", body)
	}
	request.Host = "api.unrelated.internal"
	resp, err = visitor.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unrelated host accepted: %d", resp.StatusCode)
	}
	for name, bad := range map[string]config.Client{"token": cc, "host-key": cc} {
		t.Run(name, func(t *testing.T) {
			if name == "token" {
				bad.Token = strings.Repeat("x", 64)
			} else {
				bad.Fingerprint = "SHA256:wrong"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := client.Session(ctx, bad, nil)
			if err == nil {
				t.Fatal("bad credentials accepted")
			}
			if strings.Contains(err.Error(), bad.Token) {
				t.Fatal("credential leaked into error")
			}
		})
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("client did not stop")
	}
}

func TestLocalAppUnavailableDoesNotHangVisitor(t *testing.T) {
	_, _, cc := setup(t, "ports")
	cc.Local = fmt.Sprintf("127.0.0.1:%d", freePort(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan protocol.Registration, 1)
	done := make(chan error, 1)
	go func() { done <- client.Session(ctx, cc, func(r protocol.Registration) { ready <- r }) }()
	var reg protocol.Registration
	select {
	case reg = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	h := http.Client{Timeout: 2 * time.Second}
	resp, err := h.Get(reg.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("unavailable app unexpectedly succeeded")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("client did not stop")
	}
}

func TestSnapshotJSONDoesNotExposeCredentials(t *testing.T) {
	s, _, cc := setup(t, "ports")
	b, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), cc.Token) {
		t.Fatal("client token in management status")
	}
}

func TestPortTunnelPreservesHalfClose(t *testing.T) {
	_, _, cc := setup(t, "ports")
	app, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	go func() {
		conn, err := app.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		data, err := io.ReadAll(conn)
		if err == nil {
			conn.Write(append([]byte("reply:"), data...))
		}
	}()
	cc.Local = app.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan protocol.Registration, 1)
	done := make(chan error, 1)
	go func() { done <- client.Session(ctx, cc, func(r protocol.Registration) { ready <- r }) }()
	var reg protocol.Registration
	select {
	case reg = <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("registration timeout")
	}
	visitor, err := net.Dial("tcp", strings.TrimPrefix(reg.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer visitor.Close()
	visitor.SetDeadline(time.Now().Add(3 * time.Second))
	visitor.Write([]byte("request"))
	visitor.(*net.TCPConn).CloseWrite()
	body, err := io.ReadAll(visitor)
	if err != nil || string(body) != "reply:request" {
		t.Fatalf("half-close lost response: %q, %v", body, err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("client did not stop")
	}
}
