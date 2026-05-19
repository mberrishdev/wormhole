# Wormhole

A lightweight tunnel that exposes a local port to the public internet via an SSH relay server — similar to ngrok, but self-hosted.

## How it works

1. The **server** runs on a public machine and listens for SSH connections.
2. The **client** connects and receives a randomly generated readable token (e.g. `calm-fox-535990`).
3. The server routes incoming traffic for `<token>.wormhole.mberrishdev.me` through the SSH tunnel to your local service.

Each tunnel gets a unique subdomain — no port numbers or query parameters needed.

## Requirements

- Go 1.21+
- A publicly accessible server with a wildcard DNS record pointing `*.wormhole.<your-domain>` at it

## Build

```bash
# Build the server
go build -o wormhole-server ./cmd/server

# Build the client
go build -o wormhole-client ./cmd/client
```

## Usage

### Server

Run on your public machine:

```bash
./wormhole-server --ssh-port 2222
```

The server generates a persistent `server.key` (Ed25519) on first run.

### Client

Run on your local machine:

```bash
./wormhole-client \
  --server <your-server-ip>:2222 \
  --local localhost:3000
```

| Flag | Default | Description |
|------|---------|-------------|
| `--server` | `13.62.136.105:2222` | SSH server address |
| `--password` | `secret123` | SSH password |
| `--local` | `localhost:3000` | Local service to expose |

Once connected, the client prints the public URL:

```
2026/05/20 00:37:22 tunnel is live at: http://calm-fox-535990.wormhole.mberrishdev.me
```

Anyone with that URL can reach your local service.

## Security notes

- The SSH password is hardcoded (`secret123`) — change it before deploying.
- `HostKeyCallback` is set to `InsecureIgnoreHostKey` — pin the host key for production use.
- Add TLS termination (e.g. Caddy or nginx with Let's Encrypt wildcard cert) in front of the server for production traffic.
