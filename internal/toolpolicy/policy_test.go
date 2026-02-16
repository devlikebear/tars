package toolpolicy

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/devlikebear/tarsncase/internal/tool"
)

func testTools() []tool.Tool {
	mk := func(name string) tool.Tool {
		return tool.Tool{Name: name, Description: name, Parameters: json.RawMessage(`{"type":"object"}`)}
	}
	return []tool.Tool{
		mk("read"),
		mk("read_file"),
		mk("write"),
		mk("edit"),
		mk("glob"),
		mk("exec"),
		mk("process"),
		mk("session_status"),
		mk("memory_search"),
		mk("memory_get"),
		mk("web_search"),
		mk("web_fetch"),
		mk("cron"),
		mk("heartbeat"),
		mk("mcp.filesystem.read_file"),
	}
}

func toolNames(tools []tool.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

func TestFilterTools_ProfileMinimal(t *testing.T) {
	policy := Policy{Profile: "minimal"}
	got := toolNames(FilterTools(testTools(), policy, "anthropic", "claude"))
	want := []string{"session_status"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected tools for minimal profile\nwant=%v\ngot=%v", want, got)
	}
}

func TestFilterTools_ProfileCodingWithDeny(t *testing.T) {
	policy := Policy{Profile: "coding", Deny: []string{"group:runtime"}}
	got := toolNames(FilterTools(testTools(), policy, "anthropic", "claude"))
	for _, disallowed := range []string{"exec", "process"} {
		for _, name := range got {
			if name == disallowed {
				t.Fatalf("expected %s to be denied, got=%v", disallowed, got)
			}
		}
	}
	if len(got) == 0 {
		t.Fatalf("expected non-empty coding set")
	}
}

func TestFilterTools_ByProviderOverride(t *testing.T) {
	policy := Policy{
		Profile: "coding",
		ByProvider: map[string]ProviderPolicy{
			"anthropic": {
				Profile: "minimal",
				Allow:   []string{"memory_search"},
			},
		},
	}
	got := toolNames(FilterTools(testTools(), policy, "anthropic", "claude-3-5"))
	want := []string{"session_status", "memory_search"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected provider override result\nwant=%v\ngot=%v", want, got)
	}
}

func TestSelector_HeuristicLimitsToolsAndKeepsMCPMatch(t *testing.T) {
	cfg := SelectorConfig{Mode: "heuristic", MaxTools: 4}
	selector := NewSelector(Policy{Profile: "full"}, cfg)
	selected := selector.Select(testTools(), "anthropic", "claude", "filesystem read file")
	if len(selected) > 4 {
		t.Fatalf("expected <=4 tools, got %d (%v)", len(selected), selected)
	}
	foundMCP := false
	for _, name := range selected {
		if name == "mcp.filesystem.read_file" {
			foundMCP = true
			break
		}
	}
	if !foundMCP {
		t.Fatalf("expected matching mcp tool in selected set, got %v", selected)
	}
}
