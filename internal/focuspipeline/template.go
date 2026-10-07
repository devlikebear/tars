package focuspipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Templates (ADR §4, "Templates"). A template is an ordered list of stages,
// each running with one of the development stages' behaviours (its kind):
//
//	plan       proposes the plan; G1 approves it
//	build      does the work task by task; verification decides the exit
//	review     reports findings; triage, a fix turn, verification
//	pr, pr_review, merge   the pull request stages
//
// So a template changes what the stages are called, how many work and
// review stages there are, and what each is told to do, while every
// transition stays decided by the same facts. The development template is
// the six stages every pipeline had before templates.

// DevTemplateID is the default template: the development loop.
const DevTemplateID = "dev"

// TemplateDirName is the workspace folder user templates are read from.
const TemplateDirName = "focus-templates"

// Limits on a template file.
const (
	maxTemplateStages      = 12
	maxTemplateInstruction = 4000
	maxTemplateFileBytes   = 64 << 10
)

// ErrInvalidTemplate is a template that cannot run.
var ErrInvalidTemplate = errors.New("invalid focus template")

// Template is a pipeline's shape.
type Template struct {
	ID          string          `json:"id" yaml:"id"`
	Name        string          `json:"name" yaml:"name"`
	Description string          `json:"description,omitempty" yaml:"description"`
	Stages      []TemplateStage `json:"stages" yaml:"stages"`
	// Builtin marks the templates shipped with the server; Source is the
	// file a user template was read from.
	Builtin bool   `json:"builtin" yaml:"-"`
	Source  string `json:"source,omitempty" yaml:"-"`
}

// TemplateStage is one stage of a template.
type TemplateStage struct {
	ID StageID `json:"id" yaml:"id"`
	// Kind is the behaviour the stage runs with; empty means the id.
	Kind  StageID `json:"kind,omitempty" yaml:"kind"`
	Label string  `json:"label,omitempty" yaml:"label"`
	// Instructions replace the kind's default instructions for the stage's
	// turns; FixInstructions those of a review-kind stage's fix turn.
	Instructions    string `json:"instructions,omitempty" yaml:"instructions"`
	FixInstructions string `json:"fix_instructions,omitempty" yaml:"fix_instructions"`
}

// TemplateDiagnostic is a template file that was not loaded, and why.
type TemplateDiagnostic struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

var (
	templateIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	stageIDPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

// fixedKinds are the kinds a pipeline holds at most once, under their own
// id: the machine addresses them by that id.
var fixedKinds = map[StageID]bool{StagePlan: true, StagePR: true, StagePRReview: true, StageMerge: true}

// Validate reports why a template cannot run: wrapped ErrInvalidTemplate.
func (t Template) Validate() error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidTemplate, fmt.Sprintf(format, args...))
	}
	if !templateIDPattern.MatchString(t.ID) {
		return fail("id %q must be lower-case letters, digits, - or _ (at most 32)", t.ID)
	}
	if strings.TrimSpace(t.Name) == "" {
		return fail("name is required")
	}
	if len(t.Stages) < 2 {
		return fail("a template needs the plan stage and at least one more")
	}
	if len(t.Stages) > maxTemplateStages {
		return fail("at most %d stages", maxTemplateStages)
	}
	seen := map[StageID]int{}
	for i, s := range t.Stages {
		if !stageIDPattern.MatchString(string(s.ID)) {
			return fail("stage id %q must be lower-case letters, digits or _ (at most 32)", s.ID)
		}
		if _, dup := seen[s.ID]; dup {
			return fail("stage %q is listed twice", s.ID)
		}
		seen[s.ID] = i
		kind := s.Kind
		if kind == "" {
			kind = s.ID
		}
		switch kind {
		case StagePlan, StageBuild, StageReview, StagePR, StagePRReview, StageMerge:
		default:
			return fail("stage %q: kind %q is not one of plan, build, review, pr, pr_review, merge", s.ID, kind)
		}
		if (fixedKinds[kind] || fixedKinds[s.ID]) && kind != s.ID {
			return fail("stage %q: the %s stage keeps its own id and kind", s.ID, kindOrID(kind, s.ID))
		}
		if len(s.Instructions) > maxTemplateInstruction || len(s.FixInstructions) > maxTemplateInstruction {
			return fail("stage %q: instructions are longer than %d bytes", s.ID, maxTemplateInstruction)
		}
		if strings.Contains(s.Instructions+s.FixInstructions, "<focus-") || strings.Contains(s.Instructions+s.FixInstructions, "</focus-") {
			return fail("stage %q: instructions must not contain <focus-…> tags", s.ID)
		}
	}
	if t.Stages[0].ID != StagePlan {
		return fail("the first stage must be plan")
	}
	pr, hasPR := seen[StagePR]
	for _, id := range []StageID{StagePRReview, StageMerge} {
		at, ok := seen[id]
		if !ok {
			continue
		}
		if !hasPR || at < pr {
			return fail("the %s stage needs the pr stage before it", id)
		}
	}
	if review, ok := seen[StagePRReview]; ok {
		if merge, ok := seen[StageMerge]; ok && merge < review {
			return fail("the merge stage comes after pr_review")
		}
	}
	return nil
}

func kindOrID(kind, id StageID) StageID {
	if fixedKinds[kind] {
		return kind
	}
	return id
}

// DevTemplate is the development loop: plan → build → review → pr →
// pr_review → merge, with each kind's default instructions.
func DevTemplate() Template {
	stages := make([]TemplateStage, 0, len(StageOrder))
	for _, id := range StageOrder {
		stages = append(stages, TemplateStage{ID: id})
	}
	return Template{
		ID: DevTemplateID, Name: "Development", Builtin: true, Stages: stages,
		Description: "Plan, build, review, pull request, CI review, merge.",
	}
}

// BuiltinTemplates are the templates shipped with the server, the
// development one first.
func BuiltinTemplates() []Template {
	return []Template{DevTemplate(), writingTemplate(), researchTemplate()}
}

func writingTemplate() Template {
	return Template{
		ID: "writing", Name: "Writing", Builtin: true,
		Description: "Fiction and long-form writing: outline, draft, revise.",
		Stages: []TemplateStage{
			{ID: StagePlan, Label: "Outline", Instructions: "Do not write the manuscript yet. Read what already exists in the folder, then propose a plan: " +
				"the premise, point of view and tone in the goal line; one task per chapter or scene in order, " +
				"each with what \"done\" means (the file it is written to, what happens in it, a rough length). " +
				"Verification commands are optional (a word count, a spell check); leave them empty when none apply. " +
				"The author approves or edits the plan before anything is written."},
			{ID: "draft", Kind: StageBuild, Label: "Draft", Instructions: "Write the approved chapters or scenes in order, each to the file its task names. " +
				"Keep to the outline, the point of view and the tone; do not summarize where a scene should be written. " +
				"If the story needs a decision only the author can make, ask it as a decision in the report instead of guessing."},
			{ID: "revise", Kind: StageReview, Label: "Revise", Instructions: "Read the whole draft as an editor. " +
				"Each finding names the file and line and says what fails the reader there and why: " +
				"a continuity error, a character acting against what was established, a scene that drags or is skipped, " +
				"an unclear sentence, a change of tone or point of view.",
				FixInstructions: "Revise only the passages the listed findings name; keep everything else as written."},
		},
	}
}

func researchTemplate() Template {
	return Template{
		ID: "research", Name: "Research", Builtin: true,
		Description: "Research a question: scope, investigate, write the report, check it.",
		Stages: []TemplateStage{
			{ID: StagePlan, Label: "Scope", Instructions: "Do not start the research yet. Propose a plan: the question and what is out of scope in the goal line; " +
				"tasks for the research stage (one per sub-question, each naming the kind of source that would answer it) with \"stage\":\"research\", " +
				"and tasks for the report stage (the report file and its sections) with \"stage\":\"report\". " +
				"Verification commands are optional; leave them empty when none apply. " +
				"The requester approves or edits the plan before the research starts."},
			{ID: "research", Kind: StageBuild, Label: "Research", Instructions: "Answer the approved sub-questions in order. " +
				"Use the sources you can actually reach (files in the folder, web search and fetch when available) and write what you find to a notes file: " +
				"each claim with its source (URL or file) and, for anything dated, the date. " +
				"Record what you could not find or verify as open, never as fact."},
			{ID: "report", Kind: StageBuild, Label: "Report", Instructions: "Write the report from the notes, to the file the plan names: " +
				"the answer first, then the evidence for it, the open questions, and the list of sources. " +
				"Every claim in the report must trace to a source in the notes; mark estimates and inferences as such."},
			{ID: "check", Kind: StageReview, Label: "Check", Instructions: "Check the report against the notes and the sources. " +
				"Each finding names the file and line of the report and says what is wrong there: " +
				"a claim with no source, a claim its source does not support, a number or date copied wrong, a conclusion the evidence does not carry, " +
				"a sub-question left unanswered.",
				FixInstructions: "Correct only what the listed findings name: fix the claim, add its source, or mark it as open. Keep the rest of the report as written."},
		},
	}
}

// LoadTemplates returns the built-in templates followed by the valid user
// templates of dir (<workspace>/focus-templates/*.yaml|*.yml|*.json, by file
// name), and a diagnostic for each file that was not loaded. A user template
// never replaces a built-in one. A missing folder is no error.
func LoadTemplates(dir string) ([]Template, []TemplateDiagnostic) {
	out := BuiltinTemplates()
	taken := map[string]bool{}
	for _, t := range out {
		taken[t.ID] = true
	}
	if strings.TrimSpace(dir) == "" {
		return out, nil
	}
	// Stat first: reading a file as a folder reports "not found" on Windows.
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, []TemplateDiagnostic{{Source: TemplateDirName, Error: err.Error()}}
	}
	if !info.IsDir() {
		return out, []TemplateDiagnostic{{Source: TemplateDirName, Error: "not a folder"}}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out, []TemplateDiagnostic{{Source: TemplateDirName, Error: err.Error()}}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml", ".json":
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
	}
	sort.Strings(names)
	var diags []TemplateDiagnostic
	for _, name := range names {
		t, err := readTemplateFile(filepath.Join(dir, name))
		if err == nil && taken[t.ID] {
			err = fmt.Errorf("%w: id %q is already taken", ErrInvalidTemplate, t.ID)
		}
		if err != nil {
			diags = append(diags, TemplateDiagnostic{Source: name, Error: err.Error()})
			continue
		}
		t.Source = name
		taken[t.ID] = true
		out = append(out, t)
	}
	return out, diags
}

// FindTemplate returns the template with id among the built-in ones and
// dir's; an empty id is the development template.
func FindTemplate(dir, id string) (Template, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return DevTemplate(), true
	}
	templates, _ := LoadTemplates(dir)
	for _, t := range templates {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

func readTemplateFile(path string) (Template, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Template{}, err
	}
	if info.Size() > maxTemplateFileBytes {
		return Template{}, fmt.Errorf("%w: file is larger than %d bytes", ErrInvalidTemplate, maxTemplateFileBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Template{}, err
	}
	var t Template
	if strings.EqualFold(filepath.Ext(path), ".json") {
		err = json.Unmarshal(raw, &t)
	} else {
		err = yaml.Unmarshal(raw, &t)
	}
	if err != nil {
		return Template{}, fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}
	t.ID = strings.TrimSpace(t.ID)
	if t.ID == "" {
		base := filepath.Base(path)
		t.ID = strings.TrimSuffix(base, filepath.Ext(base))
	}
	t.Name = strings.TrimSpace(t.Name)
	t.Description = strings.TrimSpace(t.Description)
	t.Builtin, t.Source = false, ""
	for i := range t.Stages {
		s := &t.Stages[i]
		s.ID = StageID(strings.TrimSpace(string(s.ID)))
		s.Kind = StageID(strings.TrimSpace(string(s.Kind)))
		s.Label = strings.TrimSpace(s.Label)
		s.Instructions = strings.TrimSpace(s.Instructions)
		s.FixInstructions = strings.TrimSpace(s.FixInstructions)
	}
	if err := t.Validate(); err != nil {
		return Template{}, err
	}
	return t, nil
}
