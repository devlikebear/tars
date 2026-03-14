# Project Manager Example

This example walks through the current project manager flow in TARS:

1. Create a project.
2. Seed the board with one developer task and one reviewer task.
3. Open the dashboard or tail the SSE stream.
4. Inspect the project from the TUI.
5. Dispatch developer work.
6. Dispatch reviewer work or start autopilot.

The example assumes the local server is running at `http://127.0.0.1:43180`.

## Prerequisites

- Start the API server:

```bash
tars serve --config ./workspace/config/tars.config.yaml
```

- Optional but recommended for GitHub Flow checks:

```bash
gh auth status
```

## 1. Create A Project

Create the project and capture the returned project ID:

```bash
PROJECT_ID="$(
  curl -s http://127.0.0.1:43180/v1/projects \
    -H 'Content-Type: application/json' \
    -d @examples/project/create-project.json | jq -r '.id'
)"
echo "$PROJECT_ID"
```

## 2. Seed The Board

Apply the example board payload:

```bash
curl -s "http://127.0.0.1:43180/v1/projects/${PROJECT_ID}/board" \
  -X PATCH \
  -H 'Content-Type: application/json' \
  -d @examples/project/board.json
```

The seeded tasks demonstrate the required metadata for the GitHub Flow and TDD gate:

- `test_command`
- `build_command`
- `issue`
- `branch`
- `pr`

## 3. Monitor Progress

Open the server-rendered dashboard:

```bash
open "http://127.0.0.1:43180/ui/projects/${PROJECT_ID}"
```

Or tail the live event stream:

```bash
curl -N "http://127.0.0.1:43180/ui/projects/${PROJECT_ID}/stream"
```

## 4. Inspect From The TUI

Open the client and inspect the current project state:

```text
/project board ${PROJECT_ID}
/project activity ${PROJECT_ID} 20
```

If you want TARS to continue from the backlog automatically:

```text
/project autopilot start ${PROJECT_ID}
/project autopilot status ${PROJECT_ID}
```

## 5. Dispatch Developer Work

Dispatch all `todo` tasks:

```bash
curl -s "http://127.0.0.1:43180/v1/projects/${PROJECT_ID}/dispatch" \
  -X POST \
  -H 'Content-Type: application/json' \
  -d @examples/project/dispatch-todo.json
```

At this stage, TARS will attempt to run the assigned developer workers and move tasks toward `review`.

The same action is available from the TUI:

```text
/project dispatch ${PROJECT_ID} todo
```

## 6. Dispatch Reviewer Work

Dispatch all `review` tasks:

```bash
curl -s "http://127.0.0.1:43180/v1/projects/${PROJECT_ID}/dispatch" \
  -X POST \
  -H 'Content-Type: application/json' \
  -d @examples/project/dispatch-review.json
```

Tasks that require review move to `done` only after the reviewer approves them.

From the TUI, the equivalent command is:

```text
/project dispatch ${PROJECT_ID} review
```

## Inspect Current State

Read the latest board snapshot:

```bash
curl -s "http://127.0.0.1:43180/v1/projects/${PROJECT_ID}/board"
```

Read the latest activity feed:

```bash
curl -s "http://127.0.0.1:43180/v1/projects/${PROJECT_ID}/activity"
```

## Notes

- This example now supports both TUI commands and direct API calls.
- The example assumes the configured worker backends (`codex` or `claude`) are available in the local environment.
- If `gh auth status` fails, GitHub Flow validation will surface that failure in task activity and the dashboard.
