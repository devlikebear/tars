# TARS Plugin and MCP Packaging Guide

TARS extensions are deliberately on-demand. Use a **skill plus companion CLI** for most domain-specific workflows, use a **hub MCP package** when an external tool server is the right boundary, and use a **plugin** only when you need to bundle skills and optional MCP declarations together with runtime gating metadata.

Plugins do not provide a built-in Go extension surface. Built-in Go plugins and plugin HTTP routes were removed; manifests may still carry compatibility fields, but the active runtime path is skills plus MCP.

## Extension Model

| Package | Primary use | Runtime prompt impact |
|---------|-------------|-----------------------|
| Skill | Markdown instructions, optional companion files/CLIs | Loaded only when the skill is available or invoked |
| Hub MCP package | Standalone local/remote MCP server package | Tools are registered when the MCP server is enabled |
| Plugin | Bundle skill directories and optional MCP server declarations | Skill dirs join the skill loader; plugin MCP is opt-in |

The recommended pattern is:

1. Put workflow instructions in `SKILL.md`.
2. Put real integration logic in a companion CLI/script.
3. Tell the model to call that CLI through the existing `bash` tool.
4. Reach for MCP only when a long-running server protocol is genuinely useful.

## Plugin Manifest

Every plugin is a directory containing a `tars.plugin.json` manifest. Current parser support spans schema versions 1-3; omitted `schema_version` is treated as `2`.

```json
{
  "schema_version": 2,
  "id": "release-helper",
  "name": "Release Helper",
  "description": "Skills for release preparation and validation.",
  "version": "0.1.0",
  "requires": {
    "bins": ["git", "gh"],
    "env": ["GITHUB_TOKEN"]
  },
  "supported_os": ["darwin", "linux"],
  "supported_arch": ["arm64", "amd64"],
  "policies": {
    "tools_allow": ["read_file", "bash"],
    "tools_deny": ["write_file"]
  },
  "skills": ["skills"],
  "mcp_servers": []
}
```

| Field | Type | Required | Active behavior |
|-------|------|----------|-----------------|
| `schema_version` | int | no | Defaults to `2`; versions `1`, `2`, and `3` parse |
| `id` | string | yes | Unique plugin identifier |
| `name` | string | no | Display name |
| `description` | string | no | Short summary |
| `version` | string | no | Display/package version |
| `requires` | object | no | Gates plugin availability by binaries/env vars |
| `supported_os` | string[] | no | Gates by `GOOS` |
| `supported_arch` | string[] | no | Gates by `GOARCH` |
| `policies` | object | no | Declared policy metadata for operators/UI |
| `skills` | string[] | no | Relative skill directories inside the plugin root |
| `mcp_servers` | object[] | no | MCP servers collected only when plugin MCP is allowed |
| `default_project_profile` | string | no | Legacy metadata only; project runtime is removed |

Schema v3 also accepts `tools_provider`, `lifecycle`, and `http_routes`:

- `tools_provider.type: "mcp_server"` is recognized as already handled by MCP runtime.
- `tools_provider.type: "go_plugin"` or `"script"` parses but is reported as unsupported; use MCP or a skill companion CLI instead.
- `lifecycle.on_start` / `on_stop` can call a non-shell built-in tool by object form, for example `{"tool":"memory_search","args":{...}}`. Legacy string shell hooks are rejected.
- `http_routes` is not an active route registration surface.

## Skill Directories

The `skills` array lists relative directories. Each directory is scanned for `SKILL.md` files and merged with bundled, user, extra, and workspace skill sources.

Example layout:

```text
plugins/release-helper/
├── tars.plugin.json
└── skills/
    └── release-check/
        ├── SKILL.md
        └── release-check.sh
```

With `"skills": ["skills"]`, `release-check` is discovered and mirrored into `workspace/_shared/skills_runtime/` for stable prompt-time reads.

## Skill Frontmatter

`SKILL.md` files use YAML frontmatter plus Markdown body content. TARS supports shared fields such as `name`, `description`, `slash`, `aliases`, and `user-invocable`, plus runtime gating fields.

```yaml
---
name: release-check
description: Validate release metadata before publishing
user-invocable: true
slash: /release-check
recommended_tools: [bash]
requires_plugin: release-helper
requires_bins: [git, gh]
requires_env: [GITHUB_TOKEN]
os: [darwin, linux]
arch: [arm64, amd64]
---
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Display name; defaults to directory name |
| `description` | string | Short summary shown in the skill prompt |
| `user-invocable` | bool | Exposes the skill through slash-command selection |
| `slash` | string | Explicit slash command name |
| `aliases` | string[] | Alternate invocation names |
| `recommended_tools` | string[] | Tools the skill expects to use, commonly `[bash]` |
| `recommended_project_files` | string[] | Project files useful for the skill |
| `requires_plugin` | string | Requires an installed plugin with this `id` |
| `requires_bins` | string[] | Requires executables on `PATH` |
| `requires_env` | string[] | Requires non-empty environment variables |
| `os` | string[] | Restricts loading to matching `GOOS` values |
| `arch` | string[] | Restricts loading to matching `GOARCH` values |

The loader resolves source priority first and then evaluates availability. If the winning definition is unavailable, TARS skips it and emits an extension diagnostic instead of silently falling back to a lower-priority copy.

## MCP Servers

MCP servers can come from three places:

1. Configured servers under `extensions.mcp.servers`.
2. Hub-installed MCP packages in `workspace/mcp-servers/`.
3. Plugin-declared `mcp_servers`, only when `extensions.plugins.allow_mcp_servers: true`.

When names collide, later groups override earlier groups in this order: config, plugin, hub. The runtime records each server's source label.

Plugin MCP declarations use the same server shape as config:

```json
{
  "id": "release-helper",
  "mcp_servers": [
    {
      "name": "release-tools",
      "transport": "stdio",
      "command": "node",
      "args": ["server.js"],
      "env": {"LOG_LEVEL": "info"}
    },
    {
      "name": "remote-release-tools",
      "transport": "streamable_http",
      "url": "https://mcp.example.com/mcp",
      "headers": {"X-Team": "release"},
      "auth_mode": "bearer",
      "auth_token_env": "MCP_RELEASE_TOKEN"
    }
  ]
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `name` | yes | Server identifier; tools are named `mcp.<server>.<tool>` |
| `transport` | no | `stdio` default, or `streamable_http`, `sse`, `websocket` |
| `command` | yes for `stdio` | Local executable |
| `args` | no | Local command arguments |
| `env` | no | Extra environment for local command |
| `url` | yes for remote | Remote MCP endpoint |
| `headers` | no | Extra HTTP/WebSocket headers |
| `auth_mode` | no | `bearer` or `oauth` for remote transports |
| `auth_token_env` | no | Env var for bearer token |
| `oauth_provider` | no | OAuth token source such as `claude-code` or `google-antigravity` |

Local stdio servers still honor `extensions.mcp.command_allowlist`; remote transports are not command-launched.

## Plugin Sources

Runtime plugin sources are merged by priority:

1. **Workspace**: `workspace/plugins/`
2. **User**: `~/.tars/plugins/` and legacy `~/.tarsncase/plugins/`
3. **Bundled**: configured `extensions.plugins.bundled_dir` (default `./plugins`)

When plugins with the same `id` exist in multiple sources, the higher-priority source wins. Within a source, `tars.plugin.json` wins over the legacy `tarsncase.plugin.json`.

## Installing From The Hub

```bash
tars skill search
tars skill install <name>
tars skill update

tars plugin search
tars plugin install <name>
tars plugin update

tars mcp search
tars mcp install <name>
tars mcp update
```

Hub-installed skills go into `workspace/skills/`, plugins into `workspace/plugins/`, and MCP packages into `workspace/mcp-servers/`. Install and update commands track state in `workspace/skillhub.json` and report updated, skipped, and failed entries separately.

### External Hubs (skill federation)

`tars skill` accepts a `--from <hub>` flag to pull skills from external repos in addition to the built-in `tars-hub`. External-hub installs always download to a preview first, generate an `ATTRIBUTION.md` alongside the skill, and refuse to materialize source-available content.

```bash
tars skill search github                         # federated search across all hubs
tars skill search --from hermes auth             # restrict to one hub
tars skill install --from openclaw github --yes  # external install (auto-approve)
tars skill install --from anthropic skill-creator --dry-run --format json
tars skill install --from anthropic docx --yes   # → blocked: proprietary
```

Built-in support today: `tars-hub`, `openclaw` (steipete/openclaw), `hermes` (NousResearch/hermes-agent), `anthropic` (anthropics/skills). The console Extensions page exposes the same flow through a hub-source dropdown plus a dry-run modal showing the converted frontmatter, per-file sha256, adapter warnings, and the ATTRIBUTION notice before any file is written. Implementation lives under `internal/skillhub/sources/<id>/`; see `docs/tutorials/14-skill-hub.md` for the architecture.

## Hub MCP Package Manifest

Standalone MCP packages use `tars.mcp.json`:

```json
{
  "schema_version": 1,
  "server": {
    "name": "safe-time",
    "command": "node",
    "args": ["${MCP_DIR}/server.js"]
  }
}
```

`${MCP_DIR}` expands to the installed package directory at runtime. Package files can be checksum-verified by the hub registry. The Extensions console can draft local MCP packages, validate `tools/list` and `tools/call`, and save them into `workspace/mcp-servers/<name>/`.

## Operational Notes

- Do not add domain features as always-registered Go tools. Tool schemas are injected into chat context and increase prompt size for every session.
- Prefer skills with companion CLIs for workflows that can be run on demand.
- Enable plugin-declared MCP servers only after reviewing the plugin source; they can launch local processes.
- Plugin HTTP routes are not registered by the runtime. Use a sidecar service with its own port if an integration needs HTTP callbacks.
- Update this guide alongside changes in `internal/plugin`, `internal/extensions`, `internal/mcp`, or `internal/skillhub`.
