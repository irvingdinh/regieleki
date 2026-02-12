# Regieleki - Development Guide

## Build & Test

```bash
go build -o regieleki .       # or: make build
go test -race -v ./...        # run tests with race detector
go vet ./...                  # lint
```

## Project Structure

Single `main` package, stdlib-only (no external dependencies), Go 1.25+.

| File | Purpose |
|------|---------|
| `main.go` | Entry point, flag parsing, subcommand routing |
| `dns.go` | UDP DNS server, query parsing, upstream forwarding |
| `web.go` | HTTP API (CRUD records), serves embedded UI |
| `store.go` | Record persistence (TSV file), mutex-protected |
| `auth.go` | Token generation, loading, HTTP auth middleware |
| `index.html` | Admin UI (embedded via `go:embed`) |

## Key Defaults

- DNS: `:53`, HTTP: `:13860`
- Data file: `records.tsv` (or `/var/lib/regieleki/records.tsv` in production)
- Token file: specified via `-token` flag (or `/var/lib/regieleki/token` in production)

## Auth

- Token auth is optional; enabled when `-token <path>` flag is provided
- `regieleki access-token -token <path>` generates or shows the token
- API routes (`/api/*`) require `Authorization: Bearer <token>` header
- Static files (`/`, `/index.html`) are served without auth
