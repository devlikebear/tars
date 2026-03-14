---
name: project-autopilot
description: "Run an existing project board toward completion by dispatching todo and review stages, automatically recovering stalled work, and continuously supervising until done."
user-invocable: false
recommended_tools:
  - project_get
  - project_state_get
  - project_state_update
  - project_board_get
  - project_activity_get
  - project_dispatch
recommended_project_files:
  - PROJECT.md
  - STATE.md
  - KANBAN.md
  - ACTIVITY.jsonl
wake_phases:
  - execute
  - review
---

# Project Autopilot

Use this skill to continue an already-created project until it reaches `done`, automatically recovering routine stalls without asking the user to manually restart the project.

## Workflow

1. Read the current project metadata, state, board, and recent activity.
2. If there are `todo` tasks, dispatch the `todo` stage.
3. If there are `review` tasks, dispatch the `review` stage.
4. After each stage, refresh board and activity.
5. If tasks are left in `in_progress`, diagnose whether they were interrupted, rejected in review, or blocked by verification and requeue them to `todo` when the next step is a routine retry.
6. Keep supervising on a timer until all tasks are `done`.
7. If all tasks are `done`, update `STATE.md` with a completion summary.

## Rules

- Do not invent success. Read the board and activity before deciding.
- Prefer dispatching existing tasks over rewriting the backlog.
- Surface blockers clearly in project state and activity, but do not stop for routine retry decisions that the PM can make safely.
- Do not mark a project `done` when the board is empty unless completed work was already recorded.
- When GitHub Flow or verification gates block progress, auto-retry the task when the fix is to rerun the implementation loop with the recorded blocker context.
- If the server restarts or heartbeat finds an active project without a live autopilot loop, restart supervision automatically.

## Completion

Stop when one of these is true:

- all tasks are `done`
- a blocker is terminal and cannot be recovered automatically
