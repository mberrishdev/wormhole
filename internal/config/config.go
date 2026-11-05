package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Server struct {
	Version        int    `json:"version"`
	Name           string `json:"name"`
	Deployment     string `json:"deployment"`
	ListenIP       string `json:"listen_ip"`
	SSHPort        int    `json:"ssh_port"`
	AdvertiseHost  string `json:"advertise_host"`
	Mode           string `json:"mode"`
	PortStart      int    `json:"port_start"`
	PortEnd        int    `json:"port_end"`
	HTTPPort       int    `json:"http_port"`
	Domain         string `json:"domain,omitempty"`
	Scheme         string `json:"scheme"`
	HostKeyFile    string `json:"host_key_file"`
	TokenFile      string `json:"token_file"`
	AdminTokenFile string `json:"admin_token_file"`
	AdminListen    string `json:"admin_listen"`
}
type Client struct {
	Version     int    `json:"version"`
	Server      string `json:"server"`
	Token       string `json:"token"`
	Fingerprint string `json:"host_fingerprint"`
	Local       string `json:"local"`
	Name        string `json:"name,omitempty"`
}

func Defaults() Server {
	return Server{Version: 1, Name: "lan-relay", Deployment: "compose", ListenIP: "0.0.0.0", SSHPort: 2222, AdvertiseHost: "127.0.0.1", Mode: "ports", PortStart: 8081, PortEnd: 8090, HTTPPort: 8080, Scheme: "http", HostKeyFile: "secrets/ssh_host_key", TokenFile: "secrets/client-token", AdminTokenFile: "secrets/admin-token", AdminListen: "127.0.0.1:4041"}
}
func ValidName(s string) bool {
	if len(s) < 1 || len(s) > 48 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func ValidHost(s string) bool {
	if ip := net.ParseIP(s); ip != nil {
		return !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLinkLocalUnicast()
	}
	if len(s) > 253 {
		return false
	}
	for _, p := range strings.Split(s, ".") {
		if !ValidName(p) {
			return false
		}
	}
	return true
}
func (c Server) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if !ValidName(c.Name) {
		return fmt.Errorf("name must be 1-48 lowercase letters, numbers or hyphens")
	}
	if c.Deployment != "compose" && c.Deployment != "native" && c.Deployment != "kubernetes" {
		return fmt.Errorf("deployment must be compose, native, or kubernetes")
	}
	if net.ParseIP(c.ListenIP) == nil {
		return fmt.Errorf("listen_ip must be an IP address")
	}
	if !ValidHost(c.AdvertiseHost) {
		return fmt.Errorf("advertise_host must be a reachable IP or DNS hostname, not a URL or wildcard")
	}
	if c.Mode != "ports" && c.Mode != "hostname" {
		return fmt.Errorf("mode must be ports or hostname")
	}
	valid := func(p int) bool { return p >= 1024 && p <= 65535 }
	if !valid(c.SSHPort) || !valid(c.HTTPPort) || !valid(c.PortStart) || !valid(c.PortEnd) || c.PortEnd < c.PortStart || c.PortEnd-c.PortStart > 99 {
		return fmt.Errorf("ports must be 1024-65535; tunnel range must contain 1-100 ports")
	}
	if c.Mode == "ports" && c.SSHPort >= c.PortStart && c.SSHPort <= c.PortEnd {
		return fmt.Errorf("SSH port overlaps tunnel ports")
	}
	if c.Mode == "hostname" && (!ValidHost(c.Domain) || net.ParseIP(c.Domain) != nil || !strings.Contains(c.Domain, ".")) {
		return fmt.Errorf("hostname mode needs a DNS suffix, e.g. tunnels.company.internal")
	}
	if c.Mode == "hostname" && c.HTTPPort == c.SSHPort {
		return fmt.Errorf("HTTP and SSH ports conflict")
	}
	if c.Scheme != "http" && c.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https (HTTPS requires an external TLS proxy)")
	}
	if c.Mode == "ports" && c.Scheme != "http" {
		return fmt.Errorf("port mode currently supports http URLs only")
	}
	h, p, err := net.SplitHostPort(c.AdminListen)
	if err != nil || net.ParseIP(h) == nil || !net.ParseIP(h).IsLoopback() {
		return fmt.Errorf("admin_listen must use a loopback IP and port")
	}
	ap, err := strconv.Atoi(p)
	if err != nil || !valid(ap) || ap == c.SSHPort || (c.Mode == "ports" && ap >= c.PortStart && ap <= c.PortEnd) || (c.Mode == "hostname" && ap == c.HTTPPort) {
		return fmt.Errorf("management port is invalid or conflicts with a listener")
	}
	if c.HostKeyFile == "" || c.TokenFile == "" || c.AdminTokenFile == "" {
		return fmt.Errorf("host key and credential file paths are required")
	}
	return nil
}
func Read(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%s: unexpected trailing data", path)
	}
	return nil
}
func Load(path string) (Server, error) {
	var c Server
	if err := Read(path, &c); err != nil {
		return c, err
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, err
	}
	for _, p := range []*string{&c.HostKeyFile, &c.TokenFile, &c.AdminTokenFile} {
		if !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}
	return c, nil
}
func Secret(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if len(s) < 32 {
		return "", fmt.Errorf("credential in %s must contain at least 32 characters", path)
	}
	return s, nil
}
func NewSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func NewKey() ([]byte, ssh.Signer, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		return nil, nil, err
	}
	data := pem.EncodeToMemory(block)
	s, err := ssh.ParsePrivateKey(data)
	return data, s, err
}
func Signer(path string) (ssh.Signer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(b)
}
func JSON(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}
