# 모듈: 인증과 플러그인 로딩

## 핵심 파일

- `internal/tarsserver/middleware.go`
- `internal/serverauth/middleware.go`
- `internal/plugin/loader.go`
- `internal/plugin/builtin.go`
- `internal/plugin/builtin_registry.go`
- `internal/plugin/manifest.go`
- `internal/plugin/types.go`

## 역할

이 모듈은 HTTP 요청이 어떤 권한으로 실행되는지 결정하고, 외부 확장인 plugin과 내장 Go plugin이 어떤 skill/MCP/tool/HTTP route를 노출할 수 있는지 안전하게 정리하는 경계 계층이다.

## 인증 흐름

`internal/tarsserver/middleware.go`는 서버 쪽 진입 파일이고, 실제 판정 로직은 `internal/serverauth/middleware.go`에 있다.

- skip path는 인증 없이 통과한다.
- `required` 모드는 항상 토큰을 요구한다.
- `external-required` 모드는 loopback 요청만 토큰 없이 허용한다.
- admin path는 일반 사용자 토큰으로 접근할 수 없다.

토큰 비교는 SHA-256 해시와 constant-time compare로 처리한다. 역할은 `user`, `admin` 두 개이며, legacy bearer token은 admin으로 간주된다.

## Workspace 문맥

이 인증 레이어는 단순 역할 체크만 하지 않는다. 요청 헤더에서 workspace ID를 읽어 context에 넣고, 필요하면 기본 workspace를 주입한다.

따라서 이후 핸들러는 "누가 요청했는가"뿐 아니라 "어느 workspace에 대한 요청인가"도 함께 받는다.

## Plugin 로딩 흐름

`internal/plugin/loader.go`는 plugin manifest를 모두 읽어 snapshot으로 합친다. `internal/plugin/builtin_registry.go`는 코드에 컴파일된 built-in plugin을 별도 registry로 관리한다.

- manifest 파일명은 `tars.plugin.json`이 우선이다.
- plugin ID 기준으로 중복을 합친다.
- plugin이 선언한 skill 디렉터리는 plugin root 내부에 있어야 한다.
- plugin이 선언한 MCP 서버는 이름 기준으로 dedupe된다.
- built-in plugin은 manifest 파일이 아니라 Go 코드의 `init()`에서 등록되지만, 이후에는 같은 plugin 정의 표면으로 합쳐진다.

## 초보자가 놓치기 쉬운 점

- 인증 정책은 handler 내부가 아니라 middleware 단계에서 거의 끝난다.
- plugin은 skill 자체를 직접 담지 않고, skill directory와 MCP server 선언을 통해 확장을 연결한다.
- console/dashboards 공개 여부도 결국 같은 auth middleware 결정에 걸린다.
- plugin path validation이 있기 때문에 임의의 외부 경로를 skill dir로 노출할 수 없다.

## 디버깅 포인트

- 인증 실패: `requirementForRequest`, `resolveRole`, `apiAdminPaths`
- workspace 바인딩 문제: `WithWorkspaceID`, `withDefaultWorkspaceBinding`
- plugin 누락: `parseManifestFile`, `collectSkillDirs`, `collectMCPServers`
