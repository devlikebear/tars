package consolewatch

import (
	"os/exec"
	"strconv"
	"testing"
)

// run executes Script under node against a stub page. The page's root gets
// a child once mountAfter reloads have happened (never when negative); the
// server answers once downFor checks have passed.
// Timers fire in order, at most ticks of them. It reports the reloads, the
// stored attempt count and the wait before each check.
func run(t *testing.T, path string, mountAfter, downFor int, stored string, ticks int) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	stub := `
var reloads = 0, timers = [], waits = [], mountAfter = ` + strconv.Itoa(mountAfter) + `;
var store = {"tars-desktop-reloads": ` + strconv.Quote(stored) + `};
var listeners = [];
var window = {addEventListener: function (e, f) { listeners.push(f); }};
// A reload is a new document: the shell injects Script into it again.
var location = {pathname: ` + strconv.Quote(path) + `, reload: function () { reloads++; boot(); }};
var document = {visibilityState: "visible", addEventListener: function (e, f) { listeners.push(f); }, getElementById: function () {
  return {childElementCount: (mountAfter >= 0 && reloads >= mountAfter) ? 1 : 0};
}};
var sessionStorage = {getItem: function (k) { return store[k] || null; }, setItem: function (k, v) { store[k] = v; }};
var setTimeout = function (f, ms) { timers.push({f: f, ms: ms}); };
var fetches = 0, downFor = ` + strconv.Itoa(downFor) + `;
var fetch = function () {
  var up = fetches++ >= downFor;
  return {then: function (ok, fail) { if (up) ok({ok: true}); else fail(new Error("down")); }};
};
function boot() { window = {addEventListener: window.addEventListener};
` + Script + `
}
boot();
for (var i = 0; i < ` + strconv.Itoa(ticks) + ` && timers.length; i++) { var t = timers.shift(); waits.push(t.ms); t.f(); }
process.stdout.write(reloads + " " + store["tars-desktop-reloads"] + " " + waits.join(","));`
	out, err := exec.Command(node, "-e", stub).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	return string(out)
}

func TestScript(t *testing.T) {
	cases := []struct {
		name, path string
		mountAfter int
		downFor    int
		stored     string
		ticks      int
		want       string // reloads, stored count, waits before each check
	}{
		{"started console is left alone", "/console/chat/a", 0, 0, "", 3, "0 0 2000"},
		{"blank console reloads once and stops when it starts", "/console", 1, 0, "", 3, "1 0 2000,4000"},
		{"blank console waits on the page while the server is down", "/console", 1, 3, "", 5, "1 0 2000,4000,8000,16000,30000"},
		{"a console that never starts backs off", "/console", -1, 0, "", 6, "6 6 2000,4000,8000,16000,30000,30000"},
		{"a reloaded page waits by the stored count", "/console", -1, 0, "3", 1, "1 4 16000"},
		{"pages outside the console are not watched", "/", -1, 0, "", 3, "0  "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := run(t, c.path, c.mountAfter, c.downFor, c.stored, c.ticks); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// A window brought back is checked without waiting for the timer.
func TestScriptChecksWhenShown(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	stub := `
var reloads = 0, listeners = [];
var window = {addEventListener: function (e, f) { listeners.push(f); }};
var location = {pathname: "/console", href: "http://127.0.0.1/console", reload: function () { reloads++; }};
var document = {visibilityState: "visible", addEventListener: function (e, f) { listeners.push(f); },
  getElementById: function () { return {childElementCount: 0}; }};
var sessionStorage = {getItem: function () { return null; }, setItem: function () {}};
var setTimeout = function () {};
var fetch = function () { return {then: function (ok) { ok({ok: true}); }}; };
` + Script + `
listeners.forEach(function (f) { f(); });
process.stdout.write(listeners.length + " " + reloads);`
	out, err := exec.Command(node, "-e", stub).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if string(out) != "2 2" {
		t.Errorf("got %q, want 2 listeners that each reload a blank console", out)
	}
}
