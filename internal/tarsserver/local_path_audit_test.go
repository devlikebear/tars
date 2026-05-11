package tarsserver

import (
	"slices"
	"strings"
	"testing"
)

// These tests pin down the path-injection trust boundary for console/admin
// local-path APIs flagged by CodeQL go/path-injection alerts #34–#56 (#805).
// Each handler that touches a local path is gated by one or more of the
// validators exercised below; this file is the documented evidence those
// alerts are dismissed against.
//
// Routing-level guard: every endpoint listed here is matched by either
// `/v1/admin/*` or one of the explicit admin paths in apiAdminPaths(), so a
// browser-user role cannot reach them — see TestAPIAdminPaths_CoverLocalPathHandlers.

func TestAPIAdminPaths_CoverLocalPathHandlers(t *testing.T) {
	paths := apiAdminPaths()
	// Every local-path-touching handler must sit behind an admin path matcher
	// so a browser-user role cannot reach it.
	wantPrefixed := []string{
		"/v1/admin/*",
		"/v1/terminal/*",
		"/v1/agentruntime/restart",
		"/v1/agentruntime/reload",
		"/v1/runtime/extensions/reload",
	}
	for _, want := range wantPrefixed {
		if !slices.Contains(paths, want) {
			t.Fatalf("apiAdminPaths() missing %q — found %v", want, paths)
		}
	}
}

func TestValidateWorkspaceDirectoryName_RejectsTraversal(t *testing.T) {
	cases := []string{
		"",
		"   ",
		".",
		"..",
		"foo/bar",
		`foo\bar`,
		"a/../b",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if err := validateWorkspaceDirectoryName(raw); err == nil {
				t.Fatalf("expected rejection for %q", raw)
			}
		})
	}
}

func TestValidateWorkspaceDirectoryName_AcceptsSafeNames(t *testing.T) {
	cases := []string{"notes", "release-v2", "tasks_2026", "a.b"}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if err := validateWorkspaceDirectoryName(raw); err != nil {
				t.Fatalf("expected %q to be accepted, got %v", raw, err)
			}
		})
	}
}

func TestIsValidAgentRuntimeAgentName_RejectsDotOnlyAndSeparators(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"single dot", "."},
		{"double dot", ".."},
		{"all dots", "...."},
		{"slash", "a/b"},
		{"backslash", `a\b`},
		{"space", "agent name"},
		{"shell metachar", "agent;rm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if isValidAgentRuntimeAgentName(tc.input) {
				t.Fatalf("expected %q to be rejected", tc.input)
			}
		})
	}
}

func TestIsValidAgentRuntimeAgentName_AcceptsSafeNames(t *testing.T) {
	cases := []string{"agent", "agent-1", "agent_v2", "release.candidate"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if !isValidAgentRuntimeAgentName(name) {
				t.Fatalf("expected %q to be accepted", name)
			}
		})
	}
}

func TestCleanSkillCreatorFilePath_RejectsTraversal(t *testing.T) {
	cases := []string{
		"",
		"/etc/passwd",
		"../escape.sh",
		"..",
		"a/../../b",
		"../../../../etc/passwd",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if _, err := cleanSkillCreatorFilePath(raw); err == nil {
				t.Fatalf("expected rejection for %q", raw)
			}
		})
	}
}

func TestCleanSkillCreatorFilePath_NormalizesAcceptedPaths(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"SKILL.md", "SKILL.md"},
		{"bin/cli.sh", "bin/cli.sh"},
		{"./bin/cli.sh", "bin/cli.sh"},
		// filepath.Clean collapses interior `..` only when they don't escape
		// the root. `a/../b` resolves safely to `b`.
		{"a/../b", "b"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := cleanSkillCreatorFilePath(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestValidateSkillCreatorName_EnforcesKebabCase(t *testing.T) {
	if err := validateSkillCreatorName("good-name-1"); err != nil {
		t.Fatalf("expected good-name-1 to pass: %v", err)
	}
	rejected := []string{"", "Bad_Name", "name with space", "../escape", "."}
	for _, raw := range rejected {
		t.Run(raw, func(t *testing.T) {
			if err := validateSkillCreatorName(raw); err == nil {
				t.Fatalf("expected rejection for %q", raw)
			}
		})
	}
}

func TestFilesystemBrowseHandlerRequiresAbsolutePath(t *testing.T) {
	// The GET handler rejects non-absolute paths up front; verify the helper
	// surface used by the handler still enforces this contract.
	cases := []struct {
		raw         string
		wantReject  bool
		errContains string
	}{
		{"", false, ""},      // empty falls back to home dir
		{"./relative", true, "absolute"},
		{"sub/path", true, "absolute"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			err := assertFilesystemBrowsePathShape(tc.raw)
			if tc.wantReject {
				if err == nil {
					t.Fatalf("expected rejection for %q", tc.raw)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error to mention %q, got %v", tc.errContains, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.raw, err)
			}
		})
	}
}
