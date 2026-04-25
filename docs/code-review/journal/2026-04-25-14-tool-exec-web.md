---
date: 2026-04-25
session: 14
scope: internal/tool/{exec,process_manager,process,web_search,web_fetch}.go (~1120줄) — exec/process/web 도메인
next: internal/tool/memory_search.go (496줄) + memory_get/save/aggregator (s15) — ID-001 평가의 핵심
findings: [RF-031, RF-032, RF-033, RF-034, RF-035, Q-011, ID-003 보강]
---

# Session 14 — `internal/tool/` exec/process/web

이번 세션 다룬 5 파일 ~1120줄:

- `exec.go` (217) — bash 디스패치 (skill+CLI 패턴의 핵심)
- `process_manager.go` (265) — long-running 프로세스 관리
- `process.go` (85) — process 빌트인 툴 (aggregator 패턴, 7 actions)
- `web_search.go` (281) — Brave/Perplexity 검색
- `web_fetch.go` (272) — URL fetch + SSRF 가드

## 한눈에

### exec — CLAUDE.md *"builtin bash tool"* 의 본체

- 11개 blocked commands (sudo/rm/...) + multi-line 거부 + workspaceDir 한정 + 30초 timeout
- background=true 시 ProcessManager로 위임
- shell **거치지 않음** (`exec.CommandContext(command, fields[1:]...)`) → injection 차단의 핵심

### process — 이미 aggregator (7 actions)

- list / poll / log / write / kill / clear / remove
- ProcessManager 에 위임
- file ops 보다 더 큰 sub-action 수로 통합 — ID-003 의 정당성 사례

### web_search + web_fetch — 둘 다 cfg-flag optional

- web_search: Brave + Perplexity 두 provider, in-process cache + TTL
- web_fetch: 강력한 SSRF 가드 (scheme/직접 IP/DNS lookup) + 5 redirect 한도, 매 redirect 재검증

## 좋은 패턴

### exec의 **shell-bypass injection 방어** ⭐

```go
fields := strings.Fields(commandLine)
cmd := exec.CommandContext(runCtx, command, fields[1:]...)
```

`bash -c "..."` 형태가 아니라 직접 fork/exec → `;`/`|`/`&&`/redirect 모두 단순 fields로 parse 됨. shell 메타문자가 작동 안 함. **blocklist 보다 훨씬 강력한 가드**. (RF-032 의 결론은 blocklist 자체가 false sense 라는 것.)

### web_fetch의 SSRF 다층 가드 ⭐

1. scheme 화이트리스트 (http/https)
2. 직접 IP private 거부 (`net.IP.IsPrivate`/IsLoopback/IsLinkLocal/IsMulticast/IsUnspecified + IPv6 fc/fd/fe80)
3. DNS lookup → 모든 IP 검증
4. **매 redirect 마다 재검증** (`cloneHTTPClientWithoutRedirect` + manual loop)
5. allowlist opt-in
6. localhost 명시 처리

→ RF-008/009 (sh 훅, HTTP 정책) 카테고리의 **모범 사례**. 외부 자원 접근하는 다른 코드의 reference 모델.

### process — aggregator 7 actions

이미 통합 패턴 적용. action 수 file (6) 보다 많고도 작동 → ID-003의 file aggregator 정당성 입증.

## 새 발견 (RF급)

### RF-031 — `exec` 의 `command` ↔ `cmd` schema-input 불일치

Schema는 `command` 만, parser 는 `cmd` 도 받음. silent acceptance — strict tool calling 에서 LLM은 schema 기반 호출이라 `cmd` 사용 안 하지만, 외부 caller 가 사용 시 schema validation 실패하는데 parser 통과 — 일관성 없음. cmd alias 제거 권장.

### RF-032 — `blockedExecCommands` 의 false sense of security

`fields[0]` 만 검사 → `bash -c "rm -rf /"` 우회 (fields[0]=bash). `mv`/`chmod`/`chown` 등 미포함. 실제 가드는 (1) shell 거치지 않음 (2) workspaceDir 한정 (3) timeout 30초 (4) multi-line 거부 — 이 4 가드가 훨씬 강력. blocklist 자체의 효용 의문.

CLAUDE.md *"Avoid backwards-compatibility hacks"* + *"Don't add error handling, fallbacks, or validation for scenarios that can't happen"* 의 정신과도 부분 충돌 — 가드가 실제로 막는 것보다 약속하는 게 많음.

### RF-033 — `web_search` 의 unbounded cache

TTL expired entry 가 map 에서 제거 안 됨. 같은 query 재호출 시 덮어쓰지만 한 번 호출 후 안 부르는 query 는 영원. 장기 운영 시 메모리 누수. lazy evict on Put 권장.

### RF-034 — `web_fetch.htmlToText` 의 단순 regex

```go
htmlTagRE = `<[^>]*>`
+ "&nbsp;" → " ", "&amp;" → "&"
```

문제:
- **`<script>` / `<style>` 본문 보존됨** — JS/CSS 가 텍스트로 LLM 들어감
- HTML entity 단편적 (nbsp/amp만)
- comment 처리 없음

→ `golang.org/x/net/html` Tokenizer 사용 권장. 출력 품질 ↑ + LLM 토큰 비용 ↓.

### RF-035 — `ProcessManager` timeout cap 30초 — long-running 의도와 충돌

`exec.go` 의 `maxExecTimeoutMS=30000` 을 그대로 재사용. **background process 가 30초 후 자동 kill됨**. *"Manage background exec sessions"* 의도 위반. dev server / 빌드 / batch 워크로드 모두 끊김. background 전용 timeout 상수 (10분 또는 unbounded) 분리 권장.

## 새 발견 (Q급)

### Q-011 — `process` 빌트인 툴의 실제 호출 빈도

7 action aggregator 인데 LLM이 얼마나 자주 사용하나? RF-035 (timeout 30초 cap) 결합 시 *"진짜 long-running 못 함"* → 사실상 exec background 수준 → 사용 빈도 낮을 가능성. usage tracker 데이터로 확인 필요.

가설 (a) drop in 빈도 시 ID-003의 외부화 후보 (skill+CLI 로 충분).

## ID-003 보강 (s14) — `web aggregator` 가 file 보다 더 명확한 첫 실험 후보

이번 세션의 가장 큰 부산물. ID-003 본문에 추가:

| 측면 | file (6 actions) | web (2 actions) |
|------|-----------------|-----------------|
| Sub-actions | read / write / edit / list / glob / apply_patch | search / fetch |
| 입력 도메인 | 4종 (file path, dir path, pattern, content) | 2종 (query, url) |
| Schema 복잡도 | 높음 (oneOf 4 case) | 낮음 (oneOf 2 case) |
| Cfg-flag | default enabled (PathPolicy 강제) | optional (둘 다 함께 켜짐) |
| LLM 워크플로 | 단계별 다양 | search → fetch 순차 일관 |

→ **web 부터 통합 실험 권장**. 비용 작고 위험 낮음. 성공 시 file 통합으로 확장.

이미 process가 7 action aggregator 로 작동 중 → schema 복잡도 자체는 LLM이 처리 가능 입증.

## 새 관점 ("필요한가/유용한가") 평가

| 툴 | 평가 | 비고 |
|----|------|------|
| `exec` | ✅ 필수 (CLAUDE.md *"builtin bash tool"* 명시) | RF-032 (blocklist false sense) 정리 가치 있음 |
| `process` | ⚠️ Q-011 (빈도 의문) + RF-035 (timeout cap 충돌). 빈도 낮으면 외부화 후보 | LLM이 직접 long-running 관리하는 워크플로 흔하지 않을 가능성 |
| `web_search` | ✅ 필수 (CLAUDE.md *"web search"* 명시) | provider 2종 (Brave/Perplexity) — 통합 자연스러움 |
| `web_fetch` | ✅ 필수 (CLAUDE.md *"web fetch"* 명시) | RF-034 (htmlToText 품질) 개선 가치 |

→ exec/web 은 명확히 필수. **process** 만 의문. ID-003 외부화 후보 목록에 포함.

## CLAUDE.md 정렬 검증

| 약속 | 실제 | 평가 |
|------|------|------|
| *"builtin bash tool"* | exec.go의 NewExecToolWithPolicy | ✅ |
| *"web search/fetch"* | web_search/web_fetch | ✅ |
| *"infrastructure that every session needs"* | exec/web 모두 부합. process 의문 | ⚠️ process |
| *"Avoid backwards-compatibility hacks"* | exec의 cmd alias [RF-031] + blocklist [RF-032] 위반 | ⚠️ |
| *"Don't add error handling ... for scenarios that can't happen"* | blockedExecCommands 가 사실상 작동 안 하는 시나리오 방어 | ⚠️ |

## 핵심 인사이트

### 보안 의식의 두 얼굴

- **web_fetch SSRF 가드** — 다층 방어 + redirect 재검증 + IP/DNS 검증. 정말 잘 됐음.
- **exec blockedCommands** — fields[0] 단순 검사라 우회 가능. false sense.

→ 보안 코드는 *"정말 작동하는가"* 검증이 핵심. exec 의 가드는 shell-bypass 만으로 충분 (실제로 잘 작동), blocklist 는 추가 가치 없음.

### 통합 정책 (ID-003) 의 명확한 첫 단계

s13 에서 file aggregator 검토했지만, s14 에서 web aggregator 가 더 안전한 첫 실험으로 부각:
- action 2개 (vs file 6개)
- 입력 도메인 2종 (vs file 4종)
- 이미 cfg-flag 군집
- process 가 7 action 으로 작동 입증

→ 사용자 결정 시 web 부터 시작 권장.

### dead code 카테고리 (RF-031/032)

CLAUDE.md *"Avoid backwards-compatibility hacks"* 의 두 위반 사례. RF-032 는 *보안 코드의 false sense* 라 RF-031 보다 무게 있음.

## 다음 세션 진입점 (s15)

`internal/tool/` memory tools 본체:

- `memory_search.go` (496줄, 가장 큰 tool 파일) — KB 통합의 핵심 (`include_knowledge` opt-in)
- `memory_get.go` (123)
- `memory_save.go` (76)
- `memory_aggregator.go` (46)
- (memory_kb.go + knowledge_aggregator.go 는 s10 에서 부분 검토)

**ID-001 평가의 핵심 세션**. KB가 chat path에 통합 안 됨을 s11 에서 확인했지만, memory_search 의 `include_knowledge` 가 KB 의 read 통합 마지막 표면. **그 성능/정확도 측정이 ID-001 가설 A vs B 의 결정적 입력**.

또 ID-003 평가 — memory aggregator (이미 통합) + knowledge aggregator (이미 통합) 의 LLM 호출 빈도 분포 + schema 복잡도 사례로 file/web aggregator 정당성 데이터 보강.
