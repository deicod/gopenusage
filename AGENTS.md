# Repository Guidelines

## Project Structure & Module Organization
This repository is a Go module (`github.com/deicod/gopenusage`) that ships both a reusable package and a CLI/API service.
- `main.go`: CLI entrypoint.
- `cmd/`: Cobra commands (`serve`, `query`) and command-level tests.
- `internal/api/`: HTTP server implementation and API tests.
- `pkg/openusage/`: core manager, plugin interfaces, manifests, and typed models.
- `pkg/openusage/client/`: reusable HTTP client for `/v1/usage`.
- `pkg/openusage/plugins/`: provider-specific plugin implementations.
- `contrib/systemd/`: user-level systemd unit and setup docs.

## Build, Test, and Development Commands
- `go test ./...`: run all unit tests across `cmd`, `internal`, and `pkg`.
- `go build ./...`: compile all packages and catch build-time regressions.
- `go run . serve`: start the JSON API service (Unix socket by default).
- `go run . query`: query all providers from the running service.
- `go run . query copilot --url http://127.0.0.1:8080`: query a single provider over TCP.

## Coding Style & Naming Conventions
Follow standard Go conventions:
- Format with `gofmt` (tabs for indentation, canonical spacing).
- Keep package names short, lowercase, and descriptive (`api`, `openusage`).
- Exported identifiers use `CamelCase`; unexported identifiers use `camelCase`.
- Prefer explicit, behavior-oriented names (`resolveQuerySocketPath`, `NewManager`).
- Keep provider logic isolated under `pkg/openusage/plugins/<provider>/`.

## Testing Guidelines
- Use Go’s `testing` package with table-driven or focused unit tests.
- Place tests next to implementation files using `*_test.go`.
- Name tests with `TestXxx` and target behavior, not internals.
- Use targeted runs when iterating, e.g. `go test ./cmd -run TestResolveQuerySocketPath`.
- Ensure `go test ./...` passes before opening a PR.

## Commit & Pull Request Guidelines
Recent commits use short, imperative-style subjects (for example: `Rename module...`, `Fix .gitignore scope...`).
- Write concise commit titles in imperative mood; keep one logical change per commit.
- In PRs, include: purpose, key changes, test evidence (`go test ./...` output), and any behavior/API impacts.
- Link related issues and include sample CLI/API output when changing command behavior.
