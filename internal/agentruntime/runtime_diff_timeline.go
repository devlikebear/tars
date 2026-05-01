package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxDiffTimelinePatchBytes = 12000

type gitDiffSnapshot struct {
	repoRoot string
	files    map[string]gitDiffFileSnapshot
}

type gitDiffFileSnapshot struct {
	path        string
	status      string
	additions   int
	deletions   int
	patch       string
	fingerprint string
}

func captureGitDiffSnapshot(workspaceDir string) (gitDiffSnapshot, bool) {
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return gitDiffSnapshot{}, false
	}
	rootOut, ok := gitOutput(workspaceDir, "rev-parse", "--show-toplevel")
	if !ok {
		return gitDiffSnapshot{}, false
	}
	repoRoot := filepath.Clean(strings.TrimSpace(rootOut))
	if repoRoot == "" {
		return gitDiffSnapshot{}, false
	}
	snapshot := gitDiffSnapshot{
		repoRoot: repoRoot,
		files:    map[string]gitDiffFileSnapshot{},
	}
	if statusOut, ok := gitOutput(repoRoot, "status", "--porcelain=v1"); ok {
		for path, status := range parseGitStatus(statusOut) {
			snapshot.files[path] = gitDiffFileSnapshot{path: path, status: status}
		}
	}
	if numstatOut, ok := gitOutput(repoRoot, "diff", "--numstat"); ok {
		applyGitNumstat(snapshot.files, numstatOut)
	}
	if numstatOut, ok := gitOutput(repoRoot, "diff", "--cached", "--numstat"); ok {
		applyGitNumstat(snapshot.files, numstatOut)
	}
	for path, file := range snapshot.files {
		patch := gitPatchForPath(repoRoot, path, file.status)
		file.patch = truncateDiffPatch(patch)
		file.fingerprint = gitDiffFingerprint(file, patch)
		snapshot.files[path] = file
	}
	return snapshot, true
}

func buildDiffTimelineEntry(before gitDiffSnapshot, after gitDiffSnapshot, run Run) *DiffTimelineEntry {
	if strings.TrimSpace(after.repoRoot) == "" {
		return nil
	}
	paths := map[string]struct{}{}
	for path := range before.files {
		paths[path] = struct{}{}
	}
	for path := range after.files {
		paths[path] = struct{}{}
	}
	if len(paths) == 0 {
		return nil
	}
	files := make([]DiffFileChange, 0, len(paths))
	for path := range paths {
		beforeFile, hadBefore := before.files[path]
		afterFile, hasAfter := after.files[path]
		beforeFingerprint := ""
		if hadBefore {
			beforeFingerprint = beforeFile.fingerprint
		}
		afterFingerprint := ""
		if hasAfter {
			afterFingerprint = afterFile.fingerprint
		}
		if beforeFingerprint == afterFingerprint {
			continue
		}
		change := DiffFileChange{
			Path:            path,
			Status:          "cleaned",
			GitInspectorURL: gitInspectorURL(path),
		}
		if hasAfter {
			change.Status = afterFile.status
			change.Additions = afterFile.additions
			change.Deletions = afterFile.deletions
			change.Patch = afterFile.patch
		}
		files = append(files, change)
	}
	if len(files) == 0 {
		return nil
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	summary := DiffTimelineSummary{Files: len(files)}
	for _, file := range files {
		summary.Additions += file.Additions
		summary.Deletions += file.Deletions
	}
	entryID := "diff_" + strings.TrimSpace(run.ID)
	if entryID == "diff_" {
		entryID = "diff"
	}
	return &DiffTimelineEntry{
		ID:              entryID,
		RunID:           run.ID,
		SessionID:       run.SessionID,
		SessionKind:     run.SessionKind,
		Agent:           run.Agent,
		Prompt:          run.Prompt,
		ParentRunID:     run.ParentRunID,
		RootRunID:       run.RootRunID,
		FlowID:          run.FlowID,
		StepID:          run.StepID,
		StartedAt:       run.StartedAt,
		CompletedAt:     run.CompletedAt,
		RepoRoot:        after.repoRoot,
		GitInspectorURL: gitInspectorURL(""),
		Summary:         summary,
		Files:           files,
	}
}

func parseGitStatus(output string) map[string]string {
	files := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 4 {
			continue
		}
		code := line[:2]
		path := strings.TrimSpace(line[3:])
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		path = unquoteGitPath(path)
		if path == "" {
			continue
		}
		files[normalizeGitDiffPath(path)] = normalizeGitStatus(code)
	}
	return files
}

func applyGitNumstat(files map[string]gitDiffFileSnapshot, output string) {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		path := normalizeGitDiffPath(unquoteGitPath(parts[len(parts)-1]))
		if path == "" {
			continue
		}
		file := files[path]
		file.path = path
		if file.status == "" {
			file.status = "modified"
		}
		file.additions += parseGitNumstatValue(parts[0])
		file.deletions += parseGitNumstatValue(parts[1])
		files[path] = file
	}
}

func parseGitNumstatValue(value string) int {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func normalizeGitStatus(code string) string {
	if strings.Contains(code, "?") {
		return "untracked"
	}
	if strings.Contains(code, "U") {
		return "conflicted"
	}
	if strings.Contains(code, "R") {
		return "renamed"
	}
	if strings.Contains(code, "C") {
		return "copied"
	}
	if strings.Contains(code, "A") {
		return "added"
	}
	if strings.Contains(code, "D") {
		return "deleted"
	}
	if strings.Contains(code, "M") || strings.Contains(code, "T") {
		return "modified"
	}
	return "modified"
}

func normalizeGitDiffPath(path string) string {
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	path = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "./")
	if path == "." {
		return ""
	}
	return path
}

func unquoteGitPath(path string) string {
	path = strings.TrimSpace(path)
	if len(path) >= 2 && path[0] == '"' {
		if unquoted, err := strconv.Unquote(path); err == nil {
			return unquoted
		}
	}
	return path
}

func gitPatchForPath(repoRoot string, path string, status string) string {
	parts := make([]string, 0, 3)
	if out, ok := gitOutput(repoRoot, "diff", "--cached", "--no-ext-diff", "--", path); ok && strings.TrimSpace(out) != "" {
		parts = append(parts, strings.TrimRight(out, "\n"))
	}
	if out, ok := gitOutput(repoRoot, "diff", "--no-ext-diff", "--", path); ok && strings.TrimSpace(out) != "" {
		parts = append(parts, strings.TrimRight(out, "\n"))
	}
	if status == "untracked" {
		if out, ok := gitNoIndexPatch(repoRoot, path); ok && strings.TrimSpace(out) != "" {
			parts = append(parts, strings.TrimRight(out, "\n"))
		}
	}
	return strings.Join(parts, "\n")
}

func gitNoIndexPatch(repoRoot string, path string) (string, bool) {
	absPath := filepath.Join(repoRoot, filepath.FromSlash(path))
	if info, err := os.Stat(absPath); err != nil || info.IsDir() {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "diff", "--no-index", "--", os.DevNull, absPath)
	out, err := cmd.CombinedOutput()
	if len(out) == 0 {
		return "", false
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return string(out), true
		}
		return "", false
	}
	return string(out), true
}

func gitOutput(workspaceDir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workspaceDir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func gitDiffFingerprint(file gitDiffFileSnapshot, rawPatch string) string {
	sum := sha256.Sum256([]byte(rawPatch))
	return strings.Join([]string{
		file.status,
		strconv.Itoa(file.additions),
		strconv.Itoa(file.deletions),
		hex.EncodeToString(sum[:]),
	}, "\x00")
}

func truncateDiffPatch(value string) string {
	if len(value) <= maxDiffTimelinePatchBytes {
		return value
	}
	return value[:maxDiffTimelinePatchBytes] + "\n... diff preview truncated ..."
}

func gitInspectorURL(path string) string {
	if strings.TrimSpace(path) == "" {
		return "/console/git"
	}
	return "/console/git?path=" + url.QueryEscape(path)
}
