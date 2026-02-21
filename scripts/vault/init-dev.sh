#!/bin/sh
set -eu

VAULT_ADDR="${VAULT_ADDR:-http://vault:8200}"
VAULT_TOKEN="${VAULT_TOKEN:-dev-only-token}"
TARS_VAULT_MOUNT="${TARS_VAULT_MOUNT:-tars}"
TARS_POLICY_NAME="${TARS_POLICY_NAME:-tars-readonly}"
TARS_SECRET_PATH="${TARS_SECRET_PATH:-sites/grafana}"
TARS_SECRET_USERNAME="${TARS_SECRET_USERNAME:-demo-user}"
TARS_SECRET_PASSWORD="${TARS_SECRET_PASSWORD:-demo-pass}"

export VAULT_ADDR
export VAULT_TOKEN

echo "[vault-init] waiting for Vault at ${VAULT_ADDR}"
i=0
while [ "$i" -lt 30 ]; do
  if vault status >/dev/null 2>&1; then
    break
  fi
  i=$((i + 1))
  sleep 1
done

if ! vault status >/dev/null 2>&1; then
  echo "[vault-init] Vault is not ready"
  exit 1
fi

echo "[vault-init] configuring KV mount (${TARS_VAULT_MOUNT})"
if ! vault secrets list | grep -q "^${TARS_VAULT_MOUNT}/"; then
  vault secrets enable -path="${TARS_VAULT_MOUNT}" -version=2 kv >/dev/null
fi

echo "[vault-init] writing sample secret (${TARS_SECRET_PATH})"
vault kv put -mount="${TARS_VAULT_MOUNT}" "${TARS_SECRET_PATH}" \
  username="${TARS_SECRET_USERNAME}" \
  password="${TARS_SECRET_PASSWORD}" >/dev/null

cat > /tmp/tars-policy.hcl <<POLICY
path "${TARS_VAULT_MOUNT}/data/sites/*" {
  capabilities = ["read"]
}
POLICY

echo "[vault-init] writing readonly policy (${TARS_POLICY_NAME})"
vault policy write "${TARS_POLICY_NAME}" /tmp/tars-policy.hcl >/dev/null

APP_TOKEN="$(vault token create -policy="${TARS_POLICY_NAME}" -ttl=24h -field=token)"

echo "[vault-init] done"
echo "[vault-init] readonly token: ${APP_TOKEN}"
echo "[vault-init] set in tars config: vault_token: ${APP_TOKEN}"
