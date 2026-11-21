// Package icon draws the shell's icons in code, so the tray can change
// colour with the server's state without shipping an image per state.
//
// The mark is a T on dark graphite: a pale bar across the top and, under a
// gap, a stem in signal green (#3ee07f). The T is the name, and the stem is
// a terminal's block cursor, the place where the agent is working. It
// replaced four upright slabs, which read as an equaliser or a bar chart,
// and before that a sparkle, the generic "AI" glyph. Below 48px the two
// blocks snap to whole pixels and the stem thickens, so a favicon keeps a
// crisp T instead of a grey smear. The tray shows its state as a badge in
// the corner, cut out of the mark.
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
	// page is the console's --surface-base, the mobile app's background.
	page   = color.NRGBA{0x0d, 0x0f, 0x11, 0xff}
	signal = color.NRGBA{0x3e, 0xe0, 0x7f, 0xff}
	amber  = color.NRGBA{0xf5, 0xb7, 0x40, 0xff}
	grey   = color.NRGBA{0x8a, 0x8f, 0x98, 0xff}
	red    = color.NRGBA{0xef, 0x5b, 0x5b, 0xff}
	black  = color.NRGBA{0, 0, 0, 0xff}
	// pale is the colour of the T's bar.
	pale = color.NRGBA{0xdf, 0xe3, 0xe8, 0xff}
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

// App is the application icon: the mark on a rounded graphite square.
func App(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	radius := s * 0.22
	paint(img, func(x, y float64) (color.NRGBA, float64) {
		return graphite, roundedRectCoverage(x, y, s*0.04, s*0.04, s*0.96, s*0.96, radius)
	})
	mark(img, s/2, s/2, s, signal, pale)
	return encode(img)
}

// Mobile is the iOS app's icon (mobile/): the same mark as App, full bleed
// and opaque, because iOS rounds the corners itself and refuses an app
// icon with transparency. The mark is scaled up slightly from App's, since
// there is no inset square padding to fill the tile with.
func Mobile(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	paint(img, func(x, y float64) (color.NRGBA, float64) { return graphite, 1 })
	mark(img, s/2, s/2, s*1.12, signal, pale)
	return encode(img)
}

// Maskable is the PWA maskable icon (frontend/console/public/pwa-maskable-*):
// full bleed and opaque like Mobile, but the mark is shrunk so its farthest
// point stays inside the maskable-icon safe zone — a circle centred on the
// icon with radius 40% of the icon's size (https://www.w3.org/TR/appmanifest/)
// — a host is free to crop anywhere outside it into a circle, squircle or
// rounded square, so nothing of the mark may cross that line.
func Maskable(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	paint(img, func(x, y float64) (color.NRGBA, float64) { return graphite, 1 })
	mark(img, s/2, s/2, s*0.95, signal, pale)
	return encode(img)
}

// Launch is the iOS launch image: the mark small in the middle of the
// console's page colour (--surface-base), so a cold start goes from this to
// the app's page — the shell draws the same mark until the page has loaded
// (LaunchMarkView in mobile/ios/App/App/SceneDelegate.swift) — with no flash
// and no change of shade. iOS fills the screen with it (aspect fill), so the
// mark sits well inside any phone's crop.
func Launch(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	paint(img, func(x, y float64) (color.NRGBA, float64) { return page, 1 })
	mark(img, s/2, s/2, s*launchMark, signal, pale)
	return encode(img)
}

// launchMark is the mark's box in the launch image, relative to the image.
// LaunchMarkView (SceneDelegate.swift) draws the same proportions.
const launchMark = 0.13

// Tray is the tray icon for a state on Windows and Linux: the mark with
// the state's dot as a badge in the bottom-right corner, cut out of the mark
// so the two never merge. Idle has no badge; offline greys the whole mark.
func Tray(s tray.State, size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	f := float64(size)
	stem, bar := signal, pale
	if s == tray.Offline {
		stem, bar = grey, grey
	}
	mark(img, f/2, f/2, f*trayMark, stem, bar)
	if dot, ok := DotColour(s); ok {
		bx, by := trayBadgeCentre(size)
		cut(img, bx, by, f*0.22)
		disc(img, bx, by, f*0.15, dot)
	}
	return encode(img)
}

// trayMark is the mark's box in a tray icon, relative to the icon: larger
// than the icon, since the tray has no tile and the mark fills only about
// half of its box.
const trayMark = 1.6

// trayBadgeCentre is where Tray draws the state badge in an icon of size.
func trayBadgeCentre(size int) (float64, float64) {
	f := float64(size)
	return f * 0.8, f * 0.8
}

// Template is the macOS menu bar icon: black with alpha, which macOS tints
// for light and dark menu bars. The state shows as the text label beside it.
func Template(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	f := float64(size)
	mark(img, f/2, f/2, f*trayMark, black, black)
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

// markRects lays out the mark in a box of side s centred at (cx, cy): the
// bar and, under a gap, the stem, each as {x0, y0, x1, y1}. Below 48px
// every edge lands on a whole pixel, both widths are even so the stem sits
// exactly under the middle of the bar, and the stem is a little thicker than
// its share, since a two-pixel stem under a two-pixel bar is a plain T and
// no longer a cursor.
func markRects(cx, cy, s float64) (bar, stem [4]float64) {
	barW, barH, gap, stemW, stemH := s*0.53, s*0.133, s*0.047, s*0.172, s*0.336
	snap := s <= 48
	if snap {
		even := func(v float64) float64 { return math.Max(2, 2*math.Round(v/2)) }
		barW, stemW = even(barW), even(s*0.2)
		barH = math.Max(2, math.Round(barH))
		gap = math.Max(1, math.Round(gap))
		stemH = math.Round(stemH)
	}
	x, y := cx-barW/2, cy-(barH+gap+stemH)/2
	if snap {
		x, y = math.Round(x), math.Round(y)
	}
	bar = [4]float64{x, y, x + barW, y + barH}
	sx, sy := x+(barW-stemW)/2, y+barH+gap
	stem = [4]float64{sx, sy, sx + stemW, sy + stemH}
	return bar, stem
}

// mark draws the mark — see the package doc — in a box of side s centred
// at (cx, cy): the stem in stemColour, the bar in barColour.
func mark(img *image.NRGBA, cx, cy, s float64, stemColour, barColour color.NRGBA) {
	radius := s * 0.031
	if s <= 48 {
		radius = 0 // a rounded corner on a two-pixel bar only blurs it
	}
	bar, stem := markRects(cx, cy, s)
	for _, b := range []struct {
		r [4]float64
		c color.NRGBA
	}{{bar, barColour}, {stem, stemColour}} {
		paint(img, func(x, y float64) (color.NRGBA, float64) {
			return b.c, roundedRectCoverage(x, y, b.r[0], b.r[1], b.r[2], b.r[3], radius)
		})
	}
}

// cut clears a disc of img back to transparent, so a badge drawn on top
// stands apart from the mark beneath it.
func cut(img *image.NRGBA, cx, cy, r float64) {
	b := img.Bounds()
	const n = 4
	for py := b.Min.Y; py < b.Max.Y; py++ {
		for px := b.Min.X; px < b.Max.X; px++ {
			var cov float64
			for sy := 0; sy < n; sy++ {
				for sx := 0; sx < n; sx++ {
					if math.Hypot(float64(px)+(float64(sx)+0.5)/n-cx, float64(py)+(float64(sy)+0.5)/n-cy) <= r {
						cov++
					}
				}
			}
			if cov == 0 {
				continue
			}
			p := img.NRGBAAt(px, py)
			p.A = uint8(math.Round(float64(p.A) * (1 - cov/(n*n))))
			img.SetNRGBA(px, py, p)
		}
	}
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
