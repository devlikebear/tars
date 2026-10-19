#!/bin/zsh
# Build a comparison sheet: each style at 256/64/32 px on the console's dark background.
# Usage: compare.sh out.png in1.png in2.png ...
set -e
out=$1; shift
S=${0:A:h}
tiles=()
i=0
for f in "$@"; do
  t="$S/tile-$i.png"
  ffmpeg -hide_banner -loglevel error -y \
    -f lavfi -i "color=c=0x141414:s=400x300" -i "$f" \
    -filter_complex "[1:v]split=3[a][b][c];[a]scale=256:256:force_original_aspect_ratio=decrease:flags=lanczos[big];[b]scale=64:64:force_original_aspect_ratio=decrease:flags=lanczos[mid];[c]scale=32:32:force_original_aspect_ratio=decrease:flags=lanczos[sm];[0:v][big]overlay=12:22[t1];[t1][mid]overlay=290:60[t2];[t2][sm]overlay=306:170" \
    -frames:v 1 "$t"
  tiles+=("$t")
  i=$((i+1))
done
inputs=()
for t in $tiles; do inputs+=(-i "$t"); done
if (( ${#tiles} > 1 )); then
  ffmpeg -hide_banner -loglevel error -y $inputs -filter_complex "hstack=inputs=${#tiles}" -frames:v 1 "$out"
else
  cp "${tiles[1]}" "$out"
fi
rm -f $tiles
echo "$out"
