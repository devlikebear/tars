---
name: openclaw-guide
description: "OpenClaw(TypeScript) 아키텍처를 TARS(Go)로 이식할 때 사용. OpenClaw 소스/문서에서 패턴을 찾아 TARS의 세션, 에이전트 루프, 도구, 스킬, 플러그인, MCP, 자동화 기능을 구현할 때 트리거."
---

# OpenClaw Guide (Codex)

OpenClaw 문서를 TARS 구현으로 변환할 때 참조 순서와 구현 규칙을 고정한다.

## 언제 쓰는가

- OpenClaw의 특정 기능을 TARS에 반영해야 할 때
- TARS의 세션/에이전트 루프/도구/스킬/플러그인/MCP/허트비트/크론잡을 구현할 때
- TS 인터페이스/런타임 패턴을 Go 규약으로 재설계할 때

## 실행 순서

1. `references/phase-map.md`에서 현재 작업의 Phase를 확인한다.
2. 해당 Phase의 OpenClaw 문서와 검색 패턴으로 범위를 좁힌다.
3. `references/automation.md`, `references/architecture.md`, `references/tools-and-extensions.md`를 순차 확인한다.
4. TS 코드를 그대로 번역하지 않고 Go로 재설계한다.
5. 최소 구현 후 기존 설계 포인트(세션, API, 플러그인, 저장소, 스케줄러)로 통합한다.

## 핵심 번역 규칙

- `class` → `struct + method`
- Promise/try-catch → `(result, error)` 반환
- 인터페이스는 최소화하고 필요한 메서드만 유지
- 콜백/이벤트는 `chan` 또는 콜백 함수로 정리
- 스트리밍은 `io.Writer`/`chan`/콜백 중 단일 흐름으로 통일
- 의존성 주입은 `struct` 필드 또는 함수 인자로 처리

## 참조 우선순위

- `references/phase-map.md` (우선 확인)
- `references/architecture.md`
- `references/tools-and-extensions.md`
- `references/automation.md`

## Codex 포팅 매핑 예시

- Agent Loop: `internal/agent/`
- 세션/컴팩션: `internal/session/`
- 도구: `internal/tool/`
- 스킬: `internal/skill/`
- Workspace/Prompt: `internal/memory/`, `internal/prompt/`
- 크론/허트비트: `internal/cron/`, `internal/heartbeat/`
- MCP/플러그인: `internal/mcp/`, `internal/plugin/`
