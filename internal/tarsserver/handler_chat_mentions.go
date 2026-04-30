package tarsserver

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

const (
	chatMentionDefaultLimit = 30
	chatMentionMaxLimit     = 80
	chatMentionFileMaxBytes = 64 * 1024
)

type chatFileMentionRequest struct {
	Kind string `json:"kind"`
	Root string `json:"root,omitempty"`
	Path string `json:"path"`
}

type chatSubagentMentionRequest struct {
	Name  string `json:"name"`
	Token string `json:"token,omitempty"`
}

type chatSubagentMention struct {
	Name        string
	Token       string
	Description string
	Tier        string
}

type chatFileMentionCandidate struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Root      string `json:"root"`
	RootLabel string `json:"root_label"`
	Token     string `json:"token"`
	Size      int64  `json:"size,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type chatFileMentionRoot struct {
	Dir     string
	Label   string
	Current bool
}

func handleChatFileMentionCandidates(w http.ResponseWriter, r *http.Request, deps chatHandlerDeps) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	reqStore, requestWorkspaceDir, _, err := resolveSessionStoreForRequest(deps.workspaceDir, deps.store, r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "resolve workspace failed")
		return
	}
	sessionID := resolveMentionLookupSessionID(r.URL.Query().Get("session_id"), deps.mainSessionID)
	roots := chatMentionRoots(reqStore, requestWorkspaceDir, sessionID)
	limit := parseChatMentionLimit(r.URL.Query().Get("limit"))
	candidates, err := listChatFileMentionCandidates(roots, r.URL.Query().Get("q"), limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mention_query", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}

func resolveMentionLookupSessionID(raw string, mainSessionID string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.EqualFold(trimmed, "main") {
		return strings.TrimSpace(mainSessionID)
	}
	if strings.EqualFold(trimmed, "new") {
		return ""
	}
	return trimmed
}

func parseChatMentionLimit(raw string) int {
	limit := chatMentionDefaultLimit
	if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n > 0 {
		limit = n
	}
	if limit > chatMentionMaxLimit {
		return chatMentionMaxLimit
	}
	return limit
}

func chatMentionRoots(store *session.Store, workspaceDir string, sessionID string) []chatFileMentionRoot {
	var roots []chatFileMentionRoot
	addRoot := func(dir, label string, current bool) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		root := resolveWorkspaceFilesRoot(workspaceDir, dir)
		canonical := canonicalWorkspacePath(root)
		for _, existing := range roots {
			if canonicalWorkspacePath(existing.Dir) == canonical {
				return
			}
		}
		if strings.TrimSpace(label) == "" {
			label = filepath.Base(root)
		}
		roots = append(roots, chatFileMentionRoot{Dir: root, Label: label, Current: current})
	}

	if store != nil && strings.TrimSpace(sessionID) != "" {
		if sess, err := store.Get(sessionID); err == nil {
			addRoot(sess.CurrentDir, "current", true)
			for _, dir := range sess.WorkDirs {
				addRoot(dir, filepath.Base(dir), false)
			}
		}
	}
	if len(roots) == 0 {
		addRoot(filepath.Join(workspaceDir, "artifacts"), "artifacts", true)
	}
	return roots
}

func listChatFileMentionCandidates(roots []chatFileMentionRoot, rawQuery string, limit int) ([]chatFileMentionCandidate, error) {
	parentPath, prefix, err := splitChatMentionQuery(rawQuery)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = chatMentionDefaultLimit
	}
	prefix = strings.ToLower(prefix)
	candidates := make([]chatFileMentionCandidate, 0, limit)
	for _, root := range roots {
		cleanParent, absParent, err := resolveWorkspaceFilesPath(root.Dir, parentPath, ".")
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(absParent)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(absParent)
		if err != nil {
			return nil, err
		}
		rootCandidates := make([]chatFileMentionCandidate, 0, len(entries))
		for _, entry := range entries {
			name := entry.Name()
			if shouldHideWorkspaceEntry(name) {
				continue
			}
			if prefix != "" && !strings.HasPrefix(strings.ToLower(name), prefix) {
				continue
			}
			path := workspaceChildPath(cleanParent, name)
			kind := "file"
			token := "@" + path
			if entry.IsDir() {
				kind = "directory"
				token += "/"
			}
			candidate := chatFileMentionCandidate{
				Kind:      kind,
				Name:      name,
				Path:      path,
				Root:      root.Dir,
				RootLabel: root.Label,
				Token:     token,
			}
			if info, err := entry.Info(); err == nil {
				candidate.Size = info.Size()
				candidate.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
			}
			rootCandidates = append(rootCandidates, candidate)
		}
		sort.Slice(rootCandidates, func(i, j int) bool {
			if rootCandidates[i].Kind != rootCandidates[j].Kind {
				return rootCandidates[i].Kind == "directory"
			}
			return strings.ToLower(rootCandidates[i].Name) < strings.ToLower(rootCandidates[j].Name)
		})
		for _, candidate := range rootCandidates {
			candidates = append(candidates, candidate)
			if len(candidates) >= limit {
				return candidates, nil
			}
		}
	}
	return candidates, nil
}

func splitChatMentionQuery(rawQuery string) (parentPath string, prefix string, err error) {
	query := strings.TrimSpace(strings.TrimPrefix(rawQuery, "@"))
	query = filepath.ToSlash(query)
	if query == "" {
		return ".", "", nil
	}
	if strings.HasPrefix(query, "/") || filepath.IsAbs(query) {
		return "", "", fmt.Errorf("absolute paths are not allowed")
	}
	parts := strings.Split(query, "/")
	for _, part := range parts {
		if part == ".." {
			return "", "", fmt.Errorf("parent traversal is not allowed")
		}
	}
	if strings.HasSuffix(query, "/") {
		parent := strings.TrimSuffix(query, "/")
		if parent == "" {
			parent = "."
		}
		return parent, "", nil
	}
	parent := filepath.ToSlash(filepath.Dir(query))
	if parent == "" || parent == "." {
		parent = "."
	}
	return parent, filepath.Base(query), nil
}

func shouldHideWorkspaceEntry(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

func chatMentionsToContentBlocks(store *session.Store, workspaceDir string, sessionID string, mentions []chatFileMentionRequest) ([]llm.ContentBlock, []string, error) {
	if len(mentions) == 0 {
		return nil, nil, nil
	}
	roots := chatMentionRoots(store, workspaceDir, sessionID)
	blocks := make([]llm.ContentBlock, 0, len(mentions))
	paths := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		block, displayPath, err := chatMentionToContentBlock(roots, mention)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(block.Text) == "" {
			continue
		}
		blocks = append(blocks, block)
		paths = append(paths, displayPath)
	}
	return blocks, paths, nil
}

func normalizeChatSubagentMentions(runtime *agentruntime.Runtime, mentions []chatSubagentMentionRequest) ([]chatSubagentMention, error) {
	if len(mentions) == 0 {
		return nil, nil
	}
	if runtime == nil || !runtime.Enabled() {
		return nil, fmt.Errorf("agent runtime is not configured")
	}
	seen := make(map[string]struct{}, len(mentions))
	out := make([]chatSubagentMention, 0, len(mentions))
	for _, mention := range mentions {
		name := normalizeChatSubagentMentionName(mention.Name)
		if name == "" {
			name = normalizeChatSubagentMentionName(mention.Token)
		}
		if name == "" {
			return nil, fmt.Errorf("subagent mention name is required")
		}
		info, ok := runtime.LookupAgent(name)
		if !ok {
			return nil, fmt.Errorf("subagent mention not found: %s", name)
		}
		if !info.Enabled {
			return nil, fmt.Errorf("subagent mention is disabled: %s", name)
		}
		canonical := strings.TrimSpace(info.Name)
		if canonical == "" {
			canonical = name
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, chatSubagentMention{
			Name:        canonical,
			Token:       firstNonEmpty(strings.TrimSpace(mention.Token), "@"+canonical),
			Description: strings.TrimSpace(info.Description),
			Tier:        strings.TrimSpace(info.Tier),
		})
	}
	return out, nil
}

func normalizeChatSubagentMentionName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "@")
	trimmed = strings.TrimRight(trimmed, ",.;:")
	return strings.TrimSpace(trimmed)
}

func formatChatSubagentMentionHint(mentions []chatSubagentMention) string {
	if len(mentions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Mentioned Subagents\n")
	b.WriteString("The user explicitly mentioned these AgentRuntime subagent targets in the current chat message.\n")
	b.WriteString("- Prefer `subagents_run` for direct delegated work. Use staged-flow tools such as `subagents_plan` or `subagents_orchestrate` only when they are explicitly available in this session's tool schema.\n")
	b.WriteString("- When calling a subagent tool for a listed target, set the top-level `agent` field to the exact name below.\n")
	for _, mention := range mentions {
		b.WriteString("- `")
		b.WriteString(mention.Name)
		b.WriteString("`")
		if mention.Tier != "" {
			b.WriteString(" tier=")
			b.WriteString(mention.Tier)
		}
		if mention.Description != "" {
			b.WriteString(" - ")
			b.WriteString(mention.Description)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func chatSubagentMentionNames(mentions []chatSubagentMention) []string {
	names := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		if strings.TrimSpace(mention.Name) == "" {
			continue
		}
		names = append(names, strings.TrimSpace(mention.Name))
	}
	return names
}

func chatMentionToContentBlock(roots []chatFileMentionRoot, mention chatFileMentionRequest) (llm.ContentBlock, string, error) {
	root, cleanPath, absPath, err := resolveChatMentionPath(roots, mention)
	if err != nil {
		return llm.ContentBlock{}, "", err
	}
	displayPath := cleanPath
	if displayPath == "." {
		displayPath = filepath.Base(root.Dir)
	}
	kind := strings.ToLower(strings.TrimSpace(mention.Kind))
	switch kind {
	case "file":
		text, err := readChatMentionFileBlock(displayPath, absPath)
		if err != nil {
			return llm.ContentBlock{}, "", err
		}
		return llm.ContentBlock{Type: "text", Text: text}, displayPath, nil
	case "directory":
		text, err := readChatMentionDirectoryBlock(displayPath, absPath)
		if err != nil {
			return llm.ContentBlock{}, "", err
		}
		return llm.ContentBlock{Type: "text", Text: text}, displayPath + "/", nil
	default:
		return llm.ContentBlock{}, "", fmt.Errorf("unsupported mention kind %q", mention.Kind)
	}
}

func resolveChatMentionPath(roots []chatFileMentionRoot, mention chatFileMentionRequest) (chatFileMentionRoot, string, string, error) {
	if len(roots) == 0 {
		return chatFileMentionRoot{}, "", "", fmt.Errorf("no file mention roots are available")
	}
	root := roots[0]
	requestedRoot := strings.TrimSpace(mention.Root)
	if requestedRoot != "" {
		resolvedRoot := canonicalWorkspacePath(resolveWorkspaceFilesRoot("", requestedRoot))
		found := false
		for _, candidate := range roots {
			if canonicalWorkspacePath(candidate.Dir) == resolvedRoot {
				root = candidate
				found = true
				break
			}
		}
		if !found {
			return chatFileMentionRoot{}, "", "", fmt.Errorf("mention root is not in this session's Files paths")
		}
	}
	if strings.TrimSpace(mention.Path) == "" {
		return chatFileMentionRoot{}, "", "", fmt.Errorf("mention path is required")
	}
	cleanPath, absPath, err := resolveWorkspaceFilesPath(root.Dir, mention.Path, "")
	if err != nil {
		return chatFileMentionRoot{}, "", "", err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return chatFileMentionRoot{}, "", "", fmt.Errorf("mentioned path not found")
		}
		return chatFileMentionRoot{}, "", "", err
	}
	kind := strings.ToLower(strings.TrimSpace(mention.Kind))
	if kind == "file" && info.IsDir() {
		return chatFileMentionRoot{}, "", "", fmt.Errorf("mentioned path is a directory")
	}
	if kind == "directory" && !info.IsDir() {
		return chatFileMentionRoot{}, "", "", fmt.Errorf("mentioned path is not a directory")
	}
	return root, filepath.ToSlash(cleanPath), absPath, nil
}

func readChatMentionFileBlock(displayPath string, absPath string) (string, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, chatMentionFileMaxBytes+1))
	if err != nil {
		return "", err
	}
	truncated := len(raw) > chatMentionFileMaxBytes
	if truncated {
		raw = raw[:chatMentionFileMaxBytes]
	}
	kind, mimeType, isBinary := workspaceFileKind(absPath, raw)
	if isBinary || kind == "binary" || kind == "image" {
		return fmt.Sprintf("--- Mentioned file: %s ---\nBinary or image content omitted (%s).\n--- End mentioned file ---", displayPath, firstNonEmpty(mimeType, "application/octet-stream")), nil
	}
	content := string(raw)
	if truncated {
		content += fmt.Sprintf("\n... (truncated to %d bytes)", chatMentionFileMaxBytes)
	}
	return fmt.Sprintf("--- Mentioned file: %s ---\n%s\n--- End mentioned file ---", displayPath, content), nil
}

func readChatMentionDirectoryBlock(displayPath string, absPath string) (string, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", err
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if shouldHideWorkspaceEntry(name) {
			continue
		}
		suffix := ""
		kind := "file"
		if entry.IsDir() {
			suffix = "/"
			kind = "directory"
		}
		items = append(items, fmt.Sprintf("- %s%s (%s)", name, suffix, kind))
		if len(items) >= chatMentionMaxLimit {
			items = append(items, fmt.Sprintf("... (%d entries max)", chatMentionMaxLimit))
			break
		}
	}
	sort.Strings(items)
	if len(items) == 0 {
		items = append(items, "(empty)")
	}
	return fmt.Sprintf("--- Mentioned directory: %s ---\n%s\n--- End mentioned directory ---", displayPath, strings.Join(items, "\n")), nil
}
