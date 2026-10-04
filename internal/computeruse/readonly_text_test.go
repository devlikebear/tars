package computeruse

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseReadOnlyText(t *testing.T) {
	tree := strings.Join([]string{
		`- [0] AXWindow "Calc" [id=main actions=[raise]]`,
		`    - AXStaticText = "7,777" (edit field)`,
		`    - [1] AXButton (Delete) [id=Delete help="Removes the last digit." actions=[press]]`,
		`    - AXGroup`,
		`    - AXStaticText = "7,777" (edit field)`,
		`    - AXHeading "Result" [id=heading]`,
		`- [30] AXMenuBar`,
		`    - [31] AXMenuBarItem "File"`,
		`        - AXStaticText = "secret-recent-file.txt"`,
		`- AXStaticText = "after the menu bar"`,
	}, "\n")
	got := parseReadOnlyText(tree)
	want := []string{`AXStaticText = "7,777" (edit field)`, `AXHeading "Result"`, `AXStaticText = "after the menu bar"`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("texts = %#v\nwant   %#v", got, want)
	}
	if parseReadOnlyText("") != nil {
		t.Fatal("empty tree must yield no text")
	}

	long := "- AXStaticText = \"" + strings.Repeat("가", 400) + "\""
	if n := len([]rune(parseReadOnlyText(long)[0])); n != maxReadOnlyTextRunes+1 {
		t.Fatalf("long line kept %d runes, want %d plus the ellipsis", n, maxReadOnlyTextRunes)
	}
	var many []string
	for i := 0; i < maxReadOnlyTexts+20; i++ {
		many = append(many, "- AXStaticText = \"line "+strings.Repeat("x", i)+"\"")
	}
	if n := len(parseReadOnlyText(strings.Join(many, "\n"))); n != maxReadOnlyTexts {
		t.Fatalf("kept %d lines, want the cap %d", n, maxReadOnlyTexts)
	}
}

// The Calculator's display is not an element: it only exists in the tree text.
func TestParseWindowState_CalculatorFixtureCarriesTheDisplay(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "get_window_state_calculator.json"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := parseWindowState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Texts) == 0 || !strings.HasPrefix(snap.Texts[0], `AXStaticText = "0"`) {
		t.Fatalf("texts = %#v, want the display first", snap.Texts)
	}
	for _, line := range snap.Texts {
		if strings.Contains(line, "AXMenu") {
			t.Errorf("menu content leaked into read-only text: %s", line)
		}
	}
}

func TestRenderState_ReadOnlyTextFollowsExposeValues(t *testing.T) {
	snap := Snapshot{
		Elements: []Element{{Index: 1, Role: "AXButton", Label: "7", Enabled: true}},
		Texts:    []string{`AXStaticText = "42"`},
	}
	shown, _ := RenderState(Request{Goal: "g"}, snap, nil, RenderOptions{ExposeValues: true})
	if !strings.Contains(shown, "TEXT ON SCREEN (read-only):\n  AXStaticText = \"42\"\n") {
		t.Fatalf("state lacks the read-only text:\n%s", shown)
	}
	hidden, _ := RenderState(Request{Goal: "g"}, snap, nil, RenderOptions{ExposeValues: false})
	if strings.Contains(hidden, "42") || strings.Contains(hidden, "TEXT ON SCREEN") {
		t.Fatalf("expose_values=false still sent read-only text:\n%s", hidden)
	}
}

func TestElementHash_ChangesWithReadOnlyText(t *testing.T) {
	els := []Element{{Index: 1, Role: "AXButton", Label: "7", Enabled: true}}
	before := ElementHash(Snapshot{Elements: els, Texts: []string{`AXStaticText = "0"`}})
	after := ElementHash(Snapshot{Elements: els, Texts: []string{`AXStaticText = "7"`}})
	if before == after {
		t.Fatal("a changed display hashed the same; the loop would count the click as no_change")
	}
}

// cua-driver 0.32 keeps snapshots per session and gives every CLI process its
// own, so each call must carry the same label or the next click is refused as
// stale_element_token.
func TestCuaDriver_EveryCallCarriesTheSharedSession(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t, `{"effect":"confirmed","elements":[],"windows":[]}`, &argv)
	w := Window{PID: 7, WindowID: 9}
	calls := map[string]func(){
		"list_windows":     func() { _ = d.Ping(context.Background()) },
		"get_window_state": func() { _, _ = d.Snapshot(context.Background(), w, SnapshotOpts{}) },
		"click":            func() { _, _ = d.Click(context.Background(), w, "s1:3") },
		"type_text":        func() { _, _ = d.TypeText(context.Background(), w, "s1:3", "x") },
		"set_value":        func() { _, _ = d.SetValue(context.Background(), w, "s1:3", "x") },
		"press_key":        func() { _, _ = d.PressKey(context.Background(), w, "return") },
		"scroll":           func() { _, _ = d.Scroll(context.Background(), w, "down") },
	}
	for tool, do := range calls {
		argv = nil
		do()
		if len(argv) != 2 || argv[0] != tool || !strings.Contains(argv[1], `"session":"`+cuaDriverSession+`"`) {
			t.Errorf("%s: argv = %v, want the shared session label", tool, argv)
		}
	}
}

func TestRejectsSessionArg(t *testing.T) {
	cases := map[string]bool{
		"computeruse: cua-driver click failed: unknown field `session`":        true,
		"computeruse: cua-driver click failed: additional property session":    true,
		"computeruse: cua-driver click failed: stale_element_token":            false,
		"computeruse: cua-driver click failed: session expired":                false,
		"computeruse: cua-driver get_window_state failed: window_id_not_found": false,
	}
	for msg, want := range cases {
		if got := rejectsSessionArg(errString(msg)); got != want {
			t.Errorf("rejectsSessionArg(%q) = %t, want %t", msg, got, want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// A driver from before sessions existed refuses the argument once; after that
// the label is dropped for good.
func TestCuaDriver_DropsSessionForADriverThatRejectsIt(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	var seen []string
	d := NewCuaDriver("cua-driver", 5*time.Second)
	d.commandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		seen = append(seen, args[1])
		if strings.Contains(args[1], `"session"`) {
			return exec.CommandContext(ctx, "sh", "-c", "echo 'unknown field `session`' >&2; exit 1")
		}
		return exec.CommandContext(ctx, "sh", "-c", `printf '%s' '{"effect":"confirmed"}'`)
	}
	w := Window{PID: 7, WindowID: 9}
	for i := 0; i < 2; i++ {
		if eff, err := d.Click(context.Background(), w, "s1:3"); err != nil || eff != EffectConfirmed {
			t.Fatalf("click %d: eff=%v err=%v", i, eff, err)
		}
	}
	if len(seen) != 3 || !strings.Contains(seen[0], `"session"`) || strings.Contains(seen[1], `"session"`) || strings.Contains(seen[2], `"session"`) {
		t.Fatalf("calls = %v, want one labelled attempt then unlabelled ones", seen)
	}
}
