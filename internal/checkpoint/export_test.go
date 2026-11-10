package checkpoint

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExportTurn(t *testing.T) {
	s := newTestStore(t, Options{})
	repo := initUserRepo(t)
	writeFile(t, repo, "app/x.go", "defect\n")
	writeFile(t, repo, "keep.txt", "same\n")
	writeFile(t, repo, "gone.txt", "removed by the fix\n")
	entry := runTurn(t, s, "s1", "t1", repo, func() {
		writeFile(t, repo, "app/x.go", "fixed\n")
		writeFile(t, repo, "new/added.txt", "added\n")
		if err := os.Remove(filepath.Join(repo, "gone.txt")); err != nil {
			t.Fatal(err)
		}
	})
	if entry.Start == "" || entry.End == "" {
		t.Fatalf("no checkpoint: %+v", entry)
	}
	ctx := context.Background()
	start, end := t.TempDir(), t.TempDir()
	if err := s.ExportTurn(ctx, "s1", "t1", SideStart, start); err != nil {
		t.Fatal(err)
	}
	if err := s.ExportTurn(ctx, "s1", "t1", SideEnd, end); err != nil {
		t.Fatal(err)
	}
	before, after := fingerprint(t, start), fingerprint(t, end)
	if len(before) != 3 || before["gone.txt"] == "" || before["app/x.go"] == after["app/x.go"] {
		t.Fatalf("start = %v", before)
	}
	if len(after) != 3 || after["new/added.txt"] == "" || after["gone.txt"] != "" || after["keep.txt"] != before["keep.txt"] {
		t.Fatalf("end = %v", after)
	}
	// The work tree itself is untouched by an export.
	if got, _ := os.ReadFile(filepath.Join(repo, "app/x.go")); string(got) != "fixed\n" {
		t.Fatalf("work tree changed: %q", got)
	}

	if err := s.ExportTurn(ctx, "s1", "t1", "middle", t.TempDir()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad side: %v", err)
	}
	if err := s.ExportTurn(ctx, "s1", "nope", SideStart, t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown turn: %v", err)
	}
	if err := s.ExportTurn(ctx, "../x", "t1", SideStart, t.TempDir()); err == nil {
		t.Fatal("bad session id")
	}
	if err := s.ExportTurn(ctx, "s1", "../t", SideStart, t.TempDir()); err == nil {
		t.Fatal("bad turn id")
	}
}

func tarOf(t *testing.T, entries ...tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, h := range entries {
		h := h
		body := []byte("x")
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(body))
		}
		if err := w.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := w.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractTarStaysInside(t *testing.T) {
	for _, name := range []string{"../escape", "/abs"} {
		dest := t.TempDir()
		err := extractTar(bytes.NewReader(tarOf(t, tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644})), dest)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	dest := t.TempDir()
	err := extractTar(bytes.NewReader(tarOf(t,
		tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader},
		tar.Header{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0o755},
		tar.Header{Name: "dir/run.sh", Typeflag: tar.TypeReg, Mode: 0o755},
		tar.Header{Name: "in", Typeflag: tar.TypeSymlink, Linkname: "dir/run.sh"},
		tar.Header{Name: "out", Typeflag: tar.TypeSymlink, Linkname: "../../etc/passwd"},
		tar.Header{Name: "abs", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
	)), dest)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dest, "dir/run.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("executable lost: %v %v", info, err)
	}
	if link, err := os.Readlink(filepath.Join(dest, "in")); err != nil || link != "dir/run.sh" {
		t.Fatalf("inside link: %q %v", link, err)
	}
	for _, name := range []string{"out", "abs", "pax_global_header"} {
		if _, err := os.Lstat(filepath.Join(dest, name)); !os.IsNotExist(err) {
			t.Errorf("%s was written", name)
		}
	}
	if err := extractTar(bytes.NewReader([]byte("not a tar archive at all, padded to look like one")), t.TempDir()); err == nil {
		t.Fatal("a broken archive is an error")
	}
}
