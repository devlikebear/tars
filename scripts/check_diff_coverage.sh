#!/usr/bin/env bash
set -euo pipefail

cover_file="${1:-coverage.diff.out}"
minimum="${2:-80}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
base="${DIFF_BASE:-$("${script_dir}/diff_base.sh")}"
head="${DIFF_HEAD:-}"
go_bin="${GO:-go}"
head_label="${head:-working tree}"

changed_file="$(mktemp)"
coverable_file="$(mktemp)"
covered_file="$(mktemp)"
relevant_file="$(mktemp)"
covered_relevant_file="$(mktemp)"
trap 'rm -f "${changed_file}" "${coverable_file}" "${covered_file}" "${relevant_file}" "${covered_relevant_file}"' EXIT

diff_command() {
  if [[ -n "${head}" ]]; then
    git diff -U0 --diff-filter=ACMRT "${base}" "${head}" -- '*.go'
  else
    git diff -U0 --diff-filter=ACMRT "${base}" -- '*.go'
  fi
}

current_file=""
while IFS= read -r line; do
  if [[ "${line}" == "+++ b/"* ]]; then
    current_file="${line#+++ b/}"
    continue
  fi
  if [[ "${line}" =~ ^@@[[:space:]] && "${current_file}" == *.go && "${current_file}" != *_test.go ]]; then
    if [[ "${line}" =~ \+([0-9]+)(,([0-9]+))? ]]; then
      start="${BASH_REMATCH[1]}"
      count="${BASH_REMATCH[3]:-1}"
      if [[ "${count}" == "0" ]]; then
        continue
      fi
      for ((offset = 0; offset < count; offset++)); do
        printf '%s:%d\n' "${current_file}" "$((start + offset))"
      done
    fi
  fi
done < <(diff_command) | sort -u > "${changed_file}"

if [[ ! -s "${changed_file}" ]]; then
  printf 'No changed non-test Go lines between %s and %s; skipping diff coverage.\n' "${base}" "${head_label}"
  exit 0
fi

if [[ ! -s "${cover_file}" ]]; then
  printf 'coverage profile not found: %s\n' "${cover_file}" >&2
  exit 1
fi

module="$("${go_bin}" list -m)"
while IFS= read -r line; do
  [[ "${line}" == "mode:"* ]] && continue
  if [[ "${line}" =~ ^([^:]+):([0-9]+)\.[0-9]+,([0-9]+)\.[0-9]+[[:space:]]+[0-9]+[[:space:]]+([0-9]+)$ ]]; then
    file="${BASH_REMATCH[1]}"
    start="${BASH_REMATCH[2]}"
    end="${BASH_REMATCH[3]}"
    count="${BASH_REMATCH[4]}"
    file="${file#${module}/}"
    file="${file#./}"
    for ((line_no = start; line_no <= end; line_no++)); do
      printf '%s:%d\n' "${file}" "${line_no}" >> "${coverable_file}"
      if ((count > 0)); then
        printf '%s:%d\n' "${file}" "${line_no}" >> "${covered_file}"
      fi
    done
  fi
done < "${cover_file}"

sort -u -o "${coverable_file}" "${coverable_file}"
sort -u -o "${covered_file}" "${covered_file}"
comm -12 "${changed_file}" "${coverable_file}" > "${relevant_file}"

if [[ ! -s "${relevant_file}" ]]; then
  printf 'No coverable changed Go lines between %s and %s; skipping diff coverage.\n' "${base}" "${head_label}"
  exit 0
fi

comm -12 "${relevant_file}" "${covered_file}" > "${covered_relevant_file}"

total="$(wc -l < "${relevant_file}" | tr -d '[:space:]')"
covered="$(wc -l < "${covered_relevant_file}" | tr -d '[:space:]')"
percent="$(awk -v covered="${covered}" -v total="${total}" 'BEGIN { printf "%.1f", (covered / total) * 100 }')"

awk -v percent="${percent}" -v minimum="${minimum}" -v covered="${covered}" -v total="${total}" 'BEGIN {
  if (percent + 0 < minimum + 0) {
    printf "diff coverage %.1f%% (%d/%d changed coverable lines) is below required %.1f%%\n", percent, covered, total, minimum > "/dev/stderr"
    exit 1
  }
  printf "diff coverage %.1f%% (%d/%d changed coverable lines) meets required %.1f%%\n", percent, covered, total, minimum
}'
