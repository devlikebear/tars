# Step 21. 인증 미들웨어

> 학습 목표: Bearer token 기반 인증 미들웨어를 구현하고, user/admin role과 경로별 접근 제어를 적용

## 왜 인증이 필요한가

로컬 개발에서는 인증을 끄고 쓰는 편이 편합니다. 하지만 TARS 서버를 외부에서 접근 가능하게 두면 다음 위험이 생깁니다.

- 외부에서 LLM API 호출을 악용할 수 있음
- `write_file`, `exec` 같은 고위험 도구가 서버 파일 시스템을 조작할 수 있음
- Agent Runtime run, Telegram webhook, config/admin API를 무단으로 조작할 수 있음

TARS는 단순한 Bearer token 인증을 쓰되, token을 user/admin role로 나누고 admin path를 별도로 보호합니다.

## 인증 모드

| Mode | 의미 |
|------|------|
| `required` | 모든 보호 API에 token 필요 |
| `external-required` | 외부 요청은 token 필요, loopback 요청은 일부 경로 완화 |
| `off` | 인증 끔. 내부적으로 admin role을 부여하므로 로컬 개발에서만 사용 |

`api_auth_mode=off` 또는 `external-required`는 명시적으로 `api_allow_insecure_local_auth=true`를 켜야 사용할 수 있습니다. 이 opt-in은 로컬 개발 편의를 의도한 장치이며, 외부 공개 서버의 보안 완화로 쓰면 안 됩니다.

## 보안 원칙

### 원문 저장 금지

토큰 원문을 비교하지 않고 SHA-256 해시로 비교합니다.

```go
presentedHash := sha256.Sum256([]byte(presentedToken))
```

### Timing attack 방지

일반 문자열 비교 대신 constant-time 비교를 사용합니다.

```go
if subtle.ConstantTimeCompare(adminHash[:], presentedHash[:]) == 1 {
    return RoleAdmin
}
```

일반 `==` 비교는 앞에서부터 틀린 바이트에서 빠르게 끝날 수 있습니다. `crypto/subtle.ConstantTimeCompare`는 같은 길이 입력에 대해 전체 바이트를 비교합니다.

## 미들웨어 구조

**`internal/tarsserverauth/middleware.go`**

```go
type Options struct {
    Mode                          string
    BearerToken                   string
    UserToken                     string
    AdminToken                    string
    WorkspaceHeader               string
    RequireWorkspaceForAuthorized bool
    UserWorkspaceAllowlist        []string
    AdminWorkspaceAllowlist       []string
    SkipPaths                     []string
    LoopbackSkipPaths             []string
    AdminPaths                    []string
}
```

핵심:

- `BearerToken`: legacy 단일 토큰. 호환성을 위해 admin role로 해석됩니다.
- `UserToken`: 일반 사용자 API 접근.
- `AdminToken`: admin/config/reload/webhook 같은 고위험 API 접근.
- `SkipPaths`: 인증 없이 통과하는 경로.
- `LoopbackSkipPaths`: loopback 요청일 때만 통과하는 경로.
- `AdminPaths`: user token으로는 접근할 수 없는 경로.

## 요청 판정

```go
func (c compiledOptions) requirementForRequest(r *http.Request) requestRequirement {
    if c.skipPaths.match(r.URL.Path) {
        return requestRequirement{skip: true}
    }
    if c.loopbackSkipPaths.match(r.URL.Path) && isLoopbackRemoteAddr(r.RemoteAddr) {
        return requestRequirement{skip: true}
    }
    requireToken := c.mode == ModeRequired
    if c.mode == ModeExternalRequired && !isLoopbackRemoteAddr(r.RemoteAddr) {
        requireToken = true
    }
    isAdminPath := c.adminPaths.match(r.URL.Path)
    return requestRequirement{
        requireToken: requireToken,
        isAdminPath:  isAdminPath,
        tokenNeeded:  requireToken || isAdminPath,
    }
}
```

판정 순서:

1. skip path면 즉시 통과
2. loopback-only skip path면 loopback 요청만 통과
3. mode에 따라 token 필요 여부 결정
4. admin path면 mode와 무관하게 admin token 필요

## Role 결정

```go
func resolveTokenRole(presentedToken string, hasLegacyToken, hasUserToken, hasAdminToken bool, legacyHash, userHash, adminHash [32]byte) string {
    presentedHash := sha256.Sum256([]byte(presentedToken))
    if hasAdminToken && subtle.ConstantTimeCompare(adminHash[:], presentedHash[:]) == 1 {
        return RoleAdmin
    }
    if hasUserToken && subtle.ConstantTimeCompare(userHash[:], presentedHash[:]) == 1 {
        return RoleUser
    }
    if hasLegacyToken && subtle.ConstantTimeCompare(legacyHash[:], presentedHash[:]) == 1 {
        return RoleAdmin
    }
    return ""
}
```

admin token을 먼저 검사합니다. legacy token은 기존 사용자 호환을 위해 admin으로 취급됩니다.

## TARS 연결

**`internal/tarsserver/middleware.go`**

```go
auth := serverauth.NewMiddleware(serverauth.Options{
    Mode:              cfg.APIAuthMode,
    BearerToken:       cfg.APIAuthToken,
    UserToken:         cfg.APIUserToken,
    AdminToken:        cfg.APIAdminToken,
    SkipPaths:         apiAuthSkipPaths(cfg),
    LoopbackSkipPaths: dashboardLoopbackSkipPaths(cfg),
    AdminPaths:        apiAdminPaths(),
}, authLog)
```

기본 skip/admin 경로:

```go
func apiAuthSkipPaths(cfg config.Config) []string {
    return []string{"/v1/healthz", "/", "/console", "/console/", "/console/*"}
}

func apiAdminPaths() []string {
    return []string{
        "/v1/admin/*",
        "/v1/runtime/extensions/reload",
        "/v1/agentruntime/reload",
        "/v1/agentruntime/restart",
        "/v1/terminal/*",
        "/v1/channels/webhook/inbound/*",
        "/v1/channels/telegram/webhook/*",
        "/v1/channels/telegram/pairings*",
    }
}
```

`dashboard_auth_mode`는 이름이 남아 있는 legacy config field이지만, 현재 의미는 `/console` shell의 loopback-only 완화입니다. `dashboard_auth_mode=off`는 콘솔을 외부에 공개한다는 뜻이 아니며, loopback 요청에서만 `/`와 `/console/*` 경로를 우회합니다.

## 설정 예시

```yaml
api_auth_mode: required
api_user_token: ${TARS_API_USER_TOKEN}
api_admin_token: ${TARS_API_ADMIN_TOKEN}
```

```bash
export TARS_API_USER_TOKEN="user-token"
export TARS_API_ADMIN_TOKEN="admin-token"
tars serve
```

API client는 `Authorization: Bearer <token>` 헤더를 보냅니다. CLI에서는 `--api-token`, `--admin-api-token`, `TARS_API_TOKEN`, `TARS_ADMIN_API_TOKEN`을 사용합니다.

로컬 개발에서만 인증 완화를 쓰려면 두 값을 함께 설정합니다.

```yaml
api_auth_mode: external-required
api_allow_insecure_local_auth: true
```

## 체크포인트

- [x] `required` 모드에서 token 없는 보호 API 요청은 401
- [x] user token은 일반 API에 접근 가능
- [x] admin path는 admin token 또는 legacy token만 접근 가능
- [x] `/v1/healthz`와 콘솔 정적 경로는 skip
- [x] `dashboard_auth_mode=off`는 `/`와 `/console/*`의 loopback-only skip으로 제한
- [x] `/v1/terminal/*`, extension reload, agent runtime reload/restart, webhook/pairing 경로는 admin path로 보호
- [x] token 비교는 SHA-256 + constant-time compare

## 다음 단계

Step 22에서는 장시간 작업을 HTTP 요청 생명주기와 분리하는 Agent Runtime을 다룹니다.
