// Command appicon writes the application icon as a PNG, for packaging:
// macOS turns it into an .icns, Windows embeds it in the executable.
//
//	go run ./tools/appicon -size 1024 -out build/appicon.png
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
	flag.Parse()
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, icon.App(*size), 0o644); err != nil {
		log.Fatal(err)
	}
}
