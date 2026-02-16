package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type globResponse struct {
	Pattern   string   `json:"pattern,omitempty"`
	Count     int      `json:"count"`
	Truncated bool     `json:"truncated,omitempty"`
	Matches   []string `json:"matches,omitempty"`
	Message   string   `json:"message,omitempty"`
}

const (
	defaultGlobLimit = 200
	maxGlobLimit     = 1000
)

func NewGlobTool(workspaceDir string) Tool {
	return Tool{
		Name:        "glob",
		Description: "Find workspace paths matching a glob pattern.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "pattern":{"type":"string","description":"Glob pattern relative to workspace, e.g. src/**/*.go"},
    "limit":{"type":"integer","minimum":1,"maximum":1000,"default":200}
  },
  "required":["pattern"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Pattern string `json:"pattern"`
				Limit   *int   `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(globResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			pattern := strings.TrimSpace(input.Pattern)
			if pattern == "" {
				return jsonTextResult(globResponse{Message: "pattern is required"}, true), nil
			}
			if filepath.IsAbs(pattern) {
				return jsonTextResult(globResponse{Message: "absolute pattern is not allowed"}, true), nil
			}
			cleanPattern := filepath.Clean(pattern)
			if cleanPattern == ".." || strings.HasPrefix(cleanPattern, ".."+string(filepath.Separator)) {
				return jsonTextResult(globResponse{Message: "pattern escapes workspace"}, true), nil
			}

			limit := defaultGlobLimit
			if input.Limit != nil {
				limit = *input.Limit
			}
			if limit <= 0 {
				limit = defaultGlobLimit
			}
			if limit > maxGlobLimit {
				limit = maxGlobLimit
			}

			rootAbs, err := filepath.Abs(workspaceDir)
			if err != nil {
				return jsonTextResult(globResponse{Message: fmt.Sprintf("resolve workspace failed: %v", err)}, true), nil
			}
			fullPattern := filepath.Join(rootAbs, cleanPattern)
			matches, err := filepath.Glob(fullPattern)
			if err != nil {
				return jsonTextResult(globResponse{Message: fmt.Sprintf("invalid glob pattern: %v", err)}, true), nil
			}

			result := make([]string, 0, len(matches))
			truncated := false
			for _, m := range matches {
				if !pathWithinWorkspace(rootAbs, m) {
					continue
				}
				if len(result) >= limit {
					truncated = true
					break
				}
				result = append(result, workspaceRelativePath(workspaceDir, m))
			}
			return jsonTextResult(globResponse{
				Pattern:   pattern,
				Count:     len(result),
				Truncated: truncated,
				Matches:   result,
			}, false), nil
		},
	}
}
