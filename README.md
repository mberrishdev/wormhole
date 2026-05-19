# Wormhole

A lightweight self-hosted tunnel that exposes a local port to the public internet via an SSH relay server — similar to ngrok, but fully under your control.

---

## How it works

Wormhole creates a secure tunnel between your local machine and a public SSH server.

1. A server runs on a public machine and accepts SSH connections.
2. A client connects to the server and requests a tunnel.
3. The server assigns a unique readable token (e.g. `calm-fox-535990`).
4. Traffic to:

```
https://calm-fox-535990.wormhole.mberrishdev.me
```

is forwarded through the SSH tunnel to your local service.

Each tunnel is isolated and ephemeral.

---

## Requirements

- Go 1.21+
- A publicly accessible server (VPS / EC2 / etc.)
- Wildcard DNS configured:

```
*.wormhole.your-domain.com → your server IP
```

- Optional but recommended: TLS termination (Caddy / Nginx / Cloudflare)

---

## Install (recommended)

The easiest way to install the client is:

```bash
curl -fsSL https://raw.githubusercontent.com/mberrishdev/wormhole/main/install.sh | bash
```

After installation:

```bash
chmod +x wormhole

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

On first run:

- a persistent Ed25519 key is generated (`server.key`)
- server starts listening for SSH tunnels

---

### Client (local machine)

```bash
./wormhole-client   --server <your-server-ip>:2222   --local localhost:3000
```

---

## Client flags

| Flag         | Default              | Description                 |
| ------------ | -------------------- | --------------------------- |
| `--server`   | `13.62.136.105:2222` | SSH server address          |
| `--password` | `secret123`          | SSH authentication password |
| `--local`    | `localhost:3000`     | Local service to expose     |

---

## Output

Once connected:

```
tunnel is live at:
http://calm-fox-535990.wormhole.mberrishdev.me
```

Anyone can access your local service through this URL.

---

## Security notes

This project is currently in early-stage development.

Before production use:

- Change default SSH password (`secret123`)
- Do not use `InsecureIgnoreHostKey` in production
- Do not expose raw SSH without firewall rules

Recommended production setup:

- Use SSH key authentication instead of passwords
- Enable host key verification (pin server key)
- Add TLS termination using Caddy, Nginx, or Cloudflare

---

## Architecture summary

Client (local machine)
↓ SSH tunnel
Wormhole server (VPS)
↓ routing
Wildcard DNS (\*.wormhole.domain)
↓
Public users → local application

---

## Future improvements

- `wormhole connect 3000` CLI UX
- automatic install script (`curl | bash`)
- TLS built-in support
- multi-tunnel sessions
- dashboard UI
