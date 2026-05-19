# Wormhole

A lightweight tunnel that exposes a local port to the public internet via an SSH relay server — similar to ngrok, but self-hosted.

## How it works

1. The **server** runs on a public machine and listens for SSH connections.
2. The **client** connects to the server over SSH and requests that a public port be opened.
3. Incoming HTTP traffic on that port is validated by a token, then forwarded through the SSH tunnel to your local service.

## Requirements

- Go 1.21+
- A publicly accessible server (e.g. an EC2 instance)

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
  --local localhost:3000 \
  --public-port 8888 \
  --token mysecrettoken
```

| Flag | Default | Description |
|------|---------|-------------|
| `--server` | `13.53.40.46:2222` | SSH server address |
| `--password` | `secret123` | SSH password |
| `--local` | `localhost:3000` | Local service to expose |
| `--public-port` | `8888` | Port to open on the server |
| `--token` | *(random)* | Token to protect the public endpoint |

The client prints a public URL once connected:

```
public URL: http://<server-ip>:8888/?token=<token>
```

Anyone with that URL can reach your local service. Requests without the correct token receive a `403 Forbidden`.

## Security notes

- The SSH password is hardcoded (`secret123`) — change it before deploying.
- `HostKeyCallback` is set to `InsecureIgnoreHostKey` — pin the host key for production use.
- The token is passed as a query parameter; use HTTPS in front of the server for production traffic.
