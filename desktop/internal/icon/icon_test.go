package icon

import (
	"bytes"
	"image"
	"image/png"
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

func TestTrayDotFollowsState(t *testing.T) {
	centre := func(s tray.State) (uint32, uint32, uint32, uint32) {
		return decode(t, Tray(s, 32), 32).At(16, 16).RGBA()
	}
	if _, _, _, a := centre(tray.Idle); a != 0 {
		t.Fatal("idle has no dot")
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
		t.Fatal("the centre dot must be drawn")
	}
}
