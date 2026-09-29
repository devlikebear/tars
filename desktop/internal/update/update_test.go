package update

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestMatchAsset(t *testing.T) {
	assets := []github.ReleaseAsset{
		{Name: "tars_0.38.0_darwin_arm64.tar.gz"},
		{Name: "checksums.txt"},
		{Name: "tars-desktop_0.38.0_darwin_amd64.tar.gz"},
		{Name: "tars-desktop_0.38.0_darwin_arm64.tar.gz"},
		{Name: "tars-desktop_0.38.0_linux_amd64.tar.gz"},
		{Name: "tars-desktop_0.38.0_windows_amd64.zip"},
		{Name: "tars-desktop_0.38.0_windows_amd64.tar.gz"},
	}
	cases := map[[2]string]int{
		{"darwin", "arm64"}:  3,
		{"darwin", "amd64"}:  2,
		{"linux", "amd64"}:   4,
		{"windows", "amd64"}: 5,
		{"linux", "arm64"}:   -1,
		{"freebsd", "amd64"}: -1,
	}
	for platform, want := range cases {
		got := MatchAsset(updater.CheckRequest{Platform: platform[0], Arch: platform[1]}, assets)
		if got != want {
			t.Errorf("%s/%s = %d, want %d", platform[0], platform[1], got, want)
		}
	}
	// The stock matcher would have taken the CLI archive.
	if github.DefaultAssetMatcher(updater.CheckRequest{Platform: "darwin", Arch: "arm64"}, assets) != 0 {
		t.Fatal("expected the stock matcher to pick the CLI; the custom one exists for that reason")
	}
}

func TestAssetName(t *testing.T) {
	if got := AssetName("0.38.0", "windows", "amd64"); got != "tars-desktop_0.38.0_windows_amd64.zip" {
		t.Fatal(got)
	}
	if got := AssetName("0.38.0", "darwin", "arm64"); got != "tars-desktop_0.38.0_darwin_arm64.tar.gz" {
		t.Fatal(got)
	}
	if MatchAsset(updater.CheckRequest{Platform: "linux", Arch: "amd64"}, []github.ReleaseAsset{{Name: AssetName("1.0.0", "linux", "amd64")}}) != 0 {
		t.Fatal("the matcher must accept what AssetName produces")
	}
}

func TestProviderAndEnabled(t *testing.T) {
	p, err := Provider()
	if err != nil || p.Name() != "github" {
		t.Fatalf("provider = %v, %v", p, err)
	}
	for v, want := range map[string]bool{"0.38.0": true, "v1.2.3": true, "": false, "dev": false, "0.0.0-dev": false} {
		if Enabled(v) != want {
			t.Errorf("Enabled(%q) != %v", v, want)
		}
	}
}
