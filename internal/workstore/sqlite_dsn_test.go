package workstore

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSQLiteFileURLHasNoHostAndAnAbsolutePath pins the shape that was wrong on
// Windows: url.URL{Scheme: "file", Path: `C:\dir\ledger.db`} put the drive
// letter in the host and percent-encoded every separator, so SQLite could not
// open the database at all.
func TestSQLiteFileURLHasNoHostAndAnAbsolutePath(t *testing.T) {
	dsnURL, err := sqliteFileURL(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("build dsn: %v", err)
	}
	if dsnURL.Scheme != "file" {
		t.Fatalf("scheme = %q, want file", dsnURL.Scheme)
	}
	if dsnURL.Host != "" {
		t.Fatalf("host = %q, want empty (a drive letter must not become the host)", dsnURL.Host)
	}
	if !strings.HasPrefix(dsnURL.Path, "/") {
		t.Fatalf("path = %q, want a leading slash", dsnURL.Path)
	}
	if strings.Contains(dsnURL.Path, `\`) {
		t.Fatalf("path = %q, want forward slashes only", dsnURL.Path)
	}
	if dsn := dsnURL.String(); !strings.HasPrefix(dsn, "file:///") {
		t.Fatalf("dsn = %q, want a file:/// prefix", dsn)
	}
}

func TestSQLiteFileURLResolvesRelativePaths(t *testing.T) {
	t.Chdir(t.TempDir())

	dsnURL, err := sqliteFileURL("ledger.db")
	if err != nil {
		t.Fatalf("build dsn: %v", err)
	}
	if !strings.HasPrefix(dsnURL.Path, "/") {
		t.Fatalf("path = %q, want an absolute path", dsnURL.Path)
	}
	if !strings.HasSuffix(dsnURL.Path, "/ledger.db") {
		t.Fatalf("path = %q, want it to end at the requested file", dsnURL.Path)
	}
}
