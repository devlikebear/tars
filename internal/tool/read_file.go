package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/secrets"
)

const (
	defaultReadFileMaxBytes   = 8192
	maxReadFileMaxBytes       = 65536
	defaultReadFileMaxLines   = 2000
	maxReadFileMaxLines       = 2000
	maxReadFileLineChars      = 2000
	maxReadFileSizeBytes      = 20 * 1024 * 1024
	readFileTruncatedLineNote = "... [truncated]"
)

type readFileResponse struct {
	Path          string `json:"path,omitempty"`
	Bytes         int    `json:"bytes,omitempty"`
	TotalLines    int    `json:"total_lines,omitempty"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
	LinesReturned int    `json:"lines_returned,omitempty"`
	NextOffset    int    `json:"next_offset,omitempty"`
	Truncated     bool   `json:"truncated,omitempty"`
	Content       string `json:"content,omitempty"`
	Message       string `json:"message,omitempty"`
}

func NewReadTool(workspaceDir string) Tool {
	return newReadToolWithName("read", workspaceDir)
}

func NewReadFileTool(workspaceDir string) Tool {
	return newReadToolWithName("read_file", workspaceDir)
}

func NewReadFileToolWithPolicy(policy PathPolicy) Tool {
	return newReadToolWithPolicy("read_file", policy)
}

func newReadToolWithName(name, workspaceDir string) Tool {
	return newReadToolWithPolicy(name, SingleDirPolicy(workspaceDir))
}

func newReadToolWithPolicy(name string, policy PathPolicy) Tool {
	return Tool{
		Name:        name,
		Description: "Read a UTF-8 text file from the workspace using line-oriented pagination.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "path":{"type":"string","description":"Workspace-relative file path to read."},
    "offset":{"type":"integer","minimum":1,"description":"1-based starting line number.","default":1},
    "limit":{"type":"integer","minimum":1,"maximum":2000,"description":"Maximum number of lines to return.","default":2000},
    "start_line":{"type":"integer","minimum":1,"description":"Alias for offset using Gemini-style naming."},
    "end_line":{"type":"integer","minimum":1,"description":"Inclusive end line using Gemini-style naming."},
    "max_bytes":{"type":"integer","minimum":1,"maximum":65536,"description":"Legacy byte cap applied after line selection."}
  },
  "required":["path"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Path      string `json:"path"`
				Offset    *int   `json:"offset,omitempty"`
				Limit     *int   `json:"limit,omitempty"`
				StartLine *int   `json:"start_line,omitempty"`
				EndLine   *int   `json:"end_line,omitempty"`
				MaxBytes  *int   `json:"max_bytes,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return readFileErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
			if input.Path == "" {
				return readFileErrorResult("path is required"), nil
			}

			absPath, err := resolvePathWithPolicy(policy, input.Path)
			if err != nil {
				return readFileErrorResult(err.Error()), nil
			}

			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return readFileErrorResult("file not found"), nil
				}
				return readFileErrorResult(fmt.Sprintf("stat file failed: %v", err)), nil
			}
			if info.IsDir() {
				return readFileErrorResult("path is a directory"), nil
			}
			if info.Size() > maxReadFileSizeBytes {
				return readFileErrorResult(fmt.Sprintf("file size exceeds the 20MB limit: %d bytes", info.Size())), nil
			}

			raw, err := os.ReadFile(absPath)
			if err != nil {
				return readFileErrorResult(fmt.Sprintf("read file failed: %v", err)), nil
			}
			if !utf8.Valid(raw) {
				return readFileErrorResult("file is not valid UTF-8 text"), nil
			}

			startLine, limit, err := resolveReadLineWindow(input.Offset, input.Limit, input.StartLine, input.EndLine)
			if err != nil {
				return readFileErrorResult(err.Error()), nil
			}
			lines := splitTextLines(strings.TrimPrefix(string(raw), "\uFEFF"))
			totalLines := len(lines)
			if totalLines > 0 && startLine > totalLines {
				return readFileErrorResult(fmt.Sprintf("start line %d exceeds total lines %d", startLine, totalLines)), nil
			}

			payload := readFileResponse{
				Path:       policyRelativePath(policy, absPath),
				Bytes:      len(raw),
				TotalLines: totalLines,
			}

			if totalLines == 0 {
				payload.Content = ""
				return JSONTextResult(payload, false), nil
			}

			startIndex := startLine - 1
			endIndex := startIndex + limit
			if endIndex > totalLines {
				endIndex = totalLines
			}

			selected, shortenedLines := truncateReadLines(lines[startIndex:endIndex], maxReadFileLineChars)
			content := strings.Join(selected, "\n")
			maxBytes := resolvePositiveBoundedInt(0, maxReadFileMaxBytes, input.MaxBytes)
			byteTruncated := false
			if maxBytes > 0 {
				var truncated string
				truncated, byteTruncated = truncateUTF8StringByBytes(content, maxBytes)
				content = truncated
			}

			payload.StartLine = startLine
			payload.EndLine = endIndex
			payload.LinesReturned = endIndex - startIndex
			payload.Content = secrets.RedactText(content)

			pageTruncated := endIndex < totalLines
			payload.Truncated = pageTruncated || shortenedLines > 0 || byteTruncated
			if pageTruncated {
				payload.NextOffset = endIndex + 1
			}
			payload.Message = buildReadFileMessage(payload, shortenedLines, byteTruncated, maxBytes)
			return JSONTextResult(payload, false), nil
		},
	}
}

func resolveReadLineWindow(offset, limit, startLine, endLine *int) (int, int, error) {
	start := 1
	if offset != nil {
		if *offset <= 0 {
			return 0, 0, fmt.Errorf("offset must be >= 1")
		}
		start = *offset
	}
	if startLine != nil {
		if *startLine <= 0 {
			return 0, 0, fmt.Errorf("start_line must be >= 1")
		}
		if offset != nil && *offset != *startLine {
			return 0, 0, fmt.Errorf("offset and start_line must match when both are provided")
		}
		start = *startLine
	}

	if endLine != nil && *endLine <= 0 {
		return 0, 0, fmt.Errorf("end_line must be >= 1")
	}

	lines := resolvePositiveBoundedInt(defaultReadFileMaxLines, maxReadFileMaxLines, limit)
	if endLine != nil {
		if *endLine < start {
			return 0, 0, fmt.Errorf("end_line must be >= start_line")
		}
		requested := (*endLine - start) + 1
		if requested < lines {
			lines = requested
		} else if requested > maxReadFileMaxLines {
			lines = maxReadFileMaxLines
		} else {
			lines = requested
		}
	}
	return start, lines, nil
}

func splitTextLines(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func truncateReadLines(lines []string, maxChars int) ([]string, int) {
	if len(lines) == 0 {
		return nil, 0
	}
	truncated := 0
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if utf8.RuneCountInString(line) <= maxChars {
			out = append(out, line)
			continue
		}
		truncated++
		out = append(out, truncateStringByRunes(line, maxChars)+readFileTruncatedLineNote)
	}
	return out, truncated
}

func truncateStringByRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for idx := range value {
		if count == maxRunes {
			return value[:idx]
		}
		count++
	}
	return value
}

func truncateUTF8StringByBytes(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	if maxBytes >= len(value) {
		return value, false
	}
	end := 0
	for idx := range value {
		if idx > maxBytes {
			break
		}
		end = idx
	}
	if end == 0 && maxBytes < len(value) {
		return "", true
	}
	return value[:end], true
}

func buildReadFileMessage(body readFileResponse, shortenedLines int, byteTruncated bool, maxBytes int) string {
	parts := make([]string, 0, 3)
	if body.NextOffset > 0 {
		parts = append(parts,
			"IMPORTANT: The file content has been truncated.",
			fmt.Sprintf("Status: Showing lines %d-%d of %d total lines.", body.StartLine, body.EndLine, body.TotalLines),
			fmt.Sprintf("Action: To read more, use start_line: %d or offset: %d.", body.NextOffset, body.NextOffset),
		)
	}
	if shortenedLines > 0 {
		parts = append(parts, fmt.Sprintf("Some lines were shortened to %d characters.", maxReadFileLineChars))
	}
	if byteTruncated {
		parts = append(parts, fmt.Sprintf("Legacy max_bytes cap truncated the selected content to %d bytes.", maxBytes))
	}
	return strings.Join(parts, "\n")
}

// JSONTextResult marshals a value to JSON text and wraps it in a Result.
func JSONTextResult(value any, isError bool) Result {
	raw, err := json.Marshal(value)
	if err != nil {
		return Result{
			Content: []ContentBlock{
				{Type: "text", Text: fmt.Sprintf("marshal result failed: %v", err)},
			},
			IsError: true,
		}
	}
	return Result{
		Content: []ContentBlock{
			{Type: "text", Text: string(raw)},
		},
		IsError: isError,
	}
}

func readFileErrorResult(message string) Result {
	return JSONTextResult(readFileResponse{Message: message}, true)
}
