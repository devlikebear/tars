#!/usr/bin/env bash
set -euo pipefail

base="${1:-$("${BASH_SOURCE%/*}/diff_base.sh")}"
head="${2:-${DIFF_HEAD:-}}"
go_bin="${GO:-go}"

if [[ -n "${head}" ]]; then
  changed_files="$(git diff --name-only --diff-filter=ACMRT "${base}" "${head}" -- '*.go' 'go.mod' 'go.sum')"
else
  changed_files="$(git diff --name-only --diff-filter=ACMRT "${base}" -- '*.go' 'go.mod' 'go.sum')"
fi

# Files in nested modules (a directory with its own go.mod, such as desktop/
# or tools/) are not packages of the root module: `go list` from here
# rejects them. Those modules run their own checks (make desktop-test).
nested_modules="$(git ls-files -- '*/go.mod' | sed 's#/go\.mod$##')"
if [[ -n "${nested_modules}" && -n "${changed_files}" ]]; then
  # Passed through the environment, not -v: macOS awk rejects a newline in
  # a -v value ("newline in string") once there is more than one module.
  changed_files="$(printf '%s\n' "${changed_files}" | NESTED_MODULES="${nested_modules}" awk '
    BEGIN { n = split(ENVIRON["NESTED_MODULES"], r, "\n") }
    { for (i = 1; i <= n; i++) if (index($0, r[i] "/") == 1) next; print }
  ')"
fi

if [[ -z "${changed_files}" ]]; then
  exit 0
fi

if printf '%s\n' "${changed_files}" | grep -Eq '^(go\.mod|go\.sum)$'; then
  printf '%s\n' './...'
  exit 0
fi

dirs_file="$(mktemp)"
trap 'rm -f "${dirs_file}"' EXIT

printf '%s\n' "${changed_files}" |
  awk 'NF { sub("/[^/]*$", "", $0); if ($0 == "") $0 = "."; print "./" $0 }' |
  sed 's#^\./\./#./#' |
  sort -u > "${dirs_file}"

if [[ ! -s "${dirs_file}" ]]; then
  exit 0
fi

"${go_bin}" list $(cat "${dirs_file}") 2>/dev/null | sed "s#^$("${go_bin}" list -m)#./#"
