#!/bin/sh
# Stand-in for gh in the console E2E (the server's TARS_FOCUS_GH_PATH, set by
# playwright.config.ts): no spec depends on the host's gh, its login, or the
# network. The server runs it in the session's folder as
# `gh pr view [<number>] --json …`; the scenario is the content of e2e-gh in
# the repository's git dir (.git/e2e-gh; a worktree's .git is a file, so the
# git dir is asked for), written by the spec:
#   (missing) unavailable — gh fails as if not logged in
#   open      an open PR #7 from the checked-out branch, one passing check and
#             SonarCloud's informational comment (a bot: never a finding, #1094)
#   merged    the same PR, merged
#   conflict  the same PR, mergeStateStatus DIRTY and no checks at all — gh
#             does not run `pull_request` workflows on a conflicting PR (#1253)
# Every call is appended to e2e-gh.log beside it.
gitdir=$(git rev-parse --git-dir 2>/dev/null || echo .git)
echo "$*" >> "$gitdir/e2e-gh.log" 2>/dev/null
scenario=$(cat "$gitdir/e2e-gh" 2>/dev/null)
case "$scenario" in
  open) state=OPEN merge=null mergestate=CLEAN checks='[{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-01T00:00:00Z"}]' comments='[{"id":"IC_sonar","author":{"login":"sonarqubecloud"},"authorAssociation":"NONE","body":"Quality Gate passed","url":"https://example.test/pr/7#issuecomment-1"}]' ;;
  merged) state=MERGED merge='{"oid":"e2e0000000000000000000000000000000000000"}' mergestate=CLEAN checks='[{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-01T00:00:00Z"}]' comments='[{"id":"IC_sonar","author":{"login":"sonarqubecloud"},"authorAssociation":"NONE","body":"Quality Gate passed","url":"https://example.test/pr/7#issuecomment-1"}]' ;;
  conflict) state=OPEN merge=null mergestate=DIRTY checks='[]' comments='[]' ;;
  *)
    echo "e2e gh stub: not logged in" >&2
    exit 4
    ;;
esac
head=$(git rev-parse HEAD)
branch=$(git rev-parse --abbrev-ref HEAD)
printf '{"number":7,"url":"https://example.test/pr/7","state":"%s","mergeStateStatus":"%s","headRefOid":"%s","headRefName":"%s","mergeCommit":%s,"author":{"login":"e2e"},"statusCheckRollup":%s,"reviews":[],"comments":%s}\n' "$state" "$mergestate" "$head" "$branch" "$merge" "$checks" "$comments"
