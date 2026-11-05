# Try the server TUI prototype

From the repository root, in an interactive terminal:

```bash
go run ./cmd/server-tui --demo
```

Requires Go 1.25 or newer and a terminal at least 58 columns by 27 rows. The first run may download dependencies. Styling uses Lip Gloss; keyboard input and rendering use Bubble Tea.

- `1`, `2`, `3` or `Tab`: switch Overview, Tunnels, and Setup.
- Up/down: select a tunnel or setup field.
- Setup: `Enter` edits a field, `Ctrl+U` clears it, `Enter` applies it, `Esc` cancels it. LAN IP must be a private IP. SSH port must be unprivileged and outside the tunnel range.
- Setup: `r` previews the configuration draft; `r` or `Esc` returns.
- `q` quits outside a text field; `Ctrl+C` always quits.

This is a runnable UX prototype with fixed sample server data. Setup edits an in-memory draft only. It does not open ports, connect to AWS, start Docker, create credentials, write config, or modify the existing server. The proposed draft format is not supported by the current server. Test real tunnel deployment after that implementation is complete.
