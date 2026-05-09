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
