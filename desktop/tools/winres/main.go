// Command winres writes the Windows resource object the shell links in:
// the application icon, version info, and the manifest that turns on
// per-monitor DPI awareness and Common Controls v6 (the native dialogs need
// it). Run it in the desktop directory before a Windows build; the go tool
// links any rsrc_windows_*.syso there automatically.
//
//	go run ./tools/winres -version 0.38.0 -arch amd64
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"

	"github.com/devlikebear/tars/desktop/internal/icon"
)

func main() {
	ver := flag.String("version", "0.0.0", "product version, e.g. 0.38.0")
	arch := flag.String("arch", "amd64", "amd64 or arm64")
	flag.Parse()

	var target winres.Arch
	switch *arch {
	case "amd64":
		target = winres.ArchAMD64
	case "arm64":
		target = winres.ArchARM64
	default:
		log.Fatalf("unsupported arch %q", *arch)
	}

	img, err := png.Decode(bytes.NewReader(icon.App(256)))
	if err != nil {
		log.Fatal(err)
	}
	ico, err := winres.NewIconFromResizedImage(img, nil)
	if err != nil {
		log.Fatal(err)
	}

	rs := winres.ResourceSet{}
	if err := rs.SetIcon(winres.Name("APPICON"), ico); err != nil {
		log.Fatal(err)
	}
	quad := versionQuad(*ver)
	vi := version.Info{FileVersion: quad, ProductVersion: quad}
	for key, value := range map[string]string{
		version.ProductName:      "TARS",
		version.FileDescription:  "TARS desktop shell",
		version.ProductVersion:   *ver,
		version.FileVersion:      *ver,
		version.OriginalFilename: "tars-desktop.exe",
	} {
		if err := vi.Set(0, key, value); err != nil {
			log.Fatal(err)
		}
	}
	rs.SetVersionInfo(vi)
	rs.SetManifest(winres.AppManifest{
		Identity:            winres.AssemblyIdentity{Name: "com.devlikebear.tars.desktop", Version: quad},
		Description:         "TARS desktop shell",
		ExecutionLevel:      winres.AsInvoker,
		DPIAwareness:        winres.DPIPerMonitorV2,
		UseCommonControlsV6: true,
	})

	name := fmt.Sprintf("rsrc_windows_%s.syso", *arch)
	out, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	if err := rs.WriteObject(out, target); err != nil {
		log.Fatal(err)
	}
	if err := out.Close(); err != nil {
		log.Fatal(err)
	}
}

// versionQuad turns "0.38.0" (or "dev") into the four numbers Windows keeps.
func versionQuad(v string) [4]uint16 {
	var q [4]uint16
	for i, part := range strings.SplitN(strings.TrimPrefix(v, "v"), ".", 4) {
		n, err := strconv.ParseUint(strings.SplitN(part, "-", 2)[0], 10, 16)
		if err != nil {
			break
		}
		q[i] = uint16(n)
	}
	return q
}
