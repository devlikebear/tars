package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const sample = `package sample

import "context"

// Exported and unexported top-level declarations.
type Widget struct {
	Name    string
	Size    int
	hidden  bool
}

type hiddenType struct{ Name string }

type Doer interface {
	Do(ctx context.Context) error
	private()
}

const (
	ModeFast = "fast"
	modeSlow = "slow"
)

var (
	Default = Widget{}
	cache   = map[string]Widget{}
)

const Single = 1

var Registry = map[string]int{}

func New() *Widget { return &Widget{} }

func helper() {}

func (w *Widget) Resize(n int) { w.Size = n }

func (w *Widget) shrink() {}

func (h hiddenType) Exported() {}
`

func parseSample(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", sample, 0)
	if err != nil {
		t.Fatalf("parse sample: %v", err)
	}
	return declLines("pkg/sample", file)
}

func TestDeclLines_RecordsExportedDeclarationsOnly(t *testing.T) {
	got := parseSample(t)

	want := []string{
		"pkg/sample const ModeFast",
		"pkg/sample const Single",
		"pkg/sample field Widget.Name",
		"pkg/sample field Widget.Size",
		"pkg/sample func New",
		"pkg/sample method Doer.Do",
		"pkg/sample method Widget.Resize",
		"pkg/sample type Doer",
		"pkg/sample type Widget",
		"pkg/sample var Default",
		"pkg/sample var Registry",
	}
	for _, line := range want {
		if !slices.Contains(got, line) {
			t.Errorf("missing %q\ngot: %v", line, got)
		}
	}
}

func TestDeclLines_OmitsUnexportedAndUnreachable(t *testing.T) {
	got := parseSample(t)

	// Unexported declarations, unexported struct fields and interface
	// methods, and — the subtle one — an exported method on an unexported
	// type, which no external consumer can reach.
	for _, unwanted := range []string{
		"pkg/sample type hiddenType",
		"pkg/sample field Widget.hidden",
		"pkg/sample method Doer.private",
		"pkg/sample const modeSlow",
		"pkg/sample var cache",
		"pkg/sample func helper",
		"pkg/sample method Widget.shrink",
		"pkg/sample method hiddenType.Exported",
	} {
		if slices.Contains(got, unwanted) {
			t.Errorf("recorded %q, which is not reachable public API\ngot: %v", unwanted, got)
		}
	}
}

func TestCollect_IsSortedAndStable(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pkg", "beta")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("zeta.go", "package beta\n\nfunc Zeta() {}\n")
	write("alpha.go", "package beta\n\nfunc Alpha() {}\n")
	// Tests are not API and must not appear.
	write("alpha_test.go", "package beta\n\nfunc TestAlpha() {}\n")

	first, err := collect(filepath.Join(root, "pkg"))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !slices.IsSorted(first) {
		t.Fatalf("output is not sorted: %v", first)
	}
	for _, line := range first {
		if strings.Contains(line, "TestAlpha") {
			t.Fatalf("a test function reached the snapshot: %v", first)
		}
	}

	second, err := collect(filepath.Join(root, "pkg"))
	if err != nil {
		t.Fatalf("collect again: %v", err)
	}
	if !slices.Equal(first, second) {
		t.Fatalf("collect is not deterministic:\n%v\n%v", first, second)
	}
}

func TestReceiverName(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "recv.go", `package p

type T struct{}

func (t T) Value() {}
func (t *T) Pointer() {}
func Free() {}
`, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	lines := declLines("pkg/p", file)
	for _, want := range []string{"pkg/p method T.Value", "pkg/p method T.Pointer", "pkg/p func Free"} {
		if !slices.Contains(lines, want) {
			t.Errorf("missing %q from %v", want, lines)
		}
	}
}

// fixtureRoot builds a throwaway module layout with one public package, so
// run's two modes can be exercised without touching the real tree.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pkgDir := filepath.Join(root, "pkg", "widget")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	body := "package widget\n\nfunc Make() {}\n\nfunc hidden() {}\n"
	if err := os.WriteFile(filepath.Join(pkgDir, "widget.go"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return root
}

func TestRun_PrintsToStdoutByDefault(t *testing.T) {
	root := fixtureRoot(t)
	var stdout, stderr strings.Builder

	if code := run([]string{"-root", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "pkg/widget func Make") {
		t.Errorf("output missing the exported function:\n%s", out)
	}
	if strings.Contains(out, "hidden") {
		t.Errorf("output leaked an unexported function:\n%s", out)
	}
	if !strings.Contains(out, "make api-snapshot") {
		t.Error("output is missing the header")
	}
	// Default mode must not write anything.
	if _, err := os.Stat(filepath.Join(root, snapshotPath)); !os.IsNotExist(err) {
		t.Error("default mode wrote the snapshot file")
	}
}

func TestRun_WriteModeUpdatesTheSnapshot(t *testing.T) {
	root := fixtureRoot(t)
	var stdout, stderr strings.Builder

	if code := run([]string{"-root", root, "-write"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("write mode printed to stdout: %q", stdout.String())
	}
	written, err := os.ReadFile(filepath.Join(root, snapshotPath))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(written), "pkg/widget func Make") {
		t.Errorf("snapshot missing the exported function:\n%s", written)
	}
	if !strings.Contains(stderr.String(), "wrote") {
		t.Errorf("write mode did not report on stderr: %q", stderr.String())
	}
}

func TestRun_ReportsAMissingTree(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run([]string{"-root", filepath.Join(t.TempDir(), "absent")}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "apisnapshot:") {
		t.Errorf("error not reported on stderr: %q", stderr.String())
	}
}

func TestRun_ReportsBadFlags(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run([]string{"-nope"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2 for a flag error", code)
	}
}

func TestHeaderPointsAtTheRegenerationCommand(t *testing.T) {
	// The header is what a contributor reads when CI fails, so it has to
	// name the command that fixes it.
	if !strings.Contains(header(), "make api-snapshot") {
		t.Fatal("header does not name the regeneration command")
	}
}
