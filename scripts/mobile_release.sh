#!/usr/bin/env bash
# Build the iOS app for the App Store, and upload it when asked.
#
#   scripts/mobile_release.sh <version> <build> [--upload]
#
#   version  the App Store version (CFBundleShortVersionString), e.g. 1.0.0.
#            The app's own, not the server's VERSION.txt.
#   build    the build number (CFBundleVersion): an integer higher than every
#            build already uploaded for this version.
#
# Without --upload it stops after validating the .ipa with App Store Connect
# (nothing is published); with it the build goes to App Store Connect, where
# it shows under TestFlight after processing. Submitting for review is done
# in App Store Connect, by a person.
#
# Signing is automatic and needs no distribution certificate in the
# keychain: xcodebuild asks App Store Connect for one with the API key.
# That also registers the bundle id and a provisioning profile on the team
# the first time, so the key needs the Admin or App Manager role.
#
# Environment (names only; the values stay out of the repository):
#   APPLE_TEAM_ID   the developer team
#   ASC_KEY_ID      App Store Connect API key id
#   ASC_ISSUER_ID   its issuer id
#   ASC_KEY_PATH    path to AuthKey_<ASC_KEY_ID>.p8
#   ASC_APP_ID      (optional) the app's numeric Apple ID. altool cannot find
#                   an app by bundle id before its first iOS build exists, so
#                   the first upload needs it.
#   MOBILE_OUT      (optional) output folder; default dist/mobile
set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "usage: $0 <version> <build> [--upload]" >&2
  exit 2
fi
version="$1"
build="$2"
upload="${3:-}"
if [ -n "${upload}" ] && [ "${upload}" != "--upload" ]; then
  echo "unknown argument: ${upload}" >&2
  exit 2
fi
case "${build}" in
  ''|*[!0-9]*) echo "build must be an integer: ${build}" >&2; exit 2 ;;
esac

: "${APPLE_TEAM_ID:?APPLE_TEAM_ID is required}"
: "${ASC_KEY_ID:?ASC_KEY_ID is required}"
: "${ASC_ISSUER_ID:?ASC_ISSUER_ID is required}"
: "${ASC_KEY_PATH:?ASC_KEY_PATH is required}"
if [ ! -f "${ASC_KEY_PATH}" ]; then
  echo "ASC_KEY_PATH does not name a file" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
bundle_id="com.devlikebear.tars.mobile"
out="${MOBILE_OUT:-${root}/dist/mobile}"
mkdir -p "${out}"
out="$(cd "${out}" && pwd)"
archive="${out}/TARS.xcarchive"
auth=(-allowProvisioningUpdates
  -authenticationKeyPath "${ASC_KEY_PATH}"
  -authenticationKeyID "${ASC_KEY_ID}"
  -authenticationKeyIssuerID "${ASC_ISSUER_ID}")

# The release web bundle: it pairs only over https and keeps desktops in the
# Keychain. `npm run sync` builds it and copies it into the iOS project.
if [ ! -d "${root}/frontend/console/node_modules" ]; then
  npm --prefix "${root}/frontend/console" ci --no-audit --no-fund
fi
if [ ! -d "${root}/mobile/node_modules" ]; then
  npm --prefix "${root}/mobile" ci --no-audit --no-fund
fi
npm --prefix "${root}/mobile" run sync

rm -rf "${archive}" "${out}/export"
xcodebuild archive \
  -project "${root}/mobile/ios/App/App.xcodeproj" \
  -scheme App \
  -configuration Release \
  -destination "generic/platform=iOS" \
  -archivePath "${archive}" \
  "${auth[@]}" \
  DEVELOPMENT_TEAM="${APPLE_TEAM_ID}" \
  MARKETING_VERSION="${version}" \
  CURRENT_PROJECT_VERSION="${build}"

options="${out}/ExportOptions.plist"
cat > "${options}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>method</key>
	<string>app-store-connect</string>
	<key>destination</key>
	<string>export</string>
	<key>teamID</key>
	<string>${APPLE_TEAM_ID}</string>
	<key>signingStyle</key>
	<string>automatic</string>
	<key>manageAppVersionAndBuildNumber</key>
	<false/>
</dict>
</plist>
EOF

xcodebuild -exportArchive \
  -archivePath "${archive}" \
  -exportPath "${out}/export" \
  -exportOptionsPlist "${options}" \
  "${auth[@]}"

ipa="$(find "${out}/export" -maxdepth 1 -name '*.ipa' | head -n 1)"
if [ -z "${ipa}" ]; then
  echo "no .ipa in ${out}/export" >&2
  exit 1
fi

# altool reads the key as AuthKey_<id>.p8 from API_PRIVATE_KEYS_DIR.
keys="$(cd "$(dirname "${ASC_KEY_PATH}")" && pwd)"
if [ "$(basename "${ASC_KEY_PATH}")" != "AuthKey_${ASC_KEY_ID}.p8" ]; then
  echo "ASC_KEY_PATH must be named AuthKey_${ASC_KEY_ID}.p8 for altool" >&2
  exit 2
fi
altool() {
  API_PRIVATE_KEYS_DIR="${keys}" xcrun altool "$@" --apiKey "${ASC_KEY_ID}" --apiIssuer "${ASC_ISSUER_ID}"
}
# The first iOS build of an app is named in full; later ones are found by
# the bundle id inside the .ipa.
target=(-f "${ipa}" -t ios)
if [ -n "${ASC_APP_ID:-}" ]; then
  target+=(--apple-id "${ASC_APP_ID}" --bundle-id "${bundle_id}" --bundle-short-version-string "${version}" --bundle-version "${build}")
fi

altool --validate-app "${target[@]}"
if [ "${upload}" = "--upload" ]; then
  altool --upload-app "${target[@]}"
  echo "uploaded ${version} (${build}); it shows under TestFlight once App Store Connect has processed it"
else
  echo "validated ${ipa}; run again with --upload to send it"
fi
