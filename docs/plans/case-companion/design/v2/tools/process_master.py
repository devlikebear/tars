"""V0 master processing for the CASE bot sprite.

Usage: process_master.py <blank.png> <pose.png> <out-dir>

- detects the face screen rectangle and the antenna LED on the blank frame
- aligns the pose frame to the blank frame on the unchanged side
- composites only the changed arm region of the pose onto the blank frame
- writes 128px frames, an onion-skin check image and manifest.json
"""
import json
import sys
from collections import deque
from pathlib import Path

import numpy as np
from PIL import Image, ImageFilter

CELL = 128


def load(path):
    return np.asarray(Image.open(path).convert("RGBA")).astype(np.float32)


def alpha_bbox(a, thr=32):
    ys, xs = np.nonzero(a > thr)
    return int(xs.min()), int(ys.min()), int(xs.max()), int(ys.max())


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
    return min(xs), min(ys), max(xs), max(ys)


def detect_screen(img):
    """The dark glass screen: the largest dark, opaque region in the upper body."""
    small = np.asarray(Image.fromarray(img.astype(np.uint8)).resize((256, 256), Image.BILINEAR)).astype(np.float32)
    lum = 0.2126 * small[..., 0] + 0.7152 * small[..., 1] + 0.0722 * small[..., 2]
    x0, y0, x1, y1 = alpha_bbox(small[..., 3])
    mask = (small[..., 3] > 200) & (lum < 28)
    mask[: y0 + (y1 - y0) // 10, :] = False  # skip the antenna
    mask[y0 + (y1 - y0) * 6 // 10 :, :] = False  # screen sits in the upper body
    bx0, by0, bx1, by1 = largest_component(mask)
    s = img.shape[1] / 256
    return [round(bx0 * s), round(by0 * s), round((bx1 + 1) * s), round((by1 + 1) * s)]


def detect_led(img):
    """Antenna LED: the opaque blob at the very top of the figure."""
    a = img[..., 3]
    x0, y0, x1, y1 = alpha_bbox(a)
    band = a[y0 : y0 + (y1 - y0) // 14, :] > 128
    ys, xs = np.nonzero(band)
    cx, cy = xs.mean(), ys.mean() + y0
    r = (xs.max() - xs.min()) / 2
    return [round(cx), round(cy), round(r)]


def side_offset(blank, pose, x_from):
    """Translation that maps pose onto blank, measured on the unchanged side (x >= x_from)."""
    b = blank[:, x_from:, 3]
    p = pose[:, x_from:, 3]
    bb, pb = alpha_bbox(b), alpha_bbox(p)
    return bb[2] - pb[2], bb[3] - pb[3]  # right edge and bottom (feet) stay put


def shift(img, dx, dy):
    out = np.zeros_like(img)
    h, w = img.shape[:2]
    ys = slice(max(0, dy), min(h, h + dy))
    yd = slice(max(0, -dy), min(h, h - dy))
    xs = slice(max(0, dx), min(w, w + dx))
    xd = slice(max(0, -dx), min(w, w - dx))
    out[ys, xs] = img[yd, xd]
    return out


def down(img, size=CELL):
    return Image.fromarray(np.clip(img, 0, 255).astype(np.uint8)).resize((size, size), Image.LANCZOS)


def main():
    blank_p, pose_p, out = Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3])
    out.mkdir(parents=True, exist_ok=True)
    blank, pose = load(blank_p), load(pose_p)
    if pose.shape != blank.shape:
        pose = np.asarray(Image.fromarray(pose.astype(np.uint8)).resize(blank.shape[1::-1], Image.LANCZOS)).astype(np.float32)

    screen = detect_screen(blank)
    led = detect_led(blank)
    bx0, by0, bx1, by1 = alpha_bbox(blank[..., 3])
    cx = (screen[0] + screen[2]) // 2

    dx, dy = side_offset(blank, pose, cx)
    pose_al = shift(pose, dx, dy)

    # Changed region: big colour/alpha difference, limited to the waving side
    # (left of the screen's right third), grown and feathered.
    diff = np.abs(pose_al - blank)
    changed = (diff[..., :3].max(axis=2) > 48) | (diff[..., 3] > 48)
    limit = screen[0] + (screen[2] - screen[0]) // 4
    changed[:, limit:] = False
    m = Image.fromarray((changed * 255).astype(np.uint8))
    # Opening first: thin outline residue (the edit redraws every edge a pixel
    # or two off) disappears, the solid arm blob survives.
    m = m.filter(ImageFilter.MinFilter(9)).filter(ImageFilter.MaxFilter(25)).filter(ImageFilter.GaussianBlur(4))
    mask = np.asarray(m).astype(np.float32)[..., None] / 255.0
    pose_comp = blank * (1 - mask) + pose_al * mask

    s = CELL / blank.shape[1]
    down(blank).save(out / "base.png")
    down(pose_comp).save(out / "wave.png")
    Image.fromarray((mask[..., 0] * 255).astype(np.uint8)).resize((256, 256)).save(out / "wave-mask.png")

    # Onion skin: blank in red channel, composite in green/blue — grey where equal.
    onion = np.zeros_like(blank)
    onion[..., 0] = blank[..., 3]
    onion[..., 1] = pose_comp[..., 3]
    onion[..., 2] = pose_comp[..., 3]
    onion[..., 3] = 255
    down(onion, 256).save(out / "wave-onion.png")

    manifest = {
        "cell": CELL,
        "source_size": blank.shape[1],
        "pose_offset_px": [int(dx), int(dy)],
        "frames": {
            "base": {"screen": [round(v * s, 1) for v in screen], "led": [round(v * s, 1) for v in led]},
            "wave": {"screen": [round(v * s, 1) for v in screen], "led": [round(v * s, 1) for v in led]},
        },
    }
    (out / "manifest.json").write_text(json.dumps(manifest, indent=2))
    print(json.dumps(manifest))


if __name__ == "__main__":
    main()
