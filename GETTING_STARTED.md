# Getting Started

Minimum path to run TARS and verify the main features.

## 1. Install

**Homebrew (recommended):**

```bash
brew tap devlikebear/tap
brew install devlikebear/tap/tars
```

**From source:**

```bash
make build-bins
export PATH="$PWD/bin:$PATH"
```

## 2. Initialize

```bash
tars init
```

This creates `~/.tars/` with default config and workspace directories.

## 3. Configure LLM Provider

Edit `~/.tars/config/config.yaml` or set environment variables:

**Anthropic (default):**

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

**OpenAI:**

```yaml
llm:
  providers:
    default:
      kind: openai
      auth_mode: api-key
      base_url: https://api.openai.com/v1
      api_key: ${OPENAI_API_KEY}
  tiers:
    heavy: { provider: default, model: gpt-4o, reasoning_effort: high }
    standard: { provider: default, model: gpt-4o, reasoning_effort: medium }
    light: { provider: default, model: gpt-4o-mini, reasoning_effort: minimal }
  default_tier: standard
```

```bash
export OPENAI_API_KEY="sk-..."
```

**Claude Code CLI (no API key needed):**

```yaml
llm:
  providers:
    default:
      kind: claude-code-cli
  tiers:
    heavy: { provider: default, model: sonnet }
    standard: { provider: default, model: sonnet }
    light: { provider: default, model: haiku }
  default_tier: standard
```

Requires Claude Code CLI installed locally.

**Optional — 3-tier model routing:**

```yaml
llm:
  default_tier: standard
  role_defaults:
    pulse_decider: light
    gateway_planner: heavy
```

## 4. Validate

```bash
tars doctor --fix
```

Checks provider credentials, workspace structure, and optional dependencies.

## 5. Start the Server

```bash
tars serve
```

Or with a specific config file:

```bash
tars serve --config ~/.tars/config/config.yaml
```

Or as a macOS background service:

```bash
tars service install && tars service start
```

## 6. Open the Console

```bash
tars
```

Or navigate to `http://127.0.0.1:43180/console` in your browser.

### First things to try

- **Chat** — Type a message and watch the agent loop execute tools
- **Memory** — Visit `/console/memory` to see durable memory and knowledge base
- **System Prompt** — Visit `/console/sysprompt` to customize agent identity and rules
- **Pulse** — Visit `/console/pulse` to see the background watchdog status

## 7. Set Up Agents (Optional)

Create agent definitions in `workspace/agents/`:

```bash
mkdir -p workspace/agents/explorer
```

```yaml
# workspace/agents/explorer/AGENT.md
---
name: explorer
tier: light
tools_allow: [read_file, list_dir, glob, memory_search]
---
Read-only codebase explorer for fast parallel searches.
```

Use agents via chat:

- `subagents_run` for independent parallel tasks.
- `subagents_plan` to ask the heavy-tier gateway planner model for a staged execution flow.
- `subagents_orchestrate` for staged flows where later tasks depend on earlier subagent output.

Task tier precedence is `task tier` > `AGENT.md tier` > gateway default role tier.

## 8. Set Up Cron Jobs (Optional)

Create scheduled jobs from chat or the Cron tab in the console:

```
매일 아침 9시에 프로젝트 상태 요약해줘
```

Use `tars cron list`, `tars cron run <job-id>`, and the Ops/Chat surfaces to inspect or trigger existing jobs.

## 9. Connect Telegram (Optional)

```yaml
channels:
  telegram:
    enabled: true
    dm_policy: pairing
    bot_token: "your-bot-token"
```

Pair your Telegram account via the `/pair` command in your bot chat.

## 10. Install Extensions (Optional)

```bash
# Browse available skills and plugins
tars skill search
tars plugin search

# Install from the hub
tars mcp install safe-time
tars plugin install code-workflow
```

## Development Setup

For live frontend development with hot-reload:

```bash
make dev-console    # Vite dev server + Go API, open http://127.0.0.1:43180/console
```

Run tests:

```bash
make test           # go test ./...
make security-scan  # gitleaks + secret scan
```

## Troubleshooting

**Server won't start:**

```bash
tars doctor --fix   # Check and repair common issues
```

**LLM calls fail:**

- Check credentials: `echo $ANTHROPIC_API_KEY`
- Check provider config: `grep -n "llm:" ~/.tars/config/config.yaml`
- Check logs: `tail -f logs/tars.log`

**Console shows blank page:**

Build the embedded frontend first:

```bash
make console-build
```
