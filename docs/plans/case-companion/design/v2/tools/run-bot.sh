#!/bin/zsh
# V0 (bot): three style variants of the CASE companion robot, in parallel.
S=${0:A:h}
cd "$S"

COMMON='Character: "CASE", a small companion robot for a developer tool (an original design). Body: a compact upright slab - a rounded-corner rectangular monolith about 1.4 times taller than wide, built from two vertical articulated halves with a thin visible seam down the middle, brushed dark-graphite metal (#2B2E2C to #3A3E3B) with slightly lighter beveled edges so the silhouette reads on a dark background. Face: a dark glass screen panel in the upper third of the front, showing two simple glowing eyes in signal green (#3EE07F) - soft rounded rectangles, friendly; no mouth. On top: one short antenna with a small round LED tip, the LED plain light grey (it will be recolored later). Two slim panel-like arms hinged at the sides, hanging down. Two short stubby feet. Personality: calm, dependable, quietly witty; cute but not childish. Must stay readable at 64x64 pixels: simple silhouette, few details, strong shapes, high-contrast screen. Front view, centered, square 1024x1024 canvas, the robot fills about 85% of the height. Fully transparent background (PNG with alpha): no background, no ground, no shadow, no text, no logo, no border. Original design: not a replica of any robot from a film, game or existing brand.'

run() {
  local name=$1 style=$2
  mkdir -p "$S/$name"
  codex exec -m gpt-5.6-sol --skip-git-repo-check -C "$S/$name" -s workspace-write --json \
    -o "$S/$name/last.txt" \
    "Use your image generation tool to create ONE image. Style: $style $COMMON Save the result into the current directory as $name.png and report its original saved path, its pixel size, and whether it has an alpha channel." \
    < /dev/null > "$S/$name/events.jsonl" 2> "$S/$name/err.txt"
  echo "$name exit=$?"
}

run bot-a 'FLAT VECTOR - flat solid color fills, no gradients, no texture, clean geometric shapes, no outlines (parts separated by tone only), like a modern app mascot icon.' &
run bot-b 'SOFT SHADING - smooth soft gradients, a subtle brushed-metal sheen and a gentle glow bloom around the screen eyes, slightly 3D premium mascot look, but still simple shapes.' &
run bot-c 'PIXEL ART - a crisp retro game sprite drawn on a 64x64 pixel grid and upscaled with hard square pixel edges, limited palette of about 16 colors, no anti-aliasing, no blur.' &

wait
echo done
