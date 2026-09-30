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
