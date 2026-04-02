# 모듈: 확장과 툴링

## 핵심 파일

- `internal/extensions/manager.go`
- `internal/extensions/lifecycle.go`
- `internal/skill/loader.go`
- `internal/skill/mirror.go`
- `internal/plugin/loader.go`
- `internal/plugin/builtin.go`
- `internal/plugin/builtin_registry.go`
- `internal/plugin/manifest.go`
- `internal/mcp/client.go`
- `internal/tool/tool.go`
- `internal/tarsserver/helpers_build_tools.go`
- `internal/agent/loop.go`
- `internal/llm/provider.go`
- `internal/prompt/builder.go`

## 역할

이 모듈은 "TARS가 왜 단순 채팅 앱이 아닌가"를 설명하는 부분이다. skill, plugin, built-in plugin, MCP 서버, 워크스페이스 문서, 툴 레지스트리, HTTP route, LLM 공급자 추상화가 여기서 한데 묶인다.

## 핵심 연결 관계

- `prompt.BuildResultFor`: 워크스페이스 문서를 시스템 프롬프트로 변환
- `extensions.Manager`: skill/plugin/MCP snapshot 생성
- `plugin.RegisterBuiltin`: compile-time Go plugin 등록
- `skill.MirrorToWorkspace`: agent가 읽을 runtime skill path 생성
- `plugin.Load`: plugin manifest를 읽고 skill dir, MCP server를 추출
- `tool.Registry`: 툴 정의를 LLM schema로 노출
- `agent.Loop`: LLM 응답의 tool call을 실제 코드로 실행

## 실행 시 모습

1. 서버가 확장 스냅샷을 읽는다.
2. built-in plugin init이 먼저 실행되고, plugin loader가 추가 skill dir와 MCP server를 안전하게 추출한다.
3. skill loader가 실제 skill 본문을 읽고 runtime mirror가 `_shared/skills_runtime`를 만든다.
4. plugin lifecycle hook이 선언돼 있으면 on_start/on_stop 명령을 best-effort로 실행한다.
5. 요청과 프로젝트 정책에 맞는 툴만 골라 schema를 만든다.
6. LLM이 tool call을 반환하면 `agent.Loop`가 registry에서 실행 대상을 찾는다.

## 설계상 장점

- 툴 구현과 툴 노출 정책이 분리되어 있다.
- skill/plugin/MCP는 reload 가능하도록 snapshot 기반으로 관리된다.
- built-in plugin도 manifest plugin과 비슷한 표면으로 합쳐져서 tool/HTTP route를 같은 경로로 노출할 수 있다.
- runtime skill path가 source path와 분리되어 companion file 접근이 더 안정적이다.
- MCP server build가 실패해도 startup 전체를 막지 않고 diagnostic으로만 남길 수 있다.
- plugin skill 경로를 plugin root 내부로 제한해 확장 경계가 비교적 명확하다.
