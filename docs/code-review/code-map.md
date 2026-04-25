# Code Map — 검토 진척표

각 항목의 상태:

- ⬜ 미확인
- 🔍 검토 중
- ✅ 완료
- ⚠️ 검토 완료 + 이슈 발견 (finding 링크 참조)
- ➖ 의도적으로 스킵 (이유 명기)

## `cmd/tars/` — CLI 진입점

| 파일                       | 상태 | 비고 |
|---------------------------|-----|------|
| main.go                   | ✅  | [journal 01](journal/2026-04-25-01-entrypoint.md) |
| mainthread_darwin.go      | ⬜  | macOS 메인스레드 처리 |
| mainthread_other.go       | ⬜  | non-darwin no-op |
| client_main.go            | ⬜  | `tars --message` 클라이언트 모드 |
| console_main.go           | ⬜  | 인자 없을 때 콘솔 띄우기 |
| server_main.go            | ✅  | [journal 02](journal/2026-04-25-02-server-bootstrap.md) — `tarsserver.Serve` 위임 |
| service_main.go           | ⬜  | 서비스 등록/관리 |
| init_main.go              | ⬜  | `tars init` |
| doctor_main.go            | ⬜  | `tars doctor` |
| approval_main.go          | ⬜  | `tars approve` |
| assistant_main.go         | ⬜  | `tars assistant` |
| cron_main.go              | ⬜  | `tars cron` |
| skill_main.go             | ⬜  | `tars skill` |
| plugin_main.go            | ⬜  | `tars plugin` |
| mcp_main.go               | ⬜  | `tars mcp` |
| hub_commands.go           | ⬜  | skillhub 관련 |
| runtime_main.go           | ⬜  | runtime 헬퍼 |

## `internal/` — 핵심 패키지

### 진입/부트스트랩
| 패키지         | 상태 | 비고 |
|---------------|-----|------|
| envloader     | ⬜  | `.env` 로딩 (main에서 호출) |
| buildinfo     | ⬜  | 버전 정보 |
| config        | ⚠️  | s25: types/load/schema/llm_resolve (RF-062/063, **모범**: ResolveLLMTier 7-validation, LLM 3-layer, JSON env override). s26: defaults/defaults_apply/yaml/yaml_paths/parse_helpers/config_input_fields (RF-064/065/066, **모범**: configInputField DRY 180+ field, applyLLMPoolDefaults kind-specific). **종료 + ID-005 Phase 3 인풋 완성** |
| cli           | ⬜  | CLI 헬퍼 |

### 서버/네트워킹
| 패키지         | 상태 | 비고 |
|---------------|-----|------|
| tarsserver    | 🔍  | s02: `main.go`/`main_cli.go` (TN-002, RF-001/002, Q-001). s03: `main_bootstrap.go` (RF-003/004, Q-002). s04: `main_serve_api.go` 핀포인트 + `main_session.go` (RF-005/006, Q-004, TN-001 종착). 미검토: `main_serve_api.go` 본문 대부분, handler_*.go, gateway_agents_*.go 등 |
| tarsclient    | ⬜  | 다른 tars 인스턴스 호출 클라이언트 |
| serverauth    | ⬜  | Bearer token 인증 |
| auth          | ⬜  | (확인 필요) |

### 플러그인 / 확장
| 패키지         | 상태 | 비고 |
|---------------|-----|------|
| plugin        | ✅  | s06: 전수 (builtin/registry/types/manifest/loader). RF-011/012/013, RF-008 보강 |
| browserplugin | ⚠️  | 빌트인 플러그인 — [TN-001](findings/tensions.md#tn-001) |
| extensions    | 🔍  | s05: `manager.go` 구조+Start/Close+initBuiltinPlugins/CollectHTTPHandlers + `lifecycle.go` 전체 (RF-007/008/009, TN-003). 다음: `Reload` (L175-319). 미검토: `disabled.go`, `manager.go` Reload 본체 + watch loop |
| skill         | ⬜  | `.md` 스킬 로더 |
| skillhub      | ⬜  | 스킬 허브 클라이언트 |
| mcp           | ⬜  | Model Context Protocol 클라이언트 |

### 도메인 기능
| 패키지         | 상태 | 비고 |
|---------------|-----|------|
| browser       | ⬜  | Chrome/CDP 자동화 (browserplugin이 감쌈) |
| gateway       | ⚠️  | s21: lifecycle + executor (RF-050/051). s22: 실행 디스패치 + consensus + persistence (RF-052/053/054, Q-014/015, **모범**: 7-layer consensus budget guard). s23: channels + executors + nodes + reports + override (RF-055/056/057, Q-016, **모범**: ResolveOverride 7-layer task override guard). **종료 + ID-005 인벤토리 완성** |
| session       | ⚠️  | s24: session/compaction/tasks/transcript/locks/message 전수 (RF-058/059/060/061, Q-017/018, **모범**: LoadHistorySnapshot compaction 보존 + stacking carry-forward + CJK token 가중치 + path-level mutex). **종료** |
| assistant     | ⬜  | 어시스턴트 (확인 필요) |
| tool          | ⚠️  | s12: core (**모범**: RegistryScope). s13: file ops (RF-029/030, **ID-003**). s14: exec/web (RF-031~035, **모범**: web_fetch SSRF). s15: memory 본체 (RF-036/037/038, **ID-001 가설 A 확정**, ID-003 process 패턴). s16: telegram/gateway/sessions/cron/subagents (RF-039/040, **모범**: subagent triple guard). s17: subagents_plan/orchestrate (RF-041/042, Q-012, **모범**: mock-able fn vars + reference rewrite). **종료** |
| llm           | ⚠️  | s18: core (RF-043/044, **모범**: NewRouter 7-validation). s19: provider 1차 (anthropic/claude_code_cli/openai_compat) — RF-045/046/047, Q-013, **모범**: anthropic 자동 prompt caching. s20: provider 2차 (openai_codex/gemini_native/model_lister) — RF-048/049, **ID-004 capability 매트릭스 완성**, **모범**: openai-codex OAuth 자동 갱신 + gemini-native preflight check. **종료** |
| memory        | ⚠️  | s09: backend/workspace/experience/file_backend/gemini_embed/semantic (RF-017/018, Q-009, RF-014 보강). s10: knowledge.go 전수 (RF-019/020/021/022, ID-001, Q-009 resolve). **종료** |
| prompt        | ⚠️  | s11: bootstrap_sections/builder/memory_retrieval 전수 (RF-019 보강, RF-023/024/025, RF-018 보강, ID-002). **종료** |
| sysprompt     | ⚠️  | s11: sysprompt.go 전수 (RF-024). **종료** |

### 시스템 surface (백그라운드)
| 패키지         | 상태 | 비고 |
|---------------|-----|------|
| pulse         | ✅  | s07: 전수 (types/state/notify/runtime/signal/decider + autofix). RF-014 + Q-004 resolve. **모범 패키지**. |
| reflection    | ✅  | s08: 전수 (types/config/jobs/scheduler/state/reflection/derivation/job_kb_cleanup/job_memory). RF-015/016 + RF-014 보강. **모범 패키지**. |
| ops           | ⬜  | 시스템 헬스/디스크 |
| cron          | ⚠️  | s27: manager/store/validation/payload_meta/helpers (RF-067/068/069/070, **모범**: 3-mode schedule + TryStartRun mutex + exponential backoff + delete_after_run + PayloadMeta `_tars_cron` 격리 + natural language schedule). **종료. backend 전수 완료**. |

### 보조
| 패키지         | 상태 | 비고 |
|---------------|-----|------|
| approval      | ⬜  | 승인 워크플로 |
| usage         | ⬜  | 사용량 추적 |
| secrets       | ⬜  | 시크릿 처리 |
| vaultclient   | ⬜  | vault 클라이언트 |
| release       | ⬜  | 릴리스 관련 |
| scheduleexpr  | ⬜  | 크론 표현식 파서 |
| assetpath     | ⬜  | 에셋 경로 헬퍼 |
| textutil      | ⬜  | 텍스트 유틸 |
| agent         | ⬜  | (확인 필요) |

## `frontend/console/` — Svelte 5 SPA

| 영역              | 상태 | 비고 |
|------------------|-----|------|
| 인프라 (App/Shell/router/api/types) | ⚠️  | s28: vanilla pushState + 9-view discriminated union + Shared EventSource + requestJSON wrapper + Legacy URL alias. RF-072/073 (types.ts mirror, fetch URL ID-005 영향). |
| 23 components (~14k줄) | 🔍  | s28: 인벤토리 + 5 component (1000+줄) RF-071 분해. ID 결정 표면 (MemoryCenter ID-001 / Config ID-005 / ChatPanel ID-002). |
| 디자인 토큰 (app.css) | ➖  | 미검토 (CLAUDE.md 약속만 확인) |

## 기타

| 항목                  | 상태 | 비고 |
|----------------------|-----|------|
| Makefile             | ⬜  | 빌드 흐름 |
| .github/workflows/   | ⬜  | CI/릴리스 |
| config/standalone.yaml | ⬜  | 기본 config |
