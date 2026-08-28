#!/usr/bin/env bash
set -euo pipefail
base="${DIFF_BASE:-origin/main}"; head="${DIFF_HEAD:-HEAD}"; cover="${1:-cover.local.out}"
changed=$(mktemp); covered=$(mktemp); coverable=$(mktemp)
trap 'rm -f "$changed" "$covered" "$coverable"' EXIT
cur=""
while IFS= read -r line; do
  if [[ "$line" == "+++ b/"* ]]; then cur="${line#+++ b/}"
  elif [[ "$line" == "@@"* && -n "$cur" ]]; then
    h=$(printf '%s' "$line" | sed -E 's/^@@ -[0-9,]+ \+([0-9]+)(,([0-9]+))? @@.*/\1 \3/')
    s=$(printf '%s' "$h" | cut -d' ' -f1); c=$(printf '%s' "$h" | cut -d' ' -f2); [[ -z "$c" ]] && c=1
    for ((i=0;i<c;i++)); do printf '%s:%s\n' "$cur" "$((s+i))"; done
  fi
done < <(git diff -U0 --diff-filter=ACMRT "$base" "$head" -- '*.go') > "$changed"
while IFS= read -r line; do
  [[ "$line" == mode:* ]] && continue
  f="${line%%:*}"; rest="${line#*:}"; span="${rest%% *}"; n="${rest##* }"
  sl="${span%%.*}"; ep="${span#*,}"; el="${ep%%.*}"; rel="${f#github.com/devlikebear/tars/}"
  for ((l=sl;l<=el;l++)); do printf '%s:%s\n' "$rel" "$l"; done >> "$coverable"
  if [[ "$n" != "0" ]]; then for ((l=sl;l<=el;l++)); do printf '%s:%s\n' "$rel" "$l"; done >> "$covered"; fi
done < "$cover"
sort -u "$changed" -o "$changed"; sort -u "$coverable" -o "$coverable"; sort -u "$covered" -o "$covered"
comm -12 "$changed" "$coverable" | comm -23 - "$covered"
