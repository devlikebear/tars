---
description: "TARS 프로젝트 릴리즈 플로우. 코드 커밋 → PR → 머지 → VERSION.txt 범프 → CHANGELOG 작성 → release pipeline 트리거 → homebrew tap 갱신까지 전체 과정을 안내한다."
---
# TARS Release Flow

TARS 프로젝트의 개발 완료 후 릴리즈까지의 전체 플로우를 실행한다.

## 전제 조건
- `gh` CLI 인증 완료
- main 브랜치 보호 규칙: PR 필수, status check 필수
- `HOMEBREW_TAP_TOKEN` GitHub secret 설정 완료
- Release workflow: `.github/workflows/release-on-version-bump.yml`

## 릴리즈 플로우 (순서대로 실행)

### 1. 코드 변경 커밋 & PR 생성
```bash
# feature branch 생성
git checkout -b <branch-name>

# 변경사항 커밋
git add <files>
git commit -m "<type>: <description>"

# PR 생성
git push -u origin <branch-name>
gh pr create --title "<title>" --body "<body>"
```

### 2. CI 통과 확인 & 머지
```bash
gh pr checks <PR_NUMBER> --watch
gh pr merge <PR_NUMBER> --squash --admin
```

### 3. VERSION.txt 범프 + CHANGELOG.md 업데이트
**중요: VERSION.txt와 CHANGELOG.md는 반드시 같은 커밋에 포함해야 한다.**
Release workflow가 push 이벤트로 트리거될 때 CHANGELOG.md 변경을 검증한다.

```bash
git checkout main && git pull --rebase origin main
git checkout -b chore/release-v<NEW_VERSION>
```

VERSION.txt 수정:
```
<NEW_VERSION>
```

CHANGELOG.md 수정 (`## [Unreleased]` 아래에 추가):
```markdown
## [<NEW_VERSION>] - <YYYY-MM-DD>

### Fixed
- ...

### Added
- ...

### Changed
- ...
```

```bash
git add VERSION.txt CHANGELOG.md
git commit -m "chore: release v<NEW_VERSION>"
git push -u origin chore/release-v<NEW_VERSION>
gh pr create --title "chore: release v<NEW_VERSION>" --body "Release v<NEW_VERSION>"
gh pr checks <PR_NUMBER> --watch
gh pr merge <PR_NUMBER> --squash --admin
```

### 4. Release Pipeline 자동 트리거
PR 머지 시 `VERSION.txt` 변경이 main에 push되면 workflow가 자동 트리거된다.

만약 VERSION.txt가 이미 머지되어 있고 CHANGELOG만 추가한 경우:
```bash
gh workflow run release-on-version-bump.yml --ref main
```

### 5. 파이프라인 모니터링
```bash
gh run list --workflow=release-on-version-bump.yml --limit 3
gh run watch <RUN_ID>
```

파이프라인 단계:
1. `prepare-release` — VERSION.txt 검증, CHANGELOG.md 검증
2. `build-assets` — macOS arm64/amd64 바이너리 빌드, 아카이브 생성
3. `publish-release` — GitHub Release 생성, 아카이브/체크섬 첨부
4. `update-homebrew` — `devlikebear/homebrew-tap` 의 `Formula/tars.rb` 갱신

### 6. 릴리즈 확인
```bash
gh release view v<NEW_VERSION>
brew update && brew upgrade tars  # homebrew tap 반영 확인
```

## 주의사항

- **수동 태그/릴리즈 금지**: `git tag` + `gh release create`로 수동 생성하면 homebrew tap이 갱신되지 않는다. 반드시 VERSION.txt 파이프라인을 사용할 것.
- **VERSION.txt + CHANGELOG.md 동시 변경**: push 트리거 시 CHANGELOG 변경이 없으면 파이프라인이 실패한다. `workflow_dispatch`는 이 검증을 건너뛴다.
- **main 직접 push 불가**: branch protection 규칙으로 인해 PR을 통해서만 머지 가능.
- **브랜치 정리**: 머지 후 `git branch -d <branch>` 로 로컬 브랜치 정리.

## 빠른 실행 (한 줄 요약)

코드 PR 머지 완료 후:
```
branch → VERSION.txt + CHANGELOG.md 수정 → commit → push → PR → merge → pipeline 자동 실행 → 완료
```
