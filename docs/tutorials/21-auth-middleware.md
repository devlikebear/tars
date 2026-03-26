# Step 21. 인증 미들웨어

> 학습 목표: Bearer token 기반 인증 미들웨어를 구현하고, 경로별 접근 제어를 적용

## 왜 인증이 필요한가

Phase 1-6까지 TARS 서버는 **누구나 접근 가능**했습니다. 로컬 개발에서는 괜찮지만, 운영 환경에서는:

- 외부에서 LLM API 호출을 악용할 수 있음 (비용 발생)
- `write_file`, `exec` 도구로 서버 파일 시스템을 조작할 수 있음
- Autopilot을 무단으로 시작/중단할 수 있음

**Bearer token 인증**은 가장 단순하면서 효과적인 API 인증 방식입니다.

## Bearer Token 인증 흐름

```
클라이언트                             서버
    │                                   │
    │ GET /v1/chat                      │
    │ Authorization: Bearer <token>     │
    │ ─────────────────────────────────→│
    │                                   │ token 검증
    │                     200 OK        │
    │ ←─────────────────────────────────│
    │                                   │
    │ GET /v1/chat (토큰 없음)           │
    │ ─────────────────────────────────→│
    │                                   │ 401 Unauthorized
    │ ←─────────────────────────────────│
```

## 실습

### 21-1. 보안 원칙: 토큰 비교

토큰을 비교할 때 두 가지 보안 원칙을 지켜야 합니다:

**1. 원문 저장 금지 — SHA-256 해싱**

```go
tokenHash := sha256.Sum256([]byte(token))
```

메모리에 토큰 원문을 들고 있으면 메모리 덤프에 노출될 수 있습니다. 서버 시작 시 해시만 저장합니다.

**2. Timing attack 방지 — constant-time 비교**

```go
// ❌ 위험: 일반 비교 (앞에서부터 틀린 바이트에서 즉시 반환)
if providedToken == expectedToken { ... }

// ✅ 안전: constant-time 비교 (항상 전체를 비교)
providedHash := sha256.Sum256([]byte(provided))
if subtle.ConstantTimeCompare(providedHash[:], tokenHash[:]) != 1 {
    // 거부
}
```

일반 `==` 비교는 첫 번째 다른 바이트에서 즉시 반환합니다. 공격자가 응답 시간을 측정하면 한 바이트씩 토큰을 추론할 수 있습니다. `crypto/subtle.ConstantTimeCompare`는 입력에 관계없이 항상 동일한 시간이 걸립니다.

### 21-2. 미들웨어 구조

**`internal/auth/middleware.go`**

```go
type Options struct {
    Mode       string   // off, required
    Token      string   // Bearer token 원문
    SkipPaths  []string // 인증 건너뛰는 경로 (예: /health)
    AdminPaths []string // admin 전용 경로 (예: /v1/admin/*)
}

type middleware struct {
    mode       string
    tokenHash  [32]byte   // SHA-256 해시만 저장
    hasToken   bool
    skipPaths  pathMatcher
    adminPaths pathMatcher
}
```

핵심 설계:
- `Options`는 외부 설정 (원문 포함)
- `middleware`는 내부 상태 (해시만 보유)
- 생성 시점에 해싱하고, 원문은 GC에 의해 회수됨

### 21-3. 인증 체크 로직

```go
func (m *middleware) check(r *http.Request) *authError {
    path := r.URL.Path

    // 1. skip paths → 무조건 통과
    if m.skipPaths.match(path) {
        return nil
    }

    // 2. mode off → 통과
    if m.mode == ModeOff {
        return nil
    }

    // 3. 토큰 미설정 + required → 서버 설정 오류
    if !m.hasToken {
        return &authError{Status: 401, Code: "no_token_configured", ...}
    }

    // 4. Authorization 헤더 파싱
    provided := parseBearerToken(r.Header.Get("Authorization"))
    if provided == "" {
        return &authError{Status: 401, Code: "missing_token", ...}
    }

    // 5. constant-time 비교
    providedHash := sha256.Sum256([]byte(provided))
    if subtle.ConstantTimeCompare(providedHash[:], m.tokenHash[:]) != 1 {
        return &authError{Status: 401, Code: "invalid_token", ...}
    }

    return nil
}
```

순서가 중요합니다:
1. **skip paths 먼저** — `/health` 같은 헬스체크는 인증 없이 통과
2. **mode 체크** — off면 전체 통과
3. **토큰 설정 여부** — required인데 토큰이 없으면 서버 설정 문제
4. **헤더 파싱** — `Bearer ` 접두사 제거, case-insensitive
5. **비교** — SHA-256 + constant-time

### 21-4. 경로 매칭 (Path Matcher)

```go
type pathMatcher struct {
    exact    map[string]struct{}  // /health → 정확히 일치
    prefixes []string             // /public/* → 접두사 매칭
}
```

`compilePaths`가 경로를 분류합니다:
- `*`로 끝나면 → prefix 매칭 (`/public/*` → `/public/docs/readme` 매칭)
- 그 외 → 정확한 일치 (`/health` → `/health`만 매칭)

`strings.CutSuffix`를 사용하면 `HasSuffix` + `TrimSuffix`를 한 번에 처리할 수 있습니다:

```go
if prefix, ok := strings.CutSuffix(p, "*"); ok {
    m.prefixes = append(m.prefixes, prefix)
} else {
    m.exact[p] = struct{}{}
}
```

### 21-5. Bearer 파싱

```go
func parseBearerToken(header string) string {
    if len(header) < 7 {
        return ""
    }
    if !strings.EqualFold(header[:7], "bearer ") {
        return ""
    }
    return strings.TrimSpace(header[7:])
}
```

`strings.EqualFold`로 case-insensitive 비교합니다. RFC 6750에 따르면 "Bearer"는 case-insensitive입니다.

### 21-6. 에러 응답

```go
func writeAuthError(w http.ResponseWriter, e *authError) {
    if e.Status == http.StatusUnauthorized {
        w.Header().Set("WWW-Authenticate", "Bearer")
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(e.Status)
    json.NewEncoder(w).Encode(map[string]string{
        "error": e.Message,
        "code":  e.Code,
    })
}
```

401 응답에는 반드시 `WWW-Authenticate: Bearer` 헤더를 포함해야 합니다 (RFC 6750).

### 21-7. 서버 연결

**설정 (config.go)**

```go
type Config struct {
    // ...
    AuthMode  string `yaml:"auth_mode"`  // off, required (default: off)
    AuthToken string `yaml:"auth_token"` // Bearer token
}
```

환경 변수로도 설정 가능:

```bash
MYCLAW_AUTH_MODE=required MYCLAW_AUTH_TOKEN=my-secret-token tars serve
```

**미들웨어 적용 (server.go)**

```go
authOpts := auth.Options{
    Mode:      cfg.AuthMode,
    Token:     cfg.AuthToken,
    SkipPaths: []string{"/health"},
}
handler := auth.NewMiddleware(authOpts, mux)
```

`/health`는 로드밸런서/모니터링이 인증 없이 사용하므로 skip 경로에 등록합니다.

### 21-8. 테스트

```go
func TestModeOff(t *testing.T) {
    h := NewMiddleware(Options{Mode: ModeOff}, okHandler())
    req := httptest.NewRequest("GET", "/v1/chat", nil)
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    if rec.Code != 200 { t.Fatalf("expected 200, got %d", rec.Code) }
}

func TestRequiredWithValidToken(t *testing.T) {
    h := NewMiddleware(Options{Mode: ModeRequired, Token: "secret"}, okHandler())
    req := httptest.NewRequest("GET", "/v1/chat", nil)
    req.Header.Set("Authorization", "Bearer secret")
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    if rec.Code != 200 { t.Fatalf("expected 200, got %d", rec.Code) }
}
```

테스트할 시나리오:
1. **mode off** → 모든 요청 통과
2. **required + 토큰 없음** → 401
3. **required + 유효 토큰** → 200
4. **required + 잘못된 토큰** → 401
5. **skip path** → 인증 없이 통과
6. **wildcard skip** → 접두사 매칭 통과
7. **Bearer 대소문자** → case-insensitive
8. **토큰 미설정 + required** → 401

## TARS 원본과의 차이

| 항목 | TARS | TARS |
|------|------|--------|
| 토큰 종류 | Bearer + User + Admin (3종) | Bearer 1종 |
| 인증 모드 | off, required, external-required | off, required |
| 로깅 | zerolog | fmt 출력 |
| loopback 감지 | 127.0.0.1 자동 통과 | 미구현 |

TARS는 최소 버전으로, 단일 토큰만 사용합니다. 향후 user/admin 토큰 분리는 `check()` 메서드의 6번 단계에서 확장할 수 있습니다.

## 체크포인트

- [x] `MYCLAW_AUTH_MODE=required` 설정 시 토큰 없는 요청이 401로 거부된다
- [x] 유효한 Bearer 토큰을 보내면 200으로 통과한다
- [x] `/health` 경로는 인증 없이 접근 가능하다
- [x] 8개 테스트 케이스가 모두 통과한다

## 다음 단계

Step 22에서는 Gateway (비동기 실행)를 구현합니다. 장시간 실행되는 작업을 백그라운드에서 처리하고, 실행 상태를 관리하는 시스템입니다.
