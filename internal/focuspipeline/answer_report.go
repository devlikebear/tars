package focuspipeline

import "time"

// Phase 3 of docs/decisions/mobile-remote.md: measure how much the developer
// answers from a phone, to decide native push and Android from use rather
// than guesses. Only cards that recorded where they were decided count; a
// card decided before that was recorded, or one the pipeline closed by
// itself, is not an answer from anywhere.

// AnswerCounts counts answers by where they were given.
type AnswerCounts struct {
	Phone int `json:"phone"`
	Desk  int `json:"desk"`
	Goal  int `json:"goal"`
}

func (c *AnswerCounts) add(by string) bool {
	switch by {
	case AnswerPhone:
		c.Phone++
	case AnswerDesk:
		c.Desk++
	case AnswerGoal:
		c.Goal++
	default:
		return false
	}
	return true
}

// AnswerReport is what was answered where since a time: gates (G1–G4,
// triage, blocked) apart from the decision and finding cards, and how many
// pipelines had any answer, and any from a phone.
type AnswerReport struct {
	Since          time.Time    `json:"since"`
	Gates          AnswerCounts `json:"gates"`
	Cards          AnswerCounts `json:"cards"`
	Pipelines      int          `json:"pipelines"`
	PhonePipelines int          `json:"phone_pipelines"`
}

// AnswerSummary counts the answers recorded at or after since.
func AnswerSummary(pipelines []Pipeline, since time.Time) AnswerReport {
	report := AnswerReport{Since: since.UTC()}
	for _, p := range pipelines {
		answered, phone := false, false
		for _, c := range p.Cards {
			if c.State != CardDecided || c.DecidedAt == nil || c.DecidedAt.Before(since) {
				continue
			}
			counts := &report.Cards
			if c.Kind == CardGate {
				counts = &report.Gates
			}
			if counts.add(c.DecidedBy) {
				answered = true
				phone = phone || c.DecidedBy == AnswerPhone
			}
		}
		if answered {
			report.Pipelines++
		}
		if phone {
			report.PhonePipelines++
		}
	}
	return report
}
