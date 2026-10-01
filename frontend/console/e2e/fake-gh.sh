#!/bin/sh
# Stand-in for gh in the console E2E (the server's TARS_FOCUS_GH_PATH, set by
# playwright.config.ts): no spec depends on the host's gh, its login, or the
# network. The server runs it in the session's folder as
# `gh pr view [<number>] --json …`; the scenario is the content of
# .git/e2e-gh there, written by the spec:
#   (missing) unavailable — gh fails as if not logged in
#   open      an open PR #7 from the checked-out branch, one passing check
#   merged    the same PR, merged
# Every call is appended to .git/e2e-gh.log.
echo "$*" >> .git/e2e-gh.log 2>/dev/null
scenario=$(cat .git/e2e-gh 2>/dev/null)
case "$scenario" in
  open) state=OPEN ;;
  merged) state=MERGED ;;
  *)
    echo "e2e gh stub: not logged in" >&2
    exit 4
    ;;
esac
head=$(git rev-parse HEAD)
branch=$(git rev-parse --abbrev-ref HEAD)
printf '{"number":7,"url":"https://example.test/pr/7","state":"%s","mergeStateStatus":"CLEAN","headRefOid":"%s","headRefName":"%s","author":{"login":"e2e"},"statusCheckRollup":[{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"2026-10-01T00:00:00Z"}],"reviews":[],"comments":[]}\n' "$state" "$head" "$branch"
