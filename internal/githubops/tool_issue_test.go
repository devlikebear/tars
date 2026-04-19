package githubops

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

type fakeGHRunner struct {
	args   [][]string
	output []byte
	err    error
}

func (f *fakeGHRunner) run(_ context.Context, args []string) ([]byte, error) {
	f.args = append(f.args, append([]string(nil), args...))
	return f.output, f.err
}

func TestIssueSearch_ReturnsParsedIssues(t *testing.T) {
	runner := &fakeGHRunner{output: []byte(`[{"number":1,"title":"bug","url":"https://github.com/o/r/issues/1"}]`)}
	tl := newIssueSearchTool(runner.run)
	res, err := tl.Execute(context.Background(), json.RawMessage(`{"repo":"o/r","query":"is:bug","state":"open","limit":5}`))
	if err != nil || res.IsError {
		t.Fatalf("unexpected error: %v / %s", err, res.Text())
	}
	var out struct {
		Count  int                      `json:"count"`
		Issues []map[string]interface{} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("expected 1 issue, got %d", out.Count)
	}
	// verify args include --search and --limit
	args := strings.Join(runner.args[0], " ")
	for _, expected := range []string{"--repo o/r", "--state open", "--limit 5", "--search is:bug", "--json"} {
		if !strings.Contains(args, expected) {
			t.Fatalf("args missing %q: %s", expected, args)
		}
	}
}

func TestIssueSearch_RejectsBadRepo(t *testing.T) {
	tl := newIssueSearchTool((&fakeGHRunner{}).run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"not-a-slug"}`))
	if !res.IsError || !strings.Contains(res.Text(), "owner/repo") {
		t.Fatalf("expected repo-format error: %s", res.Text())
	}
}

func TestIssueSearch_HandlesMissingCLI(t *testing.T) {
	runner := &fakeGHRunner{err: exec.ErrNotFound}
	tl := newIssueSearchTool(runner.run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"o/r"}`))
	if !res.IsError {
		t.Fatalf("expected error")
	}
	if !strings.Contains(res.Text(), "required CLI not found") {
		t.Fatalf("unexpected message: %s", res.Text())
	}
}

func TestIssueCreate_SuccessReturnsNumberAndURL(t *testing.T) {
	runner := &fakeGHRunner{output: []byte("Creating issue in owner/repo\nhttps://github.com/owner/repo/issues/42\n")}
	tl := newIssueCreateTool(runner.run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"owner/repo","title":"t","body":"b","labels":["bug","p1"]}`))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text())
	}
	var out struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Number != 42 || !strings.Contains(out.URL, "/issues/42") {
		t.Fatalf("number/url parse failed: %+v", out)
	}
	args := strings.Join(runner.args[0], " ")
	if !strings.Contains(args, "--label bug") || !strings.Contains(args, "--label p1") {
		t.Fatalf("labels not passed: %s", args)
	}
}

func TestIssueCreate_RejectsMissingTitle(t *testing.T) {
	tl := newIssueCreateTool((&fakeGHRunner{}).run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"o/r"}`))
	if !res.IsError || !strings.Contains(res.Text(), "title is required") {
		t.Fatalf("unexpected: %s", res.Text())
	}
}

func TestIssueComment_Success(t *testing.T) {
	runner := &fakeGHRunner{output: []byte("https://github.com/o/r/issues/1#issuecomment-99\n")}
	tl := newIssueCommentTool(runner.run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"o/r","issue_number":1,"body":"hi"}`))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text())
	}
	var out struct {
		OK  bool   `json:"ok"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.OK || !strings.Contains(out.URL, "issuecomment") {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestIssueComment_RejectsInvalidNumber(t *testing.T) {
	tl := newIssueCommentTool((&fakeGHRunner{}).run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"o/r","issue_number":0,"body":"hi"}`))
	if !res.IsError || !strings.Contains(res.Text(), "issue_number") {
		t.Fatalf("unexpected: %s", res.Text())
	}
}

func TestPRCreateDraft_BuildsCorrectArgs(t *testing.T) {
	runner := &fakeGHRunner{output: []byte("https://github.com/o/r/pull/7\n")}
	tl := newPRCreateDraftTool(runner.run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"o/r","head":"fix/bug","title":"t","body":"b"}`))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text())
	}
	args := strings.Join(runner.args[0], " ")
	for _, expected := range []string{"pr create", "--draft", "--head fix/bug", "--base main", "--title t"} {
		if !strings.Contains(args, expected) {
			t.Fatalf("args missing %q: %s", expected, args)
		}
	}
	var out struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Number != 7 {
		t.Fatalf("number parse failed: %d", out.Number)
	}
}

func TestPRCreateDraft_RejectsBadBranch(t *testing.T) {
	tl := newPRCreateDraftTool((&fakeGHRunner{}).run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"o/r","head":"has space","title":"t"}`))
	if !res.IsError {
		t.Fatalf("expected error")
	}
}

func TestExtractIssueNumber(t *testing.T) {
	cases := map[string]int{
		"https://github.com/o/r/issues/123":    123,
		"https://github.com/o/r/pull/7":        7,
		"https://github.com/o/r/issues/abc":    0,
		"":                                     0,
		"https://github.com/o/r/issues/":       0,
	}
	for input, want := range cases {
		if got := extractIssueNumber(input); got != want {
			t.Fatalf("extractIssueNumber(%q)=%d, want %d", input, got, want)
		}
	}
}

func TestIssueCreate_WrapsCommandFailure(t *testing.T) {
	runner := &fakeGHRunner{err: errors.New("auth expired"), output: []byte("error: not authenticated")}
	tl := newIssueCreateTool(runner.run)
	res, _ := tl.Execute(context.Background(), json.RawMessage(`{"repo":"o/r","title":"t"}`))
	if !res.IsError {
		t.Fatalf("expected error")
	}
	if !strings.Contains(res.Text(), "command failed") || !strings.Contains(res.Text(), "auth expired") {
		t.Fatalf("unexpected: %s", res.Text())
	}
}
