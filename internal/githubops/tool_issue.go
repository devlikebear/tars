package githubops

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/tool"
)

const issueSearchLimitMax = 100

type issueSearchInput struct {
	Repo  string `json:"repo"`
	Query string `json:"query,omitempty"`
	State string `json:"state,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

func newIssueSearchTool(run ghRunner) tool.Tool {
	if run == nil {
		run = defaultGHRunner
	}
	return tool.Tool{
		Name: "gh_issue_search",
		Description: "Search issues in a GitHub repository via `gh issue list`. " +
			"Returns an array of {number,title,labels,createdAt,body,url,state}.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "repo":  {"type":"string","description":"owner/repo slug."},
    "query": {"type":"string","description":"Optional search query forwarded to --search."},
    "state": {"type":"string","enum":["open","closed","all"],"default":"open"},
    "limit": {"type":"integer","description":"Max results (1..100).","default":20}
  },
  "required":["repo"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
			var input issueSearchInput
			if err := json.Unmarshal(params, &input); err != nil {
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.Repo = strings.TrimSpace(input.Repo)
			input.Query = strings.TrimSpace(input.Query)
			input.State = strings.TrimSpace(strings.ToLower(input.State))
			if input.Repo == "" || !validRepo.MatchString(input.Repo) {
				return tool.JSONTextResult(map[string]any{"message": "repo must be owner/repo"}, true), nil
			}
			if input.State == "" {
				input.State = "open"
			}
			switch input.State {
			case "open", "closed", "all":
			default:
				return tool.JSONTextResult(map[string]any{"message": "state must be open|closed|all"}, true), nil
			}
			limit := input.Limit
			if limit <= 0 {
				limit = 20
			}
			if limit > issueSearchLimitMax {
				limit = issueSearchLimitMax
			}

			args := []string{"issue", "list", "--repo", input.Repo, "--state", input.State, "--limit", strconv.Itoa(limit), "--json", "number,title,labels,createdAt,body,url,state"}
			if input.Query != "" {
				args = append(args, "--search", input.Query)
			}
			output, err := run(ctx, args)
			if err != nil {
				msg, detail := wrapRunError(err, output)
				resp := map[string]any{"message": msg}
				for k, v := range detail {
					resp[k] = v
				}
				return tool.JSONTextResult(resp, true), nil
			}

			var issues []map[string]any
			if err := json.Unmarshal(output, &issues); err != nil {
				return tool.JSONTextResult(map[string]any{"message": "decode gh output failed", "detail": err.Error()}, true), nil
			}
			return tool.JSONTextResult(map[string]any{"count": len(issues), "issues": issues}, false), nil
		},
	}
}

type issueCreateInput struct {
	Repo   string   `json:"repo"`
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

func newIssueCreateTool(run ghRunner) tool.Tool {
	if run == nil {
		run = defaultGHRunner
	}
	return tool.Tool{
		Name:        "gh_issue_create",
		Description: "Create a new GitHub issue. Returns {number,url}.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "repo":   {"type":"string"},
    "title":  {"type":"string"},
    "body":   {"type":"string"},
    "labels": {"type":"array","items":{"type":"string"}}
  },
  "required":["repo","title"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
			var input issueCreateInput
			if err := json.Unmarshal(params, &input); err != nil {
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.Repo = strings.TrimSpace(input.Repo)
			input.Title = strings.TrimSpace(input.Title)
			if input.Repo == "" || !validRepo.MatchString(input.Repo) {
				return tool.JSONTextResult(map[string]any{"message": "repo must be owner/repo"}, true), nil
			}
			if input.Title == "" {
				return tool.JSONTextResult(map[string]any{"message": "title is required"}, true), nil
			}

			args := []string{"issue", "create", "--repo", input.Repo, "--title", input.Title}
			if input.Body != "" {
				args = append(args, "--body", input.Body)
			} else {
				args = append(args, "--body", "")
			}
			for _, label := range input.Labels {
				label = strings.TrimSpace(label)
				if label == "" {
					continue
				}
				args = append(args, "--label", label)
			}
			output, err := run(ctx, args)
			if err != nil {
				msg, detail := wrapRunError(err, output)
				resp := map[string]any{"message": msg}
				for k, v := range detail {
					resp[k] = v
				}
				return tool.JSONTextResult(resp, true), nil
			}
			url := extractLastURL(string(output))
			number := extractIssueNumber(url)
			return tool.JSONTextResult(map[string]any{"url": url, "number": number}, false), nil
		},
	}
}

type issueCommentInput struct {
	Repo        string `json:"repo"`
	IssueNumber int    `json:"issue_number"`
	Body        string `json:"body"`
}

func newIssueCommentTool(run ghRunner) tool.Tool {
	if run == nil {
		run = defaultGHRunner
	}
	return tool.Tool{
		Name:        "gh_issue_comment",
		Description: "Add a comment to an existing GitHub issue. Returns {ok,url}.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "repo":         {"type":"string"},
    "issue_number": {"type":"integer","minimum":1},
    "body":         {"type":"string"}
  },
  "required":["repo","issue_number","body"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
			var input issueCommentInput
			if err := json.Unmarshal(params, &input); err != nil {
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.Repo = strings.TrimSpace(input.Repo)
			if input.Repo == "" || !validRepo.MatchString(input.Repo) {
				return tool.JSONTextResult(map[string]any{"message": "repo must be owner/repo"}, true), nil
			}
			if input.IssueNumber <= 0 {
				return tool.JSONTextResult(map[string]any{"message": "issue_number must be >= 1"}, true), nil
			}
			if strings.TrimSpace(input.Body) == "" {
				return tool.JSONTextResult(map[string]any{"message": "body is required"}, true), nil
			}

			args := []string{"issue", "comment", strconv.Itoa(input.IssueNumber), "--repo", input.Repo, "--body", input.Body}
			output, err := run(ctx, args)
			if err != nil {
				msg, detail := wrapRunError(err, output)
				resp := map[string]any{"message": msg, "ok": false}
				for k, v := range detail {
					resp[k] = v
				}
				return tool.JSONTextResult(resp, true), nil
			}
			url := extractLastURL(string(output))
			return tool.JSONTextResult(map[string]any{"ok": true, "url": url}, false), nil
		},
	}
}

type prCreateDraftInput struct {
	Repo  string `json:"repo"`
	Head  string `json:"head"`
	Base  string `json:"base,omitempty"`
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
}

func newPRCreateDraftTool(run ghRunner) tool.Tool {
	if run == nil {
		run = defaultGHRunner
	}
	return tool.Tool{
		Name:        "gh_pr_create_draft",
		Description: "Create a draft pull request via `gh pr create --draft`. Returns {number,url}.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "repo":  {"type":"string"},
    "head":  {"type":"string","description":"Feature branch."},
    "base":  {"type":"string","description":"Target branch (default main)."},
    "title": {"type":"string"},
    "body":  {"type":"string"}
  },
  "required":["repo","head","title"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
			var input prCreateDraftInput
			if err := json.Unmarshal(params, &input); err != nil {
				return tool.JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			input.Repo = strings.TrimSpace(input.Repo)
			input.Head = strings.TrimSpace(input.Head)
			input.Base = strings.TrimSpace(input.Base)
			input.Title = strings.TrimSpace(input.Title)
			if input.Repo == "" || !validRepo.MatchString(input.Repo) {
				return tool.JSONTextResult(map[string]any{"message": "repo must be owner/repo"}, true), nil
			}
			if input.Head == "" || !validBranch.MatchString(input.Head) {
				return tool.JSONTextResult(map[string]any{"message": "head branch is invalid"}, true), nil
			}
			if input.Base == "" {
				input.Base = "main"
			}
			if !validBranch.MatchString(input.Base) {
				return tool.JSONTextResult(map[string]any{"message": "base branch is invalid"}, true), nil
			}
			if input.Title == "" {
				return tool.JSONTextResult(map[string]any{"message": "title is required"}, true), nil
			}

			args := []string{"pr", "create", "--repo", input.Repo, "--head", input.Head, "--base", input.Base, "--title", input.Title, "--draft", "--body", input.Body}
			output, err := run(ctx, args)
			if err != nil {
				msg, detail := wrapRunError(err, output)
				resp := map[string]any{"message": msg}
				for k, v := range detail {
					resp[k] = v
				}
				return tool.JSONTextResult(resp, true), nil
			}
			url := extractLastURL(string(output))
			return tool.JSONTextResult(map[string]any{"url": url, "number": extractIssueNumber(url)}, false), nil
		},
	}
}

// extractLastURL returns the last http(s) URL on any line of output.
func extractLastURL(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
			return line
		}
	}
	return ""
}

// extractIssueNumber parses the trailing numeric segment of a gh issue/PR URL.
func extractIssueNumber(url string) int {
	if url == "" {
		return 0
	}
	idx := strings.LastIndex(url, "/")
	if idx < 0 || idx == len(url)-1 {
		return 0
	}
	n, err := strconv.Atoi(url[idx+1:])
	if err != nil {
		return 0
	}
	return n
}
