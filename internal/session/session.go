package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// SessionToolConfig holds per-session tool/skill/MCP configuration.
// nil slices mean "inherit all from system defaults".
type SessionToolConfig struct {
	ToolsEnabled     []string `json:"tools_enabled,omitempty"`
	ToolsCustom      bool     `json:"tools_custom,omitempty"`
	ToolsDisabled    []string `json:"tools_disabled,omitempty"`
	ToolsAllowGroups []string `json:"tools_allow_groups,omitempty"`
	ToolsDenyGroups  []string `json:"tools_deny_groups,omitempty"`
	SkillsEnabled    []string `json:"skills_enabled,omitempty"`
	SkillsCustom     bool     `json:"skills_custom,omitempty"`
	MCPEnabled       []string `json:"mcp_enabled,omitempty"`
}

type SessionAutomationConsent struct {
	AutoResume             bool       `json:"auto_resume,omitempty"`
	AutoResumeEnabled      bool       `json:"auto_resume_enabled,omitempty"`
	AutoResumeAfterMinutes int        `json:"auto_resume_after_minutes,omitempty"`
	AllowedResumeModes     []string   `json:"allowed_resume_modes,omitempty"`
	GitMutations           bool       `json:"git_mutations,omitempty"`
	AutonomousMutations    bool       `json:"autonomous_mutations,omitempty"`
	UpdatedAt              *time.Time `json:"updated_at,omitempty"`
}

func (c *SessionAutomationConsent) AllowsAutonomousMutation() bool {
	return c != nil && c.AutonomousMutations
}

const (
	DefaultAutoResumeAfterMinutes = 30

	AutoResumeModeProceedWithAssumption      = "proceed_with_assumption"
	AutoResumeModeMoveToNextTask             = "move_to_next_task"
	AutoResumeModeRecordAssumptionAndProceed = "record_assumption_and_proceed"
)

func (c *SessionAutomationConsent) AllowsAutoResume() bool {
	return c != nil && (c.AutoResume || c.AutoResumeEnabled)
}

func (c *SessionAutomationConsent) EffectiveAutoResumeAfterMinutes() int {
	if c == nil || c.AutoResumeAfterMinutes <= 0 {
		return DefaultAutoResumeAfterMinutes
	}
	return c.AutoResumeAfterMinutes
}

func (c *SessionAutomationConsent) EffectiveAllowedResumeModes() []string {
	modes := NormalizeAutoResumeModes(nil)
	if c == nil {
		return modes
	}
	if normalized := NormalizeAutoResumeModes(c.AllowedResumeModes); len(normalized) > 0 {
		return normalized
	}
	return modes
}

func NormalizeAutoResumeModes(values []string) []string {
	allowed := map[string]struct{}{
		AutoResumeModeProceedWithAssumption:      {},
		AutoResumeModeMoveToNextTask:             {},
		AutoResumeModeRecordAssumptionAndProceed: {},
	}
	if len(values) == 0 {
		return []string{AutoResumeModeRecordAssumptionAndProceed}
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := allowed[value]; !ok {
			continue
		}
		if _, dup := seen[value]; dup {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeAutomationConsent(consent *SessionAutomationConsent) *SessionAutomationConsent {
	if consent == nil {
		return nil
	}
	next := *consent
	resumePolicyConfigured := next.AutoResume ||
		next.AutoResumeEnabled ||
		next.AutoResumeAfterMinutes != 0 ||
		len(next.AllowedResumeModes) > 0
	if next.AutoResume || next.AutoResumeEnabled {
		next.AutoResume = true
		next.AutoResumeEnabled = true
	}
	if resumePolicyConfigured {
		if next.AutoResumeAfterMinutes <= 0 {
			next.AutoResumeAfterMinutes = DefaultAutoResumeAfterMinutes
		}
		next.AllowedResumeModes = NormalizeAutoResumeModes(next.AllowedResumeModes)
	}
	return &next
}

type Session struct {
	ID                  string                    `json:"id"`
	Title               string                    `json:"title"`
	Kind                string                    `json:"kind,omitempty"`
	Hidden              bool                      `json:"hidden,omitempty"`
	ParentSessionID     string                    `json:"parent_session_id,omitempty"`
	RootSessionID       string                    `json:"root_session_id,omitempty"`
	ForkedFromMessageID string                    `json:"forked_from_message_id,omitempty"`
	ForkedFromIndex     *int                      `json:"forked_from_index,omitempty"`
	ForkReason          string                    `json:"fork_reason,omitempty"`
	ToolConfig          *SessionToolConfig        `json:"tool_config,omitempty"`
	AutomationConsent   *SessionAutomationConsent `json:"automation_consent,omitempty"`
	LastCompactionMode  string                    `json:"last_compaction_mode,omitempty"`
	PromptOverride      string                    `json:"prompt_override,omitempty"`
	WorkDirs            []string                  `json:"work_dirs,omitempty"`
	CurrentDir          string                    `json:"current_dir,omitempty"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

type Store struct {
	dir string
}

// ForkOptions controls how a child session is created from an existing
// transcript message.
type ForkOptions struct {
	Title  string
	Reason string
}

func NewStore(dir string) *Store {
	return &Store{
		dir: filepath.Join(dir, "sessions"),
	}
}

func canonicalSessionPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	cleaned := filepath.Clean(value)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return cleaned
	}
	return canonicalizePathWithExistingAncestor(abs)
}

func canonicalizePathWithExistingAncestor(absPath string) string {
	current := filepath.Clean(absPath)
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			out := filepath.Clean(resolved)
			for i := len(suffix) - 1; i >= 0; i-- {
				out = filepath.Join(out, suffix[i])
			}
			return filepath.Clean(out)
		}
		if !os.IsNotExist(err) {
			return absPath
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absPath
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func normalizeSessionWorkDirs(requiredDir string, dirs []string, currentDir string) ([]string, string) {
	required := canonicalSessionPath(requiredDir)
	cleanPath := func(value string) string {
		return canonicalSessionPath(value)
	}

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(dirs)+1)
	addDir := func(value string) {
		value = cleanPath(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	if required != "" {
		addDir(required)
	}
	for _, dir := range dirs {
		addDir(dir)
	}

	current := cleanPath(currentDir)
	if current == "" {
		if required != "" {
			current = required
		} else if len(normalized) > 0 {
			current = normalized[0]
		}
	}
	if current != "" {
		found := false
		for _, dir := range normalized {
			if dir == current {
				found = true
				break
			}
		}
		if !found {
			if required != "" {
				current = required
			} else if len(normalized) > 0 {
				current = normalized[0]
			} else {
				current = ""
			}
		}
	}

	return normalized, current
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Store) sessionArtifactDir(id string) string {
	if s == nil {
		return ""
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return filepath.Join(s.WorkspaceDir(), "artifacts", id)
}

func (s *Store) applySessionDefaults(sess Session) (Session, bool, error) {
	var changed bool
	sess, changed = applySessionLineageDefaults(sess)

	artifactDir := s.sessionArtifactDir(sess.ID)
	if artifactDir == "" {
		return sess, changed, nil
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return sess, false, fmt.Errorf("create session artifact dir: %w", err)
	}
	if err := s.migrateLegacyArtifactDir(sess.ID, artifactDir); err != nil {
		return sess, false, err
	}

	workDirs, currentDir := normalizeSessionWorkDirs(artifactDir, sess.WorkDirs, sess.CurrentDir)
	if !sameStringSlice(sess.WorkDirs, workDirs) {
		sess.WorkDirs = workDirs
		changed = true
	}
	if strings.TrimSpace(sess.CurrentDir) != strings.TrimSpace(currentDir) {
		sess.CurrentDir = currentDir
		changed = true
	}
	return sess, changed, nil
}

func applySessionLineageDefaults(sess Session) (Session, bool) {
	changed := false
	trimString := func(value *string) {
		trimmed := strings.TrimSpace(*value)
		if *value != trimmed {
			*value = trimmed
			changed = true
		}
	}
	trimString(&sess.ParentSessionID)
	trimString(&sess.RootSessionID)
	trimString(&sess.ForkedFromMessageID)
	trimString(&sess.ForkReason)
	if sess.ID != "" && sess.RootSessionID == "" {
		sess.RootSessionID = sess.ID
		changed = true
	}
	if sess.AutomationConsent != nil {
		normalized := normalizeAutomationConsent(sess.AutomationConsent)
		if !automationConsentEqual(sess.AutomationConsent, normalized) {
			sess.AutomationConsent = normalized
			changed = true
		}
		if sess.AutomationConsent.UpdatedAt != nil {
			updatedAt := sess.AutomationConsent.UpdatedAt.UTC()
			if !updatedAt.Equal(*sess.AutomationConsent.UpdatedAt) {
				sess.AutomationConsent.UpdatedAt = &updatedAt
				changed = true
			}
		}
	}
	return sess, changed
}

func automationConsentEqual(a, b *SessionAutomationConsent) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.AutoResume == b.AutoResume &&
		a.AutoResumeEnabled == b.AutoResumeEnabled &&
		a.AutoResumeAfterMinutes == b.AutoResumeAfterMinutes &&
		sameStringSlice(a.AllowedResumeModes, b.AllowedResumeModes) &&
		a.GitMutations == b.GitMutations &&
		a.AutonomousMutations == b.AutonomousMutations &&
		((a.UpdatedAt == nil && b.UpdatedAt == nil) ||
			(a.UpdatedAt != nil && b.UpdatedAt != nil && a.UpdatedAt.Equal(*b.UpdatedAt)))
}

func (s *Store) migrateLegacyArtifactDir(id string, artifactDir string) error {
	legacyDir := filepath.Join(s.WorkspaceDir(), "workspace", "artifacts", strings.TrimSpace(id))
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read legacy session artifact dir: %w", err)
	}
	for _, entry := range entries {
		src := filepath.Join(legacyDir, entry.Name())
		dst := filepath.Join(artifactDir, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat migrated session artifact: %w", err)
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("migrate legacy session artifact: %w", err)
		}
	}
	_ = os.Remove(legacyDir)
	return nil
}

func (s *Store) Create(title string) (Session, error) {
	return s.CreateWithOptions(title, "", false)
}

func (s *Store) CreateWithOptions(title string, kind string, hidden bool) (Session, error) {
	trimmedKind := strings.TrimSpace(kind)

	// Enforce main session uniqueness: use EnsureMain() instead.
	if trimmedKind == "main" {
		return Session{}, fmt.Errorf("cannot create main session directly; use EnsureMain()")
	}

	now := time.Now().UTC()
	session := Session{
		Title:     title,
		Kind:      trimmedKind,
		Hidden:    hidden,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions directory: %w", err)
	}

	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}

	for {
		id, err := generateID()
		if err != nil {
			return Session{}, err
		}
		if _, exists := index[id]; exists {
			continue
		}
		session.ID = id
		break
	}

	session, _, err = s.applySessionDefaults(session)
	if err != nil {
		return Session{}, err
	}

	index[session.ID] = session

	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}

	return session, nil
}

// ForkFromMessage creates a new visible session whose transcript contains the
// parent transcript prefix through the selected message.
func (s *Store) ForkFromMessage(parentID string, messageID string, opts ForkOptions) (Session, error) {
	parentID = strings.TrimSpace(parentID)
	messageID = strings.TrimSpace(messageID)
	if parentID == "" {
		return Session{}, fmt.Errorf("parent session id is required")
	}
	if messageID == "" {
		return Session{}, fmt.Errorf("message id is required")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions directory: %w", err)
	}

	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	parent, ok := index[parentID]
	if !ok {
		return Session{}, fmt.Errorf("session not found")
	}
	parent, changed, err := s.applySessionDefaults(parent)
	if err != nil {
		return Session{}, err
	}
	if changed {
		index[parentID] = parent
	}

	messages, err := ReadMessages(s.TranscriptPath(parentID))
	if err != nil {
		return Session{}, fmt.Errorf("read parent transcript: %w", err)
	}
	forkIndex := -1
	for i, msg := range messages {
		if strings.TrimSpace(msg.ID) == messageID {
			forkIndex = i
			break
		}
	}
	if forkIndex < 0 {
		return Session{}, fmt.Errorf("message not found")
	}
	prefix := append([]Message(nil), messages[:forkIndex+1]...)

	now := time.Now().UTC()
	rootID := strings.TrimSpace(parent.RootSessionID)
	if rootID == "" {
		rootID = parent.ID
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = suggestedForkTitle(parent.Title, messages[forkIndex])
	}
	child := Session{
		Title:               title,
		ParentSessionID:     parent.ID,
		RootSessionID:       rootID,
		ForkedFromMessageID: messageID,
		ForkedFromIndex:     intPtr(forkIndex),
		ForkReason:          strings.TrimSpace(opts.Reason),
		ToolConfig:          cloneSessionToolConfig(parent.ToolConfig),
		LastCompactionMode:  parent.LastCompactionMode,
		PromptOverride:      parent.PromptOverride,
		WorkDirs:            forkWorkDirs(parent, s.sessionArtifactDir(parent.ID)),
		CurrentDir:          forkCurrentDir(parent, s.sessionArtifactDir(parent.ID)),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	for {
		id, err := generateID()
		if err != nil {
			return Session{}, err
		}
		if _, exists := index[id]; exists {
			continue
		}
		child.ID = id
		break
	}
	child, _, err = s.applySessionDefaults(child)
	if err != nil {
		return Session{}, err
	}

	if err := RewriteMessages(s.TranscriptPath(child.ID), prefix); err != nil {
		return Session{}, fmt.Errorf("write child transcript: %w", err)
	}
	parentTasks, err := s.GetTasks(parent.ID)
	if err != nil {
		_ = os.Remove(s.TranscriptPath(child.ID))
		return Session{}, fmt.Errorf("read parent tasks: %w", err)
	}
	if hasSessionTaskState(parentTasks) {
		if err := s.SaveTasks(child.ID, parentTasks); err != nil {
			_ = os.Remove(s.TranscriptPath(child.ID))
			return Session{}, fmt.Errorf("write child tasks: %w", err)
		}
	}

	index[child.ID] = child
	if err := s.saveIndex(index); err != nil {
		_ = os.Remove(s.TranscriptPath(child.ID))
		_ = os.Remove(s.tasksPath(child.ID))
		return Session{}, err
	}
	return child, nil
}

func (s *Store) EnsureMain() (Session, error) {
	// Deduplicate any stale main sessions before ensuring
	s.deduplicateMain()
	return s.ensureNamedSession("main", "main", false)
}

// deduplicateMain removes duplicate main sessions, keeping only the oldest.
func (s *Store) deduplicateMain() {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return
	}
	var mains []Session
	for _, sess := range index {
		if strings.TrimSpace(sess.Kind) == "main" {
			mains = append(mains, sess)
		}
	}
	if len(mains) <= 1 {
		return
	}
	// Keep the oldest main session, remove the rest
	oldest := mains[0]
	for _, m := range mains[1:] {
		if m.CreatedAt.Before(oldest.CreatedAt) {
			oldest = m
		}
	}
	changed := false
	for _, m := range mains {
		if m.ID != oldest.ID {
			delete(index, m.ID)
			changed = true
		}
	}
	if changed {
		_ = s.saveIndex(index)
	}
}

func (s *Store) EnsureWorker(projectID string) (Session, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return Session{}, fmt.Errorf("project id is required")
	}
	return s.ensureNamedSession("worker:"+id, "worker", true)
}

func (s *Store) ensureNamedSession(title string, kind string, hidden bool) (Session, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions directory: %w", err)
	}
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	trimmedTitle := strings.TrimSpace(title)
	trimmedKind := strings.TrimSpace(kind)
	for id, sess := range index {
		sessKind := strings.TrimSpace(sess.Kind)
		// For unique-kind sessions (main, worker), match by kind only — title may have been renamed.
		// For regular sessions, match by both kind and title.
		kindMatch := sessKind == trimmedKind
		titleMatch := trimmedKind == "main" || strings.TrimSpace(sess.Title) == trimmedTitle
		if kindMatch && titleMatch {
			sess.Hidden = hidden
			if sess.CreatedAt.IsZero() {
				sess.CreatedAt = time.Now().UTC()
			}
			if sess.UpdatedAt.IsZero() {
				sess.UpdatedAt = sess.CreatedAt
			}
			sess, _, err = s.applySessionDefaults(sess)
			if err != nil {
				return Session{}, err
			}
			index[id] = sess
			if err := s.saveIndex(index); err != nil {
				return Session{}, err
			}
			return sess, nil
		}
	}
	now := time.Now().UTC()
	created := Session{
		Title:     trimmedTitle,
		Kind:      trimmedKind,
		Hidden:    hidden,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for {
		id, err := generateID()
		if err != nil {
			return Session{}, err
		}
		if _, exists := index[id]; exists {
			continue
		}
		created.ID = id
		break
	}
	created, _, err = s.applySessionDefaults(created)
	if err != nil {
		return Session{}, err
	}
	index[created.ID] = created
	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}
	return created, nil
}

func (s *Store) Get(id string) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}

	session, ok := index[id]
	if !ok {
		return Session{}, fmt.Errorf("session not found")
	}
	session, changed, err := s.applySessionDefaults(session)
	if err != nil {
		return Session{}, err
	}
	if changed {
		index[id] = session
		if err := s.saveIndex(index); err != nil {
			return Session{}, err
		}
	}

	return session, nil
}

func (s *Store) List() ([]Session, error) {
	return s.list(false)
}

func (s *Store) ListAll() ([]Session, error) {
	return s.list(true)
}

func (s *Store) list(includeHidden bool) ([]Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	sessions := make([]Session, 0, len(index))
	changed := false
	for id, session := range index {
		var sessionChanged bool
		session, sessionChanged = applySessionLineageDefaults(session)
		if sessionChanged {
			index[id] = session
			changed = true
		}
		if session.Hidden && !includeHidden {
			continue
		}
		sessions = append(sessions, session)
	}
	if changed {
		if err := s.saveIndex(index); err != nil {
			return nil, err
		}
	}

	return sessions, nil
}

func (s *Store) Touch(id string, updatedAt time.Time) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.UpdatedAt = updatedAt.UTC()
	index[id] = sess
	return s.saveIndex(index)
}

// SetTitle renames a session.
func (s *Store) SetTitle(id string, title string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.Title = strings.TrimSpace(title)
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

// SetToolConfig updates the per-session tool configuration.
func (s *Store) SetToolConfig(id string, config *SessionToolConfig) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.ToolConfig = config
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

// SetAutomationConsent updates the per-session automation consent policy.
func (s *Store) SetAutomationConsent(id string, consent *SessionAutomationConsent) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	now := time.Now().UTC()
	if consent == nil {
		sess.AutomationConsent = nil
	} else {
		next := *consent
		next.UpdatedAt = &now
		sess.AutomationConsent = normalizeAutomationConsent(&next)
	}
	sess.UpdatedAt = now
	index[id] = sess
	return s.saveIndex(index)
}

// SetPromptOverride updates the per-session prompt override.
func (s *Store) SetPromptOverride(id string, override string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.PromptOverride = override
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

func (s *Store) SetLastCompactionMode(id string, mode string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.LastCompactionMode = strings.TrimSpace(mode)
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

// SetWorkDirs updates the per-session working directories and current directory.
func (s *Store) SetWorkDirs(id string, dirs []string, currentDir string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.WorkDirs = append([]string(nil), dirs...)
	sess.CurrentDir = currentDir
	sess, _, err = s.applySessionDefaults(sess)
	if err != nil {
		return err
	}
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

// SetCurrentDir updates only the current working directory for a session.
func (s *Store) SetCurrentDir(id string, dir string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess, _, err = s.applySessionDefaults(sess)
	if err != nil {
		return err
	}
	cd := canonicalSessionPath(dir)
	if cd != "" && len(sess.WorkDirs) > 0 {
		found := false
		for _, d := range sess.WorkDirs {
			if d == cd {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("directory not in work_dirs")
		}
	}
	sess.CurrentDir = cd
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

func (s *Store) Latest() (Session, error) {
	return s.latest(false)
}

func (s *Store) LatestAll() (Session, error) {
	return s.latest(true)
}

func (s *Store) latest(includeHidden bool) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	var latest Session
	hasLatest := false
	for _, sess := range index {
		if sess.Hidden && !includeHidden {
			continue
		}
		if !hasLatest {
			latest = sess
			hasLatest = true
			continue
		}
		switch {
		case sess.UpdatedAt.After(latest.UpdatedAt):
			latest = sess
		case sess.UpdatedAt.Equal(latest.UpdatedAt) && sess.CreatedAt.After(latest.CreatedAt):
			latest = sess
		}
	}
	if !hasLatest {
		return Session{}, fmt.Errorf("session not found")
	}
	return latest, nil
}

func (s *Store) Delete(id string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}

	if _, ok := index[id]; !ok {
		return nil
	}

	delete(index, id)
	if err := s.saveIndex(index); err != nil {
		return err
	}

	_ = os.Remove(s.TranscriptPath(id))

	return nil
}

func (s *Store) TranscriptPath(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}

func (s *Store) indexPath() string {
	return filepath.Join(s.dir, "sessions.json")
}

func (s *Store) WorkspaceDir() string {
	if s == nil {
		return ""
	}
	return filepath.Dir(s.dir)
}

func (s *Store) loadIndex() (map[string]Session, error) {
	raw, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Session{}, nil
		}
		return nil, err
	}

	index := make(map[string]Session)
	if len(raw) == 0 {
		return index, nil
	}

	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, fmt.Errorf("load sessions index: %w", err)
	}

	return index, nil
}

func (s *Store) saveIndex(index map[string]Session) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.Write(s.indexPath(), data)
}

func intPtr(value int) *int {
	return &value
}

func cloneSessionToolConfig(config *SessionToolConfig) *SessionToolConfig {
	if config == nil {
		return nil
	}
	return &SessionToolConfig{
		ToolsEnabled:     append([]string(nil), config.ToolsEnabled...),
		ToolsCustom:      config.ToolsCustom,
		ToolsDisabled:    append([]string(nil), config.ToolsDisabled...),
		ToolsAllowGroups: append([]string(nil), config.ToolsAllowGroups...),
		ToolsDenyGroups:  append([]string(nil), config.ToolsDenyGroups...),
		SkillsEnabled:    append([]string(nil), config.SkillsEnabled...),
		SkillsCustom:     config.SkillsCustom,
		MCPEnabled:       append([]string(nil), config.MCPEnabled...),
	}
}

func forkWorkDirs(parent Session, parentArtifactDir string) []string {
	parentArtifactDir = canonicalSessionPath(parentArtifactDir)
	dirs := make([]string, 0, len(parent.WorkDirs))
	for _, dir := range parent.WorkDirs {
		if parentArtifactDir != "" && canonicalSessionPath(dir) == parentArtifactDir {
			continue
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

func forkCurrentDir(parent Session, parentArtifactDir string) string {
	parentArtifactDir = canonicalSessionPath(parentArtifactDir)
	if parentArtifactDir != "" && canonicalSessionPath(parent.CurrentDir) == parentArtifactDir {
		return ""
	}
	return parent.CurrentDir
}

func hasSessionTaskState(tasks SessionTasks) bool {
	return tasks.Plan != nil || tasks.Contract != nil || len(tasks.Tasks) > 0
}

func suggestedForkTitle(parentTitle string, msg Message) string {
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		text = strings.TrimSpace(parentTitle)
	}
	if text == "" {
		text = "session"
	}
	text = strings.Join(strings.Fields(text), " ")
	const maxLen = 56
	if len(text) > maxLen {
		text = strings.TrimSpace(text[:maxLen-1]) + "..."
	}
	return "Fork: " + text
}

func generateID() (string, error) {
	raw := make([]byte, 8)
	n, err := rand.Read(raw)
	if err != nil {
		return "", err
	}
	if n != len(raw) {
		return "", fmt.Errorf("failed to generate random id")
	}
	return hex.EncodeToString(raw), nil
}
