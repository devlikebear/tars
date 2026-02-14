# CLAUDE.md

## 개발 규칙
1. MVP 중심으로 실행 가능한 단위로 개발한다.
2. TDD를 따른다. 실패하는 테스트를 먼저 작성한 뒤 구현한다.
3. 기능 단위마다 다음 사이클을 반복한다: 계획 수립 -> 개발 -> 버전업 -> 문서화 -> 기능 단위 커밋 -> 다음 개발.
4. 불필요한 문서 파일을 생성하지 않는다.
5. 오버엔지니어링 없이 최소한의 간결한 코드로 구현한다.
6. 미래에 사용할 가능성만으로 지금 쓰지 않을 코드를 미리 만들지 않는다.
7. 실제 필요가 생기기 전에는 인터페이스를 과도하게 설계하지 않는다.
8. 개발이 끝난 뒤 문서화 단계에서 이번 턴에 변경되었거나 새로 추가된 개발 규칙, 또는 코드 구조 변경 사항이 있으면 `CLAUDE.md`를 반드시 갱신하고 함께 커밋한다.
9. `~/workspace/opensources/openclaw`의 주요 기능을 개발할 때는 OpenClaw 소스를 분석해 참고한다.
10. `~/workspace/opensources/openclaw` 디렉터리에 접근할 수 없으면 `https://github.com/openclaw/openclaw`를 분석 대상으로 사용하고, 소스 분석은 Repomix MCP를 사용해 진행한다.

## 워크플로우
1. 사용자가 요구사항을 Claude Code에 전달
2. `/plan` — 요구사항을 작업지시서(Work Order)로 분해
3. `/implement` — Work Order를 `codex-implementer` 서브에이전트에 위임하여 코드 구현, Claude Code가 결과 검수
4. `/review` — 변경사항의 품질/보안/회귀 위험 점검, 문제 발견 시 수정 Work Order를 생성해 `/implement`로 재위임
5. 테스트 실행(`make test`)은 Claude Code가 직접 수행
6. 커밋 및 `CLAUDE.md` 갱신은 Claude Code가 수행

## 바이너리 역할 정의
- `tarsd` (메인 데몬/서버): LLM 호출, 허트비트/크론 실행, 3-Layer 메모리 처리, 작업 판단과 실행 오케스트레이션을 담당한다.
- `cased` (감시 데몬): `tarsd` 프로세스 감시, 자동 재시작/복구, 감사/보안 모니터링, 업데이트/롤백 같은 안정성 제어를 담당한다.
- `tars` (CLI 클라이언트): 사용자 입력과 명령 전달을 담당하며, 핵심 실행 로직(LLM 실행, 허트비트 루프, 데몬 오케스트레이션)은 직접 수행하지 않는다.

## tars-tarsd 통신 프로토콜 규칙
- `tarsd`가 HTTP API 서버를 서빙하고, `tars`는 해당 API를 호출하는 HTTP 클라이언트로 구현한다.
- LLM 실행, OAuth 토큰 교환/저장, heartbeat/cron 실행 같은 서버 책임 로직은 반드시 `tarsd`에서 수행한다.
- `tars`는 사용자 입력 수집, API 요청/응답 렌더링, 브라우저 열기 같은 클라이언트 UX만 담당한다.
- LLM 응답은 `tarsd`의 REST API로 제공하고, `tars`는 해당 API의 클라이언트로 구현한다.
- 인증 토큰(특히 OAuth access/refresh token)은 서버(`tarsd`)에서만 저장/관리하고, `tars`는 직접 저장하지 않는다.

## 코드 구조 변경 기록
- 2026-02-13: 런타임 설정 로더를 추가했다. `internal/config`는 기본값/파일/환경변수 우선순위를 처리하고, YAML 값에서 `${ENV_VAR}` 구문을 자동 확장한다. 샘플 설정 파일 `config/standalone.yaml`을 추가했다.
- 2026-02-13: 실행 바이너리 이름을 역할별로 분리했다. 메인 데몬은 `cmd/tarsd`, 감시 데몬은 `cmd/cased`, CLI 클라이언트는 `cmd/tars`를 사용한다.
- 2026-02-13: 개발 자동화를 위해 루트 `Makefile`을 추가하고 `.github/workflows/ci.yml`에서 `make test`를 실행하는 최소 CI를 도입했다.
- 2026-02-13: `cmd/tarsd`에 `zerolog` 기반 구조화 로그를 도입했다. 시작 성공은 info 로그로 기록하고, 설정 초기화 실패는 error 로그로 기록한다.
- 2026-02-13: `internal/memory`를 추가해 워크스페이스(`HEARTBEAT.md`, `MEMORY.md`, `_shared/`, `memory/`)를 초기화하고 `memory/YYYY-MM-DD.md`에 Daily Log를 append하도록 `tarsd` 시작 플로우를 확장했다.
- 2026-02-13: `internal/heartbeat`를 추가하고 `tarsd --run-once` 플래그를 도입했다. run-once 실행 시 `HEARTBEAT.md`를 읽어 `memory/YYYY-MM-DD.md`에 heartbeat 이벤트를 append한다.
- 2026-02-13: `cmd/tarsd`의 CLI 파서를 `cobra`로 구현했다. `--config`, `--mode`, `--workspace-dir`, `--run-once`, `--run-loop` 플래그를 지원하고 `--help` 정상 종료를 지원한다. `--run-once`와 `--run-loop`는 상호 배타적이다.
- 2026-02-13: 주기 실행용 heartbeat 루프를 추가했다. `tarsd`에 `--run-loop`, `--heartbeat-interval`, `--max-heartbeats` 플래그를 도입했고, `internal/heartbeat.RunLoop`가 지정 주기로 `HEARTBEAT.md` 기반 로그를 append한다.
- 2026-02-13: 워크스페이스 부트스트랩을 고도화했다. `EnsureWorkspace`가 `HEARTBEAT.md`/`MEMORY.md` 기본 템플릿을 생성하고, 기존 사용자 파일이 있으면 내용을 덮어쓰지 않는다.
- 2026-02-13: heartbeat에 Bifrost LLM 최소 연동을 추가했다. `tarsd`는 시작 시 `.env`를 로드하고, `BIFROST_BASE_URL`/`BIFROST_API_KEY`/`BIFROST_MODEL` 설정으로 run-once/run-loop에서 LLM 응답을 받아 daily log에 기록한다.
- 2026-02-13: LLM Provider 모듈을 추가해 `bifrost`, `openai`, `anthropic`을 공통 인터페이스로 지원한다. `bifrost`와 `openai`는 OpenAI-compatible API를 사용하는 단일 `OpenAICompatibleClient`로 통합했다. 인증 레이어를 분리해 `api-key`와 `oauth` 모드를 지원하고, OAuth 어댑터로 `codex-cli`, `claude-code`, `google-antigravity` 토큰 소스를 환경변수/로컬 파일에서 해석하도록 확장했다.
- 2026-02-13: `cmd/tars`를 `cobra` 기반 CLI 클라이언트로 전환했다. `tars heartbeat run-once --server-url ...` 명령으로 `tarsd` API를 호출해 결과를 출력한다.
- 2026-02-13: 역할 분리를 위해 heartbeat 실행 책임을 `tarsd`에 유지하고, `cmd/tars`에서는 실행 로직을 제거해 클라이언트 성격(도움말/명령 진입)으로 단순화했다.
- 2026-02-13: `tarsd`에 HTTP API(`POST /v1/heartbeat/run-once`)를 추가해 서버 측 heartbeat+LLM 실행 결과를 제공하도록 확장했다. `tars`는 `heartbeat run-once` 클라이언트 명령으로 해당 API를 호출해 결과를 출력한다.
- 2026-02-13: 브라우저 OAuth start/callback API와 `tars auth login` 명령을 제거했다. OAuth는 각 공식 CLI(`codex login` 등)로 선인증하고, 서버는 로컬 토큰 파일(예: `~/.codex/auth.json`)을 읽어 재사용한다.
- 2026-02-13: `internal/llm`에 `codex-cli` subprocess provider를 추가했다. `codex exec`를 직접 호출해 응답을 받으며, 기존 `openai` provider(`api.openai.com` API key 기반)는 변경 없이 유지한다.
- 2026-02-13: Go 모듈명을 `github.com/devlikebear/tarsncase`로 통일했다. 미사용 `internal/db` SQLite 패키지를 제거하고, 가짜 SSE 스트리밍 엔드포인트를 삭제했다. `exitError`/`isFlagError`를 `internal/cli`로 추출해 중복을 제거했다. `tarsd --serve-api`에 graceful shutdown을 추가했다. Anthropic `max_tokens`를 설정 가능하게 변경하고 기본값을 4096으로 올렸다.
- 2026-02-13: `PLAN.md`를 v3로 재작성했다. Phase 0 완료 현황을 정리하고, Phase 1~6 상세 개발 계획(LLM 채팅, 빌트인 도구+Agent Loop, 허트비트+크론잡, 스킬, 플러그인+MCP, cased 감시 데몬)과 OpenClaw 참고 지도를 추가했다.
- 2026-02-14: `EnsureWorkspace()`를 확장해 5개 부트스트랩 파일(AGENTS.md, SOUL.md, USER.md, IDENTITY.md, TOOLS.md)을 기본 템플릿으로 생성하도록 추가했다. 기존 파일이 있으면 덮어쓰지 않는다.
- 2026-02-14: `internal/prompt` 패키지를 추가했다. `Build(BuildOptions)` 함수가 워크스페이스 부트스트랩 파일 7종(IDENTITY, SOUL, USER, AGENTS, TOOLS, HEARTBEAT, MEMORY)을 읽어 시스템 프롬프트를 조립한다. 파일당 20000자 제한, sub-agent 모드에서는 AGENTS.md + TOOLS.md만 주입한다.
- 2026-02-14: `internal/llm`에 Chat API를 추가했다. `ChatMessage`, `ChatOptions`, `ChatResponse`, `Usage` 타입 정의, `Client.Chat()` 인터페이스 확장, `OpenAICompatibleClient`(bifrost/openai), `AnthropicClient`, `CodexCLIClient` 모두 구현. SSE 스트리밍 지원(`OnDelta` 콜백), 기존 `Ask()`는 `Chat()` 래퍼로 변경.
- 2026-02-14: `tarsd`에 `GET /v1/status`(workspace_dir/session_count), `POST /v1/compact`(placeholder) API를 추가했다.
- 2026-02-14: `tarsd`에 세션 관리 REST API 7종을 추가했다. `GET/POST/DELETE /v1/sessions`, `GET /v1/sessions/{id}`, `GET /v1/sessions/{id}/history`, `POST /v1/sessions/{id}/export`(마크다운), `GET /v1/sessions/search?q=keyword`(대소문자 무시 title 검색).
- 2026-02-14: `tarsd`에 `POST /v1/chat` SSE 스트리밍 채팅 API를 추가했다. 세션 자동 생성, 시스템 프롬프트 주입(`prompt.Build`), 토큰 기반 히스토리 로딩(`session.LoadHistory`), LLM Chat 호출(SSE `OnDelta`), transcript 저장을 통합했다. `llm.Client`를 직접 들고 다니도록 변경하고, heartbeat/chat 핸들러를 하나의 mux에 통합했다.
- 2026-02-14: `internal/session` 패키지를 추가했다. `Message` 타입(role/content/timestamp), JSONL transcript(append/read), `Session` 구조체, `Store`(sessions.json 기반 CRUD), 토큰 기반 동적 히스토리 로딩(`LoadHistory`)을 구현했다. 세션 저장 구조: `sessions/sessions.json`(인덱스) + `sessions/{id}.jsonl`(transcript).
- 2026-02-14: `cmd/tars`에 `chat` 명령을 추가했다. `tars chat -m "..." [--session ...] [--server-url ...]`로 `POST /v1/chat`을 호출하고 SSE `delta` 이벤트를 스트리밍 출력하며 `done` 이벤트에서 종료한다.
- 2026-02-14: `tarsd`와 `tars`에 `--verbose` 디버그 모드를 추가했다. `tars↔tarsd` HTTP 통신(요청/응답, 상태코드, 지연시간, SSE 이벤트)과 `tarsd↔LLM` 호출(provider/model/url, 메시지 수, 스트리밍 델타, usage/stop_reason)을 상세 로그로 출력한다.
- 2026-02-14: `tarsd` chat SSE 핸들러에 non-streaming LLM fallback을 추가했다. provider가 `OnDelta`를 호출하지 않아도 최종 assistant 응답을 `delta` 이벤트로 1회 전송해 `tars chat`에서 본문이 비지 않도록 수정했다.
