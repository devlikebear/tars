//go:build integration

package tarsserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
)

// A cart page whose total is wrong by 5 when bug is true: one item is 10,
// so three clicks must read "Total: 30".
func cartPage(bug bool) string {
	off := "0"
	if bug {
		off = "5"
	}
	return `<!doctype html><title>Cart</title><h1>Cart</h1><button id="add">Add item</button>` +
		`<p id="total">Total: 0</p><script>let n=0;add.onclick=()=>{n++;total.textContent="Total: "+(n*10-` + off + `)}</script>`
}

// The check script a plan stage is taught to write: its own browser
// session, closed on exit, and the four-line report on a mismatch.
const cartCheck = `AB="${AGENT_BROWSER:-agent-browser}"
S="tars-cart-check-$$"
trap '"$AB" --session "$S" close >/dev/null 2>&1' EXIT
"$AB" --session "$S" open "$CART_URL" >/dev/null || exit 2
for i in 1 2 3; do "$AB" --session "$S" click "#add" >/dev/null || exit 2; done
seen=$("$AB" --session "$S" get text "#total")
[ "$seen" = "Total: 30" ] && exit 0
echo "CHECK FAILED"
echo "screen: Cart page ($CART_URL)"
echo "expected: after adding 3 items the total reads \"Total: 30\""
echo "seen: the total reads \"$seen\""
exit 1
`

// Drives a real browser with the real agent-browser CLI through the real
// verification runner, against a page with a planted bug and then the same
// page fixed. No model anywhere.
// go test -tags integration ./internal/tarsserver -run TestLiveScriptedBrowserCheck -v
// (TARS_FOCUS_AGENT_BROWSER_PATH=<binary> when the CLI is not on PATH)
func TestLiveScriptedBrowserCheck(t *testing.T) {
	work := t.TempDir()
	cli, path := focusprobe.FindAgentBrowser(work)
	if cli == "" {
		t.Skip("agent-browser is not installed")
	}
	t.Logf("agent-browser: %s", path)
	script := filepath.Join(work, "cart.sh")
	if err := os.WriteFile(script, []byte(cartCheck), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(bug bool) (focuspipeline.Pipeline, focuspipeline.Action, taskVerificationResult) {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, cartPage(bug))
		}))
		defer srv.Close()
		command := "CART_URL='" + srv.URL + "/' AGENT_BROWSER='" + cli + "' sh '" + script + "'"
		store, id := verificationSession(t, command)
		started := time.Now()
		results, _, err := runTaskVerificationCommands(context.Background(), store, id, "", []string{command}, focusVerifyTimeout, focusprobe.FailureExcerpt)
		if err != nil || len(results) != 1 {
			t.Fatalf("results = %+v err = %v", results, err)
		}
		t.Logf("bug=%v: exit %d in %s\n%s", bug, results[0].ExitCode, time.Since(started).Round(time.Millisecond), results[0].Output)
		p, act := applyCheckResult(t, id, results[0])
		return p, act, results[0]
	}

	p, act, r := run(true)
	if r.ExitCode != 1 || p.Current != focuspipeline.StageBuild {
		t.Fatalf("the planted bug was not caught: exit %d, stage %s", r.ExitCode, p.Current)
	}
	var title string
	for _, c := range p.Cards {
		if c.Kind == focuspipeline.CardFailure {
			title = c.Title
		}
	}
	t.Logf("failure card: %s", title)
	if !strings.HasPrefix(title, "Check failed on Cart page (http://127.0.0.1:") ||
		!strings.HasSuffix(title, `: expected after adding 3 items the total reads "Total: 30", but saw the total reads "Total: 25"`) {
		t.Fatalf("failure card title = %q", title)
	}
	if act.Kind != focuspipeline.ActionSendTurn {
		t.Fatalf("no fix turn: %+v", act)
	}

	p, _, r = run(false)
	if r.ExitCode != 0 || p.Current == focuspipeline.StageBuild {
		t.Fatalf("the fixed page did not pass: exit %d, stage %s\n%s", r.ExitCode, p.Current, r.Output)
	}
	t.Logf("fixed page: exit 0, pipeline moved on to %s", p.Current)
}
