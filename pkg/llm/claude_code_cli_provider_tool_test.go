package llm

import (
	"fmt"
	"strings"
	"testing"
)

// The console shows a CLI tool card the moment the CLI calls the tool and
// settles it when the result comes back, so the parser reports both as the
// stream arrives rather than only in the final response.
func TestParseClaudeCodeCLIStreamReportsProviderToolsLive(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"reading"},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/tmp/a.txt"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"file contents"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"false"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":[{"type":"text","text":"exit 1"},{"type":"text","text":"boom"}]}]}}`,
		`{"type":"result","result":"done","is_error":false}`,
	}, "\n")
	var log []string
	resp, err := parseClaudeCodeCLIStream(strings.NewReader(stream), ChatOptions{
		OnDelta: func(s string) { log = append(log, "delta:"+s) },
		OnProviderTool: func(evt ProviderToolEvent) {
			if !evt.Finished {
				log = append(log, fmt.Sprintf("start:%s:%s:%s", evt.Call.ID, evt.Call.Name, evt.Call.Arguments))
				return
			}
			log = append(log, fmt.Sprintf("done:%s:%s:%q:%v", evt.Call.ID, evt.Call.Name, evt.Result, evt.IsError))
		},
	}, claudeCodeStreamHooks{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{
		"delta:reading",
		`start:t1:Read:{"file_path":"/tmp/a.txt"}`,
		`done:t1:Read:"file contents":false`,
		`start:t2:Bash:{"command":"false"}`,
		`done:t2:Bash:"exit 1\nboom":true`,
	}
	if strings.Join(log, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events =\n%s\nwant\n%s", strings.Join(log, "\n"), strings.Join(want, "\n"))
	}
	// The final response still lists every call for callers that read it
	// after the fact.
	if len(resp.ProviderExecutedTools) != 2 {
		t.Fatalf("ProviderExecutedTools = %+v", resp.ProviderExecutedTools)
	}
}

// A stream cut off mid-tool (timeout, cancel) has already reported the
// start, so the caller can keep the card; results for unknown IDs are
// dropped rather than inventing a call.
func TestParseClaudeCodeCLIStreamProviderToolEdgeCases(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"ghost","content":"?"}]}}`,
		`{"type":"user","message":{"content":"plain text replay"}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Edit","input":{}}]}}`,
	}, "\n")
	var events []ProviderToolEvent
	if _, err := parseClaudeCodeCLIStream(strings.NewReader(stream), ChatOptions{
		OnProviderTool: func(evt ProviderToolEvent) { events = append(events, evt) },
	}, claudeCodeStreamHooks{}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(events) != 1 || events[0].Finished || events[0].Call.ID != "t1" {
		t.Fatalf("events = %+v, want only the start of t1", events)
	}
}

// Without a callback the parser behaves exactly as before.
func TestParseClaudeCodeCLIStreamWithoutProviderToolCallback(t *testing.T) {
	stream := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}` + "\n" +
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"x"}]}}`
	resp, err := parseClaudeCodeCLIStream(strings.NewReader(stream), ChatOptions{}, claudeCodeStreamHooks{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(resp.ProviderExecutedTools) != 1 {
		t.Fatalf("ProviderExecutedTools = %+v", resp.ProviderExecutedTools)
	}
}
