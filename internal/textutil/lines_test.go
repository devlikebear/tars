package textutil

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func collectLines(t *testing.T, r io.Reader) []string {
	t.Helper()
	var got []string
	if err := EachLine(r, func(line []byte) error {
		got = append(got, string(line))
		return nil
	}); err != nil {
		t.Fatalf("EachLine: %v", err)
	}
	return got
}

func TestEachLine_LongLineAndNoTrailingNewline(t *testing.T) {
	long := strings.Repeat("x", 300*1024)
	got := collectLines(t, strings.NewReader("a\r\n\n"+long+"\nlast"))
	want := []string{"a", "", long, "last"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d differs (len %d, want len %d)", i, len(got[i]), len(want[i]))
		}
	}
}

func TestEachLine_EmptyInputAndTrailingNewline(t *testing.T) {
	if got := collectLines(t, strings.NewReader("")); len(got) != 0 {
		t.Fatalf("empty input gave %q", got)
	}
	if got := collectLines(t, strings.NewReader("one\n")); len(got) != 1 || got[0] != "one" {
		t.Fatalf("got %q, want [one]", got)
	}
}

func TestEachLine_StopsOnCallbackAndReadErrors(t *testing.T) {
	stop := errors.New("stop")
	calls := 0
	err := EachLine(strings.NewReader("a\nb\nc\n"), func([]byte) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("err = %v after %d calls, want stop after 1", err, calls)
	}

	broken := errors.New("disk gone")
	var got []string
	err = EachLine(io.MultiReader(strings.NewReader("a\npart"), iotest.ErrReader(broken)), func(line []byte) error {
		got = append(got, string(line))
		return nil
	})
	if !errors.Is(err, broken) {
		t.Fatalf("err = %v, want the read error", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "part" {
		t.Fatalf("lines before the error = %q", got)
	}
}
