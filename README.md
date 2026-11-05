# Wormhole

Share an app running on your laptop with people on your LAN or company VPN. An admin runs one Wormhole server; developers run the client; everyone else just opens a link.

## Install

On **Linux or macOS**, install the part you need:

```bash
# Admin: install on the machine that will host the server
curl -fsSL https://raw.githubusercontent.com/mberrishdev/wormhole/main/install.sh | bash -s -- server

# Developer: install on your own laptop
curl -fsSL https://raw.githubusercontent.com/mberrishdev/wormhole/main/install.sh | bash -s -- client
```

The tools are installed in `~/.local/bin`. If your terminal cannot find them, run:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

The installer downloads binaries from [GitHub Releases](https://github.com/mberrishdev/wormhole/releases). **This version must be released first.** Until then, build from source below.

On **Windows**, download the server or client `.exe` for your machine from Releases. Run it in PowerShell, for example `.\wormhole-server.exe init`.

## Admin: set up the server

```bash
wormhole-server init
```

The wizard lets you choose the deployment, enter the server's existing LAN IP, and review the settings. Press **Generate setup files** to save them.

For a quick start, choose **Native** and **LAN IP + ports**, then run:

```bash
wormhole-server serve --config wormhole-setup/server.json
```

For **Docker Compose** or **Kubernetes**, follow the generated `wormhole-setup/NEXT-STEPS.txt`. These need Docker or a configured cluster; the server installer alone does not install them or provide a prebuilt container image.

Send only `wormhole-setup/client.json` to developers through a trusted channel. Keep the rest of that directory private. Allow the chosen ports through your LAN/VPN firewall.

## Developer: share your app

Get `client.json` from your admin. Start your app, then run:

```bash
wormhole-client --config client.json --local localhost:3000 --name frontend
```

Share the printed URL, such as `http://192.168.1.100:8081`. Teammates on the LAN/VPN can open it in a browser. Stop the client to close the tunnel.

## View the server

For a native server, open another terminal:

```bash
wormhole-server tui --config wormhole-setup/server.json
```

Use **1–4** to switch views and **q** to close the TUI. The server keeps running.

## Build from source

From a checkout containing this version, with **Go 1.25+** installed:

```bash
go build -o bin/wormhole-server ./cmd/server
go build -o bin/wormhole-client ./cmd/client
./bin/wormhole-server init
```

Use `./bin/wormhole-server` and `./bin/wormhole-client` instead of the installed commands. On Windows, add `.exe` to the build output names.

Anyone who can reach a tunnel can access the app. Use your app's login and internal HTTPS when needed.

See the [deployment guide](docs/deployment.md) for Docker, Kubernetes, Windows services, domains, and configuration.
