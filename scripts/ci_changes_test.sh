#!/usr/bin/env bash
# Tests for ci_changes.sh classify. Run with `make ci-changes-test`.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/ci_changes.sh"
status=0

check() {
  local name="$1" want="$2" files="$3" got
  got="$(printf '%s' "$files" | "$script" classify | tr '\n' ' ')"
  if [ "$got" != "$want " ]; then
    printf '[ci-changes] FAIL %s: got "%s", want "%s"\n' "$name" "$got" "$want" >&2
    status=1
  fi
}

check 'docs only' 'code=false desktop=false' $'docs/decisions/product-focus.md\nREADME.md\nCLAUDE.md\n'
check 'changelog and license' 'code=false desktop=false' $'CHANGELOG.md\nLICENSE\n.github/pull_request_template.md\n'
check 'api snapshot is code' 'code=true desktop=false' $'docs/public-api-surface.txt\n'
check 'go source' 'code=true desktop=false' $'internal/tarsserver/handler_chat.go\n'
check 'docs with one source file' 'code=true desktop=false' $'docs/console.md\npkg/llm/router.go\n'
check 'nested markdown is code' 'code=true desktop=false' $'internal/prompt/templates/system.md\n'
check 'version bump' 'code=true desktop=false' $'VERSION.txt\nCHANGELOG.md\n'
check 'desktop module' 'code=true desktop=true' $'desktop/internal/server/server.go\n'
check 'desktop readme' 'code=true desktop=true' $'desktop/README.md\n'
check 'makefile' 'code=true desktop=true' $'Makefile\n'
check 'ci workflow' 'code=true desktop=true' $'.github/workflows/ci.yml\n'
check 'codeql workflow' 'code=true desktop=false' $'.github/workflows/codeql.yml\n'
check 'empty list runs everything' 'code=true desktop=true' ''

if [ "$status" -ne 0 ]; then
  exit "$status"
fi
printf '[ci-changes] passed\n'
