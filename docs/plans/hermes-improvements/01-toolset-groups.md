# 01 — Toolset Groups 정책 표면 정리

**Branch**: `feat/hermes-toolset-groups`
**Sprint**: S1
**Area**: `area/tool`
**Depends on**: —

## 배경

Hermes Agent는 툴을 `terminal / file / web / delegation` 같은 의미 있는 그룹으로 묶고, 에이전트 설정에서 그룹 단위로 켜고 끌 수 있게 해둔다. 이 덕에 "이 에이전트는 파일은 읽지만 exec은 못 씀" 같은 정책이 한 줄로 표현된다.

TARS도 이미 `internal/tool/tool_groups.go`에 그룹 분류가 있지만, (1) `AGENT.md` frontmatter와 세션 config에서 **그룹 단위** allow/deny가 일관되게 노출돼 있지 않고, (2) 차단 시 에이전트가 받는 에러 메시지가 "어느 그룹의 어느 규칙에 걸렸는지" 명시하지 않아 디버깅 비용이 크다.

현재 canonical 그룹은 `memory / files / shell / web` 네 개다 (`tool_groups.go:11-16`). 그룹 이름은 **리네이밍하지 않는다** — 이미 설정 표면(frontmatter, 세션 config)에 암묵적으로 노출돼 있고, hermes 스타일 표기(`exec`, `file`)는 입력 시점의 alias로만 받아들인다.

## 목적

1. `tools_allow_groups` / `tools_deny_groups`를 `AGENT.md` frontmatter와 세션 config에서 1급 필드로 노출.
2. 개별 툴 allow/deny와 그룹 allow/deny의 우선순위를 명시적으로 정의 (deny always wins, 교집합 동작).
3. 차단 에러 메시지에 `which tool, which rule, which group` 포함.
4. 콘솔 세션 config 편집기에서 그룹 토글 UI 제공.

## 현황 (확인 완료된 사실)

- **그룹 정의**: `internal/tool/tool_groups.go:11-16` — 네 개의 canonical 그룹 `memory / files / shell / web`. 할당 규칙:
  - `memory`: `memory`, `knowledge`, `memory_*` prefix
  - `shell`: `exec`, `process`
  - `web`: `web_search`, `web_fetch`
  - `files`: `read*`, `write*`, `edit*`, `list_dir`, `glob`, `apply_patch`
- **세션 정책 저장소**: `internal/session/session.go:14-23`의 `SessionToolConfig`
  ```go
  type SessionToolConfig struct {
      ToolsEnabled  []string `json:"tools_enabled,omitempty"`
      ToolsCustom   bool     `json:"tools_custom,omitempty"`
      ToolsDisabled []string `json:"tools_disabled,omitempty"`
      SkillsEnabled []string `json:"skills_enabled,omitempty"`
      SkillsCustom  bool     `json:"skills_custom,omitempty"`
      MCPEnabled    []string `json:"mcp_enabled,omitempty"`
  }
  ```
  는 `Session.ToolConfig` 포인터로 `workspace/sessions/sessions.json` 인덱스에 저장된다 (`session.go:25-36, 637`). 세션당 별도 YAML 파일 **없음**. 접근은 `Store.SetToolConfig(id, *SessionToolConfig)` (`session.go:477-493`).
- (구현 착수 시 추가 확인) `handler_chat_policy.go`에서 현재 tools_allow / tools_deny를 해석하는 경로, `gateway_agents_policy.go`에서 `AGENT.md` frontmatter가 파싱되는 지점, 콘솔 `SessionConfigPanel.svelte`의 현재 정책 편집 UI 구조.

## 제안 설계

### 데이터 모델

```go
// internal/tool/policy.go (신규)
type Policy struct {
    AllowTools   []string // 개별 툴 이름
    DenyTools    []string
    AllowGroups  []string // 그룹 이름 (tool_groups.go에 정의된 이름)
    DenyGroups   []string
}

// Resolve returns the effective set of allowed tool names, applying
// deny-wins semantics and expanding groups via the provided group resolver.
func (p Policy) Resolve(all []string, groups GroupResolver) []string { ... }
```

### 해석 순서

1. `all` = registry의 전체 툴 목록에서 시작
2. `AllowGroups`가 비어 있지 않으면 그룹 합집합으로 제한
3. `AllowTools`가 비어 있지 않으면 그 합집합과 다시 교집합
4. `DenyGroups` 제외
5. `DenyTools` 제외
6. 결과 반환

**Deny는 항상 이긴다.** 그룹 allow로 들어왔더라도 개별 deny에 걸리면 제외. 반대로 그룹 deny에 걸려도 개별 allow로 되살릴 수는 없다(단순성 우선).

### AGENT.md frontmatter 확장

```yaml
---
name: explorer
tier: light
tools_allow: [memory_search]             # 기존
tools_allow_groups: [files, web]         # 신규 — canonical 이름
tools_deny_groups: [shell]               # 신규 — canonical 이름
---
```

Canonical 이외의 입력(`file`, `exec`, `terminal` 등)은 **입력 정규화 테이블**에서 alias로 받아 canonical 이름으로 변환한다:

```go
// internal/tool/tool_groups.go 또는 tool_policy.go
var groupAliases = map[string]string{
    "file":     "files",
    "exec":     "shell",
    "terminal": "shell",
}
```

Alias 테이블은 `ExpandToolGroups` 진입점에서만 적용된다. 저장/표시는 항상 canonical 이름으로 수렴하므로 "같은 그룹이 두 이름으로 나뉘는 상태"가 생기지 않는다. 테이블에 없는 미지의 이름은 기존대로 `unknownGroups`로 돌려준다.

### 세션 config 확장

세션 정책은 `workspace/sessions/sessions.json` 인덱스 내부 `Session.ToolConfig` 필드(`*SessionToolConfig`)에 기록된다. 이 구조체를 확장해 그룹 필드를 추가한다:

```go
// internal/session/session.go
type SessionToolConfig struct {
    ToolsEnabled  []string `json:"tools_enabled,omitempty"`
    ToolsCustom   bool     `json:"tools_custom,omitempty"`
    ToolsDisabled []string `json:"tools_disabled,omitempty"`

    // 신규
    ToolsAllowGroups []string `json:"tools_allow_groups,omitempty"`
    ToolsDenyGroups  []string `json:"tools_deny_groups,omitempty"`

    SkillsEnabled []string `json:"skills_enabled,omitempty"`
    SkillsCustom  bool     `json:"skills_custom,omitempty"`
    MCPEnabled    []string `json:"mcp_enabled,omitempty"`
}
```

- 필드 추가는 JSON omitempty라 `sessions.json`을 마이그레이션할 필요가 없다. 기존 세션은 `nil` 슬라이스로 로드되어 "그룹 제약 없음"과 동일.
- `Store.SetToolConfig`는 현재와 동일한 atomic rewrite 경로를 그대로 탄다.
- 읽기/쓰기 API는 기존 `/v1/admin/sessions/{id}/config` 엔드포인트를 그대로 확장한다 (신규 엔드포인트 없음).
- 콘솔은 `/v1/chat/tools` 응답에 tool별 group 메타데이터를 추가해 그룹 토글 UI를 그린다.

### 차단 에러 메시지

```
tool 'exec' blocked: denied by group 'shell' in session policy (session_id=abc, agent=explorer)
```

포맷은 구조화 필드로도 함께 반환해서 프론트가 파싱 가능하도록:

```json
{
  "tool": "exec",
  "rule": "group_deny",
  "group": "shell",
  "source": "session" | "agent" | "config_default"
}
```

`source`는 어느 정책 레이어에서 차단이 발생했는지 — 세션 config, AGENT.md frontmatter, 전역 config — 를 구분한다.

## 수정 대상

### Backend
- `internal/tool/tool_groups.go` — alias 테이블 추가, `ExpandToolGroups` 진입점에서만 적용
- `internal/tool/policy.go` — 신규, `Policy` 구조체와 `Resolve` 로직 (deny-wins)
- `internal/tool/policy_test.go` — 신규, 표 기반 테스트
- `internal/session/session.go` — `SessionToolConfig`에 `ToolsAllowGroups`/`ToolsDenyGroups` 추가
- `internal/session/session_test.go` — 라운드트립 테스트 (기존 세션이 nil 슬라이스로 로드되는지 포함)
- `internal/tarsserver/handler_chat_policy.go` — `Policy`를 쓰도록 통합
- `internal/tarsserver/gateway_agents_parse.go` — frontmatter 파싱에 `tools_allow_groups`/`tools_deny_groups`
- `internal/tarsserver/gateway_agents_policy.go` — policy 빌드 경로 업데이트
- `internal/tarsserver/handler_chat_execution.go` — 차단 에러를 구조화 형태로 LLM에게 반환
- `internal/tarsserver/handler_chat_tools.go` (또는 `/v1/chat/tools` 핸들러) — tool별 group 메타 추가

### Frontend
- `frontend/console/src/components/SessionConfigPanel.svelte` — 그룹 토글 UI
- `frontend/console/src/lib/api.ts` — 세션 config 타입 확장 + tool group 메타데이터

### Config
- `internal/config/config.go` — 전역 기본값 필드 추가(선택): `default_tools_deny_groups`
- `internal/config/config_input_fields.go` — 매핑

## 테스트 계획

### Unit
- `policy_test.go`: 표 기반으로 allow/deny × tool/group 조합 (최소 12 케이스)
- Deny-wins 경계: group allow + tool deny, group deny + tool allow(되살아나지 않아야 함)
- 빈 allow의 의미(= 전체 허용)가 유지되는지

### Integration
- `handler_chat_test.go`에 정책 적용 end-to-end 테스트 추가
- 차단 에러 메시지 포맷 검증

### Frontend
- `SessionConfigPanel` 스크린샷 회귀(이미 있다면)

## Acceptance Criteria

- [ ] `Policy.Resolve`가 표 기반 테스트 전부 통과
- [ ] Canonical 그룹명 `memory / files / shell / web`만 저장/응답에 노출되고, 입력에서 `file`/`exec`/`terminal` 같은 alias는 ingest 시점에 canonical로 정규화
- [ ] `AGENT.md` frontmatter에서 `tools_allow_groups`, `tools_deny_groups` 파싱
- [ ] `SessionToolConfig`에 `ToolsAllowGroups`/`ToolsDenyGroups` 필드 추가, `sessions.json` round-trip 정상
- [ ] 기존 세션(구 필드 없음)이 변경 없이 로드·저장 (마이그레이션 없음)
- [ ] 세션 config API(`/v1/admin/sessions/{id}/config`)에서 동일 필드 읽고 쓰기 가능
- [ ] `/v1/chat/tools` 응답에 tool별 group 메타데이터 포함
- [ ] 차단 에러 메시지가 `tool`/`rule`/`group`/`source` 네 필드를 포함
- [ ] 콘솔 세션 config 편집기에 그룹 토글 UI
- [ ] `make test`, `make vet`, `make fmt` 통과
- [ ] 기존 `tools_allow` / `tools_deny`만 쓰는 에이전트는 **코드 변경 없이** 동일 동작 (backward compat)

## Identity Check

- **단일 Go 바이너리**: 새 의존성 없음 ✓
- **File-first**: 파일 형식만 확장, DB 도입 없음 ✓
- **Scope isolation**: 변경 지점이 `user` scope 정책 해석에 국한, `pulse_/reflection_` 경로에 영향 없음 ✓
- **정책은 config, 메커니즘은 Go**: 정책 필드가 YAML로 선언되고 Go가 결정론적으로 해석 ✓
- **Memory 영향 없음** ✓
