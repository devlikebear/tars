// Package initiative decides when TARS should speak first. Go computes the
// exact signals, a System One server reads the user's own words for a few
// fuzzy ones, and a pure policy turns both into an intent. See tars#997.
package initiative

import "time"

type Intent string

const (
	IntentNone     Intent = "none"
	IntentGreet    Intent = "greet"
	IntentCheckIn  Intent = "check_in"
	IntentBodyOnly Intent = "body_only"
)

type UserMessage struct {
	At   time.Time
	Text string
}

// Observation is what one tick sees. Adapters in tarsserver fill it.
type Observation struct {
	Now                time.Time
	RecentUser         []UserMessage // oldest first
	ConsoleConnectedAt time.Time     // zero when no console is subscribed
	ChatBusy           bool
	TelegramPaired     bool
	BodyAvailable      bool
	Profile            string
}

type GoSignals struct {
	QuietHours        bool `json:"quiet_hours"`
	Busy              bool `json:"busy"`
	JustArrived       bool `json:"just_arrived"`
	LongSession       bool `json:"long_session"`
	LongAbsence       bool `json:"long_absence"`
	IdleAtDesk        bool `json:"idle_at_desk"`
	Reachable         bool `json:"reachable"`
	BodyAvailable     bool `json:"body_available"`
	CooldownActive    bool `json:"cooldown_active"`
	DailyCapReached   bool `json:"daily_cap_reached"`
	CheckedInRecently bool `json:"checked_in_recently"`
	BodyRecently      bool `json:"body_recently"`
}

type TextSignals struct {
	QuietRequested bool               `json:"quiet_requested"`
	UserStrained   bool               `json:"user_strained"`
	SpecialDay     bool               `json:"special_day"`
	Probabilities  map[string]float64 `json:"probabilities,omitempty"`
	// Source is systemone, cache, skipped or error.
	Source string `json:"source"`
}

type Decision struct {
	Intent Intent `json:"intent"`
	Speak  bool   `json:"speak"`
	Reason string `json:"reason"`
}
