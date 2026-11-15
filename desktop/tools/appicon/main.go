// Command appicon writes the application icon as a PNG, for packaging:
// macOS turns it into an .icns, Windows embeds it in the executable.
// -kind mobile writes the iOS app's icon, -kind launch its launch image
// (both full bleed and opaque, committed under mobile/ios). -kind maskable
// writes the PWA maskable icon (the mark kept inside the safe zone).
//
// Desktop app icon and Windows resource source:
//
//	go run ./tools/appicon -size 1024 -out build/appicon.png
//
// iOS app icon and launch image (mobile/ios, committed):
//
//	go run ./tools/appicon -kind mobile -size 1024 -out ../mobile/ios/App/App/Assets.xcassets/AppIcon.appiconset/AppIcon-512@2x.png
//
// The launch image's Contents.json points all three scales (1x/2x/3x) at a
// same-sized 2732x2732 image, so all three files need the same command:
//
//	go run ./tools/appicon -kind launch -size 2732 -out ../mobile/ios/App/App/Assets.xcassets/Splash.imageset/splash-2732x2732.png
//	go run ./tools/appicon -kind launch -size 2732 -out ../mobile/ios/App/App/Assets.xcassets/Splash.imageset/splash-2732x2732-1.png
//	go run ./tools/appicon -kind launch -size 2732 -out ../mobile/ios/App/App/Assets.xcassets/Splash.imageset/splash-2732x2732-2.png
//
// Console public assets (frontend/console/public, committed) and the brand
// source (docs/brand/tars-icon.png, also committed) — apple-touch-icon
// reuses -kind mobile's full-bleed treatment since iOS rounds the corners
// itself; the PWA's non-maskable icons and the favicon/nav/login mark reuse
// -kind app's rounded square:
//
//	go run ./tools/appicon -kind app -size 512 -out ../docs/brand/tars-icon.png
//	go run ./tools/appicon -kind app -size 512 -out ../docs/brand/tars-avatar.png
//	go run ./tools/appicon -kind app -size 256 -out ../frontend/console/public/tars-icon.png
//	go run ./tools/appicon -kind app -size 256 -out ../frontend/console/public/tars-avatar.png
//	go run ./tools/appicon -kind app -size 32 -out ../frontend/console/public/favicon-32.png
//	go run ./tools/appicon -kind app -size 192 -out ../frontend/console/public/pwa-icon-192.png
//	go run ./tools/appicon -kind app -size 512 -out ../frontend/console/public/pwa-icon-512.png
//	go run ./tools/appicon -kind maskable -size 192 -out ../frontend/console/public/pwa-maskable-192.png
//	go run ./tools/appicon -kind maskable -size 512 -out ../frontend/console/public/pwa-maskable-512.png
//	go run ./tools/appicon -kind mobile -size 180 -out ../frontend/console/public/apple-touch-icon.png
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/devlikebear/tars/desktop/internal/icon"
)

func main() {
	size := flag.Int("size", 1024, "icon size in pixels")
	out := flag.String("out", "appicon.png", "output PNG path")
	kind := flag.String("kind", "app", "app (desktop/favicon/PWA), mobile (iOS icon / apple-touch-icon), launch (iOS launch image) or maskable (PWA maskable icon)")
	flag.Parse()
	draw := map[string]func(int) []byte{
		"app":      icon.App,
		"mobile":   icon.Mobile,
		"launch":   icon.Launch,
		"maskable": icon.Maskable,
	}[*kind]
	if draw == nil {
		log.Fatalf("unknown -kind %q", *kind)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, draw(*size), 0o644); err != nil {
		log.Fatal(err)
	}
}
