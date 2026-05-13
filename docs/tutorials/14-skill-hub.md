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

`registry.json`은 skills, plugins, mcp_servers, packs를 함께 담는다.

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
  "mcp_servers": [],
  "packs": [
    {
      "name": "github-maintainer-pack",
      "description": "GitHub maintainer workflow bundle",
      "version": "0.1.0",
      "author": "devlikebear",
      "tags": ["github", "maintenance"],
      "plugins": ["browser-devtools"],
      "mcp_servers": ["safe-time"],
      "skills": ["github-ops"]
    }
  ]
}
```

`files`는 legacy string 배열과 checksum-bearing object 배열을 모두 받아들인다. 최신 package는 companion file까지 함께 설치할 수 있도록 object form을 권장한다. `quality`는 설치 전 신뢰 신호다. `score`는 0-100 정수이고, last updated, tests passing, required tools, permissions, companion CLI, install count 같은 선택 필드를 함께 둘 수 있다. Console Extensions Hub는 이 값을 패키지 카드에 표시한다.

`packs`는 여러 package를 한 번에 설치하는 manifest다. Pack install plan은 plugins → MCP servers → skills 순서로 정렬되어 skill이 요구하는 plugin을 먼저 준비한다. `tars pack install <name>`은 계획을 출력하고 확인을 받은 뒤 각 member의 기존 sandbox install 경로를 재사용한다.

핵심 API:

```go
func (r *Registry) FetchIndex(ctx context.Context) (*RegistryIndex, error)
func (r *Registry) Search(ctx context.Context, query string) ([]RegistryEntry, error)
func (r *Registry) SearchPlugins(ctx context.Context, query string) ([]PluginEntry, error)
func (r *Registry) SearchMCPServers(ctx context.Context, query string) ([]MCPEntry, error)
func (r *Registry) SearchPacks(ctx context.Context, query string) ([]PackEntry, error)
func (r *Registry) FetchSkillFile(ctx context.Context, entry *RegistryEntry, relPath string) ([]byte, error)
```

검색은 이름, 설명, 태그에 대한 대소문자 무시 substring matching이다.

### 14-2. Installer

```go
type Installer struct {
    WorkspaceDir string
    Registry     *Registry
}

func (inst *Installer) Install(ctx, name) (*InstallResult, error)
func (inst *Installer) InstallPlugin(ctx, name) (*InstallResult, error)
func (inst *Installer) InstallMCP(ctx, name) (*InstallResult, error)
func (inst *Installer) PlanPackInstall(ctx, name) (PackInstallPlan, error)
func (inst *Installer) InstallPack(ctx, name) (*PackInstallResult, error)
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

tars pack search [query]
tars pack info <name>
tars pack install <name>
tars pack install <name> --yes
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
│       ├── source.go            # HubSource 인터페이스
│       ├── source_registry.go   # SourceRegistry + ResolveSkillRef
│       ├── source_tarshub.go    # 빌트인 tars-hub 어댑터
│       ├── attribution.go       # ATTRIBUTION.md 생성 + 라이선스 탐지
│       ├── dry_run.go           # PreviewInstall + DryRunResult
│       ├── search.go            # SearchAllSkills (federated)
│       ├── install.go           # InstallWithOptions, ConfirmFn, OnPreview
│       ├── types.go, mcp.go
│       └── sources/             # 외부 hub 어댑터
│           ├── openclaw/        # steipete/openclaw
│           ├── hermes/          # NousResearch/hermes-agent
│           └── anthropic/       # anthropics/skills (per-skill LICENSE.txt)
├── cmd/tars/
│   ├── skill_installer.go       # 외부 hub 자동 등록 wiring
│   ├── skill_main.go            # --from, --yes, --dry-run, --format
│   ├── plugin_main.go
│   └── mcp_main.go
└── docs/tutorials/
    ├── 12-skill-loader.md
    ├── 13-plugin-mcp.md
    └── 14-skill-hub.md
```

## Hub Federation (Phase 1~5, v0.32.43+)

`tars-hub` 외에 외부 hub에서도 skill을 import할 수 있다. CLI는 `--from <hub>` 플래그, 콘솔은 Extensions의 hub 셀렉터를 통해 선택한다.

### 지원 Hub

| Source ID | Repo | 라이선스 | 특이사항 |
|---|---|---|---|
| `tars-hub` | devlikebear/tars-skills | (자체) | manifest sha256 검증 |
| `openclaw` | steipete/openclaw | MIT | JSON-in-YAML metadata 파싱, `install[]` 블록은 **실행하지 않고** `metadata.adapter_warnings.install_blocks_skipped`로 보존 |
| `hermes` | NousResearch/hermes-agent | MIT | `skills/<category>/<name>/` 계층 구조, GitHub Trees API recursive 인덱싱 |
| `anthropic` | anthropics/skills | Apache-2.0 / Proprietary | **per-skill `LICENSE.txt`** 필수. docx/pdf/pptx/xlsx 같은 source-available skill은 attribution 단계에서 **차단** |

### 핵심 설계 — HubSource 인터페이스 + 옵션 capability

```go
type HubSource interface {
    ID() string
    SearchSkills(ctx, query) ([]RegistryEntry, error)
    FindSkillByName(ctx, name) (*RegistryEntry, error)
    FetchSkillContent(ctx, entry) ([]byte, error)
    FetchSkillFile(ctx, entry, relPath) ([]byte, error)
}

// 외부 hub만 구현하는 선택 인터페이스
type SkillContentConverter interface {  // JSON-in-YAML → TARS YAML 재작성
    ConvertSkillContent(entry, raw) ([]byte, []string, error)
}
type LicenseFetcher interface {         // ATTRIBUTION.md 본문 fetch
    FetchLicense(ctx, entry) ([]byte, string, error)
}
type CompanionFileLister interface {   // manifest 없는 hub의 file 자동 발견
    ListCompanionFiles(ctx, entry) ([]string, error)
}
```

`Installer`가 source ID로 라우팅한다. tars-hub는 기존 sha256 manifest 경로를 유지하고, 외부 hub는 다운로드 후 변환·라이선스 가져오기·ATTRIBUTION.md 생성을 거친다.

### Install 흐름 (외부 hub)

1. `ResolveSkillRef("openclaw:github")` → `(sourceID, bareName)`
2. `Installer.InstallWithOptions(ctx, ref, opts)` → `PreviewInstall`로 `DryRunResult` 생성 (각 파일 sha256 직접 계산)
3. `opts.OnPreview` 콜백으로 CLI/콘솔이 preview 렌더
4. `opts.DryRun=true` 또는 `opts.Confirm` 거부 → 머터리얼라이즈 skip
5. 승인 시 sandbox → materialize → ATTRIBUTION.md → DB에 `source` 기록
6. 사후 sha256 drift 감지 (preview와 다른 내용 → 거부)

### 라이선스 의무 자동화

`internal/skillhub/attribution.go`의 `BuildAttribution`이:
- MIT/Apache-2.0 → `ATTRIBUTION.md` 자동 생성 (원 저작권 + 라이선스 본문 + 원 URL/commit SHA)
- Apache-2.0 → NOTICE 섹션 추가 (NOTICE 파일 fetch 시도)
- Proprietary/Unknown → install 거부 ("refusing to materialize a proprietary skill")

### CLI 사용 예

```bash
tars skill search github                       # 4 hub 통합 검색
tars skill search --from hermes auth           # 특정 hub
tars skill install --from openclaw github --yes
tars skill install --from anthropic skill-creator --dry-run --format json
tars skill install --from anthropic docx --yes   # → 거부됨 (Proprietary)
```

## 배운 패턴

- **GitHub as Registry** — 별도 backend 없이 raw content로 package manager 구현
- **Package files as contract** — `files` 목록과 checksum으로 companion files를 안전하게 설치
- **Workspace-local DB** — 설치 상태는 repo가 아니라 사용자의 workspace에 남긴다
- **부분 실패 보고** — update 실패를 전체 성공처럼 숨기지 않는다
- **Federation as plug-in capability** — built-in 인터페이스에 외부 hub가 옵션 capability(`SkillContentConverter`/`LicenseFetcher`)로 끼워지므로 새 hub 추가가 한 파일짜리 작업
- **License compliance as code** — ATTRIBUTION.md를 install 단계에 강제 + Proprietary 차단으로 라이선스 실수를 컴파일러처럼 잡는다
- **Preview-then-confirm** — `--dry-run` + `OnPreview`로 외부 소스 신뢰 결정을 사용자에게 명시적으로 노출, 사후 sha256 drift도 차단
- **runtime reload** — 설치/삭제 후 extension snapshot을 재빌드해 콘솔과 chat에 반영한다
