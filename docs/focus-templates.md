# Focus templates and goal mode

Focus mode runs one task as a pipeline of stages. The default pipeline is the development loop (plan → build → review → PR → PR review → merge). A **template** gives a task a different list of stages, and **goal mode** runs a pipeline to its end with nobody at the gates. The design and its reasons are in [`decisions/focus-mode.md`](decisions/focus-mode.md) §4.1 and §4.2.

## Templates

Three templates ship with the server:

| Id | Stages | For |
|---|---|---|
| `dev` | plan → build → review → pr → pr_review → merge | Development (the default) |
| `writing` | plan (outline) → draft → revise | Fiction and long-form writing |
| `research` | plan (scope) → research → report → check | Researching a question and writing it up |

Pick one in **New task** on the focus home, or pass `template` to `POST /v1/focus/pipelines`.

### Writing your own

Put a YAML (or JSON) file in `<workspace>/focus-templates/`. The file name without its extension is the template's id unless the file sets `id`.

```yaml
# <workspace>/focus-templates/blog.yaml
name: Blog post
description: Outline, write, edit.
stages:
  - id: plan
    label: Outline
    instructions: >-
      Do not write the post yet. Propose a plan: the angle and the reader in
      the goal line, one task per section with what "done" means, and the
      file the post goes to. Leave the verification commands empty.
  - id: write
    kind: build
    label: Write
    instructions: Write the sections in order to the file the plan names.
  - id: edit
    kind: review
    label: Edit
    instructions: >-
      Read the post as an editor. Each finding names the file and line and
      says what fails the reader there: a claim without support, a paragraph
      that repeats another, a sentence that is hard to follow.
    fix_instructions: Rewrite only the passages the findings name.
```

Each stage has a **kind**, which is how it behaves. A template names stages and tells each one what to do; it cannot change how a stage ends.

| Kind | What the stage does | How it ends |
|---|---|---|
| `plan` | Proposes tasks, the stages that apply and verification commands | The plan is approved |
| `build` | Works through its tasks | The report says `tasks_done` and every verification command passes |
| `review` | Reports findings; accepted ones are fixed and verified | A round with nothing left to fix passes verification |
| `pr`, `pr_review`, `merge` | Draft and open the pull request, follow CI and comments, merge | Facts from `gh pr view` |

Rules:

- The first stage is `plan`.
- `plan`, `pr`, `pr_review` and `merge` appear at most once, under exactly those ids. `pr_review` and `merge` need `pr` before them.
- `build` and `review` stages can appear any number of times under ids of your own (`a-z`, digits, `_`), each with `kind: build` or `kind: review`.
- At most 12 stages. Instructions are at most 4000 bytes and may not contain `<focus-…>` tags.
- A file cannot replace a built-in template.

A file that breaks a rule is skipped; `GET /v1/focus/templates` lists it under `diagnostics` with the reason, and the New task form says how many files were skipped.

With more than one `build` stage, the plan assigns each task to a stage (`"stage": "write"`), and each stage works only on its own tasks. Verification commands are optional: with none, a stage ends as soon as its report says the tasks are done.

A pipeline copies its template when it starts, so editing the file changes new tasks only.

## Goal mode

Tick **Goal mode** when starting a task, or use the **◎ Goal mode** toggle on the pipeline screen. The server then decides every gate itself:

| Waiting on | What happens |
|---|---|
| Plan, pull request draft | Approved as proposed |
| Merge | Approved; the merge gate only opens when the checks are green |
| Review findings | Fixed, except `low` severity, which is dismissed |
| A failed check or review comment on the pull request | Fixed |
| A question from the agent | Answered "decide yourself, and say what you chose" |
| A stage that stopped (loop limit, the same failure twice, a failed turn, a server restart) | Retried |

Retries draw on a budget of 20. When it is used up, goal mode stops where it is and sends a notification; the open gate waits for you, and you can turn goal mode on again. A turn that failed is retried after a wait that starts at 30 seconds and doubles up to 15 minutes.

While goal mode is on, the session runs tools without asking (permission mode `auto`); the mode it had comes back when goal mode ends. Every decision is recorded in the automation audit under `focus_goal_mode`.

Goal mode ends when the pipeline finishes, when you toggle it off, stop the pipeline, or stop a running turn, and when someone closes the pull request. It does not act when `gh` cannot run on the server: the pull request stages then wait to be passed by hand, as they do without goal mode.

Turning goal mode on needs the admin token, because it approves the merge and every tool call. A verification command that can never pass (a wrong command approved with the plan) uses up the whole budget; check the plan's commands when a goal-mode task stops that way.
