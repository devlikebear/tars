# AGENTS.md

## 목적
- 이 저장소에서 Codex/에이전트가 일관된 방식으로 개발을 진행하도록 운영 기준을 정의한다.
- 현재 목표는 Phase 1의 대화형 LLM 채팅을 완성하는 것이다.

## 개발 원칙
- MVP 중심으로 작은 단위로 구현한다.
- TDD를 따른다. 실패 테스트를 먼저 추가하고 구현으로 통과시킨다.
- 오버엔지니어링을 피하고 지금 필요한 코드만 작성한다.
- 서버 책임 로직은 `tarsd`, 클라이언트 UX는 `tars`에 둔다.
- OpenClaw는 개념/패턴 참고용으로만 사용하고 Go 관용구로 독립 구현한다.

## 현재 구현 상태 (2026-02-14 기준)
- 서버 측 채팅 API `POST /v1/chat`는 구현되어 있다.
- 세션 관리 API(`GET/POST/DELETE /v1/sessions`, history/export/search)와 상태 API(`GET /v1/status`)가 구현되어 있다.
- LLM Chat 인터페이스(`Client.Chat`)와 스트리밍 콜백(`OnDelta`)이 구현되어 있다.
- 워크스페이스 부트스트랩 파일(AGENTS/SOUL/USER/IDENTITY/TOOLS/HEARTBEAT/MEMORY) 생성과 시스템 프롬프트 조립이 구현되어 있다.

## 다음 우선순위
1. `tars` CLI 채팅 UX를 완성한다.
2. `/compact` 실제 동작(요약 저장 + 로딩 경계)을 구현한다.
3. 세션 전환/조회 슬래시 명령(`/new`, `/sessions`, `/resume`, `/history`, `/export`)을 `tars chat`에 연결한다.

## 작업 체크리스트
- 변경 전 현재 구현 상태와 범위를 먼저 요약한다.
- 코드 변경 시 테스트를 함께 추가한다.
- 기능 단위 완료 후 `CLAUDE.md`의 코드 구조 변경 기록을 갱신한다.
