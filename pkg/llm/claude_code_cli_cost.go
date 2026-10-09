package llm

import "sync"

// A result's total_cost_usd is the upstream session's running total, not
// the call's: every --resume of a session reports what the session has cost
// since it began. Recorded as it came, a ten-turn session was counted as
// the sum of ten growing totals (a $19 session showed as $440).
//
// claudeCodeSessionCosts remembers the last total each upstream session
// reported, so a resumed call is charged the difference. It is shared by
// every client in the process, because one session is resumed under
// different tiers (a plan turn on one model, the build on another).
var claudeCodeSessionCosts = newClaudeCodeSessionCosts(1024)

type claudeCodeSessionCostLedger struct {
	mu     sync.Mutex
	limit  int
	totals map[string]float64
	order  []string // insertion order, for eviction
}

func newClaudeCodeSessionCosts(limit int) *claudeCodeSessionCostLedger {
	return &claudeCodeSessionCostLedger{limit: limit, totals: map[string]float64{}}
}

// callCost turns the total a call reported into that call's own cost and
// remembers the total. resumeID is the session the call resumed ("" for a
// fresh one) and sessionID the one its stream named.
//
// A resumed call whose earlier total is not known — the first one after a
// restart — reports 0: its share of the total cannot be told apart, and an
// unpriced call is a smaller error than charging the whole session again.
func (l *claudeCodeSessionCostLedger) callCost(resumeID, sessionID string, total float64) float64 {
	if total <= 0 {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	prior, known := l.totals[resumeID]
	if !known && sessionID != resumeID {
		prior, known = l.totals[sessionID]
	}
	for _, id := range []string{resumeID, sessionID} {
		l.rememberLocked(id, total)
	}
	switch {
	case resumeID == "":
		return total
	case !known:
		return 0
	case total < prior:
		// The session's counter started over; the total is this call's.
		return total
	default:
		return total - prior
	}
}

func (l *claudeCodeSessionCostLedger) rememberLocked(id string, total float64) {
	if id == "" {
		return
	}
	if _, ok := l.totals[id]; !ok {
		l.order = append(l.order, id)
		for len(l.order) > l.limit {
			delete(l.totals, l.order[0])
			l.order = l.order[1:]
		}
	}
	l.totals[id] = total
}
