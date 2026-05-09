#!/usr/bin/env bash
set -euo pipefail

cover_file="${1:-coverage.out}"
minimum="${2:-60}"
go_bin="${GO:-go}"

if [[ ! -s "${cover_file}" ]]; then
  printf 'coverage profile not found: %s\n' "${cover_file}" >&2
  exit 1
fi

total="$("${go_bin}" tool cover -func="${cover_file}" | awk '/^total:/ { gsub("%", "", $3); print $3 }')"
if [[ -z "${total}" ]]; then
  printf 'coverage total not found in %s\n' "${cover_file}" >&2
  exit 1
fi

awk -v total="${total}" -v minimum="${minimum}" 'BEGIN {
  if (total + 0 < minimum + 0) {
    printf "coverage %.1f%% is below required %.1f%%\n", total, minimum > "/dev/stderr"
    exit 1
  }
  printf "coverage %.1f%% meets required %.1f%%\n", total, minimum
}'
