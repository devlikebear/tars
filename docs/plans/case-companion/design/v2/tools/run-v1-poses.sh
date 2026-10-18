#!/bin/zsh
# V1: remaining pose frames, each an -i edit of the blank-screen master.
S=${0:A:h}
M="$S/blank/blank.png"

KEEP='Use your image generation tool in edit mode with the attached image as the reference. Produce the SAME robot: identical design, proportions, materials, lighting, colors, camera, framing, body position, canvas size (1024x1024) and fully transparent background (PNG with alpha, no shadow, no ground). The face screen stays switched off and empty (no eyes, no glow). The antenna LED stays plain light grey.'

run() {
  local name=$1 change=$2
  mkdir -p "$S/$name"
  codex exec -m gpt-5.6-sol --skip-git-repo-check -C "$S/$name" -s workspace-write --json \
    -o "$S/$name/last.txt" -i "$M" -- \
    "$KEEP Change ONLY this: $change The body, head, screen and feet must not move. Do not change anything else. Save the result into the current directory as $name.png and report its original saved path, pixel size, and whether it has an alpha channel." \
    < /dev/null > "$S/$name/events.jsonl" 2> "$S/$name/err.txt"
  echo "$name exit=$?"
}

run pose-alert 'ALERT pose: the antenna extends straight up to about twice its current length (same thin rod, same round LED tip, just taller), and both arm panels swing slightly outward from the body at the shoulder hinges (about 20 degrees each), like a startled stance.' &
run pose-working 'WORKING pose: both arm panels bend at the shoulder hinges so their lower ends come forward and inward in front of the lower half of the body, as if typing on an invisible keyboard at waist height; both arms stay visible and symmetric.' &
run pose-wave2 'the robot raises its RIGHT arm (the arm on the viewer'"'"'s left side) up beside its head in a friendly wave, tilted further OUTWARD than straight up - about 35 degrees away from the head, hand end pointing up and out. The other arm stays down.' &

wait
echo done
