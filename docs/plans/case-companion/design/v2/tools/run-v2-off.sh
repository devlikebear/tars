#!/bin/zsh
# V2: the powered-off pose (sleep / hide, and the first frame of boot-up),
# an -i edit of the blank-screen master.
S=${0:A:h}
M="$S/blank/blank.png"
name=pose-off
mkdir -p "$S/$name"
codex exec -m gpt-5.6-sol --skip-git-repo-check -C "$S/$name" -s workspace-write --json \
  -o "$S/$name/last.txt" -i "$M" -- \
  "Use your image generation tool in edit mode with the attached image as the reference. Produce the SAME robot: identical design, proportions, materials, lighting, colors, camera, framing, body position, canvas size (1024x1024) and fully transparent background (PNG with alpha, no shadow, no ground). The face screen stays switched off and empty (no eyes, no glow). Change ONLY this: POWERED-OFF pose - the antenna folds down flat to the right, lying along the top of the head (same thin rod, the round LED tip now dark grey, unlit), and both arm panels fold in tight against the sides of the body, hanging straight down and touching it. The body, head, screen and feet must not move. Do not change anything else. Save the result into the current directory as $name.png and report its original saved path, pixel size, and whether it has an alpha channel." \
  < /dev/null > "$S/$name/events.jsonl" 2> "$S/$name/err.txt"
echo "$name exit=$?"
