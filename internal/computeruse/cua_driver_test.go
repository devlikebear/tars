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

func TestCuaDriver_ClickParsesEffect(t *testing.T) {
	d := NewCuaDriver("cua-driver", time.Second)
	d.commandContext = fakeCommand(t, `{"structuredContent":{"effect":"confirmed"}}`, false)
	eff, err := d.Click(context.Background(), Window{PID: 1, WindowID: 2}, "s1:3")
	if err != nil || eff != EffectConfirmed {
		t.Fatalf("eff=%v err=%v", eff, err)
	}
}
