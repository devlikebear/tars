# 튜토리얼 읽기 순서

## 1. 최소 채팅 런타임

1. `docs/tutorials/01-thin-cli.md`
2. `docs/tutorials/02-session-transcript.md`
3. `docs/tutorials/03-prompt-tool-registry.md`
4. `docs/tutorials/04-http-sse-chat.md`

이 구간은 CLI 진입점, session/transcript, prompt/tool registry, SSE chat을 묶어 최소 동작 서버를 만든다.

## 2. 실제 LLM과 도구

1. `docs/tutorials/05-openai-provider.md`
2. `docs/tutorials/06-config-system.md`
3. `docs/tutorials/07-practical-tools.md`
4. `docs/tutorials/08-anthropic-provider.md`

이 구간은 provider adapter, config/env override, filesystem/exec tool, Anthropic adapter를 추가한다.

## 3. Memory와 확장

1. `docs/tutorials/09-transcript-compaction.md`
2. `docs/tutorials/10-file-memory.md`
3. `docs/tutorials/11-semantic-memory.md`
4. `docs/tutorials/12-skill-loader.md`
5. `docs/tutorials/13-plugin-mcp.md`
6. `docs/tutorials/14-skill-hub.md`

이 구간은 긴 대화 관리와 on-demand extension boundary를 익히는 구간이다.

## 4. 최신 운영 표면

1. `docs/tutorials/15-terminal-chat.md`
2. `docs/tutorials/19-human-in-the-loop.md`
3. `docs/tutorials/20-sse-realtime.md`
4. `docs/tutorials/21-auth-middleware.md`
5. `docs/tutorials/22-agentruntime.md`

이 구간은 레거시 TUI/project/autopilot 대신 현재 활성 표면인 console, one-shot CLI, ops approval, runtime event stream, auth, agent runtime을 다룬다.
