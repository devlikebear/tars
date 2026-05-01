# Step 14. Skill Hub (원격 설치)

> 학습 목표: GitHub raw registry에서 skill/plugin/MCP package를 검색, 설치, 업데이트하는 패키지 매니저 패턴

## 원본 코드 분석 (TARS)

TARS의 `internal/skillhub/` 패키지:

```
types.go     ← RegistryIndex, skill/plugin/MCP entry, installed DB, update result
registry.go  ← registry.json 조회, 검색, 파일 다운로드
install.go   ← skill/plugin/MCP 설치/삭제/업데이트, skillhub.json 관리
mcp.go       ← tars.mcp.json 파싱, ${MCP_DIR} 확장, checksum 검증
```

### 아키텍처

```
GitHub Repository (devlikebear/tars-skills)
├── registry.json
├── skills/
│   └── daily-briefing/
│       ├── SKILL.md
│       └── briefing.sh
├── plugins/
│   └── ...
└── mcp-servers/
    └── safe-time/
        └── tars.mcp.json

    ↓ HTTP GET (raw.githubusercontent.com)

Local Workspace
├── skills/
├── plugins/
├── mcp-servers/
└── skillhub.json
```

### 핵심 설계: GitHub Raw Content

- 별도 registry server 없이 GitHub repository가 source of truth가 된다.
- package 파일은 git history와 PR review를 그대로 활용한다.
- 설치 상태는 workspace-local `skillhub.json`에 남긴다.
- update 결과는 updated/skipped/failed를 나눠 보고해 부분 실패를 숨기지 않는다.

## 실습

### 14-1. Registry index

`registry.json`은 skills, plugins, mcp_servers를 함께 담는다.

```json
{
  "version": 3,
  "skills": [
    {
      "name": "daily-briefing",
      "description": "Create a short daily briefing",
      "version": "0.1.0",
      "author": "devlikebear",
      "tags": ["planning"],
      "path": "skills/daily-briefing",
      "user_invocable": true,
      "files": [
        {"path": "SKILL.md", "sha256": "..."},
        {"path": "briefing.sh", "sha256": "..."}
      ],
      "quality": {
        "score": 88,
        "last_updated": "2026-05-01",
        "tests_passing": true,
        "required_tools": ["bash"],
        "permissions": ["filesystem", "shell"],
        "companion_cli": true
      }
    }
  ],
  "plugins": [],
  "mcp_servers": []
}
```

`files`는 legacy string 배열과 checksum-bearing object 배열을 모두 받아들인다. 최신 package는 companion file까지 함께 설치할 수 있도록 object form을 권장한다. `quality`는 설치 전 신뢰 신호다. `score`는 0-100 정수이고, last updated, tests passing, required tools, permissions, companion CLI, install count 같은 선택 필드를 함께 둘 수 있다. Console Extensions Hub는 이 값을 패키지 카드에 표시한다.

핵심 API:

```go
func (r *Registry) FetchIndex(ctx context.Context) (*RegistryIndex, error)
func (r *Registry) Search(ctx context.Context, query string) ([]RegistryEntry, error)
func (r *Registry) SearchPlugins(ctx context.Context, query string) ([]PluginEntry, error)
func (r *Registry) SearchMCPServers(ctx context.Context, query string) ([]MCPEntry, error)
func (r *Registry) FetchSkillFile(ctx context.Context, entry *RegistryEntry, relPath string) ([]byte, error)
```

검색은 이름, 설명, 태그에 대한 대소문자 무시 substring matching이다.

### 14-2. Installer

```go
type Installer struct {
    WorkspaceDir string
    Registry     *Registry
}

func (inst *Installer) Install(ctx, name) (InstallResult, error)
func (inst *Installer) InstallPlugin(ctx, name) error
func (inst *Installer) InstallMCP(ctx, name) error
func (inst *Installer) Update(ctx) (UpdateResult, error)
func (inst *Installer) UpdatePlugins(ctx) (UpdateResult, error)
func (inst *Installer) UpdateMCPs(ctx) (UpdateResult, error)
```

설치 흐름:

```
Install("daily-briefing")
  ↓ Registry.FindByName
  ↓ listed files 다운로드
  ↓ checksum 검증(있을 때)
  ↓ workspace/skills/daily-briefing/ 에 파일 기록
  ↓ workspace/skillhub.json 업데이트
```

Skill이 `requires_plugin`을 선언했는데 workspace에 해당 plugin이 없으면 설치 자체는 가능하지만, CLI/API는 어떤 plugin을 추가 설치해야 하는지 경고한다.

### 14-3. 설치 상태 DB

`workspace/skillhub.json`:

```json
{
  "skills": [
    {
      "name": "daily-briefing",
      "version": "0.1.0",
      "source": "tars-hub",
      "dir": "workspace/skills/daily-briefing"
    }
  ],
  "plugins": [],
  "mcps": []
}
```

Update는 registry version과 installed version을 비교해 필요한 package만 다시 설치하고, skipped/failed diagnostic을 함께 반환한다.

### 14-4. CLI 서브커맨드

```
tars skill search [query]
tars skill install <name>
tars skill uninstall <name>
tars skill list
tars skill update
tars skill info <name>

tars plugin search [query]
tars plugin install <name>
tars plugin uninstall <name>
tars plugin list
tars plugin update
tars plugin info <name>

tars mcp search [query]
tars mcp install <name>
tars mcp uninstall <name>
tars mcp list
tars mcp update
tars mcp info <name>
```

서버가 떠 있는 상태에서 콘솔/API로 설치하면 extension manager reload를 호출해 새 skill/MCP 상태를 바로 반영한다.

### 14-5. MCP package

MCP package는 `tars.mcp.json`을 포함한다.

```json
{
  "schema_version": 1,
  "server": {
    "name": "safe-time",
    "command": "node",
    "args": ["${MCP_DIR}/server.js"]
  }
}
```

설치 후 `${MCP_DIR}`은 실제 package directory로 확장된다. local stdio command는 여전히 `extensions.mcp.command_allowlist`를 통과해야 실행된다.

## 체크포인트

- [x] 원격 registry에서 skill/plugin/MCP package를 검색할 수 있다
- [x] companion file과 checksum metadata를 보존해 설치한다
- [x] 설치 상태를 `workspace/skillhub.json`에 추적한다
- [x] update가 updated/skipped/failed를 구분한다
- [x] 설치 후 extension manager reload로 runtime snapshot이 갱신된다

## 최종 구조

```
tars/
├── internal/
│   ├── skill/
│   ├── plugin/
│   ├── mcp/
│   ├── extensions/
│   └── skillhub/
│       ├── types.go
│       ├── registry.go
│       ├── install.go
│       └── mcp.go
├── cmd/tars/
│   ├── skill_main.go
│   ├── plugin_main.go
│   └── mcp_main.go
└── docs/tutorials/
    ├── 12-skill-loader.md
    ├── 13-plugin-mcp.md
    └── 14-skill-hub.md
```

## 배운 패턴

- **GitHub as Registry** — 별도 backend 없이 raw content로 package manager 구현
- **Package files as contract** — `files` 목록과 checksum으로 companion files를 안전하게 설치
- **Workspace-local DB** — 설치 상태는 repo가 아니라 사용자의 workspace에 남긴다
- **부분 실패 보고** — update 실패를 전체 성공처럼 숨기지 않는다
- **runtime reload** — 설치/삭제 후 extension snapshot을 재빌드해 콘솔과 chat에 반영한다
