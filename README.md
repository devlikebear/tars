# TARS

Language docs:

- Korean: [README.ko.md](README.ko.md)
- Japanese: [README.ja.md](README.ja.md)
- English: this file (`README.md`)

Additional docs:

- Contributing (versioning/PR policy): [CONTRIBUTING.md](CONTRIBUTING.md)
- Change log: [CHANGE.log](CHANGE.log)
- License: [LICENSE](LICENSE)

TARS is a lightweight local AI automation stack with two Go binaries:

- `tarsd`: daemon/server (LLM orchestration, sessions, tools, gateway, automation)
- `tars`: terminal client (Bubble Tea 3-pane TUI)

The current architecture is intentionally simplified for public use and operations.

## Features

- Chat API with SSE streaming (`/v1/chat`)
- Session lifecycle (`/v1/sessions`, history/export/search, compact)
- Agent loop + built-in tools (`read/write/edit/glob/exec/process/memory/cron/heartbeat/...`)
- In-process gateway runtime
  - async runs (`/v1/agent/runs`)
  - channels/webhooks
  - browser/nodes/message tools
- Skills/plugins/MCP hot-reload (`/v1/runtime/extensions/reload`)
- Browser automation runtime
  - status/profiles/login/check/run API
  - local browser relay (`/extension`, `/cdp`) with token/origin/loopback checks
- Vault read-only integration for browser auto-login workflows (opt-in)

## Repository Layout

- `cmd/tarsd`: main server
- `cmd/tars`: Go TUI client
- `internal/*`: runtime modules (gateway, tool, llm, session, extensions, browser, vaultclient, ...)
- `config/tarsd.config.example.yaml`: example config
- `workspace/`: local runtime workspace (sessions, memory, automation, etc.)

## Quick Start

## 1) Prerequisites

- Go 1.24+
- LLM provider credential (e.g. `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`)

## 2) Configure

Use your runtime config (default local path in this repo):

- `workspace/config/tarsd.config.yaml`

You can also start from:

- `config/tarsd.config.example.yaml`

## 3) Run server

```bash
make dev-tarsd
# or directly:
# go run ./cmd/tars serve --verbose --serve-api --config ./workspace/config/tarsd.config.yaml
```

Default API address:

- `http://127.0.0.1:43180`

## 4) Run client

```bash
make dev-tars
```

## 5) Smoke checks

```bash
make api-status
make api-sessions
make smoke-auth
```

## Authentication / Authorization

`api_auth_mode` supports role-aware tokens:

- `api_user_token`: chat/general operations
- `api_admin_token`: control operations (`/v1/runtime/extensions/reload`, `/v1/gateway/reload`, `/v1/gateway/restart`, channel inbound endpoints)

Workspace model is fixed to a single `workspace_dir` (no `workspace_id` routing).

## cmd/tars Highlights

- Chat + streaming status trace panel
- Session commands: `/new`, `/sessions`, `/resume`, `/history`, `/export`, `/search`
- Runtime commands: `/agents`, `/spawn`, `/runs`, `/run`, `/cancel-run`, `/gateway`, `/channels`
- Automation: `/cron`, `/notify`, `/heartbeat`
- Browser/Vault:
  - `/browser status|profiles|login|check|run`
  - `/vault status`

## Browser + Vault (Opt-in)

Enable in `tarsd` config:

- `vault_enabled: true`
- `browser_runtime_enabled: true`
- `browser_relay_enabled: true`
- `tools_browser_enabled: true`

Optional site flow directory:

- `browser_site_flows_dir: ./workspace/automation/sites`

For `vault_form` login mode, enforce allowlists:

- `vault_secret_path_allowlist_json`
- `browser_auto_login_site_allowlist_json`

## Vault via Docker Compose (dev)

Run local Vault + one-shot initializer:

```bash
docker compose -f docker-compose.vault.yaml up -d
docker compose -f docker-compose.vault.yaml logs -f vault-init
```

What this sets up:

- Vault dev server at `http://127.0.0.1:8200`
- KV v2 mount: `tars`
- sample secret: `tars/sites/grafana` (`username`, `password`)
- readonly policy: `tars-readonly`
- readonly token printed in `vault-init` logs

Stop:

```bash
docker compose -f docker-compose.vault.yaml down
```

## Testing

```bash
make test
# or
go test ./... -count=1
```

## Security Scan

```bash
make security-scan
```

This runs:

- `gitleaks` history scan
- absolute local path leak check (e.g. `/Users/...`)
- private key marker check in tracked files

## Notes

- `cased` sentinel daemon was removed during simplification.
- Process supervision is delegated to systemd/launchd/docker in production.
- `GET /v1/healthz` remains available for external health probing.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for versioning and PR policy.
