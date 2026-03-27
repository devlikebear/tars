---
name: project-autopilot
description: "Supervise an existing project by reading the current phase, advancing the phase engine when useful, resuming background autonomy when the phase is ready, and surfacing blockers or human decisions clearly."
user-invocable: false
recommended_tools:
  - project_get
  - project_state_get
  - project_activity_get
  - project_board_get
  - project_autopilot_advance
  - project_autopilot_start
recommended_project_files:
  - PROJECT.md
  - STATE.md
  - ACTIVITY.jsonl
  - KANBAN.md
wake_phases:
  - plan
  - execute
  - review
---

# Project Autopilot

Use this skill to continue an already-created project while keeping the real control flow centered on project phase and human decision points.

## Workflow

1. Read the current project metadata, state, recent activity, and board projection.
2. Treat `STATE.md` and the phase snapshot as the source of truth. Use `KANBAN.md` only to inspect current work items.
3. If the current state is unclear, call `project_autopilot_advance` once to let the phase engine resolve the next step synchronously.
4. Re-read state or activity after the step and identify one of these cases:
   - the project is `planning` and waiting for approval or missing answers
   - the project is actively `executing` or `reviewing`
   - the project is `blocked` on a real blocker
   - the project is `done`
5. Only call `project_autopilot_start` when the current phase is ready for autonomous execution and there is no pending human decision.
6. If the engine returns a planning blocker, summarize the required approval or clarification instead of dispatching work manually.
7. If background autonomy is resumed, report the current phase, next action, and the condition that should bring the project back to the user.

## Rules

- Do not treat the board as the execution engine. It is only a projection of current work.
- Do not invent or seed backlog items from this skill.
- Do not bypass planning blockers by dispatching tasks directly when the phase still needs approval.
- Prefer a single `project_autopilot_advance` call to clarify ambiguity before starting a background loop.
- Surface `next_action`, `pending decision`, and `current blocker` in plain language.
- Stop immediately when the project needs human approval, missing requirements, or a non-routine blocker.

## Completion

Stop when one of these is true:

- the project is actively running in the background and the next human checkpoint is clear
- the phase engine reports `done`
- the phase engine reports a blocker or planning decision that needs the user
