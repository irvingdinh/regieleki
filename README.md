# Regieleki

A lightweight DNS server with a web UI for managing custom DNS records on a private Tailscale network.

Built for personal use to resolve local domains (e.g., `app.my.local`) to Tailscale machine IPs without relying on external DNS services.

## Features

- Custom A, AAAA, and CNAME records
- Web UI for managing records
- Forwards unmatched queries to upstream DNS
- API token authentication
- Single binary, no external dependencies

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/irvingdinh/regieleki/main/install.sh | sudo bash
```

This installs the binary, creates the data directory, generates an API token, and sets up a systemd service.

## Usage

### Start the Server

```bash
regieleki -dns :53 -http :13860 -data /var/lib/regieleki/records.tsv -token /var/lib/regieleki/token
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-dns` | `:53` | DNS listen address |
| `-http` | `:13860` | HTTP listen address |
| `-data` | `records.tsv` | Path to records file |
| `-token` | _(empty)_ | Path to API token file (empty disables auth) |
| `-debug` | `false` | Enable debug logging |

### Access Token

Generate or retrieve your API token:

```bash
regieleki access-token -token /var/lib/regieleki/token
```

The token is stored in plaintext. On first run, a random 64-character hex token is generated.

### Web UI

Open `http://<server-ip>:13860` in your browser. You'll be prompted for the access token on first visit.

### API

All API endpoints require an `Authorization: Bearer <token>` header when auth is enabled.

```bash
TOKEN=$(regieleki access-token -token /var/lib/regieleki/token)

# List records
curl -H "Authorization: Bearer $TOKEN" http://localhost:13860/api/records

# Create record
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"domain":"app.my.local","type":"A","value":"100.70.30.1"}' \
  http://localhost:13860/api/records

# Update record
curl -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"domain":"app.my.local","type":"A","value":"100.70.30.2"}' \
  http://localhost:13860/api/records/1

# Delete record
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://localhost:13860/api/records/1
```

## systemd

```bash
sudo systemctl status regieleki
sudo systemctl restart regieleki
journalctl -u regieleki -f
```

## Build from Source

```bash
make build
```
