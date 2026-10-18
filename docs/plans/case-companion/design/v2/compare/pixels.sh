#!/bin/zsh
# Show what 64px really looks like: full body vs. bust crop (top 62%), each
# downscaled to 64px on the console background, then blown up 4x with nearest
# neighbour so the actual pixels are visible.
# Usage: pixels.sh out.png in1.png in2.png ...
set -e
out=$1; shift
S=${0:A:h}
rows=()
i=0
for f in "$@"; do
  r="$S/prow-$i.png"
  ffmpeg -hide_banner -loglevel error -y \
    -f lavfi -i "color=c=0x141414:s=64x64" -f lavfi -i "color=c=0x141414:s=64x64" -i "$f" \
    -filter_complex "[2:v]split=2[a][b];[a]scale=64:64:force_original_aspect_ratio=decrease:flags=lanczos[full];[b]crop=iw*0.8:ih*0.62:iw*0.1:0,scale=64:64:force_original_aspect_ratio=decrease:flags=lanczos[bust];[0:v][full]overlay=(W-w)/2:(H-h)/2[f];[1:v][bust]overlay=(W-w)/2:(H-h)/2[c];[f][c]hstack=inputs=2,scale=iw*4:ih*4:flags=neighbor,pad=iw+16:ih+16:8:8:color=0x2a2a2a" \
    -frames:v 1 "$r"
  rows+=("$r")
  i=$((i+1))
done
inputs=()
for r in $rows; do inputs+=(-i "$r"); done
if (( ${#rows} > 1 )); then
  ffmpeg -hide_banner -loglevel error -y $inputs -filter_complex "hstack=inputs=${#rows}" -frames:v 1 "$out"
else
  cp "${rows[1]}" "$out"
fi
rm -f $rows
echo "$out"
