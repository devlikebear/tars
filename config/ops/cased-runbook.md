# cased Single-Target Runbook

This runbook assumes one `cased` process supervises one `tarsd` process.

## 1) Prepare config

- `config/cased.config.yaml`
  - `target_command`: required (`go`, binary path, etc.)
  - `target_args_json`: JSON array of args
  - `probe_url`: usually `http://127.0.0.1:43180/v1/healthz`
  - `probe_start_grace_ms`: startup grace window
  - `event_persistence_enabled`: set `true` for local history

## 2) Start manually

```bash
go run ./cmd/cased --config ./config/cased.config.yaml --verbose
```

## 3) Verify sentinel state

```bash
curl -s http://127.0.0.1:43181/v1/sentinel/status | jq
curl -s 'http://127.0.0.1:43181/v1/sentinel/events?limit=20' | jq
```

## 4) Control actions

```bash
curl -s -X POST http://127.0.0.1:43181/v1/sentinel/pause | jq
curl -s -X POST http://127.0.0.1:43181/v1/sentinel/resume | jq
curl -s -X POST http://127.0.0.1:43181/v1/sentinel/restart | jq
```

## 5) TUI checks

```bash
make dev-tars-ui
# in tars-ui:
# /sentinel
# /sentinel events 20
```

## 6) Optional service install

- Linux: copy `.docs/ops/cased.systemd.service.example` to `/etc/systemd/system/cased.service`
- macOS: copy `.docs/ops/cased.launchd.plist.example` to `~/Library/LaunchAgents/dev.tars.cased.plist`

Use these templates as examples and adjust absolute paths/user.
