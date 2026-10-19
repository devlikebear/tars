# CASE v2 — codex 프롬프트

생성 명령 (2026-10-18, codex-cli 0.144.3, ChatGPT 계정):

```bash
codex exec -m gpt-5.6-sol --skip-git-repo-check -C <out-dir> -s workspace-write --json -o last.txt "<prompt>"
# 참조 편집: -i는 파일을 여러 개 받으므로 반드시 -- 로 끝낸다
codex exec -m gpt-5.6-sol --skip-git-repo-check -C <out-dir> -s workspace-write --json -o last.txt -i master.png -- "<prompt>"
```

각 프롬프트는 "Use your image generation tool to create ONE image. Style: <스타일 문장> <COMMON>" 이고, 끝에 "Save the result into the current directory as <file> …" 를 붙인다.

## 공통 캐릭터 지정 (`COMMON`)

> Character: "CASE", a small companion robot for a developer tool (an original design). Body: a compact upright slab - a rounded-corner rectangular monolith about 1.4 times taller than wide, built from two vertical articulated halves with a thin visible seam down the middle, brushed dark-graphite metal (#2B2E2C to #3A3E3B) with slightly lighter beveled edges so the silhouette reads on a dark background. Face: a dark glass screen panel in the upper third of the front, showing two simple glowing eyes in signal green (#3EE07F) - soft rounded rectangles, friendly; no mouth. On top: one short antenna with a small round LED tip, the LED plain light grey (it will be recolored later). Two slim panel-like arms hinged at the sides, hanging down. Two short stubby feet. Personality: calm, dependable, quietly witty; cute but not childish. Must stay readable at 64x64 pixels: simple silhouette, few details, strong shapes, high-contrast screen. Front view, centered, square 1024x1024 canvas, the robot fills about 85% of the height. Fully transparent background (PNG with alpha): no background, no ground, no shadow, no text, no logo, no border. Original design: not a replica of any robot from a film, game or existing brand.

## 스타일 3안

| 파일 | 스타일 문장 |
|---|---|
| `source/bot-a.png` | FLAT VECTOR - flat solid color fills, no gradients, no texture, clean geometric shapes, no outlines (parts separated by tone only), like a modern app mascot icon. |
| `source/bot-b.png` | SOFT SHADING - smooth soft gradients, a subtle brushed-metal sheen and a gentle glow bloom around the screen eyes, slightly 3D premium mascot look, but still simple shapes. |
| `source/bot-c.png` | PIXEL ART - a crisp retro game sprite drawn on a 64x64 pixel grid and upscaled with hard square pixel edges, limited palette of about 16 colors, no anti-aliasing, no blur. |

## 참조 편집 스파이크

`source/spike-realistic.png`(스타일 지정 없는 첫 스파이크, 미어캣 시절)를 `-i`로 붙이고:

> Use your image generation tool in edit mode with the attached image as the reference. Produce the SAME character: identical pose, framing, proportions, colors, canvas size and transparent background. Change ONLY the eyes: both eyes closed (a blink, eyelids shut as curved lines). Do not change anything else.

결과: `source/spike-realistic-blink.png`. 캐릭터는 폐기됐지만 `-i` 편집이 어떻게 동작하는지(무엇을 보존하고 무엇을 보존하지 않는지)의 기록으로 남긴다.
