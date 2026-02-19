#!/usr/bin/env bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke script" >&2
  exit 1
fi

SERVER_URL="${SERVER_URL:-http://127.0.0.1:43180}"
USER_TOKEN="${USER_TOKEN:-}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
USER_WORKSPACE="${USER_WORKSPACE:-team-a}"
ADMIN_WORKSPACE="${ADMIN_WORKSPACE:-team-admin}"

if [[ -z "${USER_TOKEN}" || -z "${ADMIN_TOKEN}" ]]; then
  echo "set USER_TOKEN and ADMIN_TOKEN first" >&2
  exit 1
fi

echo "[1/5] whoami (user)"
curl -sS \
  -H "Authorization: Bearer ${USER_TOKEN}" \
  -H "Tars-Workspace-Id: ${USER_WORKSPACE}" \
  "${SERVER_URL}/v1/auth/whoami" \
  | jq -e '.authenticated == true and .auth_role == "user" and .workspace_id == "'"${USER_WORKSPACE}"'"' >/dev/null

echo "[2/5] whoami (admin)"
curl -sS \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Tars-Workspace-Id: ${ADMIN_WORKSPACE}" \
  "${SERVER_URL}/v1/auth/whoami" \
  | jq -e '.authenticated == true and .auth_role == "admin" and .is_admin == true' >/dev/null

echo "[3/5] user blocked on admin endpoint"
user_status="$(curl -sS -o /tmp/tars_smoke_user_reload.json -w '%{http_code}' \
  -X POST \
  -H "Authorization: Bearer ${USER_TOKEN}" \
  -H "Tars-Workspace-Id: ${USER_WORKSPACE}" \
  "${SERVER_URL}/v1/gateway/reload")"
[[ "${user_status}" == "403" ]]

echo "[4/5] admin allowed on admin endpoint"
admin_status="$(curl -sS -o /tmp/tars_smoke_admin_reload.json -w '%{http_code}' \
  -X POST \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Tars-Workspace-Id: ${ADMIN_WORKSPACE}" \
  "${SERVER_URL}/v1/gateway/reload")"
[[ "${admin_status}" == "200" ]]

echo "[5/5] workspace echo on status"
curl -sS \
  -H "Authorization: Bearer ${USER_TOKEN}" \
  -H "Tars-Workspace-Id: ${USER_WORKSPACE}" \
  "${SERVER_URL}/v1/status" \
  | jq -e '.workspace_id == "'"${USER_WORKSPACE}"'"' >/dev/null

echo "smoke_auth_workspace: PASS"
