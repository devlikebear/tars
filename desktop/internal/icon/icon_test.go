package icon

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"

	"github.com/devlikebear/tars/desktop/internal/tray"
)

func decode(t *testing.T, raw []byte, size int) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
		t.Fatalf("bounds = %v, want %dx%d", b, size, size)
	}
	return img
}

func alphaAt(img image.Image, x, y int) uint32 {
	_, _, _, a := img.At(x, y).RGBA()
	return a
}

func TestApp(t *testing.T) {
	img := decode(t, App(64), 64)
	if alphaAt(img, 0, 0) != 0 {
		t.Fatal("the rounded corner must be transparent")
	}
	if alphaAt(img, 32, 32) == 0 || alphaAt(img, 10, 32) == 0 {
		t.Fatal("the square and its mark must be opaque")
	}
}

// iOS refuses an app icon with an alpha channel, so the mobile icon and the
// launch image must be opaque everywhere and encode without alpha.
func TestMobileAndLaunchAreOpaque(t *testing.T) {
	for name, tc := range map[string]struct {
		raw  []byte
		size int
	}{"mobile": {Mobile(64), 64}, "launch": {Launch(256), 256}} {
		// The IHDR colour type (byte 25 of the file): 2 is RGB, 6 is RGBA.
		if len(tc.raw) < 26 || tc.raw[25] != 2 {
			t.Errorf("%s: colour type %d, want 2 (RGB, no alpha)", name, tc.raw[25])
		}
		img := decode(t, tc.raw, tc.size)
		for _, p := range [][2]int{{0, 0}, {tc.size - 1, tc.size - 1}, {tc.size / 2, tc.size / 2}} {
			if alphaAt(img, p[0], p[1]) != 0xffff {
				t.Errorf("%s: pixel %v is not opaque", name, p)
			}
		}
	}
	// The mark is green in the middle, on graphite at the corner.
	img := decode(t, Mobile(64), 64)
	if r, g, _, _ := img.At(32, 32).RGBA(); g <= r {
		t.Error("the middle of the mobile icon is the green mark")
	}
	if r, g, b, _ := img.At(0, 0).RGBA(); r > 0x2000 || g > 0x2000 || b > 0x2000 {
		t.Error("the corner of the mobile icon is graphite")
	}
}

// The maskable icon must stay inside its safe zone: no opaque mark pixel
// farther than 40% of the icon size from the centre (the W3C maskable-icon
// safe zone), so a host cropping it into a circle never clips the mark.
func TestMaskableStaysInSafeZone(t *testing.T) {
	size := 192
	img := decode(t, Maskable(size), size)
	cx, cy := float64(size)/2, float64(size)/2
	safe := float64(size) * 0.4
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, _, a := img.At(x, y).RGBA()
			if a == 0 || g < r+5000 {
				continue // background (graphite has r and g close together), not the mark
			}
			if d := math.Hypot(float64(x)-cx, float64(y)-cy); d > safe {
				t.Fatalf("mark pixel at (%d,%d) is %.1fpx from centre, past the %.1fpx safe zone", x, y, d, safe)
			}
		}
	}
	if alphaAt(img, 0, 0) != 0xffff {
		t.Fatal("maskable must be full bleed, opaque at the corner")
	}
}

func TestTrayDotFollowsState(t *testing.T) {
	// 64 matches the real call site (shell.go).
	const size = 64
	bx, by := trayBadgeCentre(size)
	centre := func(s tray.State) (uint32, uint32, uint32, uint32) {
		return decode(t, Tray(s, size), size).At(int(bx), int(by)).RGBA()
	}
	if _, _, _, a := centre(tray.Idle); a != 0 {
		t.Fatal("idle has no dot")
	}
	// The badge never reaches the mark's centre, so the stem stays whole.
	if _, _, _, a := decode(t, Tray(tray.NeedsInput, size), size).At(size/2, size/2).RGBA(); a == 0 {
		t.Fatal("the mark's centre is cut out")
	}
	if r, g, _, _ := centre(tray.NeedsInput); r <= g {
		t.Fatal("needs input is amber")
	}
	if r, g, _, _ := centre(tray.Running); g <= r {
		t.Fatal("running is green")
	}
	for _, s := range []tray.State{tray.Offline, tray.Locked, tray.Setup} {
		if _, ok := DotColour(s); !ok {
			t.Errorf("%v shows a dot", s)
		}
	}
}

// At every size the mark is drawn at, it reads as the T: the pale bar on
// top, the green stem under it, and both centred in the tile.
func TestMarkIsBarOverGreenStem(t *testing.T) {
	isGreen := func(img image.Image, x, y int) bool {
		r, g, _, a := img.At(x, y).RGBA()
		return a != 0 && g > r+0x4000
	}
	isPale := func(img image.Image, x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r > 0xc000 && g > 0xc000 && b > 0xc000
	}
	for _, size := range []int{16, 32, 64, 512} {
		img := decode(t, App(size), size)
		mid, barY, stemY := size/2, size*31/100, size*6/10
		if !isPale(img, mid, barY) {
			t.Errorf("size %d: no pale bar at row %d", size, barY)
		}
		if !isGreen(img, mid, stemY) {
			t.Errorf("size %d: no green stem at row %d", size, stemY)
		}
		// The bar is wider than the stem: beside the stem is the tile.
		if x := size * 3 / 10; !isPale(img, x, barY) || isGreen(img, x, stemY) || isPale(img, x, stemY) {
			t.Errorf("size %d: column %d must hold the bar and no stem", size, x)
		}
		for _, y := range []int{barY, stemY} {
			for x := 0; x < size/2; x++ {
				l, r := img.At(x, y), img.At(size-1-x, y)
				if l != r {
					t.Fatalf("size %d: row %d is not symmetric at column %d", size, y, x)
				}
			}
		}
	}
}

func TestTemplateIsMonochrome(t *testing.T) {
	img := decode(t, Template(22), 22)
	for y := 0; y < 22; y++ {
		for x := 0; x < 22; x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r|g|b != 0 {
				t.Fatalf("pixel %d,%d is not black: %d %d %d", x, y, r, g, b)
			}
		}
	}
	if alphaAt(img, 11, 11) == 0 {
		t.Fatal("the stem must be drawn")
	}
}
