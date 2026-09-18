package computeruse

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/jev"
)

const (
	OpClick       = "click"
	OpType        = "type"
	OpSetValue    = "set_value"
	OpPressEnter  = "press_enter"
	OpPressEscape = "press_escape"
	OpScrollDown  = "scroll_down"
	OpScrollUp    = "scroll_up"
	OpLook        = "look"
	OpDone        = "done"
	OpNone        = "none"
)

var opCriteria = map[string]string{
	OpClick:       "press or activate the target element",
	OpType:        "type one of the available inputs into the target text field",
	OpSetValue:    "set the target popup, checkbox, or slider to the needed value",
	OpPressEnter:  "press Return to confirm the focused control",
	OpPressEscape: "press Escape to dismiss a dialog or menu",
	OpScrollDown:  "scroll the window down to reveal more content",
	OpScrollUp:    "scroll the window up",
	OpLook:        "inspect more of the screen before acting (hidden or truncated elements)",
	OpDone:        "the GOAL is already fully achieved on the current screen",
	OpNone:        "no safe or useful action is available",
}

// BuildQuestions is the whole per-step Jev request. Screen content lives in
// the state only; criteria hold element lines (no values) so a hostile label
// cannot become an instruction.
func BuildQuestions(shown []Element, inputKeys []string) map[string]jev.Question {
	target := map[string]string{"none": "no element"}
	for _, el := range shown {
		target[fmt.Sprintf("e%d", el.Index)] = ElementLine(el, false)
	}
	qs := map[string]jev.Question{
		"op":     jev.Choice("Which single operation moves toward the GOAL next?", opCriteria),
		"target": jev.Choice("Which element should the operation act on? Pick none for operations that need no element.", target),
		"risky": jev.Noul("Would the next action be hard to undo?",
			"deletes, sends, pays, submits, or changes system settings irreversibly",
			"navigation, selection, typing, or otherwise reversible"),
		"done": jev.Noul("Is the GOAL already achieved on the current screen?", "the goal's end state is visible now", "not yet"),
	}
	if len(inputKeys) > 0 {
		ik := map[string]string{"none": "no input needed"}
		for _, k := range inputKeys {
			ik[k] = "the input named " + k
		}
		qs["input_key"] = jev.Choice("If typing, which available input should be typed?", ik)
	}
	return qs
}

type Decision struct {
	Op               string
	OpConfidence     float64
	TargetIndex      int
	TargetConfidence float64
	InputKey         string
	Risky            float64
	Done             float64
}

func ParseDecision(resp jev.Response) (Decision, error) {
	get := func(k string) (jev.Answer, error) {
		a, ok := resp.Answers[k]
		if !ok {
			return jev.Answer{}, fmt.Errorf("computeruse: jev answer %q missing", k)
		}
		return a, nil
	}
	op, err := get("op")
	if err != nil {
		return Decision{}, err
	}
	target, err := get("target")
	if err != nil {
		return Decision{}, err
	}
	risky, err := get("risky")
	if err != nil {
		return Decision{}, err
	}
	done, err := get("done")
	if err != nil {
		return Decision{}, err
	}
	d := Decision{Op: op.Choice, OpConfidence: op.Confidence, TargetConfidence: target.Confidence, Risky: risky.Noul, Done: done.Noul}
	if strings.HasPrefix(target.Choice, "e") {
		if n, err := strconv.Atoi(target.Choice[1:]); err == nil && n > 0 {
			d.TargetIndex = n
		}
	}
	if ik, ok := resp.Answers["input_key"]; ok && ik.Choice != "none" {
		d.InputKey = ik.Choice
	}
	if _, known := opCriteria[d.Op]; !known {
		return Decision{}, fmt.Errorf("computeruse: unknown op %q", d.Op)
	}
	return d, nil
}

func NeedsTarget(op string) bool {
	return op == OpClick || op == OpType || op == OpSetValue
}

func Compatible(op, role string) bool {
	r := strings.ToLower(role)
	switch op {
	case OpType:
		return isTextRole(r)
	case OpSetValue:
		return strings.Contains(r, "popup") || strings.Contains(r, "checkbox") || strings.Contains(r, "slider") || strings.Contains(r, "combobox") || strings.Contains(r, "radio")
	case OpClick:
		return true
	default:
		return true
	}
}
