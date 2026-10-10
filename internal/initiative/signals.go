package initiative

import "time"

// History is what the runtime remembers about its own past initiatives.
type History struct {
	Today         int
	LastSpokeAt   time.Time
	LastCheckInAt time.Time
	LastBodyAt    time.Time
	// TextCallsToday and LastTextCallAt track the daily text-signal
	// backend call cap (tars#1219), independent of Today (spoken
	// initiatives).
	TextCallsToday int
	LastTextCallAt time.Time
	// TextReadFailCount is how many text-signal backend calls have failed
	// in a row, ending at LastTextCallAt — the input to the read backoff
	// schedule (tars#1220 addendum to #1219: an always-failing backend
	// must not be retried every tick). Reset to 0 by a successful call.
	TextReadFailCount int
	// SpeakCallsToday, LastSpeakCallAt and SpeakFailCount are the speak
	// composer's equivalents (tars#1220): the daily DailySpeakCalls cap and
	// the speak backoff schedule, independent of Today (which live mode
	// only increments on an actually delivered speak).
	SpeakCallsToday int
	LastSpeakCallAt time.Time
	SpeakFailCount  int
}

func deriveGoSignals(cfg Config, obs Observation, hist History) GoSignals {
	now := obs.Now
	lastUser := obs.LastUserAt
	if n := len(obs.RecentUser); n > 0 && obs.RecentUser[n-1].At.After(lastUser) {
		lastUser = obs.RecentUser[n-1].At
	}
	since := func(ts time.Time) time.Duration { return now.Sub(ts) }
	consoleUp := !obs.ConsoleConnectedAt.IsZero()

	g := GoSignals{
		QuietHours:    inQuietWindow(cfg.QuietHours, now.In(cfg.Location)),
		Busy:          obs.ChatBusy || (!lastUser.IsZero() && since(lastUser) < typingWindow),
		Reachable:     consoleUp || obs.TelegramPaired,
		BodyAvailable: obs.BodyAvailable,
	}
	g.JustArrived = consoleUp && since(obs.ConsoleConnectedAt) <= arrivalFreshness &&
		(lastUser.IsZero() || obs.ConsoleConnectedAt.Sub(lastUser) >= arrivalGap)
	g.LongSession = !lastUser.IsZero() && since(lastUser) <= longSessionTail && activeSpan(obs.RecentUser) >= longSession
	g.LongAbsence = !lastUser.IsZero() && since(lastUser) >= longAbsence
	g.IdleAtDesk = consoleUp && !lastUser.IsZero() && since(lastUser) >= idleMin && since(lastUser) <= idleMax
	g.CooldownActive = !hist.LastSpokeAt.IsZero() && since(hist.LastSpokeAt) < cfg.Cooldown
	g.DailyCapReached = hist.Today >= cfg.DailyCap
	g.CheckedInRecently = !hist.LastCheckInAt.IsZero() && since(hist.LastCheckInAt) < checkInSpacing
	g.BodyRecently = !hist.LastBodyAt.IsZero() && since(hist.LastBodyAt) < bodySpacing
	return g
}

// activeSpan is how long the newest unbroken run of user messages lasts.
func activeSpan(msgs []UserMessage) time.Duration {
	if len(msgs) == 0 {
		return 0
	}
	end := msgs[len(msgs)-1].At
	start := end
	for i := len(msgs) - 2; i >= 0; i-- {
		if start.Sub(msgs[i].At) > sessionGap {
			break
		}
		start = msgs[i].At
	}
	return end.Sub(start)
}
