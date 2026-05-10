package tarsserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var skillCreatorNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type skillCreatorSubmitter func(context.Context, skillCreatorSubmitRequest) (skillCreatorSubmitResponse, error)

type skillCreatorDraftRequest struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Category         string   `json:"category"`
	Language         string   `json:"language"`
	Layout           string   `json:"layout"`
	UseCase          string   `json:"use_case"`
	RecommendedTools []string `json:"recommended_tools"`
}

type skillCreatorFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type skillCreatorDraftResponse struct {
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	Category         string             `json:"category,omitempty"`
	Language         string             `json:"language"`
	Layout           string             `json:"layout"`
	UseCase          string             `json:"use_case"`
	RecommendedTools []string           `json:"recommended_tools"`
	Files            []skillCreatorFile `json:"files"`
	Warnings         []string           `json:"warnings,omitempty"`
}

type skillCreatorSaveResponse struct {
	Saved bool     `json:"saved"`
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

type skillCreatorTestResponse struct {
	Success     bool                    `json:"success"`
	ExitCode    int                     `json:"exit_code"`
	Stdout      string                  `json:"stdout"`
	Stderr      string                  `json:"stderr"`
	SandboxPath string                  `json:"sandbox_path"`
	SessionKind string                  `json:"session_kind"`
	Hidden      bool                    `json:"hidden"`
	DurationMS  int64                   `json:"duration_ms"`
	ToolTrail   []skillCreatorToolTrail `json:"tool_trail"`
}

type skillCreatorToolTrail struct {
	Tool       string `json:"tool"`
	Command    string `json:"command"`
	Cwd        string `json:"cwd"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
}

type skillCreatorSubmitRequest struct {
	Name     string `json:"name"`
	RepoPath string `json:"repo_path,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Confirm  bool   `json:"confirm"`
}

type skillCreatorSubmitResponse struct {
	Submitted bool     `json:"submitted"`
	Ready     bool     `json:"ready"`
	Message   string   `json:"message"`
	Commands  []string `json:"commands,omitempty"`
}

func newSkillCreatorAPIHandler(workspaceDir string, logger zerolog.Logger, submitter skillCreatorSubmitter, provider extensionsProvider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/admin/skills/draft", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req skillCreatorDraftRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		draft, err := buildSkillCreatorDraft(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, draft)
	})
	mux.HandleFunc("/v1/admin/skills/save-local", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var draft skillCreatorDraftResponse
		if !decodeJSONBody(w, r, &draft) {
			return
		}
		saved, err := saveSkillCreatorDraft(workspaceDir, draft)
		if err != nil {
			logger.Error().Err(err).Str("skill", draft.Name).Msg("save skill creator draft failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if provider != nil {
			if reloadErr := provider.Reload(r.Context()); reloadErr != nil {
				logger.Warn().Err(reloadErr).Str("skill", draft.Name).Msg("reload extensions after skill save failed")
			}
		}
		writeJSON(w, http.StatusOK, saved)
	})
	mux.HandleFunc("/v1/admin/skills/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/admin/skills/"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill name is required"})
			return
		}
		if err := validateSkillCreatorName(name); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		skillDir := filepath.Join(workspaceDir, "skills", name)
		skillFile := filepath.Join(skillDir, "SKILL.md")
		switch r.Method {
		case http.MethodGet:
			content, err := os.ReadFile(skillFile)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"name": name, "content": string(content), "path": skillFile})
		case http.MethodPut:
			var req struct {
				Content string `json:"content"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			if strings.TrimSpace(req.Content) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
				return
			}
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				logger.Error().Err(err).Str("skill", name).Msg("create skill directory failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
				return
			}
			if err := os.WriteFile(skillFile, []byte(req.Content), 0o644); err != nil {
				logger.Error().Err(err).Str("skill", name).Msg("write skill file failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update skill"})
				return
			}
			if provider != nil {
				if reloadErr := provider.Reload(r.Context()); reloadErr != nil {
					logger.Warn().Err(reloadErr).Str("skill", name).Msg("reload extensions after skill update failed")
				}
			}
			writeJSON(w, http.StatusOK, map[string]string{"name": name, "path": skillFile})
		case http.MethodDelete:
			if _, err := os.Stat(skillDir); os.IsNotExist(err) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
				return
			}
			if err := os.RemoveAll(skillDir); err != nil {
				logger.Error().Err(err).Str("skill", name).Msg("delete skill directory failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete skill"})
				return
			}
			if provider != nil {
				if reloadErr := provider.Reload(r.Context()); reloadErr != nil {
					logger.Warn().Err(reloadErr).Str("skill", name).Msg("reload extensions after skill delete failed")
				}
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/v1/admin/skills/test", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var draft skillCreatorDraftResponse
		if !decodeJSONBody(w, r, &draft) {
			return
		}
		result, err := testSkillCreatorDraft(r.Context(), workspaceDir, draft)
		if err != nil {
			logger.Error().Err(err).Str("skill", draft.Name).Msg("test skill creator draft failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/v1/admin/skills/submit-pr", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req skillCreatorSubmitRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if err := validateSkillCreatorName(req.Name); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if submitter != nil {
			resp, err := submitter(r.Context(), req)
			if err != nil {
				logger.Error().Err(err).Str("skill", req.Name).Msg("submit skill draft PR failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
		resp := skillCreatorSubmitResponse{
			Ready:   true,
			Message: "Local draft is ready. Draft PR submission requires a configured tars-skills checkout and explicit confirmation.",
			Commands: []string{
				fmt.Sprintf("cp -R %s %s", shellQuote(filepath.ToSlash(filepath.Join(workspaceDir, "skills", req.Name))), shellQuote("tars-skills/skills/")),
				fmt.Sprintf("gh pr create --draft --title %q", "Add "+req.Name+" skill"),
			},
		}
		writeJSON(w, http.StatusOK, resp)
	})
	return mux
}

func buildSkillCreatorDraft(req skillCreatorDraftRequest) (skillCreatorDraftResponse, error) {
	name := strings.TrimSpace(req.Name)
	if err := validateSkillCreatorName(name); err != nil {
		return skillCreatorDraftResponse{}, err
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		return skillCreatorDraftResponse{}, fmt.Errorf("description is required")
	}
	useCase := strings.TrimSpace(req.UseCase)
	if useCase == "" {
		return skillCreatorDraftResponse{}, fmt.Errorf("use_case is required")
	}
	language, err := normalizeSkillCreatorLanguage(req.Language)
	if err != nil {
		return skillCreatorDraftResponse{}, err
	}
	layout := normalizeSkillCreatorLayout(req.Layout)
	tools := inferSkillCreatorTools(req.RecommendedTools, language, useCase)
	files := []skillCreatorFile{
		{Path: "SKILL.md", Content: renderSkillCreatorMarkdown(name, description, req.Category, useCase, tools)},
		renderSkillCreatorCLI(name, language, layout),
	}
	return skillCreatorDraftResponse{
		Name:             name,
		Description:      description,
		Category:         strings.TrimSpace(req.Category),
		Language:         language,
		Layout:           layout,
		UseCase:          useCase,
		RecommendedTools: tools,
		Files:            files,
	}, nil
}

func saveSkillCreatorDraft(workspaceDir string, draft skillCreatorDraftResponse) (skillCreatorSaveResponse, error) {
	if strings.TrimSpace(workspaceDir) == "" {
		return skillCreatorSaveResponse{}, fmt.Errorf("workspace directory is required")
	}
	cleanFiles, err := cleanSkillCreatorDraftFiles(draft)
	if err != nil {
		return skillCreatorSaveResponse{}, err
	}
	targetDir := filepath.Join(workspaceDir, "skills", draft.Name)
	saved, err := writeSkillCreatorDraftFiles(targetDir, cleanFiles)
	if err != nil {
		return skillCreatorSaveResponse{}, err
	}
	return skillCreatorSaveResponse{Saved: true, Path: targetDir, Files: saved}, nil
}

func testSkillCreatorDraft(ctx context.Context, workspaceDir string, draft skillCreatorDraftResponse) (skillCreatorTestResponse, error) {
	if strings.TrimSpace(workspaceDir) == "" {
		return skillCreatorTestResponse{}, fmt.Errorf("workspace directory is required")
	}
	cleanFiles, err := cleanSkillCreatorDraftFiles(draft)
	if err != nil {
		return skillCreatorTestResponse{}, err
	}
	baseDir := filepath.Join(workspaceDir, "tmp", "skill-tests")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return skillCreatorTestResponse{}, fmt.Errorf("create skill test directory: %w", err)
	}
	sandboxPath, err := os.MkdirTemp(baseDir, draft.Name+"-")
	if err != nil {
		return skillCreatorTestResponse{}, fmt.Errorf("create skill test sandbox: %w", err)
	}
	targetDir := filepath.Join(sandboxPath, "skills", draft.Name)
	if _, err := writeSkillCreatorDraftFiles(targetDir, cleanFiles); err != nil {
		return skillCreatorTestResponse{}, err
	}
	cliPath, err := findSkillCreatorCLIPath(cleanFiles)
	if err != nil {
		return skillCreatorTestResponse{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	command := "." + "/" + cliPath
	cmd := exec.CommandContext(runCtx, command, strings.TrimSpace(draft.UseCase))
	cmd.Dir = targetDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	status := "pass"
	if err != nil {
		status = "fail"
		if runCtx.Err() == context.DeadlineExceeded {
			status = "timeout"
		}
	}
	return skillCreatorTestResponse{
		Success:     err == nil,
		ExitCode:    exitCode,
		Stdout:      stdout.String(),
		Stderr:      stderr.String(),
		SandboxPath: sandboxPath,
		SessionKind: "worker",
		Hidden:      true,
		DurationMS:  duration.Milliseconds(),
		ToolTrail: []skillCreatorToolTrail{
			{
				Tool:       "bash",
				Command:    shellQuote(command) + " " + shellQuote(strings.TrimSpace(draft.UseCase)),
				Cwd:        targetDir,
				Status:     status,
				ExitCode:   exitCode,
				DurationMS: duration.Milliseconds(),
			},
		},
	}, nil
}

func cleanSkillCreatorDraftFiles(draft skillCreatorDraftResponse) ([]skillCreatorFile, error) {
	if err := validateSkillCreatorName(draft.Name); err != nil {
		return nil, err
	}
	if len(draft.Files) == 0 {
		return nil, fmt.Errorf("at least one file is required")
	}
	cleanFiles := make([]skillCreatorFile, 0, len(draft.Files))
	hasSkill := false
	for _, file := range draft.Files {
		rel, err := cleanSkillCreatorFilePath(file.Path)
		if err != nil {
			return nil, err
		}
		if rel == "SKILL.md" {
			hasSkill = true
		}
		cleanFiles = append(cleanFiles, skillCreatorFile{Path: rel, Content: file.Content})
	}
	if !hasSkill {
		return nil, fmt.Errorf("SKILL.md is required")
	}
	return cleanFiles, nil
}

func writeSkillCreatorDraftFiles(targetDir string, cleanFiles []skillCreatorFile) ([]string, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("create skill directory: %w", err)
	}
	saved := make([]string, 0, len(cleanFiles))
	for _, file := range cleanFiles {
		rel := file.Path
		target := filepath.Join(targetDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("create parent directory for %s: %w", rel, err)
		}
		mode := os.FileMode(0o644)
		if isSkillCreatorExecutable(rel) {
			mode = 0o755
		}
		if err := os.WriteFile(target, []byte(file.Content), mode); err != nil {
			return nil, fmt.Errorf("write %s: %w", rel, err)
		}
		saved = append(saved, rel)
	}
	sort.Strings(saved)
	return saved, nil
}

func findSkillCreatorCLIPath(files []skillCreatorFile) (string, error) {
	for _, file := range files {
		if file.Path == "SKILL.md" {
			continue
		}
		if isSkillCreatorExecutable(file.Path) {
			return file.Path, nil
		}
	}
	return "", fmt.Errorf("no executable companion CLI file found")
}

func validateSkillCreatorName(name string) error {
	if !skillCreatorNamePattern.MatchString(strings.TrimSpace(name)) {
		return fmt.Errorf("name must be kebab-case using lowercase letters, numbers, and dashes")
	}
	return nil
}

func normalizeSkillCreatorLanguage(language string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "shell", "bash", "sh":
		return "shell", nil
	case "python", "py":
		return "python", nil
	case "typescript", "ts", "node":
		return "typescript", nil
	default:
		return "", fmt.Errorf("language must be python, typescript, or shell")
	}
}

func normalizeSkillCreatorLayout(layout string) string {
	switch strings.ToLower(strings.TrimSpace(layout)) {
	case "directory", "dir":
		return "directory"
	default:
		return "single_file"
	}
}

func inferSkillCreatorTools(explicit []string, language string, useCase string) []string {
	seen := map[string]bool{"bash": true}
	tools := []string{"bash"}
	for _, tool := range explicit {
		tool = sanitizeSkillCreatorToolName(tool)
		if tool == "" || seen[tool] {
			continue
		}
		seen[tool] = true
		tools = append(tools, tool)
	}
	lowerUseCase := strings.ToLower(useCase)
	for _, candidate := range []struct {
		needle string
		tool   string
	}{
		{"http", "web_fetch"},
		{"url", "web_fetch"},
		{"github", "bash"},
		{"file", "bash"},
		{"docker", "bash"},
		{"slack", "web_fetch"},
	} {
		if strings.Contains(lowerUseCase, candidate.needle) && !seen[candidate.tool] {
			seen[candidate.tool] = true
			tools = append(tools, candidate.tool)
		}
	}
	if language == "typescript" && !seen["bash"] {
		tools = append(tools, "bash")
	}
	return tools
}

func sanitizeSkillCreatorToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return ""
	}
	return name
}

func renderSkillCreatorMarkdown(name, description, category, useCase string, tools []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + name + "\n")
	b.WriteString("description: " + yamlSingleLine(description) + "\n")
	if strings.TrimSpace(category) != "" {
		b.WriteString("category: " + yamlSingleLine(category) + "\n")
	}
	b.WriteString("recommended_tools: [" + strings.Join(tools, ", ") + "]\n")
	b.WriteString("user-invocable: true\n")
	b.WriteString("---\n\n")
	b.WriteString("# " + titleFromKebab(name) + "\n\n")
	b.WriteString("Use this skill when the user wants to " + sentenceFragment(useCase) + ".\n\n")
	b.WriteString("## Workflow\n\n")
	b.WriteString("1. Restate the target outcome and identify any missing inputs.\n")
	b.WriteString("2. Run the companion CLI from this skill directory for the repeatable work.\n")
	b.WriteString("3. Summarize outputs, errors, and any follow-up action clearly.\n\n")
	b.WriteString("## Companion CLI\n\n")
	b.WriteString("Start with the generated CLI stub and keep user-specific secrets in environment variables.\n")
	return b.String()
}

func renderSkillCreatorCLI(name, language, layout string) skillCreatorFile {
	path := skillCreatorCLIPath(name, language, layout)
	switch language {
	case "python":
		return skillCreatorFile{Path: path, Content: fmt.Sprintf(`#!/usr/bin/env python3
import argparse


def main() -> int:
    parser = argparse.ArgumentParser(description=%q)
    parser.add_argument("request", nargs="*", help="Natural-language task or inputs")
    args = parser.parse_args()
    request = " ".join(args.request).strip()
    print("TODO: implement %s for:", request or "(no request provided)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
`, "Companion CLI for "+name, name)}
	case "typescript":
		return skillCreatorFile{Path: path, Content: fmt.Sprintf(`#!/usr/bin/env node
const request = process.argv.slice(2).join(' ').trim()

console.log('TODO: implement %s for:', request || '(no request provided)')
`, name)}
	default:
		return skillCreatorFile{Path: path, Content: fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

request="${*:-}"
printf 'TODO: implement %s for: %%s\n' "${request:-"(no request provided)"}"
`, name)}
	}
}

func skillCreatorCLIPath(name, language, layout string) string {
	ext := ".sh"
	if language == "python" {
		ext = ".py"
	} else if language == "typescript" {
		ext = ".ts"
	}
	if layout == "directory" {
		return "bin/" + name + ext
	}
	return name + ext
}

func cleanSkillCreatorFilePath(path string) (string, error) {
	trimmed := strings.TrimSpace(filepath.ToSlash(path))
	if trimmed == "" || strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("file path must be relative")
	}
	clean := filepath.ToSlash(filepath.Clean(trimmed))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("unsafe file path %q", path)
	}
	return clean, nil
}

func isSkillCreatorExecutable(path string) bool {
	return strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".ts")
}

func yamlSingleLine(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\n", " ")
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func sentenceFragment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return "complete this workflow"
	}
	return value
}

func titleFromKebab(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
