package computeruse

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func TestBuildCuaArgs(t *testing.T) {
	args, err := buildCuaArgs("click", map[string]any{"pid": 7, "element_token": "s1:3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "click" || !strings.Contains(args[1], `"element_token":"s1:3"`) || !strings.Contains(args[1], `"pid":7`) {
		t.Fatalf("args = %v", args)
	}
}

func TestParseWindowState_Fixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "get_window_state.json"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := parseWindowState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Elements) == 0 || snap.TotalElements < len(snap.Elements) {
		t.Fatalf("elements=%d total=%d", len(snap.Elements), snap.TotalElements)
	}
	first := snap.Elements[0]
	if first.Index != 1 || first.Token == "" || first.Role == "" {
		t.Fatalf("first element = %+v", first)
	}
	for i, el := range snap.Elements {
		if el.Index != i+1 {
			t.Fatalf("element %d has Index %d; must be 1-based sequential", i, el.Index)
		}
	}
}

func TestParseWindowState_SecureFieldAndDegraded(t *testing.T) {
	raw := []byte(`{"structuredContent":{"snapshot_id":"s1","element_count":2,"degraded_reason":"ax_window_unresolved","elements":[
	  {"element_index":4,"element_token":"s1:4","role":"AXSecureTextField","label":"Password","value":"hunter2","enabled":true},
	  {"element_index":9,"element_token":"s1:9","role":"AXButton","label":"OK","enabled":false}]}}`)
	snap, err := parseWindowState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Degraded != "ax_window_unresolved" {
		t.Errorf("degraded = %q", snap.Degraded)
	}
	if !snap.Elements[0].Secure || snap.Elements[0].Value != "" {
		t.Errorf("secure field must drop value: %+v", snap.Elements[0])
	}
	if snap.Elements[1].Enabled || snap.Elements[1].Index != 2 || snap.Elements[1].Token != "s1:9" {
		t.Errorf("second = %+v", snap.Elements[1])
	}
}

func TestParseListWindows_Fixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "list_windows.json"))
	if err != nil {
		t.Fatal(err)
	}
	wins, err := parseListWindows(raw)
	if err != nil || len(wins) == 0 || wins[0].PID == 0 || wins[0].WindowID == 0 {
		t.Fatalf("wins=%+v err=%v", wins, err)
	}
}

func TestParseEffect(t *testing.T) {
	for raw, want := range map[string]Effect{
		`{"structuredContent":{"effect":"confirmed","route":"accessibility"}}`: EffectConfirmed,
		`{"effect":"refused"}`:     EffectRefused,
		`{"structuredContent":{}}`: EffectUnverifiable,
	} {
		got, err := parseEffect([]byte(raw))
		if err != nil || got != want {
			t.Errorf("%s → %v,%v want %v", raw, got, err, want)
		}
	}
}

// A refused tool call exits 0 and prints a bare {"code":…,"suggestion":…}
// envelope on stdout, so an empty-looking payload must not parse as an empty
// snapshot. Captured from `get_window_state` against a closed window_id.
func TestParseWindowState_ErrorEnvelopeIsNotAnEmptySnapshot(t *testing.T) {
	raw := []byte(`{"code":"window_id_not_found","pid":2415,"suggestion":"call list_windows for current window_ids; the window may have closed","window_id":999999}`)
	if _, err := parseWindowState(raw); err == nil {
		t.Fatal("want error for a code envelope, got nil")
	} else if !strings.Contains(err.Error(), "window_id_not_found") {
		t.Fatalf("err = %v", err)
	}
}

// A code envelope naming the daemon or permissions must reach the engine as
// ErrDriverUnavailable even though the process exited 0.
func TestParseEffect_DaemonCodeEnvelopeMapsToUnavailable(t *testing.T) {
	_, err := parseEffect([]byte(`{"code":"daemon_not_running","suggestion":"run cua-driver serve"}`))
	if err == nil || !errorsIs(err, ErrDriverUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

// A success payload that happens to carry a code-like field alongside real
// content must still parse as content.
func TestParseEffect_CodeAlongsideEffectStillParses(t *testing.T) {
	eff, err := parseEffect([]byte(`{"effect":"confirmed","route":"accessibility","code":"ok"}`))
	if err != nil || eff != EffectConfirmed {
		t.Fatalf("eff=%v err=%v", eff, err)
	}
}

// fakeCommand returns a commandContext that runs `echo <stdout>` (exit 0) or
// `sh -c 'echo <stderr> >&2; exit 1'`; both are POSIX-only helpers, so the
// tests below skip on Windows rather than adding to windows_test.sh.
func fakeCommand(t *testing.T, stdout string, fail bool) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	return func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if fail {
			return exec.CommandContext(ctx, "sh", "-c", "echo 'Cua Driver daemon is not running' >&2; exit 1")
		}
		return exec.CommandContext(ctx, "sh", "-c", "printf '%s' \""+strings.ReplaceAll(stdout, `"`, `\"`)+"\"")
	}
}

func TestCuaDriver_PingMapsDaemonDownToUnavailable(t *testing.T) {
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = fakeCommand(t, "", true)
	err := d.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "daemon") || !errorsIs(err, ErrDriverUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

// recordingCommand captures the argv the driver builds and replies with a
// fixed stdout, pinning the tool names and argument keys verified against
// `cua-driver describe` at the time this adapter was written.
func recordingCommand(t *testing.T, stdout string, got *[]string) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	return func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		*got = args
		return exec.CommandContext(ctx, "sh", "-c", "printf '%s' "+shellQuote(stdout))
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func TestCuaDriver_ActionArgKeysMatchDescribeSchema(t *testing.T) {
	w := Window{PID: 11, WindowID: 22}
	cases := []struct {
		name     string
		run      func(*CuaDriver) error
		wantTool string
		wantKeys []string
	}{
		{"click", func(d *CuaDriver) error {
			_, err := d.Click(context.Background(), w, "s1:3")
			return err
		}, "click", []string{`"pid":11`, `"element_token":"s1:3"`}},
		{"set_value", func(d *CuaDriver) error {
			_, err := d.SetValue(context.Background(), w, "s1:3", "abc")
			return err
		}, "set_value", []string{`"pid":11`, `"element_token":"s1:3"`, `"value":"abc"`}},
		{"type_text", func(d *CuaDriver) error {
			_, err := d.TypeText(context.Background(), w, "s1:3", "hello")
			return err
		}, "type_text", []string{`"pid":11`, `"element_token":"s1:3"`, `"text":"hello"`}},
		{"press_key", func(d *CuaDriver) error {
			_, err := d.PressKey(context.Background(), w, "return")
			return err
		}, "press_key", []string{`"pid":11`, `"window_id":22`, `"key":"return"`}},
		{"scroll", func(d *CuaDriver) error {
			_, err := d.Scroll(context.Background(), w, "down")
			return err
		}, "scroll", []string{`"pid":11`, `"window_id":22`, `"direction":"down"`}},
		{"snapshot", func(d *CuaDriver) error {
			_, err := d.Snapshot(context.Background(), w, SnapshotOpts{Query: "Save", MaxDepth: 4})
			return err
		}, "get_window_state", []string{`"pid":11`, `"window_id":22`, `"include_screenshot":false`, `"max_elements":`, `"query":"Save"`, `"max_depth":4`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var argv []string
			d := NewCuaDriver("cua-driver", time.Second)
			d.commandContext = recordingCommand(t, `{"effect":"confirmed","elements":[]}`, &argv)
			if err := tc.run(d); err != nil {
				t.Fatal(err)
			}
			if len(argv) != 2 || argv[0] != tc.wantTool {
				t.Fatalf("argv = %v, want tool %q", argv, tc.wantTool)
			}
			for _, key := range tc.wantKeys {
				if !strings.Contains(argv[1], key) {
					t.Errorf("argv[1] = %s, missing %s", argv[1], key)
				}
			}
		})
	}
}

// A dead daemon reports through an exit-0 {"code":…} envelope, so Ping must
// decode the payload rather than trust the exit status.
func TestCuaDriver_PingRejectsExitZeroErrorEnvelope(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t,
		`{"code":"daemon_not_running","suggestion":"start it with cua-driver serve"}`, &argv)
	err := d.Ping(context.Background())
	if err == nil {
		t.Fatal("want error for an exit-0 daemon envelope, got nil")
	}
	if !errorsIs(err, ErrDriverUnavailable) {
		t.Fatalf("err = %v, want ErrDriverUnavailable", err)
	}
}

func TestCuaDriver_PingAcceptsHealthyPayload(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t, `{"current_space_id":1,"windows":[]}`, &argv)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestRankWindows_OrdersByZIndexDescending(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantIDs  []int
		wantHasZ bool
	}{
		{
			name:     "shuffled input is sorted frontmost first",
			raw:      `{"windows":[{"window_id":1,"pid":1,"z_index":3},{"window_id":2,"pid":1,"z_index":9},{"window_id":3,"pid":1,"z_index":5}]}`,
			wantIDs:  []int{2, 3, 1},
			wantHasZ: true,
		},
		{
			name:     "null z sorts last, behind a negative z",
			raw:      `{"windows":[{"window_id":1,"pid":1,"z_index":null},{"window_id":2,"pid":1,"z_index":-4},{"window_id":3,"pid":1,"z_index":2}]}`,
			wantIDs:  []int{3, 2, 1},
			wantHasZ: true,
		},
		{
			name:     "missing z key behaves like null",
			raw:      `{"windows":[{"window_id":1,"pid":1},{"window_id":2,"pid":1,"z_index":0}]}`,
			wantIDs:  []int{2, 1},
			wantHasZ: true,
		},
		{
			name:     "all null keeps array order and reports no z",
			raw:      `{"windows":[{"window_id":7,"pid":1,"z_index":null},{"window_id":8,"pid":1,"z_index":null}]}`,
			wantIDs:  []int{7, 8},
			wantHasZ: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wins, hasZ, err := parseListWindowsRanked([]byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if hasZ != tc.wantHasZ {
				t.Errorf("hasZ = %v, want %v", hasZ, tc.wantHasZ)
			}
			got := make([]int, len(wins))
			for i, w := range wins {
				got[i] = w.WindowID
			}
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("ids = %v, want %v", got, tc.wantIDs)
			}
			for i := range got {
				if got[i] != tc.wantIDs[i] {
					t.Fatalf("ids = %v, want %v", got, tc.wantIDs)
				}
			}
		})
	}
}

// The driver's own overlay and menu-bar strips sit high in the stack but are
// not drivable app windows; they must never win the frontmost race.
func TestRankWindows_DropsOverlayAndMenuBarSurfaces(t *testing.T) {
	raw := []byte(`{"windows":[
		{"window_id":1,"pid":1,"app_name":"Cua Driver","z_index":100,"bounds":{"x":0,"y":0,"width":2560,"height":1440}},
		{"window_id":2,"pid":2,"app_name":"MenuBarApp","z_index":90,"bounds":{"x":0,"y":0,"width":1800,"height":39}},
		{"window_id":3,"pid":3,"app_name":"Thin","z_index":80,"bounds":{"x":0,"y":0,"width":40,"height":600}},
		{"window_id":4,"pid":4,"app_name":"Real App","z_index":5,"bounds":{"x":0,"y":34,"width":1512,"height":948}},
		{"window_id":5,"pid":5,"app_name":"Other Real","z_index":3,"bounds":{"x":0,"y":34,"width":800,"height":600}},
		{"window_id":6,"pid":6,"app_name":"No Bounds","z_index":1}]}`)
	wins, _, err := parseListWindowsRanked(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{4, 5, 6}
	if len(wins) != len(want) {
		t.Fatalf("windows = %+v, want ids %v", wins, want)
	}
	for i, id := range want {
		if wins[i].WindowID != id {
			t.Fatalf("windows = %+v, want ids %v", wins, want)
		}
	}
}

func TestCuaDriver_ResolveWindowSkipsOverlayForRealWindow(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t, `{"windows":[
		{"window_id":1,"pid":1,"app_name":"Cua Driver","z_index":100,"bounds":{"width":2560,"height":1440}},
		{"window_id":2,"pid":2,"app_name":"MenuBar","z_index":90,"bounds":{"width":1800,"height":39}},
		{"window_id":7,"pid":3,"app_name":"Real App","z_index":5,"bounds":{"width":1512,"height":948}}]}`, &argv)
	w, err := d.ResolveWindow(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if w.WindowID != 7 || w.App != "Real App" {
		t.Fatalf("window = %+v, want the real app window", w)
	}
}

func TestCuaDriver_ResolveWindowPicksHighestZ(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t,
		`{"windows":[{"window_id":1,"pid":5,"app_name":"A","z_index":2},{"window_id":9,"pid":5,"app_name":"A","z_index":8}]}`, &argv)
	w, err := d.ResolveWindow(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if w.WindowID != 9 {
		t.Fatalf("window = %+v, want the z_index 8 window", w)
	}
}

// With several candidates and no stacking order anywhere, array order must not
// be mistaken for a frontmost signal.
func TestCuaDriver_ResolveWindowRefusesToGuessWithoutZ(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t,
		`{"windows":[{"window_id":1,"pid":5,"z_index":null},{"window_id":2,"pid":5,"z_index":null}]}`, &argv)
	_, err := d.ResolveWindow(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "stacking order") {
		t.Fatalf("err = %v, want a stacking-order refusal", err)
	}
}

// A single window needs no stacking order: there is nothing to infer.
func TestCuaDriver_ResolveWindowAcceptsLoneWindowWithoutZ(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t, `{"windows":[{"window_id":4,"pid":5,"z_index":null}]}`, &argv)
	w, err := d.ResolveWindow(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if w.WindowID != 4 {
		t.Fatalf("window = %+v", w)
	}
}

func TestCuaDriver_ResolveAppWindowPicksHighestZFromLaunch(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t,
		`{"pid":5,"name":"Example","launch_state":"window_ready","windows":[{"window_id":1,"pid":5,"z_index":2},{"window_id":9,"pid":5,"z_index":8}]}`, &argv)
	w, err := d.ResolveWindow(context.Background(), "Example")
	if err != nil {
		t.Fatal(err)
	}
	if w.WindowID != 9 {
		t.Fatalf("window = %+v, want the z_index 8 window", w)
	}
	if w.App != "Example" {
		t.Errorf("app = %q", w.App)
	}
}

// scriptedCommand dispatches on the tool name so a test can script a whole
// resolve sequence. Each tool's replies are consumed in order; the last reply
// repeats once exhausted. tools records the call order.
func scriptedCommand(t *testing.T, replies map[string][]string, tools *[]string) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	return func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		tool := ""
		if len(args) > 0 {
			tool = args[0]
		}
		*tools = append(*tools, tool)
		queue, ok := replies[tool]
		if !ok || len(queue) == 0 {
			t.Errorf("unexpected call to %q", tool)
			return exec.CommandContext(ctx, "sh", "-c", "printf '%s' '{}'")
		}
		out := queue[0]
		if len(queue) > 1 {
			replies[tool] = queue[1:]
		}
		return exec.CommandContext(ctx, "sh", "-c", "printf '%s' "+shellQuote(out))
	}
}

func countTool(tools []string, name string) int {
	n := 0
	for _, t := range tools {
		if t == name {
			n++
		}
	}
	return n
}

// launch_app can answer before the window is mapped; the driver polls the pid
// instead of failing the race.
func TestCuaDriver_ResolveAppWindowPollsUntilWindowAppears(t *testing.T) {
	var tools []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = scriptedCommand(t, map[string][]string{
		// First list_windows is the already-running probe (no match), then the polls.
		"list_windows": {
			`{"windows":[]}`,
			`{"windows":[]}`,
			`{"windows":[{"window_id":12,"pid":5,"app_name":"Example","z_index":1}]}`,
		},
		"launch_app": {`{"pid":5,"name":"Example","launch_state":{"process_running":true,"requested":true,"window_ready":false},"windows":[]}`},
	}, &tools)
	w, err := d.ResolveWindow(context.Background(), "Example")
	if err != nil {
		t.Fatal(err)
	}
	if w.WindowID != 12 {
		t.Fatalf("window = %+v", w)
	}
	if got := countTool(tools, "launch_app"); got != 1 {
		t.Errorf("launch_app calls = %d, want 1", got)
	}
	if got := countTool(tools, "list_windows"); got != 3 {
		t.Errorf("list_windows calls = %d, want 3 (probe + 2 polls)", got)
	}
}

// An app that is already running is resolved from list_windows alone — no
// launch, so nothing can steal focus.
func TestCuaDriver_ResolveAppWindowPrefersRunningApp(t *testing.T) {
	var tools []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = scriptedCommand(t, map[string][]string{
		"list_windows": {`{"windows":[
			{"window_id":1,"pid":5,"app_name":"Example","z_index":2},
			{"window_id":9,"pid":5,"app_name":"example","z_index":8},
			{"window_id":3,"pid":7,"app_name":"Other","z_index":99}]}`},
	}, &tools)
	w, err := d.ResolveWindow(context.Background(), "Example")
	if err != nil {
		t.Fatal(err)
	}
	// Max-z among the case-insensitive name matches, not the frontmost overall.
	if w.WindowID != 9 {
		t.Fatalf("window = %+v, want window 9", w)
	}
	if countTool(tools, "launch_app") != 0 {
		t.Errorf("must not launch a running app; tools = %v", tools)
	}
}

// A name in the alias table launches by bundle id: launching Finder by name
// fails with APP_NOT_INSTALLED, and a running app reports a localized name.
func TestCuaDriver_ResolveAppWindowLaunchesAliasByBundleID(t *testing.T) {
	cases := []struct{ app, wantArg string }{
		{"Finder", `"bundle_id":"com.apple.finder"`},
		{"calculator", `"bundle_id":"com.apple.calculator"`},
		{"System Settings", `"bundle_id":"com.apple.systempreferences"`},
		{"com.example.Thing", `"bundle_id":"com.example.Thing"`},
		{"Some Unknown App", `"name":"Some Unknown App"`},
	}
	for _, tc := range cases {
		t.Run(tc.app, func(t *testing.T) {
			var tools []string
			var launchArgs string
			d := NewCuaDriver("cua-driver", time.Second)
			inner := scriptedCommand(t, map[string][]string{
				"list_windows": {`{"windows":[]}`},
				"launch_app":   {`{"pid":5,"name":"Localized","windows":[{"window_id":2,"pid":5,"z_index":1}]}`},
			}, &tools)
			d.commandContext = func(ctx context.Context, path string, args ...string) *exec.Cmd {
				if len(args) > 1 && args[0] == "launch_app" {
					launchArgs = args[1]
				}
				return inner(ctx, path, args...)
			}
			if _, err := d.ResolveWindow(context.Background(), tc.app); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(launchArgs, tc.wantArg) {
				t.Errorf("launch args = %s, want %s", launchArgs, tc.wantArg)
			}
		})
	}
}

// launch_state is an object; the diagnostic must not print an empty string.
func TestFormatLaunchState(t *testing.T) {
	got := formatLaunchState(map[string]any{"process_running": true, "requested": true, "window_ready": false})
	want := "process_running=true requested=true window_ready=false"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if s := formatLaunchState(nil); s != "" {
		t.Errorf("nil → %q, want empty", s)
	}
	if s := formatLaunchState("sent"); s != "sent" {
		t.Errorf("string → %q", s)
	}
}

func TestCuaDriver_NoWindowErrorReportsLaunchState(t *testing.T) {
	// Exhausts the full poll budget (cuaLaunchPollAttempts × interval), so it is
	// the slowest test in the package by design.
	var tools []string
	d := NewCuaDriver("cua-driver", 5*time.Second)
	d.commandContext = scriptedCommand(t, map[string][]string{
		"list_windows": {`{"windows":[]}`},
		"launch_app":   {`{"pid":5,"name":"Example","launch_state":{"process_running":true,"requested":true,"window_ready":false},"windows":[]}`},
	}, &tools)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := d.ResolveWindow(ctx, "Example")
	if err == nil {
		t.Fatal("want an error when no window ever appears")
	}
	if !strings.Contains(err.Error(), "window_ready=false") {
		t.Errorf("err = %v, want the decoded launch_state", err)
	}
}

// A cancelled context must abandon the poll rather than sleep out the budget.
func TestCuaDriver_ResolveAppWindowPollRespectsContext(t *testing.T) {
	var argv []string
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = recordingCommand(t, `{"pid":5,"name":"Example","windows":[]}`, &argv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := d.ResolveWindow(ctx, "Example"); err == nil {
		t.Fatal("want error from a cancelled context")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %s; poll ignored the cancelled context", elapsed)
	}
}

func TestLooksLikeBundleID(t *testing.T) {
	bundles := []string{"com.apple.calculator", "com.google.Chrome", "org.mozilla.firefox"}
	names := []string{"Node.js", "Calculator", "Google Chrome", "Foo v1.2", "", "Visual Studio Code"}
	for _, s := range bundles {
		if !looksLikeBundleID(s) {
			t.Errorf("looksLikeBundleID(%q) = false, want true", s)
		}
	}
	for _, s := range names {
		if looksLikeBundleID(s) {
			t.Errorf("looksLikeBundleID(%q) = true, want false", s)
		}
	}
}

func TestFindCuaDriverPath(t *testing.T) {
	newBinary := func(t *testing.T, name string) (dir, path string) {
		t.Helper()
		dir = t.TempDir()
		path = filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return dir, path
	}

	t.Run("configured path wins over env", func(t *testing.T) {
		_, configured := newBinary(t, "configured-driver")
		_, fromEnv := newBinary(t, "env-driver")
		t.Setenv(CuaDriverPathEnv, fromEnv)
		got, err := FindCuaDriverPath(configured)
		if err != nil {
			t.Fatal(err)
		}
		if got != configured {
			t.Fatalf("path = %q, want %q", got, configured)
		}
	})

	t.Run("invalid configured path fails loudly without falling through", func(t *testing.T) {
		_, fromEnv := newBinary(t, "env-driver")
		t.Setenv(CuaDriverPathEnv, fromEnv)
		missing := filepath.Join(t.TempDir(), "definitely-not-here")
		got, err := FindCuaDriverPath(missing)
		if err == nil {
			t.Fatalf("want error, got path %q", got)
		}
		if !errorsIs(err, ErrDriverUnavailable) {
			t.Errorf("err = %v, want ErrDriverUnavailable", err)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("err = %v, want it to name the configured path", err)
		}
		if got != "" {
			t.Errorf("path = %q, want empty", got)
		}
	})

	t.Run("env used when configured is empty", func(t *testing.T) {
		_, fromEnv := newBinary(t, "env-driver")
		t.Setenv(CuaDriverPathEnv, fromEnv)
		got, err := FindCuaDriverPath("   ")
		if err != nil {
			t.Fatal(err)
		}
		if got != fromEnv {
			t.Fatalf("path = %q, want %q", got, fromEnv)
		}
	})

	t.Run("PATH fallback finds the binary", func(t *testing.T) {
		dir, path := newBinary(t, "cua-driver")
		t.Setenv(CuaDriverPathEnv, "")
		t.Setenv("PATH", dir)
		got, err := FindCuaDriverPath("")
		if err != nil {
			t.Fatal(err)
		}
		if got != path {
			t.Fatalf("path = %q, want %q", got, path)
		}
	})

	t.Run("PATH miss names the env override", func(t *testing.T) {
		t.Setenv(CuaDriverPathEnv, "")
		t.Setenv("PATH", t.TempDir())
		_, err := FindCuaDriverPath("")
		if err == nil {
			t.Fatal("want error when cua-driver is not on PATH")
		}
		if !errorsIs(err, ErrDriverUnavailable) {
			t.Errorf("err = %v, want ErrDriverUnavailable", err)
		}
		if !strings.Contains(err.Error(), CuaDriverPathEnv) {
			t.Errorf("err = %v, want it to mention %s", err, CuaDriverPathEnv)
		}
	})
}

func TestCuaDriver_ClickParsesEffect(t *testing.T) {
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = fakeCommand(t, `{"structuredContent":{"effect":"confirmed"}}`, false)
	eff, err := d.Click(context.Background(), Window{PID: 1, WindowID: 2}, "s1:3")
	if err != nil || eff != EffectConfirmed {
		t.Fatalf("eff=%v err=%v", eff, err)
	}
}

// Captured from cua-driver 0.28.2 against the stock macOS Calculator. Every
// element carries depth, and every non-root element carries parent_index, so
// the snapshot can be projected as a tree.
func TestParseWindowState_CalculatorFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "get_window_state_calculator.json"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := parseWindowState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Elements) <= 100 {
		t.Fatalf("elements = %d; want a full Calculator tree (>100)", len(snap.Elements))
	}
	byIndex := make(map[int]Element, len(snap.Elements))
	for _, el := range snap.Elements {
		byIndex[el.Index] = el
	}
	var equals *Element
	for i := range snap.Elements {
		if snap.Elements[i].Label == "등호" {
			equals = &snap.Elements[i]
			break
		}
	}
	if equals == nil {
		t.Fatal(`no element labeled "등호"`)
	}
	if equals.Role != "AXButton" || !equals.Enabled {
		t.Fatalf("equals button = %+v", *equals)
	}
	root := snap.Elements[0]
	if root.Role != "AXWindow" || root.Depth != 0 || root.ParentIndex != 0 {
		t.Fatalf("root = %+v", root)
	}
	linked, deep := 0, 0
	for _, el := range snap.Elements {
		if el.ParentIndex != 0 {
			linked++
		}
		if el.Depth > 1 {
			deep++
		}
	}
	// 0.28.2 carries parent_index on every element but the two roots (the
	// window and the menu bar), and a menu tree several levels deep.
	if linked < len(snap.Elements)-2 || deep == 0 {
		t.Fatalf("linked=%d deep=%d of %d elements", linked, deep, len(snap.Elements))
	}
	for _, el := range snap.Elements {
		if el.ParentIndex == 0 {
			continue
		}
		parent, ok := byIndex[el.ParentIndex]
		if !ok {
			t.Fatalf("element %d parent %d not in snapshot", el.Index, el.ParentIndex)
		}
		if parent.Index >= el.Index {
			t.Fatalf("element %d parent %d is not earlier in the snapshot", el.Index, parent.Index)
		}
		// The walk omits non-actionable intermediate nodes, so a parent is
		// shallower than its child but not necessarily by exactly one.
		if parent.Depth >= el.Depth {
			t.Fatalf("element %d (depth %d) parent %d has depth %d", el.Index, el.Depth, parent.Index, parent.Depth)
		}
	}
}

// macOS puts a row's / cell's / control's text in a child AXStaticText and
// leaves the container itself unlabeled, so the snapshot would otherwise show
// an anonymous row the model cannot name.
func TestParseWindowState_AdoptsChildStaticTextLabel(t *testing.T) {
	long := strings.Repeat("가", 80)
	raw := []byte(`{"structuredContent":{"element_count":8,"elements":[
	  {"element_index":0,"depth":0,"element_token":"s1:0","role":"AXWindow","label":"","enabled":true},
	  {"element_index":1,"depth":1,"parent_index":0,"element_token":"s1:1","role":"AXStaticText","label":"Settings","enabled":true},
	  {"element_index":3,"depth":1,"parent_index":0,"element_token":"s1:3","role":"AXRow","label":"","enabled":true},
	  {"element_index":4,"depth":2,"parent_index":3,"element_token":"s1:4","role":"AXStaticText","label":"Accessibility","enabled":true},
	  {"element_index":5,"depth":1,"parent_index":0,"element_token":"s1:5","role":"AXButton","label":"","enabled":true},
	  {"element_index":6,"depth":2,"parent_index":5,"element_token":"s1:6","role":"AXStaticText","label":"","value":"Continue","enabled":true},
	  {"element_index":7,"depth":1,"parent_index":0,"element_token":"s1:7","role":"AXCell","label":"","enabled":true},
	  {"element_index":8,"depth":2,"parent_index":7,"element_token":"s1:8","role":"AXStaticText","label":"` + long + `","enabled":true}]}}`)
	snap, err := parseWindowState(raw)
	if err != nil {
		t.Fatal(err)
	}
	byToken := make(map[string]Element, len(snap.Elements))
	for _, el := range snap.Elements {
		byToken[el.Token] = el
	}
	row := byToken["s1:3"]
	if row.Label != "Accessibility" || !row.LabelAdopted {
		t.Errorf("row = %+v; want adopted label Accessibility", row)
	}
	// A static text with no label of its own still carries its value.
	button := byToken["s1:5"]
	if button.Label != "Continue" || !button.LabelAdopted {
		t.Errorf("button = %+v; want adopted label Continue", button)
	}
	cell := byToken["s1:7"]
	if got := len([]rune(cell.Label)); got != 60 {
		t.Errorf("cell label = %d runes; want capped at 60", got)
	}
	// A window is a container, not a control: it keeps its own (empty) label.
	win := byToken["s1:0"]
	if win.Label != "" || win.LabelAdopted {
		t.Errorf("window = %+v; want no adoption", win)
	}
	if byToken["s1:4"].LabelAdopted {
		t.Errorf("static text must not adopt: %+v", byToken["s1:4"])
	}
	// Parent links are resolved against our 1-based renumbering, not the
	// driver's sparse element_index.
	if row.ParentIndex != win.Index || win.ParentIndex != 0 || byToken["s1:4"].ParentIndex != row.Index {
		t.Errorf("parent links: win=%+v row=%+v child=%+v", win, row, byToken["s1:4"])
	}
	if row.Depth != 1 || byToken["s1:4"].Depth != 2 {
		t.Errorf("depths: row=%d child=%d", row.Depth, byToken["s1:4"].Depth)
	}
}
