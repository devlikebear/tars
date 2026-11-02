package computeruse

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// adSnapshotFixture follows what agent-desktop 1.0.2 returned for a test window.
const adSnapshotFixture = `{"app":"Probe","complete":true,"ref_count":8,"snapshot_id":"s1","window":{"id":"w-6758","title":"Quarterly secrets"},
"tree":{"name":"Quarterly secrets","role":"window","children":[
 {"available_actions":["Click","SetValue","SetFocus","TypeText"],"name":"Name field","ref_id":"@s1:e1","role":"textfield","states":["focused"],"value":"ada"},
 {"available_actions":["Click","SetValue","SetFocus"],"name":"Secret","ref_id":"@s1:e2","role":"textfield","states":["secure"],"value":"hunter2"},
 {"name":"count: 1","role":"statictext"},
 {"name":"count: 1","role":"statictext"},
 {"ref_id":"@s1:e9","role":"statictext","value":"= 5"},
 {"available_actions":["Click"],"name":"Disabled","ref_id":"@s1:e4","role":"button","states":["disabled"]},
 {"available_actions":["Click","Toggle"],"name":"Agree","ref_id":"@s1:e5","role":"checkbox","states":["checked"],"value":"1"},
 {"available_actions":["Scroll"],"ref_id":"@s1:e6","role":"scrollbar"},
 {"available_actions":["Scroll"],"ref_id":"@s1:e7","role":"scrollarea","children":[
  {"available_actions":["Click"],"ref_id":"@s1:e8","role":"row","children":[{"name":"row 0","role":"statictext"}]}]},
 {"role":"menubar","children":[{"available_actions":["Click"],"name":"Recent report.pdf","ref_id":"@s1:e20","role":"menuitem"}]}
]}}`

func envelope(data string) string { return `{"ok":true,"version":"2.5","data":` + data + `}` }

func TestParseADSnapshot(t *testing.T) {
	snap, targets, scroller, err := parseADSnapshot(json.RawMessage(adSnapshotFixture))
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]Element{}
	for _, el := range snap.Elements {
		byLabel[el.Label] = el
	}
	if got := byLabel["Name field"]; got.Role != "AXTextField" || got.Value != "ada" || got.Token != "@s1:e1" || !got.Enabled || got.Secure {
		t.Errorf("name field = %+v", got)
	}
	if got := byLabel["Secret"]; !got.Secure || got.Value != "" {
		t.Errorf("secure field must be marked and drop its value: %+v", got)
	}
	if got := byLabel["Disabled"]; got.Enabled {
		t.Errorf("disabled = %+v", got)
	}
	if got := byLabel["Agree"]; got.Selected == nil || !*got.Selected || got.Value != "" {
		t.Errorf("checkbox = %+v", got)
	}
	row := byLabel["row 0"]
	if row.Role != "AXRow" || !row.LabelAdopted || row.ParentIndex == 0 || row.Depth != 2 {
		t.Errorf("row = %+v", row)
	}
	if _, leaked := byLabel["Recent report.pdf"]; leaked {
		t.Error("menu subtree must not become elements")
	}
	want := []string{`AXStaticText = "count: 1"`, `AXStaticText = "= 5"`, `AXStaticText = "row 0"`}
	if !reflect.DeepEqual(snap.Texts, want) {
		t.Errorf("texts = %q, want %q", snap.Texts, want)
	}
	all, _ := json.Marshal(snap)
	for _, secret := range []string{"Quarterly secrets", "hunter2", "report.pdf"} {
		if strings.Contains(string(all), secret) {
			t.Errorf("snapshot carries %q: %s", secret, all)
		}
	}
	if scroller != "@s1:e7" {
		t.Errorf("scroller = %q: a scroll area beats the bar found first", scroller)
	}
	if targets["@s1:e5"].role != "AXCheckBox" || !targets["@s1:e5"].actions["Toggle"] {
		t.Errorf("targets = %+v", targets["@s1:e5"])
	}
	if snap.Partial || snap.Degraded != "" || snap.TotalElements != len(snap.Elements) {
		t.Errorf("snap = %+v", snap)
	}
}

func TestParseADSnapshot_PartialTree(t *testing.T) {
	raw := `{"complete":false,"tree":{"role":"window","children":[{"available_actions":["Click"],"name":"Ok","ref_id":"@s:e1","role":"button"}]}}`
	snap, _, _, err := parseADSnapshot(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Partial || snap.Degraded != "" {
		t.Fatalf("a usable partial tree must not end the run: %+v", snap)
	}
	state, _ := RenderState(Request{Goal: "g"}, snap, nil, RenderOptions{})
	if !strings.Contains(state, "partly read") {
		t.Fatalf("state = %s", state)
	}

	snap, _, _, err = parseADSnapshot(json.RawMessage(`{"complete":false,"tree":{"role":"window"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Degraded == "" {
		t.Fatal("an empty partial tree has nothing to act on")
	}
}

func TestADRole(t *testing.T) {
	for in, want := range map[string]string{"button": "AXButton", "textfield": "AXTextField", "menubaritem": "AXMenuBarItem", "text_area": "AXTextArea", "gauge": "AXGauge", "": "AXUnknown"} {
		if got := adRole(in); got != want {
			t.Errorf("adRole(%q) = %q, want %q", in, got, want)
		}
	}
}

func adDriverWithSnapshot(t *testing.T, argv *[]string, reply string) *AgentDesktopDriver {
	t.Helper()
	d := NewAgentDesktopDriver("agent-desktop", testDriverTimeout)
	d.commandContext = recordingCommand(t, envelope(adSnapshotFixture), argv)
	if _, err := d.Snapshot(context.Background(), Window{WindowID: 6758, App: "계산기"}, SnapshotOpts{MaxDepth: 40}); err != nil {
		t.Fatal(err)
	}
	d.commandContext = recordingCommand(t, reply, argv)
	return d
}

func TestAgentDesktop_SnapshotArgs(t *testing.T) {
	var argv []string
	adDriverWithSnapshot(t, &argv, envelope(`{}`))
	want := []string{"snapshot", "--session", agentDesktopSession, "--window-id", "w-6758", "--compact", "--max-depth", "40", "--timeout-ms", "13000"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
}

func TestAgentDesktop_Actions(t *testing.T) {
	ok := envelope(`{"disposition":{"delivery":"delivered_verified"}}`)
	w := Window{PID: 1, WindowID: 6758, App: "계산기"}
	ctx := context.Background()
	session := []string{"--session", agentDesktopSession}
	cases := []struct {
		name string
		run  func(*AgentDesktopDriver) (Effect, error)
		want []string
	}{
		{"click", func(d *AgentDesktopDriver) (Effect, error) { return d.Click(ctx, w, "@s1:e4") }, []string{"click", "--", "@s1:e4"}},
		{"type keeps a dash value positional", func(d *AgentDesktopDriver) (Effect, error) { return d.TypeText(ctx, w, "@s1:e1", "-v") }, []string{"type", "--", "@s1:e1", "-v"}},
		{"secure field has no text insertion", func(d *AgentDesktopDriver) (Effect, error) { return d.TypeText(ctx, w, "@s1:e2", "x") }, []string{"set-value", "--", "@s1:e2", "x"}},
		{"set field", func(d *AgentDesktopDriver) (Effect, error) { return d.SetValue(ctx, w, "@s1:e1", "x") }, []string{"set-value", "--", "@s1:e1", "x"}},
		{"check", func(d *AgentDesktopDriver) (Effect, error) { return d.SetValue(ctx, w, "@s1:e5", "true") }, []string{"check", "--", "@s1:e5"}},
		{"uncheck", func(d *AgentDesktopDriver) (Effect, error) { return d.SetValue(ctx, w, "@s1:e5", "false") }, []string{"uncheck", "--", "@s1:e5"}},
		{"press goes by app", func(d *AgentDesktopDriver) (Effect, error) { return d.PressKey(ctx, w, "return") }, []string{"press", "--app", "계산기", "--", "return"}},
		{"scroll uses the window's scroll area", func(d *AgentDesktopDriver) (Effect, error) { return d.Scroll(ctx, w, "up") }, []string{"scroll", "--direction", "up", "--", "@s1:e7"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var argv []string
			d := adDriverWithSnapshot(t, &argv, ok)
			eff, err := tc.run(d)
			if err != nil || eff != EffectConfirmed {
				t.Fatalf("eff=%v err=%v", eff, err)
			}
			want := append([]string{tc.want[0]}, session...)
			want = append(want, tc.want[1:]...)
			if !reflect.DeepEqual(argv, want) {
				t.Fatalf("argv = %v, want %v", argv, want)
			}
		})
	}
}

func TestAgentDesktop_SetValueByRole(t *testing.T) {
	ok := envelope(`{"disposition":{"delivery":"delivered_verified"}}`)
	for role, command := range map[string]string{"AXRadioButton": "click", "AXPopUpButton": "select", "AXComboBox": "select"} {
		var argv []string
		d := NewAgentDesktopDriver("agent-desktop", testDriverTimeout)
		d.targets["@s:e1"] = adTarget{role: role}
		d.commandContext = recordingCommand(t, ok, &argv)
		if _, err := d.SetValue(context.Background(), Window{}, "@s:e1", "Two"); err != nil {
			t.Fatal(err)
		}
		if argv[0] != command {
			t.Errorf("%s: argv = %v", role, argv)
		}
	}
}

func TestAgentDesktop_ScrollWithoutScrollAreaIsRefused(t *testing.T) {
	d := NewAgentDesktopDriver("agent-desktop", testDriverTimeout)
	eff, err := d.Scroll(context.Background(), Window{WindowID: 1}, "down")
	if err != nil || eff != EffectRefused {
		t.Fatalf("eff=%v err=%v", eff, err)
	}
}

func TestParseADEffect(t *testing.T) {
	for delivery, want := range map[string]Effect{
		"delivered_verified":   EffectConfirmed,
		"delivered_unverified": EffectUnverifiable,
		"not_delivered":        EffectConfirmed, // ok:true with an already-checked box
		"":                     EffectUnverifiable,
	} {
		raw, _ := json.Marshal(map[string]any{"disposition": map[string]string{"delivery": delivery}})
		if got, err := parseADEffect(raw); err != nil || got != want {
			t.Errorf("%q: got %v, %v; want %v", delivery, got, err, want)
		}
	}
}

// failingCommand prints stdout and exits 1, as the CLI does on an error.
func failingCommand(t *testing.T, stdout string) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "printf '%s' "+shellQuote(stdout)+"; exit 1")
	}
}

func adFailure(code string) string {
	return `{"ok":false,"version":"2.5","error":{"code":"` + code + `","message":"nope","disposition":{"delivery":"not_delivered","retry":"safe"}}}`
}

func TestAgentDesktop_Errors(t *testing.T) {
	ctx := context.Background()
	newDriver := func(stdout string) *AgentDesktopDriver {
		d := NewAgentDesktopDriver("agent-desktop", testDriverTimeout)
		d.commandContext = failingCommand(t, stdout)
		return d
	}
	if eff, err := newDriver(adFailure("POLICY_DENIED")).Click(ctx, Window{}, "@s:e1"); err != nil || eff != EffectRefused {
		t.Errorf("policy: eff=%v err=%v", eff, err)
	}
	if eff, err := newDriver(adFailure("ACTION_NOT_SUPPORTED")).Click(ctx, Window{}, "@s:e1"); err != nil || eff != EffectRefused {
		t.Errorf("unsupported: eff=%v err=%v", eff, err)
	}
	_, err := newDriver(adFailure("STALE_REF")).Click(ctx, Window{}, "@s:e1")
	if err == nil || !strings.Contains(err.Error(), "STALE_REF") || errors.Is(err, ErrDriverUnavailable) {
		t.Errorf("stale: %v", err)
	}
	if err := newDriver(adFailure("PERM_DENIED")).Ping(ctx); !errors.Is(err, ErrDriverUnavailable) {
		t.Errorf("permission: %v", err)
	}
	if _, err := newDriver("not json").Snapshot(ctx, Window{}, SnapshotOpts{}); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Errorf("garbage: %v", err)
	}
}

func TestAgentDesktop_Ping(t *testing.T) {
	var argv []string
	d := NewAgentDesktopDriver("agent-desktop", testDriverTimeout)
	d.commandContext = recordingCommand(t, envelope(`{"accessibility":{"state":"granted"}}`), &argv)
	if err := d.Ping(context.Background()); err != nil || argv[0] != "permissions" {
		t.Fatalf("err=%v argv=%v", err, argv)
	}
	d.commandContext = recordingCommand(t, envelope(`{"accessibility":{"state":"denied"}}`), &argv)
	if err := d.Ping(context.Background()); !errors.Is(err, ErrDriverUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestAgentDesktop_MissingBinaryIsUnavailable(t *testing.T) {
	t.Setenv(AgentDesktopPathEnv, "")
	d := NewLazyAgentDesktopDriver(filepath.Join(t.TempDir(), "no-such-agent-desktop"), testDriverTimeout)
	if err := d.Ping(context.Background()); !errors.Is(err, ErrDriverUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if h := d.SetupHint(); !strings.Contains(h, "agent-desktop") {
		t.Fatalf("hint = %q", h)
	}
}

func TestFindAgentDesktopPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in binary is a shell script, which LookPath does not treat as executable on Windows")
	}
	bin := filepath.Join(t.TempDir(), "agent-desktop")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv(AgentDesktopPathEnv, "")
	orig := agentDesktopFallbacks
	t.Cleanup(func() { agentDesktopFallbacks = orig })

	agentDesktopFallbacks = func() []string { return nil }
	if _, err := FindAgentDesktopPath(""); !errors.Is(err, ErrDriverUnavailable) {
		t.Fatalf("nothing installed: %v", err)
	}
	if got, err := FindAgentDesktopPath(bin); err != nil || got != bin {
		t.Fatalf("configured: %q %v", got, err)
	}
	t.Setenv(AgentDesktopPathEnv, bin)
	if got, err := FindAgentDesktopPath(""); err != nil || got != bin {
		t.Fatalf("env: %q %v", got, err)
	}
	t.Setenv(AgentDesktopPathEnv, "")
	agentDesktopFallbacks = func() []string { return []string{bin} }
	if got, err := FindAgentDesktopPath(""); err != nil || got != bin {
		t.Fatalf("fallback: %q %v", got, err)
	}
}

const adWindows = `[
 {"id":"w-1","pid":7,"app_name":"Helper","title":"hidden","accessible":false,"visible":false,"is_focused":false,"bounds":{"width":500,"height":500}},
 {"id":"w-2","pid":8,"app_name":"Strip","title":"bar","accessible":true,"visible":true,"is_focused":true,"bounds":{"width":2000,"height":30}},
 {"id":"w-3","pid":9,"app_name":"계산기","title":"계산기","accessible":true,"visible":true,"is_focused":false,"bounds":{"width":230,"height":400}},
 {"id":"w-4","pid":10,"app_name":"Notes","title":"one","accessible":true,"visible":true,"is_focused":false,"bounds":{"width":800,"height":600}},
 {"id":"w-5","pid":10,"app_name":"Notes","title":"two","accessible":true,"visible":true,"is_focused":true,"bounds":{"width":800,"height":600}}
]`

func TestAgentDesktop_ResolveWindow(t *testing.T) {
	ctx := context.Background()
	var tools []string
	d := NewAgentDesktopDriver("agent-desktop", testDriverTimeout)
	d.commandContext = scriptedCommand(t, map[string][]string{"list-windows": {envelope(adWindows)}}, &tools)
	if w, err := d.ResolveWindow(ctx, ""); err != nil || w.WindowID != 5 {
		t.Fatalf("frontmost: %+v %v", w, err)
	}
	if w, err := d.ResolveWindow(ctx, "notes"); err != nil || w.WindowID != 5 || w.PID != 10 || w.App != "Notes" {
		t.Fatalf("by name prefers focus: %+v %v", w, err)
	}
	if w, err := d.ResolveWindow(ctx, "계산기"); err != nil || w.WindowID != 3 {
		t.Fatalf("local name: %+v %v", w, err)
	}
}

func TestAgentDesktop_ResolveWindowWithoutFocusNeedsApp(t *testing.T) {
	two := `[{"id":"w-3","pid":9,"app_name":"A","accessible":true,"visible":true,"bounds":{"width":300,"height":300}},
	         {"id":"w-4","pid":10,"app_name":"B","accessible":true,"visible":true,"bounds":{"width":300,"height":300}}]`
	var tools []string
	d := NewAgentDesktopDriver("agent-desktop", testDriverTimeout)
	d.commandContext = scriptedCommand(t, map[string][]string{"list-windows": {envelope(two)}}, &tools)
	if _, err := d.ResolveWindow(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "pass app") {
		t.Fatalf("err = %v", err)
	}
}

func TestAgentDesktop_ResolveWindowLaunchesByBundleAlias(t *testing.T) {
	// English "Calculator" never matches the localized app_name, so the alias
	// goes through launch, which attaches to a running app.
	var argvs [][]string
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	replies := map[string][]string{
		"list-windows": {envelope(`[]`), envelope(`[]`), envelope(adWindows)},
		"launch":       {envelope(`{"pid":9}`)},
	}
	d := NewAgentDesktopDriver("agent-desktop", testDriverTimeout)
	d.commandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		argvs = append(argvs, args)
		q := replies[args[0]]
		out := q[0]
		if len(q) > 1 {
			replies[args[0]] = q[1:]
		}
		return exec.CommandContext(ctx, "sh", "-c", "printf '%s' "+shellQuote(out))
	}
	w, err := d.ResolveWindow(context.Background(), "Calculator")
	if err != nil || w.WindowID != 3 {
		t.Fatalf("%+v %v", w, err)
	}
	want := []string{"launch", "--session", agentDesktopSession, "--", "com.apple.calculator"}
	if !reflect.DeepEqual(argvs[1], want) {
		t.Fatalf("argv = %v", argvs[1])
	}
}

func TestParseADWindowID(t *testing.T) {
	if n, err := parseADWindowID("w-6758"); err != nil || n != 6758 {
		t.Fatalf("%d %v", n, err)
	}
	for _, bad := range []string{"6758", "w-x", ""} {
		if _, err := parseADWindowID(bad); err == nil {
			t.Errorf("%q must not parse", bad)
		}
	}
}
