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
	Now        time.Time
	RecentUser []UserMessage // oldest first
	// LastUserAt is the newest user message anywhere, even when it is too
	// old to be in RecentUser. Zero means unknown; RecentUser then decides.
	LastUserAt time.Time
	// MainUserAt is the newest user message timestamp reply tracking
	// (tars#1221) treats as a reply to a delivered greet/check_in (zero
	// when none). Usually the main session specifically, since that is
	// where a delivered speak is always written — except when it was
	// delivered over Telegram under session_telegram_scope: per-user
	// (tars#1221 P2c): a reply there lands in that Telegram user's own
	// session, not the main one, so the adapter folds that session's
	// newest user message in too (whichever is newer).
	MainUserAt         time.Time
	ConsoleConnectedAt time.Time // zero when no console is subscribed
	ChatBusy           bool
	// TelegramPaired is true when at least one Telegram chat is paired —
	// used only for GoSignals.Reachable, which treats several paired
	// chats with none chosen as still reachable (tars#1219/#1220).
	TelegramPaired bool
	// TelegramReady is stricter: true only when exactly one Telegram chat
	// resolves as the default delivery target right now (tars#1221 P2c).
	// deliverLocked requires this (together with Config.Telegram) before
	// ever picking the telegram channel — several paired chats with none
	// chosen, like zero paired chats, is not ready even though it still
	// counts as Reachable above.
	TelegramReady bool
	BodyAvailable bool
	Profile       string
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
	// StoppedToday reports whether a reply-tracking "stopped" outcome
	// (tars#1221) was recorded earlier the same local day — set from
	// History.LastStoppedAt. True blocks every Speak decision for the
	// rest of that day, the same hard-gate shape as QuietHours/CooldownActive.
	StoppedToday bool `json:"stopped_today"`
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
