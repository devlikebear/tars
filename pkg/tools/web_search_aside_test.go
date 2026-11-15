package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// asideStub writes a stand-in aside CLI that records its arguments and
// prints one marked result line, the way the real search script does.
func asideStub(t *testing.T) (bin, argsLog string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell stand-in for the aside CLI")
	}
	dir := t.TempDir()
	bin = filepath.Join(dir, "aside")
	argsLog = filepath.Join(dir, "args.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$2\" > '" + argsLog + "'\n" +
		`echo '__TARS_ASIDE_RESULTS__{"blocked":false,"results":[{"title":"TARS","url":"https://tars.example","description":"assistant"}]}'` + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsLog
}

func decodeSearch(t *testing.T, res Result) webSearchResponse {
	t.Helper()
	var payload webSearchResponse
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode %s: %v", res.Text(), err)
	}
	return payload
}

func TestWebSearchTool_AsideProvider(t *testing.T) {
	bin, argsLog := asideStub(t)
	tool := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "aside", AsidePath: bin, AsideEngine: "naver"})

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"고 컨텍스트","count":3}`))
	if err != nil || res.IsError {
		t.Fatalf("execute: %v %s", err, res.Text())
	}
	payload := decodeSearch(t, res)
	if payload.Provider != "aside:naver" || len(payload.Results) != 1 || payload.Results[0].URL != "https://tars.example" {
		t.Fatalf("got %+v", payload)
	}
	script, _ := os.ReadFile(argsLog)
	if !strings.Contains(string(script), "search.naver.com") {
		t.Fatalf("configured engine not used: %s", script)
	}

	res, _ = tool.Execute(context.Background(), json.RawMessage(`{"query":"tars","engine":"DuckDuckGo"}`))
	if res.IsError || decodeSearch(t, res).Provider != "aside:duckduckgo" {
		t.Fatalf("engine argument not used: %s", res.Text())
	}

	res, _ = tool.Execute(context.Background(), json.RawMessage(`{"query":"tars","engine":"bing"}`))
	if !res.IsError || !strings.Contains(res.Text(), "engine must be one of") {
		t.Fatalf("want engine error, got %s", res.Text())
	}
}

func TestWebSearchTool_AsideOnlyWhenConfigured(t *testing.T) {
	bin, _ := asideStub(t)
	tool := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "rustxng", AsidePath: bin})
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"tars","provider":"aside"}`))
	if !res.IsError || !strings.Contains(res.Text(), "configured web_search provider") {
		t.Fatalf("aside must not run unless configured, got %s", res.Text())
	}
}

func TestWebSearchTool_AsideBadConfiguredEngine(t *testing.T) {
	bin, _ := asideStub(t)
	tool := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "aside", AsidePath: bin, AsideEngine: "bing"})
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"tars"}`))
	if !res.IsError || !strings.Contains(res.Text(), "tools.web_search.aside.engine") {
		t.Fatalf("want config error, got %s", res.Text())
	}
}

func TestWebSearchTool_AsideMissingBinary(t *testing.T) {
	tool := NewWebSearchToolWithOptions(WebSearchOptions{Enabled: true, Provider: "aside", AsidePath: filepath.Join(t.TempDir(), "none")})
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"tars"}`))
	if !res.IsError || !strings.Contains(res.Text(), "aside CLI not found") {
		t.Fatalf("want not-found error, got %s", res.Text())
	}
}
