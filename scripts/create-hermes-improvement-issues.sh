#!/usr/bin/env bash
# create-hermes-improvement-issues.sh
#
# Creates the GitHub milestone, labels, and issues for the
# hermes-inspired improvements plan (docs/plans/hermes-improvements/).
#
# Usage:
#   scripts/create-hermes-improvement-issues.sh            # create
#   scripts/create-hermes-improvement-issues.sh --dry-run  # print only
#
# Requirements:
#   - gh CLI installed and authenticated against the tars repo
#   - Current working directory: any; script resolves repo root
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
  echo "[hermes-issues] DRY RUN — no changes will be made"
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "[hermes-issues] gh CLI not found. Install from https://cli.github.com/" >&2
  exit 2
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "[hermes-issues] gh is not authenticated. Run: gh auth login" >&2
  exit 2
fi

REPO="devlikebear/tars"
MILESTONE="v0.25.0-hermes"
MILESTONE_DESC="Hermes-inspired improvements. See docs/plans/hermes-improvements/README.md"

# ---------- helpers ----------
run() {
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    printf '[dry-run] %s\n' "$*"
  else
    eval "$@"
  fi
}

ensure_label() {
  local name="$1"
  local color="$2"
  local desc="$3"
  if gh label list --repo "${REPO}" --limit 200 --json name -q '.[].name' 2>/dev/null | grep -Fxq "${name}"; then
    echo "[hermes-issues] label '${name}' exists"
  else
    echo "[hermes-issues] creating label '${name}'"
    run gh label create "${name}" --repo "${REPO}" --color "${color}" --description "${desc}"
  fi
}

ensure_milestone() {
  local title="$1"
  local desc="$2"
  local existing
  existing="$(gh api "repos/${REPO}/milestones?state=open" --jq ".[] | select(.title==\"${title}\") | .number" 2>/dev/null || true)"
  if [[ -n "${existing}" ]]; then
    echo "[hermes-issues] milestone '${title}' exists (#${existing})"
    MILESTONE_NUMBER="${existing}"
  else
    echo "[hermes-issues] creating milestone '${title}'"
    if [[ "${DRY_RUN}" -eq 1 ]]; then
      MILESTONE_NUMBER="DRY"
    else
      MILESTONE_NUMBER="$(gh api "repos/${REPO}/milestones" -f title="${title}" -f description="${desc}" --jq '.number')"
      echo "[hermes-issues] created milestone #${MILESTONE_NUMBER}"
    fi
  fi
}

create_issue() {
  local title="$1"
  local body_file="$2"
  shift 2
  local labels=("$@")
  local label_args=()
  for l in "${labels[@]}"; do
    label_args+=(--label "$l")
  done
  echo "[hermes-issues] creating issue: ${title}"
  if [[ "${DRY_RUN}" -eq 1 ]]; then
    printf '[dry-run] gh issue create --repo %s --title %q --body-file %s --milestone %s %s\n' \
      "${REPO}" "${title}" "${body_file}" "${MILESTONE}" "${label_args[*]}"
    return
  fi
  local url
  url="$(gh issue create \
    --repo "${REPO}" \
    --title "${title}" \
    --body-file "${body_file}" \
    --milestone "${MILESTONE}" \
    "${label_args[@]}")"
  echo "  → ${url}"
  CREATED_URLS+=("${url}")
}

# ---------- labels ----------
ensure_label "enhancement"   "a2eeef" "New feature or improvement"
ensure_label "area/tool"     "c5def5" "internal/tool package"
ensure_label "area/gateway"  "c5def5" "internal/gateway package"
ensure_label "area/llm"      "c5def5" "internal/llm package"
ensure_label "area/memory"   "c5def5" "internal/memory package"
ensure_label "area/config"   "c5def5" "internal/config package"

# ---------- milestone ----------
MILESTONE_NUMBER=""
ensure_milestone "${MILESTONE}" "${MILESTONE_DESC}"

# ---------- prepare issue bodies ----------
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

DOC_BASE="https://github.com/${REPO}/blob/main/docs/plans/hermes-improvements"

write_body() {
  local file="$1"
  local doc="$2"
  local summary="$3"
  local sprint="$4"
  cat >"${file}" <<EOF
## Summary

${summary}

## Design Doc

[${doc}](${DOC_BASE}/${doc})

## Sprint

${sprint}

## Acceptance Criteria

See the **Acceptance Criteria** section in the design doc.

## Identity Check

See the **Identity Check** section in the design doc. All five TARS identity
guardrails must remain intact before this issue is closed:

1. 단일 Go 바이너리
2. File-first, DB-less
3. User surface ↔ System surface 컴파일 타임 분리
4. 정책은 config, 메커니즘은 Go
5. Durable semantic memory + 야간 배치 컴파일

---

_Part of the [\`v0.25.0-hermes\`](${DOC_BASE}/README.md) milestone._
EOF
}

# ---------- 5 implementation issues ----------
CREATED_URLS=()

write_body "${TMP_DIR}/01.md" "01-toolset-groups.md" \
  "Expose \`tools_allow_groups\` / \`tools_deny_groups\` in \`AGENT.md\` and session config, define deny-wins resolution, improve blocked-tool error messages." \
  "S1"
create_issue "feat(tool): toolset groups policy surface (hermes #01)" "${TMP_DIR}/01.md" \
  "enhancement" "area/tool"

write_body "${TMP_DIR}/02.md" "02-context-compression-knobs.md" \
  "Expose compaction knobs (threshold ratio, protect-last-N, budget, timeout) and add a deterministic \`simple\` fallback mode when the structured LLM summary fails." \
  "S1"
create_issue "feat(config): context compression knobs + simple fallback (hermes #02)" "${TMP_DIR}/02.md" \
  "enhancement" "area/config"

write_body "${TMP_DIR}/03.md" "03-provider-override.md" \
  "Add optional per-task \`provider_override\` on \`AgentTask\` with config allowlist. Credentials stay in env/config — never accepted as tool parameters. Record resolved provider in gateway run audit log." \
  "S2"
create_issue "feat(gateway): per-task provider/credential override (hermes #03)" "${TMP_DIR}/03.md" \
  "enhancement" "area/gateway" "area/llm"

write_body "${TMP_DIR}/04.md" "04-moa-consensus.md" \
  "Add opt-in \`mode=consensus\` to the gateway executor. Run a task across N provider/model variants in parallel and synthesize with a light-tier aggregator. Strict budget/fanout/timeout limits enforced deterministically in Go. Console SSE visualizes parallel streams. Depends on #03." \
  "S2"
create_issue "feat(gateway): MoA consensus executor mode (hermes #04)" "${TMP_DIR}/04.md" \
  "enhancement" "area/gateway"

write_body "${TMP_DIR}/05.md" "05-memory-backend-interface.md" \
  "Refactor \`internal/memory\` to expose a \`Backend\` interface. Wrap the existing file-based implementation as \`FileBackend\`. No behavior change, no external adapters in this PR — interface extraction only." \
  "S3"
create_issue "refactor(memory): extract Backend interface, wrap file impl (hermes #05)" "${TMP_DIR}/05.md" \
  "enhancement" "area/memory"

# ---------- meta tracker ----------
META_BODY="${TMP_DIR}/meta.md"
cat >"${META_BODY}" <<EOF
## Hermes-inspired improvements tracker

This meta issue tracks a small family of TARS improvements inspired by
Hermes Agent, without touching any of the five TARS identity guardrails
(see [README](${DOC_BASE}/README.md)).

### Sprint 1 — 자체 완성도

- [ ] #01 toolset groups policy surface
- [ ] #02 context compression knobs + simple fallback

### Sprint 2 — 핵심 차별화 (opt-in)

- [ ] #03 per-task provider/credential override
- [ ] #04 MoA consensus executor mode (depends on #03)

### Sprint 3 — 미래 준비

- [ ] #05 memory backend interface extraction

### Non-goals

- Python/Node runtime dependency
- External memory platform as a mandatory backend
- LLM gaining control of system surface (pulse/reflection)
- Making any of the above default-on

### Design Docs

- [README](${DOC_BASE}/README.md)
- [01 toolset groups](${DOC_BASE}/01-toolset-groups.md)
- [02 context compression knobs](${DOC_BASE}/02-context-compression-knobs.md)
- [03 provider override](${DOC_BASE}/03-provider-override.md)
- [04 MoA consensus](${DOC_BASE}/04-moa-consensus.md)
- [05 memory backend interface](${DOC_BASE}/05-memory-backend-interface.md)

### Identity Checklist (each PR must pass)

- [ ] 단일 Go 바이너리 유지
- [ ] File-first, DB-less 유지
- [ ] Scope isolation (pulse_/reflection_/ops_ wiring 보증) 유지
- [ ] 정책은 config, 메커니즘은 Go 유지
- [ ] Memory/reflection 경로 회귀 없음
EOF

echo "[hermes-issues] creating meta tracker"
if [[ "${DRY_RUN}" -eq 1 ]]; then
  printf '[dry-run] gh issue create --repo %s --title %q --body-file %s --milestone %s --label enhancement\n' \
    "${REPO}" "Hermes-inspired improvements tracker" "${META_BODY}" "${MILESTONE}"
else
  META_URL="$(gh issue create \
    --repo "${REPO}" \
    --title "Hermes-inspired improvements tracker" \
    --body-file "${META_BODY}" \
    --milestone "${MILESTONE}" \
    --label "enhancement")"
  echo "  → ${META_URL}"
  CREATED_URLS+=("${META_URL}")
fi

# ---------- summary ----------
echo
echo "[hermes-issues] done"
if [[ "${#CREATED_URLS[@]}" -gt 0 ]]; then
  echo "Created:"
  for u in "${CREATED_URLS[@]}"; do
    echo "  ${u}"
  done
fi
echo
echo "Next steps:"
echo "  1. Review created issues on GitHub"
echo "  2. Pin the meta tracker to the repo if desired"
echo "  3. Update the meta tracker checkboxes as PRs land"
