//go:build integration

package easyproject

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestLivePreview scaffolds a real project (network, npm), runs its
// development server through NPMPreviewRunner and reads the page from the
// address the result screen would show. Then it stops the preview and
// checks the port is free again (Vite died with npm). Skipped without npm.
//
//	go test -tags integration ./internal/easyproject -run TestLivePreview -v
func TestLivePreview(t *testing.T) {
	if err := (NPMScaffolder{}).Ready(); err != nil {
		t.Skip("npm is not installed")
	}
	dir, err := NewFolder(t.TempDir(), "A sign-up page for a book club")
	if err != nil {
		t.Fatal(err)
	}
	if err := (NPMScaffolder{}).Scaffold(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	ps := &Previews{Runner: NPMPreviewRunner{}, LogDir: t.TempDir()}
	defer ps.Close()
	start := time.Now()
	ps.Start("0123456789abcdef", dir)
	var p Preview
	for {
		p = ps.Status("0123456789abcdef")
		if p.State != PreviewStarting {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if p.State != PreviewRunning {
		t.Fatalf("preview = %+v", p)
	}
	t.Logf("first answer after %s at %s", time.Since(start).Round(100*time.Millisecond), p.URL)
	resp, err := http.Get(p.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `id="root"`) {
		t.Fatalf("page = %d %.300s", resp.StatusCode, body)
	}
	ps.Stop("0123456789abcdef")
	if _, err := (&http.Client{Timeout: time.Second}).Get(p.URL); err == nil {
		t.Fatal("the stopped preview still answers")
	}
}
