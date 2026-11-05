# Wormhole internal deployment research

Research date: 2026-11-05. Scope: an internally reachable reverse-tunnel server, convenient server CLI/TUI, Docker, Kubernetes, and Windows Server. Recommendations below are proposals, not implemented features or verified deployment results.

## Recommendation

Ship one headless Go server with a configuration file and consistent CLI first. Add an optional setup TUI and a TUI that attaches to the running server's management API. Make Docker Compose on a small Linux VM the first supported company installation; offer native binaries for Linux and Windows. Add Kubernetes manifests or a Helm chart once the same server can run unattended.

Support two publishing modes:

- **Port mode:** `http://192.168.1.100:18001`, with an explicitly configured bounded range such as 18000–18020. This is the easiest LAN setup because it needs no DNS or certificates. It can also forward raw TCP if the implementation streams bytes without assuming HTTP.
- **Hostname mode:** `https://demo.tunnels.company.example`, using internal wildcard DNS and a fixed HTTP listener behind the company's private reverse proxy/TLS ingress. Prefer this mode for a larger company or Kubernetes, where fixed ports are easier to expose.

These modes serve different installation constraints; selecting port mode alone would unnecessarily limit the corporate deployment path. frp already distinguishes TCP listeners per service from HTTP hostname routing over one listener. [frp HTTP/HTTPS](https://gofrp.org/en/docs/features/http-https/)

## What established tunnel tools package

frp separates the client connection listener, proxy bind address, HTTP/HTTPS listeners, authentication, permitted port ranges, and monitoring configuration. That is a useful precedent for separating Wormhole's control port, published service ports, and administration endpoint. Its documentation provides binary installation and systemd guidance. [frp server configuration](https://gofrp.org/en/docs/reference/server-configures/), [frp setup](https://gofrp.org/en/docs/setup/)

Rathole distributes binaries and a Docker image, reads TOML configuration, requires service tokens, and supplies Linux systemd examples. Server-side and client-side service names/configuration agree; a server bind port is distinct from each exposed service's port. Its documentation also distinguishes encrypted TLS/Noise transports and supports configuration hot reload. This suggests that declarative configuration and unattended startup matter more than making the TUI the running server. [Rathole official repository](https://github.com/rathole-org/rathole)

These are documented product patterns, not evidence that every company installs tunnels identically.

## Deployment choices

| Environment | Proposed Wormhole installation | Reachability | Main caveat |
|---|---|---|---|
| Small team LAN | Native binary or Docker Compose on an always-on Linux VM | Stable private IP; SSH control port and a bounded service port range | Firewall/routing must permit intended office/VPN subnets |
| Existing company Kubernetes | One server replica; ConfigMap, Secret, persistent host identity; private Service/LB | VPN/routed corporate network to private TCP control listener; private HTTP ingress or fixed service ports | Pod-local tunnels prevent naive horizontal scaling |
| Windows Server estate | Signed/versioned native `.exe` with Windows Service integration | Private NIC plus Windows Firewall rules | Service lifecycle integration must be implemented; an arbitrary console exe is insufficient |
| Windows host with Linux VM platform | Same Linux Compose deployment inside managed Linux VM | VM private IP and controlled host/network routing | Requires VM operations support |

These are recommendations inferred from the platform capabilities described below, not deployed artifacts.

## Docker and Compose

Docker maps published ports to host addresses; without an explicit host IP, mappings normally listen on all host interfaces. Binding `127.0.0.1` allows only host-local access and does not provide team LAN access. For LAN use, publish on the intended private host IP and apply the organization's network/firewall policy. Binding a private IP by itself is not proof that an endpoint is inaccessible from other routed networks. [Docker port publishing](https://docs.docker.com/engine/network/port-publishing/)

Publish the fixed SSH control port plus a bounded port range in port mode. Docker supports publishing a range; allocating an arbitrary new listener in a container does not automatically create a matching host mapping. Host and container service port numbers should initially match to keep advertised URLs predictable. [Docker run: exposed ports](https://docs.docker.com/engine/containers/run/#exposed-ports)

Proposed packaging: multi-stage Linux image, pinned release tags, unprivileged runtime user, read-only mounted configuration, persistent writable host-key directory, restart policy, stdout logs, and a health command. The server must bind container interfaces, while a separate `advertise_host` setting names the LAN address clients/users should receive. No Docker socket mount is needed for ordinary operation. Config and secrets should be supplied at runtime; never bake a password or host key into an image.

Keep the container headless. Run `wormhole-server setup` interactively before startup or attach an operator TUI separately. Docker and Kubernetes startup must never stop at a prompt.

## Kubernetes

A Kubernetes Service exposes declared ports and maps them to target ports; multiple ports require names. `ClusterIP` is normally cluster-internal, which does not automatically make it reachable from employees' laptops. An office/VPN needs routing or a private load balancer. Internal load-balancer settings depend on the cloud/controller; `type: LoadBalancer` alone does not guarantee a private endpoint. [Kubernetes Service](https://kubernetes.io/docs/concepts/services-networking/service/)

Ordinary Ingress routes HTTP/HTTPS and does not expose arbitrary protocols such as Wormhole's SSH control channel. It also requires an installed controller. Expose SSH through a private TCP-capable Service/load balancer; use an existing internal ingress for the HTTP publishing listener if hostname mode is enabled. [Kubernetes Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/)

**Inference for Wormhole:** in port mode, Helm can enumerate a small configured service port range, but dynamic ports require Service changes or an additional controller. This is operationally heavier than one fixed HTTP listener and hostname routing. Do not introduce a privileged Kubernetes operator merely for the first release.

Start with exactly one server replica. The current tunnel lookup table and live SSH connections belong to one process; adding replicas behind a load balancer can send a browser connection to a pod that does not own its client tunnel. Persisting the host key preserves identity but does not persist active SSH sessions. A restart disconnects active tunnels; clients need backoff/reconnect. Use a rollout strategy that avoids overlapping independent servers claiming the same logical endpoint. Multi-replica operation needs explicit routing/session ownership or a relay design.

Store non-secret settings in a ConfigMap and credentials in a Secret or the organization's external secret system. Kubernetes Secrets require appropriate RBAC and encryption-at-rest configuration; they are not automatically encrypted in etcd by default. [ConfigMaps](https://kubernetes.io/docs/concepts/configuration/configmap/), [Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)

Provide readiness/liveness endpoints, requests/limits, graceful shutdown, and restricted management access. A NetworkPolicy can restrict permitted traffic only when the cluster's network implementation supports enforcement; it complements the private entrypoint and corporate firewall. [NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

## Windows Server

Prefer a native Windows binary for the first Windows Server installation. Go's official `golang.org/x/sys/windows/svc` package supports implementing a Windows Service; its example demonstrates installation, removal, lifecycle control, and Event Log integration. Wormhole would need OS-specific service handling around the same core server runtime, plus configurable paths and clean stop behavior. Simply registering the current console program with `sc.exe` would not add service protocol handling. [Go Windows service package](https://pkg.go.dev/golang.org/x/sys/windows/svc), [official service example](https://pkg.go.dev/golang.org/x/sys/windows/svc/example)

Do not instruct administrators to install Docker Desktop on Windows Server: Docker explicitly does not support that host platform. Microsoft's Windows container guidance describes supported Windows runtimes and host requirements; a Linux Wormhole image is not the same artifact as a native Windows container. Prefer a Linux VM for the Linux image, or build/test a separate Windows container image only when there is demand. [Docker Windows FAQ](https://docs.docker.com/desktop/troubleshoot-and-support/faqs/windowsfaqs/), [Microsoft container support policy](https://learn.microsoft.com/en-us/troubleshoot/windows-server/containers/support-for-windows-containers-docker-on-premises-scenarios)

## Proposed operator experience

Suggested command contract:

```text
wormhole-server init                 # generate example config and host identity
wormhole-server setup                # optional interactive setup TUI
wormhole-server config validate      # fail before deployment on invalid settings
wormhole-server serve --config PATH  # unattended foreground runtime
wormhole-server status --json        # scriptable operational status
wormhole-server tui                  # attach to an existing server
wormhole-server doctor               # bind/config/identity/connectivity diagnostics
```

The TUI should show bind and advertised addresses, health, tunnel owner/name, assigned URL/port, active connections and recent logs. It should operate over a local authenticated administration API; configuration changes should be validated before applying. It should never be required for the server to stay alive.

Configuration should distinguish control listener, publishing mode and listener/range, advertised host/domain, host-key path, authentication source, management bind address, log format, and timeouts. An interface binding does not define an authorization policy. Authentication for publishers and access control for visitors are separate requirements; a shared client token alone does not authenticate browser users.

## Recommended implementation order

1. Extract server runtime from `cmd/server/main.go`; implement validated config and headless lifecycle; remove hardcoded AWS IP/domain/certificate/password assumptions.
2. Implement bounded LAN port publishing and return assigned endpoint metadata to updated clients; pin the server host key instead of ignoring it; add client reconnect behavior.
3. Package native releases, Linux Docker image, Compose example, and end-to-end LAN instructions; test persistent identity and graceful restart behavior.
4. Add setup/management TUI on the same runtime/API; keep config and CLI fully usable without a terminal UI.
5. Add Windows Service integration and fixed-port hostname routing; test on actual Windows Server and the organization's DNS/TLS infrastructure.
6. Supply one-replica Kubernetes/Helm installation with private exposure, probes, ConfigMap/Secret mounts, and explicit restart/disconnection behavior.

No production uptime, cross-platform GUI/TUI behavior, Kubernetes installation, or Windows Server installation was tested during this research.
