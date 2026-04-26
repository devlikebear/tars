# 프로젝트 개요

TARS는 로컬에서 실행되는 self-hosted AI agent runtime이다. 하나의 Go 바이너리가 CLI, HTTP API, 웹 콘솔, 세션/메모리, agent runtime, cron, pulse/reflection 같은 백그라운드 surface를 함께 제공한다.

## 핵심 사용자 표면

- `tars serve`: API 서버와 콘솔 정적 자산을 제공한다.
- `tars`: 기본 브라우저로 `/console`을 연다.
- `tars --message "..."`
  : `/v1/chat`에 one-shot 메시지를 보내고 SSE 응답을 출력한다.
- `/console`: Chat, Memory, System Prompt, Ops, Pulse, Reflection, Extensions, Config를 다루는 주 UI다.
- `tars cron`, `tars approve`, `tars skill|plugin|mcp`: 서버 API를 호출하는 얇은 CLI wrapper다.

## 현재 상태 요약

2026-04-25~26 코드리뷰 기준으로 큰 레거시 축은 대부분 정리됐다.

- KB Wiki user-facing surface는 제거 완료됐지만, bootstrap/routes/workspace template 잔여물은 #410에서 추적한다.
- `gateway` 패키지명은 `agentruntime`으로 hard cut 됐지만, `/v1/agent/*` 호환 route와 client/docs 정리는 #414에서 추적한다.
- `internal/project`, macOS dashboard, Autopilot, Bubble Tea TUI는 현재 활성 기능이 아니다.
- 현재 운영 자동화는 cron, ops approval, event stream, pulse/reflection 중심이다.

## 저장 구조

- `workspace/sessions/`: main/worker session index와 transcript.
- `workspace/memory/`: `MEMORY.md`, daily logs, experiences, semantic index 원천.
- `workspace/_shared/agentruntime/`: agent runtime runs/channels/notifications persistence.
- `workspace/ops/`: cleanup approvals와 ops event state.
- `workspace/config/`: 로컬 설정 override.

## 최근 문서 최신화 포인트

- README/GETTING_STARTED에서 KB Wiki 설명을 durable memory + semantic search + experiences 중심으로 정리했다.
- CLAUDE.md에서 실제 CLI subcommand, reflection job 의미, chat memory hook 범위를 현재 코드와 맞췄다.
- docs/tutorials에서 제거된 project/dashboard/autopilot/TUI 흐름을 ops approval/event stream/console/agent runtime 흐름으로 교체했다.
