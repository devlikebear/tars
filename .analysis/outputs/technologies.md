# 사용 기술

## Backend

- Go 1.25+
- Cobra CLI
- `net/http` ServeMux
- SSE over HTTP
- JSONL transcript/event persistence
- YAML config + env override
- zerolog logging

## LLM Providers

- Anthropic
- OpenAI compatible
- OpenAI Codex OAuth/compat path
- Gemini / Gemini native
- Claude Code CLI

3-tier routing은 `heavy`, `standard`, `light` bundle과 role default mapping으로 동작한다.

## Frontend

- Svelte 5
- TypeScript
- Vite dev proxy
- Embedded production assets via Go `embed`
- Console design system source: `frontend/console/DESIGN.md`

## Extension Runtime

- Markdown skill frontmatter
- Plugin manifest
- MCP stdio/remote transports
- Skill Hub registry and install/update APIs

## Operations

- macOS LaunchAgent service helper
- cron expressions and one-shot `@at`
- pulse autofix whitelist
- ops cleanup approval
- GitHub Actions CI/security/release workflows
