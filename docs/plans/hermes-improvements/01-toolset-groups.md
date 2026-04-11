# 01 — Toolset Groups 정책 표면 정리

**Branch**: `feat/hermes-toolset-groups`
**Sprint**: S1
**Area**: `area/tool`
**Depends on**: —

## 배경

Hermes Agent는 툴을 `terminal / file / web / delegation` 같은 의미 있는 그룹으로 묶고, 에이전트 설정에서 그룹 단위로 켜고 끌 수 있게 해둔다. 이 덕에 "이 에이전트는 파일은 읽지만 exec은 못 씀" 같은 정책이 한 줄로 표현된다.

TARS도 이미 `internal/tool/tool_groups.go`를 갖고 있지만, (1) `AGENT.md` frontmatter와 세션 config에서 **그룹 단위** allow/deny가 일관되게 노출돼 있지 않고, (2) 차단 시 에이전트가 받는 에러 메시지가 "어느 그룹의 어느 규칙에 걸렸는지" 명시하지 않아 디버깅 비용이 크다.

## 목적

1. `tools_allow_groups` / `tools_deny_groups`를 `AGENT.md` frontmatter와 세션 config에서 1급 필드로 노출.
2. 개별 툴 allow/deny와 그룹 allow/deny의 우선순위를 명시적으로 정의 (deny always wins, 교집합 동작).
3. 차단 에러 메시지에 `which tool, which rule, which group` 포함.
4. 콘솔 세션 config 편집기에서 그룹 토글 UI 제공.

## 현황 조사

(구현 착수 시 확인할 것)

- `internal/tool/tool_groups.go`에 정의된 그룹 목록과 멤버 매핑
- `handler_chat_policy.go`에서 현재 tools_allow / tools_deny를 해석하는 경로
- `gateway_agents_policy.go`에서 `AGENT.md` frontmatter의 `tools_allow`가 파싱되는 지점
- 콘솔 `SessionConfigPanel.svelte`의 현재 정책 편집 UI 구조

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
tools_allow: [memory_search]           # 기존
tools_allow_groups: [file, web]        # 신규
tools_deny_groups: [exec]              # 신규
---
```

### 세션 config 확장

`workspace/sessions/<id>/config.yaml` 및 `/v1/admin/sessions/{id}/config` 엔드포인트에 동일 필드 추가.

### 차단 에러 메시지

```
tool 'exec' blocked: denied by group 'exec' in session policy (session_id=abc, agent=explorer)
```

포맷은 구조화 필드로도 함께 반환해서 프론트가 파싱 가능하도록.

## 수정 대상

### Backend
- `internal/tool/tool_groups.go` — 그룹 정의 정리(멤버 누락 점검), 문서화 주석 추가
- `internal/tool/policy.go` — 신규, `Policy` 구조체와 `Resolve` 로직
- `internal/tool/policy_test.go` — 신규, 표 기반 테스트
- `internal/tarsserver/handler_chat_policy.go` — `Policy`를 쓰도록 통합
- `internal/tarsserver/gateway_agents_parse.go` — frontmatter 파싱에 신규 필드
- `internal/tarsserver/gateway_agents_policy.go` — policy 빌드 경로 업데이트
- `internal/tarsserver/handler_chat_execution.go` — 차단 에러를 구조화 형태로 LLM에게 반환

### Frontend
- `frontend/console/src/components/SessionConfigPanel.svelte` — 그룹 토글 UI
- `frontend/console/src/lib/api.ts` — 세션 config 타입 확장

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
- [ ] `AGENT.md` frontmatter에서 `tools_allow_groups`, `tools_deny_groups` 파싱
- [ ] 세션 config API에서 동일 필드 읽고 쓰기 가능
- [ ] 차단 에러 메시지가 `group`/`rule`/`tool` 세 필드를 포함
- [ ] 콘솔 세션 config 편집기에 그룹 토글 UI
- [ ] `make test`, `make vet`, `make fmt` 통과
- [ ] 기존 `tools_allow` / `tools_deny`만 쓰는 에이전트는 **코드 변경 없이** 동일 동작 (backward compat)

## Identity Check

- **단일 Go 바이너리**: 새 의존성 없음 ✓
- **File-first**: 파일 형식만 확장, DB 도입 없음 ✓
- **Scope isolation**: 변경 지점이 `user` scope 정책 해석에 국한, `pulse_/reflection_` 경로에 영향 없음 ✓
- **정책은 config, 메커니즘은 Go**: 정책 필드가 YAML로 선언되고 Go가 결정론적으로 해석 ✓
- **Memory 영향 없음** ✓
