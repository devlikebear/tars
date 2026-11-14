// Command appicon writes the application icon as a PNG, for packaging:
// macOS turns it into an .icns, Windows embeds it in the executable.
// -kind mobile writes the iOS app's icon, -kind launch its launch image
// (both full bleed and opaque, committed under mobile/ios).
//
//	go run ./tools/appicon -size 1024 -out build/appicon.png
//	go run ./tools/appicon -kind mobile -size 1024 -out ../mobile/ios/App/App/Assets.xcassets/AppIcon.appiconset/AppIcon-512@2x.png
//	go run ./tools/appicon -kind launch -size 2732 -out ../mobile/ios/App/App/Assets.xcassets/Splash.imageset/splash-2732x2732.png
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
	kind := flag.String("kind", "app", "app (desktop), mobile (iOS icon) or launch (iOS launch image)")
	flag.Parse()
	draw := map[string]func(int) []byte{"app": icon.App, "mobile": icon.Mobile, "launch": icon.Launch}[*kind]
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
