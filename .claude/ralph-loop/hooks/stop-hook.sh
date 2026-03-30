#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$PWD}"
STATE_FILE="${PROJECT_DIR}/.claude/ralph-loop/state.json"
HOOK_INPUT="$(cat)"

if [[ ! -f "${STATE_FILE}" ]]; then
  exit 0
fi

python3 - "${STATE_FILE}" "${HOOK_INPUT}" <<'PY'
import json
import sys
from pathlib import Path


state_path = Path(sys.argv[1])
hook_input = sys.argv[2]

try:
    state = json.loads(state_path.read_text())
except Exception:
    raise SystemExit(0)

try:
    payload = json.loads(hook_input or "{}")
except Exception:
    payload = {}

if not state.get("active"):
    raise SystemExit(0)

state_session = str(state.get("session_id", "")).strip()
payload_session = str(payload.get("session_id", "")).strip()
if state_session and payload_session and state_session != payload_session:
    raise SystemExit(0)

prompt = str(state.get("prompt", "")).strip()
completion_promise = str(state.get("completion_promise", "")).strip()
last_message = str(payload.get("last_assistant_message", "") or "")
iterations = int(state.get("iterations", 0) or 0)
max_iterations = int(state.get("max_iterations", 0) or 0)

if completion_promise and completion_promise in last_message:
    state["active"] = False
    state["finished_reason"] = "completion_promise_matched"
    state_path.write_text(json.dumps(state, indent=2) + "\n")
    raise SystemExit(0)

if max_iterations > 0 and iterations >= max_iterations:
    state["active"] = False
    state["finished_reason"] = "max_iterations_reached"
    state_path.write_text(json.dumps(state, indent=2) + "\n")
    print(json.dumps({
        "systemMessage": f"Ralph loop stopped after max_iterations={max_iterations}."
    }))
    raise SystemExit(0)

if not prompt:
    state["active"] = False
    state["finished_reason"] = "missing_prompt"
    state_path.write_text(json.dumps(state, indent=2) + "\n")
    raise SystemExit(0)

state["iterations"] = iterations + 1
state_path.write_text(json.dumps(state, indent=2) + "\n")
print(json.dumps({
    "decision": "block",
    "reason": prompt,
}))
PY
