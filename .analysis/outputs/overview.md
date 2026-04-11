# TARS 개요

TARS는 Go로 작성된 **셀프호스트 AI 에이전트 런타임**이다. 단일 바이너리로 로컬 머신에서 실행되며, 인터랙티브 채팅 + 내구성 메모리 + 병렬 서브에이전트 + 3-tier 모델 라우팅 + 백그라운드 감시 + 스케줄링 + 멀티채널 I/O를 하나의 HTTP 서버에 묶는다.

"프로젝트 오토파일럿"은 여전히 하나의 워크플로우로 존재하지만, 시스템의 핵심 정체성은 **에이전트 런타임 플랫폼**이다.

## 핵심 구조

처음 읽을 때는 진입점을 아홉 묶음으로 잡으면 빠르다.

| 묶음 | 핵심 패키지 | 역할 |
|------|------------|------|
| CLI 진입 | `cmd/tars/` | Cobra 서브커맨드 14개 (serve, init, cron, skill, plugin, mcp 등) |
| HTTP 서버 | `internal/tarsserver/` | 60+ API 라우트, 콘솔 SPA 서빙, 세션 관리 |
| LLM 추상화 | `internal/llm/` | Provider 추상화 (Anthropic, OpenAI, Gemini) + 3-tier Router |
| 에이전트 실행 | `internal/gateway/` + `internal/agent/` | 멀티스레드 서브에이전트 런타임, tier별 모델 선택 |
| 채팅 + 메모리 | `internal/session/` + `internal/memory/` | 파일 기반 세션, 시맨틱 검색, 지식베이스, 경험 추출 |
| 도구 | `internal/tool/` | 39개 파일, 파일 I/O, exec, 메모리, 웹, 게이트웨이, MCP 브릿지 |
| 백그라운드 | `internal/pulse/` + `internal/reflection/` | 1분 감시자 + 야간 배치 (메모리 정리 + KB 컴파일) |
| 스케줄링 | `internal/cron/` | 세션 바인딩 크론 + @at 일회성 + 실행 이력 |
| 확장 | `internal/skill/` + `internal/plugin/` + `internal/mcp/` | 스킬허브, 플러그인, MCP 서버 |

## 주요 수치

| 항목 | 값 |
|------|-----|
| Go 소스 파일 | ~645 |
| 내부 패키지 | 30+ |
| CLI 서브커맨드 | 14 |
| API 라우트 | 60+ |
| 콘솔 페이지 | 8 (chat, memory, sysprompt, ops, config, extensions, pulse, reflection) |
| 백그라운드 런타임 | 3 (Pulse 1m, Reflection 5m tick, Cron 30s) |
| LLM 프로바이더 | 5 (Anthropic, OpenAI, OpenAI-Codex, Gemini, Gemini-native) |
| 모델 티어 | 3 (heavy, standard, light) |

## 기술 스택

| 레이어 | 기술 |
|--------|------|
| 언어 | Go 1.25.6 |
| CLI | Cobra |
| HTTP | net/http + gorilla/websocket |
| 프론트엔드 | Svelte 5 + Vite (go:embed) |
| LLM | Anthropic, OpenAI (Codex 포함), Gemini |
| 임베딩 | Gemini embedding API |
| 스케줄링 | robfig/cron/v3 |
| 로깅 | zerolog |
| 브라우저 | Playwright (Node.js) |
| 시크릿 | HashiCorp Vault (선택) |
| 영속성 | 파일 기반 (JSON, JSONL, YAML, Markdown) — 외부 DB 없음 |

## 다른 도구와의 차이

| | OpenClaw | Hermes Agent | TARS |
|---|---|---|---|
| 언어 | TypeScript | Python | Go (단일 바이너리) |
| 서브에이전트 | ACP + subagent, push 완료, Docker 샌드박스 | ThreadPool (max 3), 임시 프롬프트 | Gateway executor, per-task tier, allowlist 정책 |
| 모델 라우팅 | per-agent override | per-child provider/model + MoA | 3-tier 명명 번들 + role→tier config |
| 메모리 | 세션 트랜스크립트 | Honcho/Holographic 훅 | 내구성 KB + 시맨틱 검색 + 경험 추출 + 야간 컴파일 |
| 백그라운드 | 없음 | 없음 | Pulse 감시자 (1분) + Reflection 야간 배치 |
| 스케줄링 | 없음 | 없음 | 세션 바인딩 크론 + 감사 로그 |
| 채널 | CLI | CLI + Gateway API | 콘솔 + Telegram + 웹훅 |
