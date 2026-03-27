# Ops Service Demo Repository

This repository is a small Dockerized Go service used by the TARS `ops-service` example workflow.

## Commands

```bash
docker compose up -d --build
./opsctl status
./opsctl inject-failure timeout
./opsctl errors
./opsctl clear-failure
```

## Failure Modes

- `timeout` produces recurring structured timeout logs and a degraded `/healthz`
- `http500` produces recurring structured downstream 500 logs and a degraded `/healthz`

## Tests

```bash
go test ./...
```
