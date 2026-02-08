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

Or run over TCP:

```bash
go run . serve --addr 127.0.0.1:8080
go run . query --url http://127.0.0.1:8080
```

Query one plugin:

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

- `--addr` (default `unix://$XDG_RUNTIME_DIR/gopenusage/gopenusage.sock` with safe fallbacks)
- `--plugins-dir` (optional path to plugin manifests/icons)
- `--data-dir` (default `${XDG_CONFIG_HOME}/gopenusage`)

### `query`

Calls the running JSON API and prints pretty JSON.

```bash
go run . query [plugin-id] [flags]
```

Flags:

- `--url` (default `http://127.0.0.1:8080`)
- `--plugin` (alternative to positional plugin id)
- `--socket` (optional unix socket path; auto-detected from `$XDG_RUNTIME_DIR/gopenusage/gopenusage.sock` when `--url` is not set)
- `--timeout` (default `15s`)

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

```go
package main

import (
    "context"
    "fmt"

    "github.com/deicod/gopenusage/pkg/openusage"
    "github.com/deicod/gopenusage/pkg/openusage/builtin"
)

func main() {
    manager, err := openusage.NewManager(openusage.Options{
        PluginsDir: "openusage/plugins",
    }, builtin.Plugins())
    if err != nil {
        panic(err)
    }

    out, err := manager.QueryOne(context.Background(), "copilot")
    if err != nil {
        panic(err)
    }

    fmt.Printf("%s: %s\n", out.ProviderID, out.Plan)
}
```

Main types:

- `openusage.Manager`
- `openusage.PluginOutput`
- `openusage.MetricLine`

Reusable API client:

```go
package main

import (
    "context"
    "fmt"

    openusageclient "github.com/deicod/gopenusage/pkg/openusage/client"
)

func main() {
    c, err := openusageclient.New(openusageclient.Options{
        BaseURL: "http://127.0.0.1:8080",
    })
    if err != nil {
        panic(err)
    }

    output, err := c.QueryOne(context.Background(), "copilot")
    if err != nil {
        panic(err)
    }

    fmt.Println(output.ProviderID, output.Plan)
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
