# Wormhole

A lightweight self-hosted tunnel that exposes a local port to the public internet via an SSH relay server — similar to ngrok, but fully under your control.

---

## How it works

Wormhole creates a secure tunnel between your local machine and a public SSH server.

1. A server runs on a public machine and accepts SSH connections.
2. A client connects to the server and requests a tunnel.
3. The server assigns a unique readable token (e.g. `calm-fox-535990`).
4. Traffic to `https://calm-fox-535990.wormhole.mberrishdev.me` is forwarded through the SSH tunnel to your local service.

Each tunnel is isolated and ephemeral.

---

## Requirements

- Go 1.21+
- A publicly accessible server (VPS / EC2 / etc.)
- Wildcard DNS configured:

```
*.wormhole.your-domain.com → your server IP
```

---

## Install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/mberrishdev/wormhole/main/install.sh | bash
```

After installation:

```bash
wormhole
```

## Build

### Server

```bash
go build -o wormhole-server ./cmd/server
```

### Client

```bash
go build -o wormhole-client ./cmd/client
```

---

## Usage

### Server (public machine)

```bash
./wormhole-server --ssh-port 2222
```

On first run a persistent Ed25519 host key is generated (`server.key`).

---

### Client (local machine)

```bash
./wormhole-client --server <your-server-ip>:2222 --local localhost:3000
```

Once connected:

```
tunnel is live at: https://calm-fox-535990.wormhole.mberrishdev.me
dashboard: http://localhost:4040
```

---

## Client flags

| Flag         | Default              | Description                 |
| ------------ | -------------------- | --------------------------- |
| `--server`   | `13.62.136.105:2222` | SSH server address          |
| `--password` | `secret123`          | SSH authentication password |
| `--local`    | `localhost:3000`     | Local service to expose     |

---

## Dashboard

Every client automatically starts a local dashboard at `http://localhost:4040`.

When multiple clients run on the same machine they share the same dashboard — the first client to start owns it, subsequent clients register into it over HTTP.

| Column       | Description                                      |
| ------------ | ------------------------------------------------ |
| token        | Unique tunnel identifier                         |
| local        | Local address being exposed                      |
| public url   | Public HTTPS URL                                 |
| connections  | Number of currently active connections           |
| active ips   | IPs with an open connection right now            |
| started      | Time the tunnel was established                  |

The dashboard refreshes every 3 seconds. Sessions are removed automatically when the client exits.

---

## Security notes

This project is currently in early-stage development.

Before production use:

- Change the default SSH password (`secret123`)
- Do not use `InsecureIgnoreHostKey` in production
- Add firewall rules to restrict SSH port access

Recommended production setup:

- SSH key authentication instead of passwords
- Host key verification (pin the server key on the client)
- TLS termination via Caddy, Nginx, or Cloudflare

---

## Architecture

```
Client (local machine)
  ↓ SSH tunnel (port 2222)
Wormhole server (VPS)
  ↓ TLS (port 443) + wildcard DNS
Public users → your local application
```
