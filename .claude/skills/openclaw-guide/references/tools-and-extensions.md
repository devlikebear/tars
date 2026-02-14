# OpenClaw Tools, Skills, Plugins & MCP Reference

> Source: `https://github.com/openclaw/openclaw`

## Table of Contents

- [Tool Surface](#tool-surface)
- [Built-in Tools](#built-in-tools)
- [Skills System](#skills-system)
- [Plugin System](#plugin-system)
- [MCP Integration](#mcp-integration)
- [Slash Commands](#slash-commands)

---

## Tool Surface

**OpenClaw doc**: `docs/tools/index.md`

### Tool Interface
```typescript
interface Tool {
  name: string;
  description: string;
  parameters: JSONSchema;  // JSON Schema for input validation
  execute(ctx: Context, params: object): Promise<Result>;
}

interface Result {
  content: ContentBlock[];
  isError?: boolean;
}

interface ContentBlock {
  type: "text" | "image";
  text?: string;
  data?: string;  // base64 for images
}
```

### Allow/Deny Profiles
- Tools can be grouped into profiles (e.g., "safe", "full", "readonly")
- Allow/deny lists control which tools are available per session
- Default profile includes all built-in tools

### Tool Registration
- Tools registered in a `Registry` at agent initialization
- Registry provides `Get(name)`, `All()`, `Schemas()` methods
- `Schemas()` returns JSON Schema array for LLM tool definitions

**TARS mapping**: `internal/tool/tool.go`, `internal/tool/registry.go`

---

## Built-in Tools

**OpenClaw docs**: `docs/tools/exec.md`, `docs/tools/web.md`, `docs/tools/browser.md`

### exec (Shell Execution)
- **Parameters**: `command` (string), `timeout` (number, ms), `background` (bool)
- **Safety**: configurable allow/deny command patterns
- **Background mode**: returns immediately, process runs in background
- **Output**: stdout + stderr combined, truncated at max length

### read_file
- **Parameters**: `path` (string), `offset` (number), `limit` (number)
- **Features**: line range support, binary file detection
- **Output**: file content with line numbers

### write_file
- **Parameters**: `path` (string), `content` (string)
- **Features**: creates parent directories, preserves permissions
- **Output**: success confirmation

### edit_file
- **Parameters**: `path` (string), `old_text` (string), `new_text` (string)
- **Features**: exact string replacement, uniqueness check
- **Output**: success confirmation with line numbers

### web_search (Brave/Perplexity)
- **Parameters**: `query` (string), `count` (number)
- **Provider**: Brave Search API or Perplexity
- **Output**: search results with title, URL, snippet

### web_fetch
- **Parameters**: `url` (string)
- **Features**: HTML → text conversion, size limit
- **Output**: extracted text content

### memory_search
- **Parameters**: `query` (string)
- **Scope**: daily logs + MEMORY.md
- **Output**: matching entries with dates

### memory_get
- **Parameters**: `date` (string, YYYY-MM-DD)
- **Output**: daily log content for specified date

### session_status
- **Parameters**: none
- **Output**: current session info (token count, message count, session age)

### Browser Tools
- **Doc**: `docs/tools/browser.md`
- **Features**: managed Chrome instance, page navigation, click, type, screenshot
- **Profile**: separate browser profile per agent
- **Actions**: navigate, click, type, screenshot, evaluate JS

**TARS mapping**: `internal/tool/` — each tool in separate file (exec.go, readfile.go, etc.)

---

## Skills System

**OpenClaw docs**: `docs/tools/skills.md`, `docs/tools/creating-skills.md`, `docs/tools/skills-config.md`

### SKILL.md Format
```markdown
---
name: weather
description: Weather information retrieval
user-invocable: true
---
# Weather Skill
[Skill usage instructions...]
```

### Load Locations (priority order)
1. `{workspace}/skills/` — highest priority (workspace-specific)
2. `~/.tarsncase/skills/` — user global
3. Bundled skills — built-in (lowest priority)

### System Prompt Injection
Loaded skills are injected as XML:
```xml
<skills>
  <skill><name>weather</name><description>Weather retrieval</description></skill>
  <skill><name>code-review</name><description>Code review assistant</description></skill>
</skills>
```

### Slash Command Integration
- `user-invocable: true` → skill available as `/{skill-name}` command
- Slash command triggers skill content injection into chat context
- Skill body (markdown instructions) loaded only when triggered

**TARS mapping**: `internal/skill/`

---

## Plugin System

**OpenClaw docs**: `docs/tools/plugin.md`, `docs/plugins/agent-tools.md`, `docs/plugins/manifest.md`

### Plugin Structure
```
plugins/my-plugin/
  tarsncase.plugin.json   # manifest
  main                    # executable binary
  skills/
    my-skill/SKILL.md     # plugin-bundled skills
```

### Manifest Schema
```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Plugin description",
  "tools": ["my_tool_1", "my_tool_2"],
  "skills": ["my-skill"]
}
```

### Plugin Runtime
- Subprocess execution via stdin/stdout JSON-RPC
- Tools registered into main Registry at startup
- Skills loaded from plugin's `skills/` directory
- Plugin discovery: scan configured plugin directories

### Tool Registration Pattern
```typescript
// OpenClaw pattern (TypeScript)
registerTool({
  name: "my_tool",
  description: "Does something",
  parameters: { type: "object", properties: { ... } },
  execute: async (ctx, params) => { ... }
});
```

**TARS mapping**: `internal/plugin/`

---

## MCP Integration

**OpenClaw docs**: implied from plugin architecture + MCP spec

### MCP Client
- Connects to MCP servers via stdio or SSE transport
- JSON-RPC 2.0 protocol
- `tools/list` → discover available tools
- `tools/call` → execute a tool

### Configuration
```yaml
mcp:
  servers:
    - name: filesystem
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    - name: brave-search
      command: npx
      args: ["-y", "@anthropic/brave-search-mcp"]
      env:
        BRAVE_API_KEY: "${BRAVE_API_KEY}"
```

### Tool Adapter
- MCP tools converted to internal `Tool` interface
- Name prefixed with MCP server name: `mcp__{server}__{tool}`
- Parameters schema passed through from MCP
- Execution delegates to MCP server via `tools/call` RPC

**TARS mapping**: `internal/mcp/`

---

## Slash Commands

**OpenClaw doc**: `docs/tools/slash-commands.md`

### Built-in Commands
| Command | API Endpoint | Description |
|---------|-------------|-------------|
| `/new` | `POST /v1/sessions` | New session |
| `/sessions` | `GET /v1/sessions` | List sessions |
| `/resume {id}` | session switch | Resume session |
| `/history` | `GET /v1/sessions/{id}/history` | Current session history |
| `/export` | `POST /v1/sessions/{id}/export` | Export session as markdown |
| `/search {q}` | `GET /v1/sessions/search?q=` | Search sessions |
| `/status` | `GET /v1/status` | tarsd status |
| `/compact` | `POST /v1/compact` | Trigger compaction |
| `/skills` | `GET /v1/skills` | List skills |
| `/cron list` | `GET /v1/cron/jobs` | List cron jobs |
| `/cron add` | `POST /v1/cron/jobs` | Add cron job |
| `/cron run {id}` | `POST /v1/cron/jobs/{id}/run` | Run cron job |
| `/plugins` | `GET /v1/plugins` | List plugins |
| `/mcp` | `GET /v1/mcp/servers` | MCP server status |

### Command vs Directive
- **Command**: `/{name}` — triggers API call, returns result
- **Directive**: inline shorthand in chat (e.g., `@file.txt`) — context injection

**TARS mapping**: `cmd/tars/` — REPL command parser
