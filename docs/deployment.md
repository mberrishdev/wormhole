# Deployment and configuration

For the short installation steps, see the [README](../README.md).

Self-hosted reverse tunnels for a LAN or corporate VPN. One relay runs in your infrastructure; developers connect local applications to it, and teammates open the returned URLs in a browser.

The server includes a Lip Gloss / Bubble Tea setup wizard and a live, read-only monitoring TUI. Docker Compose is the primary deployment path. Native Linux/Windows service files and Kubernetes manifests are also supported.

## Build

Requires Go 1.25 or newer:

```bash
go build -o bin/wormhole-server ./cmd/server
go build -o bin/wormhole-client ./cmd/client
```

On Windows, use `.exe` output names. Server and client must both use the new version; the earlier public-domain protocol is incompatible.

## Set up the server

Run the wizard **on the machine that will host the relay**, preferably from the repository root when using Compose:

```bash
./bin/wormhole-server init
```

1. Choose Docker Compose, native, or Kubernetes.
2. Choose LAN IP with ports, or internal hostname routing.
3. Enter the existing server LAN address and ports. Entering an IP does not configure the network interface.
4. Press `r` to review, then `Enter` on **Generate setup files**.
5. Read `wormhole-setup/NEXT-STEPS.txt` and start the chosen deployment. Generation does not silently start Docker, install services, or change a cluster.

The wizard generates a new private directory and refuses to overwrite an existing installation. It contains JSON config files, separate client/management credentials, a persistent SSH host key, a developer `client.json` profile, and deployment files. Treat this directory as a secret; do not commit it. Keep backups of the host key and credentials. Generate a separate directory to try another setup.

### Docker Compose on a LAN server

Choose the server's stable LAN IP, e.g. `192.168.1.100`, and port mode for the simplest install.

```bash
cd wormhole-setup
docker compose up -d --build
docker compose logs -f
```

The generated Compose file has a build context when setup is run from this repository. If the binary was installed without the source, first build `wormhole-server:local` from the repository (or replace the image with your registry image):

```bash
docker build -t wormhole-server:local /path/to/wormhole
```

Compose publishes the SSH port and configured tunnel ports on the selected LAN IP. The container runs with the generating user's UID/GID so it can read the private bind-mounted credentials; set the `user` setting appropriately if moving the installation to another host. The mounted installation directory is read-only and holds the persistent identity. The management port is not published.

Open the live TUI:

```bash
docker compose exec server /wormhole-server tui --config /etc/wormhole/container.json
```

### Native server

Choose native in the wizard, or generate non-interactively:

```bash
./bin/wormhole-server init --non-interactive --deployment native --host 192.168.1.100
./bin/wormhole-server serve --config wormhole-setup/server.json
```

In another terminal:

```bash
./bin/wormhole-server tui --config wormhole-setup/server.json
```

Linux: the generated `wormhole.service` refers to the executable used during setup. Build to a permanent path before generating it, set `User`/`Group` to the account that owns the config directory, and install it with your normal systemd procedure. Native foreground mode also works on macOS; the systemd unit is Linux-only.

Windows Server, from an Administrator terminal after placing the executable/config at permanent paths:

```powershell
.\wormhole-server.exe service install --config C:\Wormhole\wormhole-setup\server.json
.\wormhole-server.exe service start
.\wormhole-server.exe tui --config C:\Wormhole\wormhole-setup\server.json
```

`service stop` and `service uninstall` manage the installed service. Windows Service Control Manager integration is implemented in the native executable. Configure the account and Windows Firewall for your organization. Running a Linux container on Windows Server requires a Linux VM; this setup does not require Docker Desktop.

### Kubernetes

Choose Kubernetes in the wizard. Hostname mode is recommended, but port mode is available. Setup generates `kubernetes.json`, a Kubernetes List containing a Namespace, ConfigMap, Secret, one-replica Deployment with Recreate rollout, Service, and an Ingress in hostname mode.

1. Build/push the image to your registry and replace `wormhole-server:local` in the manifest.
2. Review the namespace, resources, image, secret handling, and networking for your cluster.
3. Apply the manifest:

```bash
kubectl apply -f wormhole-setup/kubernetes.json
```

The generated Service is **ClusterIP**, so it is not automatically reachable from developers' laptops. Configure corporate routing or your provider's **private** TCP load balancer for SSH. In port mode, expose every declared tunnel port too. Do not assume `type: LoadBalancer` is private.

Hostname mode additionally requires an internal Ingress controller, internal wildcard DNS, and optionally company TLS. Set `ingressClassName` and TLS settings in the generated Ingress to match the cluster. Ordinary HTTP Ingress does not carry the SSH control connection. No Helm chart or Kubernetes operator is required by this implementation.

```bash
kubectl -n wormhole exec -it deploy/lan-relay -- /wormhole-server tui --config /etc/wormhole/config.json
```

The SSH key is stored in a Secret, preserving identity across pod restarts. Reapplying the same generated files retains it. Regenerating credentials creates a new identity. Single-replica only: active SSH sessions belong to one process, and restarts disconnect tunnels. Do not scale this Deployment horizontally.

## Connect a developer's app

Send **only `client.json`** to authorized developers through a trusted channel. It contains the client credential and server fingerprint, not the management credential or private key. No Docker is needed on developer laptops.

Start an app locally, then:

```bash
./bin/wormhole-client --config client.json --local localhost:3000 --name frontend
```

Port mode prints a URL such as `http://192.168.1.100:8081`. Hostname mode prints `http://frontend.tunnels.company.internal`. `--name` is optional; automatic names are unique. Explicit names must be available and use lowercase letters, numbers, or hyphens.

The client verifies the configured SSH fingerprint, forwards traffic, and reconnects with backoff after a network/server disconnect. Reconnect can change the allocated port, so use the latest printed URL. Invalid credentials, host key changes, name conflicts, and registration errors require correction and exit with a clear error. `--once` disables reconnecting. `--server host:port` overrides the endpoint while retaining fingerprint verification.

Teammates open the returned URL from the LAN/VPN. They need no Wormhole software. When the developer exits, its listener and tunnel registration are released.

## LAN ports versus internal domains

- **Ports:** no DNS or certificates. Bounded range of listeners, one per tunnel; HTTP URLs use the relay's LAN IP. Port mode forwards byte streams, but the UI advertises HTTP and does not implement UDP.
- **Hostnames:** a fixed HTTP backend port routes `name.<domain>` through SSH to the right developer. Configure internal wildcard DNS and a reverse proxy/Ingress forwarding the Host header to `http_port`. Generated hostname URLs assume port 80, or 443 when `scheme` is `https`; they omit the backend port. For direct testing without a proxy, include `:<http_port>` and send the correct Host header. HTTPS is terminated externally; Wormhole's HTTP listener does not load certificates itself.

To generate hostname configuration without the wizard:

```bash
./bin/wormhole-server init --non-interactive --deployment kubernetes --host relay.company.internal --mode hostname --domain tunnels.company.internal --scheme https
```

DNS, TLS certificates, firewall rules, and private load balancers are managed by your infrastructure. Passing a domain to setup does not create those resources.

## Monitoring and configuration

```bash
./bin/wormhole-server config validate --config wormhole-setup/server.json
./bin/wormhole-server doctor --config wormhole-setup/server.json
./bin/wormhole-server status --config wormhole-setup/server.json
./bin/wormhole-server health --config wormhole-setup/server.json
```

`status` returns JSON. The monitoring TUI polls every two seconds: `1` Overview, `2` Tunnels, `3` Events, `4` Settings; arrows select tunnels; `q` detaches without stopping the server. Settings are read-only. Edit the config or deployment source, validate, then restart. Changes to the port range also require matching Compose/Service mappings. The server binds management to loopback and protects status with a separate credential.

Configuration is strict JSON; unknown fields are rejected. Credential/key paths in local config resolve relative to its file. Container config uses mounted absolute paths. `server.json`, `container.json`, Compose/Kubernetes settings are generated together, but do not auto-synchronize later manual edits: edit the artifact used by your deployment and its port mappings together.

The older sample UI is still available with `go run ./cmd/server-tui --demo`. The actual wizard and monitoring commands above are no longer demos. The old client browser dashboard is not started by the new client; monitoring lives on the relay. The root `main.go` is a historical standalone port-forwarding experiment, not the server entrypoint.

## Network access and authentication

Allow only intended office/VPN subnets through the host/network firewall. A private bind address alone is not authorization or a guarantee of isolation. Do not configure internet/router port forwarding for a LAN-only deployment.

SSH protects the developer-to-relay connection. Plain HTTP from a browser to the relay is unencrypted unless you configure TLS termination. A client credential authorizes tunnel creation, **not visitor access**: anyone able to reach a tunnel may access the app. Use application authentication and company network controls as needed. Initial publisher authentication uses one shared credential; per-developer keys, credential revocation UI, SSO, and audit storage are future work.

## Verify

```bash
go test -race ./...
```

Integration tests exercise SSH authentication and host verification, HTTP forwarding, hostname routing, capacity limits, disconnect cleanup, and setup generation. To prove LAN availability, additionally connect from a second machine in your own network. Docker, Kubernetes, and Windows runtime validation depend on those environments being available.
