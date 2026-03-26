# Step 6. 설정 시스템

> 학습 목표: 하드코딩된 값을 설정 파일과 환경 변수로 외부화하는 3단계 설정 로딩

## 원본 코드 분석 (TARS)

TARS의 `internal/config/` 패키지:

```
load.go                 ← 설정 로딩 엔진 (defaults < YAML < env)
config_input_fields.go  ← 필드 테이블 (이름, 환경변수, 기본값 매핑)
defaults.go             ← 기본값 정의
```

### 핵심 설계 포인트

**1. 3단계 우선순위: defaults < YAML < env**

```
Default()      → port: 8080, provider: "mock"
    ↓
YAML 파일      → port: 3000  (override)
    ↓
환경 변수      → MYCLAW_PORT=9000  (최종 override)
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

접두사 `MYCLAW_` + 대문자 필드명: `MYCLAW_PORT`, `MYCLAW_API_KEY`, `MYCLAW_PROVIDER`

접두사가 있어야 다른 애플리케이션의 환경 변수와 충돌하지 않습니다.

## 실습

### 6-1. Config 구조체와 기본값

**`internal/config/config.go`**

```go
type Config struct {
    Port         int    `yaml:"port"`
    WorkspaceDir string `yaml:"workspace_dir"`
    Provider     string `yaml:"provider"`
    Model        string `yaml:"model"`
    APIKey       string `yaml:"api_key"`
    BaseURL      string `yaml:"base_url"`
}

func Default() Config {
    return Config{
        Port:         8080,
        WorkspaceDir: ".workspace",
        Provider:     "mock",
        Model:        "gpt-5.4-nano",
        BaseURL:      "https://api.openai.com/v1",
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
    if v := envStr("MYCLAW_PROVIDER"); v != "" {
        cfg.Provider = v
    }
    if v := envStr("MYCLAW_PORT"); v != "" {
        if port, err := strconv.Atoi(v); err == nil {
            cfg.Port = port
        }
    }
    // ... 나머지 필드도 동일 패턴
}
```

포인트:
- 빈 문자열이면 override하지 않음 (기본값 유지)
- `Port`는 `strconv.Atoi`로 변환 — 환경 변수는 항상 문자열

### 6-4. serve.go 변경 — Config 통합

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

func buildLLMClient(cfg config.Config) (llm.Client, error) {
    switch strings.ToLower(cfg.Provider) {
    case "openai":
        return llm.NewOpenAIClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
    case "mock", "":
        return llm.NewMockClient(), nil
    default:
        return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
    }
}
```

## 테스트

```bash
# 기본값으로 실행 (mock provider)
go run ./cmd/tars/ serve

# 환경 변수로 포트 변경
MYCLAW_PORT=3000 go run ./cmd/tars/ serve

# YAML 설정 파일
cat > config.yaml <<'EOF'
port: 3000
provider: openai
model: gpt-4o-mini
api_key: sk-...
EOF

go run ./cmd/tars/ serve --config config.yaml
```

## 체크포인트

- [x] `config.yaml`로 provider, model, port를 변경할 수 있다
- [x] 환경 변수가 YAML 값을 override한다
- [x] 설정 파일이 없어도 기본값으로 정상 동작한다

## 최종 구조 (Phase 2 추가분)

```
tars/
├── internal/
│   ├── config/
│   │   └── config.go           ← Config 구조체 + Load() + Default()
│   └── ...
└── cmd/tars/
    └── serve.go                ← config.Load() → buildLLMClient() → server.Serve()
```

## 배운 패턴

- **3단계 설정 합성** — defaults < YAML < env, 가장 구체적인 것이 우선
- **없으면 기본값** — YAML 파일이 없어도 에러가 아님 (graceful degradation)
- **접두사 환경 변수** — `MYCLAW_` 접두사로 네임스페이스 충돌 방지
- **`yaml` 태그** — 구조체 필드를 YAML 키에 자동 매핑
