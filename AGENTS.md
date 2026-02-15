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

## 코딩 가이드라인 (상세)

상세한 가이드라인은 `CLAUDE.md`를 참조. 핵심 원칙:

### 1. 구현 전 사고 (Think Before Coding)

- 가정을 명시적으로 표현한다. 불확실하면 질문한다.
- 여러 해석이 가능하면 모두 제시하고, 임의로 선택하지 않는다.
- 더 간단한 방법이 있으면 제안한다.
- 불명확한 부분이 있으면 멈추고 질문한다.

### 2. 단순함 우선 (Simplicity First)

- 문제를 해결하는 최소한의 코드만 작성한다.
- 요청되지 않은 기능, 추상화, 유연성, 설정 가능성을 추가하지 않는다.
- 일회용 코드에 추상화를 만들지 않는다.
- 불가능한 시나리오에 대한 에러 처리를 하지 않는다.
- 200줄을 50줄로 줄일 수 있으면 다시 작성한다.

### 3. 외과적 변경 (Surgical Changes)

- 반드시 필요한 부분만 수정한다. 본인이 만든 문제만 정리한다.
- 인접 코드, 주석, 포맷팅을 "개선"하지 않는다.
- 기존 스타일을 유지한다.
- 무관한 dead code를 발견하면 언급만 하고 삭제하지 않는다.
- 본인의 변경으로 생긴 미사용 import/변수/함수만 제거한다.
- 모든 변경 라인이 사용자 요청과 직접 연결되어야 한다.

### 4. 목표 주도 실행 (Goal-Driven Execution)

- 검증 가능한 성공 기준을 정의한다.
- 작업을 검증 가능한 목표로 변환한다:
  - "검증 추가" → "잘못된 입력에 대한 테스트 작성 후 통과시키기"
  - "버그 수정" → "재현 테스트 작성 후 통과시키기"
  - "리팩터링 X" → "리팩터링 전후 테스트 통과 확인"
- 다단계 작업은 간단한 계획을 먼저 작성한다:
  1. [단계] → 검증: [확인사항]
  2. [단계] → 검증: [확인사항]
  3. [단계] → 검증: [확인사항]

## 현재 구현 상태 (2026-02-14 기준)

- 서버 측 채팅 API `POST /v1/chat`는 구현되어 있다.
- 세션 관리 API(`GET/POST/DELETE /v1/sessions`, history/export/search)와 상태 API(`GET /v1/status`)가 구현되어 있다.
- LLM Chat 인터페이스(`Client.Chat`)와 스트리밍 콜백(`OnDelta`)이 구현되어 있다.
- 워크스페이스 부트스트랩 파일(AGENTS/SOUL/USER/IDENTITY/TOOLS/HEARTBEAT/MEMORY) 생성과 시스템 프롬프트 조립이 구현되어 있다.

## LLM Provider 운영 정책 (2026-02-15)

- `codex-cli` provider는 제거되었다. `LLM_PROVIDER=codex-cli`는 더 이상 지원하지 않는다.
- `openai-codex` provider는 제거되었다. `LLM_PROVIDER=openai-codex`는 더 이상 지원하지 않는다.
- 현재 지원 provider: `bifrost`, `openai`, `anthropic`, `gemini`, `gemini-native`

권장 설정:
- 안정 운영: `LLM_PROVIDER=openai`, `LLM_AUTH_MODE=api-key`, `OPENAI_API_KEY` 사용
- 대체 운영: `LLM_PROVIDER=anthropic`, `LLM_AUTH_MODE=api-key`, `ANTHROPIC_API_KEY` 사용
- gemini 운영: `LLM_PROVIDER=gemini`, `LLM_AUTH_MODE=api-key`, `GEMINI_API_KEY` 사용
- gemini-native 운영: `LLM_PROVIDER=gemini-native`, `LLM_AUTH_MODE=api-key`, `GEMINI_API_KEY` 사용

## 다음 우선순위

1. `tars` CLI 채팅 UX를 완성한다.
2. `/compact` 실제 동작(요약 저장 + 로딩 경계)을 구현한다.
3. 세션 전환/조회 슬래시 명령(`/new`, `/sessions`, `/resume`, `/history`, `/export`)을 `tars chat`에 연결한다.

## 작업 체크리스트

- 변경 전 현재 구현 상태와 범위를 먼저 요약한다.
- 코드 변경 시 테스트를 함께 추가한다.
- 기능 단위 완료 후 `CLAUDE.md`의 코드 구조 변경 기록을 갱신한다.
