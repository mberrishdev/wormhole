package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mberrishdev/wormhole/internal/config"
	"github.com/mberrishdev/wormhole/internal/protocol"
	"github.com/mberrishdev/wormhole/internal/tunnel"
	"golang.org/x/crypto/ssh"
)

type Tunnel struct {
	ID          string    `json:"id"`
	Owner       string    `json:"owner"`
	Local       string    `json:"local"`
	URL         string    `json:"url"`
	Connections int       `json:"connections"`
	Started     time.Time `json:"started"`
	conn        *ssh.ServerConn
	listener    net.Listener
}
type Event struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}
type Status struct {
	Name        string    `json:"name"`
	Mode        string    `json:"mode"`
	SSHAddress  string    `json:"ssh_address"`
	Fingerprint string    `json:"host_fingerprint"`
	Started     time.Time `json:"started"`
	Clients     int       `json:"clients"`
	Tunnels     []Tunnel  `json:"tunnels"`
	Events      []Event   `json:"events"`
}
type Server struct {
	cfg                                      config.Server
	token, adminToken                        string
	signer                                   ssh.Signer
	mu                                       sync.Mutex
	tunnels                                  map[string]*Tunnel
	clients                                  map[*ssh.ServerConn]bool
	sockets                                  map[net.Conn]bool
	events                                   []Event
	started                                  time.Time
	sshListener, adminListener, httpListener net.Listener
	adminHTTP, publicHTTP                    *http.Server
	stopped                                  bool
	once                                     sync.Once
}

func New(c config.Server) (*Server, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	token, err := config.Secret(c.TokenFile)
	if err != nil {
		return nil, err
	}
	admin, err := config.Secret(c.AdminTokenFile)
	if err != nil {
		return nil, err
	}
	if token == admin {
		return nil, fmt.Errorf("client and management credentials must differ")
	}
	signer, err := config.Signer(c.HostKeyFile)
	if err != nil {
		return nil, err
	}
	return &Server{cfg: c, token: token, adminToken: admin, signer: signer, tunnels: map[string]*Tunnel{}, clients: map[*ssh.ServerConn]bool{}, sockets: map[net.Conn]bool{}}, nil
}
func (s *Server) eventLocked(message string) {
	s.events = append(s.events, Event{time.Now(), message})
	if len(s.events) > 30 {
		s.events = s.events[len(s.events)-30:]
	}
}
func (s *Server) Start() error {
	var err error
	s.sshListener, err = net.Listen("tcp", net.JoinHostPort(s.cfg.ListenIP, strconv.Itoa(s.cfg.SSHPort)))
	if err != nil {
		return fmt.Errorf("SSH listener: %w", err)
	}
	s.adminListener, err = net.Listen("tcp", s.cfg.AdminListen)
	if err != nil {
		s.Close()
		return fmt.Errorf("management listener: %w", err)
	}
	if s.cfg.Mode == "hostname" {
		s.httpListener, err = net.Listen("tcp", net.JoinHostPort(s.cfg.ListenIP, strconv.Itoa(s.cfg.HTTPPort)))
		if err != nil {
			s.Close()
			return err
		}
	}
	s.started = time.Now()
	s.adminHTTP = &http.Server{Handler: s.adminHandler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, WriteTimeout: 10 * time.Second}
	go s.serveHTTP(s.adminHTTP, s.adminListener)
	if s.httpListener != nil {
		s.publicHTTP = &http.Server{Handler: http.HandlerFunc(s.proxyHTTP), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
		go s.serveHTTP(s.publicHTTP, s.httpListener)
	}
	go s.acceptClients()
	log.Printf("server %s listening for clients on %s; host fingerprint %s", s.cfg.Name, s.sshListener.Addr(), ssh.FingerprintSHA256(s.signer.PublicKey()))
	return nil
}
func (s *Server) serveHTTP(h *http.Server, l net.Listener) {
	if err := h.Serve(l); err != nil && err != http.ErrServerClosed && err != net.ErrClosed {
		log.Printf("HTTP listener stopped: %v", err)
		s.Close()
	}
}
func (s *Server) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		s.stopped = true
		for _, l := range []net.Listener{s.sshListener, s.adminListener, s.httpListener} {
			if l != nil {
				l.Close()
			}
		}
		for _, t := range s.tunnels {
			if t.listener != nil {
				t.listener.Close()
			}
		}
		for c := range s.sockets {
			c.Close()
		}
		for c := range s.clients {
			c.Close()
		}
		s.mu.Unlock()
		if s.adminHTTP != nil {
			s.adminHTTP.Close()
		}
		if s.publicHTTP != nil {
			s.publicHTTP.Close()
		}
	})
}
func Run(ctx context.Context, c config.Server) error {
	s, err := New(c)
	if err != nil {
		return err
	}
	if err = s.Start(); err != nil {
		return err
	}
	defer s.Close()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			s.mu.Lock()
			stopped := s.stopped
			s.mu.Unlock()
			if stopped {
				return fmt.Errorf("server listener stopped unexpectedly")
			}
		}
	}
}
func (s *Server) track(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		c.Close()
		return false
	}
	s.sockets[c] = true
	return true
}
func (s *Server) untrack(c net.Conn) { c.Close(); s.mu.Lock(); delete(s.sockets, c); s.mu.Unlock() }
func (s *Server) acceptClients() {
	for {
		c, err := s.sshListener.Accept()
		if err != nil {
			if err != net.ErrClosed && !strings.Contains(err.Error(), "use of closed network connection") {
				log.Printf("SSH listener stopped: %v", err)
				s.Close()
			}
			return
		}
		if s.track(c) {
			go s.handleClient(c)
		}
	}
}
func (s *Server) handleClient(raw net.Conn) {
	defer s.untrack(raw)
	raw.SetDeadline(time.Now().Add(10 * time.Second))
	cfg := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if len(meta.User()) > 64 || subtle.ConstantTimeCompare([]byte(s.token), password) != 1 {
			return nil, fmt.Errorf("authentication rejected")
		}
		return nil, nil
	}}
	cfg.AddHostKey(s.signer)
	conn, channels, requests, err := ssh.NewServerConn(raw, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	raw.SetDeadline(time.Time{})
	go func() {
		for ch := range channels {
			ch.Reject(ssh.Prohibited, "clients cannot open channels")
		}
	}()
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.clients[conn] = true
	s.eventLocked("Client connected: " + displayText(conn.User()))
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		for id, t := range s.tunnels {
			if t.conn == conn {
				if t.listener != nil {
					t.listener.Close()
				}
				delete(s.tunnels, id)
				s.eventLocked("Tunnel closed: " + id)
			}
		}
		s.eventLocked("Client disconnected: " + displayText(conn.User()))
		s.mu.Unlock()
	}()
	for req := range requests {
		if req.Type == "keepalive@wormhole" {
			req.Reply(true, nil)
			continue
		}
		if req.Type != protocol.Forward {
			req.Reply(false, []byte("unsupported protocol; update the Wormhole client"))
			continue
		}
		var r protocol.Request
		if len(req.Payload) > 4096 || json.Unmarshal(req.Payload, &r) != nil {
			req.Reply(false, []byte("invalid registration"))
			continue
		}
		t, err := s.register(conn, r)
		if err != nil {
			req.Reply(false, []byte(err.Error()))
			continue
		}
		req.Reply(true, config.JSON(protocol.Registration{Version: 1, ID: t.ID, URL: t.URL}))
	}
}
func (s *Server) register(conn *ssh.ServerConn, r protocol.Request) (*Tunnel, error) {
	if r.Name != "" && !config.ValidName(r.Name) {
		return nil, fmt.Errorf("tunnel name must be lowercase letters, numbers or hyphens (max 48)")
	}
	if _, _, err := net.SplitHostPort(r.Local); err != nil || len(r.Local) > 256 {
		return nil, fmt.Errorf("invalid local address")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return nil, fmt.Errorf("server is stopping")
	}
	for _, t := range s.tunnels {
		if t.conn == conn {
			return nil, fmt.Errorf("one tunnel per client connection")
		}
	}
	id := r.Name
	if id == "" {
		random, err := config.NewSecret()
		if err != nil {
			return nil, err
		}
		id = "t-" + random[:12]
	}
	if _, ok := s.tunnels[id]; ok {
		return nil, fmt.Errorf("tunnel name is already in use")
	}
	t := &Tunnel{ID: id, Owner: displayText(conn.User()), Local: displayText(r.Local), Started: time.Now(), conn: conn}
	if s.cfg.Mode == "ports" {
		for p := s.cfg.PortStart; p <= s.cfg.PortEnd; p++ {
			l, err := net.Listen("tcp", net.JoinHostPort(s.cfg.ListenIP, strconv.Itoa(p)))
			if err == nil {
				t.listener = l
				t.URL = "http://" + net.JoinHostPort(s.cfg.AdvertiseHost, strconv.Itoa(p))
				break
			}
		}
		if t.listener == nil {
			return nil, fmt.Errorf("no available tunnel ports in configured range")
		}
	} else {
		t.URL = s.cfg.Scheme + "://" + id + "." + s.cfg.Domain
	}
	s.tunnels[id] = t
	s.eventLocked("Tunnel published: " + id)
	if t.listener != nil {
		go s.acceptVisitors(t)
	}
	return t, nil
}

func displayText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return -1
		}
		return r
	}, s)
}
func (s *Server) changeConnections(t *Tunnel, delta int) {
	s.mu.Lock()
	t.Connections += delta
	s.mu.Unlock()
}
func (s *Server) acceptVisitors(t *Tunnel) {
	for {
		c, err := t.listener.Accept()
		if err != nil {
			return
		}
		if s.track(c) {
			go func() {
				defer s.untrack(c)
				s.changeConnections(t, 1)
				defer s.changeConnections(t, -1)
				ip, _, _ := net.SplitHostPort(c.RemoteAddr().String())
				ch, reqs, err := t.conn.OpenChannel("tunnel", []byte(ip))
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				tunnel.Pipe(c, ch)
			}()
		}
	}
}
func (s *Server) Snapshot() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Name: s.cfg.Name, Mode: s.cfg.Mode, SSHAddress: net.JoinHostPort(s.cfg.AdvertiseHost, strconv.Itoa(s.cfg.SSHPort)), Fingerprint: ssh.FingerprintSHA256(s.signer.PublicKey()), Started: s.started, Clients: len(s.clients), Tunnels: []Tunnel{}, Events: append([]Event{}, s.events...)}
	for _, t := range s.tunnels {
		copy := *t
		copy.conn = nil
		copy.listener = nil
		st.Tunnels = append(st.Tunnels, copy)
	}
	return st
}
func (s *Server) adminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		stopped := s.stopped
		s.mu.Unlock()
		if stopped {
			http.Error(w, "stopping", 503)
			return
		}
		io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.adminToken)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.Snapshot())
	})
	return mux
}

// channelConn adapts an SSH channel to an HTTP transport's net.Conn.
type channelConn struct {
	ssh.Channel
	done func()
	once sync.Once
}
type addr string

func (a addr) Network() string                          { return "ssh" }
func (a addr) String() string                           { return string(a) }
func (c *channelConn) LocalAddr() net.Addr              { return addr("relay") }
func (c *channelConn) RemoteAddr() net.Addr             { return addr("client") }
func (c *channelConn) SetDeadline(time.Time) error      { return nil }
func (c *channelConn) SetReadDeadline(time.Time) error  { return nil }
func (c *channelConn) SetWriteDeadline(time.Time) error { return nil }
func (c *channelConn) Close() error                     { err := c.Channel.Close(); c.once.Do(c.done); return err }
func (s *Server) proxyHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	suffix := "." + s.cfg.Domain
	if !strings.HasSuffix(host, suffix) {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSuffix(host, suffix)
	s.mu.Lock()
	t := s.tunnels[id]
	s.mu.Unlock()
	if t == nil {
		http.NotFound(w, r)
		return
	}
	transport := &http.Transport{DisableKeepAlives: true, ResponseHeaderTimeout: 30 * time.Second, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		type result struct {
			ch   ssh.Channel
			reqs <-chan *ssh.Request
			err  error
		}
		out := make(chan result)
		go func() {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			ch, reqs, err := t.conn.OpenChannel("tunnel", []byte(ip))
			select {
			case out <- result{ch, reqs, err}:
			case <-ctx.Done():
				if ch != nil {
					ch.Close()
				}
			}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-out:
			if result.err != nil {
				return nil, result.err
			}
			go ssh.DiscardRequests(result.reqs)
			s.changeConnections(t, 1)
			return &channelConn{Channel: result.ch, done: func() { s.changeConnections(t, -1) }}, nil
		}
	}}
	defer transport.CloseIdleConnections()
	target := &url.URL{Scheme: "http", Host: "tunnel"}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	original := proxy.Director
	proxy.Director = func(req *http.Request) {
		original(req)
		req.Header.Del("X-Forwarded-For")
		req.Header.Del("X-Forwarded-Host")
		req.Header.Del("X-Forwarded-Proto")
	}
	proxy.ServeHTTP(w, r)
}

func StatusAt(c config.Server) (Status, error) {
	var st Status
	token, err := config.Secret(c.AdminTokenFile)
	if err != nil {
		return st, err
	}
	req, err := http.NewRequest("GET", "http://"+c.AdminListen+"/api/status", nil)
	if err != nil {
		return st, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return st, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return st, fmt.Errorf("management returned HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&st)
	return st, err
}
