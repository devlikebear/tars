package tarsserver

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/session"
)

// The facts the morning brief and the evening recap read (tars#1001
// brief). Every one comes from a store TARS already keeps — sessions,
// focus pipelines, cron jobs, the daily log — and is read here, in Go. The
// words are initiative.MorningBrief / initiative.EveningRecap's fixed
// frames; nothing is summarized by a model, so no user text goes to one.

// initiativeDaySources reads what both briefs need. Every function is
// optional; a missing one reads as nothing.
type initiativeDaySources struct {
	Store        *session.Store
	CronJobs     func() ([]cron.Job, error)
	WorkspaceDir string
	Location     *time.Location
}

func (s initiativeDaySources) loc() *time.Location {
	if s.Location == nil {
		return time.Local
	}
	return s.Location
}

// read lists sessions and focus pipelines. Errors read as none.
func (s initiativeDaySources) read() ([]session.Session, []focuspipeline.Pipeline) {
	if s.Store == nil {
		return nil, nil
	}
	sessions, err := s.Store.List()
	if err != nil {
		return nil, nil
	}
	var pipelines []focuspipeline.Pipeline
	if focus := focusStoreFor(s.Store); focus != nil {
		pipelines, _ = focus.List()
	}
	return sessions, pipelines
}

func (s initiativeDaySources) jobs() []cron.Job {
	if s.CronJobs == nil {
		return nil
	}
	jobs, _ := s.CronJobs()
	return jobs
}

// morning is the morning brief's facts at now: the rest of today's cron
// runs, yesterday's daily log in counts, and the work still open.
func (s initiativeDaySources) morning(now time.Time) initiative.MorningFacts {
	sessions, pipelines := s.read()
	local := now.In(s.loc())
	dayStart := localDayStart(local)
	return initiative.MorningFacts{
		Cron:      todayCronRuns(s.jobs(), now, dayStart.AddDate(0, 0, 1)),
		Yesterday: readDayLog(s.WorkspaceDir, dayStart.AddDate(0, 0, -1)),
		Open:      openWork(sessions, pipelines),
	}
}

// recap is the evening recap's facts at now, nil when there is nothing to
// say: work finished today, cron jobs whose run today failed, and the work
// still open.
func (s initiativeDaySources) recap(now time.Time) *initiative.RecapFacts {
	sessions, pipelines := s.read()
	return recapFacts(now.In(s.loc()), sessions, pipelines, s.jobs())
}

// recapFacts is recap over what was read, now in the brief's zone.
func recapFacts(now time.Time, sessions []session.Session, pipelines []focuspipeline.Pipeline, jobs []cron.Job) *initiative.RecapFacts {
	dayStart := localDayStart(now)
	today := func(t time.Time) bool { return !t.Before(dayStart) && t.Before(dayStart.AddDate(0, 0, 1)) }
	facts := &initiative.RecapFacts{Open: openWork(sessions, pipelines)}
	visible, titles := visibleSessions(sessions)
	type dated struct {
		at   time.Time
		item initiative.WorkItem
	}
	var done []dated
	withPipeline := map[string]bool{}
	for _, p := range pipelines {
		withPipeline[p.SessionID] = true
		if !visible[p.SessionID] || p.FinishedAt == nil || !today(*p.FinishedAt) {
			continue
		}
		done = append(done, dated{*p.FinishedAt, initiative.WorkItem{Title: workTitle(titles[p.SessionID], p.Goal), Href: "/console/focus/" + p.SessionID}})
	}
	for _, sess := range sessions {
		g := sess.Goal
		if !visible[sess.ID] || withPipeline[sess.ID] || g == nil || g.Status != session.SessionGoalStatusSatisfied || g.LastJudgedAt == nil || !today(*g.LastJudgedAt) {
			continue
		}
		done = append(done, dated{*g.LastJudgedAt, initiative.WorkItem{Title: workTitle(sess.Title, g.Description), Href: "/console/chat/" + sess.ID}})
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].at.Before(done[j].at) })
	for _, d := range done {
		facts.Done = append(facts.Done, d.item)
	}
	for _, j := range jobs {
		if j.LastRunAt != nil && today(*j.LastRunAt) && strings.TrimSpace(j.LastRunError) != "" {
			facts.Failed = append(facts.Failed, initiative.WorkItem{Title: workTitle(j.Name, j.ID), Href: "/console/cron"})
		}
	}
	if facts.Empty() {
		return nil
	}
	return facts
}

// openWork lists the work still open, newest first: every unfinished focus
// pipeline of a visible session (with its stage), then every visible
// session's active goal that has no pipeline of its own.
func openWork(sessions []session.Session, pipelines []focuspipeline.Pipeline) []initiative.WorkItem {
	visible, titles := visibleSessions(sessions)
	type dated struct {
		at   time.Time
		item initiative.WorkItem
	}
	var items []dated
	withPipeline := map[string]bool{}
	for _, p := range pipelines {
		withPipeline[p.SessionID] = true
		if !visible[p.SessionID] || p.FinishedAt != nil || focuspipeline.Finished(p) {
			continue
		}
		items = append(items, dated{p.UpdatedAt, initiative.WorkItem{
			Title: workTitle(titles[p.SessionID], p.Goal),
			Stage: focusStageName(p),
			Href:  "/console/focus/" + p.SessionID,
		}})
	}
	for _, sess := range sessions {
		if !visible[sess.ID] || withPipeline[sess.ID] || !sess.Goal.IsActive() {
			continue
		}
		items = append(items, dated{sess.UpdatedAt, initiative.WorkItem{Title: workTitle(sess.Title, sess.Goal.Description), Href: "/console/chat/" + sess.ID}})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].at.After(items[j].at) })
	out := make([]initiative.WorkItem, 0, len(items))
	for _, it := range items {
		out = append(out, it.item)
	}
	return out
}

// todayCronRuns lists the enabled jobs that still run before end: timed
// runs in order, then interval jobs by name.
func todayCronRuns(jobs []cron.Job, now, end time.Time) []initiative.CronRun {
	var timed, every []initiative.CronRun
	for _, j := range jobs {
		at, interval, ok := j.NextRunIn(now, end)
		if !ok {
			continue
		}
		run := initiative.CronRun{Name: workTitle(j.Name, j.ID), At: at, Every: interval}
		if interval > 0 {
			every = append(every, run)
		} else {
			timed = append(timed, run)
		}
	}
	sort.SliceStable(timed, func(i, j int) bool { return timed[i].At.Before(timed[j].At) })
	sort.SliceStable(every, func(i, j int) bool { return every[i].Name < every[j].Name })
	return append(timed, every...)
}

// readDayLog counts one day's daily log (memory/YYYY-MM-DD.md, written by
// memory.AppendDailyLog): chat turns and the sessions they came from, and
// cron runs. Only the line prefixes are read; a missing file is an empty
// day.
func readDayLog(workspaceDir string, day time.Time) initiative.DayLog {
	var out initiative.DayLog
	if strings.TrimSpace(workspaceDir) == "" {
		return out
	}
	f, err := os.Open(filepath.Join(workspaceDir, "memory", day.Format("2006-01-02")+".md"))
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sessions := map[string]bool{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "chat session="):
			out.ChatTurns++
			id, _, _ := strings.Cut(strings.TrimPrefix(line, "chat session="), " ")
			if id = strings.TrimSpace(id); id != "" {
				sessions[id] = true
			}
		case strings.HasPrefix(line, "cron job="):
			out.CronRuns++
		}
	}
	out.Sessions = len(sessions)
	return out
}

// visibleSessions are the sessions a brief may name — not hidden, not a
// worker — with their titles.
func visibleSessions(sessions []session.Session) (map[string]bool, map[string]string) {
	visible := map[string]bool{}
	titles := map[string]string{}
	for _, s := range sessions {
		if s.Hidden || strings.TrimSpace(s.Kind) == "worker" {
			continue
		}
		visible[s.ID] = true
		titles[s.ID] = s.Title
	}
	return visible, titles
}

// workTitle is the first non-empty of title and fallback, on one line.
func workTitle(title, fallback string) string {
	if t := oneLine(title); t != "" {
		return t
	}
	return oneLine(fallback)
}

func localDayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
