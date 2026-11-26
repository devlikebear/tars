package easyproject

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)

func TestSlug(t *testing.T) {
	for idea, want := range map[string]string{
		"A sign-up page for a book club":     "a-sign-up-page-for-a-book-club",
		"  Todo   list!! v2  ":               "todo-list-v2",
		"동네 독서모임 신청 페이지":                     "동네-독서모임-신청-페이지",
		"독서모임 sign-up page":                  "독서모임-sign-up-page",
		"Café menu\nwith prices on page two": "café-menu",
		"Cafe\u0301 menu":                    "cafe\u0301-menu",
		"日本語のページ":                            "日本語のページ",
		"a/b\\c:d*e?f\"g<h>i|j":              "a-b-c-d-e-f-g-h-i-j",
		"../../etc/passwd":                   "etc-passwd",
		"!!! 🎉 ???":                          "project",
		strings.Repeat("가나다 ", 20):           strings.TrimSuffix(strings.Repeat("가나다-", 10), "-"),
		strings.Repeat("abc ", 30):           "abc-abc-abc-abc-abc-abc-abc-abc-abc-abc",
	} {
		if got := Slug(idea); got != want {
			t.Errorf("Slug(%q) = %q, want %q", idea, got, want)
		}
	}
}

func TestCleanIdeaAndName(t *testing.T) {
	if _, err := CleanIdea("  \n "); !errors.Is(err, ErrIdeaRequired) {
		t.Fatalf("empty idea: %v", err)
	}
	if _, err := CleanIdea(strings.Repeat("가", MaxIdeaRunes+1)); !errors.Is(err, ErrIdeaTooLong) {
		t.Fatalf("long idea: %v", err)
	}
	if got, _ := CleanIdea("  a page \n"); got != "a page" {
		t.Fatalf("clean = %q", got)
	}
	if got := NameOf("First line\nsecond line"); got != "First line" {
		t.Fatalf("name = %q", got)
	}
	if got := []rune(NameOf(strings.Repeat("가", 100))); len(got) != nameRunes || got[len(got)-1] != '…' {
		t.Fatalf("long name = %q", string(got))
	}
}

func TestNewFolderNeverReusesAFolder(t *testing.T) {
	base := filepath.Join(t.TempDir(), "TARS Projects")
	first, err := NewFolder(base, "Book club")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFolder(base, "Book club")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "book-club" || filepath.Base(second) != "book-club-2" {
		t.Fatalf("folders %s, %s", first, second)
	}
}

func TestBaseDir(t *testing.T) {
	t.Setenv(BaseDirEnv, "")
	home, _ := os.UserHomeDir()
	if got, err := BaseDir(); err != nil || got != filepath.Join(home, DefaultBaseDirName) {
		t.Fatalf("default base = %q, %v", got, err)
	}
	dir := t.TempDir()
	t.Setenv(BaseDirEnv, dir)
	if got, _ := BaseDir(); got != dir {
		t.Fatalf("base from env = %q", got)
	}
}

func TestStore(t *testing.T) {
	s := NewStore(t.TempDir())
	if list, err := s.List(); err != nil || len(list) != 0 {
		t.Fatalf("empty store: %v %v", list, err)
	}
	older := Project{ID: newID(), Name: "old", Status: StatusReady, CreatedAt: t0}
	newer := Project{ID: newID(), Name: "new", Status: StatusCreating, CreatedAt: t0.Add(time.Hour)}
	for _, p := range []Project{older, newer} {
		if err := s.Put(p); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.List()
	if len(list) != 2 || list[0].ID != newer.ID {
		t.Fatalf("list = %+v", list)
	}
	newer.Name = "renamed"
	if err := s.Put(newer); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(newer.ID); err != nil || got.Name != "renamed" {
		t.Fatalf("get = %+v %v", got, err)
	}
	for _, id := range []string{"nope", "../../etc", "0123456789abcdef"} {
		if _, err := s.Get(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("get %q: %v", id, err)
		}
	}
	if n, err := s.MarkInterrupted(t0); err != nil || n != 1 {
		t.Fatalf("interrupted %d %v", n, err)
	}
	got, _ := s.Get(newer.ID)
	if got.Status != StatusFailed || got.ErrorCode != ErrorInterrupted {
		t.Fatalf("after restart: %+v", got)
	}
	if n, _ := s.MarkInterrupted(t0); n != 0 {
		t.Fatal("marked twice")
	}
}

type fakeScaffolder struct {
	notReady bool
	fail     error
}

func (f fakeScaffolder) Ready() error {
	if f.notReady {
		return ErrNodeMissing
	}
	return nil
}

func (f fakeScaffolder) Scaffold(_ context.Context, dir string) error {
	if f.fail != nil {
		return f.fail
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}\n"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n"), 0o644)
}

func newCreator(t *testing.T, s Scaffolder, open Opener) (*Creator, string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(tmp, "projects")
	return &Creator{
		Store: NewStore(t.TempDir()), BaseDir: func() (string, error) { return base, nil },
		Scaffolder: s, Open: open, Now: func() time.Time { return t0 },
	}, base
}

func TestCreatorMakesAReadyProject(t *testing.T) {
	var opened string
	c, base := newCreator(t, fakeScaffolder{}, func(_ context.Context, folder, idea string) (string, error) {
		opened = folder + "|" + idea
		return "sess-1", nil
	})
	p, err := c.Start(context.Background(), "  Book club sign-up  ")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusCreating || p.Name != "Book club sign-up" || p.Folder != filepath.Join(base, "book-club-sign-up") {
		t.Fatalf("started: %+v", p)
	}
	c.Wait()
	got, err := c.Store.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusReady || got.SessionID != "sess-1" || got.ErrorCode != "" {
		t.Fatalf("finished: %+v", got)
	}
	if opened != p.Folder+"|Book club sign-up" {
		t.Fatalf("opened %q", opened)
	}
	out, err := exec.Command("git", "-C", p.Folder, "log", "--format=%an %s").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "TARS Start the project" {
		t.Fatalf("first version: %q", out)
	}
}

func TestSaveVersion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InitGit(ctx, dir); err != nil {
		t.Fatal(err)
	}
	first, err := SaveVersion(ctx, dir, "nothing new")
	if err != nil || first == "" {
		t.Fatalf("unchanged: %q %v", first, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := SaveVersion(ctx, dir, "Change the text")
	if err != nil || second == first {
		t.Fatalf("changed: %q %v", second, err)
	}
	out, _ := exec.Command("git", "-C", dir, "log", "--format=%an|%s").Output()
	if got := strings.TrimSpace(string(out)); got != "TARS|Change the text\nTARS|Start the project" {
		t.Fatalf("history:\n%s", got)
	}
	if _, err := SaveVersion(ctx, t.TempDir(), "x"); err == nil || !strings.Contains(err.Error(), "git status") {
		t.Fatalf("no git: %v", err)
	}
}

func TestCreatorRefusesBeforeMakingAnything(t *testing.T) {
	c, base := newCreator(t, fakeScaffolder{notReady: true}, nil)
	if _, err := c.Start(context.Background(), "a page"); !errors.Is(err, ErrNodeMissing) {
		t.Fatalf("no node: %v", err)
	}
	if _, err := c.Start(context.Background(), " "); !errors.Is(err, ErrIdeaRequired) {
		t.Fatalf("no idea: %v", err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("a folder was made: %v", err)
	}
	if list, _ := c.Store.List(); len(list) != 0 {
		t.Fatalf("recorded: %+v", list)
	}
}

func TestCreatorFailuresRemoveTheFolder(t *testing.T) {
	for name, tc := range map[string]struct {
		s    Scaffolder
		open Opener
		code string
	}{
		"scaffold": {fakeScaffolder{fail: errors.New("network down")}, nil, ErrorScaffoldFailed},
		"node":     {fakeScaffolder{fail: ErrNodeMissing}, nil, ErrorNodeMissing},
		"pipeline": {fakeScaffolder{}, func(context.Context, string, string) (string, error) { return "", errors.New("503") }, ErrorPipelineFailed},
	} {
		t.Run(name, func(t *testing.T) {
			var reported error
			c, _ := newCreator(t, tc.s, tc.open)
			c.OnFailure = func(_ Project, err error) { reported = err }
			p, err := c.Start(context.Background(), "a page")
			if err != nil {
				t.Fatal(err)
			}
			c.Wait()
			got, _ := c.Store.Get(p.ID)
			if got.Status != StatusFailed || got.ErrorCode != tc.code || got.SessionID != "" {
				t.Fatalf("failed project: %+v", got)
			}
			if reported == nil {
				t.Fatal("the failure was not reported")
			}
			if _, err := os.Stat(p.Folder); !os.IsNotExist(err) {
				t.Fatalf("the half-made folder is left: %v", err)
			}
		})
	}
}

func TestInside(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "home", "me", "TARS Projects")
	for path, want := range map[string]bool{
		filepath.Join(base, "a"):          true,
		base:                              false,
		filepath.Dir(base):                false,
		filepath.Join(base, "..", "else"): false,
	} {
		if got := inside(base, path); got != want {
			t.Errorf("inside(%q) = %v", path, got)
		}
	}
}

func TestScriptScaffolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a #!/bin/sh script")
	}
	dir := t.TempDir()
	script := filepath.Join(t.TempDir(), "scaffold.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{}' > \"$1/package.json\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ScaffoldPathEnv, script)
	s := ScaffolderFromEnv()
	if err := s.Ready(); err != nil {
		t.Fatal(err)
	}
	if err := s.Scaffold(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		t.Fatal(err)
	}
	if err := (ScriptScaffolder{Path: filepath.Join(dir, "missing")}).Ready(); !errors.Is(err, ErrNodeMissing) {
		t.Fatalf("missing script: %v", err)
	}
	t.Setenv(ScaffoldPathEnv, "")
	if _, ok := ScaffolderFromEnv().(NPMScaffolder); !ok {
		t.Fatal("without the env the npm scaffolder is used")
	}
}
