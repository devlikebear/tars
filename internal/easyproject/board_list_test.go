package easyproject

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func listedAt(i int, state State, at time.Time, archived bool) Listed {
	l := Listed{Project: Project{ID: fmt.Sprintf("%016x", i)}, Board: Board{State: state, ChangedAt: at}}
	if archived {
		l.ArchivedAt = &at
	}
	return l
}

func TestPageOrdersCountsAndPages(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	var all []Listed
	for i := range 30 {
		state := StateDone
		if i%3 == 0 {
			state = StateYourTurn
		}
		// Two projects share each time: the id breaks the tie.
		all = append(all, listedAt(i, state, t0.Add(time.Duration(i/2)*time.Minute), false))
	}
	all = append(all, listedAt(99, StateDone, t0.Add(time.Hour), true))

	page, err := Page(all, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Projects) != 30 || page.Next != "" || page.Archived != 1 ||
		page.Counts[StateYourTurn] != 10 || page.Counts[StateDone] != 20 || page.Counts[StateStuck] != 0 {
		t.Fatalf("all: %d projects, next %q, counts %v, archived %d", len(page.Projects), page.Next, page.Counts, page.Archived)
	}
	if page.Projects[0].ID != fmt.Sprintf("%016x", 29) || page.Projects[29].ID != fmt.Sprintf("%016x", 0) {
		t.Fatalf("order: first %s last %s", page.Projects[0].ID, page.Projects[29].ID)
	}

	// Walking pages of 7 sees every project once, in order.
	var seen []string
	cursor := ""
	for range 10 {
		p, err := Page(all, ListQuery{Limit: 7, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range p.Projects {
			seen = append(seen, l.ID)
		}
		if p.Next == "" {
			break
		}
		cursor = p.Next
	}
	if len(seen) != 30 {
		t.Fatalf("paged %d projects", len(seen))
	}
	for i, id := range seen {
		if id != page.Projects[i].ID {
			t.Fatalf("page order differs at %d: %s vs %s", i, id, page.Projects[i].ID)
		}
	}

	// A state filter pages within the state; counts stay whole.
	p, _ := Page(all, ListQuery{State: StateYourTurn, Limit: 4})
	if len(p.Projects) != 4 || p.Next == "" || p.Counts[StateDone] != 20 {
		t.Fatalf("your_turn page: %d, next %q, counts %v", len(p.Projects), p.Next, p.Counts)
	}
	for _, l := range p.Projects {
		if l.Board.State != StateYourTurn {
			t.Fatalf("filtered page has %s", l.Board.State)
		}
	}

	archived, _ := Page(all, ListQuery{Archived: true})
	if len(archived.Projects) != 1 || archived.Counts[StateDone] != 1 || archived.Counts[StateYourTurn] != 0 {
		t.Fatalf("archived: %+v", archived)
	}

	for _, q := range []ListQuery{{State: "nope"}, {Cursor: "!!"}, {Cursor: encodeCursor(t0, "short")}} {
		if _, err := Page(all, q); err == nil {
			t.Fatalf("query %+v accepted", q)
		}
	}
	if p, _ := Page(all, ListQuery{Limit: 1000}); len(p.Projects) != 30 {
		t.Fatalf("a large limit is capped at %d, not below the list", MaxListLimit)
	}
}

func TestStoreUpdateAndRemove(t *testing.T) {
	s := NewStore(t.TempDir())
	p := Project{ID: "0123456789abcdef", Name: "a", Status: StatusReady}
	if err := s.Put(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Update(p.ID, func(p Project) (Project, error) { p.Name = "b"; p.ID = "ignored"; return p, nil })
	if err != nil || got.Name != "b" || got.ID != p.ID {
		t.Fatalf("update = %+v %v", got, err)
	}
	boom := errors.New("boom")
	if _, err := s.Update(p.ID, func(p Project) (Project, error) { return p, boom }); !errors.Is(err, boom) {
		t.Fatalf("update error = %v", err)
	}
	if _, err := s.Update("fedcba9876543210", func(p Project) (Project, error) { return p, nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update unknown = %v", err)
	}
	if err := s.Remove(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after remove: %v", err)
	}
	if err := s.Remove(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove twice = %v", err)
	}
	if err := s.Remove("../x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove bad id = %v", err)
	}
}

func TestCleanName(t *testing.T) {
	if got, err := CleanName("  My\n  shop  "); err != nil || got != "My shop" {
		t.Fatalf("CleanName = %q %v", got, err)
	}
	if _, err := CleanName("   "); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("empty = %v", err)
	}
	long := make([]rune, nameRunes+1)
	for i := range long {
		long[i] = '가'
	}
	if _, err := CleanName(string(long)); !errors.Is(err, ErrNameTooLong) {
		t.Fatalf("long = %v", err)
	}
}

func TestDeleteFolderMovesOnlyProjectFolders(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "TARS Projects")
	outside := filepath.Join(root, "Documents")
	for _, dir := range []string{filepath.Join(base, "shop"), filepath.Join(base, "shop", "src"), outside, filepath.Join(base, ".hidden")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "shop", "src", "app.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.Local)

	refused := []string{
		outside,                                // outside the projects directory
		base,                                   // the projects directory itself
		root,                                   // its parent
		filepath.Join(base, "shop", "src"),     // inside a project, not a project
		filepath.Join(base, "..", "Documents"), // escaping with ..
		filepath.Join(base, ".hidden"),         // not a project name
		filepath.Join(base, DeletedDirName),    // the trash itself
		filepath.Join(base, "notes.txt"),       // not a folder
		"",
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(base, "linked")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		refused = append(refused, link) // a link to elsewhere
		// The projects directory reached through a link is still checked
		// by where it really is.
		baseLink := filepath.Join(root, "projects-link")
		if err := os.Symlink(outside, baseLink); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(outside, "victim"), 0o755); err != nil {
			t.Fatal(err)
		}
		refused = append(refused, filepath.Join(baseLink, "victim"))
	}
	if err := os.MkdirAll(filepath.Join(base, DeletedDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, folder := range refused {
		if dest, err := DeleteFolder(base, folder, now); !errors.Is(err, ErrOutsideProjects) || dest != "" {
			t.Fatalf("DeleteFolder(%q) = %q, %v; want refused", folder, dest, err)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside folder touched: %v", err)
	}

	dest, err := DeleteFolder(base, filepath.Join(base, "shop"), now)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, DeletedDirName, "shop-20261124-100000")
	if dest != want {
		t.Fatalf("moved to %q, want %q", dest, want)
	}
	if _, err := os.Stat(filepath.Join(dest, "src", "app.js")); err != nil {
		t.Fatalf("moved folder lost its files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "shop")); !os.IsNotExist(err) {
		t.Fatalf("folder still in place: %v", err)
	}

	// A second project of the same name deleted in the same second.
	if err := os.MkdirAll(filepath.Join(base, "shop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if dest, err := DeleteFolder(base, filepath.Join(base, "shop"), now); err != nil || dest != want+"-2" {
		t.Fatalf("second delete = %q %v", dest, err)
	}
	// A folder already gone moves nothing.
	if dest, err := DeleteFolder(base, filepath.Join(base, "gone"), now); err != nil || dest != "" {
		t.Fatalf("gone = %q %v", dest, err)
	}
}
