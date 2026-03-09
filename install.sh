#!/usr/bin/env sh
set -eu

REPO_SLUG="${REPO_SLUG:-devlikebear/tars}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
REPOSITORY_URL="${REPOSITORY_URL:-https://github.com/$REPO_SLUG}"
LATEST_RELEASE_URL="${LATEST_RELEASE_URL:-$REPOSITORY_URL/releases/latest}"
RELEASE_BASE_URL="${RELEASE_BASE_URL:-$REPOSITORY_URL/releases/download}"

log() {
  printf '%s\n' "$*" >&2
}

fail() {
  log "install.sh: $*"
  exit 1
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

detect_goarch() {
  case "$(uname -m)" in
    arm64)
      printf 'arm64\n'
      ;;
    x86_64|amd64)
      printf 'amd64\n'
      ;;
    *)
      fail "unsupported architecture: $(uname -m)"
      ;;
  esac
}

detect_goos() {
  case "$(uname -s)" in
    Darwin)
      printf 'darwin\n'
      ;;
    *)
      fail "unsupported operating system: $(uname -s)"
      ;;
  esac
}

fetch_latest_version() {
  if ! resolved_url="$(curl -fsSIL -o /dev/null -w '%{url_effective}' "$LATEST_RELEASE_URL")"; then
    fail "failed to fetch latest release from $LATEST_RELEASE_URL"
  fi
  tag_name="${resolved_url##*/}"
  version="${tag_name#v}"
  if [ -z "$tag_name" ] || [ -z "$version" ] || [ "$tag_name" = "$version" ]; then
    fail "failed to resolve latest release version from $LATEST_RELEASE_URL"
  fi
  printf '%s\n' "$version"
}

download_asset() {
  asset_url="$1"
  destination="$2"
  if ! curl -fsSL "$asset_url" >"$destination"; then
    fail "failed to download release asset: $asset_url"
  fi
}

require_cmd curl
require_cmd tar
require_cmd uname
require_cmd install
require_cmd mktemp
require_cmd find

GOOS="$(detect_goos)"
GOARCH="$(detect_goarch)"
VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
  VERSION="$(fetch_latest_version)"
fi
VERSION="$(printf '%s' "$VERSION" | tr -d '[:space:]')"
[ -n "$VERSION" ] || fail "release version is empty"

ARCHIVE_NAME="tars_${VERSION}_${GOOS}_${GOARCH}.tar.gz"
ASSET_URL="$RELEASE_BASE_URL/v$VERSION/$ARCHIVE_NAME"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' 0 INT HUP TERM

ARCHIVE_PATH="$TMP_DIR/$ARCHIVE_NAME"
download_asset "$ASSET_URL" "$ARCHIVE_PATH"
mkdir -p "$INSTALL_DIR"
if ! tar -xzf "$ARCHIVE_PATH" -C "$TMP_DIR"; then
  fail "failed to extract archive: $ARCHIVE_PATH"
fi
EXTRACTED_BINARY="$(find "$TMP_DIR" -type f -name tars -print -quit)"
if [ -z "$EXTRACTED_BINARY" ]; then
  fail "release archive did not contain a tars binary: $ASSET_URL"
fi

TARGET_PATH="$INSTALL_DIR/tars"
install -m 0755 "$EXTRACTED_BINARY" "$TARGET_PATH"
if ! "$TARGET_PATH" --version >/dev/null 2>&1; then
  fail "installed binary failed version check: $TARGET_PATH --version"
fi

printf 'Installed tars to %s\n' "$TARGET_PATH"
