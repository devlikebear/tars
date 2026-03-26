# Step 12. Skill 로더

> 학습 목표: SKILL.md 기반 확장 포맷 이해, frontmatter 파싱, 프롬프트 주입 패턴

## 원본 코드 분석 (TARS)

TARS의 `internal/skill/` 패키지:

```
types.go        ← Definition, Snapshot, Source 타입
frontmatter.go  ← YAML frontmatter 파서 (라이브러리 없이 직접 구현)
loader.go       ← WalkDir로 SKILL.md 발견, 머지, 정렬
prompt.go       ← XML 형식 skill 목록 생성
mirror.go       ← 런타임 경로에 skill 복사
```

### 업계 비교: OpenClaw, Gemini CLI, TARS

| | OpenClaw | Gemini CLI | TARS |
|--|----------|------------|------|
| 포맷 | SKILL.md (YAML frontmatter) | SKILL.md (동일) | SKILL.md (동일) |
| 발견 | `skills/` 워킹 | `skills/` 워킹 | `skills/` 워킹 |
| 우선순위 | workspace > user > bundled | extension > project > global | workspace > user > bundled |
| 사용자 호출 | `user-invocable: true` → `/skill` | `activate_skill` 도구 | `user-invocable: true` → `/skill` |
| 게이팅 | `requires.bins`, `requires.env`, OS 필터 | `excludeTools` 설정 | 없음 |

**핵심:** SKILL.md 포맷은 사실상 업계 표준. 세 프로젝트 모두 Markdown + YAML frontmatter를 사용합니다.

### SKILL.md 포맷

```markdown
---
name: greeting
description: 사용자에게 인사하는 스킬
user-invocable: true
---
# Greeting Skill

사용자가 인사하면 친근하게 답변하세요.
```

`---` 사이가 frontmatter (YAML 메타데이터), 나머지가 content (LLM에 전달되는 지시).

## 실습

### 12-1. Definition 타입

```go
type Definition struct {
    Name          string `json:"name"`
    Description   string `json:"description"`
    UserInvocable bool   `json:"user_invocable"`
    FilePath      string `json:"file_path"`
    Content       string `json:"content,omitempty"`
}
```

TARS 원본의 `RecommendedTools`, `WakePhases` 등 고급 필드는 생략 — 최소 버전에 충분.

### 12-2. Load — SKILL.md 탐색

```go
func Load(dir string) ([]Definition, error) {
    filepath.WalkDir(dir, func(path string, d os.DirEntry, ...) error {
        if !strings.EqualFold(filepath.Base(path), "SKILL.md") {
            return nil
        }
        // 파일 읽기 → parseSkill → 목록에 추가
    })
}
```

포인트:
- `strings.EqualFold`로 대소문자 무시 매칭
- 파싱 실패는 skip (에러가 아님) — core를 깨지 않는 optional layer
- 결과를 이름순 정렬

### 12-3. Frontmatter 파싱

TARS는 YAML 라이브러리 없이 **직접 key: value 파싱**합니다:

```go
func parseSkill(path, raw string) (Definition, error) {
    if strings.HasPrefix(normalized, "---\n") {
        // "---" 사이의 메타 블록 추출
        // key: value 줄별 파싱 (name, description, user-invocable)
    }
    // frontmatter 없으면 전체를 content로 사용
    // 이름 없으면 디렉터리 이름에서 추론
    // 설명 없으면 첫 번째 텍스트 줄에서 추론
}
```

### 12-4. FormatAvailableSkills — 프롬프트 생성

```go
func FormatAvailableSkills(skills []Definition) string {
    // <available_skills>
    //   <skill>
    //     <name>greeting</name>
    //     <description>...</description>
    //     <content>실제 skill 본문</content>  ← 핵심!
    //   </skill>
    // </available_skills>
}
```

**중요:** `<content>` 블록에 skill 본문을 포함해야 LLM이 skill 지시를 따릅니다. 목록(이름+설명)만 주입하면 LLM은 "skill이 있다"는 것만 알고 내용은 모릅니다.

### 12-5. 프롬프트 빌더 연결

`BuildOptions`에 `Skills string` 필드 추가, 시스템 프롬프트 끝에 skill XML 주입.

채팅 핸들러에서:
```go
skills, _ := skill.Load(filepath.Join(h.workspaceDir, "skills"))
systemPrompt := prompt.Build(prompt.BuildOptions{
    WorkspaceDir: h.workspaceDir,
    Skills:       skill.FormatAvailableSkills(skills),
})
```

## 체크포인트

- [x] `skills/` 폴더에 SKILL.md를 넣으면 자동 인식
- [x] LLM이 skill 내용을 참고해 답변
- [x] skill이 없어도 기존 채팅이 정상 동작

## 배운 패턴

- **SKILL.md = 업계 표준** — OpenClaw, Gemini CLI, TARS 모두 같은 포맷
- **Frontmatter 직접 파싱** — YAML 라이브러리 없이 key: value로 충분
- **이름/설명 추론** — frontmatter가 없어도 디렉터리명, 첫 줄에서 유추
- **Content 주입 필수** — skill 목록만이 아니라 본문까지 프롬프트에 넣어야 동작
