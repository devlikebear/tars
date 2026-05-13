package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// ErrCwdNotEligible is returned by SetCurrentDir when the supplied directory
// is not present in the session's normalized work_dirs (i.e. neither the
// artifact dir nor any user-registered work_dir).
var ErrCwdNotEligible = errors.New("session: cwd not in eligible work_dirs")

// ErrSessionNotFound is returned when a session ID does not resolve to an
// entry in the index. The message is intentionally kept stable for callers
// that match on the substring "session not found".
var ErrSessionNotFound = errors.New("session not found")

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
	CommandsEnabled  []string `json:"commands_enabled,omitempty"`
	CommandsCustom   bool     `json:"commands_custom,omitempty"`
	MCPEnabled       []string `json:"mcp_enabled,omitempty"`
	MCPCustom        bool     `json:"mcp_custom,omitempty"`
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

type SessionStyleControl struct {
	Directness *int       `json:"directness,omitempty"`
	Humor      *int       `json:"humor,omitempty"`
	Caution    *int       `json:"caution,omitempty"`
	Autonomy   *int       `json:"autonomy,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

// Session goal status values.
const (
	SessionGoalStatusActive    = "active"
	SessionGoalStatusSatisfied = "satisfied"
	SessionGoalStatusExhausted = "exhausted"

	DefaultGoalMaxAutoContinues = 3
	MaxGoalMaxAutoContinues     = 20
	MaxGoalDescriptionLen       = 2000
)

// SessionGoal captures a single active goal for a chat session. When set, the
// chat handler appends the goal to the system prompt and runs an independent
// judge LLM after each turn; if the judge says "not satisfied" the loop may
// auto-continue up to MaxAutoContinues times.
type SessionGoal struct {
	Description       string     `json:"description"`
	CreatedAt         time.Time  `json:"created_at"`
	MaxAutoContinues  int        `json:"max_auto_continues"`
	AutoContinueCount int        `json:"auto_continue_count"`
	LastJudgedAt      *time.Time `json:"last_judged_at,omitempty"`
	Status            string     `json:"status"`
}

// NormalizeGoal trims and clamps fields, defaulting status/max where unset.
// Returns nil when the description is empty (treated as "no goal").
func NormalizeGoal(goal *SessionGoal) *SessionGoal {
	if goal == nil {
		return nil
	}
	desc := strings.TrimSpace(goal.Description)
	if desc == "" {
		return nil
	}
	if len(desc) > MaxGoalDescriptionLen {
		desc = desc[:MaxGoalDescriptionLen]
	}
	next := *goal
	next.Description = desc
	if next.MaxAutoContinues <= 0 {
		next.MaxAutoContinues = DefaultGoalMaxAutoContinues
	}
	if next.MaxAutoContinues > MaxGoalMaxAutoContinues {
		next.MaxAutoContinues = MaxGoalMaxAutoContinues
	}
	if next.AutoContinueCount < 0 {
		next.AutoContinueCount = 0
	}
	switch strings.TrimSpace(next.Status) {
	case SessionGoalStatusActive, SessionGoalStatusSatisfied, SessionGoalStatusExhausted:
		next.Status = strings.TrimSpace(next.Status)
	default:
		next.Status = SessionGoalStatusActive
	}
	return &next
}

// IsActive reports whether the goal is in the active state and should be
// surfaced to the LLM / agent loop.
func (g *SessionGoal) IsActive() bool {
	return g != nil && g.Status == SessionGoalStatusActive
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

func NormalizeStyleControl(style *SessionStyleControl) *SessionStyleControl {
	if style == nil {
		return nil
	}
	next := *style
	normalizeScore := func(value **int) {
		if *value == nil {
			return
		}
		clamped := clampStyleScore(**value)
		*value = &clamped
	}
	normalizeScore(&next.Directness)
	normalizeScore(&next.Humor)
	normalizeScore(&next.Caution)
	normalizeScore(&next.Autonomy)
	return &next
}

func clampStyleScore(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
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
	StyleControl        *SessionStyleControl      `json:"style_control,omitempty"`
	LastCompactionMode  string                    `json:"last_compaction_mode,omitempty"`
	PromptOverride      string                    `json:"prompt_override,omitempty"`
	WorkDirs            []string                  `json:"work_dirs,omitempty"`
	CurrentDir          string                    `json:"current_dir,omitempty"`
	ArchivedAt          *time.Time                `json:"archived_at,omitempty"`
	PinnedAt            *time.Time                `json:"pinned_at,omitempty"`
	Goal                *SessionGoal              `json:"goal,omitempty"`
	Critic              *SessionCritic            `json:"critic,omitempty"`
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
	if sess.StyleControl != nil {
		normalized := NormalizeStyleControl(sess.StyleControl)
		if !styleControlEqual(sess.StyleControl, normalized) {
			sess.StyleControl = normalized
			changed = true
		}
		if sess.StyleControl.UpdatedAt != nil {
			updatedAt := sess.StyleControl.UpdatedAt.UTC()
			if !updatedAt.Equal(*sess.StyleControl.UpdatedAt) {
				sess.StyleControl.UpdatedAt = &updatedAt
				changed = true
			}
		}
	}
	if sess.Critic != nil {
		normalized := NormalizeCritic(sess.Critic)
		if !criticEqual(sess.Critic, normalized) {
			sess.Critic = normalized
			changed = true
		}
		if sess.Critic != nil && sess.Critic.UpdatedAt != nil {
			updatedAt := sess.Critic.UpdatedAt.UTC()
			if !updatedAt.Equal(*sess.Critic.UpdatedAt) {
				sess.Critic.UpdatedAt = &updatedAt
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

func styleControlEqual(a, b *SessionStyleControl) bool {
	if a == nil || b == nil {
		return a == b
	}
	return intPtrEqual(a.Directness, b.Directness) &&
		intPtrEqual(a.Humor, b.Humor) &&
		intPtrEqual(a.Caution, b.Caution) &&
		intPtrEqual(a.Autonomy, b.Autonomy) &&
		((a.UpdatedAt == nil && b.UpdatedAt == nil) ||
			(a.UpdatedAt != nil && b.UpdatedAt != nil && a.UpdatedAt.Equal(*b.UpdatedAt)))
}

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
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
		StyleControl:        cloneSessionStyleControl(parent.StyleControl),
		Critic:              InheritCriticConfig(parent.Critic),
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
	// Worker sessions inherit the main session's critic config so a user who
	// toggles critic once in main automatically gets reviews on background
	// worker turns too. Runtime state is reset by InheritCriticConfig.
	if trimmedKind != "main" {
		for _, sess := range index {
			if strings.TrimSpace(sess.Kind) == "main" {
				created.Critic = InheritCriticConfig(sess.Critic)
				break
			}
		}
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
		return Session{}, ErrSessionNotFound
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

func (s *Store) SetArchived(id string, archived bool) (Session, error) {
	return s.updateOrganization(id, func(sess *Session, now time.Time) {
		if archived {
			archivedAt := now
			sess.ArchivedAt = &archivedAt
			sess.PinnedAt = nil
		} else {
			sess.ArchivedAt = nil
		}
	})
}

func (s *Store) SetPinned(id string, pinned bool) (Session, error) {
	return s.updateOrganization(id, func(sess *Session, now time.Time) {
		if pinned {
			pinnedAt := now
			sess.PinnedAt = &pinnedAt
			sess.ArchivedAt = nil
		} else {
			sess.PinnedAt = nil
		}
	})
}

func (s *Store) updateOrganization(id string, apply func(*Session, time.Time)) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	sess, ok := index[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	now := time.Now().UTC()
	apply(&sess, now)
	sess.UpdatedAt = now
	index[id] = sess
	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}
	return sess, nil
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

// SetStyleControl updates the per-session behavioral style override.
func (s *Store) SetStyleControl(id string, style *SessionStyleControl) error {
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
	if style == nil {
		sess.StyleControl = nil
	} else {
		next := *style
		next.UpdatedAt = &now
		sess.StyleControl = NormalizeStyleControl(&next)
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

// ErrSessionKindUnsupported is returned by goal mutations when the session
// kind does not permit a goal (currently only "main" kind sessions support
// goals).
var ErrSessionKindUnsupported = errors.New("session: kind does not support goals")

// SetGoal replaces the session's active goal. Only "main" sessions are
// permitted. Passing nil or a goal with empty description clears it.
func (s *Store) SetGoal(id string, goal *SessionGoal) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	sess, ok := index[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if strings.TrimSpace(sess.Kind) != "main" {
		return Session{}, ErrSessionKindUnsupported
	}
	now := time.Now().UTC()
	normalized := NormalizeGoal(goal)
	if normalized == nil {
		sess.Goal = nil
	} else {
		if normalized.CreatedAt.IsZero() {
			normalized.CreatedAt = now
		}
		sess.Goal = normalized
	}
	sess.UpdatedAt = now
	index[id] = sess
	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}
	return sess, nil
}

// ClearGoal removes the session's goal regardless of status. Safe to call when
// no goal is set.
func (s *Store) ClearGoal(id string) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	sess, ok := index[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if sess.Goal != nil {
		sess.Goal = nil
		sess.UpdatedAt = time.Now().UTC()
		index[id] = sess
		if err := s.saveIndex(index); err != nil {
			return Session{}, err
		}
	}
	return sess, nil
}

// SetCritic replaces the session's critic configuration. Any session kind is
// permitted — worker/subagent sessions still benefit from review on the
// assistant_turn trigger even when no user-visible plan exists. A nil critic
// clears the configuration entirely.
func (s *Store) SetCritic(id string, critic *SessionCritic) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	sess, ok := index[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	now := time.Now().UTC()
	if critic == nil {
		sess.Critic = nil
	} else {
		next := *critic
		next.UpdatedAt = &now
		sess.Critic = NormalizeCritic(&next)
	}
	sess.UpdatedAt = now
	index[id] = sess
	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}
	return sess, nil
}

// UpdateCriticProgress applies a mutation to the session's critic state. The
// mutator may return nil to clear runtime state (typically only useful in
// tests). A no-op when no critic config is present.
func (s *Store) UpdateCriticProgress(id string, mutate func(*SessionCritic) *SessionCritic) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	sess, ok := index[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if sess.Critic == nil {
		return sess, nil
	}
	current := *sess.Critic
	next := mutate(&current)
	now := time.Now().UTC()
	if next == nil {
		sess.Critic = nil
	} else {
		next.UpdatedAt = &now
		sess.Critic = NormalizeCritic(next)
	}
	sess.UpdatedAt = now
	index[id] = sess
	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}
	return sess, nil
}

// UpdateGoalProgress applies a mutation to the session's goal (e.g. to bump
// AutoContinueCount or change Status). If the mutator returns nil the goal is
// cleared. If no goal is present the call is a no-op.
func (s *Store) UpdateGoalProgress(id string, mutate func(*SessionGoal) *SessionGoal) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	sess, ok := index[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if sess.Goal == nil {
		return sess, nil
	}
	current := *sess.Goal
	next := mutate(&current)
	now := time.Now().UTC()
	if next == nil {
		sess.Goal = nil
	} else {
		sess.Goal = NormalizeGoal(next)
	}
	sess.UpdatedAt = now
	index[id] = sess
	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}
	return sess, nil
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
		return ErrSessionNotFound
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
			return fmt.Errorf("%w: %s", ErrCwdNotEligible, cd)
		}
	}
	sess.CurrentDir = cd
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

// EligibleCwds returns the canonical list of directories the session may use
// as its current working directory. The result always contains the session's
// artifact dir (as element 0) followed by any user-registered work_dirs in
// insertion order, deduplicated. The slice is a defensive copy.
func (s *Store) EligibleCwds(id string) ([]string, error) {
	if s == nil {
		return nil, ErrSessionNotFound
	}
	sess, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(sess.WorkDirs))
	copy(out, sess.WorkDirs)
	return out, nil
}

// GetCurrentDir returns the session's active cwd, falling back to the
// artifact dir when no explicit current_dir is set.
func (s *Store) GetCurrentDir(id string) (string, error) {
	if s == nil {
		return "", ErrSessionNotFound
	}
	sess, err := s.Get(id)
	if err != nil {
		return "", err
	}
	if cur := strings.TrimSpace(sess.CurrentDir); cur != "" {
		return cur, nil
	}
	if len(sess.WorkDirs) > 0 {
		return sess.WorkDirs[0], nil
	}
	return canonicalSessionPath(s.sessionArtifactDir(id)), nil
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
		CommandsEnabled:  append([]string(nil), config.CommandsEnabled...),
		CommandsCustom:   config.CommandsCustom,
		MCPEnabled:       append([]string(nil), config.MCPEnabled...),
		MCPCustom:        config.MCPCustom,
	}
}

func cloneSessionStyleControl(style *SessionStyleControl) *SessionStyleControl {
	if style == nil {
		return nil
	}
	next := *style
	cloneInt := func(value *int) *int {
		if value == nil {
			return nil
		}
		cloned := *value
		return &cloned
	}
	next.Directness = cloneInt(style.Directness)
	next.Humor = cloneInt(style.Humor)
	next.Caution = cloneInt(style.Caution)
	next.Autonomy = cloneInt(style.Autonomy)
	if style.UpdatedAt != nil {
		updatedAt := *style.UpdatedAt
		next.UpdatedAt = &updatedAt
	}
	return &next
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
