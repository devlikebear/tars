#!/bin/sh
# Stands in for Cloudflare's scaffolder in the console e2e server
# (TARS_EASY_SCAFFOLD_PATH, internal/easyproject): writes the files a
# scaffolded web app starts with, with no npm and no network. $1 is the new,
# empty project folder.
set -eu
dir="$1"
# An idea that starts with "Scaffold fails once" (easy-states.spec.ts) fails
# the first time, as a lost connection would, and works when tried again.
case "$(basename "$dir")" in
scaffold-fails-once*)
	marker="$(dirname "$dir")/.e2e-scaffold-failed"
	if [ ! -f "$marker" ]; then
		: > "$marker"
		echo "could not reach the template server" >&2
		exit 1
	fi
	;;
esac
printf '{"name":"app","private":true,"scripts":{"build":"true"}}\n' > "$dir/package.json"
printf 'node_modules\n.wrangler\n' > "$dir/.gitignore"
mkdir -p "$dir/src" "$dir/worker"
printf 'export default function App() { return null }\n' > "$dir/src/App.tsx"
printf 'export default { fetch() { return new Response(null, { status: 404 }) } }\n' > "$dir/worker/index.ts"
