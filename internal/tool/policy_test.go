package tool

import (
	"reflect"
	"testing"
)

func TestPolicyResolve(t *testing.T) {
	all := []string{"read_file", "write_file", "web_fetch", "exec", "memory", "session"}
	tests := []struct {
		name    string
		policy  Policy
		want    []string
		blocked map[string]BlockedToolError
	}{
		{
			name:   "no constraints keeps all tools",
			policy: Policy{},
			want:   []string{"exec", "memory", "read_file", "session", "web_fetch", "write_file"},
		},
		{
			name: "allow groups restricts to grouped tools only",
			policy: Policy{
				AllowGroups:    []string{"files", "web"},
				UseAllowGroups: true,
			},
			want: []string{"read_file", "web_fetch", "write_file"},
			blocked: map[string]BlockedToolError{
				"exec":    {Tool: "exec", Rule: "group_allow", Group: "shell", Source: "session"},
				"memory":  {Tool: "memory", Rule: "group_allow", Group: "memory", Source: "session"},
				"session": {Tool: "session", Rule: "group_allow", Source: "session"},
			},
		},
		{
			name: "allow tools intersects after group filter",
			policy: Policy{
				AllowGroups:    []string{"files"},
				UseAllowGroups: true,
				AllowTools:     []string{"read_file"},
				UseAllowTools:  true,
			},
			want: []string{"read_file"},
			blocked: map[string]BlockedToolError{
				"exec":       {Tool: "exec", Rule: "group_allow", Group: "shell", Source: "session"},
				"memory":     {Tool: "memory", Rule: "group_allow", Group: "memory", Source: "session"},
				"session":    {Tool: "session", Rule: "group_allow", Source: "session"},
				"web_fetch":  {Tool: "web_fetch", Rule: "group_allow", Group: "web", Source: "session"},
				"write_file": {Tool: "write_file", Rule: "tool_allow", Group: "files", Source: "session"},
			},
		},
		{
			name: "group deny wins over allow group",
			policy: Policy{
				AllowGroups:    []string{"shell"},
				UseAllowGroups: true,
				DenyGroups:     []string{"exec"},
			},
			want: []string{},
			blocked: map[string]BlockedToolError{
				"exec":       {Tool: "exec", Rule: "group_deny", Group: "shell", Source: "session"},
				"memory":     {Tool: "memory", Rule: "group_allow", Group: "memory", Source: "session"},
				"read_file":  {Tool: "read_file", Rule: "group_allow", Group: "files", Source: "session"},
				"session":    {Tool: "session", Rule: "group_allow", Source: "session"},
				"web_fetch":  {Tool: "web_fetch", Rule: "group_allow", Group: "web", Source: "session"},
				"write_file": {Tool: "write_file", Rule: "group_allow", Group: "files", Source: "session"},
			},
		},
		{
			name: "tool deny wins over allow tool",
			policy: Policy{
				AllowTools:    []string{"read_file", "write_file"},
				UseAllowTools: true,
				DenyTools:     []string{"write_file"},
			},
			want: []string{"read_file"},
			blocked: map[string]BlockedToolError{
				"exec":       {Tool: "exec", Rule: "tool_allow", Group: "shell", Source: "session"},
				"memory":     {Tool: "memory", Rule: "tool_allow", Group: "memory", Source: "session"},
				"session":    {Tool: "session", Rule: "tool_allow", Source: "session"},
				"web_fetch":  {Tool: "web_fetch", Rule: "tool_allow", Group: "web", Source: "session"},
				"write_file": {Tool: "write_file", Rule: "tool_deny", Group: "files", Source: "session"},
			},
		},
		{
			name: "explicit empty allowlist blocks all tools",
			policy: Policy{
				UseAllowTools: true,
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.policy.Resolve(all, "session")
			if !reflect.DeepEqual(got.Allowed, tt.want) {
				t.Fatalf("allowed mismatch: got=%+v want=%+v", got.Allowed, tt.want)
			}
			for name, wantBlocked := range tt.blocked {
				gotBlocked, ok := got.Blocked[name]
				if !ok {
					t.Fatalf("expected blocked entry for %s", name)
				}
				if !reflect.DeepEqual(gotBlocked, wantBlocked) {
					t.Fatalf("blocked mismatch for %s: got=%+v want=%+v", name, gotBlocked, wantBlocked)
				}
			}
		})
	}
}

func TestNormalizeToolGroupName(t *testing.T) {
	tests := map[string]string{
		"files":    "files",
		"file":     "files",
		"exec":     "shell",
		"terminal": "shell",
		"shell":    "shell",
		"unknown":  "",
	}
	for input, want := range tests {
		if got := NormalizeToolGroupName(input); got != want {
			t.Fatalf("NormalizeToolGroupName(%q) = %q, want %q", input, got, want)
		}
	}
}
