package tarsserver

import (
	"context"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

// The session board (#971) is the console's home: every visible chat
// session, grouped by the repository it works in, with what each one is
// doing. GET /v1/chat/board answers it in one call so the board does not
// fan out a request per session.
//
// Status comes from the chat activity tracker: needs_input when a turn waits
// for a tool approval, running while a turn runs, idle otherwise. A cron,
// Telegram or subagent run waiting on an approval in the ops queue (#970)
// needs input too: the chat shows those questions with the turn's own. Whether
// a finished turn has been looked at is the viewer's to know, so "done but
// unread" is worked out by the console from last_turn_at.

type boardSession struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	PendingApprovals int    `json:"pending_approvals"`
	// QueuedApprovals are unattended runs' tool calls waiting in the ops
	// queue.
	QueuedApprovals int          `json:"queued_approvals"`
	RunningSince    *time.Time   `json:"running_since,omitempty"`
	LastTurnAt      *time.Time   `json:"last_turn_at,omitempty"`
	UpdatedAt       time.Time    `json:"updated_at"`
	PinnedAt        *time.Time   `json:"pinned_at,omitempty"`
	Cwd             string       `json:"cwd,omitempty"`
	Repo            string       `json:"repo,omitempty"`
	Branch          string       `json:"branch,omitempty"`
	LastChange      *boardChange `json:"last_change,omitempty"`
	CostUSD         float64      `json:"cost_usd"`
	// UnpricedCalls counts the session's calls with no known price; CostUSD
	// leaves them out.
	UnpricedCalls int             `json:"unpriced_calls"`
	Goal          *boardGoalBrief `json:"goal,omitempty"`
}

// boardChange is the latest turn that changed files, from its checkpoint.
type boardChange struct {
	TurnID    string    `json:"turn_id"`
	At        time.Time `json:"at"`
	Files     int       `json:"files"`
	Additions int       `json:"additions"`
	Deletions int       `json:"deletions"`
}

type boardGoalBrief struct {
	Status string `json:"status,omitempty"`
}

type boardResponse struct {
	Sessions []boardSession `json:"sessions"`
	// CostPeriod names the period cost_usd covers.
	CostPeriod string `json:"cost_period"`
}

const (
	boardStatusNeedsInput = "needs_input"
	boardStatusRunning    = "running"
	boardStatusIdle       = "idle"

	// boardCostPeriod matches the composer status bar's per-session cost.
	boardCostPeriod = "month"
	// boardRepoTTL is how long a folder's repository and branch are reused.
	// The board polls every few seconds; git does not need to.
	boardRepoTTL = 15 * time.Second
)

// sessionBoard assembles the board. Every source but the session store is
// optional: without checkpoints there is no repository or change count,
// without costs every cost is 0.
type sessionBoard struct {
	sessions    *session.Store
	activity    *chatActivity
	checkpoints *checkpoint.Store
	costs       func() (map[string]boardCost, error)
	// queued counts each session's unattended approvals; nil counts none.
	queued   func() (map[string]int, error)
	branchOf func(ctx context.Context, dir string) string
	now      func() time.Time

	mu    sync.Mutex
	repos map[string]boardRepo
}

// boardCost is one session's cost for the board's period.
type boardCost struct {
	USD           float64
	UnpricedCalls int
}

type boardRepo struct {
	root, branch string
	at           time.Time
}

func newSessionBoard(sessions *session.Store, activity *chatActivity, checkpoints *checkpoint.Store, costs func() (map[string]boardCost, error)) *sessionBoard {
	return &sessionBoard{
		sessions:    sessions,
		activity:    activity,
		checkpoints: checkpoints,
		costs:       costs,
		branchOf:    gitBranch,
		now:         time.Now,
		repos:       map[string]boardRepo{},
	}
}

func (b *sessionBoard) build(ctx context.Context) (boardResponse, error) {
	list, err := b.sessions.List()
	if err != nil {
		return boardResponse{}, err
	}
	snap := b.activity.snapshot()
	running := map[string]time.Time{}
	for _, r := range snap.Running {
		running[r.SessionID] = r.StartedAt
	}
	pending := map[string]int{}
	for _, p := range snap.Pending {
		pending[p.SessionID]++
	}
	queued := optionalCounts(b.queued)
	costs := optionalCounts(b.costs)

	out := boardResponse{Sessions: []boardSession{}, CostPeriod: boardCostPeriod}
	for _, s := range list {
		if s.Hidden || s.ArchivedAt != nil || (s.Kind != "" && s.Kind != "main") {
			continue
		}
		item := boardSession{
			ID:               s.ID,
			Title:            s.Title,
			Status:           boardStatusIdle,
			PendingApprovals: pending[s.ID],
			QueuedApprovals:  queued[s.ID],
			UpdatedAt:        s.UpdatedAt,
			PinnedAt:         s.PinnedAt,
			Cwd:              b.workingFolder(s),
			CostUSD:          costs[s.ID].USD,
			UnpricedCalls:    costs[s.ID].UnpricedCalls,
		}
		if started, ok := running[s.ID]; ok {
			item.Status = boardStatusRunning
			item.RunningSince = &started
		}
		if item.PendingApprovals > 0 || item.QueuedApprovals > 0 {
			item.Status = boardStatusNeedsInput
		}
		if s.Goal != nil {
			item.Goal = &boardGoalBrief{Status: s.Goal.Status}
		}
		b.addCheckpointFacts(ctx, &item)
		out.Sessions = append(out.Sessions, item)
	}
	slices.SortStableFunc(out.Sessions, func(x, y boardSession) int {
		return y.UpdatedAt.Compare(x.UpdatedAt)
	})
	return out, nil
}

// optionalCounts reads an optional per-session source. A missing or
// failing source counts nothing rather than failing the board.
func optionalCounts[V any](read func() (map[string]V, error)) map[string]V {
	if read == nil {
		return nil
	}
	out, err := read()
	if err != nil {
		return nil
	}
	return out
}

// workingFolder is the session's active cwd, or "" while it is still the
// session's own artifact folder, which every new session starts in. Those
// sessions are not working in a project, so the board keeps them out of the
// repository groups.
func (b *sessionBoard) workingFolder(s session.Session) string {
	cwd := strings.TrimSpace(s.CurrentDir)
	if cwd == "" {
		return ""
	}
	// The store saves cwd with symlinks resolved; the workspace path keeps
	// its configured spelling, which sameDir sees through.
	if sessionArtifactFolder(b.sessions, s, cwd) {
		return ""
	}
	return cwd
}

// addCheckpointFacts fills the repository, branch, last turn time, and
// latest change from the session's checkpoints and working folder.
func (b *sessionBoard) addCheckpointFacts(ctx context.Context, item *boardSession) {
	if b.checkpoints == nil {
		return
	}
	if turns, err := b.checkpoints.List(item.ID); err == nil {
		for i := len(turns) - 1; i >= 0; i-- {
			t := turns[i]
			if item.LastTurnAt == nil && !t.EndedAt.IsZero() {
				at := t.EndedAt
				item.LastTurnAt = &at
			}
			if item.LastChange == nil && t.Files > 0 {
				item.LastChange = &boardChange{TurnID: t.TurnID, At: t.EndedAt, Files: t.Files, Additions: t.Additions, Deletions: t.Deletions}
			}
			if item.LastTurnAt != nil && item.LastChange != nil {
				break
			}
		}
	}
	if item.Cwd == "" {
		return
	}
	repo := b.repoFor(ctx, item.Cwd)
	item.Repo, item.Branch = repo.root, repo.branch
}

// repoFor resolves a folder's repository top level and branch, cached for
// boardRepoTTL. A folder outside any repository is its own group.
func (b *sessionBoard) repoFor(ctx context.Context, dir string) boardRepo {
	now := b.now()
	b.mu.Lock()
	cached, ok := b.repos[dir]
	b.mu.Unlock()
	if ok && now.Sub(cached.at) < boardRepoTTL {
		return cached
	}
	repo := boardRepo{root: dir, at: now}
	if root, err := b.checkpoints.ResolveRoot(ctx, dir); err == nil {
		repo.root = root
		repo.branch = b.branchOf(ctx, root)
	}
	b.mu.Lock()
	b.repos[dir] = repo
	b.mu.Unlock()
	return repo
}

// gitBranch names the branch checked out in dir, or a short commit for a
// detached HEAD, or "" outside a repository.
func gitBranch(ctx context.Context, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	run := func(args ...string) string {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) // NOSONAR: git is the person's own toolchain, resolved from their PATH like every other TARS git call.
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	if name := run("symbolic-ref", "--short", "-q", "HEAD"); name != "" {
		return name
	}
	return run("rev-parse", "--short", "HEAD")
}

func (b *sessionBoard) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	resp, err := b.build(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// sessionCostsFrom reads this month's cost per session from the usage
// tracker in one pass over the month's entries.
func sessionCostsFrom(tracker *usage.Tracker) func() (map[string]boardCost, error) {
	if tracker == nil {
		return nil
	}
	return func() (map[string]boardCost, error) {
		summary, err := tracker.Summary(boardCostPeriod, "session")
		if err != nil {
			return nil, err
		}
		out := make(map[string]boardCost, len(summary.Rows))
		for _, row := range summary.Rows {
			out[row.Key] = boardCost{USD: row.CostUSD, UnpricedCalls: row.UnpricedCalls}
		}
		return out, nil
	}
}
