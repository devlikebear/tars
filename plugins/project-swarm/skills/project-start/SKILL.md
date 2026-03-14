---
name: project-start
description: "Kick off a software project from natural language, collect the minimum clarifying answers, finalize a project brief, seed the board, and start autonomous execution."
user-invocable: true
recommended_tools:
  - project_brief_get
  - project_brief_update
  - project_brief_finalize
  - project_state_get
  - project_state_update
  - project_board_get
  - project_board_update
  - project_autopilot_start
  - project_dispatch
recommended_project_files:
  - PROJECT.md
  - STATE.md
  - KANBAN.md
  - ACTIVITY.jsonl
wake_phases:
  - plan
  - execute
---

# Project Start

Use this skill when the user wants to start a new software project from a chat UI.

## Goals

- Turn a rough goal into a finalized project brief.
- Ask only the minimum useful follow-up questions.
- Finalize the brief into a project and activate it for the current session.
- Seed a practical MVP board with developer and reviewer tasks.
- Start autonomous execution once requirements are sufficiently clear.

## Workflow

1. Call `project_brief_get` for the current session.
2. If there is no active brief or the brief is empty, call `project_brief_update` with the user's goal.
3. Ask at most 3 to 5 concrete questions that materially affect implementation:
   - platform or UI surface
   - auth or user accounts
   - persistence or database expectations
   - required integrations
   - deployment target
4. Store answers in `project_brief_update`.
5. When the brief is sufficiently specified, set the brief status to `ready`.
6. Call `project_brief_finalize`.
7. Call `project_board_update` to seed the first MVP backlog.
   - Use canonical board columns and statuses: `todo`, `in_progress`, `review`, `done`
   - Do not invent alternate column names such as `backlog` or `doing`
8. Call `project_state_update` with the next execution step.
9. Call `project_autopilot_start`.

## Rules

- Prefer short follow-up questions over long questionnaires.
- If the user already specified enough detail, do not ask unnecessary questions.
- If the user explicitly wants to start now, work autonomously, or keep to MVP scope, default low-risk implementation choices instead of blocking on one last stack or styling preference.
- Treat framework or stack selection as defaultable when the core product shape, persistence, deployment target, and MVP scope are already clear.
- Keep the backlog small and MVP-focused.
- Use the built-in project tools before describing raw HTTP API routes.
- If you mention APIs, reference these canonical routes:
  - `PATCH /v1/project-briefs/{session_id}`
  - `POST /v1/project-briefs/{session_id}/finalize`
  - `PATCH /v1/projects/{project_id}/board`
  - `POST /v1/projects/{project_id}/dispatch`

## Output Contract

- If more information is needed:
  - ask the next smallest set of questions
- If the project is ready:
  - summarize the brief
  - name the created project
  - state that autonomous execution has started
