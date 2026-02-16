package toolpolicy

import (
	"encoding/json"
	"fmt"
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

func TestSelector_Heuristic_MCPTopKWithLargeToolset(t *testing.T) {
	tools := testTools()
	for i := 0; i < 120; i++ {
		name := fmt.Sprintf("mcp.server_%03d.action_%03d", i, i)
		desc := "generic mcp tool"
		if i == 77 {
			name = "mcp.filesystem.read_file"
			desc = "read file from filesystem server"
		}
		tools = append(tools, tool.Tool{
			Name:        name,
			Description: desc,
			Parameters:  json.RawMessage(`{"type":"object"}`),
		})
	}
	cfg := SelectorConfig{Mode: "heuristic", MaxTools: 8}
	selector := NewSelector(Policy{Profile: "full"}, cfg)
	selected := selector.Select(tools, "anthropic", "claude", "filesystem read file")
	if len(selected) > 8 {
		t.Fatalf("expected <=8 selected tools, got %d", len(selected))
	}
	found := false
	for _, name := range selected {
		if name == "mcp.filesystem.read_file" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected relevant mcp tool in selected set, got %v", selected)
	}
}

func TestSelector_Heuristic_DoesNotFloodWithUnrelatedMCPTools(t *testing.T) {
	tools := []tool.Tool{
		{Name: "cron", Description: "manage cron jobs", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "session_status", Description: "session status", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	for i := 0; i < 60; i++ {
		tools = append(tools, tool.Tool{
			Name:        fmt.Sprintf("mcp.random.tool_%03d", i),
			Description: "mcp random utility",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		})
	}
	selector := NewSelector(Policy{Profile: "full"}, SelectorConfig{Mode: "heuristic", MaxTools: 6})
	selected := selector.Select(tools, "anthropic", "claude", "등록된 크론잡은?")
	mcpCount := 0
	for _, name := range selected {
		if len(name) >= 4 && name[:4] == "mcp." {
			mcpCount++
		}
	}
	if mcpCount > 1 {
		t.Fatalf("expected unrelated mcp tools to be mostly excluded, got %v", selected)
	}
}

func TestSelector_Heuristic_KoreanDirectoryQueryDoesNotPickSessionStatusOnly(t *testing.T) {
	selector := NewSelector(Policy{Profile: "full"}, SelectorConfig{Mode: "heuristic", MaxTools: 1})
	selected := selector.Select(testTools(), "gemini-native", "gemini-3-flash-preview", "지금 어느 디렉토리에 있지?")
	if len(selected) != 1 {
		t.Fatalf("expected exactly 1 selected tool, got %v", selected)
	}
	if selected[0] == "session_status" {
		t.Fatalf("expected directory intent tool, got %v", selected)
	}
}
