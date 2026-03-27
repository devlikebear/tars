---
name: project-start
description: "Start or continue project planning from chat, collect the minimum clarifying answers, finalize a project brief, and prepare the first phase for approval."
user-invocable: true
recommended_tools:
  - project_brief_get
  - project_brief_update
  - project_brief_finalize
  - project_get
  - project_state_get
  - project_state_update
recommended_project_files:
  - PROJECT.md
  - STATE.md
  - ACTIVITY.jsonl
wake_phases:
  - plan
---

# Project Start

Use this skill when the user wants to start a new software project from a chat UI.

## Goals

- Turn a rough goal into a finalized project brief.
- Ask only the minimum useful follow-up questions.
- Finalize the brief into a project and activate it for the current session.
- Prepare the first phase goal and next action without starting execution yet.

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
7. Call `project_get` for the created project and summarize the first phase goal.
8. Call `project_state_update` so the project stays in planning until the user approves the first phase.
   - Prefer next actions such as `Approve the first phase backlog` or `Answer the remaining planning questions`.
   - Do not start autonomous execution from this skill unless the user explicitly asks to begin execution after reviewing the plan.
9. Stop after the planning summary or ask the smallest missing clarification.

## Rules

- Prefer short follow-up questions over long questionnaires.
- If the user already specified enough detail, do not ask unnecessary questions.
- If the user explicitly wants to start now, work autonomously, or keep to MVP scope, default low-risk planning choices instead of blocking on one last stack or styling preference.
- Treat framework or stack selection as defaultable when the core product shape, persistence, deployment target, and MVP scope are already clear.
- Keep the first phase plan small and MVP-focused.
- Use the built-in project tools before describing raw HTTP API routes.
- If you mention APIs, reference these canonical routes:
  - `PATCH /v1/project-briefs/{session_id}`
  - `POST /v1/project-briefs/{session_id}/finalize`
  - `GET /v1/projects/{project_id}`
  - `PATCH /v1/projects/{project_id}/state`

## Output Contract

- If more information is needed:
  - ask the next smallest set of questions
- If the project is ready:
  - summarize the brief
  - name the created project
  - show the proposed first phase and next action
  - ask for approval or the smallest missing clarification
