package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type projectSkillResponse struct {
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Path        string `json:"path,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	Created     bool   `json:"created,omitempty"`
	Overwritten bool   `json:"overwritten,omitempty"`
	Message     string `json:"message,omitempty"`
}

type projectSkillInput struct {
	Kind             string   `json:"kind"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Body             string   `json:"body,omitempty"`
	TargetSkill      string   `json:"target_skill,omitempty"`
	Slash            string   `json:"slash,omitempty"`
	Aliases          []string `json:"aliases,omitempty"`
	RecommendedTools []string `json:"recommended_tools,omitempty"`
	Overwrite        bool     `json:"overwrite,omitempty"`
}

func NewProjectSkillToolWithPolicy(policy PathPolicy) Tool {
	return Tool{
		Name:        "project_skill",
		Description: "Create cwd-local TARS skills and explicit slash commands under the current session cwd's .tars directory.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "kind":{"type":"string","enum":["skill","command"],"description":"Create a cwd-local skill or explicit slash command."},
    "name":{"type":"string","description":"Slash-safe file name. Whitespace, path separators, and control characters are not allowed."},
    "description":{"type":"string","description":"Short frontmatter description."},
    "body":{"type":"string","description":"Markdown instruction body. Required for both skills and commands."},
    "target_skill":{"type":"string","description":"Deprecated legacy field; commands are standalone prompts and do not alias skills."},
    "slash":{"type":"string","description":"Optional skill slash command name. Defaults to name."},
    "aliases":{"type":"array","items":{"type":"string"},"description":"Optional additional slash aliases."},
    "recommended_tools":{"type":"array","items":{"type":"string"},"description":"Optional recommended builtin tools for the skill."},
    "overwrite":{"type":"boolean","default":false,"description":"Set true to replace an existing generated skill or command file."}
  },
  "required":["kind","name"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input projectSkillInput
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(projectSkillResponse{Message: fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}

			kind, err := normalizeProjectSkillKind(input.Kind)
			if err != nil {
				return JSONTextResult(projectSkillResponse{Message: err.Error()}, true), nil
			}
			name, err := normalizeProjectSkillName(input.Name)
			if err != nil {
				return JSONTextResult(projectSkillResponse{Message: err.Error()}, true), nil
			}

			var relPath string
			var content string
			switch kind {
			case "skill":
				content, err = renderProjectSkillFile(name, input)
				relPath = filepath.Join(".tars", "skills", name, "SKILL.md")
			case "command":
				content, err = renderProjectCommandFile(name, input)
				relPath = filepath.Join(".tars", "commands", name+".md")
			}
			if err != nil {
				return JSONTextResult(projectSkillResponse{Message: err.Error()}, true), nil
			}

			absPath, err := resolveWritePathWithPolicy(policy, relPath)
			if err != nil {
				return JSONTextResult(projectSkillResponse{Message: err.Error()}, true), nil
			}
			created, overwritten, err := writeProjectSkillFile(absPath, content, input.Overwrite)
			if err != nil {
				return JSONTextResult(projectSkillResponse{Message: err.Error()}, true), nil
			}
			return JSONTextResult(projectSkillResponse{
				Kind:        kind,
				Name:        name,
				Path:        policyRelativePath(policy, absPath),
				CWD:         filepath.ToSlash(policy.PrimaryDir),
				Created:     created,
				Overwritten: overwritten,
				Message:     "created under the current session cwd; it will be picked up on the next chat context refresh",
			}, false), nil
		},
	}
}

func normalizeProjectSkillKind(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "skill":
		return "skill", nil
	case "command", "slash_command", "slash-command", "alias":
		return "command", nil
	default:
		return "", fmt.Errorf("kind must be one of: skill, command")
	}
}

func normalizeProjectSkillName(raw string) (string, error) {
	name := strings.TrimSpace(strings.TrimPrefix(raw, "/"))
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if utf8.RuneCountInString(name) > 80 {
		return "", fmt.Errorf("name must be 80 characters or fewer")
	}
	if name == "." || name == ".." || strings.Contains(name, "..") {
		return "", fmt.Errorf("name must not contain path traversal")
	}
	if strings.ContainsAny(name, `/\:`) {
		return "", fmt.Errorf("name must not contain path separators")
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", fmt.Errorf("name must not contain whitespace or control characters")
		}
	}
	return name, nil
}

func renderProjectSkillFile(name string, input projectSkillInput) (string, error) {
	description, err := cleanProjectFrontmatterScalar(input.Description, "description")
	if err != nil {
		return "", err
	}
	slash := name
	if strings.TrimSpace(input.Slash) != "" {
		slash, err = normalizeProjectSkillName(input.Slash)
		if err != nil {
			return "", fmt.Errorf("slash %w", err)
		}
	}
	aliases, err := cleanProjectNameList(input.Aliases, "aliases")
	if err != nil {
		return "", err
	}
	recommendedTools, err := cleanProjectScalarList(input.RecommendedTools, "recommended_tools")
	if err != nil {
		return "", err
	}
	body := strings.TrimSpace(strings.ReplaceAll(input.Body, "\r\n", "\n"))
	if body == "" {
		return "", fmt.Errorf("body is required for kind=skill")
	}

	var b strings.Builder
	b.WriteString("---\n")
	writeProjectScalar(&b, "name", name)
	if description != "" {
		writeProjectScalar(&b, "description", description)
	}
	writeProjectScalar(&b, "slash", slash)
	writeProjectScalar(&b, "user_invocable", "true")
	writeProjectList(&b, "aliases", aliases)
	writeProjectList(&b, "recommended_tools", recommendedTools)
	b.WriteString("---\n\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String(), nil
}

func renderProjectCommandFile(name string, input projectSkillInput) (string, error) {
	description, err := cleanProjectFrontmatterScalar(input.Description, "description")
	if err != nil {
		return "", err
	}
	slash := name
	if strings.TrimSpace(input.Slash) != "" {
		slash, err = normalizeProjectSkillName(input.Slash)
		if err != nil {
			return "", fmt.Errorf("slash %w", err)
		}
	}
	aliases, err := cleanProjectNameList(input.Aliases, "aliases")
	if err != nil {
		return "", err
	}
	recommendedTools, err := cleanProjectScalarList(input.RecommendedTools, "recommended_tools")
	if err != nil {
		return "", err
	}
	body := strings.TrimSpace(strings.ReplaceAll(input.Body, "\r\n", "\n"))
	if body == "" {
		return "", fmt.Errorf("body is required for kind=command")
	}

	var b strings.Builder
	b.WriteString("---\n")
	writeProjectScalar(&b, "name", name)
	if description != "" {
		writeProjectScalar(&b, "description", description)
	}
	writeProjectScalar(&b, "slash", slash)
	writeProjectScalar(&b, "user_invocable", "true")
	writeProjectList(&b, "aliases", aliases)
	writeProjectList(&b, "recommended_tools", recommendedTools)
	b.WriteString("---\n\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String(), nil
}

func cleanProjectFrontmatterScalar(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("%s must be a single line", field)
	}
	return value, nil
}

func cleanProjectScalarList(values []string, field string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		cleaned, err := cleanProjectFrontmatterScalar(value, field)
		if err != nil {
			return nil, err
		}
		if cleaned == "" {
			continue
		}
		key := strings.ToLower(cleaned)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cleaned)
	}
	return out, nil
}

func cleanProjectNameList(values []string, field string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		cleaned, err := normalizeProjectSkillName(value)
		if err != nil {
			return nil, fmt.Errorf("%s %w", field, err)
		}
		key := strings.ToLower(cleaned)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cleaned)
	}
	return out, nil
}

func writeProjectScalar(b *strings.Builder, key string, value string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteString("\n")
}

func writeProjectList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		return
	}
	b.WriteString(key)
	b.WriteString(":\n")
	for _, value := range values {
		b.WriteString("  - ")
		b.WriteString(value)
		b.WriteString("\n")
	}
}

func writeProjectSkillFile(absPath, content string, overwrite bool) (created bool, overwritten bool, err error) {
	info, statErr := os.Stat(absPath)
	if statErr == nil {
		if info.IsDir() {
			return false, false, fmt.Errorf("path is a directory: %s", absPath)
		}
		if !overwrite {
			return false, false, fmt.Errorf("file already exists: %s (set overwrite=true to replace it)", absPath)
		}
	} else if os.IsNotExist(statErr) {
		created = true
	} else {
		return false, false, fmt.Errorf("stat file failed: %v", statErr)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return false, false, fmt.Errorf("create parent directories failed: %v", err)
	}

	mode := fs.FileMode(0o644)
	if statErr == nil {
		mode = info.Mode().Perm()
		overwritten = true
	}
	if err := writeTextFileAtomic(absPath, content, mode); err != nil {
		return false, false, fmt.Errorf("write file failed: %v", err)
	}
	return created, overwritten, nil
}
