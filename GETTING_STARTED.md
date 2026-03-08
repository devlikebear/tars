# TARS Getting Started

This guide covers the minimum path to run TARS locally and verify the main workflows.

## 1. Prepare Local Config

Start from the example config and adjust provider credentials as needed.

```bash
cp config/tars.config.example.yaml workspace/config/tars.config.yaml
```

Common environment files:

- `.env.example`: checked-in reference values
- `.env`: local development overrides
- `.env.secret`: local secrets, not tracked

## 2. Run the Server

Start the local runtime:

```bash
make dev-serve
```

Default API endpoint:

- `http://127.0.0.1:43180`

The debug log is appended to `.logs/tars-debug.log`.

```bash
tail -f .logs/tars-debug.log
```

## 3. Run the Client

Open the terminal client:

```bash
make dev-tars
```

Useful first commands inside the client:

```text
/health
/whoami
/status
```

## 4. Validate the Local Runtime

Use the helper targets to confirm the API is reachable:

```bash
make api-status
make api-sessions
make smoke-auth
```

Run the test suite and repository security scan before publishing changes:

```bash
make test
make security-scan
```

## 5. Build a Versioned Binary

Binary builds embed version information from `VERSION.txt` plus the current Git commit and build date.

```bash
make build-bins
bin/tars version
```

## 6. Optional: Playwright Browser Runtime

Playwright is the primary browser automation path.

Install browser dependencies:

```bash
make browser-install
```

Then configure browser settings in `workspace/config/tars.config.yaml` and use the browser runtime commands from the TUI or API.

## 7. Optional: macOS Assistant

Check local dependencies:

```bash
tars assistant doctor
```

Start the assistant manually:

```bash
tars assistant start --server-url http://127.0.0.1:43180
```

Install the LaunchAgents:

```bash
make install
launchctl list | rg 'io.tars.server|io.tars.assistant'
```

## 8. Optional: Relay Extension

The Chrome relay extension remains available for local experimental use, but it is not the primary browser workflow.

See [`web/relay-extension/README.md`](web/relay-extension/README.md) if you need that path.
