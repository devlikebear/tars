---
description: Start a Ralph loop for the current Claude Code session. Use when the user runs /ralph-loop with a task prompt, an exact completion promise, and an optional max-iteration limit.
allowed-tools: Bash, Read, Write, Edit, MultiEdit, LS, Glob, Grep
hooks:
  Stop:
    - hooks:
        - type: command
          command: "\"${CLAUDE_PROJECT_DIR}/.claude/ralph-loop/hooks/stop-hook.sh\""
---

# Ralph Loop

Configuration:
!python3 "${CLAUDE_PROJECT_DIR}/.claude/ralph-loop/bin/setup_state.py" "$ARGUMENTS" "${CLAUDE_SESSION_ID}"

## Execution rules

- Execute the stored prompt now.
- Let the Stop hook replay the stored prompt verbatim on each blocked stop.
- Do not emit the completion promise until the task is truly complete.
- Prefer automatic verification such as tests, linters, or builds whenever the prompt implies them.
- If you are close to the max-iteration cap, leave a concise blocker summary before the final stop attempt.
