#!/bin/sh
# Stands in for a project's development server in the console e2e server
# (TARS_EASY_PREVIEW_PATH, internal/easyproject/preview.go): serves a fixed
# page on 127.0.0.1 with no npm and no Vite. $1 is the project folder, $2
# the port.
set -eu
exec node "$(dirname "$0")/fake-preview.mjs" "$2"
