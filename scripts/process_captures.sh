#!/bin/sh
# Post-process `make console-screenshots` output:
#   - raw PNGs (frontend/console/e2e/capture/output/raw/*.png) -> webp under
#     docs/screenshots/, quality stepped down if a file lands over ~200KB.
#   - the single recorded Playwright video (output/test-results/**/*.webm)
#     -> a trimmed, scaled demo clip (mp4 + webm + jpg poster) under
#     output/media/, capped at 25s and sized for a <=4MB budget.
#
# Requires cwebp and ffmpeg on PATH. Run via `make console-screenshots`
# after the capture spec; never run standalone against stale output.
set -eu

repo_root=$(cd "$(dirname "$0")/.." && pwd)
raw_dir="$repo_root/frontend/console/e2e/capture/output/raw"
results_dir="$repo_root/frontend/console/e2e/capture/output/test-results"
media_dir="$repo_root/frontend/console/e2e/capture/output/media"
screenshots_dir="$repo_root/docs/screenshots"

command -v cwebp >/dev/null 2>&1 || { echo "process_captures: cwebp not found on PATH" >&2; exit 1; }
command -v ffmpeg >/dev/null 2>&1 || { echo "process_captures: ffmpeg not found on PATH" >&2; exit 1; }
[ -d "$raw_dir" ] || { echo "process_captures: $raw_dir missing — run the capture spec first" >&2; exit 1; }

mkdir -p "$screenshots_dir" "$media_dir"

max_bytes=204800 # ~200KB
for png in "$raw_dir"/*.png; do
  [ -e "$png" ] || continue
  name=$(basename "$png" .png)
  out="$screenshots_dir/$name.webp"
  quality=82
  cwebp -quiet -q "$quality" "$png" -o "$out"
  size=$(wc -c <"$out" | tr -d ' ')
  while [ "$size" -gt "$max_bytes" ] && [ "$quality" -gt 40 ]; do
    quality=$((quality - 10))
    cwebp -quiet -q "$quality" "$png" -o "$out"
    size=$(wc -c <"$out" | tr -d ' ')
  done
  echo "wrote $out (${size} bytes, q=$quality)"
done

video=$(find "$results_dir" -name '*.webm' -print -quit 2>/dev/null || true)
if [ -z "$video" ]; then
  echo "process_captures: no recorded .webm under $results_dir — skipping video" >&2
else
  echo "encoding demo clip from $video"
  # Trim to 25s, scale to a 1280-wide frame, strip audio (screen recordings
  # carry none anyway), two bitrate-capped encodes plus a poster frame.
  ffmpeg -y -loglevel error -i "$video" -t 25 -vf "scale=1280:-2" -an \
    -c:v libx264 -preset slow -crf 30 -maxrate 1200k -bufsize 2400k -movflags +faststart \
    "$media_dir/demo.mp4"
  ffmpeg -y -loglevel error -i "$video" -t 25 -vf "scale=1280:-2" -an \
    -c:v libvpx-vp9 -b:v 1000k -crf 34 \
    "$media_dir/demo.webm"
  # Seek past the initial "Connecting to TARS..." loading splash so the
  # poster shows real UI, not a loading screen.
  ffmpeg -y -loglevel error -ss 2 -i "$video" -frames:v 1 -vf "scale=1280:-2" "$media_dir/demo-poster.jpg"
  for f in demo.mp4 demo.webm; do
    size=$(wc -c <"$media_dir/$f" | tr -d ' ')
    echo "wrote $media_dir/$f (${size} bytes)"
    if [ "$size" -gt 4194304 ]; then
      echo "process_captures: WARNING $f is over the 4MB budget (${size} bytes) — re-encode with a lower bitrate" >&2
    fi
  done
fi

echo "done. Review docs/screenshots/*.webp and $media_dir/demo.* by eye before committing."
