package focuspipeline

import (
	"fmt"
	"strings"
)

// Decision answers (ADR §4, "a decision card pauses the loop"). The
// decisions one turn asked are answered as one turn: the last answer sends
// every answer. An answer carries its card so the server can tell, before
// sending, whether its stage and round are still current: an answer that
// arrives after the pipeline moved on is never sent as a turn of a stage it
// was not asked in.

// Answer is the developer's answer to one decision card.
type Answer struct {
	CardID   string `json:"card_id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// NoticeLateAnswers is the title of the notice a late answer leaves when no
// turn follows to carry it.
const NoticeLateAnswers = "decision answers arrived after their stage moved on"

// AnswersPrompt is the turn that delivers answers: "question → answer" for
// one, a list for several.
func AnswersPrompt(answers []Answer) string {
	if len(answers) == 1 {
		return answers[0].Question + " → " + answers[0].Answer
	}
	var b strings.Builder
	b.WriteString("Answers to your questions:")
	for _, a := range answers {
		fmt.Fprintf(&b, "\n- %s → %s", a.Question, a.Answer)
	}
	return b.String()
}

// LateAnswersContext introduces answers that arrived after their stage or
// round ended, for the next turn's prompt: context, not an instruction for
// the current stage.
func LateAnswersContext(answers []Answer) string {
	var b strings.Builder
	b.WriteString("For context — answers to questions from an earlier step, given after it ended (apply them only where they still matter):")
	for _, a := range answers {
		fmt.Fprintf(&b, "\n- %s → %s", a.Question, a.Answer)
	}
	return b.String()
}

// SplitStaleAnswers splits answers into those whose decision card belongs to
// the pipeline's current stage and round, and the rest (card gone, another
// stage, another round, or the pipeline no longer active). A card from
// before cards recorded their round counts by stage alone.
func SplitStaleAnswers(p Pipeline, answers []Answer) (fresh, stale []Answer) {
	stage, _ := p.Stage(p.Current)
	for _, a := range answers {
		i := p.cardIndex(a.CardID)
		ok := p.Active() && i >= 0 && p.Cards[i].Stage == p.Current &&
			(p.Cards[i].Iteration == 0 || p.Cards[i].Iteration == stage.Iteration)
		if ok {
			fresh = append(fresh, a)
		} else {
			stale = append(stale, a)
		}
	}
	return fresh, stale
}

// turnAnswers returns the answers to every decision card the turn of card
// asked, or ok false while one of them is still open.
func turnAnswers(p Pipeline, card Card) ([]Answer, bool) {
	var answers []Answer
	for _, c := range p.Cards {
		if c.Kind != CardDecision || c.Stage != card.Stage || c.Turn != card.Turn {
			continue
		}
		if c.State != CardDecided {
			return nil, false
		}
		answers = append(answers, Answer{CardID: c.ID, Question: c.Title, Answer: c.Decision})
	}
	return answers, true
}
