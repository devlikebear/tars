package tarsserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// "Always allow in this folder" rules (#970).
//
// A rule widens what a tool call may do without asking, so it lives where
// only TARS writes: <workspace>/_shared/permissions/always-allow.json, keyed
// by the project folder. The .tars settings files a repository can carry
// stay deny-only; reading allow rules from them would let a cloned
// repository grant itself tools.

const (
	// chatRuleProviderClaudeCode rules are Claude Code permission rules,
	// handed to the CLI through --settings.
	chatRuleProviderClaudeCode = "claude-code"
	// chatRuleProviderTARS rules cover native providers' TARS tools and are
	// checked by chatToolGate.
	chatRuleProviderTARS = "tars"
)

type chatAlwaysRule struct {
	Provider string `json:"provider"`
	Tool     string `json:"tool"`
	// Content narrows the rule, e.g. "npm test:*" for Bash or exec.
	Content   string    `json:"content,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Display is the rule in Claude Code's notation: Tool or Tool(content).
func (r chatAlwaysRule) Display() string {
	if r.Content == "" {
		return r.Tool
	}
	return r.Tool + "(" + r.Content + ")"
}

func (r chatAlwaysRule) same(other chatAlwaysRule) bool {
	return r.Provider == other.Provider && r.Tool == other.Tool && r.Content == other.Content
}

type chatAlwaysRuleFile struct {
	Version int                         `json:"version"`
	Dirs    map[string][]chatAlwaysRule `json:"dirs"`
}

type chatAlwaysRuleStore struct {
	path string
	mu   sync.Mutex
}

func newChatAlwaysRuleStore(workspaceDir string) *chatAlwaysRuleStore {
	return &chatAlwaysRuleStore{path: filepath.Join(workspaceDir, "_shared", "permissions", "always-allow.json")}
}

// chatAlwaysRuleDir is the key for a folder: absolute, cleaned, with
// symlinks resolved when the folder exists, so every path to it agrees.
func chatAlwaysRuleDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return filepath.Clean(dir)
}

// load reads the file; a missing or unreadable one is empty, so a damaged
// store fails toward asking again. Callers hold mu.
func (s *chatAlwaysRuleStore) load() chatAlwaysRuleFile {
	file := chatAlwaysRuleFile{Version: 1, Dirs: map[string][]chatAlwaysRule{}}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return file
	}
	if json.Unmarshal(data, &file) != nil || file.Dirs == nil {
		return chatAlwaysRuleFile{Version: 1, Dirs: map[string][]chatAlwaysRule{}}
	}
	return file
}

func (s *chatAlwaysRuleStore) save(file chatAlwaysRuleFile) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.Write(s.path, append(data, '\n'))
}

func (s *chatAlwaysRuleStore) list(dir string) []chatAlwaysRule {
	key := chatAlwaysRuleDir(dir)
	if key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]chatAlwaysRule(nil), s.load().Dirs[key]...)
}

func (s *chatAlwaysRuleStore) add(dir string, rule chatAlwaysRule) error {
	key := chatAlwaysRuleDir(dir)
	if key == "" {
		return errors.New("an always-allow rule needs a folder")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file := s.load()
	for _, existing := range file.Dirs[key] {
		if existing.same(rule) {
			return nil
		}
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	file.Dirs[key] = append(file.Dirs[key], rule)
	return s.save(file)
}

// remove deletes the provider's rule with that display text from dir and
// reports whether it existed.
func (s *chatAlwaysRuleStore) remove(dir, provider, display string) (bool, error) {
	key := chatAlwaysRuleDir(dir)
	s.mu.Lock()
	defer s.mu.Unlock()
	file := s.load()
	rules := file.Dirs[key]
	kept := rules[:0:0]
	for _, rule := range rules {
		if rule.Provider == provider && rule.Display() == display {
			continue
		}
		kept = append(kept, rule)
	}
	if len(kept) == len(rules) {
		return false, nil
	}
	if len(kept) == 0 {
		delete(file.Dirs, key)
	} else {
		file.Dirs[key] = kept
	}
	return true, s.save(file)
}

// handleChatPermissionRules serves /v1/chat/permission-rules?dir=<folder>:
// GET lists the folder's always-allow rules; DELETE with provider and rule
// removes one. Rules are only ever added by answering a prompt.
func handleChatPermissionRules(w http.ResponseWriter, r *http.Request, store *chatAlwaysRuleStore) {
	dir := strings.TrimSpace(r.URL.Query().Get("dir"))
	if dir == "" {
		writeError(w, http.StatusBadRequest, "", "dir is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		type ruleView struct {
			Provider  string    `json:"provider"`
			Rule      string    `json:"rule"`
			CreatedAt time.Time `json:"created_at"`
		}
		views := []ruleView{}
		for _, rule := range store.list(dir) {
			views = append(views, ruleView{Provider: rule.Provider, Rule: rule.Display(), CreatedAt: rule.CreatedAt})
		}
		writeJSON(w, http.StatusOK, map[string]any{"dir": chatAlwaysRuleDir(dir), "rules": views})
	case http.MethodDelete:
		provider := strings.TrimSpace(r.URL.Query().Get("provider"))
		rule := strings.TrimSpace(r.URL.Query().Get("rule"))
		removed, err := store.remove(dir, provider, rule)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "could not update the rules")
			return
		}
		if !removed {
			writeError(w, http.StatusNotFound, "", "no such rule for this folder")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
	default:
		writeMethodNotAllowed(w)
	}
}
