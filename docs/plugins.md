# TARS Plugin Guide

A plugin is an installable package that provides infrastructure to the TARS runtime: MCP servers, tool definitions, and skill directories. Skills are LLM instruction files; plugins are the machinery that powers them.

## Manifest Schema

Every plugin is a directory containing a `tars.plugin.json` manifest:

```json
{
  "id": "project-swarm",
  "name": "Project Swarm",
  "description": "Project kickoff and autonomous execution skills.",
  "version": "0.1.0",
  "skills": ["skills"],
  "mcp_servers": []
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier, used as directory name |
| `name` | string | no | Human-readable display name |
| `description` | string | no | Short summary |
| `version` | string | no | SemVer version |
| `skills` | string[] | no | Relative paths to skill directories within the plugin |
| `mcp_servers` | object[] | no | MCP server definitions (see below) |

## Skill Directories

The `skills` array lists relative directory paths. Each path is scanned for `SKILL.md` files. These skill directories are merged into the runtime's skill loader pipeline alongside workspace-level and user-level skills.

Example layout:

```
plugins/project-swarm/
├── tars.plugin.json
└── skills/
    ├── project-start/
    │   └── SKILL.md
    └── project-autopilot/
        └── SKILL.md
```

With `"skills": ["skills"]`, both `project-start` and `project-autopilot` are discovered.

## Skill Frontmatter

`SKILL.md` files use YAML frontmatter plus Markdown body content. TARS supports the shared `name`, `description`, and `user-invocable` fields, plus runtime gating fields that control whether a skill is exposed in the current environment.

```yaml
---
name: deploy
description: Ship the current branch through GitHub Flow
user-invocable: true
requires_plugin: project-swarm
requires_bins: [git, gh]
requires_env: [GITHUB_TOKEN]
os: [darwin, linux]
arch: [arm64, amd64]
---
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | no | Display name; defaults to the directory name |
| `description` | string | no | Short summary shown in the skill prompt |
| `user-invocable` | bool | no | Exposes the skill as `/skill-name` |
| `requires_plugin` | string | no | Requires an installed plugin with the same `id` |
| `requires_bins` | string[] | no | Requires each named executable to be present on `PATH` |
| `requires_env` | string[] | no | Requires each environment variable to exist and be non-empty |
| `os` | string[] | no | Restricts loading to matching `GOOS` values |
| `arch` | string[] | no | Restricts loading to matching `GOARCH` values |

The loader resolves source priority first and then evaluates these requirements. If the winning definition is unavailable, TARS skips it and emits an extension diagnostic instead of silently falling back to a lower-priority copy.

## MCP Servers

Plugins can declare MCP servers that the runtime starts alongside the main process:

```json
{
  "id": "my-plugin",
  "mcp_servers": [
    {
      "name": "my-tools",
      "command": "node",
      "args": ["server.js"],
      "env": {"PORT": "9100"}
    }
  ]
}
```

The runtime manages the lifecycle of declared MCP servers and injects their tools into the agent loop.

## Plugin Sources

The runtime loads plugins from multiple sources, in priority order:

1. **Workspace plugins** — `workspace/plugins/` (highest priority, user-managed)
2. **User plugins** — `~/.tars/plugins/` (user-global)
3. **Bundled plugins** — `share/tars/plugins/` (shipped with the binary)

When plugins with the same `id` exist in multiple sources, the highest-priority source wins.

## Installing Plugins

### From the Hub

```bash
tars plugin search
tars plugin install project-swarm
tars plugin list
tars plugin update
```

Hub-installed plugins go into `workspace/plugins/`.

### Bundled via `tars init`

`tars init` copies bundled plugins (like `project-swarm`) from `share/tars/plugins/` into `workspace/plugins/`. `tars doctor --fix` restores missing bundled plugins.

### Manual

Drop a directory with a valid `tars.plugin.json` into any plugin source directory.

## Trusted Hub MCP Packages

TARS also supports MCP packages that are hosted directly in the trusted [`tars-skills`](https://github.com/devlikebear/tars-skills) registry instead of being embedded in a plugin.

```bash
tars mcp search
tars mcp install safe-time
tars mcp list
tars mcp update
```

These packages install into `workspace/mcp-servers/` and are tracked in `workspace/skillhub.json`.

Each MCP package contains a `tars.mcp.json` manifest:

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

`"${MCP_DIR}"` expands to the installed package directory at runtime. All declared package files are checksum-verified during install. Runtime launch still honors `mcp_command_allowlist_json`, so the command in the manifest must be explicitly allowlisted.

## Skills vs Plugins

| | Skill | Plugin |
|---|---|---|
| What it is | LLM instruction file (`SKILL.md`) | Infrastructure package (`tars.plugin.json`) |
| Provides | Prompts, recommended tools, workflows | MCP servers, tools, skill directories |
| Standalone | Yes | Yes |
| Relationship | Can require a plugin via `requires_plugin` | Can bundle skills via `skills` directories |

A skill can work independently if it only needs built-in tools. When a skill requires tools provided by a plugin, its registry entry declares `requires_plugin`, and `tars skill install` warns if the dependency is missing.

At runtime, skill availability is also gated by `requires_plugin`, `requires_bins`, `requires_env`, `os`, and `arch`, so only usable skills appear in the active prompt and manager snapshot.

## Reference Implementation

The bundled [`project-swarm`](../plugins/project-swarm) plugin is the reference:

- **Manifest**: `tars.plugin.json` with `"skills": ["skills"]`
- **Skills**: `project-start` (user-invocable kickoff) and `project-autopilot` (PM supervisor loop)
- **Runtime tools**: Project board, activity log, task dispatch, and autopilot start are built into the runtime and activated when the plugin is loaded

The plugin is also published to the [TARS Hub registry](https://github.com/devlikebear/tars-skills) for `tars plugin install`.
