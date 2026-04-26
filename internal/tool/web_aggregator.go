package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// WebOptions wires the inputs both sub-tools need. SearchEnabled and
// FetchEnabled gate each action independently — when both are off the
// aggregator still registers but reports "disabled" per call. This
// matches the prior split where web_search and web_fetch could be
// flipped on/off separately via their cfg flags.
type WebOptions struct {
	Search        WebSearchOptions
	Fetch         WebFetchOptions
	SearchEnabled bool
	FetchEnabled  bool
}

// NewWebTool returns the unified web aggregator (action: search|fetch),
// replacing the separate web_search and web_fetch tools (ID-003 file/web
// aggregator decision). The tool description and parameters use the
// process pattern: every sub-action input lives at the top level so
// strict tool-call schemas can validate it. Sub-action handlers are
// closed over from the existing tool factories so the SSRF guard, brave
// vs perplexity routing, cache, and error envelopes stay intact.
func NewWebTool(opts WebOptions) Tool {
	searchOpts := opts.Search
	searchOpts.Enabled = opts.SearchEnabled
	fetchOpts := opts.Fetch
	fetchOpts.Enabled = opts.FetchEnabled

	searchTool := NewWebSearchToolWithOptions(searchOpts)
	fetchTool := NewWebFetchToolWithOptions(fetchOpts)

	return Tool{
		Name:        "web",
		Description: "Web operations. Actions: search (provider snippets) | fetch (URL → text with SSRF guard).",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "action":{"type":"string","enum":["search","fetch"]},
    "query":{"type":"string","description":"Search query (action=search)."},
    "count":{"type":"integer","minimum":1,"maximum":10,"default":5,"description":"Search result count (action=search)."},
    "provider":{"type":"string","enum":["brave","perplexity"],"description":"Search provider override (action=search)."},
    "url":{"type":"string","description":"URL to fetch (action=fetch)."},
    "max_chars":{"type":"integer","minimum":1,"maximum":50000,"default":12000,"description":"Max characters in fetched text (action=fetch)."}
  },
  "required":["action"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			payload, action, err := dispatchAction(params)
			if err != nil {
				return aggregatorError(err.Error()), nil
			}
			switch action {
			case "search":
				return searchTool.Execute(ctx, payload)
			case "fetch":
				return fetchTool.Execute(ctx, payload)
			default:
				return aggregatorError(fmt.Sprintf("action must be one of: search, fetch (got %q)", action)), nil
			}
		},
	}
}
