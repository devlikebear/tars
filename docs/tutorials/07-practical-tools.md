# Step 7. 실용 도구 추가

> 학습 목표: LLM이 실제로 활용할 수 있는 파일 읽기/쓰기/명령 실행 도구 구현과 보안 고려사항

## 원본 코드 분석 (TARS)

TARS의 `internal/tool/` 디렉터리에는 다양한 도구가 있습니다:

```
read_file.go    ← 파일 읽기 (줄 번호, 오프셋, 제한)
write_file.go   ← 파일 쓰기 (디렉터리 자동 생성)
exec.go         ← 셸 명령 실행 (보안 필터링, 타임아웃)
web_fetch.go    ← URL 가져오기
list_files.go   ← glob 패턴으로 파일 목록
search_files.go ← 파일 내용 검색
```

### 핵심 설계 포인트

**1. 워크스페이스 경계**

모든 도구는 `workspaceDir`을 기준으로 동작합니다. 상대 경로를 받아서 `filepath.Join(workspaceDir, path)`로 절대 경로를 만듭니다. 이렇게 하면 LLM이 워크스페이스 밖의 파일에 접근하는 것을 방지합니다.

**2. 출력 크기 제한**

LLM의 컨텍스트 윈도우는 유한합니다. 파일이 수 MB일 수 있으므로 출력을 제한합니다:
- `read_file`: 8KB 초과 시 잘라냄
- `exec`: stdout 8KB, stderr 2KB 제한

참고로 실제 AI agent들의 파일 읽기 제한:
- **Gemini CLI**: 2,000줄 (20MB 초과 시 거부)
- **Codex**: 2,000줄 (줄당 500자 제한)
- **Claude Code**: 2,000줄 (~25K 토큰)

**3. 위험한 명령어 차단**

```go
var blockedCommands = map[string]struct{}{
    "sudo": {}, "rm": {}, "shutdown": {}, "reboot": {},
    "halt": {}, "poweroff": {}, "mkfs": {}, "dd": {},
}
```

LLM이 실수로 위험한 명령을 실행하는 것을 방지합니다. 최소 버전에서는 명령어 이름만 체크하지만, 원본에는 더 정교한 필터가 있습니다.

**4. `IsError: true` vs `error` 반환**

| | `return Result{}, err` | `return Result{IsError: true}, nil` |
|--|--|--|
| 의미 | 시스템 장애 | 도구는 실행됐지만 결과가 실패 |
| 예시 | 메모리 부족, 패닉 | 파일 없음, 잘못된 인자, 차단된 명령 |
| LLM에게 | 에러를 노출하지 않을 수도 있음 | "실패했어, 다시 해봐" 전달 |

실용 도구에서는 대부분 `IsError: true`를 사용합니다. LLM에게 실패 이유를 알려줘서 수정 기회를 줍니다.

## 실습

### 7-1. read_file 도구

**`internal/tool/read_file.go`**

```go
func NewReadFileTool(workspaceDir string) Tool {
    return Tool{
        Name:        "read_file",
        Description: "Read a text file from the workspace",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "File path relative to workspace"}
            },
            "required": ["path"]
        }`),
        Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
            // 1. 인자 파싱
            // 2. 절대 경로 구성: filepath.Join(workspaceDir, filepath.Clean(path))
            // 3. 파일 읽기 (없으면 IsError)
            // 4. 8KB 초과 시 잘라냄
        },
    }
}
```

`filepath.Clean()`은 `../` 같은 경로 조작을 정리합니다.

### 7-2. write_file 도구

**`internal/tool/write_file.go`**

```go
func NewWriteFileTool(workspaceDir string) Tool {
    Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
        // 1. path, content 파싱
        // 2. 부모 디렉터리 자동 생성: os.MkdirAll(filepath.Dir(absPath), 0o755)
        // 3. 파일 쓰기: os.WriteFile(absPath, content, 0o644)
        // 4. 결과: "wrote N bytes to path"
    }
}
```

`MkdirAll`이 핵심입니다 — LLM이 새 디렉터리에 파일을 쓸 때 별도로 `mkdir` 명령을 실행할 필요가 없습니다.

### 7-3. exec 도구

**`internal/tool/exec.go`**

보안 3계층:

```go
// 1. 차단된 명령어 확인
if _, blocked := blockedCommands[fields[0]]; blocked {
    return Result{Content: "blocked: " + fields[0], IsError: true}, nil
}

// 2. 타임아웃 10초
runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()

// 3. 출력 크기 제한
out := truncate(stdout.String(), 8192)
```

`exec.CommandContext`를 쓰면 타임아웃 시 프로세스가 자동 kill됩니다.

### 7-4. 서버에 도구 등록

**`internal/server/server.go`** 변경:

```go
registry := tool.NewRegistry()
registry.Register(tool.NewEchoTool())
registry.Register(tool.NewCurrentTimeTool())
registry.Register(tool.NewReadFileTool(cfg.WorkspaceDir))   // 추가
registry.Register(tool.NewWriteFileTool(cfg.WorkspaceDir))  // 추가
registry.Register(tool.NewExecTool(cfg.WorkspaceDir))       // 추가
```

도구 생성자가 `workspaceDir`을 받아서 클로저에 캡처합니다. 이 패턴 덕분에 도구 구현체가 전역 상태 없이 동작합니다.

## 테스트

```bash
# 서버 시작
go run ./cmd/tars/ serve

# 파일 읽기 (도구 실행 확인)
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"go.mod 파일 읽어줘"}'

# 명령 실행
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"현재 디렉터리의 파일 목록을 보여줘"}'
```

## 체크포인트

- [x] LLM이 도구를 선택하고 실행 결과를 활용한다
- [x] 위험한 명령어가 차단된다
- [x] 파일 출력이 8KB로 제한된다
- [x] 부모 디렉터리가 자동 생성된다

## 배운 패턴

- **워크스페이스 경계** — 모든 경로를 `workspaceDir` 기준으로 제한
- **출력 크기 제한** — 컨텍스트 윈도우 보호와 토큰 비용 절약
- **차단 목록** — 위험한 명령어를 사전 차단 (화이트리스트보다 구현이 쉬움)
- **클로저 패턴** — 도구 생성자가 `workspaceDir`을 캡처해서 전역 상태 없이 동작
- **`IsError: true`** — 도구 실패를 LLM에게 전달해서 재시도 기회 제공
