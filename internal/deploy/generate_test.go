package deploy

import (
	"encoding/json"
	"github.com/mberrishdev/wormhole/internal/config"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedInstallations(t *testing.T) {
	for _, deployment := range []string{"compose", "native", "kubernetes"} {
		t.Run(deployment, func(t *testing.T) {
			c := config.Defaults()
			c.Deployment = deployment
			dir := filepath.Join(t.TempDir(), "install")
			if deployment == "kubernetes" {
				c.Mode = "hostname"
				c.Domain = "tunnels.company.internal"
			}
			if err := Generate(dir, c); err != nil {
				t.Fatal(err)
			}
			loaded, err := config.Load(filepath.Join(dir, "server.json"))
			if err != nil {
				t.Fatal(err)
			}
			signer, err := config.Signer(loaded.HostKeyFile)
			if err != nil {
				t.Fatal(err)
			}
			var client config.Client
			if err = config.Read(filepath.Join(dir, "client.json"), &client); err != nil {
				t.Fatal(err)
			}
			if client.Fingerprint != ssh.FingerprintSHA256(signer.PublicKey()) {
				t.Fatal("client fingerprint does not match persistent identity")
			}
			clientToken, _ := config.Secret(loaded.TokenFile)
			adminToken, _ := config.Secret(loaded.AdminTokenFile)
			if client.Token != clientToken || clientToken == adminToken {
				t.Fatal("incorrect credential separation")
			}
			before, _ := os.ReadFile(filepath.Join(dir, "client.json"))
			if err = Generate(dir, c); err == nil {
				t.Fatal("existing installation overwritten")
			}
			after, _ := os.ReadFile(filepath.Join(dir, "client.json"))
			if string(before) != string(after) {
				t.Fatal("existing credentials changed")
			}
			if deployment == "compose" {
				var compose map[string]any
				data, _ := os.ReadFile(filepath.Join(dir, "compose.yaml"))
				if err = json.Unmarshal(data, &compose); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(data), "4041:") {
					t.Fatal("management port published")
				}
			}
			if deployment == "kubernetes" {
				var list struct {
					Items []map[string]any `json:"items"`
				}
				data, _ := os.ReadFile(filepath.Join(dir, "kubernetes.json"))
				if err = json.Unmarshal(data, &list); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, item := range list.Items {
					if item["kind"] == "Service" {
						found = true
						if item["spec"].(map[string]any)["type"] != "ClusterIP" {
							t.Fatal("public service generated")
						}
					}
				}
				if !found {
					t.Fatal("missing Service")
				}
			}
		})
	}
}
