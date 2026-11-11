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
	// IntentRemind delivers a cron reminder the user scheduled
	// (initiative.remind, tars#1001) through the speak path instead of the
	// cron system note.
	IntentRemind Intent = "remind"
	// IntentConfirmMemory asks whether to keep one pending reflection
	// inbox candidate (initiative.confirm_memory, tars#1001).
	IntentConfirmMemory Intent = "confirm_memory"
	// IntentShareResult reports background work that finished or failed
	// while nobody was looking — an agent run no turn waited for, a cron job
	// that left no session note (initiative.share_result, tars#1002).
	IntentShareResult Intent = "share_result"
	// IntentFollowUp is the first arrival of the day when work was left
	// unfinished on an earlier day — an active session goal, an unfinished
	// focus pipeline or its pull request (initiative.follow_up, tars#1002).
	// It takes the place of that arrival's greet.
	IntentFollowUp Intent = "follow_up"
)

// tracksReplies reports whether a delivered speak of this intent opens a
// reply-tracking window (tars#1221). A reminder is something the user asked
// for; not answering it says nothing about whether TARS should speak less,
// so it never feeds the ignored streak or the cooldown multiplier. A
// share_result report (tars#1002) is the same kind of thing: news the user
// can read and move on from, so silence after it is not a signal either.
func tracksReplies(intent Intent) bool {
	return intent != IntentRemind && intent != IntentShareResult
}

// Subject is what a remind, confirm_memory, share_result or follow_up speak
// is about: an id the server resolves (a queued reminder, a memory inbox
// candidate, the queued results a report covers) and the text to say. The ledger never records either field (tars#1003) — only
// whether one existed, as a Go signal.
type Subject struct {
	ID   string
	Text string
	// At is when the subject became due (a reminder's fire time); zero for
	// a memory candidate.
	At time.Time
}

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
	MainUserAt time.Time
	// ConsolePresentAt is the last time the console told the server a
	// person was actually there (tars#1249's presence signal, POST
	// /v1/console/presence) — not merely that something is subscribed to
	// /v1/events/stream, which stays open for hours on an unattended tab
	// or the desktop tray's own subscription. Zero means no presence
	// signal has ever been recorded. consolePresent treats a signal older
	// than presenceTTL the same as zero: a quiet console reads as "not at
	// the desk" again once pings stop, whether or not the SSE connection
	// itself is still open.
	ConsolePresentAt time.Time
	// ConsoleArrivedAt is when the current unbroken run of presence
	// signals began (RecordPresence) — the timestamp of the first signal
	// after a gap of presenceTTL or more, or of the very first signal
	// ever. JustArrived reads this instead of ConsolePresentAt, which
	// keeps advancing on every fresh ping and would otherwise never stay
	// "just arrived" for long.
	ConsoleArrivedAt time.Time
	ChatBusy         bool
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
	// Reminder is the oldest queued cron reminder that is due now
	// (initiative.remind, tars#1001), nil when none. The adapter hands a
	// reminder back to cron's own delivery once it has waited too long
	// (ReminderGrace), so one here is always still deliverable.
	Reminder *Subject
	// MemoryCandidate is the pending reflection inbox candidate to ask
	// about (initiative.confirm_memory, tars#1001), nil when none.
	MemoryCandidate *Subject
	// Result is the background work waiting to be reported
	// (initiative.share_result, tars#1002), nil when none: every queued
	// result the next report covers, ID listing their queue ids and Text
	// one line per result. The adapter only queues results that left no
	// conversational trace — see tarsserver's initiativeResultQueue.
	Result *Subject
	// FollowUp is the work left unfinished on an earlier local day
	// (initiative.follow_up, tars#1002), nil when none. Only filled around
	// a console arrival, the one moment Decide reads it.
	FollowUp *Subject
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
	// GreetedToday reports whether a greet already counted as spoken
	// (History.LastGreetAt — live mode only when actually delivered,
	// shadow mode per Entry.spoken()'s existing rule) earlier the same
	// local day (tars#1269). Unlike the other hard gates above, this only
	// disables the JustArrived arrival case in Decide — it does not block
	// check_in or body_only, so a second arrival the same day still falls
	// through to those.
	GreetedToday bool `json:"greeted_today"`
	// ReminderDue reports a queued cron reminder is due (tars#1001). Only
	// ever true with initiative.remind on.
	ReminderDue bool `json:"reminder_due,omitempty"`
	// MemoryPending reports a pending reflection inbox candidate to ask
	// about (tars#1001). Only ever true with initiative.confirm_memory on.
	MemoryPending bool `json:"memory_pending,omitempty"`
	// MemoryAskedToday reports a confirm_memory speak already counted as
	// spoken earlier the same local day (History.LastMemoryAskAt, the same
	// spoken() rule as GreetedToday): at most one question a day.
	MemoryAskedToday bool `json:"memory_asked_today,omitempty"`
	// ResultReady reports a background result is queued to be reported
	// (tars#1002). Only ever true with initiative.share_result on.
	ResultReady bool `json:"result_ready,omitempty"`
	// FollowUpPending reports work left unfinished on an earlier day
	// (tars#1002). Only ever true with initiative.follow_up on.
	FollowUpPending bool `json:"follow_up_pending,omitempty"`
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
