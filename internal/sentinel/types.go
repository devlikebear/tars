package sentinel

import (
	"net/http"
	"time"
)

type SupervisionState string

const (
	StateStarting SupervisionState = "starting"
	StateRunning  SupervisionState = "running"
	StatePaused   SupervisionState = "paused"
	StateCooldown SupervisionState = "cooldown"
	StateStopped  SupervisionState = "stopped"
	StateError    SupervisionState = "error"
)

type EventType string

const (
	EventStart         EventType = "start"
	EventExit          EventType = "exit"
	EventHealthOK      EventType = "health_ok"
	EventHealthFail    EventType = "health_fail"
	EventRestart       EventType = "restart"
	EventCooldownEnter EventType = "cooldown_enter"
	EventCooldownExit  EventType = "cooldown_exit"
	EventPause         EventType = "pause"
	EventResume        EventType = "resume"
	EventError         EventType = "error"
)

type Event struct {
	ID      int64          `json:"id"`
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Type    EventType      `json:"type"`
	Message string         `json:"message"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type TargetSummary struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
}

type Status struct {
	Enabled            bool             `json:"enabled"`
	SupervisionState   SupervisionState `json:"supervision_state"`
	Target             TargetSummary    `json:"target"`
	TargetPID          int              `json:"target_pid,omitempty"`
	TargetStartedAt    string           `json:"target_started_at,omitempty"`
	TargetLastExitAt   string           `json:"target_last_exit_at,omitempty"`
	TargetLastExitCode *int             `json:"target_last_exit_code,omitempty"`
	HealthOK           bool             `json:"health_ok"`
	HealthLastOKAt     string           `json:"health_last_ok_at,omitempty"`
	HealthLastError    string           `json:"health_last_error,omitempty"`
	RestartAttempt     int              `json:"restart_attempt"`
	RestartMaxAttempts int              `json:"restart_max_attempts"`
	CooldownUntil      string           `json:"cooldown_until,omitempty"`
	LastRestartAt      string           `json:"last_restart_at,omitempty"`
	EventCount         int              `json:"event_count"`
}

type Options struct {
	Enabled            bool
	TargetCommand      string
	TargetArgs         []string
	TargetWorkingDir   string
	TargetEnv          map[string]string
	ProbeURL           string
	ProbeInterval      time.Duration
	ProbeTimeout       time.Duration
	ProbeFailThreshold int
	RestartMaxAttempts int
	RestartBackoff     time.Duration
	RestartBackoffMax  time.Duration
	RestartCooldown    time.Duration
	EventBufferSize    int
	Autostart          bool
	Now                func() time.Time
	HTTPClient         *http.Client
}
