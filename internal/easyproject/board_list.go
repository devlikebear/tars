package easyproject

import (
	"encoding/base64"
	"errors"
	"sort"
	"strings"
	"time"
)

// Listed is a project as the board lists it: the record and its board.
type Listed struct {
	Project
	Board Board `json:"board"`
}

// ListQuery picks a page of the board.
type ListQuery struct {
	// State keeps only projects in that state; empty keeps all.
	State State
	// Archived lists the archived projects instead of the board's.
	Archived bool
	// Limit caps the page; 0 lists everything after the cursor.
	Limit int
	// Cursor is a previous page's Next.
	Cursor string
}

// MaxListLimit caps a page.
const MaxListLimit = 100

// ListPage is one page of the board.
type ListPage struct {
	Projects []Listed `json:"projects"`
	// Next is the cursor of the page after this one; empty on the last.
	Next string `json:"next_cursor,omitempty"`
	// Counts are how many projects of the listed side (board or archive)
	// are in each state, whatever the state filter and the page.
	Counts map[State]int `json:"counts"`
	// Archived is how many projects are archived.
	Archived int `json:"archived"`
}

// ErrBadCursor and ErrBadState reject a query.
var (
	ErrBadCursor = errors.New("bad cursor")
	ErrBadState  = errors.New("unknown state")
)

// ValidState reports whether s is a board state.
func ValidState(s State) bool {
	for _, known := range States {
		if s == known {
			return true
		}
	}
	return false
}

// Page orders the listed projects newest change first and answers the
// page q asks for.
func Page(all []Listed, q ListQuery) (ListPage, error) {
	if q.State != "" && !ValidState(q.State) {
		return ListPage{}, ErrBadState
	}
	page := ListPage{Projects: []Listed{}, Counts: map[State]int{}}
	for _, s := range States {
		page.Counts[s] = 0
	}
	var side []Listed
	for _, l := range all {
		if l.ArchivedAt != nil {
			page.Archived++
		}
		if (l.ArchivedAt != nil) != q.Archived {
			continue
		}
		page.Counts[l.Board.State]++
		if q.State == "" || l.Board.State == q.State {
			side = append(side, l)
		}
	}
	sort.SliceStable(side, func(i, j int) bool { return before(side[i], side[j]) })

	start := 0
	if q.Cursor != "" {
		at, id, err := decodeCursor(q.Cursor)
		if err != nil {
			return ListPage{}, err
		}
		mark := Listed{Project: Project{ID: id}, Board: Board{ChangedAt: at}}
		start = sort.Search(len(side), func(i int) bool { return before(mark, side[i]) })
	}
	end := len(side)
	limit := min(q.Limit, MaxListLimit)
	if limit > 0 && start+limit < end {
		end = start + limit
		last := side[end-1]
		page.Next = encodeCursor(last.Board.ChangedAt, last.ID)
	}
	page.Projects = append(page.Projects, side[start:end]...)
	return page, nil
}

// before is the board's order: newest change first, then id, so the order
// is total and a cursor finds its place.
func before(a, b Listed) bool {
	if !a.Board.ChangedAt.Equal(b.Board.ChangedAt) {
		return a.Board.ChangedAt.After(b.Board.ChangedAt)
	}
	return a.ID > b.ID
}

func encodeCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", ErrBadCursor
	}
	stamp, id, ok := strings.Cut(string(raw), "|")
	if !ok || !validID(id) {
		return time.Time{}, "", ErrBadCursor
	}
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Time{}, "", ErrBadCursor
	}
	return at, id, nil
}
