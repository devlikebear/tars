#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
TEMPLATE_DIR="$SCRIPT_DIR/template"

usage() {
  cat <<'EOF'
usage: ./bootstrap-demo-repo.sh <destination-dir> [--github owner/name] [--public]

Creates a standalone demo repository from the ops-service template.

Examples:
  ./bootstrap-demo-repo.sh ../ops-demo
  ./bootstrap-demo-repo.sh ../ops-demo --github your-org/ops-demo
EOF
}

if [ $# -lt 1 ]; then
  usage >&2
  exit 1
fi

DEST_DIR=$1
shift

GITHUB_REPO=""
VISIBILITY="private"
while [ $# -gt 0 ]; do
  case "$1" in
    --github)
      if [ $# -lt 2 ]; then
        printf '%s\n' '--github requires owner/name' >&2
        exit 1
      fi
      GITHUB_REPO=$2
      shift 2
      ;;
    --public)
      VISIBILITY="public"
      shift
      ;;
    -h|--help|help)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      exit 1
      ;;
  esac
done

if [ ! -d "$TEMPLATE_DIR" ]; then
  printf 'template directory not found: %s\n' "$TEMPLATE_DIR" >&2
  exit 1
fi

mkdir -p "$DEST_DIR"
if [ -n "$(find "$DEST_DIR" -mindepth 1 -maxdepth 1 2>/dev/null | head -n 1)" ]; then
  printf 'destination must be empty: %s\n' "$DEST_DIR" >&2
  exit 1
fi

cp -R "$TEMPLATE_DIR"/. "$DEST_DIR"/
chmod +x "$DEST_DIR/opsctl"

if command -v git >/dev/null 2>&1; then
  git -C "$DEST_DIR" init -b main >/dev/null
  git -C "$DEST_DIR" add . >/dev/null
  if ! git -C "$DEST_DIR" commit -m "chore: bootstrap ops service demo" >/dev/null 2>&1; then
    printf 'warning: initial git commit skipped; configure git user.name/email and commit manually\n' >&2
  fi
fi

if [ -n "$GITHUB_REPO" ]; then
  if ! command -v gh >/dev/null 2>&1; then
    printf 'gh is required for --github\n' >&2
    exit 1
  fi
  gh repo create "$GITHUB_REPO" --source "$DEST_DIR" --push "--$VISIBILITY"
fi

printf 'bootstrapped demo repo: %s\n' "$DEST_DIR"
printf 'next steps:\n'
printf '  cd %s\n' "$DEST_DIR"
printf '  docker compose up -d --build\n'
printf '  ./opsctl status\n'
