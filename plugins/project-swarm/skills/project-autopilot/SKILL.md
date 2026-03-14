---
name: project-autopilot
description: "Run an existing project board toward completion by dispatching todo and review stages, updating project state, and surfacing blockers."
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

Use this skill to continue an already-created project until it reaches `done` or a clear blocker.

## Workflow

1. Read the current project metadata, state, board, and recent activity.
2. If there are `todo` tasks, dispatch the `todo` stage.
3. If there are `review` tasks, dispatch the `review` stage.
4. After each stage, refresh board and activity.
5. If tasks are blocked in `in_progress`, update `STATE.md` with the blocker and the next user-facing question.
6. If all tasks are `done`, update `STATE.md` with a completion summary.

## Rules

- Do not invent success. Read the board and activity before deciding.
- Prefer dispatching existing tasks over rewriting the backlog.
- Surface blockers clearly in project state and activity.
- Do not mark a project `done` when the board is empty unless completed work was already recorded.
- When GitHub Flow or verification gates block progress, tell the user exactly what is missing.

## Completion

Stop when one of these is true:

- all tasks are `done`
- a blocker requires user input
- the project has no actionable tasks left
