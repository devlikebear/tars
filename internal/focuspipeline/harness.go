package focuspipeline

import (
	"strconv"
	"strings"
	"time"
)

// Harness proposals (tars#1291, docs/decisions/focus-mode.md §5.2). A
// pipeline that opted in when it was created (Pipeline.Harness) has, once it
// finishes with its merge stage done, the facts it ran into collected
// (CollectHarnessEvidence: findings decided fix, failed verification, failed
// CI checks — read from its own cards, never from a model). When there is at
// least one, a model on the focus_harness role proposes edits to the
// repository's harness (its .tars skills and their scripts, a section of
// CLAUDE.md/AGENTS.md), each citing one of those facts; an edit that cites
// none is dropped (ValidateHarnessLessons). What survives is opened as a new
// development pipeline on the repository (Pipeline.HarnessFrom): the server
// never writes a harness file itself, a person merges the pull request or
// not. The repository's harness library (HarnessLibrary) remembers every
// lesson proposed and adopted, so the same lesson is not proposed twice.

// Harness proposal outcomes (HarnessOptIn.Outcome).
const (
	// HarnessProposed: a proposal pipeline was opened (HarnessOptIn.Proposal).
	HarnessProposed = "proposed"
	// HarnessNoEvidence: the pipeline ran clean — nothing fixed, nothing
	// failed — so there was nothing to learn and no model was asked.
	HarnessNoEvidence = "no_evidence"
	// HarnessNoEdits: the model proposed nothing that survived validation
	// (no citation, a path outside the harness, a lesson already proposed).
	HarnessNoEdits = "no_edits"
	// HarnessFailed: the model call or opening the proposal failed.
	HarnessFailed = "failed"
	// HarnessUnavailable: the server could not run the proposal at all (no
	// focus_harness role, no repository).
	HarnessUnavailable = "unavailable"
)

// NoticeHarnessProposed is the notice the source pipeline gets when its
// proposal pipeline opens, and NoticeHarnessSkipped the one it gets when
// the model was asked and nothing came of it.
const (
	NoticeHarnessProposed = "harness improvement proposed"
	NoticeHarnessSkipped  = "harness improvement not proposed"
)

// HarnessOptIn is a pipeline's harness proposal setting and record.
type HarnessOptIn struct {
	// Repo is the main checkout of the pipeline's repository, read when the
	// pipeline was created: the session's own folder may be a worktree
	// that is gone by the time the pipeline finishes.
	Repo string `json:"repo"`
	// Outcome is how the proposal ended ("" until it ran: it runs once).
	Outcome string `json:"outcome,omitempty"`
	// Proposal is the session of the proposal pipeline it opened.
	Proposal string `json:"proposal,omitempty"`
	// At is when the outcome was recorded.
	At *time.Time `json:"at,omitempty"`
}

// HarnessSource marks a pipeline that is itself a harness proposal: where it
// came from and the library lessons it carries, adopted when its pull
// request merges.
type HarnessSource struct {
	SessionID string   `json:"session_id"`
	Repo      string   `json:"repo"`
	Lessons   []string `json:"lessons"`
}

// RecordHarnessOutcome stores how the pipeline's harness proposal ended, with
// a notice card for the outcomes worth a look (a proposal opened, or a model
// that was asked and proposed nothing). It records once: a pipeline that
// already has an outcome is returned unchanged.
func RecordHarnessOutcome(p Pipeline, outcome, proposal string, details map[string]any, now time.Time) Pipeline {
	if p.Harness == nil || p.Harness.Outcome != "" {
		return p
	}
	next := p.clone()
	at := now.UTC()
	next.Harness.Outcome, next.Harness.Proposal, next.Harness.At = outcome, proposal, &at
	payload := map[string]any{"outcome": outcome}
	for k, v := range details {
		payload[k] = v
	}
	switch outcome {
	case HarnessProposed:
		payload["session_id"] = proposal
		next.addCard(CardNotice, 0, NoticeHarnessProposed, payload, now)
	case HarnessNoEdits, HarnessFailed, HarnessUnavailable:
		next.addCard(CardNotice, 0, NoticeHarnessSkipped, payload, now)
	}
	next.UpdatedAt = now
	return next
}

// HarnessDue reports whether a pipeline's harness proposal should run now:
// it opted in, has not run it yet, and finished with its work merged.
func HarnessDue(p Pipeline) bool {
	return p.Harness != nil && p.Harness.Outcome == "" && Releasable(p)
}

// Evidence kinds (EvidenceItem.Kind).
const (
	EvidenceFinding       = "finding"
	EvidenceVerifyFailure = "verify_failure"
	EvidenceCIFailure     = "ci_failure"
)

// Finding sources an evidence item names (EvidenceItem.Source). The
// pipeline's own review agent reports findings with no source; a PR probe
// finding is a merge conflict or a review comment.
const (
	EvidenceSourceAgent         = "agent"
	EvidenceSourceSecondOpinion = FindingSourceSecondOpinion
	EvidenceSourceDeveloper     = FindingSourceDeveloper
	EvidenceSourcePRProbe       = "pr_probe"
	EvidenceSourcePRComment     = "pr_comment"
)

// evidenceTextRunes caps an excerpt or scenario carried into evidence.
const evidenceTextRunes = 800

// EvidenceItem is one fact a harness lesson can cite.
type EvidenceItem struct {
	// ID is the citation key ("E1", "E2", …), in the order collected.
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	Stage StageID `json:"stage,omitempty"`
	// Source is a finding's origin (agent, second_opinion, developer,
	// pr_probe, pr_comment); empty for failures.
	Source   string `json:"source,omitempty"`
	Severity string `json:"severity,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Title    string `json:"title,omitempty"`
	Scenario string `json:"scenario,omitempty"`
	// Command, ExitCode and Excerpt describe a failed verification command.
	Command  string `json:"command,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Excerpt  string `json:"excerpt,omitempty"`
	// Check is a failed CI check's name.
	Check string `json:"check,omitempty"`
}

// StageRounds is how many rounds one stage took.
type StageRounds struct {
	Stage  StageID `json:"stage"`
	Kind   StageID `json:"kind"`
	Rounds int     `json:"rounds"`
}

// HarnessEvidence is what one finished pipeline ran into: the facts a lesson
// can cite (Items) and the context they came with (rounds, pushes, cost).
type HarnessEvidence struct {
	SessionID string         `json:"session_id"`
	Goal      string         `json:"goal"`
	Items     []EvidenceItem `json:"items"`
	Rounds    []StageRounds  `json:"rounds,omitempty"`
	// Pushes counts the pull request's pushes: the one that opened it and
	// one per pr_review fix round.
	Pushes int `json:"pushes"`
	// GoalPushes is goal mode's spent budget (retries and pushes), 0 when
	// the pipeline never ran in goal mode.
	GoalPushes int `json:"goal_pushes,omitempty"`
	// CostUSD is the session's recorded cost, set by the server (0 when
	// unknown).
	CostUSD float64 `json:"cost_usd,omitempty"`
	// PRURL is the merged pull request.
	PRURL string `json:"pr_url,omitempty"`
}

// Empty reports whether there is nothing to learn from: no finding was
// fixed and nothing failed. Rounds, pushes and cost alone are not evidence.
func (e HarnessEvidence) Empty() bool { return len(e.Items) == 0 }

// Item returns the evidence item with the citation id, or false.
func (e HarnessEvidence) Item(id string) (EvidenceItem, bool) {
	id = strings.ToUpper(strings.TrimSpace(id))
	for _, it := range e.Items {
		if it.ID == id {
			return it, true
		}
	}
	return EvidenceItem{}, false
}

// CollectHarnessEvidence reads a pipeline's cards for the facts a harness
// lesson can cite: every finding decided fix (whoever reported it), every
// failed verification command (once per command and output), and every CI
// check that failed on the pull request (once per check name, decided or
// not — a failed check is a fact either way). An untrusted PR comment's text
// never becomes evidence: only that it was accepted. Pure: no model, no I/O.
func CollectHarnessEvidence(p Pipeline) HarnessEvidence {
	ev := HarnessEvidence{SessionID: p.SessionID, Goal: strings.TrimSpace(p.Goal)}
	seenFailures := map[string]bool{}
	seenChecks := map[string]bool{}
	add := func(it EvidenceItem) {
		it.ID = "E" + strconv.Itoa(len(ev.Items)+1)
		ev.Items = append(ev.Items, it)
	}
	for _, c := range p.Cards {
		switch c.Kind {
		case CardFinding:
			var f PRFinding
			if !decodePayload(c.Payload, &f) {
				continue
			}
			if strings.HasPrefix(f.Key, "check:") {
				name := strings.TrimPrefix(c.Title, "CI check failed: ")
				if name == "" || seenChecks[name] {
					continue
				}
				seenChecks[name] = true
				add(EvidenceItem{Kind: EvidenceCIFailure, Stage: c.Stage, Check: name, Title: c.Title})
				continue
			}
			if c.State != CardDecided || c.Decision != DecisionFix {
				continue
			}
			add(findingEvidence(c, f))
		case CardFailure:
			var fact FailureFact
			if !decodePayload(c.Payload, &fact) || strings.TrimSpace(fact.Command) == "" {
				continue
			}
			key := fact.key()
			if seenFailures[key] {
				continue
			}
			seenFailures[key] = true
			add(EvidenceItem{
				Kind: EvidenceVerifyFailure, Stage: c.Stage, Command: strings.TrimSpace(fact.Command),
				ExitCode: fact.ExitCode, TimedOut: fact.TimedOut,
				Excerpt: tailRunes(strings.TrimSpace(fact.Excerpt), evidenceTextRunes),
			})
		}
	}
	for _, s := range p.Stages {
		if s.Iteration > 0 {
			ev.Rounds = append(ev.Rounds, StageRounds{Stage: s.ID, Kind: s.KindOf(), Rounds: s.Iteration})
		}
	}
	if p.PR != nil {
		ev.PRURL = p.PR.URL
		ev.Pushes = 1
		if s, ok := p.Stage(StagePRReview); ok && s.Iteration > 1 {
			ev.Pushes += s.Iteration - 1
		}
	}
	if p.GoalMode != nil {
		ev.GoalPushes = p.GoalMode.Pushes
	}
	return ev
}

func findingEvidence(c Card, f PRFinding) EvidenceItem {
	it := EvidenceItem{
		Kind: EvidenceFinding, Stage: c.Stage, Severity: strings.TrimSpace(f.Severity),
		File: strings.TrimSpace(f.File), Line: f.Line, Title: strings.TrimSpace(c.Title),
		Scenario: tailRunes(strings.TrimSpace(f.Scenario), evidenceTextRunes),
	}
	switch {
	case f.Source == FindingSourceDeveloper:
		it.Source = EvidenceSourceDeveloper
	case f.Source == FindingSourceSecondOpinion:
		it.Source = EvidenceSourceSecondOpinion
	case strings.HasPrefix(f.Key, "conflict:"):
		it.Source = EvidenceSourcePRProbe
	case strings.HasPrefix(f.Key, "comment:"):
		it.Source = EvidenceSourcePRComment
		if !f.Trusted {
			// Someone outside the repository wrote it: what it says is not
			// carried into a prompt, only that the developer accepted it.
			it.Scenario = ""
			it.Title = "Pull request comment accepted for a fix"
		}
	default:
		it.Source = EvidenceSourceAgent
	}
	return it
}
