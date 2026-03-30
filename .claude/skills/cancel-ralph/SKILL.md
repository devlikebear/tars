---
description: Cancel the active Ralph loop for the current Claude Code session and clear its saved state.
allowed-tools: Bash
disable-model-invocation: true
---

!python3 "${CLAUDE_PROJECT_DIR}/.claude/ralph-loop/bin/cancel_state.py" "${CLAUDE_SESSION_ID}"
