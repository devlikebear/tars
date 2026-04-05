package tarsserver

import (
	"context"
	"fmt"
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

func analyzeWatchdogJob(_ string, store *session.Store, job cron.Job) []watchdogFinding {
	findings := make([]watchdogFinding, 0)
	target := strings.TrimSpace(job.SessionTarget)
	if finding, ok := findContaminatedTranscript(store, target); ok {
		finding.JobID = strings.TrimSpace(job.ID)
		findings = append(findings, finding)
	}
	return findings
}

func findContaminatedTranscript(store *session.Store, explicitTarget string) (watchdogFinding, bool) {
	if store == nil {
		return watchdogFinding{}, false
	}
	sess, err := resolveWatchdogSession(store, explicitTarget)
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
					SessionID: strings.TrimSpace(sess.ID),
					Message:   fmt.Sprintf("session %s contains tool/meta output contamination", strings.TrimSpace(sess.ID)),
				}, true
			}
		}
	}
	return watchdogFinding{}, false
}

func resolveWatchdogSession(store *session.Store, explicitTarget string) (*session.Session, error) {
	target := strings.TrimSpace(explicitTarget)
	if target != "" && !strings.EqualFold(target, "main") && !strings.EqualFold(target, "isolated") {
		sess, err := store.Get(target)
		if err != nil {
			return nil, err
		}
		return &sess, nil
	}
	return nil, nil
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
