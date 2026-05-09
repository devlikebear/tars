#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "[security-scan] running gitleaks"
go -C tools tool github.com/zricethezav/gitleaks/v8 detect --source .. --no-banner --redact

echo "[security-scan] checking tracked files for absolute local paths"
set +e
ABS_PATH_MATCHES="$(git ls-files -z | xargs -0 rg -n --no-heading '(/Users/[A-Za-z0-9_-]+/|[A-Za-z]:\\Users\\[A-Za-z0-9_-]+\\)' 2>/dev/null)"
set -e
if [[ -n "${ABS_PATH_MATCHES}" ]]; then
  echo "[security-scan] found absolute local paths in tracked files:" >&2
  echo "${ABS_PATH_MATCHES}" >&2
  exit 1
fi

echo "[security-scan] checking tracked files for private key blocks"
set +e
KEY_MATCHES="$(git ls-files -z | xargs -0 rg -n --no-heading '-----BEGIN (RSA|OPENSSH|EC|DSA|PGP)? ?PRIVATE KEY-----' 2>/dev/null)"
set -e
if [[ -n "${KEY_MATCHES}" ]]; then
  echo "[security-scan] found private key markers in tracked files:" >&2
  echo "${KEY_MATCHES}" >&2
  exit 1
fi

echo "[security-scan] passed"
