#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
base="${DIFF_BASE:-$("${script_dir}/diff_base.sh")}"
head="${DIFF_HEAD:-}"
go_bin="${GO:-go}"
cover_out="${COVER_OUT:-coverage.diff.out}"
cover_min="${COVER_MIN:-60}"
head_label="${head:-working tree}"

packages_file="$(mktemp)"
trap 'rm -f "${packages_file}"' EXIT

if [[ -n "${head}" ]]; then
  "${script_dir}/go_changed_packages.sh" "${base}" "${head}" > "${packages_file}"
else
  "${script_dir}/go_changed_packages.sh" "${base}" > "${packages_file}"
fi
if [[ ! -s "${packages_file}" ]]; then
  printf 'No changed Go packages between %s and %s; skipping go test.\n' "${base}" "${head_label}"
  exit 0
fi

printf 'Testing changed Go packages between %s and %s:\n' "${base}" "${head_label}"
sed 's/^/  /' "${packages_file}"

"${go_bin}" test -coverprofile="${cover_out}" $(cat "${packages_file}")
if [[ -n "${cover_min}" ]]; then
  "${script_dir}/check_coverage.sh" "${cover_out}" "${cover_min}"
fi
