# OpenClaw Core Architecture Reference

> Source: `https://github.com/openclaw/openclaw`
> Language: TypeScript (concepts only — TARS implements in Go)

## Table of Contents

- [Agent Loop](#agent-loop)
- [Session Management](#session-management)
- [System Prompt Assembly](#system-prompt-assembly)
- [Memory System](#memory-system)
- [Context Compaction](#context-compaction)
- [Workspace & Bootstrap Files](#workspace--bootstrap-files)
- [Pi Integration Architecture](#pi-integration-architecture)

---

## Agent Loop

**OpenClaw doc**: `docs/concepts/agent-loop.md`

Lifecycle:
1. **Intake** — user message received
2. **Context assembly** — system prompt + tools + skills + session history
3. **Model inference** — LLM call with messages array + tool definitions
4. **Tool execution** — if response contains `tool_calls`, execute each tool, append `tool` role messages
5. **Loop** — re-invoke LLM with tool results, repeat until no more `tool_calls` or max iterations
6. **Streaming** — SSE deltas for text + tool start/result events
7. **Persistence** — append assistant message + tool messages to session transcript

**Key design**:
- Max iteration limit prevents infinite loops
- Each tool execution is sandboxed with timeout
- Tool results are added as `tool` role messages with `tool_call_id` matching the request
- SSE event types: `delta` (text), `tool_start`, `tool_result`, `done`

**TARS mapping**: `internal/agent/loop.go`

---

## Session Management

**OpenClaw docs**: `docs/concepts/session.md`, `docs/reference/session-management-compaction.md`

### Storage Structure
```
{workspace}/sessions/
  sessions.json          # sessionKey → {sessionId, updatedAt, title, ...}
  {sessionId}.jsonl      # append-only transcript
```

### Session Lifecycle
- **Create**: generate sessionId (UUID), add entry to sessions.json
- **Resume**: load session by sessionId, replay transcript
- **Switch**: change active sessionId in runtime state
- **Delete**: remove from sessions.json, optionally delete JSONL
- **Export**: render session transcript as markdown
- **Search**: keyword search across session transcripts

### Transcript (JSONL) Format
Each line is a JSON object:
```json
{"role":"user","content":"Hello","ts":"2026-02-14T10:00:00Z"}
{"role":"assistant","content":"Hi!","ts":"2026-02-14T10:00:01Z","usage":{"input_tokens":50,"output_tokens":10}}
{"role":"assistant","content":"","tool_calls":[{"id":"call_1","name":"exec","arguments":"{\"command\":\"ls\"}"}],"ts":"..."}
{"role":"tool","tool_call_id":"call_1","content":"file1.txt\nfile2.txt","ts":"..."}
{"type":"compaction","summary":"User asked about files...","ts":"..."}
```

### Token-Based Dynamic Loading
- Load JSONL in reverse order
- Estimate tokens: `len(content) / 4` heuristic
- Stop when cumulative tokens exceed `contextWindow - reserveTokens`
- Default: 128K context, 4096 reserve
- If a `compaction` entry exists, load its summary + messages after it

**TARS mapping**: `internal/session/`

---

## System Prompt Assembly

**OpenClaw doc**: `docs/concepts/system-prompt.md`

### Prompt Sections (in order)
1. **Base role definition** — agent identity, safety guidelines
2. **Bootstrap files injection** — workspace files content:
   - `AGENTS.md` — agent operating guidelines
   - `SOUL.md` — persona, tone, boundaries
   - `USER.md` — user profile
   - `IDENTITY.md` — agent name, personality
   - `TOOLS.md` — user environment tool notes
   - `HEARTBEAT.md` — heartbeat checklist
   - `BOOTSTRAP.md` — one-time first-run ritual (brand-new workspace only, deleted after completion)
   - `MEMORY.md` and/or `memory.md` — curated long-term memory (when present)
3. **Runtime info** — current time (UTC + user timezone), reply tags, heartbeat behavior, host/OS/model
4. **Tool definitions** — registered tools list (JSON Schema)
5. **Skill definitions** — loaded skills as XML block

> **Note**: `memory/*.md` daily log files are NOT auto-injected into the system prompt.
> They are available on-demand via memory tools (`memory_search`, `memory_get`).
> Sub-agent sessions only inject `AGENTS.md` + `TOOLS.md` (not other bootstrap files).

### File Size Limit
- Each bootstrap file: max 20000 chars (`bootstrapMaxChars`)
- Truncate with `[truncated]` marker if exceeded

**TARS mapping**: `internal/prompt/builder.go`

---

## Memory System

**OpenClaw doc**: `docs/concepts/memory.md`

### 3-Layer Structure
| Layer | File | Purpose | Update Pattern |
|-------|------|---------|---------------|
| Curated | `MEMORY.md` | Long-term important facts | AI writes during compaction or explicit save |
| Daily | `memory/YYYY-MM-DD.md` | Daily activity log | Append on each heartbeat/interaction |
| Shared | `_shared/` | Cross-agent shared files | Manual or tool-based |

### Memory Operations
- **memory_search**: keyword search across daily logs + MEMORY.md
- **memory_get**: read specific date's daily log
- **Auto-flush**: before compaction, AI saves important context to MEMORY.md and daily log (silent turn)

### Pre-Compaction Memory Flush
Before compacting old messages, the system injects a silent turn asking the AI to save any important information from the context being compressed into MEMORY.md or daily logs. This prevents information loss.

**TARS mapping**: `internal/memory/`

---

## Context Compaction

**OpenClaw doc**: `docs/concepts/compaction.md`

### Trigger Conditions
- **Auto**: estimated tokens exceed `contextWindow - reserveTokensFloor`
- **Manual**: user invokes `/compact` command

### Compaction Process
1. **Pre-compaction flush**: silent turn → AI saves important info to memory
2. **Summarize**: send old messages to LLM with summarization prompt
3. **Store**: append `{"type":"compaction","summary":"..."}` to JSONL
4. **Reload**: next session load uses compaction summary + subsequent messages only

### Session Loading with Compaction
```
[system prompt]
[compaction summary]         ← replaces all messages before compaction point
[messages after compaction]  ← loaded in full
[new user message]
```

**TARS mapping**: `internal/session/compaction.go`

---

## Workspace & Bootstrap Files

**OpenClaw doc**: `docs/concepts/agent-workspace.md`, `docs/reference/templates/`

### Workspace File Map
```
{workspace}/
├── AGENTS.md       # Agent operating guidelines, memory usage rules
├── SOUL.md         # Persona, tone, boundaries, voice
├── USER.md         # User profile (name, preferences, context)
├── IDENTITY.md     # Agent name, personality traits
├── TOOLS.md        # User environment tool notes
├── HEARTBEAT.md    # Heartbeat checklist (natural language)
├── BOOTSTRAP.md    # One-time first-run ritual (deleted after completion)
├── MEMORY.md       # Curated long-term memory (auto-injected when present)
├── memory.md       # Alternative memory file (auto-injected when present)
├── _shared/        # Cross-agent shared files
├── memory/         # Daily logs (YYYY-MM-DD.md) — on-demand via memory tools
├── skills/         # Workspace-specific skills (highest priority)
├── sessions/       # Session transcripts
│   ├── sessions.json
│   └── {id}.jsonl
└── cron/           # Cron job definitions
    └── jobs.json
```

> `MEMORY.md` is optional (not auto-created); when present, it is loaded for normal sessions.
> `BOOTSTRAP.md` is only created for brand-new workspaces, deleted after ritual completion.

### Bootstrap File Templates
Templates from `docs/reference/templates/`:
- **SOUL.md**: defines persona voice, communication style, boundaries
- **USER.md**: user profile template (dev mode: USER.dev.md)
- **IDENTITY.md**: agent identity template (dev mode: IDENTITY.dev.md)
- **AGENTS.md**: agent operating guidelines, how to use memory
- **TOOLS.md**: tool environment notes
- **HEARTBEAT.md**: checklist items for periodic execution

**TARS mapping**: `internal/memory/workspace.go` (extend `EnsureWorkspace`)

---

## Pi Integration Architecture

**OpenClaw doc**: `docs/concepts/pi-integration.md`

### Core Components
- **pi-agent-core**: agent loop engine (model inference, tool dispatch, streaming)
- **pi-coding-agent**: higher-level wrapper
  - `SessionManager`: creates/manages agent sessions
  - `createAgentSession()`: assembles context (system prompt + tools + history)
  - `AgentSession.sendMessage()`: entry point for agent loop

### Tool Architecture
- Tools defined with JSON Schema parameters
- Registry pattern: tools registered at agent initialization
- Each tool has `name`, `description`, `parameters` (JSON Schema), `execute` function
- Tool results are `ContentBlock[]` (text, image, etc.)

**TARS mapping**: `internal/tool/`, `internal/agent/`
