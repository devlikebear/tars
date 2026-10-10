package focusprobe

import (
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

const ghViewFixture = `{
  "number": 42,
  "url": "https://example.test/acme/repo/pull/42",
  "state": "OPEN",
  "mergeStateStatus": "BLOCKED",
  "headRefOid": "abc123",
  "headRefName": "tars/session-s1",
  "author": {"login": "erin"},
  "statusCheckRollup": [
    {"__typename": "CheckRun", "name": "test", "workflowName": "CI", "status": "COMPLETED", "conclusion": "FAILURE", "detailsUrl": "https://example.test/run/1", "startedAt": "2026-10-01T08:00:00Z"},
    {"__typename": "CheckRun", "name": "lint", "status": "COMPLETED", "conclusion": "SUCCESS", "startedAt": "2026-10-01T08:00:00Z"},
    {"__typename": "CheckRun", "name": "e2e", "status": "IN_PROGRESS", "conclusion": "", "startedAt": "2026-10-01T08:01:00Z"},
    {"__typename": "CheckRun", "name": "docs", "status": "COMPLETED", "conclusion": "SKIPPED"},
    {"__typename": "CheckRun", "name": "neutral", "status": "COMPLETED", "conclusion": "NEUTRAL"},
    {"__typename": "StatusContext", "context": "sonar", "state": "ERROR", "targetUrl": "https://example.test/sonar", "createdAt": "2026-10-01T08:02:00Z"},
    {"__typename": "StatusContext", "context": "codecov", "state": "PENDING"},
    {"__typename": "StatusContext", "context": "deploy", "state": "SUCCESS"}
  ],
  "reviews": [
    {"id": "PRR_1", "author": {"login": "alice"}, "authorAssociation": "MEMBER", "state": "CHANGES_REQUESTED", "body": "Rename the helper."},
    {"id": "PRR_2", "author": {"login": "bob"}, "authorAssociation": "COLLABORATOR", "state": "APPROVED", "body": "LGTM"},
    {"id": "PRR_3", "author": {"login": "carol"}, "authorAssociation": "OWNER", "state": "COMMENTED", "body": ""},
    {"id": "PRR_4", "author": {"login": "frank"}, "authorAssociation": "OWNER", "state": "CHANGES_REQUESTED", "body": ""}
  ],
  "comments": [
    {"id": "IC_1", "author": {"login": "dave"}, "authorAssociation": "CONTRIBUTOR", "body": "Add a test for the empty case.", "url": "https://example.test/c/1"},
    {"id": "IC_2", "author": {"login": "erin"}, "authorAssociation": "NONE", "body": "Pushed a fix.", "url": "https://example.test/c/2"},
    {"id": "IC_3", "author": {"login": "sonarqubecloud"}, "authorAssociation": "NONE", "body": "Quality Gate passed", "url": "https://example.test/c/3"},
    {"id": "IC_4", "author": {"login": "codecov[bot]"}, "authorAssociation": "NONE", "body": "Coverage 81%", "url": "https://example.test/c/4"},
    {"id": "IC_5", "author": {"login": "deployer", "is_bot": true}, "authorAssociation": "NONE", "body": "Preview ready", "url": "https://example.test/c/5"},
    {"id": "IC_6", "author": {"login": "linter", "type": "Bot"}, "authorAssociation": "NONE", "body": "2 warnings", "url": "https://example.test/c/6"}
  ]
}`

func TestParseFocusPRViewMergeCommit(t *testing.T) {
	got := parseFocusPRView([]byte(`{"number": 7, "state": "MERGED", "mergeCommit": {"oid": "m1"}}`))
	if got.Status != focuspipeline.ProbeFound || got.MergeOID != "m1" {
		t.Fatalf("probe = %+v", got)
	}
	if open := parseFocusPRView([]byte(`{"number": 7, "state": "OPEN", "mergeCommit": null}`)); open.Status != focuspipeline.ProbeFound || open.MergeOID != "" {
		t.Fatalf("open probe = %+v", open)
	}
	if !strings.Contains(focusGHFields, "mergeCommit") {
		t.Fatalf("gh is not asked for the merge commit: %s", focusGHFields)
	}
}

func TestParseFocusPRView(t *testing.T) {
	got := parseFocusPRView([]byte(ghViewFixture))
	if got.Status != focuspipeline.ProbeFound || got.Number != 42 || got.State != "OPEN" || got.MergeState != "BLOCKED" || got.URL == "" ||
		got.HeadOID != "abc123" || got.HeadRef != "tars/session-s1" {
		t.Fatalf("probe = %+v", got)
	}
	wantChecks := []focuspipeline.PRCheck{
		{Name: "test", State: focuspipeline.CheckFail, URL: "https://example.test/run/1", StartedAt: "2026-10-01T08:00:00Z"},
		{Name: "lint", State: focuspipeline.CheckPass, StartedAt: "2026-10-01T08:00:00Z"},
		{Name: "e2e", State: focuspipeline.CheckPending, StartedAt: "2026-10-01T08:01:00Z"},
		{Name: "docs", State: focuspipeline.CheckSkipped},
		{Name: "neutral", State: focuspipeline.CheckPass},
		{Name: "sonar", State: focuspipeline.CheckFail, URL: "https://example.test/sonar", StartedAt: "2026-10-01T08:02:00Z"},
		{Name: "codecov", State: focuspipeline.CheckPending},
		{Name: "deploy", State: focuspipeline.CheckPass},
	}
	if !reflect.DeepEqual(got.Checks, wantChecks) {
		t.Fatalf("checks = %+v", got.Checks)
	}
	// A changes-requested review counts even without a body; only owners,
	// members, collaborators and the PR's author are trusted.
	wantComments := []focuspipeline.PRComment{
		{ID: "PRR_1", Author: "alice", Body: "Rename the helper.", ChangesRequested: true, Trusted: true},
		{ID: "PRR_4", Author: "frank", ChangesRequested: true, Trusted: true},
		{ID: "IC_1", Author: "dave", Body: "Add a test for the empty case.", URL: "https://example.test/c/1"},
		{ID: "IC_2", Author: "erin", Body: "Pushed a fix.", URL: "https://example.test/c/2", Trusted: true},
		// gh gives a comment author only a login today: a GitHub App's login
		// has no "[bot]" (sonarqubecloud). The machine knows such logins;
		// the parser flags only what gh marks a bot (is_bot / type Bot).
		{ID: "IC_3", Author: "sonarqubecloud", Body: "Quality Gate passed", URL: "https://example.test/c/3"},
		{ID: "IC_4", Author: "codecov[bot]", Body: "Coverage 81%", URL: "https://example.test/c/4"},
		{ID: "IC_5", Author: "deployer", Body: "Preview ready", URL: "https://example.test/c/5", Bot: true},
		{ID: "IC_6", Author: "linter", Body: "2 warnings", URL: "https://example.test/c/6", Bot: true},
	}
	if !reflect.DeepEqual(got.Comments, wantComments) {
		t.Fatalf("comments = %+v", got.Comments)
	}
}

func TestParseFocusPRViewMalformed(t *testing.T) {
	for _, raw := range []string{"", "not json", `{"url":"x"}`, `[]`} {
		got := parseFocusPRView([]byte(raw))
		if got.Status != focuspipeline.ProbeUnavailable || got.Error == "" {
			t.Fatalf("%q → %+v", raw, got)
		}
	}
}

func TestClassifyFocusGHFailure(t *testing.T) {
	tests := []struct {
		stderr string
		want   string
	}{
		{"no pull requests found for branch \"tars/session-x\"", focuspipeline.ProbeNone},
		{"To get started with GitHub CLI, please run:  gh auth login", focuspipeline.ProbeUnavailable},
		{"fatal: not a git repository", focuspipeline.ProbeUnavailable},
	}
	for _, tt := range tests {
		got := classifyFocusGHFailure(tt.stderr)
		if got.Status != tt.want || (tt.want == focuspipeline.ProbeUnavailable && !strings.Contains(got.Error, strings.Fields(tt.stderr)[0])) {
			t.Fatalf("%q → %+v", tt.stderr, got)
		}
	}
}

func TestFocusGHProbeWithoutGH(t *testing.T) {
	got := runFocusGH(t.Context(), t.TempDir(), "tars-test-no-such-gh", 0)
	if got.Status != focuspipeline.ProbeUnavailable || !strings.Contains(got.Error, "not found") {
		t.Fatalf("probe = %+v", got)
	}
	if got := runFocusGH(t.Context(), "", "gh", 0); got.Status != focuspipeline.ProbeUnavailable {
		t.Fatalf("no folder: %+v", got)
	}
}

func TestFocusCheckStates(t *testing.T) {
	// J4: both rollup shapes, every state they report.
	tests := []struct {
		name  string
		entry string
		want  focuspipeline.PRCheck
	}{
		{"run success", `{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"SUCCESS","startedAt":"s"}`, focuspipeline.PRCheck{Name: "a", State: focuspipeline.CheckPass, StartedAt: "s"}},
		{"run failure", `{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"FAILURE"}`, focuspipeline.PRCheck{Name: "a", State: focuspipeline.CheckFail}},
		{"run neutral", `{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"NEUTRAL"}`, focuspipeline.PRCheck{Name: "a", State: focuspipeline.CheckPass}},
		{"run skipped", `{"__typename":"CheckRun","name":"a","status":"COMPLETED","conclusion":"SKIPPED"}`, focuspipeline.PRCheck{Name: "a", State: focuspipeline.CheckSkipped}},
		{"run in progress", `{"__typename":"CheckRun","name":"a","status":"IN_PROGRESS","conclusion":""}`, focuspipeline.PRCheck{Name: "a", State: focuspipeline.CheckPending}},
		{"run queued", `{"__typename":"CheckRun","name":"a","status":"QUEUED"}`, focuspipeline.PRCheck{Name: "a", State: focuspipeline.CheckPending}},
		{"status success", `{"__typename":"StatusContext","context":"c","state":"SUCCESS","targetUrl":"u","createdAt":"t"}`, focuspipeline.PRCheck{Name: "c", State: focuspipeline.CheckPass, URL: "u", StartedAt: "t"}},
		{"status failure", `{"__typename":"StatusContext","context":"c","state":"FAILURE","createdAt":"t"}`, focuspipeline.PRCheck{Name: "c", State: focuspipeline.CheckFail, StartedAt: "t"}},
		{"status error", `{"__typename":"StatusContext","context":"c","state":"ERROR"}`, focuspipeline.PRCheck{Name: "c", State: focuspipeline.CheckFail}},
		{"status pending", `{"__typename":"StatusContext","context":"c","state":"PENDING"}`, focuspipeline.PRCheck{Name: "c", State: focuspipeline.CheckPending}},
		{"status expected", `{"__typename":"StatusContext","context":"c","state":"EXPECTED"}`, focuspipeline.PRCheck{Name: "c", State: focuspipeline.CheckPending}},
		{"status neutral", `{"__typename":"StatusContext","context":"c","state":"NEUTRAL"}`, focuspipeline.PRCheck{Name: "c", State: focuspipeline.CheckPass}},
		{"status skipped", `{"__typename":"StatusContext","context":"c","state":"SKIPPED"}`, focuspipeline.PRCheck{Name: "c", State: focuspipeline.CheckSkipped}},
		{"status unknown waits", `{"__typename":"StatusContext","context":"c","state":"WHATEVER"}`, focuspipeline.PRCheck{Name: "c", State: focuspipeline.CheckPending}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFocusPRView([]byte(`{"number":1,"state":"OPEN","statusCheckRollup":[` + tt.entry + `]}`))
			if len(got.Checks) != 1 || got.Checks[0] != tt.want {
				t.Fatalf("checks = %+v", got.Checks)
			}
		})
	}
}

func TestFocusGHBinary(t *testing.T) {
	t.Setenv("TARS_FOCUS_GH_PATH", "")
	if got := focusGHBinary(); got != "gh" {
		t.Fatalf("default = %q", got)
	}
	t.Setenv("TARS_FOCUS_GH_PATH", " e2e/fake-gh.sh ")
	if got := focusGHBinary(); got != "e2e/fake-gh.sh" {
		t.Fatalf("override = %q", got)
	}
}
