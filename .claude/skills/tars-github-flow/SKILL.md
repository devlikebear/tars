---
name: tars-github-flow
description: "TARS 프로젝트 전용 GitHub Flow. 모든 작업을 git worktree 기반 feature branch에서 수행. 사용 시점: feat/fix/chore 구현, PR 생성, 릴리스 작업 등 코드를 main에 머지하는 전체 라이프사이클이 필요할 때. 워크트리 생성 → TDD 구현 → 커밋 → 푸시 → PR → CI 확인 → 머지 → 워크트리 정리 순서로 진행."
---

# TARS GitHub Flow

## 핵심 원칙

- **모든 작업은 git worktree에서 수행.** main에서 직접 작업하지 않는다.
- `gh` CLI로 issue/PR/merge/release 작업.
- conventional commits: `feat:`, `fix:`, `chore:`.
- PR 머지 전 `security`, `test` CI 통과 필수.

## Workflow

### 1. Plan (비자명한 작업 시)

- `EnterPlanMode` → Explore/Plan 에이전트로 코드베이스 탐색 → 플랜 파일 작성 → `ExitPlanMode`로 사용자 승인
- 단순 작업은 스킵 가능

### 2. Worktree 생성

```bash
git fetch origin && git switch main && git pull --rebase
git worktree add .claude/worktrees/<branch-name> -b <branch-name> main
```

- 브랜치명: `feat/<name>`, `fix/<name>`, `chore/<name>` (lowercase kebab-case)
- worktree 경로에서 모든 코드 변경 수행

### 3. TDD 개발

- 테스트 먼저 작성 → 최소 구현 → `make test` 통과 확인
- `make vet` + `make fmt` 로 lint clean 확인
- 변경한 파일만 명시적으로 `git add` (절대 `git add .` 사용 금지)
- `make fmt`가 관련없는 파일도 포맷팅할 수 있으므로, 변경한 파일만 선별 stage

### 4. Commit & Push

```bash
git add <specific-files>
git commit -m "feat: 설명"
git push -u origin <branch-name>
```

- subject: 명령형, 72자 이하
- `Closes #N`으로 이슈 참조

### 5. PR 생성

```bash
gh pr create --title "feat: 설명" --body "$(cat <<'EOF'
## Summary
- 변경 요약

## Test plan
- [x] make test
- [x] make vet
EOF
)"
```

### 6. CI 확인 & Merge

```bash
gh pr checks <number> --watch
gh pr merge <number> --squash --admin
```

- `--delete-branch` 사용 금지 (worktree와 충돌)

### 7. Worktree 정리 & Main 동기화

```bash
rm -rf .claude/worktrees/<branch-name>
git worktree prune
git fetch origin && git switch main && git pull --rebase
```

- `git worktree remove`가 실패하면 `rm -rf`로 강제 삭제 후 `git worktree prune`

## 릴리스 포함 시 (tars-release 스킬 연계)

머지 후 릴리스가 필요하면:

1. 새 worktree: `chore/release-v<VERSION>`
2. `VERSION.txt` 범프 + `CHANGELOG.md` 갱신 (같은 커밋)
3. commit → push → PR → CI → merge
4. `release-on-version-bump` 워크플로우 자동 트리거
5. `gh run watch` 으로 파이프라인 모니터링
6. worktree 정리

## 금지 사항

- main에서 직접 커밋/push 금지
- dirty worktree 상태에서 PR 생성 금지
- `--no-verify` 사용 금지 (사용자 명시 승인 없이)
- `--delete-branch` 사용 금지 (worktree 충돌 방지)
- 자동 릴리스 워크플로우를 수동 `git tag` + `gh release create`로 대체 금지
