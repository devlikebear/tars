"""Build CASE's sprite sheet from the codex-generated source frames.

    uv run --no-project --with pillow --with numpy python -I scripts/case_sprites.py [--preview DIR]

Reads the source frames under docs/plans/case-companion/design/v2/source/,
writes:

    frontend/console/src/assets/case/case-bot.webp   one row of 128px cells
    frontend/console/src/lib/caseSprite.generated.ts frame order, screen rect, LED per frame

Every pose frame is an `-i` edit of the blank-screen master. codex redraws
the whole picture a pixel or two off, so a pose is never used as is: it is
aligned to the master on the feet (which never move), and only the region
that should change (arms, antenna) is composited onto the master. The body
and screen are then pixel-identical in every frame, so switching poses does
not shimmer. See docs/plans/case-companion/phase-1.5-visual-motion.md §5.

The sheet is a checked-in build output; CI does not run this script.
"""
import argparse
import json
from collections import deque
from pathlib import Path

import numpy as np
from PIL import Image, ImageFilter

ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "docs/plans/case-companion/design/v2/source"
SHEET = ROOT / "frontend/console/src/assets/case/case-bot.webp"
MODULE = ROOT / "frontend/console/src/lib/caseSprite.generated.ts"

CELL = 128
# Fraction of the cell height the tallest frame fills; the rest is margin.
FILL = 0.94

# (frame name, source file, regions allowed to change). Order is the sheet's
# column order. Regions are relative to the master's screen rectangle:
# left/right of it (arms), above it (antenna), below it (lower body).
FRAMES = [
    ("base", "master-blank.png", []),
    ("alert", "pose-alert-raw.png", ["left", "right", "top"]),
    ("working", "pose-working-raw.png", ["left", "right", "below"]),
    ("wave", "pose-wave-raw.png", ["left"]),
    ("wave2", "pose-wave2-raw.png", ["left"]),
]


def load(path):
    return np.asarray(Image.open(path).convert("RGBA")).astype(np.float32)


def alpha_bbox(a, thr=32):
    ys, xs = np.nonzero(a > thr)
    return int(xs.min()), int(ys.min()), int(xs.max()) + 1, int(ys.max()) + 1


def largest_component(mask):
    h, w = mask.shape
    seen = np.zeros_like(mask, dtype=bool)
    best = []
    for y0, x0 in zip(*np.nonzero(mask)):
        if seen[y0, x0]:
            continue
        comp = []
        q = deque([(y0, x0)])
        seen[y0, x0] = True
        while q:
            y, x = q.popleft()
            comp.append((y, x))
            for dy, dx in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                ny, nx = y + dy, x + dx
                if 0 <= ny < h and 0 <= nx < w and mask[ny, nx] and not seen[ny, nx]:
                    seen[ny, nx] = True
                    q.append((ny, nx))
        if len(comp) > len(best):
            best = comp
    ys = [p[0] for p in best]
    xs = [p[1] for p in best]
    return min(xs), min(ys), max(xs) + 1, max(ys) + 1


def detect_screen(img):
    """The dark glass screen: the largest dark, opaque region in the upper body."""
    size = img.shape[1]
    small = np.asarray(Image.fromarray(img.astype(np.uint8)).resize((256, 256), Image.BILINEAR)).astype(np.float32)
    lum = 0.2126 * small[..., 0] + 0.7152 * small[..., 1] + 0.0722 * small[..., 2]
    x0, y0, x1, y1 = alpha_bbox(small[..., 3])
    mask = (small[..., 3] > 200) & (lum < 28)
    mask[: y0 + (y1 - y0) // 10, :] = False  # skip the antenna
    mask[y0 + (y1 - y0) * 6 // 10 :, :] = False  # the screen sits in the upper body
    bx0, by0, bx1, by1 = largest_component(mask)
    s = size / 256
    return [bx0 * s, by0 * s, bx1 * s, by1 * s]


def detect_led(img):
    """The antenna LED: the opaque blob at the very top of the figure."""
    a = img[..., 3]
    _, y0, _, y1 = alpha_bbox(a)
    band_h = max(4, (y1 - y0) // 14)
    ys, xs = np.nonzero(a[y0 : y0 + band_h, :] > 128)
    return [float(xs.mean()), float(ys.mean() + y0), float((xs.max() - xs.min() + 1) / 2)]


def feet_anchor(img):
    """Centre x and bottom y of the feet (the bottom 10% of the figure)."""
    a = img[..., 3]
    _, y0, _, y1 = alpha_bbox(a)
    top = y1 - max(4, (y1 - y0) // 10)
    ys, xs = np.nonzero(a[top:y1, :] > 128)
    return float((xs.min() + xs.max()) / 2), float(ys.max() + top)


def shift(img, dx, dy):
    out = np.zeros_like(img)
    h, w = img.shape[:2]
    out[max(0, dy) : min(h, h + dy), max(0, dx) : min(w, w + dx)] = img[max(0, -dy) : min(h, h - dy), max(0, -dx) : min(w, w - dx)]
    return out


def region_mask(shape, screen, regions):
    """Where a pose may differ from the master, in terms of the master's screen.

    The body's bezel is about a fifth of the screen width on each side and
    the head's top bezel about a fifth of its height, so these stop just
    short of the body: left/right are the arms (outside the body's sides),
    top is the antenna (above the head), below is the lower body.
    """
    h, w = shape[:2]
    x0, y0, x1, y1 = screen
    sw, sh = x1 - x0, y1 - y0
    m = np.zeros((h, w), dtype=bool)
    for r in regions:
        if r == "left":
            m[:, : int(x0 - 0.15 * sw)] = True
        elif r == "right":
            m[:, int(x1 + 0.15 * sw) :] = True
        elif r == "top":
            m[: int(y0 - 0.2 * sh), :] = True
        elif r == "below":
            m[int(y1 + 0.1 * sh) :, :] = True
        else:
            raise ValueError(f"unknown region {r!r}")
    return m


def composite(master, pose, screen, regions):
    mx, my = feet_anchor(master)
    px, py = feet_anchor(pose)
    aligned = shift(pose, round(mx - px), round(my - py))
    diff = np.abs(aligned - master)
    changed = (diff[..., :3].max(axis=2) > 48) | (diff[..., 3] > 48)
    changed &= region_mask(master.shape, screen, regions)
    # Opening drops the thin outline residue (every edge redrawn a pixel or
    # two off); the solid arm/antenna blob survives and is then grown and
    # feathered so the seam does not show.
    k = max(3, master.shape[1] // 110) | 1
    m = Image.fromarray((changed * 255).astype(np.uint8))
    m = m.filter(ImageFilter.MinFilter(k)).filter(ImageFilter.MaxFilter(k * 3)).filter(ImageFilter.GaussianBlur(k / 2))
    mask = np.asarray(m).astype(np.float32)[..., None] / 255.0
    return master * (1 - mask) + aligned * mask


def crop_box(frames):
    """One square crop shared by every frame: the union of their figures, bottom-aligned."""
    boxes = [alpha_bbox(f[..., 3]) for f in frames]
    x0 = min(b[0] for b in boxes)
    y0 = min(b[1] for b in boxes)
    x1 = max(b[2] for b in boxes)
    y1 = max(b[3] for b in boxes)
    side = max((y1 - y0) / FILL, (x1 - x0) / FILL)
    cx = (x0 + x1) / 2
    bottom = y1 + (side - (y1 - y0)) * 0.25  # a little floor under the feet, more air above
    return cx - side / 2, bottom - side, side


def to_cell(img, box):
    left, top, side = box
    pil = Image.fromarray(np.clip(img, 0, 255).astype(np.uint8))
    return pil.resize((CELL, CELL), Image.LANCZOS, box=(left, top, left + side, top + side))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--preview", type=Path, help="also write a preview sheet on the console background here")
    args = ap.parse_args()

    master = load(SOURCE / FRAMES[0][1])
    screen = detect_screen(master)
    frames = [master]
    for _, file, regions in FRAMES[1:]:
        pose = load(SOURCE / file)
        if pose.shape != master.shape:
            pose = np.asarray(Image.fromarray(pose.astype(np.uint8)).resize(master.shape[1::-1], Image.LANCZOS)).astype(np.float32)
        frames.append(composite(master, pose, screen, regions))

    # The square crop around the figure can reach past the canvas (the robot
    # fills most of its height): pad every frame with transparency first.
    pad = master.shape[0] // 4
    frames = [np.pad(f, ((pad, pad), (pad, pad), (0, 0))) for f in frames]
    screen = [v + pad for v in screen]

    box = crop_box(frames)
    left, top, side = box
    s = CELL / side

    def pt(x, y):
        return round((x - left) * s, 2), round((y - top) * s, 2)

    sheet = Image.new("RGBA", (CELL * len(frames), CELL), (0, 0, 0, 0))
    leds = {}
    for i, ((name, _, _), img) in enumerate(zip(FRAMES, frames)):
        sheet.paste(to_cell(img, box), (i * CELL, 0))
        cx, cy, r = detect_led(img)
        x, y = pt(cx, cy)
        leds[name] = [x, y, round(r * s, 2)]

    sx0, sy0 = pt(screen[0], screen[1])
    sx1, sy1 = pt(screen[2], screen[3])
    data = {
        "cell": CELL,
        "frames": [f[0] for f in FRAMES],
        "screen": [sx0, sy0, round(sx1 - sx0, 2), round(sy1 - sy0, 2)],
        "led": leds,
    }

    SHEET.parent.mkdir(parents=True, exist_ok=True)
    sheet.save(SHEET, "WEBP", quality=90, method=6)
    MODULE.write_text(
        "// Generated by scripts/case_sprites.py — do not edit by hand.\n"
        "// Coordinates are in pixels of one 128px cell of case-bot.webp:\n"
        "// screen = [x, y, width, height], led = [centre x, centre y, radius].\n"
        f"export const caseSprite = {json.dumps(data, indent=2)} as const\n"
    )

    if args.preview:
        args.preview.mkdir(parents=True, exist_ok=True)
        bg = Image.new("RGBA", sheet.size, (20, 20, 20, 255))
        bg.alpha_composite(sheet)
        bg.save(args.preview / "case-bot-preview.png")
        small = sheet.resize((64 * len(frames), 64), Image.LANCZOS)
        bg64 = Image.new("RGBA", small.size, (20, 20, 20, 255))
        bg64.alpha_composite(small)
        bg64.resize((bg64.width * 4, bg64.height * 4), Image.NEAREST).save(args.preview / "case-bot-64px-4x.png")

    print(f"{SHEET.relative_to(ROOT)}: {SHEET.stat().st_size} bytes, {len(frames)} frames")
    print(json.dumps(data))


if __name__ == "__main__":
    main()
