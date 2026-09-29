// Package icon draws the shell's icons in code, so the tray can change
// colour with the server's state without shipping an image per state.
//
// The mark is the console's: a ring in signal green (#3ee07f) on dark
// graphite. The tray shows the ring alone, with a dot in the middle whose
// colour carries the state.
package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/devlikebear/tars/desktop/internal/tray"
)

var (
	graphite = color.NRGBA{0x16, 0x18, 0x1b, 0xff}
	signal   = color.NRGBA{0x3e, 0xe0, 0x7f, 0xff}
	amber    = color.NRGBA{0xf5, 0xb7, 0x40, 0xff}
	grey     = color.NRGBA{0x8a, 0x8f, 0x98, 0xff}
	red      = color.NRGBA{0xef, 0x5b, 0x5b, 0xff}
	black    = color.NRGBA{0, 0, 0, 0xff}
)

// DotColour is the colour of the tray dot for a state; ok is false when the
// state shows no dot.
func DotColour(s tray.State) (color.NRGBA, bool) {
	switch s {
	case tray.NeedsInput:
		return amber, true
	case tray.Running:
		return signal, true
	case tray.Offline:
		return grey, true
	case tray.Locked, tray.Setup:
		return red, true
	}
	return color.NRGBA{}, false
}

// App is the application icon: the ring on a rounded graphite square.
func App(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	radius := s * 0.22
	paint(img, func(x, y float64) (color.NRGBA, float64) {
		return graphite, roundedRectCoverage(x, y, s*0.04, s*0.04, s*0.96, s*0.96, radius)
	})
	ring(img, s/2, s/2, s*0.27, s*0.075, signal)
	disc(img, s/2, s/2, s*0.09, signal)
	return encode(img)
}

// Tray is the tray icon for a state on Windows and Linux: the ring in
// signal green with a state dot. Idle has no dot.
func Tray(s tray.State, size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	f := float64(size)
	ringColour := signal
	if s == tray.Offline {
		ringColour = grey
	}
	ring(img, f/2, f/2, f*0.36, f*0.12, ringColour)
	if dot, ok := DotColour(s); ok {
		disc(img, f/2, f/2, f*0.17, dot)
	}
	return encode(img)
}

// Template is the macOS menu bar icon: black with alpha, which macOS tints
// for light and dark menu bars. The state shows as the text label beside it.
func Template(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	f := float64(size)
	ring(img, f/2, f/2, f*0.36, f*0.12, black)
	disc(img, f/2, f/2, f*0.13, black)
	return encode(img)
}

func encode(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img) // encoding an in-memory NRGBA cannot fail
	return buf.Bytes()
}

// paint blends shape into img, sampling each pixel at 4x4 points for
// anti-aliased edges. shape returns a colour and its coverage (0..1) at a
// point.
func paint(img *image.NRGBA, shape func(x, y float64) (color.NRGBA, float64)) {
	b := img.Bounds()
	const n = 4
	for py := b.Min.Y; py < b.Max.Y; py++ {
		for px := b.Min.X; px < b.Max.X; px++ {
			var cov float64
			var c color.NRGBA
			for sy := 0; sy < n; sy++ {
				for sx := 0; sx < n; sx++ {
					col, a := shape(float64(px)+(float64(sx)+0.5)/n, float64(py)+(float64(sy)+0.5)/n)
					if a > 0 {
						c = col
						cov += a
					}
				}
			}
			cov /= n * n
			if cov > 0 {
				blend(img, px, py, c, cov)
			}
		}
	}
}

func blend(img *image.NRGBA, x, y int, c color.NRGBA, cov float64) {
	dst := img.NRGBAAt(x, y)
	srcA := float64(c.A) / 255 * cov
	dstA := float64(dst.A) / 255
	outA := srcA + dstA*(1-srcA)
	if outA == 0 {
		return
	}
	mix := func(s, d uint8) uint8 {
		return uint8(math.Round((float64(s)*srcA + float64(d)*dstA*(1-srcA)) / outA))
	}
	img.SetNRGBA(x, y, color.NRGBA{mix(c.R, dst.R), mix(c.G, dst.G), mix(c.B, dst.B), uint8(math.Round(outA * 255))})
}

func ring(img *image.NRGBA, cx, cy, r, width float64, c color.NRGBA) {
	paint(img, func(x, y float64) (color.NRGBA, float64) {
		d := math.Hypot(x-cx, y-cy)
		if d <= r+width/2 && d >= r-width/2 {
			return c, 1
		}
		return c, 0
	})
}

func disc(img *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	paint(img, func(x, y float64) (color.NRGBA, float64) {
		if math.Hypot(x-cx, y-cy) <= r {
			return c, 1
		}
		return c, 0
	})
}

func roundedRectCoverage(x, y, x0, y0, x1, y1, r float64) float64 {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return 0
	}
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	if math.Hypot(x-cx, y-cy) <= r {
		return 1
	}
	return 0
}
