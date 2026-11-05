package deploy

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mberrishdev/wormhole/internal/config"
	"golang.org/x/crypto/ssh"
)

// Generate writes a new private directory. It never overwrites an installation.
func Generate(dir string, c config.Server) error {
	if err := c.Validate(); err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(abs); !os.IsNotExist(err) {
		return fmt.Errorf("output directory already exists or cannot be inspected: %s", abs)
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(abs), ".wormhole-setup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = os.Mkdir(filepath.Join(stage, "secrets"), 0700); err != nil {
		return err
	}
	key, signer, err := config.NewKey()
	if err != nil {
		return err
	}
	token, err := config.NewSecret()
	if err != nil {
		return err
	}
	admin, err := config.NewSecret()
	if err != nil {
		return err
	}
	// Generated paths are always self-contained, independent of caller input.
	c.HostKeyFile = "secrets/ssh_host_key"
	c.TokenFile = "secrets/client-token"
	c.AdminTokenFile = "secrets/admin-token"
	client := config.Client{Version: 1, Server: net.JoinHostPort(c.AdvertiseHost, strconv.Itoa(c.SSHPort)), Token: token, Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()), Local: "localhost:3000"}
	files := map[string][]byte{"server.json": config.JSON(c), "client.json": config.JSON(client), "secrets/ssh_host_key": key, "secrets/client-token": []byte(token + "\n"), "secrets/admin-token": []byte(admin + "\n")}
	container := c
	container.HostKeyFile = "/etc/wormhole/secrets/ssh_host_key"
	container.TokenFile = "/etc/wormhole/secrets/client-token"
	container.AdminTokenFile = "/etc/wormhole/secrets/admin-token"
	files["container.json"] = config.JSON(container)
	files[".gitignore"] = []byte("*\n!.gitignore\n")
	switch c.Deployment {
	case "compose":
		uid, gid := os.Getuid(), os.Getgid()
		if uid < 0 {
			uid = 1000
		}
		if gid < 0 {
			gid = 1000
		}
		host := net.ParseIP(c.AdvertiseHost)
		if host == nil {
			return fmt.Errorf("Compose setup needs the host's LAN IP for explicit published-port binding; use a DNS name only for the tunnel domain")
		}
		mapping := func(port string) string { return net.JoinHostPort(c.AdvertiseHost, port) + ":" + port }
		ports := []string{mapping(strconv.Itoa(c.SSHPort))}
		if c.Mode == "ports" {
			ports = append(ports, mapping(fmt.Sprintf("%d-%d", c.PortStart, c.PortEnd)))
		} else {
			ports = append(ports, mapping(strconv.Itoa(c.HTTPPort)))
		}
		service := map[string]any{"image": "wormhole-server:local", "user": fmt.Sprintf("%d:%d", uid, gid), "command": []string{"serve", "--config", "/etc/wormhole/container.json"}, "restart": "unless-stopped", "ports": ports, "volumes": []string{".:/etc/wormhole:ro"}, "read_only": true, "cap_drop": []string{"ALL"}, "security_opt": []string{"no-new-privileges:true"}, "healthcheck": map[string]any{"test": []string{"CMD", "/wormhole-server", "health", "--config", "/etc/wormhole/container.json"}, "interval": "15s", "timeout": "5s", "retries": 3}, "logging": map[string]any{"driver": "json-file", "options": map[string]string{"max-size": "10m", "max-file": "3"}}}
		cwd, _ := os.Getwd()
		if _, err := os.Stat(filepath.Join(cwd, "Dockerfile")); err == nil {
			service["build"] = map[string]any{"context": cwd}
		}
		files["compose.yaml"] = config.JSON(map[string]any{"services": map[string]any{"server": service}})
	case "kubernetes":
		files["kubernetes.json"] = kubernetes(c, container, key, token, admin)
	case "native":
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		if strings.ContainsAny(executable+abs, "\n\r\"%") {
			return fmt.Errorf("native service paths cannot contain quotes, percent signs or newlines")
		}
		unit := fmt.Sprintf("[Unit]\nDescription=Wormhole LAN tunnel server\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nWorkingDirectory=%s\nExecStart=\"%s\" serve --config \"%s\"\nRestart=on-failure\nRestartSec=3\nNoNewPrivileges=true\nPrivateTmp=true\nProtectSystem=strict\nProtectHome=read-only\n\n[Install]\nWantedBy=multi-user.target\n", abs, executable, filepath.Join(abs, "server.json"))
		files["wormhole.service"] = []byte(unit)
	}
	files["NEXT-STEPS.txt"] = []byte(Instructions(abs, c))
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(stage, name), data, 0600); err != nil {
			return err
		}
	}
	// Recheck before rename; a nonempty destination is never replaced.
	if _, err = os.Lstat(abs); !os.IsNotExist(err) {
		return fmt.Errorf("output directory appeared during generation")
	}
	return os.Rename(stage, abs)
}

func Instructions(dir string, c config.Server) string {
	base := fmt.Sprintf("Setup generated in %s\n\nDo not commit this directory: it contains credentials and a private SSH key.\nShare only client.json with authorized developers over a trusted channel.\nKeep secrets/ssh_host_key to preserve server identity.\n\n", dir)
	switch c.Deployment {
	case "compose":
		base += "From this directory:\n  docker compose up -d --build\n  docker compose exec server /wormhole-server tui --config /etc/wormhole/container.json\nIf no build context was found, first build the image from the repository:\n  docker build -t wormhole-server:local <repository-path>\nUse `docker compose logs -f` for startup errors.\n"
	case "native":
		base += fmt.Sprintf("Start without installation:\n  wormhole-server serve --config %q\n  wormhole-server tui --config %q\nLinux: copy wormhole.service to /etc/systemd/system/, set an appropriate User/Group, then daemon-reload and enable --now wormhole.\nWindows (Administrator): wormhole-server.exe service install --config %q\nThen: wormhole-server.exe service start\nUse a built binary at a permanent path, not `go run`, for service installation.\n", filepath.Join(dir, "server.json"), filepath.Join(dir, "server.json"), filepath.Join(dir, "server.json"))
	case "kubernetes":
		base += "Build and push wormhole-server to your private registry, then replace wormhole-server:local in kubernetes.json.\n  kubectl apply -f kubernetes.json\nThe generated Service is ClusterIP: configure private TCP exposure/routing for developers before connecting.\nDo not use a public LoadBalancer by accident. Private LB settings depend on your provider.\n  kubectl -n wormhole exec -it deploy/" + c.Name + " -- /wormhole-server tui --config /etc/wormhole/config.json\nSingle replica with Recreate rollout: restarts disconnect tunnels; clients reconnect.\n"
	}
	base += fmt.Sprintf("\nAllow only your LAN/VPN subnets to reach SSH port %d and ", c.SSHPort)
	if c.Mode == "ports" {
		base += fmt.Sprintf("tunnel ports %d-%d.\n", c.PortStart, c.PortEnd)
	} else {
		base += fmt.Sprintf("HTTP port %d (or your internal reverse proxy).\nConfigure internal wildcard DNS for *.%s to your HTTP entrypoint.\n", c.HTTPPort, c.Domain)
		if c.Deployment != "kubernetes" {
			base += "A reverse proxy must forward the wildcard hostnames to the configured HTTP port; returned URLs omit that backend port.\n"
		}
		if c.Scheme == "https" {
			base += "Configure trusted TLS termination in your reverse proxy/Ingress before using the advertised HTTPS URLs.\n"
		}
	}
	base += "No router/public port forwarding is needed. Management listens on loopback only.\nBrowser visitors are not authenticated by Wormhole: anyone who can reach a tunnel can use it. HTTP is unencrypted on the LAN; use application auth and internal TLS when required.\n\nDeveloper:\n  wormhole-client --config client.json --local localhost:3000 --name frontend\n\nVerify from a SECOND LAN machine: start a local app and client, then open the returned URL.\nA local health check cannot prove LAN routing or firewall isolation.\n"
	return base
}

func kubernetes(c, container config.Server, key []byte, token, admin string) []byte {
	meta := func(name string) map[string]any { return map[string]any{"name": name, "namespace": "wormhole"} }
	container.HostKeyFile = "/etc/wormhole/secrets/ssh_host_key"
	container.TokenFile = "/etc/wormhole/secrets/client-token"
	container.AdminTokenFile = "/etc/wormhole/secrets/admin-token"
	labels := map[string]string{"app": c.Name}
	ports := []any{map[string]any{"name": "ssh", "port": c.SSHPort, "targetPort": c.SSHPort}}
	if c.Mode == "ports" {
		for p := c.PortStart; p <= c.PortEnd; p++ {
			ports = append(ports, map[string]any{"name": fmt.Sprintf("t-%d", p), "port": p, "targetPort": p})
		}
	} else {
		ports = append(ports, map[string]any{"name": "http", "port": c.HTTPPort, "targetPort": c.HTTPPort})
	}
	items := []any{
		map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]string{"name": "wormhole"}},
		map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": meta(c.Name), "data": map[string]string{"config.json": string(config.JSON(container))}},
		map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": meta(c.Name), "type": "Opaque", "data": map[string]string{"ssh_host_key": base64.StdEncoding.EncodeToString(key), "client-token": base64.StdEncoding.EncodeToString([]byte(token)), "admin-token": base64.StdEncoding.EncodeToString([]byte(admin))}},
		map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": meta(c.Name), "spec": map[string]any{"replicas": 1, "strategy": map[string]string{"type": "Recreate"}, "selector": map[string]any{"matchLabels": labels}, "template": map[string]any{"metadata": map[string]any{"labels": labels}, "spec": map[string]any{"automountServiceAccountToken": false, "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 65532}, "containers": []any{map[string]any{"name": "server", "image": "wormhole-server:local", "args": []string{"serve", "--config", "/etc/wormhole/config.json"}, "securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []string{"ALL"}}}, "resources": map[string]any{"requests": map[string]string{"cpu": "100m", "memory": "64Mi"}, "limits": map[string]string{"cpu": "1", "memory": "256Mi"}}, "readinessProbe": map[string]any{"exec": map[string]any{"command": []string{"/wormhole-server", "health", "--config", "/etc/wormhole/config.json"}}, "periodSeconds": 10}, "livenessProbe": map[string]any{"exec": map[string]any{"command": []string{"/wormhole-server", "health", "--config", "/etc/wormhole/config.json"}}, "initialDelaySeconds": 10, "periodSeconds": 20}, "volumeMounts": []any{map[string]any{"name": "config", "mountPath": "/etc/wormhole", "readOnly": true}, map[string]any{"name": "secrets", "mountPath": "/etc/wormhole/secrets", "readOnly": true}}}}, "volumes": []any{map[string]any{"name": "config", "configMap": map[string]string{"name": c.Name}}, map[string]any{"name": "secrets", "secret": map[string]any{"secretName": c.Name, "defaultMode": 292}}}}}}},
		map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": meta(c.Name), "spec": map[string]any{"type": "ClusterIP", "selector": labels, "ports": ports}},
	}
	if c.Mode == "hostname" {
		items = append(items, map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": meta(c.Name), "spec": map[string]any{"rules": []any{map[string]any{"host": "*." + c.Domain, "http": map[string]any{"paths": []any{map[string]any{"path": "/", "pathType": "Prefix", "backend": map[string]any{"service": map[string]any{"name": c.Name, "port": map[string]int{"number": c.HTTPPort}}}}}}}}}})
	}
	return config.JSON(map[string]any{"apiVersion": "v1", "kind": "List", "items": items})
}
