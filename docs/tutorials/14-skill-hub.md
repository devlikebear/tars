# Step 14. Skill Hub (원격 설치)

> 학습 목표: 원격 registry에서 skill을 검색/설치하는 패키지 매니저 패턴

## 원본 코드 분석 (TARS)

TARS의 `internal/skillhub/` 패키지:

```
types.go     ← RegistryIndex, RegistryEntry, InstalledSkill 타입
registry.go  ← GitHub raw content에서 인덱스 조회, 검색, SKILL.md 다운로드
install.go   ← 설치/삭제/업데이트, skillhub.json DB 관리
mcp.go       ← MCP 패키지 매니페스트 파싱, checksum 검증
```

### 아키텍처

```
GitHub Repository (tars-skills)
├── registry.json        ← 스킬 인덱스 (이름, 설명, 버전, 태그)
├── skills/
│   ├── greeting/
│   │   └── SKILL.md
│   └── code-review/
│       └── SKILL.md
└── plugins/
    └── ...

    ↓ HTTP GET (raw.githubusercontent.com)

Local Workspace
├── skills/              ← 설치된 skill
│   └── greeting/
│       └── SKILL.md
└── skillhub.json        ← 설치 상태 DB
```

### 핵심 설계: 왜 GitHub Raw Content인가

- **인프라 불필요** — GitHub 리포지토리가 곧 registry
- **버전 관리** — git으로 skill 이력 추적
- **PR 기반 기여** — 누구나 PR로 skill 추가 가능
- **캐싱 무료** — GitHub CDN이 처리

OpenClaw의 ClawHub는 전용 웹사이트, npm은 전용 레지스트리가 필요하지만, TARS는 GitHub 하나로 충분합니다.

## 실습

### 14-1. Registry — 원격 인덱스 조회

**`internal/skillhub/hub.go`**

`registry.json` 형식:
```json
{
  "version": 1,
  "skills": [
    {
      "name": "greeting",
      "description": "친근한 인사 스킬",
      "version": "0.1.0",
      "author": "devlikebear",
      "tags": ["chat", "greeting"],
      "path": "skills/greeting",
      "user_invocable": true
    }
  ]
}
```

```go
type Registry struct {
    RegistryURL  string       // registry.json URL
    SkillBaseURL string       // SKILL.md 다운로드 베이스 URL
    HTTPClient   *http.Client // 30초 타임아웃
}

func (r *Registry) FetchIndex(ctx) (*RegistryIndex, error)
func (r *Registry) Search(ctx, query) ([]RegistryEntry, error)
func (r *Registry) FindByName(ctx, name) (*RegistryEntry, error)
func (r *Registry) FetchSkillContent(ctx, entry) ([]byte, error)
```

검색은 이름, 설명, 태그에 대한 **대소문자 무시 substring 매칭**. 매우 단순하지만 소규모 레지스트리에 충분합니다.

### 14-2. Installer — 설치/삭제/업데이트

```go
type Installer struct {
    WorkspaceDir string
    Registry     *Registry
}

func (inst *Installer) Install(ctx, name) error
func (inst *Installer) Uninstall(name) error
func (inst *Installer) List() ([]InstalledSkill, error)
func (inst *Installer) Update(ctx) ([]string, error)
```

설치 흐름:
```
Install("greeting")
  ↓ FindByName → RegistryEntry
  ↓ FetchSkillContent → SKILL.md 내용
  ↓ os.MkdirAll → .workspace/skills/greeting/
  ↓ os.WriteFile → SKILL.md 저장
  ↓ addToDB → skillhub.json 업데이트
```

TARS 원본은 companion files, plugin 의존성 체크, checksum 검증이 있지만, 최소 버전에서는 SKILL.md 하나만 다운로드.

### 14-3. 설치 상태 DB

**`skillhub.json`:**
```json
{
  "skills": [
    {
      "name": "greeting",
      "version": "0.1.0",
      "dir": ".workspace/skills/greeting"
    }
  ]
}
```

Update 시 registry 버전과 로컬 버전을 비교, 다르면 재다운로드.

### 14-4. CLI 서브커맨드

**`cmd/tars/skill_main.go`:**

```
tars skill search [query]     ← 레지스트리 검색
tars skill install <name>     ← 스킬 설치
tars skill uninstall <name>   ← 스킬 삭제
tars skill list               ← 설치된 스킬 목록
tars skill update             ← 전체 업데이트
```

각 커맨드는 `Installer`를 생성하고 해당 메서드를 호출하는 얇은 래퍼.

### 14-5. 설치 → 즉시 사용

skill이 `.workspace/skills/` 에 설치되고, 채팅 핸들러가 같은 경로를 읽으므로:

```
tars skill install greeting
→ .workspace/skills/greeting/SKILL.md 생성
→ 다음 채팅 요청에서 skill.Load() 가 자동 감지
→ 프롬프트에 주입 → LLM이 바로 사용
```

서버 재시작 없이 설치 즉시 사용 가능.

## 체크포인트

- [x] 원격에서 skill을 검색하고 설치할 수 있다
- [x] 설치된 skill이 즉시 채팅에서 사용 가능하다
- [x] `skillhub.json`으로 설치 상태를 추적한다

## 최종 구조 (Phase 4 완료)

```
tars/
├── internal/
│   ├── skill/
│   │   └── skill.go             ← Load, parseSkill, FormatAvailableSkills
│   ├── plugin/
│   │   └── plugin.go            ← Load, parseManifest, collectSkillDirs/MCPServers
│   ├── mcp/
│   │   └── client.go            ← Client, session, RPC (Content-Length + JSONLine)
│   ├── skillhub/
│   │   └── hub.go               ← Registry, Installer, InstalledDB
│   └── ...
├── cmd/tars/
│   ├── main.go                  ← + skill 커맨드 등록
│   └── skill_main.go            ← search/install/uninstall/list/update
└── docs/lessons/
    ├── 12-skill-loader.md
    ├── 13-plugin-mcp.md
    └── 14-skill-hub.md
```

## 배운 패턴

- **GitHub as Registry** — 별도 인프라 없이 raw content로 패키지 매니저 구현
- **설치 = 파일 복사** — SKILL.md를 다운로드해서 skills/ 디렉터리에 놓으면 끝
- **JSON DB** — `skillhub.json`으로 설치 상태 추적, 간단하지만 충분
- **런타임/배포 분리** — Step 12(런타임 로딩)와 Step 14(원격 배포)가 독립적 계층
- **즉시 사용** — 설치 경로 = 로딩 경로이므로 서버 재시작 불필요
