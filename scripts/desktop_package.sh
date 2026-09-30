#!/usr/bin/env bash
# Package the desktop shell as a release archive.
#
#   scripts/desktop_package.sh <goos> <goarch> <version> <outdir>
#
# Each archive holds exactly one top-level entry, because that is what the
# shell's self-updater swaps in place of the running app:
#
#   darwin   tars-desktop_<v>_darwin_<arch>.tar.gz   TARS.app
#   linux    tars-desktop_<v>_linux_<arch>.tar.gz    tars-desktop
#   windows  tars-desktop_<v>_windows_<arch>.zip     tars-desktop.exe
#
# darwin must be built on macOS and linux on Linux (both need cgo for the
# webview); windows cross-compiles from anywhere without cgo.
#
# macOS signing: with DESKTOP_CODESIGN_IDENTITY set, the bundle is signed
# with that identity and the hardened runtime; otherwise it is ad-hoc signed,
# which runs locally but makes Gatekeeper ask the first time it is opened.
set -euo pipefail

if [ "$#" -ne 4 ]; then
  echo "usage: $0 <goos> <goarch> <version> <outdir>" >&2
  exit 2
fi
goos="$1"
goarch="$2"
version="$3"
mkdir -p "$4"
outdir="$(cd "$4" && pwd)"

root="$(cd "$(dirname "$0")/.." && pwd)"
desktop="${root}/desktop"
stage="$(mktemp -d)"
syso=""
cleanup() {
  rm -rf "${stage}"
  if [ -n "${syso}" ]; then rm -f "${syso}"; fi
}
trap cleanup EXIT

ldflags="-s -w -X main.version=${version}"
base="tars-desktop_${version}_${goos}_${goarch}"

build() {
  (cd "${desktop}" && GOOS="${goos}" GOARCH="${goarch}" go build -trimpath -tags production -ldflags "${ldflags}${1:+ $1}" -o "$2" .)
}

case "${goos}" in
  darwin)
    app="${stage}/TARS.app"
    mkdir -p "${app}/Contents/MacOS" "${app}/Contents/Resources"
    CGO_ENABLED=1 build "" "${app}/Contents/MacOS/TARS"
    sed "s/@VERSION@/${version}/g" "${desktop}/build/darwin/Info.plist" > "${app}/Contents/Info.plist"

    (cd "${desktop}" && go run ./tools/appicon -size 1024 -out "${stage}/appicon.png")
    iconset="${stage}/icon.iconset"
    mkdir -p "${iconset}"
    for size in 16 32 128 256 512; do
      sips -z "${size}" "${size}" "${stage}/appicon.png" --out "${iconset}/icon_${size}x${size}.png" >/dev/null
      double=$((size * 2))
      sips -z "${double}" "${double}" "${stage}/appicon.png" --out "${iconset}/icon_${size}x${size}@2x.png" >/dev/null
    done
    iconutil -c icns -o "${app}/Contents/Resources/icon.icns" "${iconset}"

    if [ -n "${DESKTOP_CODESIGN_IDENTITY:-}" ]; then
      codesign --force --options runtime --timestamp --sign "${DESKTOP_CODESIGN_IDENTITY}" "${app}"
    else
      codesign --force --sign - "${app}"
    fi
    tar -C "${stage}" -czf "${outdir}/${base}.tar.gz" TARS.app
    ;;
  linux)
    CGO_ENABLED=1 build "" "${stage}/tars-desktop"
    tar -C "${stage}" -czf "${outdir}/${base}.tar.gz" tars-desktop
    ;;
  windows)
    (cd "${desktop}" && go run ./tools/winres -version "${version}" -arch "${goarch}")
    syso="${desktop}/rsrc_windows_${goarch}.syso"
    CGO_ENABLED=0 build "-H windowsgui" "${stage}/tars-desktop.exe"
    (cd "${stage}" && zip -q "${outdir}/${base}.zip" tars-desktop.exe)
    ;;
  *)
    echo "unsupported GOOS: ${goos}" >&2
    exit 2
    ;;
esac

echo "${outdir}/${base}"
