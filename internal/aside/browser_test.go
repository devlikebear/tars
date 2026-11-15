package aside

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLocalStartURL(t *testing.T) {
	for _, ok := range []string{
		"http://localhost:5173/", "http://127.0.0.1:43180/console", "http://[::1]:8080",
		"https://app.localhost", "http://myapp.test/login",
	} {
		if _, err := LocalStartURL(ok); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"https://mail.google.com", "http://localhost.evil.com", "ftp://localhost", "localhost:5173", "http://10.0.0.2", "",
	} {
		if _, err := LocalStartURL(bad); !errors.Is(err, ErrNotLocal) {
			t.Fatalf("%q: want ErrNotLocal, got %v", bad, err)
		}
	}
}

func TestCheckPrompt(t *testing.T) {
	p := CheckPrompt(Check{StartURL: "http://localhost:5173/x", Goal: "the cart shows 2 items"}, "http://localhost:5173")
	for _, want := range []string{"http://localhost:5173/x", "Stay on http://localhost:5173", "the cart shows 2 items", "VERDICT: PASS", "VERDICT: FAIL:"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q:\n%s", want, p)
		}
	}
}

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		out    string
		passed bool
		reason string
	}{
		{"looked around\nVERDICT: PASS\n", true, ""},
		{"**VERDICT: PASS**", true, ""},
		{"VERDICT: PASS\nlater\nVERDICT: FAIL: button missing", false, "button missing"},
		{"verdict: fail - empty page", false, "empty page"},
		{"VERDICT: FAIL", false, "the agent reported a failure"},
		{"VERDICT: maybe", false, "unreadable verdict: VERDICT: maybe"},
		{"I could not tell.", false, "the agent gave no verdict"},
	}
	for _, c := range cases {
		got := ParseVerdict(c.out)
		if got.Passed != c.passed || got.Reason != c.reason {
			t.Fatalf("%q: got %+v", c.out, got)
		}
	}
}

func TestVerify(t *testing.T) {
	bin := fakeBinary(t)
	f := &fakeRun{stdout: "\x1b[2mrepl(...)\x1b[0m\nAll good.\nVERDICT: PASS\x1b[0m\n"}
	c := Client{Path: bin, run: f.run}

	res, err := c.Verify(context.Background(), Check{StartURL: "http://localhost:3000", Goal: "home loads"})
	if err != nil || !res.Passed {
		t.Fatalf("got %+v, %v", res, err)
	}
	if f.gotArgs[0] != "exec" || !strings.Contains(f.gotArgs[len(f.gotArgs)-1], "Stay on http://localhost:3000") {
		t.Fatalf("args %v", f.gotArgs)
	}

	if _, err := c.Verify(context.Background(), Check{StartURL: "https://example.com", Goal: "x"}); !errors.Is(err, ErrNotLocal) {
		t.Fatalf("want ErrNotLocal, got %v", err)
	}

	f.err = errors.New("exit status 2")
	if _, err := c.Verify(context.Background(), Check{StartURL: "http://localhost:3000", Goal: "x"}); err == nil {
		t.Fatal("want exec error")
	}
}
