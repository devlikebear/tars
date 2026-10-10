#!/usr/bin/env bash
# Decides which CI jobs a change needs, from the list of files it touches.
#
#   ci_changes.sh classify   reads file paths on stdin, prints the outputs
#   ci_changes.sh            lists the event's files through the GitHub API,
#                            then writes the outputs to $GITHUB_OUTPUT
#
# Outputs:
#   code=false     every file is documentation: skip builds, tests and CodeQL
#   desktop=false  nothing the desktop shell is built from changed
#
# Jobs test for `!= 'false'`, so anything that goes wrong here — no token, an
# API error, an event this script does not know — runs everything.
set -euo pipefail

# Documentation is what no build, test or check reads. docs/public-api-surface.txt
# is the API snapshot `make api-check` compares against, so it counts as code.
is_doc() {
  case "$1" in
    docs/public-api-surface.txt) return 1 ;;
    docs/*) return 0 ;;
    LICENSE | .github/pull_request_template.md) return 0 ;;
    */*) return 1 ;;
    *.md) return 0 ;;
  esac
  return 1
}

# The desktop jobs build desktop/ (its own Go module) through the Makefile and
# the packaging script. This script and the workflow decide whether they run.
is_desktop() {
  case "$1" in
    desktop/* | Makefile | scripts/desktop_package.sh | scripts/ci_changes.sh | .github/workflows/ci.yml) return 0 ;;
  esac
  return 1
}

classify() {
  local code=false desktop=false seen=false file
  while IFS= read -r file; do
    [ -n "$file" ] || continue
    seen=true
    if ! is_doc "$file"; then code=true; fi
    if is_desktop "$file"; then desktop=true; fi
  done
  # An empty list is not "nothing changed", it is "could not tell".
  if [ "$seen" = false ]; then
    code=true
    desktop=true
  fi
  printf 'code=%s\ndesktop=%s\n' "$code" "$desktop"
}

run_everything() {
  printf '::notice title=CI changes::%s; running every job\n' "$1"
  printf 'code=true\ndesktop=true\n'
}

# The compare API returns at most 300 files and does not say when it cut the
# list, so a full page is treated as unknown.
compare_limit=300

list_files() {
  local files
  case "${GITHUB_EVENT_NAME:-}" in
    pull_request)
      gh api --paginate "repos/${GITHUB_REPOSITORY}/pulls/${PR_NUMBER}/files" --jq '.[].filename'
      ;;
    push)
      case "${PUSH_BEFORE:-}" in
        "" | 0000000000000000000000000000000000000000) return 1 ;;
      esac
      files="$(gh api "repos/${GITHUB_REPOSITORY}/compare/${PUSH_BEFORE}...${GITHUB_SHA}" --jq '.files[].filename')"
      if [ "$(printf '%s\n' "$files" | grep -c .)" -ge "$compare_limit" ]; then
        return 1
      fi
      printf '%s\n' "$files"
      ;;
    *)
      return 1
      ;;
  esac
}

main() {
  local files result
  if files="$(list_files)"; then
    result="$(printf '%s\n' "$files" | classify)"
  else
    result="$(run_everything "could not list the files of this ${GITHUB_EVENT_NAME:-unknown} event")"
  fi
  printf '%s\n' "$result"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    printf '%s\n' "$result" | grep -E '^(code|desktop)=' >> "$GITHUB_OUTPUT"
  fi
}

if [ "${1:-}" = classify ]; then
  classify
else
  main
fi
