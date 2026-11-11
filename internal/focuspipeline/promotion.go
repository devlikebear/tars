package focuspipeline

import (
	"slices"
	"strings"
	"time"
	"unicode"
)

// Verify promotion (tars#1292, docs/decisions/focus-mode.md §5.4). The
// harness proposal (harness.go) turns a finished pipeline's facts into
// prose and skills — instructions a model may miss again. Promotion is the
// stronger rung of the same ladder: a defect that keeps coming back in the
// same repository becomes a check in the repository's default verify list,
// decided by an exit code from then on.
//
// A pipeline that opted in when it was created (Pipeline.Promotion) has,
// once it finishes with its merge stage done, its facts
// (CollectHarnessEvidence) turned into deterministic recurrence keys
// (RecurrenceKey: a finding's file pattern and normalized title, a failed
// verify command, a failed CI check's name) and recorded in the
// repository's recurrence ledger, next to its harness library
// (promotion_ledger.go). A key seen in PromotionThreshold of the last
// PromotionWindow recorded pipelines is a candidate. No model groups
// meanings: two findings recur only when their keys are equal.
//
// Only when a candidate exists is a model asked (promotion_check.go) for
// the check that catches it — a lint rule, a test, a short script — and the
// server keeps a check only after running it itself: it must fail on the
// work tree the fix turn started from and pass on the one it ended with
// (focusprobe.Replay, sandboxed in a temporary clone). A failed verify
// command needs neither: it already failed and then passed in the
// pipeline's own record. What survives is opened as a new development
// pipeline that adds the checks and lists their commands in the
// repository's .tars/verify.json; the server writes no file. Each promoted
// key goes to the harness library as a check, "enforced" once that pull
// request merges, so a harness proposal never offers the same defect again
// as prose (DropEnforcedLessons).

// Promotion outcomes (PromotionOptIn.Outcome).
const (
	// PromotionProposed: a promotion pipeline was opened
	// (PromotionOptIn.Proposal).
	PromotionProposed = "proposed"
	// PromotionNoCandidates: the pipeline's keys were recorded and none
	// recurs often enough yet; no model was asked.
	PromotionNoCandidates = "no_candidates"
	// PromotionNoChecks: candidates existed but no check survived — the
	// model proposed none that validated, or none failed on the defect and
	// passed on the fix when the server ran it.
	PromotionNoChecks = "no_checks"
	// PromotionFailed: the model call, the ledger or opening the pipeline
	// failed.
	PromotionFailed = "failed"
	// PromotionUnavailable: the server could not run the promotion at all.
	PromotionUnavailable = "unavailable"
)

// Notices the source pipeline gets: a promotion pipeline opened, or
// candidates existed and nothing came of them.
const (
	NoticePromotionProposed = "verify promotion proposed"
	NoticePromotionSkipped  = "verify promotion not proposed"
)

const (
	// PromotionWindow is how many of the repository's most recently
	// recorded pipelines a recurrence is counted over.
	PromotionWindow = 10
	// PromotionThreshold is how many of them must share a key for it to
	// become a candidate.
	PromotionThreshold = 3
	// PromotionMaxCandidates caps the candidates one run promotes (each
	// costs two sandboxed runs).
	PromotionMaxCandidates = 3
)

// RepoVerifyFile is the repository's default verify list, relative to its
// top level: {"verify": ["command", …]}.
const RepoVerifyFile = ".tars/verify.json"

// PromotionOptIn is a pipeline's verify promotion setting and record.
type PromotionOptIn struct {
	// Repo is the main checkout of the pipeline's repository, read when the
	// pipeline was created.
	Repo string `json:"repo"`
	// Outcome is how the promotion ended ("" until it ran: it runs once).
	Outcome string `json:"outcome,omitempty"`
	// Proposal is the session of the promotion pipeline it opened.
	Proposal string     `json:"proposal,omitempty"`
	At       *time.Time `json:"at,omitempty"`
}

// PromotionDue reports whether a pipeline's verify promotion should run
// now: it opted in, has not run yet, and finished with its work merged.
func PromotionDue(p Pipeline) bool {
	return p.Promotion != nil && p.Promotion.Outcome == "" && Releasable(p)
}

// RecordPromotionOutcome stores how the pipeline's promotion ended, with a
// notice card when a pipeline opened or candidates came to nothing. It
// records once.
func RecordPromotionOutcome(p Pipeline, outcome, proposal string, details map[string]any, now time.Time) Pipeline {
	if p.Promotion == nil || p.Promotion.Outcome != "" {
		return p
	}
	next := p.clone()
	at := now.UTC()
	next.Promotion.Outcome, next.Promotion.Proposal, next.Promotion.At = outcome, proposal, &at
	payload := map[string]any{"outcome": outcome}
	for k, v := range details {
		payload[k] = v
	}
	switch outcome {
	case PromotionProposed:
		payload["session_id"] = proposal
		next.addCard(CardNotice, 0, NoticePromotionProposed, payload, now)
	case PromotionNoChecks, PromotionFailed, PromotionUnavailable:
		next.addCard(CardNotice, 0, NoticePromotionSkipped, payload, now)
	}
	next.UpdatedAt = now
	return next
}

// Recurrence key kinds (RecurrenceHit.Kind), the prefix of every key.
const (
	RecurrenceFinding = "finding"
	RecurrenceVerify  = "verify"
	RecurrenceCI      = "ci"
)

// recurrenceTitleWords caps the words a normalized title keeps.
const recurrenceTitleWords = 8

// recurrenceStopWords are dropped from a normalized title.
var recurrenceStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "in": true, "to": true, "for": true, "on": true,
	"at": true, "by": true, "with": true, "and": true, "or": true, "is": true, "are": true, "be": true,
	"from": true, "into": true, "this": true, "that": true, "it": true, "its": true, "when": true,
}

// RecurrenceKey is the deterministic key one evidence item recurs under,
// "" for an item that has none:
//
//	finding:<file pattern>|<normalized title>   a finding decided fix
//	verify:<command>                            a failed verify command
//	ci:<check name>                             a failed CI check
//
// The file pattern keeps a finding's folder and extension
// (internal/x/*.go; * without a file). The normalized title is lower case,
// without quoted or backticked text, numbers, identifiers carrying digits,
// and stop words, cut to recurrenceTitleWords words — so "Missing ko
// translation for `focus.notice`" and "Missing ko translation for
// \"chat.title\"" share one key. A merge conflict and a pull request comment
// from outside the repository have none: neither is a defect of the code a
// check could catch, and the latter's text is never read.
func RecurrenceKey(it EvidenceItem) string {
	switch it.Kind {
	case EvidenceVerifyFailure:
		if cmd := oneLine(it.Command); cmd != "" {
			return RecurrenceVerify + ":" + cmd
		}
	case EvidenceCIFailure:
		if name := oneLine(it.Check); name != "" {
			return RecurrenceCI + ":" + name
		}
	case EvidenceFinding:
		if it.Source == EvidenceSourcePRProbe || (it.Source == EvidenceSourcePRComment && it.Title == untrustedCommentTitle) {
			return ""
		}
		if title := normalizeFindingTitle(it.Title); title != "" {
			return RecurrenceFinding + ":" + findingFilePattern(it.File) + "|" + title
		}
	}
	return ""
}

// RecurrenceKind is the kind prefix of a key.
func RecurrenceKind(key string) string {
	kind, _, _ := strings.Cut(key, ":")
	return kind
}

func findingFilePattern(file string) string {
	file = strings.TrimSpace(strings.ReplaceAll(file, "\\", "/"))
	if file == "" {
		return "*"
	}
	dir, name := "", file
	if i := strings.LastIndex(file, "/"); i >= 0 {
		dir, name = file[:i+1], file[i+1:]
	}
	ext := ""
	if i := strings.LastIndex(name, "."); i > 0 {
		ext = name[i:]
	}
	return dir + "*" + ext
}

// normalizeFindingTitle is a title's recurrence form (RecurrenceKey).
func normalizeFindingTitle(title string) string {
	var b strings.Builder
	var closing rune
	for _, r := range strings.ToLower(title) {
		if closing != 0 {
			if r == closing {
				closing = 0
			}
			continue
		}
		switch r {
		case '`':
			closing = '`'
			continue
		case '"':
			closing = '"'
			continue
		case '“':
			closing = '”'
			continue
		}
		b.WriteRune(r)
	}
	words := strings.FieldsFunc(b.String(), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' })
	kept := make([]string, 0, recurrenceTitleWords)
	for _, w := range words {
		if len([]rune(w)) < 2 || recurrenceStopWords[w] || strings.ContainsFunc(w, func(r rune) bool { return unicode.IsDigit(r) || r == '_' }) {
			continue
		}
		kept = append(kept, w)
		if len(kept) == recurrenceTitleWords {
			break
		}
	}
	return strings.Join(kept, " ")
}

// RecurrenceHit is one key a finished pipeline ran into, with what the
// prompt and the replay need to know about it.
type RecurrenceHit struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Title, File, Command and Check come from the evidence item.
	Title   string `json:"title,omitempty"`
	File    string `json:"file,omitempty"`
	Command string `json:"command,omitempty"`
	Check   string `json:"check,omitempty"`
	// Scenario is a finding's scenario (kept for the prompt).
	Scenario string `json:"scenario,omitempty"`
	// CardID is the card the hit was read from, in its pipeline.
	CardID string `json:"card_id,omitempty"`
}

// RecurrenceHits are the distinct keys of a pipeline's evidence, in the
// order first seen.
func RecurrenceHits(ev HarnessEvidence) []RecurrenceHit {
	var out []RecurrenceHit
	seen := map[string]bool{}
	for _, it := range ev.Items {
		key := RecurrenceKey(it)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, RecurrenceHit{
			Key: key, Kind: RecurrenceKind(key), Title: it.Title, File: it.File,
			Command: strings.TrimSpace(it.Command), Check: it.Check, Scenario: it.Scenario, CardID: it.CardID,
		})
	}
	return out
}

// DropEnforcedLessons removes the harness edits whose every cited evidence
// item recurs under a key the harness library already enforces with a
// check (enforced: recurrence keys): a check decides that defect now, and
// prose about it would only repeat the check. Reason HarnessDropEnforced.
func DropEnforcedLessons(edits []HarnessEdit, ev HarnessEvidence, enforced map[string]bool) ([]HarnessEdit, []HarnessDrop) {
	if len(enforced) == 0 {
		return edits, nil
	}
	var kept []HarnessEdit
	var dropped []HarnessDrop
	for _, e := range edits {
		all := len(e.Evidence) > 0
		for _, id := range e.Evidence {
			it, ok := ev.Item(id)
			if !ok || !enforced[RecurrenceKey(it)] {
				all = false
				break
			}
		}
		if all {
			dropped = append(dropped, HarnessDrop{Path: e.Path, Reason: HarnessDropEnforced})
			continue
		}
		kept = append(kept, e)
	}
	return kept, dropped
}

// HarnessDropEnforced is an edit whose evidence a check already enforces.
const HarnessDropEnforced = "enforced"

// MergeRepoVerify is list with every repository verify command it lacks
// appended, in the repository list's order: a plan or an edit can add
// commands to the repository's default verify, never leave one out.
func MergeRepoVerify(list, repo []string) []string {
	if len(repo) == 0 {
		return list
	}
	out := slices.Clone(list)
	for _, cmd := range repo {
		if !slices.Contains(out, cmd) {
			out = append(out, cmd)
		}
	}
	return out
}
