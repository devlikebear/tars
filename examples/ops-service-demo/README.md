# Ops Service Example

This example demonstrates a concrete TARS ops loop:

1. run a local Dockerized demo service
2. inspect logs with a small operational CLI
3. create or reuse GitHub issues for recurring failures
4. implement a safe fix in a separate demo repo
5. open a PR
6. send periodic human-readable reports

The example keeps the core runtime unchanged. It uses:

- a bundled `ops-service` plugin
- a standalone demo repository bootstrapped from `examples/ops-service-demo/template/`
- the existing phase-based project autopilot
- cron jobs for recurring triage and reporting
- Telegram as a report channel only

From the TARS repo root, the bundled template itself is testable with:

```bash
go test ./examples/ops-service-demo/...
```

The standalone demo repo gets its own `go.mod` when you bootstrap it with the script below.

## Prerequisites

- local TARS server running
- Docker and Docker Compose
- `gh auth status` succeeds
- a TARS workspace created with `tars init` or `tars doctor --fix`

If the workspace was initialized from a recent build, the bundled `ops-service` plugin should already exist at `workspace/plugins/ops-service`.

## 1. Bootstrap The Seed Repo

Create a standalone seed repo outside this TARS checkout. Use it only to seed or publish the demo repository. Do not treat this path as the long-running runtime repo once the TARS project exists.

```bash
examples/ops-service-demo/bootstrap-demo-repo.sh ../ops-service-demo-seed
cd ../ops-service-demo-seed
```

Optionally publish the demo repo to GitHub so issue/PR commands work against a remote:

```bash
gh repo create your-org/ops-service-demo --source . --push --private
```

Capture the clone URL for the next step:

```bash
DEMO_REPO_URL="$(gh repo view --json url -q .url)"
echo "$DEMO_REPO_URL"
```

## 2. Create The TARS Project

Render the project payload with your demo repo URL:

```bash
perl -0pe "s#__DEMO_REPO_URL__#${DEMO_REPO_URL}#g" \
  examples/ops-service-demo/project/create-project.template.json > /tmp/ops-create.json
```

Create the project:

```bash
PROJECT_ID="$(
  curl -s http://127.0.0.1:43180/v1/projects \
    -H 'Content-Type: application/json' \
    -d @/tmp/ops-create.json | jq -r '.id'
)"
echo "$PROJECT_ID"
```

Patch the project to limit its skill/tool surface to the ops workflow:

```bash
perl -0pe "s#__PROJECT_ID__#${PROJECT_ID}#g" \
  examples/ops-service-demo/project/update-project-policy.template.json > /tmp/ops-update.json

curl -s "http://127.0.0.1:43180/v1/projects/${PROJECT_ID}" \
  -X PATCH \
  -H 'Content-Type: application/json' \
  -d @/tmp/ops-update.json
```

Inspect the project in the CLI or console:

```text
/project get ${PROJECT_ID}
/project autopilot status ${PROJECT_ID}
```

Use the cloned project repo as the authoritative runtime repo from this point forward so cron jobs and your manual checks operate on the same path:

```bash
PROJECT_REPO_DIR="$(
  curl -s "http://127.0.0.1:43180/v1/projects/${PROJECT_ID}" | jq -r '.path + "/repo"'
)"
echo "$PROJECT_REPO_DIR"

docker compose -f "${PROJECT_REPO_DIR}/docker-compose.yml" up -d --build
"${PROJECT_REPO_DIR}/opsctl" status
"${PROJECT_REPO_DIR}/opsctl" inject-failure timeout
"${PROJECT_REPO_DIR}/opsctl" errors
```

Docker Compose now uses the default project-scoped container names, so repeated seed repos no longer collide on a fixed `ops-service-demo` container. The host port is still `18080`, so only one copy of the demo service should be running at a time.

If you started the service earlier from the standalone seed repo, stop it before switching:

```bash
docker compose -f ../ops-service-demo-seed/docker-compose.yml down || true
```

## 3. Register Cron Jobs

Create the log triage job:

```bash
perl -0pe "s#__PROJECT_ID__#${PROJECT_ID}#g" \
  examples/ops-service-demo/cron/triage-logs.template.json > /tmp/ops-triage.json

curl -s http://127.0.0.1:43180/v1/cron/jobs \
  -H 'Content-Type: application/json' \
  -d @/tmp/ops-triage.json
```

Create the periodic report job:

```bash
perl -0pe "s#__PROJECT_ID__#${PROJECT_ID}#g" \
  examples/ops-service-demo/cron/report-project.template.json > /tmp/ops-report.json

curl -s http://127.0.0.1:43180/v1/cron/jobs \
  -H 'Content-Type: application/json' \
  -d @/tmp/ops-report.json
```

Run the triage job once immediately:

```bash
JOB_ID="$(
  curl -s http://127.0.0.1:43180/v1/cron/jobs |
    jq -r --arg pid "${PROJECT_ID}" '.[] | select(.name=="triage-logs" and .project_id==$pid) | .id'
)"
curl -s -X POST "http://127.0.0.1:43180/v1/cron/jobs/${JOB_ID}/run"
```

## 4. Observe The Loop

Use the console or CLI:

```bash
open "http://127.0.0.1:43180/console/projects/${PROJECT_ID}"
curl -N "http://127.0.0.1:43180/v1/events/stream?project_id=${PROJECT_ID}"
```

```text
/project activity ${PROJECT_ID} 20
/project autopilot advance ${PROJECT_ID}
/project autopilot start ${PROJECT_ID}
/project autopilot status ${PROJECT_ID}
```

Confirm the tracking artifacts in the demo repo:

```bash
gh issue list --repo "${DEMO_REPO_URL#https://github.com/}"
gh pr list --repo "${DEMO_REPO_URL#https://github.com/}"
```

## Human In The Loop

For this example, Telegram is report-only. Use the console, CLI, or API when TARS needs approval to:

- accept the next phase plan
- continue after an unclear root cause
- continue after repeated failures
- merge or deploy a PR

## Escalation Rules

TARS should stop and report rather than continue autonomously when:

- the service needs a risky restart or rollback
- the failure signature is ambiguous
- the same remediation fails repeatedly
- GitHub auth or repo permissions are missing
- the fix is ready for merge/deploy
