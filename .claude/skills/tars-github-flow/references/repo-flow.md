# TARS Repo Flow - 상세 절차

## Phase 1: Plan

- 코딩 전에 범위를 정한다.
- 비자명한 작업은 최대 3개 work order로 분할.
- 퍼블릭 릴리스 포함 여부를 결정.

## Phase 2: Issue

```bash
gh issue create \
  --title "feat: 기능 설명" \
  --body $'## Summary\n- 문제/목표\n\n## Acceptance Criteria\n- 조건1\n- 조건2'
```

- 논리적 변경 단위당 하나의 이슈.
- 본문: problem, scope, acceptance criteria.

## Phase 3: Worktree 생성

```bash
git fetch origin
git switch main && git pull --rebase
mkdir -p .claude/worktrees
git worktree add .claude/worktrees/<branch> -b <prefix>/<name> main
cd .claude/worktrees/<branch>
```

- `feat/`, `fix/`, `chore/` prefix 사용.
- lowercase kebab-case.
- worktree 안에서만 작업. main 직접 수정 금지.

## Phase 4: TDD 개발

- 실패하는 테스트 먼저 작성.
- 테스트 통과하는 최소 구현.
- 커밋 전 diff 검토 (regression, 누락 테스트, 문서 갭).

규칙:
- `feat:`, `fix:`, `chore:` commit prefix.
- 특정 파일만 stage (`git add` 명시 경로).
- `--no-verify` 사용자 승인 없이 사용 금지.

## Phase 5: Release Metadata

릴리스 PR인 경우:
- `VERSION.txt` 업데이트 (SemVer).
- `CHANGELOG.md`에 날짜와 변경사항 추가.
- `README.md` 업데이트 (사용법/설치 변경 시).
- `VERSION.txt`와 `CHANGELOG.md`는 반드시 같은 PR에.

릴리스 규칙:
- main 머지 = 릴리스 승인.
- `release-on-version-bump` 워크플로우가 자동으로 태그/릴리스/Homebrew 업데이트.

## Phase 6: Commit & Push

```bash
git add <specific-files>
git commit -m "feat: 설명"
git status --short --branch   # clean 확인 필수
git push -u origin <branch>
```

- subject: 명령형, 72자 이하.
- worktree가 clean하지 않으면 push 금지.

## Phase 7: PR 생성

```bash
gh pr create --base main --head <branch> \
  --title "feat: 설명" \
  --body "$(cat <<'EOF'
## Summary
- 변경 요약

## Changes
- 주요 변경사항

## Validation
- [x] `go test ./...`
- [x] `go build ./...`

## Checklist
- [x] Conventional commit
- [x] 테스트 추가/수정
- [ ] CHANGELOG.md 업데이트 (릴리스 시)

## Risks / Rollback
- Risk level: low
- Rollback plan: revert merge commit
EOF
)"
```

필수 조건:
- `security` CI 통과
- `test` CI 통과
- 1 approval
- 모든 review thread resolved
- squash merge only

## Phase 8: CI 확인 & Merge

```bash
gh pr checks <number>
gh pr view <number> --json mergeable,mergeStateStatus,reviewDecision,statusCheckRollup
gh pr merge <number> --squash --delete-branch
```

- admin bypass: `gh pr merge <number> --squash --delete-branch --admin`

## Phase 9: Worktree 정리 & Main 동기화

```bash
git worktree remove .claude/worktrees/<branch>
git worktree prune
git fetch origin
git switch main
git pull --rebase
```

## Phase 10: Release 검증

릴리스 PR이었다면:

```bash
gh run list --workflow release-on-version-bump.yml --limit 5
gh release view v<version> --repo devlikebear/tars
gh api repos/devlikebear/tars/git/ref/tags/v<version>
gh api 'repos/devlikebear/homebrew-tars/contents/Formula/tars.rb?ref=main'
```

확인 항목:
- tag가 merge commit을 가리키는지
- release에 macOS archive + checksums.txt 포함
- homebrew-tars가 동일 버전/체크섬 참조

## 완료 보고

- Issue URL, PR URL, merge commit SHA, release URL (해당 시) 보고.
- 로컬 동기화 미완료 사항이 있으면 명시.
