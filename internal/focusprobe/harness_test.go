package focusprobe

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadHarnessFiles(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "CLAUDE.md"), "# Title\n\nText\n```sh\n# not a heading\n```\n## Build\n#nospace\n####### too deep\n#\n")
	writeFile(t, filepath.Join(repo, ".tars", "skills", "lint", "SKILL.md"), "---\nname: lint\ndescription: Lint the tree\n---\nBody\n")
	writeFile(t, filepath.Join(repo, ".tars", "skills", "lint", "run.sh"), "#!/bin/sh\n")
	writeFile(t, filepath.Join(repo, ".tars", "skills", "lint", "scripts", "check.py"), "print()\n")
	writeFile(t, filepath.Join(repo, ".tars", "skills", "bare", "notes.txt"), "x")
	writeFile(t, filepath.Join(repo, ".tars", "skills", ".hidden", "SKILL.md"), "x")
	writeFile(t, filepath.Join(repo, ".tars", "skills", "loose.md"), "x")

	got := ReadHarnessFiles(repo)
	if len(got.Guides) != 1 || got.Guides[0].Path != "CLAUDE.md" || strings.Join(got.Guides[0].Headings, "|") != "# Title|## Build" {
		t.Fatalf("guides = %+v", got.Guides)
	}
	if len(got.Skills) != 2 {
		t.Fatalf("skills = %+v", got.Skills)
	}
	bare, lint := got.Skills[0], got.Skills[1]
	if bare.Name != "bare" || bare.Description != "" || strings.Join(bare.Files, ",") != "notes.txt" {
		t.Errorf("bare = %+v", bare)
	}
	if lint.Name != "lint" || lint.Description != "Lint the tree" || strings.Join(lint.Files, ",") != "SKILL.md,run.sh,scripts/check.py" {
		t.Errorf("lint = %+v", lint)
	}

	if empty := ReadHarnessFiles(""); len(empty.Guides)+len(empty.Skills) != 0 {
		t.Fatalf("no repo = %+v", empty)
	}
	if none := ReadHarnessFiles(t.TempDir()); len(none.Guides)+len(none.Skills) != 0 {
		t.Fatalf("empty repo = %+v", none)
	}
}

func TestReadHarnessFilesSkipsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.md"), "# Secret\n")
	writeFile(t, filepath.Join(outside, "skills", "x", "SKILL.md"), "---\ndescription: d\n---\n")
	repo := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(repo, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".tars"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "skills"), filepath.Join(repo, ".tars", "skills")); err != nil {
		t.Fatal(err)
	}
	if got := ReadHarnessFiles(repo); len(got.Guides)+len(got.Skills) != 0 {
		t.Fatalf("followed a link: %+v", got)
	}
	if _, ok := readRegular(filepath.Join(repo, "missing")); ok {
		t.Fatal("read a missing file")
	}
}
