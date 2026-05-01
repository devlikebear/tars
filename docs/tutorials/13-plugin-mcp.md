# Step 13. Plugin 로더 + MCP 클라이언트

> 학습 목표: JSON 매니페스트 기반 확장 패키지와 MCP(Model Context Protocol) 서버를 core prompt surface 바깥에서 연결하는 구조 이해

## 원본 코드 분석 (TARS)

### Plugin 시스템

```
internal/plugin/
├── types.go     ← Manifest, Definition, Snapshot, source priority
├── manifest.go  ← JSON 파싱, schema/version/legacy lifecycle 검증
└── loader.go    ← source merge, availability gate, skill dir/MCP 수집
```

TARS의 plugin은 런타임 Go 코드를 주입하는 시스템이 아닙니다. 현재 활성 경로는:

- plugin manifest가 skill directory를 선언한다.
- plugin manifest가 MCP server를 선언할 수 있다.
- plugin-declared MCP server는 `extensions.plugins.allow_mcp_servers=true`일 때만 활성화된다.
- schema v3의 `tools_provider`는 `mcp_server`만 실질적으로 인정되고, `go_plugin`/`script`는 diagnostic으로 안내된다.
- plugin HTTP route surface는 제거되어 활성 라우트로 등록되지 않는다.

### MCP 클라이언트

```
internal/mcp/
├── client.go              ← pooled session, local process / remote transport 관리
├── protocol_transport.go  ← stdio JSON-RPC framing(Content-Length + JSONLine)
├── remote_transport.go    ← streamable HTTP/SSE/WebSocket 계열 remote transport
└── client_api.go          ← ListTools, BuildTools, CallTool, MCPToolName
```

MCP server는 세 경로에서 합쳐집니다.

1. config의 `extensions.mcp.servers`
2. plugin manifest의 `mcp_servers` (명시 opt-in 필요)
3. Hub로 설치한 `workspace/mcp-servers/*/tars.mcp.json`

## 업계 비교

| | OpenClaw | Gemini CLI | TARS |
|--|----------|------------|------|
| 매니페스트 | `openclaw.plugin.json` | `gemini-extension.json` | `tars.plugin.json` |
| 런타임 | In-process TypeScript 모듈 | Content/MCP 위임 | Skill/MCP 위임 |
| capability | provider, channel, tool 등 | MCP tools, commands | skill dirs, gated MCP servers |
| core code injection | 가능 | 없음 | 없음 |
| MCP naming | `mcp__plugin_server__tool` | `mcp_{server}_{tool}` | `mcp.<server>.<tool>` |

TARS는 domain-specific 기능을 core Go tool로 늘리는 대신, skill + companion CLI 또는 MCP server로 분리하는 쪽을 기본값으로 둔다.

## 실습

### 13-A. Plugin manifest 파싱

매니페스트 예시:

```json
{
  "schema_version": 2,
  "id": "release-helper",
  "name": "Release Helper",
  "skills": ["skills"],
  "requires": {
    "bins": ["git"],
    "env": ["GITHUB_TOKEN"]
  },
  "mcp_servers": [
    {"name": "release-tools", "transport": "stdio", "command": "node", "args": ["server.js"]}
  ]
}
```

핵심 흐름:

```go
func Load(opts LoadOptions) (Snapshot, error) {
    // 1. source를 priority 순서로 정렬한다: bundled < user < workspace
    // 2. 각 source에서 tars.plugin.json / legacy manifest를 찾는다
    // 3. manifest를 parse/validate하고 id 기준으로 merge한다
    // 4. availability gate(requires/os/arch)를 통과한 plugin만 남긴다
    // 5. skill directories와 MCP declarations를 수집한다
}
```

포인트:

- `schema_version` 생략은 `2`로 처리된다.
- v1/v2 manifest에 v3 field가 있으면 거부한다.
- legacy shell lifecycle string은 보안상 거부한다.
- plugin source priority는 workspace > user > bundled다.

### 13-B. Extensions manager 연결

`internal/extensions.Manager`가 plugin, skill, MCP를 한 번에 조립합니다.

```go
plugins, _ := plugin.Load(plugin.LoadOptions{Sources: pluginSources})
skillSources := mergeSkillSources(baseSkillSources, plugins.Plugins, plugins.SkillDirs)
skills, _ := skill.Load(skill.LoadOptions{
    Sources: skillSources,
    Availability: skill.AvailabilityOptions{
        InstalledPlugins: pluginIDs(plugins.Plugins),
    },
})

pluginMCPServers := []config.MCPServer{}
if opts.PluginsAllowMCPServers {
    pluginMCPServers = plugins.MCPServers
}
hubMCPServers, _ := skillhub.LoadInstalledMCPServers(opts.WorkspaceDir)
mcpServers, _ := mergeMCPServers(configServers, pluginMCPServers, hubMCPServers)
```

Plugin skill directories는 skill loader에 들어가고, MCP servers는 runtime MCP client가 tool로 빌드합니다.

### 13-C. MCP tool 이름과 호출

MCP tool 이름은 server/tool 이름을 sanitize해서 만듭니다.

```go
func MCPToolName(serverName, toolName string) string {
    return "mcp." + sanitizeToken(serverName) + "." + sanitizeToken(toolName)
}
```

예를 들어 server `Release Tools`, tool `create/pr`는 `mcp.release_tools.create_pr`처럼 등록됩니다.

실행 흐름:

```
BuildTools(ctx)
  → server별 tools/list
  → tool.Tool wrapper 생성
  → chat registry에 등록

CallTool(ctx, name, input)
  → mcp.<server>.<tool> 이름에서 server/tool resolve
  → tools/call JSON-RPC 요청
  → tool.Result로 변환
```

### 13-D. stdio framing

stdio MCP 서버는 두 가지 framing을 지원합니다.

#### Content-Length 모드

```text
→ Content-Length: 155\r\n\r\n{"jsonrpc":"2.0","id":1,...}
← Content-Length: 200\r\n\r\n{"jsonrpc":"2.0","id":1,"result":{...}}
```

#### JSONLine 모드

```text
→ {"jsonrpc":"2.0","id":1,...}\n
← {"jsonrpc":"2.0","id":1,"result":{...}}\n
```

응답 읽기 루프는 request id가 맞는 response만 반환하고 notification은 건너뜁니다.

## 체크포인트

- [x] plugin manifest에서 skill directory를 수집한다
- [x] plugin MCP server는 opt-in일 때만 merge된다
- [x] hub MCP package와 config MCP server가 같은 runtime으로 합쳐진다
- [x] stdio Content-Length / JSONLine framing을 지원한다
- [x] remote MCP transport는 command allowlist와 분리된다

## 배운 패턴

- **선언형 plugin** — Go 코드를 주입하지 않고 skill/MCP만 선언한다
- **source priority merge** — workspace > user > bundled로 예측 가능한 override 제공
- **explicit opt-in** — plugin manifest가 local process를 띄울 수 있는 MCP는 기본 비활성화
- **MCP as boundary** — third-party tool surface는 core prompt/tool schema 대신 외부 protocol에 둔다
- **graceful degradation** — plugin/MCP 실패는 diagnostic으로 남기고 서버 전체를 막지 않는다
