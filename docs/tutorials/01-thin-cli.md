# Step 1. 얇은 CLI 만들기

> 학습 목표: 엔트리포인트는 비즈니스 로직을 갖지 않는다는 원칙 이해

## 원본 코드 분석 (TARS)

TARS의 `cmd/tars/main.go`를 보면 `main()` 함수가 하는 일은 딱 세 가지입니다:

```go
func main() {
    bootstrapEnv()           // .env 파일 로딩
    runOnMainThread(func() { // 플랫폼 shim
        newRootCommand().Execute()  // Cobra 명령 실행
    })
    os.Exit(exitCode)
}
```

비즈니스 로직은 전부 `internal/` 패키지에 있고, `cmd/` 는 플래그 파싱과 내부 함수 호출만 합니다.

### 핵심 설계 원칙 3가지

1. **`main.go`에 비즈니스 로직 없음** — env 로딩, cobra 명령 연결, `os.Exit`만
2. **각 서브커맨드는 "옵션 구조체 → 내부 패키지 호출"** — `server_main.go`는 `serveOptions`를 만들어 `tarsserver.Serve()`에 넘기기만 함
3. **플랫폼 차이는 빌드 태그로 분리** — `mainthread_darwin.go` vs `mainthread_other.go`

## 실습

### 1-1. 프로젝트 초기화

```bash
mkdir -p tars
cd tars
go mod init github.com/devlikebear/tars
go get github.com/spf13/cobra
```

### 1-2. buildinfo 패키지

빌드 시 `-ldflags`로 버전 정보를 주입하는 패턴입니다.

**`internal/buildinfo/buildinfo.go`**

```go
package buildinfo

import "fmt"

var (
    Version = "dev"
    Commit  = "unknown"
    Date    = "unknown"
)

func Summary() string {
    return fmt.Sprintf("tars %s (%s, %s)", Version, Commit, Date)
}
```

`var`로 선언하면 빌드 시 `go build -ldflags "-X .../buildinfo.Version=1.0.0"` 처럼 외부에서 값을 주입할 수 있습니다.

### 1-3. main.go

```go
// cmd/tars/main.go
func main() {
    if err := newRootCommand(os.Stdout).Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func newRootCommand(stdout io.Writer) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "tars",
        Short: "Minimal clone of TARS",
        RunE: func(cmd *cobra.Command, args []string) error {
            return cmd.Help()
        },
    }
    cmd.AddCommand(newVersionCommand(stdout))
    return cmd
}
```

**포인트:**
- `newRootCommand()`가 `io.Writer`를 받는 이유 → 테스트할 때 stdout을 바꿔 끼울 수 있음
- 기본 동작은 `cmd.Help()` — 이후 웹 콘솔(`/console`)로 발전

### 1-4. serve 서브커맨드

원본 패턴을 따라 **파일을 분리**합니다.

```go
// cmd/tars/serve.go
type serveOptions struct {
    port    int
    verbose bool
}

func newServeCommand(stdout, stderr io.Writer) *cobra.Command {
    opts := serveOptions{port: 8080}
    cmd := &cobra.Command{
        Use:   "serve",
        Short: "Run tars server",
        RunE: func(cmd *cobra.Command, args []string) error {
            return runServe(cmd.Context(), opts, stdout, stderr)
        },
    }
    cmd.Flags().IntVar(&opts.port, "port", opts.port, "listen port")
    cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "verbose logging")
    return cmd
}
```

## 확인

```bash
go run ./cmd/tars/              # help 출력
go run ./cmd/tars/ version      # tars dev (unknown, unknown)
go run ./cmd/tars/ serve        # starting tars server on :8080
go run ./cmd/tars/ serve --port 3000  # starting tars server on :3000
```

## 체크포인트

- [x] 엔트리포인트(`main.go`)가 비즈니스 로직을 갖지 않는다
- [x] `version`, `serve` 서브커맨드가 동작한다
- [x] 서버 옵션이 플래그로 받아진다

## 최종 구조

```
tars/
├── cmd/tars/
│   ├── main.go          ← 엔트리포인트 (로직 없음)
│   └── serve.go         ← 옵션 구조체 + 내부 위임
├── internal/
│   └── buildinfo/
│       └── buildinfo.go ← 빌드 메타데이터
└── go.mod
```
