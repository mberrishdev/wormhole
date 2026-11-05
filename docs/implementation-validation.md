# LAN implementation validation

The runnable implementation is documented in [README](../README.md). The earlier setup plan is a historical proposal; not every future feature in it is part of this change.

Completed locally:

- `go test -race ./...`: passed. Includes real SSH and HTTP integration tests, hostname routing, authentication/fingerprint rejection, management access control, bounded port capacity, cleanup/reallocation, unavailable local apps, and preserving TCP half-close responses.
- `go vet ./...`: passed.
- Linux amd64 server and Windows amd64 server/client cross-compilation: passed.
- Generated Compose configuration: `docker compose config --quiet` passed.
- Installer shell syntax and `git diff --check`: passed.
- Atlas terminal check: selected native deployment, reviewed settings, explicitly generated files, and viewed the success screen. Started that generated server and checked the live monitoring TUI against its management API.
- Native CLI smoke test in the helper: client retried while server was unavailable, then connected and forwarded a request through SSH to a local HTTP app; returned `wormhole-live-ok`. The live TUI/API showed the connected client and tunnel. Test processes, copied binaries, and generated helper configuration were cleaned up.

Environment limits:

- Local Docker daemon was not running. Image build and container execution were not tested locally. A CI Compose job builds the image and forwards a real HTTP request; its result must be checked on the PR.
- Kubernetes resources were generated and structurally exercised in tests, but were not applied to a cluster. Private load balancer, routing, Ingress controller, wildcard DNS and TLS configuration remain environment-specific admin work.
- Windows binaries cross-compiled, but Windows Service installation/start/stop was not exercised on a Windows Server host.
- Tests used loopback. Reachability and firewall isolation from a second physical LAN/VPN machine must be checked in the target network.

The protocol change requires updating both client and server. Publisher authentication initially uses a shared generated credential. Visitor authentication, per-developer credentials/revocation UI, multi-replica operation, and configuration hot reload are not implemented.
