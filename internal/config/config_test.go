package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnsafeOrConflictingListenersRejected(t *testing.T) {
	for name, change := range map[string]func(*Server){
		"public-management": func(c *Server) { c.AdminListen = "0.0.0.0:4041" },
		"overlapping-ssh":   func(c *Server) { c.SSHPort = c.PortStart },
		"oversized-range":   func(c *Server) { c.PortEnd = c.PortStart + 100 },
		"invalid-host":      func(c *Server) { c.AdvertiseHost = "http://192.168.1.1" },
		"invalid-domain":    func(c *Server) { c.Mode = "hostname"; c.Domain = "*.company.internal" },
	} {
		t.Run(name, func(t *testing.T) {
			c := Defaults()
			change(&c)
			if c.Validate() == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
func TestStrictConfigurationRejectsTyposAndTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"version":1,"ssh_prt":2222}`, `{"version":1} {"version":2}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		var c Server
		if Read(path, &c) == nil {
			t.Fatal("malformed configuration accepted")
		}
	}
}
