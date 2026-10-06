package focuspipeline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Block tags a focus turn ends with (ADR §6). The payload between the tags
// is JSON.
const (
	TagPlan     = "focus-plan"
	TagReport   = "focus-report"
	TagFindings = "focus-findings"
	TagPR       = "focus-pr"
)

// Blocks is what one assistant message reported.
type Blocks struct {
	Plan     *Plan
	Report   *Report
	Findings []Finding
	PR       *PRDraft
	// Errors are diagnostics for blocks that were present but malformed.
	Errors []string
}

// Report is a stage report: a short summary, the questions the agent needs
// answered, and the risks it sees.
type Report struct {
	Summary   string     `json:"summary"`
	Decisions []Decision `json:"decisions,omitempty"`
	Risks     []string   `json:"risks,omitempty"`
	// TasksDone is the build stage's claim that every approved task is
	// complete. It only counts together with passing verification.
	TasksDone bool `json:"tasks_done,omitempty"`
}

// Decision is one question the agent asks with its options.
type Decision struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// PRDraft is the pull request the pr stage drafts.
type PRDraft struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Finding is one review finding.
type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Title    string `json:"title"`
	Scenario string `json:"scenario"`
	// Excerpt is the diff around File:Line since the pipeline's base
	// commit, added by the server (never by the model) for the card.
	Excerpt string `json:"excerpt,omitempty"`
}

// tagPattern matches any focus open or close tag; group 1 is "/" for a
// close tag, group 2 the name after "focus-".
var tagPattern = regexp.MustCompile(`<(/?)focus-([a-z_]+)>`)

// rawBlock is one complete block outside code fences.
type rawBlock struct {
	tag        string // e.g. "focus-plan"
	body       string
	start, end int // byte span of the whole block, tags included
}

// ParseBlocks reads the focus blocks of an assistant message. For each kind
// the last well-formed block outside ``` fences wins; a malformed one adds a
// diagnostic to Errors and is otherwise ignored. Unknown JSON fields are
// ignored.
func ParseBlocks(text string) Blocks {
	var out Blocks
	for _, b := range scanBlocks(text) {
		switch b.tag {
		case TagPlan:
			plan, err := parsePlan(b.body)
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", b.tag, err))
				continue
			}
			out.Plan = &plan
		case TagReport:
			var report Report
			if err := decodeObject(b.body, &report); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", b.tag, err))
				continue
			}
			report.Summary = strings.TrimSpace(report.Summary)
			out.Report = &report
		case TagPR:
			var draft PRDraft
			if err := decodeObject(b.body, &draft); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", b.tag, err))
				continue
			}
			draft.Title = strings.TrimSpace(draft.Title)
			if draft.Title == "" {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: title is required", b.tag))
				continue
			}
			out.PR = &draft
		case TagFindings:
			var findings []Finding
			body := strings.TrimSpace(b.body)
			if !strings.HasPrefix(body, "[") {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: want a JSON array", b.tag))
				continue
			}
			if err := json.Unmarshal([]byte(body), &findings); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", b.tag, err))
				continue
			}
			if findings == nil {
				findings = []Finding{}
			}
			for i := range findings {
				findings[i].Excerpt = "" // the server's alone, never the model's
			}
			out.Findings = findings
		}
	}
	return out
}

// StripBlocks returns text without its <focus-*> blocks. Blocks quoted in
// ``` fences are content and stay.
func StripBlocks(text string) string {
	blocks := scanBlocks(text)
	if len(blocks) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, blk := range blocks {
		b.WriteString(text[last:blk.start])
		last = blk.end
	}
	b.WriteString(text[last:])
	return strings.TrimSpace(collapseBlankLines(b.String()))
}

func decodeObject(body string, v any) error {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "{") {
		return fmt.Errorf("want a JSON object")
	}
	return json.Unmarshal([]byte(body), v)
}

// parsePlan decodes a plan and cleans it. Its stages are fitted to the
// pipeline's template when the plan reaches a pipeline (Pipeline.fitPlan).
func parsePlan(body string) (Plan, error) {
	var plan Plan
	if err := decodeObject(body, &plan); err != nil {
		return Plan{}, err
	}
	return normalizePlan(plan)
}

func normalizePlan(plan Plan) (Plan, error) {
	plan.Goal = strings.TrimSpace(plan.Goal)
	tasks := make([]PlanTask, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		task.Title = strings.TrimSpace(task.Title)
		task.Done = strings.TrimSpace(task.Done)
		if task.Title != "" {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 {
		return Plan{}, fmt.Errorf("plan has no tasks")
	}
	plan.Tasks = tasks
	plan.Verify = cleanStrings(plan.Verify)
	plan.E2E = cleanStrings(plan.E2E)
	for i := range plan.Tasks {
		plan.Tasks[i].Stage = StageID(strings.TrimSpace(string(plan.Tasks[i].Stage)))
	}
	return plan, nil
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// scanBlocks finds every complete focus block outside code fences, in
// order. A close tag pairs with an open tag of the same name before it (and
// after the previous block): the nearest one whose body is valid JSON, else
// the nearest one. So an inline mention of an open tag — in prose before the
// block, or quoted inside the block's own JSON, of any tag — never swallows
// or splits the real block. A block never spans a fence, and tags inside a
// block found are not reused.
func scanBlocks(text string) []rawBlock {
	var out []rawBlock
	for _, seg := range unfencedSegments(text) {
		part := text[seg[0]:seg[1]]
		opens := map[string][][2]int{} // tag → [start, end) of its open tags
		for _, m := range tagPattern.FindAllStringSubmatchIndex(part, -1) {
			tag := "focus-" + part[m[4]:m[5]]
			if m[3] == m[2] { // open tag
				opens[tag] = append(opens[tag], [2]int{m[0], m[1]})
				continue
			}
			candidates := opens[tag]
			if len(candidates) == 0 {
				continue
			}
			open := candidates[len(candidates)-1]
			for i := len(candidates) - 1; i >= 0; i-- {
				if json.Valid([]byte(strings.TrimSpace(part[candidates[i][1]:m[0]]))) {
					open = candidates[i]
					break
				}
			}
			out = append(out, rawBlock{
				tag:   tag,
				body:  part[open[1]:m[0]],
				start: seg[0] + open[0],
				end:   seg[0] + m[1],
			})
			clear(opens)
		}
	}
	return out
}

// unfencedSegments returns the byte ranges of text outside ``` fenced code
// blocks. An unclosed fence runs to the end of the text.
func unfencedSegments(text string) [][2]int {
	var segs [][2]int
	inFence := false
	segStart := 0
	pos := 0
	for pos <= len(text) {
		lineEnd := strings.IndexByte(text[pos:], '\n')
		next := len(text) + 1
		line := text[pos:]
		if lineEnd >= 0 {
			line = text[pos : pos+lineEnd]
			next = pos + lineEnd + 1
		}
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inFence {
				segStart = min(next, len(text))
			} else {
				segs = append(segs, [2]int{segStart, pos})
			}
			inFence = !inFence
		}
		pos = next
	}
	if !inFence {
		segs = append(segs, [2]int{segStart, len(text)})
	}
	return segs
}

var blankLines = regexp.MustCompile(`\n{3,}`)

func collapseBlankLines(s string) string {
	return blankLines.ReplaceAllString(s, "\n\n")
}
