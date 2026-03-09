# TARS

TARS is a local-first automation runtime written in Go.

It combines a terminal client, a local HTTP runtime, agent tools, sessions, scheduling, and optional browser automation in a single repository.

## Status

- Pre-`1.0.0`
- Primary module path: `github.com/devlikebear/tars`
- Version source: [`VERSION.txt`](VERSION.txt)
- Release notes: [`CHANGELOG.md`](CHANGELOG.md)

## Core Capabilities

- Terminal client with a Bubble Tea TUI
- Local HTTP API via `tars serve`
- Session lifecycle and transcript storage
- Agent loop with built-in file, process, scheduling, memory, and ops tools
- Runtime extension loading for skills, plugins, and MCP servers
- Playwright-based browser automation
- Optional macOS assistant workflow

## Requirements

- Go `1.25.6` or newer
- Provider credentials for the models you want to use
- Optional: Node.js for Playwright browser installation

## Install

Homebrew tap:

```bash
brew tap devlikebear/tars
brew install devlikebear/tars/tars
```

Curl installer:

```bash
curl -fsSL https://raw.githubusercontent.com/devlikebear/tars/main/install.sh | sh
```

The installer downloads the latest published GitHub Release by default.

Install to a custom path or pin a version:

```bash
curl -fsSL https://raw.githubusercontent.com/devlikebear/tars/main/install.sh | INSTALL_DIR="$HOME/.local/bin" VERSION=0.1.0 sh
```

## Quick Start

1. Review the example config:

```bash
cp config/tars.config.example.yaml workspace/config/tars.config.yaml
```

2. Start the local server:

```bash
make dev-serve
```

3. Start the client:

```bash
make dev-tars
```

4. Run basic checks:

```bash
make api-status
make api-sessions
make smoke-auth
```

## Build

Build the binary with version metadata from [`VERSION.txt`](VERSION.txt):

```bash
make build-bins
bin/tars version
```

Build a macOS release archive with the same version metadata used by GitHub Releases:

```bash
make release-asset RELEASE_GOOS=darwin RELEASE_GOARCH=arm64
```

## Browser Automation

Playwright runtime is the primary browser automation path.

- Install browser dependencies with `make browser-install`
- Configure browser-related settings in `workspace/config/tars.config.yaml`
- Use the runtime APIs and TUI commands for browser status, profiles, login checks, and runs

The Chrome relay extension in [`web/relay-extension/README.md`](web/relay-extension/README.md) is still available as an experimental legacy workflow for local debugging.

## Security

- Default API auth mode is token-based and role-aware
- High-risk tools are restricted by default on non-admin routes
- Run `make security-scan` before publishing or tagging a release

## Repository Layout

- `cmd/tars`: CLI entrypoint for client, server, and assistant commands
- `internal/*`: runtime packages
- `config/`: example and standalone configuration
- `workspace/`: local runtime state, ignored by Git
- `web/relay-extension/`: optional Chrome extension for the legacy relay flow

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for development workflow, release rules, and PR requirements.

## Getting Started

See [`GETTING_STARTED.md`](GETTING_STARTED.md) for a short setup and operations guide.
