#!/bin/zsh
# V0 master: from bot-b, make a blank-screen frame and one pose (wave) frame, in parallel.
S=${0:A:h}
M="$S/master/master.png"
mkdir -p "$S/master"
cp "$S/bot-b/bot-b.png" "$M"

KEEP='Use your image generation tool in edit mode with the attached image as the reference. Produce the SAME robot: identical design, proportions, materials, lighting, colors, camera, framing, canvas size (1024x1024) and fully transparent background (PNG with alpha, no shadow, no ground).'

run() {
  local name=$1 change=$2
  mkdir -p "$S/$name"
  codex exec -m gpt-5.6-sol --skip-git-repo-check -C "$S/$name" -s workspace-write --json \
    -o "$S/$name/last.txt" -i "$M" -- \
    "$KEEP Change ONLY this: $change Do not change anything else. Save the result into the current directory as $name.png and report its original saved path, pixel size, and whether it has an alpha channel." \
    < /dev/null > "$S/$name/events.jsonl" 2> "$S/$name/err.txt"
  echo "$name exit=$?"
}

run blank 'the face screen is switched off and empty - remove both green eyes and their glow completely, leaving only the plain dark glass screen with its subtle reflection. The antenna LED stays plain light grey.' &
run pose-wave 'the robot raises its RIGHT arm (the arm on the viewer'"'"'s left side) up beside its head in a friendly wave, the arm panel rotated at the shoulder hinge so the hand end points upward. The other arm stays down. The face screen is switched off and empty (no eyes, no glow), the antenna LED stays plain light grey.' &

wait
echo done
