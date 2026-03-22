# 튜토리얼: 미니 TARS 클론코딩 가이드

> 대상 독자: 구조를 이해한 뒤 최소 기능을 직접 복제해보고 싶은 개발자
> 목표 시간: 120분
> 기준 세션: `analyze-20260310-140305` / `checkpoint-013`

---

## 학습 목표

- TARS의 최소 골격을 직접 다시 만들면서 각 레이어의 책임을 체감한다.
- 세션 저장, 프롬프트 조립, tool registry, SSE 스트리밍의 최소 조합을 직접 구현한다.
- semantic memory, Skill Hub, project board/activity/dashboard를 나중에 얹을 확장 지점을 미리 파악한다.
- provider adapter나 host integration 을 붙일 때 경계를 어디에 두어야 하는지 미리 파악한다.
- 원본 저장소의 어떤 파일이 각 단계의 참고본인지 빠르게 찾을 수 있다.

---

## 사전지식과 실행환경

- 필수 사전지식: Go, `net/http`, JSON, context
- 런타임: Go `1.25.6`
- 권장 명령:
  - `go test ./...`
  - `make dev-serve`
  - `make dev-tars`
- 시작 파일:
  - `cmd/tars/main.go`
  - `internal/tarsserver/handler_chat.go`

---

## 아키텍처 한눈에 보기

| 레이어 | 원본 참고 파일 | 클론코딩 목표 |
|--------|----------------|---------------|
| CLI | `cmd/tars/main.go` | `mini-tars` 루트 명령 만들기 |
| Session Store | `internal/session/session.go` | 대화 세션 메타데이터 저장 |
| Transcript | `internal/session/transcript.go` | JSONL append/read 구현 |
| Prompt Builder | `internal/prompt/builder.go` | 단순 시스템 프롬프트 조립 |
| Tool Registry | `internal/tool/tool.go` | 1-2개 도구만 가진 registry |
| Chat Handler | `internal/tarsserver/handler_chat.go` | `/chat` SSE 엔드포인트 구현 |
| Client | `pkg/tarsclient/client.go` | 스트리밍 클라이언트 구현 |
| Optional Memory | `internal/memory/semantic.go` | embedding recall 확장 지점 만들기 |
| Optional Hub | `internal/skillhub/*` | 원격 설치 계층을 분리 설계하기 |

```text
CLI -> HTTP chat endpoint -> prompt -> optional memory/tool -> transcript save -> stream back
```

---

## 단계별 클론코딩

### Step 1. 얇은 CLI부터 만든다

작업 순서:

1. `cmd/tars/main.go`를 참고해 루트 명령과 `serve` 서브커맨드를 만든다.
2. 플래그 파싱만 하고 실제 로직은 별도 패키지로 넘긴다.

참고 파일:

- `cmd/tars/main.go`
- `cmd/tars/server_main.go`

확인 포인트:

- [ ] 엔트리포인트가 비즈니스 로직을 갖지 않는다.
- [ ] `version` 또는 `serve` 같은 최소 서브커맨드가 실행된다.

### Step 2. 세션과 transcript를 붙인다

작업 순서:

1. `internal/session/session.go`를 참고해 세션 ID 생성과 목록 저장을 구현한다.
2. `internal/session/transcript.go`를 참고해 JSONL append/read를 구현한다.

참고 파일:

- `internal/session/session.go`
- `internal/session/transcript.go`

확인 포인트:

- [ ] 새 세션 생성 후 다시 읽을 수 있다.
- [ ] 사용자/assistant 메시지가 JSONL에 순서대로 쌓인다.

### Step 3. 프롬프트와 툴 registry를 최소 구현한다

작업 순서:

1. `internal/prompt/builder.go`를 참고해 고정 시스템 프롬프트 + 간단한 workspace 문서 읽기를 만든다.
2. `internal/tool/tool.go`를 참고해 `echo`나 `time` 정도의 단순 툴을 registry에 넣는다.
3. `internal/agent/loop.go`를 그대로 복제하기보다, 처음에는 "LLM 응답만 반환"하는 단순 루프부터 만든다.

참고 파일:

- `internal/prompt/builder.go`
- `internal/tool/tool.go`
- `internal/agent/loop.go`

확인 포인트:

- [ ] 툴 schema를 LLM에 넘길 수 있다.
- [ ] 툴이 없어도 기본 채팅이 동작한다.

### Step 4. HTTP/SSE 채팅을 붙인다

작업 순서:

1. `internal/tarsserver/handler_chat.go`를 참고해 `/chat` POST 엔드포인트를 만든다.
2. 스트리밍 응답은 `pkg/tarsclient/client.go`의 SSE 패턴을 단순화해서 구현한다.
3. 완료 시 transcript를 저장한다.

참고 파일:

- `internal/tarsserver/handler_chat.go`
- `internal/tarsserver/handler_chat_context.go`
- `pkg/tarsclient/client.go`

확인 포인트:

- [ ] 클라이언트가 delta 이벤트를 순서대로 받는다.
- [ ] 세션 ID가 생성되면 응답에서 확인할 수 있다.

### Step 5. semantic recall은 선택적 확장으로 붙인다

작업 순서:

1. 기본 버전이 안정화된 뒤 `internal/memory/semantic.go`처럼 별도 memory service를 추가한다.
2. prompt 빌더 쪽에서 semantic hit를 우선하고, 실패하면 파일 검색으로 fallback 하게 만든다.
3. `memory_save` 같은 도구가 파일 저장과 semantic index 갱신을 같이 하도록 한다.

참고 파일:

- `internal/memory/semantic.go`
- `internal/prompt/memory_retrieval.go`
- `internal/tool/memory_search.go`
- `internal/tool/memory_save.go`

확인 포인트:

- [ ] memory 기능이 core chat path를 깨지 않고 optional layer로 붙는다.
- [ ] index가 비활성화돼도 기존 파일 기반 검색이 유지된다.

### Step 6. 원본 저장소처럼 점진 확장한다

작업 순서:

1. project policy를 추가하려면 `internal/project/store.go`와 `workflow_policy.go`를 참고한다.
2. skill/plugin/MCP를 붙이려면 `internal/extensions/manager.go`, `internal/skill/loader.go`, `internal/mcp/client.go`를 본다.
3. 긴 transcript 축약은 `internal/session/compaction.go`를 참고한다.

참고 파일:

- `internal/project/store.go`
- `internal/project/workflow_policy.go`
- `internal/extensions/manager.go`
- `internal/session/compaction.go`

확인 포인트:

- [ ] 최소 버전이 안정화된 뒤에만 확장 레이어를 붙인다.
- [ ] 원본처럼 프롬프트, 툴, 워크스페이스 상태를 분리해 유지한다.

### Step 6.5. Skill Hub 같은 원격 설치 계층은 runtime 로더와 분리한다

작업 순서:

1. 원격 registry를 읽는 search/install/update 로직을 runtime loader와 별도 패키지로 둔다.
2. 설치 결과는 workspace 하위 디렉터리에 내려받고 별도 installed DB를 기록한다.
3. 런타임에서 실제로 읽는 path는 `internal/skill/mirror.go`처럼 별도 runtime mirror를 둘지 검토한다.

참고 파일:

- `cmd/tars/skill_main.go`
- `cmd/tars/plugin_main.go`
- `internal/skillhub/registry.go`
- `internal/skillhub/install.go`
- `internal/skill/mirror.go`

확인 포인트:

- [ ] 원격 배포와 runtime 실행 경로를 한 타입에 섞지 않았다.
- [ ] partial install이나 integrity 검증 같은 운영 문제를 고려할 수 있는 구조다.

### Step 7. provider adapter 는 마지막에 붙인다

작업 순서:

1. 최소 버전이 안정화된 뒤에만 외부 provider adapter를 추가한다.
2. OpenAI 호환 API부터 붙일 때는 `internal/llm/openai_compat_client.go`처럼 `/chat/completions` 하나만 지원하는 얇은 adapter부터 만든다.
3. Anthropic, Gemini Native, Codex 같은 특수 provider는 공통 client에 if 문을 늘리기보다 별도 구현체로 분리한다.

참고 파일:

- `internal/llm/openai_compat_client.go`
- `internal/llm/anthropic.go`
- `internal/llm/gemini_native.go`
- `internal/llm/openai_codex_client.go`

확인 포인트:

- [ ] provider별 wire format 차이를 하나의 giant switch 로 뭉개지 않았다.
- [ ] OAuth refresh 같은 특수 lifecycle 은 일반 token resolver 와 분리했다.

### Step 8. 프로젝트 운영 레이어를 별도 확장으로 붙인다

작업 순서:

1. `internal/project/kanban.go`와 `internal/project/activity.go`를 참고해 task board와 activity log를 문서로 저장한다.
2. `internal/project/task_report.go`처럼 worker 결과를 고정 포맷으로 파싱한다.
3. 읽기 전용 운영 화면이 필요하면 `internal/tarsserver/dashboard.go`처럼 서버 렌더링 HTML + SSE refresh 구조를 붙인다.

참고 파일:

- `internal/project/kanban.go`
- `internal/project/activity.go`
- `internal/project/task_report.go`
- `internal/tarsserver/dashboard.go`

확인 포인트:

- [ ] board/status를 자유 문자열로 두지 않고 canonical 값으로 제한한다.
- [ ] worker 결과를 사람이 읽는 문장 대신 고정 필드 계약으로 저장한다.
- [ ] 운영 UI는 API 전체를 새로 만들기보다 기존 문서를 읽는 얇은 화면이어도 충분한지 검토한다.

---

## 자가검증 체크리스트

- [ ] 내가 만든 최소 버전의 entrypoint가 얇은 래퍼인지 확인했다.
- [ ] transcript 저장과 읽기가 테스트로 검증된다.
- [ ] SSE 기반 스트리밍 채팅이 동작한다.
- [ ] semantic recall을 나중에 얹을 인터페이스 지점을 확보했다.
- [ ] 확장 기능을 붙일 위치를 원본 경로 기준으로 설명할 수 있다.

---

## 확장 미션

1. `internal/usage/tracker.go`를 참고해 토큰 사용량 기록을 추가한다.
2. `internal/tarsclient/app_model.go`를 참고해 TUI 버전을 별도 실험 프로젝트로 만들어 본다.
3. `internal/browser/service.go`를 참고해 브라우저 자동화 진입점을 최소 1개 붙여 본다.
4. `internal/project/project_runner.go`를 참고해 "문서 기반 PM loop"를 별도 실험 기능으로 붙여 본다.
5. `internal/browserrelay/server.go`와 `cmd/tars/service_main.go`를 참고해 host integration adapter를 별도 패키지로 실험해 본다.
