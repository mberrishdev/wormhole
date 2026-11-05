# Wormhole server setup proposal

Status: historical design proposal. The first implementation now exists; see `../README.md` for actual commands, supported behavior, and deployment requirements. This document preserves the original proposal and includes future ideas beyond the implemented scope. First target selected by the user: Docker Compose on a LAN server. See `deployment-research.md` for deployment research and primary sources.

## Product behavior

An administrator runs one server on a stable LAN address. Developers run clients on their own machines; viewers use browsers. Access is intended for the LAN or corporate VPN. Binding a socket does not enforce that boundary: host firewall rules, private network routing, and the absence of public forwarding must enforce it.

The server is a long-running process. The TUI is an optional management client; closing it must not stop a separately running server. Compose, Kubernetes, systemd, and Windows Service mode run without a terminal or setup prompts. Interactive initialization writes configuration before the service starts.

## Proposed command interface

These commands describe the intended interface; they do not exist yet.

```text
wormhole-server init                # setup wizard; generate config and credentials
wormhole-server config validate     # validate config without starting listeners
wormhole-server serve               # foreground service; structured logs
wormhole-server status              # status through management API
wormhole-server tui                 # attach terminal dashboard
wormhole-server doctor              # check config, bind addresses, state permissions
```

All commands accept an explicit config path. Configuration precedence: flags, environment, file, defaults. Secrets support mounted files so Compose/Kubernetes do not need to put credentials in arguments. Diagnostics redact secrets. `doctor` distinguishes local checks from reachability checks that require a second LAN machine.

Use a Go terminal framework such as [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI. Its first version lists clients, active tunnels, URLs, connection counts, and recent events. Include keyboard navigation, help, terminal resize behavior, and plain CLI output for automation. A local authenticated management API serves the TUI; it does not reuse the current unauthenticated client dashboard.

## First release: LAN port routing

Keep the encrypted SSH transport on port 2222. Remove the requirement for a public domain, public certificate, or privileged ports. Reserve a small explicit range, initially 8081–8090, for HTTP tunnels. Each active tunnel owns a listener; release it on disconnect and reject allocations when the range is exhausted. Return a versioned registration response containing tunnel identity and the complete advertised URL to the client.

Example configuration shape (proposed YAML, subject to implementation validation):

```yaml
version: 1
ssh:
  listen: "0.0.0.0:2222"
  host_key_file: "/var/lib/wormhole/ssh_host_key"
auth:
  token_file: "/run/secrets/wormhole_token"
routing:
  mode: ports
  listen_ip: "0.0.0.0"
  advertised_host: "192.168.1.100"
  port_start: 8081
  port_end: 8090
management:
  listen: "127.0.0.1:4041"
  token_file: "/run/secrets/wormhole_admin_token"
```

The advertised host is the address users can reach, not the container IP or bind address. The wizard must ask for it when detection is ambiguous. Client authentication and management authentication use different generated secrets. Pin the persistent SSH host key on clients, with an explicit first-use enrollment step; do not retain `InsecureIgnoreHostKey`. Client authorization to create a tunnel is separate from browser access: the initial HTTP mode allows anyone who can reach a tunnel port to open the app. HTTP contents are visible on that LAN segment; document optional TLS termination for sensitive use.

Compose publishes SSH and the exact configured tunnel range on a chosen host LAN IP. It mounts read-only config and secrets, plus persistent state for the SSH key. The management endpoint is not published. Administrators attach using `docker compose exec server wormhole-server tui`; provide a local native CLI path as well. A container loopback endpoint cannot be used directly by a host-side TUI without an explicit management connection arrangement.

Ship a multi-stage non-root image, Compose example, restart policy, resource guidance, health command, graceful shutdown, and logs to stdout/stderr. No Docker socket mount or privileged container is needed. Changing the published port range requires updating Compose as well as application config; the wizard should generate matching files.

Client changes are required too: remove the dead AWS default, accept server configuration, parse registration metadata, print returned LAN URLs, update dashboard URLs, reconnect with bounded backoff, and bind the client dashboard to loopback. Old clients receive a clear protocol compatibility error rather than silently treating metadata as a hostname.

## Later company deployment modes

For HTTP applications at larger scale, support internal hostname routing, e.g. `demo.tunnels.corp.example`, with internal wildcard DNS and one HTTP listener. Avoid path-prefix routing as the default: apps often assume they live at `/`. A corporate reverse proxy can terminate trusted HTTPS.

Kubernetes: start with one replica and a recreate rollout, mounted ConfigMap/Secret, persistent SSH host key, probes, resource requests, and explicit network access rules. Expose SSH through a TCP-capable Service and HTTP through the chosen private Service/Ingress setup. A private cloud load balancer needs provider-specific configuration. ClusterIP alone is not reachable from ordinary developer laptops. Do not promise HA: sessions live in the serving process, and more replicas need session-aware routing or a redesigned data plane. Reconnect after restarts is expected.

Windows Server: ship a native Windows executable with proper Service Control Manager integration and persistent state/config paths. A console program registered with `sc.exe` alone is insufficient. Keep TUI operation separate from service execution. A Linux VM running Compose is an alternative; do not base Windows Server instructions on Docker Desktop.

## Repository changes and implementation order

1. Extract the server from `cmd/server/main.go` into lifecycle/config/routing packages. Add validation, generated secrets, persistent host identity, clean shutdown, and removal of domain/certificate constants. Fix connection closure/cancellation while refactoring.
2. Add port routing and versioned client registration. Update client configuration, host verification, advertised URLs, and dashboard binding. Exercise concurrent traffic, disconnect cleanup, exhaustion, authentication failure, and reconnect.
3. Add Dockerfile, generated Compose/config examples, health command, and LAN setup guide. Build/test the image and perform a real two-machine LAN smoke test before claiming LAN reachability.
4. Add authenticated management API, CLI status/doctor, setup wizard, and attachable TUI. Validate keyboard interaction in a real terminal.
5. Add hostname mode, Kubernetes manifests, and native Windows service packaging after the Compose path works.

The release workflow currently builds only clients, selects Go 1.22 while `go.mod` requires Go 1.25.0, and the installer only handles macOS. Update toolchain selection, server artifacts, checksums, container publishing, and platform-specific install instructions as part of packaging. Documentation must replace the obsolete AWS/8888 deployment guide.

## Acceptance criteria for the Compose milestone

- A fresh LAN host can generate config and start the server without public DNS or certificate issuance.
- Two developers can expose different local apps concurrently and another LAN machine can open both advertised URLs.
- Closing the TUI leaves the server running. Restarting the container preserves SSH identity; clients reconnect and receive current endpoints.
- Config/secrets are not baked into the image; logs and status redact credentials.
- Only the chosen SSH port and bounded tunnel range are published. Management remains private.
- Invalid config, wrong credentials, local app failure, range exhaustion, and server unavailability produce useful errors.
- Existing public-domain assumptions and the dead AWS address are absent from the LAN setup path.
