# systemd user service

This directory contains a user unit for running `gopenusage` as a per-user background service.

## Why `%t` for the socket path?

For **user** units, systemd's standard runtime location is `%t`, which expands to `$XDG_RUNTIME_DIR` (typically `/run/user/<uid>`).

That is why this service listens on:

- `unix://%t/gopenusage/gopenusage.sock`

This is preferred over hardcoding `/run/...` for user services.

## Install and enable (user)

1. Build/install the binary:

```bash
mkdir -p ~/.local/bin
go build -o ~/.local/bin/gopenusage .
```

2. Install the unit:

```bash
mkdir -p ~/.config/systemd/user
cp contrib/systemd/gopenusage.service ~/.config/systemd/user/
```

3. Edit `~/.config/systemd/user/gopenusage.service` if needed:

- `Environment=PATH=...` if `gopenusage` is installed somewhere else
- `WorkingDirectory` (default points at `%h/go/src/github.com/deicod/gopenusage` so `openusage/plugins` resolves correctly)

4. Reload and start:

```bash
systemctl --user daemon-reload
systemctl --user enable --now gopenusage.service
```

5. Verify:

```bash
systemctl --user status gopenusage.service
```

## Querying the API over Unix socket

Using `gopenusage query`:

```bash
gopenusage query antigravity --socket "$XDG_RUNTIME_DIR/gopenusage/gopenusage.sock"
```

Or via curl:

```bash
curl --unix-socket "$XDG_RUNTIME_DIR/gopenusage/gopenusage.sock" http://localhost/v1/usage
```

## Optional: start even without interactive login

```bash
loginctl enable-linger "$USER"
```
