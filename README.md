# gopenusage

Go package + CLI service for tracking AI coding subscription usage across providers.

## What This Project Provides

1. A reusable Go package that queries provider plugins and returns typed Go structs.
2. A JSON API service (`serve`) built with Cobra.
3. A CLI client (`query`) that calls the JSON API.

This repo intentionally focuses on backend/plugin querying. The UI app lives under `openusage/` and is used as the plugin reference source.

## Supported Plugins

- `antigravity`
- `claude`
- `codex`
- `copilot`
- `cursor`
- `mock`
- `windsurf`

## Quick Start

```bash
go test ./...
```

Start API server (default Unix socket):

```bash
go run . serve
```

Query all plugins (auto-detects default Unix socket if present):

```bash
go run . query
```

Query one plugin over the default Unix socket:

```bash
go run . query antigravity
```

Or run over TCP:

```bash
go run . serve --addr 127.0.0.1:8080
go run . query --url http://127.0.0.1:8080
```

Query one plugin over TCP:

```bash
go run . query antigravity --url http://127.0.0.1:8080
```

User-level systemd setup guide:

- `contrib/systemd/README.md`

## CLI Commands

### `serve`

Runs the JSON API service.

```bash
go run . serve [flags]
```

Flags:

- `--addr` (default `unix://$XDG_RUNTIME_DIR/gopenusage/gopenusage.sock`; fallback: `/run/user/<uid>/gopenusage/gopenusage.sock` or `${TMPDIR}/gopenusage/gopenusage.sock`)
- `--plugins-dir` (optional path to plugin manifests/icons)
- `--data-dir` (default `${XDG_CONFIG_HOME}/gopenusage`)

### `query`

Calls the running JSON API and prints pretty JSON.

```bash
go run . query [plugin-id] [flags]
```

Flags:

- `--url` (default `http://127.0.0.1:8080`; auto-detected socket is only used when `--url` is not explicitly set)
- `--plugin` (alternative to positional plugin id)
- `--socket` (optional unix socket path; when set, requests are sent over this socket)
- `--timeout` (default `15s`)

Socket precedence for `query`:

1. If `--socket` is set, use that Unix socket.
2. Else if `--url` is explicitly set, use URL over TCP/HTTP(S).
3. Else auto-detect the default Unix socket path and use it if present.
4. Else fall back to `--url` default (`http://127.0.0.1:8080`).

## JSON API

### `GET /healthz`

Returns:

```json
{"ok": true}
```

### `GET /v1/usage`

Returns all plugin outputs.

Optional query param:

- `plugins=codex,copilot` (comma-separated plugin ids)

### `GET /v1/usage/{pluginId}`

Returns one plugin output.

## Reusable Package Usage

Manager example (`pkg/openusage`):

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/builtin"
)

func main() {
	manager, err := openusage.NewManager(openusage.Options{
		// Optional:
		// PluginsDir: "/path/to/openusage/plugins",
		// DataDir:    "/path/to/state",
	}, builtin.Plugins())
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	outputs, err := manager.QueryAll(ctx, nil) // nil = all plugins
	if err != nil {
		log.Fatal(err)
	}

	for _, out := range outputs {
		if out.Error != "" {
			fmt.Printf("%s: error: %s\n", out.ProviderID, out.Error)
			continue
		}
		fmt.Printf("%s: plan=%s, lines=%d\n", out.ProviderID, out.Plan, len(out.Lines))
	}
}
```

Main types:

- `openusage.Manager`
- `openusage.PluginOutput`
- `openusage.MetricLine`

Reusable API client example (`pkg/openusage/client`):

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	openusageclient "github.com/deicod/gopenusage/pkg/openusage/client"
)

func main() {
	client, err := openusageclient.New(openusageclient.Options{
		// TCP server:
		BaseURL: "http://127.0.0.1:8080",
		// Or Unix socket:
		// SocketPath: "/run/user/1000/gopenusage/gopenusage.sock",
		Timeout: 15 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	one, err := client.QueryOne(ctx, "copilot")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("one: %s plan=%s\n", one.ProviderID, one.Plan)

	selected, err := client.QueryPlugins(ctx, []string{"copilot", "codex"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("selected plugins: %d\n", len(selected))
}
```

## Provider Prerequisites

- `copilot`: run `gh auth login`.
- `codex`: requires Codex auth file (`CODEX_HOME/auth.json`, `~/.config/codex/auth.json`, or `~/.codex/auth.json`).
- `claude`: requires `~/.claude/.credentials.json` or Keychain credentials.
- `cursor`: requires Cursor `state.vscdb` with auth tokens.
- `antigravity`: requires Antigravity app running (local language server available).
- `windsurf`: requires Windsurf/Windsurf Next running and auth status in SQLite.
- `mock`: no prerequisites.

## Repository Layout

- `cmd/`: Cobra commands (`serve`, `query`).
- `contrib/systemd/`: user-level systemd unit + setup instructions.
- `internal/api/`: HTTP server handlers.
- `pkg/openusage/`: reusable core package.
- `pkg/openusage/client/`: reusable JSON API client package.
- `pkg/openusage/plugins/*`: provider-specific implementations.
- `openusage/plugins/*`: source plugin manifests/icons used for metadata.

## Notes

- Plugin outputs include provider errors as structured data (`error` + `lines`), so API consumers can display partial results safely.
- Some providers are reverse-engineered and may change behavior without notice.
