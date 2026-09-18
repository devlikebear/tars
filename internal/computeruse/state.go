package computeruse

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	// MaxChoiceElements is Jev's 255-option ceiling minus one slot for "none".
	MaxChoiceElements = 254
	maxLabelRunes     = 60
	maxValueRunes     = 40
	maxRecentActions  = 6
)

// RenderOptions controls what the rendered state is allowed to disclose.
type RenderOptions struct {
	ExposeValues bool
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ElementLine renders one element as the model sees it, e.g.
// [e4] AXTextField 'Search' value='x' enabled
func ElementLine(el Element, exposeValues bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[e%d] %s '%s'", el.Index, el.Role, truncateRunes(strings.ReplaceAll(el.Label, "\n", " "), maxLabelRunes))
	if exposeValues && !el.Secure && el.Role != "" && (el.Value != "" || isTextRole(el.Role)) {
		fmt.Fprintf(&b, " value='%s'", truncateRunes(strings.ReplaceAll(el.Value, "\n", " "), maxValueRunes))
	}
	if el.Enabled {
		b.WriteString(" enabled")
	} else {
		b.WriteString(" disabled")
	}
	if el.Selected != nil {
		if *el.Selected {
			b.WriteString(" checked")
		} else {
			b.WriteString(" unchecked")
		}
	}
	return b.String()
}

func isTextRole(role string) bool {
	r := strings.ToLower(role)
	return strings.Contains(r, "textfield") || strings.Contains(r, "textarea") || strings.Contains(r, "searchfield") || strings.Contains(r, "combobox")
}

// RenderState builds the Jev state block. Only input KEYS are listed; values
// never leave the process. Elements beyond MaxChoiceElements are cut so the
// target choice stays within Jev's 255-option limit (one slot for "none").
func RenderState(req Request, snap Snapshot, trace []TraceStep, opts RenderOptions) (string, []Element) {
	var b strings.Builder
	fmt.Fprintf(&b, "GOAL: %s\n", strings.TrimSpace(req.Goal))
	if len(req.Inputs) > 0 {
		keys := make([]string, 0, len(req.Inputs))
		for k := range req.Inputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(&b, "INPUTS AVAILABLE: %s\n", strings.Join(keys, ", "))
	}
	// The window title is deliberately omitted: it is often a document or
	// message subject, which is content the goal never asked us to disclose.
	fmt.Fprintf(&b, "APP: %s\n", snap.Window.App)
	if len(trace) > 0 {
		b.WriteString("RECENT ACTIONS (newest last):\n")
		start := 0
		if len(trace) > maxRecentActions {
			start = len(trace) - maxRecentActions
		}
		for _, ts := range trace[start:] {
			line := ts.Op
			if ts.InputKey != "" {
				line += "[input:" + ts.InputKey + "]"
			}
			if ts.Target != "" {
				line += " " + ts.Target
			}
			fmt.Fprintf(&b, "  %d. %s → %s\n", ts.Step, line, ts.Effect)
		}
	}
	candidates := make([]Element, 0, len(snap.Elements))
	for _, el := range snap.Elements {
		if IsMenuRole(el.Role) || isBareContainer(el) {
			continue
		}
		candidates = append(candidates, el)
	}
	// Elements the driver truncated are still "there" for the hidden count.
	total := len(candidates) + (snap.TotalElements - len(snap.Elements))
	shown := candidates
	if len(shown) > MaxChoiceElements {
		shown = shown[:MaxChoiceElements]
	}
	fmt.Fprintf(&b, "SCREEN (%d elements):\n", total)
	for _, el := range shown {
		b.WriteString(ElementLine(el, opts.ExposeValues))
		b.WriteByte('\n')
	}
	if hidden := total - len(shown); hidden > 0 {
		fmt.Fprintf(&b, "(…%d more elements not shown; choose look to see them)\n", hidden)
	}
	return b.String(), shown
}

// IsMenuRole reports application-menu nodes. The AX walk of any macOS window
// reaches the whole menu bar (including Recent Items file names); those are
// never valid click targets by element index and must not leave the machine.
func IsMenuRole(role string) bool {
	r := strings.ToLower(role)
	return strings.HasPrefix(r, "axmenu")
}

// isBareContainer drops unlabeled, valueless structural nodes (AXWindow,
// AXToolbar, AXScrollArea, AXGroup, …) that the model can never act on.
func isBareContainer(el Element) bool {
	return el.Label == "" && el.Value == "" && !isTextRole(el.Role)
}

// ElementHash fingerprints what the snapshot shows, ignoring element tokens so
// a re-snapshot of an unchanged screen hashes the same.
func ElementHash(snap Snapshot) string {
	h := sha256.New()
	for _, el := range snap.Elements {
		fmt.Fprintf(h, "%s|%s|%s|%v\n", el.Role, el.Label, el.Value, el.Enabled)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
