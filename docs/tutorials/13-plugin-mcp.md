# Step 13. Plugin 로더 + MCP 클라이언트

> 학습 목표: JSON 매니페스트 기반 플러그인 시스템과 MCP (Model Context Protocol) stdio 통신 구현

## 원본 코드 분석 (TARS)

### Plugin 시스템

```
internal/plugin/
├── types.go     ← Manifest, Definition, Snapshot
├── manifest.go  ← JSON 파싱, 서버 검증
└── loader.go    ← WalkDir, 머지, skill dir/MCP 수집
```

### MCP 클라이언트

```
internal/mcp/
├── client.go              ← pooledSession, 프로세스 관리, 세션 풀링
├── protocol_transport.go  ← RPC 프로토콜 (Content-Length + JSONLine)
└── client_api.go          ← ListTools, BuildTools, CallTool
```

### 업계 비교: Plugin 시스템

| | OpenClaw | Gemini CLI | TARS |
|--|----------|------------|------|
| 매니페스트 | `openclaw.plugin.json` | `gemini-extension.json` | `tars.plugin.json` |
| 런타임 | **In-process** TypeScript 모듈 | Content-only (MCP 위임) | Content-only (MCP 위임) |
| capability | provider, channel, tool 등 7종 | MCP tools, commands | skills, mcp_servers |
| 배포 | npm 패키지 | GitHub URL | 로컬 디렉터리 |

OpenClaw은 런타임 플러그인(코드 실행), Gemini CLI/TARS는 선언형 플러그인(설정만 선언).

### 업계 비교: MCP 클라이언트

| | OpenClaw | Gemini CLI | TARS |
|--|----------|------------|------|
| 트랜스포트 | stdio, SSE, HTTP, WS | stdio, SSE, Streamable HTTP | stdio (Content-Length + JSONLine) |
| 네이밍 | `mcp__plugin_server__tool` | `mcp_{server}_{tool}` | `mcp.{server}.{tool}` |
| RPC 모드 | SDK 표준 | SDK 표준 | **자체 구현** (두 모드 지원) |

TARS는 MCP SDK 없이 자체 RPC를 구현 — Content-Length/JSONLine 모드 전환과 서버별 자동 감지.

## 실습

### 13-A. Plugin 로더

**`internal/plugin/plugin.go`**

매니페스트 (`tars.plugin.json`) 예시:
```json
{
  "id": "demo",
  "name": "Demo Plugin",
  "skills": ["skills"],
  "mcp_servers": [
    {"name": "echo", "command": "node", "args": ["/path/to/server.js"]}
  ]
}
```

핵심 흐름:
```go
func Load(dir string) (Snapshot, error) {
    // 1. WalkDir로 tars.plugin.json 탐색
    // 2. JSON 파싱, ID 검증
    // 3. ID 기준 중복 제거 (나중 것이 이김)
    // 4. collectSkillDirs — 플러그인 제공 skill 디렉터리 수집
    // 5. collectMCPServers — 플러그인 선언 MCP 서버 수집
}
```

포인트:
- `MCPServer` 타입을 plugin 패키지에 직접 정의 — config 의존 없이 독립적
- 파싱 실패는 skip — graceful degradation

### 13-B. MCP 클라이언트

**`internal/mcp/client.go`**

MCP는 JSON-RPC 2.0 기반 프로토콜입니다. 두 가지 전송 모드:

#### Content-Length 모드 (기본)
```
→ Content-Length: 155\r\n\r\n{"jsonrpc":"2.0","id":1,...}
← Content-Length: 200\r\n\r\n{"jsonrpc":"2.0","id":1,"result":{...}}
```

#### JSONLine 모드 (sequential-thinking 등)
```
→ {"jsonrpc":"2.0","id":1,...}\n
← {"jsonrpc":"2.0","id":1,"result":{...}}\n
```

#### 세션 라이프사이클

```
getOrStart(server)     ← 프로세스 시작, stdin/stdout 파이프
  ↓
initialize(ctx, sess)  ← "initialize" RPC → 서버 capabilities 확인
  ↓                       "notifications/initialized" 전송
buildToolsForServer()  ← "tools/list" RPC → 도구 목록 수집
  ↓
callTool()             ← "tools/call" RPC → 실행 시간에 도구 호출
```

#### RPC 모드 자동 감지

```go
func inferRPCMode(server plugin.MCPServer) rpcMode {
    // sequential-thinking → JSONLine
    // 나머지 → Content-Length
}
```

TARS 원본은 Content-Length 실패 시 JSONLine으로 자동 폴백하는 기능도 있습니다.

#### Notification 건너뛰기

MCP 서버는 응답 외에 notification을 보낼 수 있습니다. 응답 읽기 루프에서 요청 ID와 일치하는 메시지만 반환:

```go
go func() {
    for {
        d, e := readMessage(sess.reader, sess.mode)
        var resp rpcResponse
        json.Unmarshal(d, &resp)
        if resp.ID == id {  // 요청 ID 매칭
            ch <- readResult{d, nil}
            return
        }
        // notification은 건너뛰기
    }
}()
```

### 13-C. 서버 연결

```go
// server.go에서
pluginSnap, _ := plugin.Load(filepath.Join(cfg.WorkspaceDir, "plugins"))

if len(pluginSnap.MCPServers) > 0 {
    mcpClient = mcp.NewClient(pluginSnap.MCPServers)
    mcpTools, _ := mcpClient.BuildTools(ctx)
    for _, t := range mcpTools {
        registry.Register(t)  // MCP 도구를 기존 registry에 등록
    }
}
```

MCP 도구는 기존 내장 도구와 동일한 `tool.Tool` 인터페이스. LLM은 차이를 모릅니다.

## 체크포인트

- [x] MCP 서버의 도구가 채팅에서 사용 가능하다
- [x] MCP 서버가 실패해도 나머지 기능은 정상 동작한다
- [x] JSONLine/Content-Length 두 모드를 지원한다

## 배운 패턴

- **선언형 플러그인** — JSON 매니페스트로 skill과 MCP만 선언, 런타임 코드 없음
- **두 가지 RPC 모드** — Content-Length (HTTP 스타일) vs JSONLine (스트림 스타일)
- **자동 모드 감지** — 서버 이름/인자로 RPC 모드 추론
- **Notification 건너뛰기** — 요청 ID 매칭으로 비동기 메시지 필터링
- **Graceful degradation** — MCP 실패가 전체 서버를 막지 않음
- **단일 write** — 헤더+바디를 한 번에 전송해야 파이프 전달이 확실
