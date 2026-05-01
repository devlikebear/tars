# Step 6. 설정 시스템

> 학습 목표: 하드코딩된 값을 설정 파일과 환경 변수로 외부화하는 3단계 설정 로딩

## 원본 코드 분석 (TARS)

TARS의 `internal/config/` 패키지:

```
load.go                 ← 설정 로딩 엔진 (defaults < YAML < env)
config_input_fields.go  ← 필드 테이블 (이름, 환경변수, 기본값 매핑)
defaults.go             ← 기본값 정의
yaml_paths.go           ← flat field와 nested YAML path 매핑
llm_providers_field.go  ← provider pool JSON/YAML 파싱
llm_resolve.go          ← provider alias + tier binding 해석
```

### 핵심 설계 포인트

**1. 3단계 우선순위: defaults < YAML < env**

```
Default()      → workspace/auth/runtime 기본값
    ↓
config/default.yaml + 사용자 YAML  (override)
    ↓
환경 변수      → TARS_*  (최종 override)
```

가장 구체적인 설정이 우선합니다. 환경 변수는 배포 환경(Docker, CI)에서 코드 변경 없이 설정을 바꿀 때 유용합니다.

**2. YAML 파일이 없으면 에러가 아님**

```go
func loadYAML(path string, cfg *Config) error {
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return nil  // 파일 없으면 조용히 건너뜀
    }
}
```

Phase 1의 Session과 같은 패턴 — "없으면 기본값 사용"은 에러가 아닙니다.

**3. 환경 변수 네이밍 컨벤션**

접두사 `TARS_` + 대문자 필드명: `TARS_API_AUTH_MODE`, `TARS_LLM_PROVIDERS_JSON`, `TARS_LLM_TIERS_JSON`

접두사가 있어야 다른 애플리케이션의 환경 변수와 충돌하지 않습니다.

**4. LLM provider pool**

최신 TARS는 `provider/model/api_key` flat field가 아니라 provider pool을 씁니다.

```yaml
llm:
  providers:
    codex:
      kind: openai-codex
      auth_mode: oauth
      oauth_provider: openai-codex
  tiers:
    heavy: { provider: codex, model: gpt-5.4, reasoning_effort: high }
    standard: { provider: codex, model: gpt-5.4, reasoning_effort: medium }
    light: { provider: codex, model: gpt-5.4, reasoning_effort: minimal }
  default_tier: standard
```

Provider는 "어디에 어떻게 인증해서 호출할지"를, tier는 "어떤 provider alias와 model을 쓸지"를 맡습니다.

## 실습

### 6-1. Config 구조체와 기본값

학습용 최소 버전은 flat field로 시작해도 되지만, 원본 TARS의 현재 구조는 여러 embedded config struct를 `Config`에 합성한다.

**학습용 최소 구조**

```go
type Config struct {
    WorkspaceDir   string `yaml:"workspace_dir"`
    APIAuthMode    string `yaml:"api_auth_mode"`
    LLMProviders   map[string]LLMProviderSettings `yaml:"llm_providers"`
    LLMTiers       map[string]LLMTierBinding      `yaml:"llm_tiers"`
    LLMDefaultTier string `yaml:"llm_default_tier"`
}

func Default() Config {
    return Config{
        WorkspaceDir:   ".workspace",
        APIAuthMode:    "required",
        LLMProviders:   map[string]LLMProviderSettings{},
        LLMTiers:       map[string]LLMTierBinding{},
        LLMDefaultTier: "standard",
    }
}
```

`yaml` 태그를 붙이면 `yaml.Unmarshal`이 자동으로 매핑합니다.

### 6-2. Load 함수 — 3단계 합성

```go
func Load(path string) (Config, error) {
    cfg := Default()              // 1. 기본값
    if path != "" {
        loadYAML(path, &cfg)      // 2. YAML (있으면 덮어씀)
    }
    applyEnv(&cfg)                // 3. 환경 변수 (최종 override)
    return cfg, nil
}
```

### 6-3. 환경 변수 적용

```go
func applyEnv(cfg *Config) {
    if v := envStr("TARS_WORKSPACE_DIR"); v != "" {
        cfg.WorkspaceDir = v
    }
    if v := envStr("TARS_API_AUTH_MODE"); v != "" {
        cfg.APIAuthMode = strings.ToLower(strings.TrimSpace(v))
    }
    if v := envStr("TARS_LLM_PROVIDERS_JSON"); v != "" {
        cfg.LLMProviders = parseProviderJSON(v)
    }
    // ... 나머지 필드도 동일 패턴
}
```

포인트:
- 빈 문자열이면 override하지 않음 (기본값 유지)
- 복합 map/list 필드는 JSON 환경 변수 하나로 override한다

### 6-4. server_main.go 변경 — Config 통합

기존에 플래그로 받던 값들을 Config로 교체:

```go
func newServeCommand(stdout, stderr io.Writer) *cobra.Command {
    configPath := ""
    cmd := &cobra.Command{
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg, err := config.Load(configPath)
            return runServe(cmd.Context(), cfg, stdout, stderr)
        },
    }
    cmd.Flags().StringVar(&configPath, "config", "", "path to config file")
    return cmd
}

func buildLLMRouter(cfg config.Config) (llm.Router, error) {
    resolved, err := config.ResolveAllLLMTiers(&cfg)
    if err != nil {
        return nil, err
    }
    tiers := buildTierEntries(resolved)
    defaultTier, err := llm.ParseTier(cfg.LLMDefaultTier)
    if err != nil {
        return nil, err
    }
    return llm.NewRouter(llm.RouterConfig{
        Tiers:       tiers,
        DefaultTier: defaultTier,
    })
}
```

원본 TARS는 이 위치에서 client를 직접 하나만 만들지 않고, `config.ResolveAllLLMTiers`로 heavy/standard/light tier를 해석한 뒤 `internal/llm.Router`를 구성한다.

## 테스트

```bash
# 기본값으로 실행
go run ./cmd/tars/ serve

# API listen 주소는 serve flag로 변경
go run ./cmd/tars/ serve --api-addr 127.0.0.1:43181

# YAML 설정 파일
cat > config.yaml <<'EOF'
llm:
  providers:
    default:
      kind: openai
      auth_mode: api-key
      api_key: ${OPENAI_API_KEY}
  tiers:
    heavy: { provider: default, model: gpt-5.4, reasoning_effort: high }
    standard: { provider: default, model: gpt-5.4, reasoning_effort: medium }
    light: { provider: default, model: gpt-5.4, reasoning_effort: minimal }
  default_tier: standard
EOF

go run ./cmd/tars/ serve --config config.yaml
```

## 체크포인트

- [x] `config.yaml`로 provider pool과 tier를 변경할 수 있다
- [x] `serve --api-addr`로 API listen 주소를 변경할 수 있다
- [x] 환경 변수가 YAML 값을 override한다
- [x] 설정 파일이 없어도 기본값으로 정상 동작한다

## 최종 구조 (Phase 2 추가분)

```
tars/
├── internal/
│   ├── config/
│   │   ├── defaults.go         ← Config 기본값
│   │   ├── load.go             ← Load()
│   │   ├── config_input_fields.go
│   │   └── llm_resolve.go      ← provider pool/tier 해석
│   └── ...
└── cmd/tars/
    └── server_main.go          ← config.Load() → tarsserver.Serve()
```

## 배운 패턴

- **3단계 설정 합성** — defaults < YAML < env, 가장 구체적인 것이 우선
- **없으면 기본값** — YAML 파일이 없어도 에러가 아님 (graceful degradation)
- **접두사 환경 변수** — `TARS_` 접두사로 네임스페이스 충돌 방지
- **`yaml` 태그** — 구조체 필드를 YAML 키에 자동 매핑
