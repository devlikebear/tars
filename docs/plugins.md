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
2. **Bundled plugins** — `share/tars/plugins/` (shipped with the binary)
3. **User plugins** — `~/.tars/plugins/` (user-global)

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

## Skills vs Plugins

| | Skill | Plugin |
|---|---|---|
| What it is | LLM instruction file (`SKILL.md`) | Infrastructure package (`tars.plugin.json`) |
| Provides | Prompts, recommended tools, workflows | MCP servers, tools, skill directories |
| Standalone | Yes | Yes |
| Relationship | Can require a plugin via `requires_plugin` | Can bundle skills via `skills` directories |

A skill can work independently if it only needs built-in tools. When a skill requires tools provided by a plugin, its registry entry declares `requires_plugin`, and `tars skill install` warns if the dependency is missing.

## Reference Implementation

The bundled [`project-swarm`](../plugins/project-swarm) plugin is the reference:

- **Manifest**: `tars.plugin.json` with `"skills": ["skills"]`
- **Skills**: `project-start` (user-invocable kickoff) and `project-autopilot` (PM supervisor loop)
- **Runtime tools**: Project board, activity log, task dispatch, and autopilot start are built into the runtime and activated when the plugin is loaded

The plugin is also published to the [TARS Hub registry](https://github.com/devlikebear/tars-skills) for `tars plugin install`.
