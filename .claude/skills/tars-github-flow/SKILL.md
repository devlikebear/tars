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
- 상세 절차는 [references/repo-flow.md](references/repo-flow.md) 참조.

## Workflow

1. **Plan** - 작업 범위를 정하고 릴리스 여부 결정
2. **Issue** - `gh issue create`로 이슈 생성
3. **Worktree** - `main`에서 feature worktree 생성
4. **TDD** - 테스트 먼저, 최소 구현, `go test` 확인
5. **Release metadata** - 릴리스면 `VERSION.txt` + `CHANGELOG.md` 업데이트
6. **Commit & Push** - worktree에서 커밋, `git status --short --branch`로 clean 확인 후 push
7. **PR** - PR 템플릿 채워서 `gh pr create`
8. **CI & Merge** - checks 통과 후 `gh pr merge --squash --delete-branch`
9. **Cleanup** - worktree 제거, local main 동기화
10. **Release verify** - 자동 릴리스 워크플로우 결과 확인

## Worktree 생성 규칙

```bash
git fetch origin
git switch main && git pull --rebase
git worktree add .claude/worktrees/<branch-name> -b <branch-name> main
cd .claude/worktrees/<branch-name>
```

- 브랜치명: `feat/<name>`, `fix/<name>`, `chore/<name>` (lowercase kebab-case)
- worktree 경로: `.claude/worktrees/` 하위 (프로젝트 루트 기준 상대경로)

## Worktree 정리

```bash
git worktree remove .claude/worktrees/<branch-name>
git worktree prune
git fetch origin && git switch main && git pull --rebase
```

## 금지 사항

- main에서 직접 커밋/push 금지
- dirty worktree 상태에서 PR 생성 금지
- `--no-verify` 사용 금지 (사용자 명시 승인 없이)
- 자동 릴리스 워크플로우를 수동으로 대체 금지
