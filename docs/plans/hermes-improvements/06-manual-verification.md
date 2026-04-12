# Hermes Improvements Manual Verification

이 문서는 `feat/hermes-improvements` worktree 기준 수동 검증 시나리오다.

대상 기능:
- `#01` Toolset groups
- `#02` Context compression knobs
- `#03` Provider override
- `#04` Gateway run surface + consensus mode
- `#05` Memory backend interface

## 전제

- 작업 위치:
  - `<repo-root>`
- 예시 서버 주소:
  - `http://127.0.0.1:43180`
- 예시 명령은 `jq`가 있다고 가정한다. 없으면 JSON 응답에서 값을 수동 확인해도 된다.
- provider override / consensus 검증은 실제로 동작 가능한 `llm_providers` alias와 credential이 필요하다.

## 공통 실행

터미널 1:

```bash
cd <repo-root>
TARS_API_AUTH_MODE=off \
TARS_DASHBOARD_AUTH_MODE=off \
TARS_API_ALLOW_INSECURE_LOCAL_AUTH=true \
make dev-serve WORKSPACE_DIR=./workspace API_ADDR=127.0.0.1:43180 TARS_CONFIG=./config/standalone.yaml
```

터미널 2:

```bash
cd <repo-root>
export BASE_URL=http://127.0.0.1:43180
```

기본 smoke:

```bash
curl -sS "$BASE_URL/v1/status" | jq
curl -sS "$BASE_URL/v1/chat/tools" | jq '.tools | length'
curl -sS "$BASE_URL/v1/gateway/runs?limit=5" | jq
```

기대 결과:
- `/v1/status` 응답 성공
- `/v1/chat/tools` 응답 성공
- `/v1/gateway/runs` 응답 성공

## 테스트 데이터 준비

세션 하나 생성:

```bash
SESSION_ID=$(curl -sS -X POST "$BASE_URL/v1/admin/sessions" \
  -H 'Content-Type: application/json' \
  -d '{"title":"hermes-manual-verify"}' | jq -r '.id')
echo "$SESSION_ID"
```

워크스페이스 agent 디렉터리:

```bash
mkdir -p ./workspace/agents/group-smoke
mkdir -p ./workspace/agents/override-smoke
```

## Scenario 1. Tool Group Metadata Surface

목표:
- `/v1/chat/tools`에 tool `group` 메타가 붙는지 확인

실행:

```bash
curl -sS "$BASE_URL/v1/chat/tools" | jq '.tools[] | {name, group, high_risk} | select(.name == "read_file" or .name == "exec" or .name == "memory_search" or .name == "web_fetch")'
```

기대 결과:
- `read_file.group == "files"`
- `exec.group == "shell"`
- `memory_search.group == "memory"`
- `web_fetch.group == "web"`

## Scenario 2. Session Tool Group Allow/Deny

목표:
- 세션 config에 `tools_allow_groups`, `tools_deny_groups`가 저장되고 chat context에 반영되는지 확인

실행:

```bash
curl -sS -X PATCH "$BASE_URL/v1/admin/sessions/$SESSION_ID/config" \
  -H 'Content-Type: application/json' \
  -d '{
    "tools_custom": true,
    "tools_allow_groups": ["files", "web"],
    "tools_deny_groups": ["shell"]
  }'

curl -sS "$BASE_URL/v1/admin/sessions/$SESSION_ID/config" | jq
curl -sS "$BASE_URL/v1/chat/context?session_id=$SESSION_ID" | jq '{tool_names, compaction_trigger_tokens, compaction_keep_recent_tokens}'
```

기대 결과:
- config 응답에 `tools_allow_groups`, `tools_deny_groups`가 그대로 보임
- `tool_names`에 `read_file`, `write_file`, `web_fetch` 등은 남아 있음
- `tool_names`에 `exec`, `process`는 없어야 함

선택 검증(UI):
- 브라우저에서 `/console/chat/<SESSION_ID>` 진입
- Session Config Panel에서 group chip이 보이고 allow/deny 토글이 저장됨

## Scenario 3. Workspace Agent Frontmatter Group Policy

목표:
- AGENT frontmatter의 `tools_allow_groups`, `tools_deny_groups` alias normalization 확인

테스트 agent 작성:

`./workspace/agents/group-smoke/AGENT.md`

```md
---
name: group-smoke
description: Group policy smoke test
tools_allow_groups:
  - file
  - web
tools_deny_groups:
  - exec
---
Use file and web tools when needed. Do not use shell tools.
```

실행:

```bash
curl -sS "$BASE_URL/v1/agent/agents" | jq '.agents[] | select(.name == "group-smoke") | {name, tools_allow_groups, tools_deny_groups, tools_allow}'
```

기대 결과:
- `tools_allow_groups`에 canonical 값 `files`, `web`
- `tools_deny_groups`에 canonical 값 `shell`
- `tools_allow`에 file/web 계열은 포함 가능
- `tools_allow`에 `exec`는 포함되지 않아야 함

## Scenario 4. Compaction Knobs + SSE

목표:
- compaction config 노출
- deterministic mode 강제
- `context_info`, `compaction_applied` SSE 확인

설정 파일에 아래 값 반영 후 서버 재기동:

```yaml
compaction_trigger_tokens: 200
compaction_keep_recent_tokens: 80
compaction_keep_recent_fraction: 0.25
compaction_llm_mode: deterministic
compaction_llm_timeout_seconds: 1
```

긴 메시지 전송 예시:

```bash
LONG_MSG="$(printf 'compress-me %.0s' {1..240})"
curl -sS -N -X POST "$BASE_URL/v1/chat" \
  -H 'Content-Type: application/json' \
  -d "{\"session_id\":\"$SESSION_ID\",\"message\":\"$LONG_MSG\"}"
```

필요하면 2-3회 반복한다.

기대 결과:
- 스트림에 `type":"context_info"` 가 보임
- threshold를 넘기면 이후 호출에서 `type":"compaction_applied"` 가 보임
- `mode":"deterministic"` 가 보임

추가 확인:

```bash
curl -sS "$BASE_URL/v1/chat/context?session_id=$SESSION_ID" | jq '{compaction_trigger_tokens, compaction_keep_recent_tokens, compaction_keep_recent_fraction, compaction_last_mode}'
```

기대 결과:
- 응답에 위 4개 필드가 존재
- `compaction_last_mode == "deterministic"`

선택 검증(UI):
- `/console/chat/<SESSION_ID>`
- ContextMonitor에 trigger / protect / mode 정보가 표시됨

## Scenario 5. Provider Override Positive Path

목표:
- agent frontmatter `provider_override` 적용
- run detail에 `resolved_alias`, `resolved_kind`, `resolved_model`, `override_source` 기록 확인

설정 예시:

```yaml
gateway_task_override:
  enabled: true
  allowed_aliases:
    - anthropic_prod
    - anthropic_dev
  allowed_models:
    - claude-3-5-haiku-latest
    - claude-3-5-sonnet-latest
```

테스트 agent 작성:

`./workspace/agents/override-smoke/AGENT.md`

```md
---
name: override-smoke
description: Provider override smoke test
tier: light
provider_override:
  alias: anthropic_dev
  model: claude-3-5-haiku-latest
---
Answer briefly.
```

실행:

```bash
RUN_ID=$(curl -sS -X POST "$BASE_URL/v1/gateway/runs" \
  -H 'Content-Type: application/json' \
  -d '{"agent":"override-smoke","message":"say hello in one sentence"}' | jq -r '.run_id')

echo "$RUN_ID"
sleep 3
curl -sS "$BASE_URL/v1/gateway/runs/$RUN_ID" | jq '{run_id, status, tier, resolved_alias, resolved_kind, resolved_model, override_source, error}'
```

기대 결과:
- `status`가 최종적으로 `completed` 또는 provider 설정 실패 시 `failed`
- 성공 시:
  - `resolved_alias == "anthropic_dev"`
  - `resolved_model == "claude-3-5-haiku-latest"`
  - `override_source == "agent"`

## Scenario 6. Provider Override Negative Path

목표:
- allowlist 밖 alias가 loud failure로 막히는지 확인

`override-smoke`의 frontmatter를 아래처럼 수정:

```yaml
provider_override:
  alias: not_allowed_alias
```

실행:

```bash
RUN_ID=$(curl -sS -X POST "$BASE_URL/v1/gateway/runs" \
  -H 'Content-Type: application/json' \
  -d '{"agent":"override-smoke","message":"say hello in one sentence"}' | jq -r '.run_id')

sleep 3
curl -sS "$BASE_URL/v1/gateway/runs/$RUN_ID" | jq '{run_id, status, error, diagnostic_code, diagnostic_reason}'
```

기대 결과:
- `status == "failed"`
- `error` 또는 `diagnostic_reason`에 allowlist/override rejection 의미가 포함됨

## Scenario 7. Gateway Run List / Detail / Events

목표:
- `/v1/gateway/runs`, `/v1/gateway/runs/{id}`, `/v1/gateway/runs/{id}/events`
- 콘솔 run list/detail 동작 확인

실행:

```bash
RUN_ID=$(curl -sS -X POST "$BASE_URL/v1/gateway/runs" \
  -H 'Content-Type: application/json' \
  -d '{"agent":"override-smoke","message":"summarize the repository in one sentence"}' | jq -r '.run_id')

curl -sS "$BASE_URL/v1/gateway/runs?limit=5" | jq '.runs[0]'
curl -sS "$BASE_URL/v1/gateway/runs/$RUN_ID" | jq '{run_id, status, created_at, started_at, completed_at}'
curl -sS -N "$BASE_URL/v1/gateway/runs/$RUN_ID/events"
```

기대 결과:
- list/detail 모두 응답 성공
- SSE 스트림에서 최소 아래 순서가 보임:
  - `run_accepted`
  - `run_started`
  - `run_finished` 또는 `run_failed`

선택 검증(UI):
- `/console/gateway`
- 방금 run이 리스트에 보임
- 클릭 시 `/console/gateway/runs/<run_id>`로 이동
- detail 페이지에 prompt/response/run events 표시

## Scenario 8. Consensus Positive Path

목표:
- `subagents_run`의 `mode=consensus`
- run detail에 consensus badge / variant cards / consensus events 표시 확인

사전 설정:

```yaml
gateway_consensus_enabled: true
gateway_consensus_max_fanout: 3
gateway_consensus_budget_tokens: 20000
gateway_consensus_budget_usd: 1.0
gateway_consensus_timeout_seconds: 120
gateway_consensus_allowed_aliases_json:
  - anthropic_prod
  - openai_prod
```

채팅 프롬프트 예시:

```text
Use the subagents_run tool exactly once with this payload and then print the tool result only.

{
  "mode": "consensus",
  "agent": "explorer",
  "timeout_ms": 120000,
  "consensus": {
    "strategy": "synthesize",
    "variants": [
      {"alias": "anthropic_prod", "model": "claude-3-5-haiku-latest"},
      {"alias": "openai_prod", "model": "gpt-4o-mini"}
    ]
  },
  "tasks": [
    {"title": "repo-summary", "prompt": "Summarize this repository in 3 bullets."}
  ]
}
```

기대 결과:
- tool call 성공
- `/console/gateway`에 새 run 생성
- run detail에서 아래 확인 가능:
  - `consensus` badge
  - estimated / actual USD
  - variant 카드 2개
  - event log에 다음 이벤트
    - `consensus_planned`
    - `consensus_variant_started`
    - `consensus_variant_finished`
    - `consensus_aggregating`
    - `consensus_finished`

## Scenario 9. Consensus Negative Path

목표:
- consensus hard stop 확인

방법 A. fanout 초과

```yaml
gateway_consensus_max_fanout: 1
```

그 다음 Scenario 8 payload를 그대로 다시 실행.

기대 결과:
- tool result 또는 최종 오류에 `max_fanout` 초과 의미가 포함됨

방법 B. token budget 초과

```yaml
gateway_consensus_budget_tokens: 10
```

기대 결과:
- run 또는 tool 결과가 `consensus_budget_exceeded` 계열 실패로 끝남

## Scenario 10. Memory Backend Positive Path

목표:
- `memory_backend: file`에서 기존 memory API와 KB API가 그대로 동작하는지 확인

설정:

```yaml
memory_backend: file
```

실행:

```bash
curl -sS "$BASE_URL/v1/memory/assets" | jq

curl -sS -X PUT "$BASE_URL/v1/memory/file" \
  -H 'Content-Type: application/json' \
  -d '{"path":"MEMORY.md","content":"# Manual verify\n- coffee: latte\n"}' | jq

curl -sS "$BASE_URL/v1/memory/file?path=MEMORY.md" | jq '{path, kind, content}'

curl -sS -X POST "$BASE_URL/v1/memory/kb/notes" \
  -H 'Content-Type: application/json' \
  -d '{"slug":"manual-verify","title":"Manual Verify","kind":"note","summary":"verification note","body":"backend smoke"}' | jq '{slug, title, kind}'

curl -sS "$BASE_URL/v1/memory/kb/notes?query=manual&limit=10" | jq '{count, items}'

curl -sS -X POST "$BASE_URL/v1/memory/search" \
  -H 'Content-Type: application/json' \
  -d '{"query":"latte","limit":5,"include_memory":true,"include_daily":true,"include_knowledge":true}' | jq
```

기대 결과:
- assets/file/kb/search 모두 정상 응답
- `MEMORY.md` 읽기/쓰기 성공
- KB note 생성/조회 성공
- `memory/search` 결과에 `MEMORY.md` 또는 KB source가 포함됨

## Scenario 11. Memory Backend Invalid Config

목표:
- 허용되지 않은 backend 값에서 서버가 fail-fast 하는지 확인

설정:

```yaml
memory_backend: bogus
```

실행:

```bash
TARS_API_AUTH_MODE=off \
TARS_DASHBOARD_AUTH_MODE=off \
TARS_API_ALLOW_INSECURE_LOCAL_AUTH=true \
make dev-serve WORKSPACE_DIR=./workspace API_ADDR=127.0.0.1:43180 TARS_CONFIG=./config/standalone.yaml
```

기대 결과:
- 서버가 시작되지 않음
- 로그/stderr에 invalid memory backend 의미가 포함됨

## 최소 검증 세트

빠르게 확인하려면 아래만 먼저 돌리면 된다.

1. Scenario 1
2. Scenario 2
3. Scenario 4
4. Scenario 5
5. Scenario 7
6. Scenario 10

## 전체 검증 세트

릴리즈 전 확인이면 아래를 권장한다.

1. `go test ./...`
2. `npm run check`
3. `npm run build`
4. Scenario 1-11 전부
