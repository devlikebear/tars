package pulse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/pulse/autofix"
	"github.com/devlikebear/tars/internal/session"
	zlog "github.com/rs/zerolog/log"
)

// CronJobLister is the narrow interface pulse requires from a cron store
// to count failing jobs. The real *cron.Store satisfies it.
type CronJobLister interface {
	List() ([]cron.Job, error)
}

// AgentRuntimeRunLister is the narrow interface pulse requires from the
// agent runtime to find stuck runs. The real *agentruntime.Runtime satisfies
// it.
type AgentRuntimeRunLister interface {
	List(limit int) []agentruntime.Run
}

// DiskStatProvider is the narrow interface pulse requires from the ops
// manager to read disk usage. The real *ops.Manager satisfies it.
type DiskStatProvider interface {
	Status(ctx context.Context) (ops.Status, error)
}

// DeliveryFailureCounter is the narrow interface pulse requires from the
// telegram delivery counter. The real counter in internal/tarsserver
// satisfies it.
type DeliveryFailureCounter interface {
	FailuresWithin(window time.Duration) int
}

// ReflectionHealthSource is the narrow interface pulse requires from the
// reflection runtime to observe nightly-run health. The real
// *reflection.State satisfies it without importing pulse in reverse.
type ReflectionHealthSource interface {
	ConsecutiveFailures() int
	LastRunAt() time.Time
}

// ChatSessionSource is the narrow session-store surface pulse needs to
// detect chats that appear to be waiting on a user answer while work is active.
type ChatSessionSource interface {
	List() ([]session.Session, error)
	GetTasks(sessionID string) (session.SessionTasks, error)
	TranscriptPath(sessionID string) string
}

type StalledChatCandidate struct {
	SessionID              string    `json:"session_id"`
	Title                  string    `json:"title,omitempty"`
	LastMessageID          string    `json:"last_message_id,omitempty"`
	LastAssistantAt        time.Time `json:"last_assistant_at"`
	AgeMinutes             int       `json:"age_minutes"`
	AutoResumeEnabled      bool      `json:"auto_resume_enabled"`
	AutoResumeAfterMinutes int       `json:"auto_resume_after_minutes"`
	AllowedResumeModes     []string  `json:"allowed_resume_modes,omitempty"`
	ResumeMode             string    `json:"resume_mode,omitempty"`
	CanAutoResume          bool      `json:"can_auto_resume"`
	BlockReason            string    `json:"block_reason,omitempty"`
	QuestionPreview        string    `json:"question_preview,omitempty"`
}

// Thresholds controls when signals are emitted. Zero values mean
// "disabled" for that signal (never emit).
type Thresholds struct {
	// CronConsecutiveFailures — emit when any job's consecutive failures
	// reaches or exceeds this value. 0 = disabled.
	CronConsecutiveFailures int

	// StuckRunMinutes — emit when any agent runtime run has been in Running
	// status for at least this many minutes. 0 = disabled.
	StuckRunMinutes int

	// DiskUsedPercentWarn — emit a warn signal when disk usage percent
	// reaches or exceeds this value. 0 = disabled.
	DiskUsedPercentWarn float64

	// DiskUsedPercentCritical — emit a critical signal above this value.
	// 0 = disabled.
	DiskUsedPercentCritical float64

	// DeliveryFailuresWithinWindow — emit when telegram delivery failures
	// in the last DeliveryFailureWindow duration reach this count.
	// 0 = disabled.
	DeliveryFailuresWithinWindow int

	// DeliveryFailureWindow — rolling window for counting delivery
	// failures. Zero defaults to 10 minutes.
	DeliveryFailureWindow time.Duration

	// ReflectionConsecutiveFailures — emit when the reflection health
	// source reports this many (or more) consecutive nightly failures.
	// 0 = disabled.
	ReflectionConsecutiveFailures int
}

// ScannerSources bundles the data sources a Scanner reads from. Any field
// may be nil; nil sources yield no signals for that domain.
type ScannerSources struct {
	Cron         CronJobLister
	AgentRuntime AgentRuntimeRunLister
	Ops          DiskStatProvider
	Delivery     DeliveryFailureCounter
	Reflection   ReflectionHealthSource
	ChatSessions ChatSessionSource
}

// Scanner collects Signals from the configured sources. It is stateless
// and safe to call concurrently from multiple ticks, though in practice
// the runtime serializes ticks.
type Scanner struct {
	sources    ScannerSources
	thresholds Thresholds
	now        func() time.Time
}

// NewScanner constructs a Scanner. Callers typically build one at server
// startup and reuse it across ticks.
func NewScanner(sources ScannerSources, thresholds Thresholds) *Scanner {
	return &Scanner{
		sources:    sources,
		thresholds: thresholds,
		now:        time.Now,
	}
}

// Scan runs each enabled signal source once and returns the resulting
// signals. Sources that fail are surfaced as a SignalKindInfo with
// severity Warn — we do not propagate errors, because a single broken
// source should not block the whole tick.
func (s *Scanner) Scan(ctx context.Context) []Signal {
	if s == nil {
		return nil
	}
	now := s.now()
	var signals []Signal

	if sig := s.scanCron(); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanStuckRuns(now); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanDisk(ctx, now); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanDelivery(now); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanReflection(now); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanStalledChats(ctx, now); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanFailedChats(ctx, now); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanAutoContinueGoals(ctx, now); sig != nil {
		signals = append(signals, *sig)
	}
	return signals
}

func (s *Scanner) scanStalledChats(ctx context.Context, now time.Time) *Signal {
	if s.sources.ChatSessions == nil {
		return nil
	}
	candidates, err := DetectStalledChatCandidates(ctx, s.sources.ChatSessions, now)
	if err != nil {
		zlog.Logger.Warn().Err(err).Msg("pulse: session stalled-chat scan failed; skipping this tick")
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}
	primary := candidates[0]
	details := newChatSignalDetails("stalled_count", candidates, autofix.AutoContinueChatName, map[string]any{
		"resume_mode": primary.ResumeMode,
	})
	return &Signal{
		Kind:     SignalKindStalledChat,
		Severity: SeverityWarn,
		Summary: fmt.Sprintf(
			"%d chat session(s) appear stalled waiting for user input",
			len(candidates),
		),
		Details: details,
		At:      now,
	}
}

func DetectStalledChatCandidates(ctx context.Context, source ChatSessionSource, now time.Time) ([]StalledChatCandidate, error) {
	if source == nil {
		return nil, nil
	}
	sessions, err := source.List()
	if err != nil {
		return nil, err
	}
	var candidates []StalledChatCandidate
	for _, sess := range sessions {
		if err := ctx.Err(); err != nil {
			return candidates, err
		}
		tasks, err := source.GetTasks(sess.ID)
		if err != nil {
			zlog.Logger.Debug().Err(err).Str("session_id", sess.ID).Msg("pulse: skip stalled-chat task read failure")
			continue
		}
		if !hasActiveSessionWork(tasks) {
			continue
		}
		messages, err := session.ReadMessages(source.TranscriptPath(sess.ID))
		if err != nil {
			zlog.Logger.Debug().Err(err).Str("session_id", sess.ID).Msg("pulse: skip stalled-chat transcript read failure")
			continue
		}
		msg, ok := latestConversationalMessage(messages)
		if !ok || msg.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if !looksLikeWaitingQuestion(content) {
			continue
		}
		lastAt := msg.Timestamp.UTC()
		if lastAt.IsZero() {
			lastAt = sess.UpdatedAt.UTC()
		}
		consent := sess.AutomationConsent
		afterMinutes := session.DefaultAutoResumeAfterMinutes
		autoResumeEnabled := false
		modes := []string{session.AutoResumeModeRecordAssumptionAndProceed}
		if consent != nil {
			afterMinutes = consent.EffectiveAutoResumeAfterMinutes()
			autoResumeEnabled = consent.AllowsAutoResume()
			modes = consent.EffectiveAllowedResumeModes()
		}
		if now.Sub(lastAt) < time.Duration(afterMinutes)*time.Minute {
			continue
		}
		blockReason := ""
		if isHighRiskQuestion(content) {
			blockReason = "high_risk_question"
		}
		resumeMode := ""
		if len(modes) > 0 {
			resumeMode = modes[0]
		}
		canAutoResume := autoResumeEnabled && blockReason == "" && resumeMode != ""
		candidates = append(candidates, StalledChatCandidate{
			SessionID:              sess.ID,
			Title:                  sess.Title,
			LastMessageID:          msg.ID,
			LastAssistantAt:        lastAt,
			AgeMinutes:             int(now.Sub(lastAt).Minutes()),
			AutoResumeEnabled:      autoResumeEnabled,
			AutoResumeAfterMinutes: afterMinutes,
			AllowedResumeModes:     modes,
			ResumeMode:             resumeMode,
			CanAutoResume:          canAutoResume,
			BlockReason:            blockReason,
			QuestionPreview:        trimPulsePreview(content, 180),
		})
	}
	return candidates, nil
}

func hasActiveSessionWork(tasks session.SessionTasks) bool {
	if tasks.Plan != nil {
		switch strings.TrimSpace(tasks.Plan.Status) {
		case "", session.PlanStatusDrafting, session.PlanStatusProposed, session.PlanStatusExecuting, session.PlanStatusPaused:
			return true
		}
	}
	for _, task := range tasks.Tasks {
		switch strings.TrimSpace(task.Status) {
		case "pending", "in_progress":
			return true
		}
	}
	return false
}

func latestConversationalMessage(messages []session.Message) (session.Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		switch strings.TrimSpace(messages[i].Role) {
		case "user", "assistant":
			return messages[i], true
		}
	}
	return session.Message{}, false
}

func looksLikeWaitingQuestion(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "?") {
		return true
	}
	cues := []string{
		"please confirm",
		"should i",
		"would you like",
		"which option",
		"what would you",
		"can you confirm",
		"확인해",
		"진행할까요",
		"어떻게 할까요",
		"선택해",
		"응답",
		"입력해",
	}
	for _, cue := range cues {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	return false
}

func isHighRiskQuestion(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	risky := []string{
		"password",
		"credential",
		"secret",
		"api key",
		"token",
		"otp",
		"2fa",
		"production",
		"delete",
		"discard",
		"reset --hard",
		"payment",
		"비밀번호",
		"토큰",
		"시크릿",
		"운영",
		"삭제",
		"폐기",
		"결제",
	}
	for _, term := range risky {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func trimPulsePreview(content string, limit int) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if limit <= 0 {
		return content
	}
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return strings.TrimSpace(string(runes[:limit]))
}

// scanReflection emits a signal when the reflection runtime has
// accumulated consecutive nightly failures at or above the configured
// threshold. Reflection is cheap to read (in-memory counter) so this
// scan runs on every pulse tick.
func (s *Scanner) scanReflection(now time.Time) *Signal {
	if s.sources.Reflection == nil || s.thresholds.ReflectionConsecutiveFailures <= 0 {
		return nil
	}
	failures := s.sources.Reflection.ConsecutiveFailures()
	if failures < s.thresholds.ReflectionConsecutiveFailures {
		return nil
	}
	sev := SeverityWarn
	if failures >= s.thresholds.ReflectionConsecutiveFailures*2 {
		sev = SeverityError
	}
	return &Signal{
		Kind:     SignalKindReflectionFailure,
		Severity: sev,
		Summary: fmt.Sprintf(
			"reflection has failed %d consecutive night(s)",
			failures,
		),
		Details: map[string]any{
			"consecutive_failures": failures,
			"threshold":            s.thresholds.ReflectionConsecutiveFailures,
			"last_run_at":          s.sources.Reflection.LastRunAt().Format(time.RFC3339),
		},
		At: now,
	}
}

func (s *Scanner) scanCron() *Signal {
	if s.sources.Cron == nil || s.thresholds.CronConsecutiveFailures <= 0 {
		return nil
	}
	jobs, err := s.sources.Cron.List()
	if err != nil {
		zlog.Logger.Warn().Err(err).Msg("pulse: cron.List() failed; skipping cron scan this tick")
		return nil
	}
	worst := 0
	var worstJob cron.Job
	total := 0
	for _, j := range jobs {
		if j.ConsecutiveFailures >= s.thresholds.CronConsecutiveFailures {
			total++
			if j.ConsecutiveFailures > worst {
				worst = j.ConsecutiveFailures
				worstJob = j
			}
		}
	}
	if total == 0 {
		return nil
	}
	sev := SeverityWarn
	if worst >= s.thresholds.CronConsecutiveFailures*2 {
		sev = SeverityError
	}
	return &Signal{
		Kind:     SignalKindCronFailures,
		Severity: sev,
		Summary: fmt.Sprintf(
			"%d cron job(s) are failing (worst: %q at %d consecutive failures)",
			total, worstJob.Name, worst,
		),
		Details: map[string]any{
			"jobs_failing":    total,
			"worst_job_id":    worstJob.ID,
			"worst_job_name":  worstJob.Name,
			"worst_failures":  worst,
			"worst_job_error": worstJob.LastRunError,
		},
		At: s.now(),
	}
}

func (s *Scanner) scanStuckRuns(now time.Time) *Signal {
	if s.sources.AgentRuntime == nil || s.thresholds.StuckRunMinutes <= 0 {
		return nil
	}
	runs := s.sources.AgentRuntime.List(100)
	if len(runs) == 0 {
		return nil
	}
	cutoff := now.Add(-time.Duration(s.thresholds.StuckRunMinutes) * time.Minute)
	var stuck []agentruntime.Run
	var worstStarted time.Time
	for _, r := range runs {
		if r.Status != agentruntime.RunStatusRunning {
			continue
		}
		started, ok := parseRunTimestamp(r.StartedAt)
		if !ok {
			zlog.Logger.Warn().
				Str("run_id", r.ID).
				Str("started_at", r.StartedAt).
				Msg("pulse: agent runtime run StartedAt is malformed; skipping stuck-run check for this run")
			continue
		}
		if started.Before(cutoff) {
			stuck = append(stuck, r)
			if worstStarted.IsZero() || started.Before(worstStarted) {
				worstStarted = started
			}
		}
	}
	if len(stuck) == 0 {
		return nil
	}
	sev := SeverityWarn
	if len(stuck) >= 3 {
		sev = SeverityError
	}
	return &Signal{
		Kind:     SignalKindStuckAgentRuntimeRun,
		Severity: sev,
		Summary: fmt.Sprintf(
			"%d agent runtime run(s) stuck in running for more than %d minute(s)",
			len(stuck), s.thresholds.StuckRunMinutes,
		),
		Details: map[string]any{
			"stuck_count":           len(stuck),
			"oldest_started_at":     worstStarted.Format(time.RFC3339),
			"stuck_minutes_minimum": s.thresholds.StuckRunMinutes,
		},
		At: now,
	}
}

func (s *Scanner) scanDisk(ctx context.Context, now time.Time) *Signal {
	if s.sources.Ops == nil {
		return nil
	}
	if s.thresholds.DiskUsedPercentWarn <= 0 && s.thresholds.DiskUsedPercentCritical <= 0 {
		return nil
	}
	status, err := s.sources.Ops.Status(ctx)
	if err != nil {
		zlog.Logger.Warn().Err(err).Msg("pulse: ops.Status() failed; skipping disk scan this tick")
		return nil
	}
	pct := status.DiskUsedPercent
	critical := s.thresholds.DiskUsedPercentCritical
	warn := s.thresholds.DiskUsedPercentWarn
	var sev Severity
	switch {
	case critical > 0 && pct >= critical:
		sev = SeverityCritical
	case warn > 0 && pct >= warn:
		sev = SeverityWarn
	default:
		return nil
	}
	return &Signal{
		Kind:     SignalKindDiskUsage,
		Severity: sev,
		Summary:  fmt.Sprintf("disk usage at %.1f%%", pct),
		Details: map[string]any{
			"disk_used_percent":  pct,
			"disk_total_bytes":   status.DiskTotalBytes,
			"disk_free_bytes":    status.DiskFreeBytes,
			"warn_threshold":     warn,
			"critical_threshold": critical,
		},
		At: now,
	}
}

func (s *Scanner) scanDelivery(now time.Time) *Signal {
	if s.sources.Delivery == nil || s.thresholds.DeliveryFailuresWithinWindow <= 0 {
		return nil
	}
	window := s.thresholds.DeliveryFailureWindow
	if window <= 0 {
		window = 10 * time.Minute
	}
	count := s.sources.Delivery.FailuresWithin(window)
	if count < s.thresholds.DeliveryFailuresWithinWindow {
		return nil
	}
	sev := SeverityWarn
	if count >= s.thresholds.DeliveryFailuresWithinWindow*2 {
		sev = SeverityError
	}
	return &Signal{
		Kind:     SignalKindDeliveryFailures,
		Severity: sev,
		Summary: fmt.Sprintf(
			"%d telegram delivery failure(s) in the last %s",
			count, window,
		),
		Details: map[string]any{
			"failures":  count,
			"window":    window.String(),
			"threshold": s.thresholds.DeliveryFailuresWithinWindow,
		},
		At: now,
	}
}

// parseRunTimestamp parses the string-typed StartedAt/CreatedAt fields
// used by agentruntime.Run. Returns false for empty or malformed values.
func parseRunTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
