package tarsserver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

const (
	defaultWatchdogInterval       = 5 * time.Minute
	watchdogProjectStaleThreshold = 5 * time.Minute
)

var watchdogContaminationMarkers = []string{
	"assistant to=functions.",
	"need maybe ",
	`"command":"`,
	`{"command":`,
}

type watchdogFinding struct {
	Kind      string `json:"kind"`
	Severity  string `json:"severity"`
	JobID     string `json:"job_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message"`
}

type watchdogRunResult struct {
	Healthy       bool              `json:"healthy"`
	InspectedJobs int               `json:"inspected_jobs"`
	Summary       string            `json:"summary,omitempty"`
	Findings      []watchdogFinding `json:"findings,omitempty"`
}

type watchdogStatus struct {
	LastRunAt string            `json:"last_run_at,omitempty"`
	LastError string            `json:"last_error,omitempty"`
	Healthy   bool              `json:"healthy"`
	Summary   string            `json:"summary,omitempty"`
	Findings  []watchdogFinding `json:"findings,omitempty"`
}

type watchdogRuntimeState struct {
	mu        sync.RWMutex
	hasRun    bool
	lastRunAt time.Time
	lastErr   string
	last      watchdogRunResult
}

type watchdogWorkspaceState struct {
	mu    sync.RWMutex
	items map[string]*watchdogRuntimeState
}

func newWatchdogWorkspaceState() *watchdogWorkspaceState {
	return &watchdogWorkspaceState{items: map[string]*watchdogRuntimeState{}}
}

func (s *watchdogWorkspaceState) getOrCreate(workspaceID string) *watchdogRuntimeState {
	if s == nil {
		return nil
	}
	id := normalizeWorkspaceID(workspaceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]*watchdogRuntimeState{}
	}
	if existing := s.items[id]; existing != nil {
		return existing
	}
	created := &watchdogRuntimeState{}
	s.items[id] = created
	return created
}

func (s *watchdogWorkspaceState) get(workspaceID string) *watchdogRuntimeState {
	if s == nil {
		return nil
	}
	id := normalizeWorkspaceID(workspaceID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items[id]
}

func (s *watchdogWorkspaceState) record(workspaceID string, ranAt time.Time, result watchdogRunResult, runErr error) {
	state := s.getOrCreate(workspaceID)
	if state == nil {
		return
	}
	state.record(ranAt, result, runErr)
}

func (s *watchdogWorkspaceState) snapshot(workspaceID string) watchdogStatus {
	state := s.get(workspaceID)
	if state == nil {
		return watchdogStatus{Healthy: true}
	}
	return state.snapshot()
}

func (s *watchdogRuntimeState) record(ranAt time.Time, result watchdogRunResult, runErr error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasRun = true
	s.lastRunAt = ranAt.UTC()
	s.last = result
	if runErr != nil {
		s.lastErr = strings.TrimSpace(runErr.Error())
	} else {
		s.lastErr = ""
	}
}

func (s *watchdogRuntimeState) snapshot() watchdogStatus {
	if s == nil {
		return watchdogStatus{Healthy: true}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := watchdogStatus{
		Healthy:  true,
		Findings: nil,
	}
	if !s.hasRun {
		return status
	}
	status.LastRunAt = s.lastRunAt.Format(time.RFC3339)
	status.LastError = s.lastErr
	status.Healthy = s.last.Healthy
	status.Summary = s.last.Summary
	status.Findings = slices.Clone(s.last.Findings)
	return status
}

func newWorkspaceWatchdogRunnerWithNotify(
	baseWorkspaceDir string,
	cronResolver *workspaceCronStoreResolver,
	nowFn func() time.Time,
	state *watchdogWorkspaceState,
	emit func(ctx context.Context, evt notificationEvent),
) func(ctx context.Context) (watchdogRunResult, error) {
	runner := newSerializedSupervisorRunner(serializedSupervisorOptions[watchdogRunResult]{
		nowFn:   nowFn,
		timeout: 30 * time.Second,
		run: func(ctx context.Context, _ time.Time) (watchdogRunResult, error) {
			workspaceID := defaultWorkspaceID
			workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, workspaceID)
			if err := memory.EnsureWorkspace(workspaceDir); err != nil {
				return watchdogRunResult{}, err
			}
			store, err := resolveWatchdogCronStore(cronResolver, workspaceDir)
			if err != nil {
				return watchdogRunResult{}, err
			}
			return runWorkspaceWatchdog(ctx, workspaceDir, store)
		},
		record: func(ranAt time.Time, result watchdogRunResult, runErr error) {
			if state != nil {
				state.record(defaultWorkspaceID, ranAt, result, runErr)
			}
		},
		emit: func(ctx context.Context, result watchdogRunResult, runErr error) {
			if emit == nil {
				return
			}
			switch {
			case runErr != nil:
				emit(ctx, newNotificationEvent("watchdog", "error", "Watchdog failed", trimForMemory(runErr.Error(), 240)))
			case result.Healthy:
				return
			default:
				evt := newNotificationEvent("watchdog", "warn", "Watchdog detected unhealthy background work", trimForMemory(result.Summary, 280))
				emit(ctx, evt)
			}
		},
	})
	return runner
}

func resolveWatchdogCronStore(cronResolver *workspaceCronStoreResolver, workspaceDir string) (*cron.Store, error) {
	if cronResolver != nil {
		return cronResolver.Resolve(defaultWorkspaceID)
	}
	return cron.NewStore(workspaceDir), nil
}

func runWorkspaceWatchdog(_ context.Context, workspaceDir string, cronStore *cron.Store) (watchdogRunResult, error) {
	if cronStore == nil {
		return watchdogRunResult{Healthy: true}, nil
	}
	jobs, err := cronStore.List()
	if err != nil {
		return watchdogRunResult{}, err
	}
	store := session.NewStore(workspaceDir)
	findings := make([]watchdogFinding, 0)
	for _, job := range jobs {
		findings = append(findings, analyzeWatchdogJob(workspaceDir, store, job)...)
	}
	result := watchdogRunResult{
		Healthy:       len(findings) == 0,
		InspectedJobs: len(jobs),
		Findings:      findings,
	}
	if result.Healthy {
		result.Summary = fmt.Sprintf("inspected %d background jobs; no unhealthy signals detected", len(jobs))
		return result, nil
	}
	result.Summary = summarizeWatchdogFindings(findings)
	return result, nil
}

func analyzeWatchdogJob(workspaceDir string, store *session.Store, job cron.Job) []watchdogFinding {
	findings := make([]watchdogFinding, 0)
	projectID := strings.TrimSpace(job.ProjectID)
	target := strings.TrimSpace(job.SessionTarget)
	if projectID != "" && target != "" && !strings.EqualFold(target, "main") && !strings.EqualFold(target, "isolated") {
		findings = append(findings, watchdogFinding{
			Kind:      "explicit_session_target",
			Severity:  "warn",
			JobID:     strings.TrimSpace(job.ID),
			ProjectID: projectID,
			SessionID: target,
			Message:   fmt.Sprintf("job %s pins project work to explicit session %s", strings.TrimSpace(job.Name), target),
		})
	}
	if finding, ok := findContaminatedTranscript(store, projectID, target); ok {
		finding.JobID = strings.TrimSpace(job.ID)
		if finding.ProjectID == "" {
			finding.ProjectID = projectID
		}
		findings = append(findings, finding)
	}
	if finding, ok := findStaleProjectProgress(workspaceDir, job); ok {
		findings = append(findings, finding)
	}
	return findings
}

func findContaminatedTranscript(store *session.Store, projectID string, explicitTarget string) (watchdogFinding, bool) {
	if store == nil {
		return watchdogFinding{}, false
	}
	sess, err := resolveWatchdogSession(store, projectID, explicitTarget)
	if err != nil || sess == nil {
		return watchdogFinding{}, false
	}
	messages, err := session.ReadMessages(store.TranscriptPath(sess.ID))
	if err != nil || len(messages) == 0 {
		return watchdogFinding{}, false
	}
	start := max(0, len(messages)-8)
	for _, msg := range messages[start:] {
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
			continue
		}
		content := strings.ToLower(strings.TrimSpace(msg.Content))
		for _, marker := range watchdogContaminationMarkers {
			if strings.Contains(content, marker) {
				return watchdogFinding{
					Kind:      "contaminated_transcript",
					Severity:  "warn",
					ProjectID: strings.TrimSpace(projectID),
					SessionID: strings.TrimSpace(sess.ID),
					Message:   fmt.Sprintf("session %s contains tool/meta output contamination", strings.TrimSpace(sess.ID)),
				}, true
			}
		}
	}
	return watchdogFinding{}, false
}

func resolveWatchdogSession(store *session.Store, projectID string, explicitTarget string) (*session.Session, error) {
	target := strings.TrimSpace(explicitTarget)
	if target != "" && !strings.EqualFold(target, "main") && !strings.EqualFold(target, "isolated") {
		sess, err := store.Get(target)
		if err != nil {
			return nil, err
		}
		return &sess, nil
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, nil
	}
	sessions, err := store.List()
	if err != nil {
		return nil, err
	}
	for _, sess := range sessions {
		if strings.TrimSpace(sess.ProjectID) == strings.TrimSpace(projectID) {
			sessionCopy := sess
			return &sessionCopy, nil
		}
	}
	return nil, nil
}

func findStaleProjectProgress(workspaceDir string, job cron.Job) (watchdogFinding, bool) {
	projectID := strings.TrimSpace(job.ProjectID)
	if workspaceDir == "" || projectID == "" || job.LastRunAt == nil {
		return watchdogFinding{}, false
	}
	projectDocTime, hasProjectDoc := newestProjectContentTime(workspaceDir, projectID)
	if !hasProjectDoc {
		return watchdogFinding{}, false
	}
	artifactTime, hasArtifact := newestCronArtifactTime(workspaceDir, projectID)
	if !hasArtifact {
		return watchdogFinding{}, false
	}
	if !artifactTime.After(projectDocTime.Add(watchdogProjectStaleThreshold)) {
		return watchdogFinding{}, false
	}
	return watchdogFinding{
		Kind:      "stale_project_progress",
		Severity:  "warn",
		JobID:     strings.TrimSpace(job.ID),
		ProjectID: projectID,
		Message: fmt.Sprintf(
			"project %s has cron runs after %s without project document updates",
			projectID,
			projectDocTime.UTC().Format(time.RFC3339),
		),
	}, true
}

func newestProjectContentTime(workspaceDir string, projectID string) (time.Time, bool) {
	projectDir := filepath.Join(strings.TrimSpace(workspaceDir), "projects", strings.TrimSpace(projectID))
	var newest time.Time
	found := false
	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "cron_runs") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !found || info.ModTime().After(newest) {
			newest = info.ModTime().UTC()
			found = true
		}
		return nil
	})
	return newest, found
}

func newestCronArtifactTime(workspaceDir string, projectID string) (time.Time, bool) {
	artifactDir := filepath.Join(strings.TrimSpace(workspaceDir), "projects", strings.TrimSpace(projectID), "cron_runs")
	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		return time.Time{}, false
	}
	var newest time.Time
	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !found || info.ModTime().After(newest) {
			newest = info.ModTime().UTC()
			found = true
		}
	}
	return newest, found
}

func summarizeWatchdogFindings(findings []watchdogFinding) string {
	if len(findings) == 0 {
		return ""
	}
	parts := make([]string, 0, min(3, len(findings)))
	for _, finding := range findings {
		if len(parts) == 3 {
			break
		}
		parts = append(parts, finding.Kind+": "+finding.Message)
	}
	if len(findings) > len(parts) {
		parts = append(parts, fmt.Sprintf("%d more findings", len(findings)-len(parts)))
	}
	return strings.Join(parts, " | ")
}

type workspaceWatchdogManager struct {
	run      func(ctx context.Context) (watchdogRunResult, error)
	interval time.Duration
}

func newWorkspaceWatchdogManager(
	run func(ctx context.Context) (watchdogRunResult, error),
	interval time.Duration,
) *workspaceWatchdogManager {
	if interval <= 0 {
		interval = defaultWatchdogInterval
	}
	return &workspaceWatchdogManager{
		run:      run,
		interval: interval,
	}
}

func (m *workspaceWatchdogManager) Start(ctx context.Context) error {
	if m == nil || m.run == nil {
		return nil
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = m.run(ctx)
		}
	}
}
