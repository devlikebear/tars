package focuspipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuiltinTemplatesAreValid(t *testing.T) {
	seen := map[string]bool{}
	for _, tpl := range BuiltinTemplates() {
		if err := tpl.Validate(); err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		if !tpl.Builtin || seen[tpl.ID] {
			t.Fatalf("template %+v", tpl)
		}
		seen[tpl.ID] = true
	}
	if !seen[DevTemplateID] || !seen["writing"] || !seen["research"] {
		t.Fatalf("built-in ids = %v", seen)
	}
}

func TestTemplateValidate(t *testing.T) {
	stages := func(s ...TemplateStage) []TemplateStage { return s }
	plan := TemplateStage{ID: StagePlan}
	work := TemplateStage{ID: "draft", Kind: StageBuild}
	cases := []struct {
		name string
		tpl  Template
		want string // substring of the error; "" = valid
	}{
		{"valid", Template{ID: "novel", Name: "Novel", Stages: stages(plan, work, TemplateStage{ID: "edit", Kind: StageReview})}, ""},
		{"valid with pr stages", Template{ID: "docs", Name: "Docs", Stages: stages(plan, work, TemplateStage{ID: StagePR}, TemplateStage{ID: StageMerge})}, ""},
		{"bad id", Template{ID: "No Spaces", Name: "x", Stages: stages(plan, work)}, "id"},
		{"no name", Template{ID: "x", Stages: stages(plan, work)}, "name is required"},
		{"plan only", Template{ID: "x", Name: "x", Stages: stages(plan)}, "at least one more"},
		{"plan not first", Template{ID: "x", Name: "x", Stages: stages(work, plan)}, "first stage must be plan"},
		{"duplicate stage", Template{ID: "x", Name: "x", Stages: stages(plan, work, work)}, "listed twice"},
		{"unknown kind", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "draft", Kind: "write"})}, "kind"},
		{"kind missing on a custom id", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "draft"})}, "kind"},
		{"bad stage id", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "Draft One", Kind: StageBuild})}, "stage id"},
		{"second plan", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "replan", Kind: StagePlan})}, "keeps its own id"},
		{"build under the pr id", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: StagePR, Kind: StageBuild})}, "keeps its own id"},
		{"merge without pr", Template{ID: "x", Name: "x", Stages: stages(plan, work, TemplateStage{ID: StageMerge})}, "needs the pr stage"},
		{"pr_review before pr", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: StagePRReview}, TemplateStage{ID: StagePR})}, "needs the pr stage"},
		{"merge before pr_review", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: StagePR}, TemplateStage{ID: StageMerge}, TemplateStage{ID: StagePRReview})}, "after pr_review"},
		{"focus tag in instructions", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "draft", Kind: StageBuild, Instructions: "end with </focus-stage>"})}, "tags"},
		{"instructions too long", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "draft", Kind: StageBuild, Instructions: strings.Repeat("a", maxTemplateInstruction+1)})}, "longer"},
		{"too many stages", Template{ID: "x", Name: "x", Stages: func() []TemplateStage {
			out := []TemplateStage{plan}
			for i := range maxTemplateStages {
				out = append(out, TemplateStage{ID: StageID("s" + string(rune('a'+i))), Kind: StageBuild})
			}
			return out
		}()}, "at most"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tpl.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidTemplate) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func research(t *testing.T) Pipeline {
	t.Helper()
	tpl, ok := FindTemplate("", "research")
	if !ok {
		t.Fatal("no research template")
	}
	return NewFromTemplate("s1", "why is the sky blue", tpl, t0)
}

func TestNewFromTemplate(t *testing.T) {
	p := research(t)
	if p.Template != "research" || p.Current != StagePlan || len(p.Stages) != 4 {
		t.Fatalf("pipeline = %+v", p)
	}
	want := []struct {
		id, kind StageID
		label    string
	}{{StagePlan, StagePlan, "Scope"}, {"research", StageBuild, "Research"}, {"report", StageBuild, "Report"}, {"check", StageReview, "Check"}}
	for i, w := range want {
		s := p.Stages[i]
		if s.ID != w.id || s.KindOf() != w.kind || s.Label != w.label || s.Instructions == "" {
			t.Fatalf("stage %d = %+v", i, s)
		}
	}
	if dev := New("s1", "g", t0); dev.Template != "" || dev.Stages[1].Kind != "" || dev.Stages[1].Label != "" {
		t.Fatalf("dev pipeline = %+v", dev)
	}
}

// A research pipeline runs on the same machine: two work stages with their
// own tasks, then a review-kind stage, and it finishes without a merge.
func TestTemplatePipelineRunsToTheEnd(t *testing.T) {
	p := research(t)
	plan := &Plan{Goal: "g", Tasks: []PlanTask{
		{Title: "find sources", Done: "notes.md", Stage: "research"},
		{Title: "write", Done: "report.md", Stage: "report"},
		{Title: "stray", Done: "x", Stage: "nowhere"},
	}, Stages: []StageID{"check", "report", "research", "bogus"}}
	p, _, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
	if err != nil || p.OpenGate != GatePlan {
		t.Fatalf("plan turn: %v gate=%q", err, p.OpenGate)
	}
	if got := p.Plan.Stages; len(got) != 4 || got[1] != "research" || got[3] != "check" {
		t.Fatalf("stages = %v", got)
	}
	if p.Plan.Tasks[2].Stage != "" {
		t.Fatalf("a task of an unknown stage keeps it: %+v", p.Plan.Tasks[2])
	}
	p, act, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	if err != nil || act.Prompt != "Plan approved. Start the research stage with task 1." || p.Current != "research" {
		t.Fatalf("approve: %v %q current=%s", err, act.Prompt, p.Current)
	}
	if s, _ := p.Stage("research"); s.Limit != DefaultLimits[StageBuild] {
		t.Fatalf("research limit = %d", s.Limit)
	}
	g := Guidance(p)
	if !strings.Contains(g, "Approved tasks of this stage") || !strings.Contains(g, "find sources") || !strings.Contains(g, "stray") || strings.Contains(g, "1. write") {
		t.Fatalf("research guidance lists the wrong tasks:\n%s", g)
	}
	if !strings.Contains(g, "write what you find to a notes file") || !strings.Contains(g, `"tasks_done"`) {
		t.Fatalf("research guidance:\n%s", g)
	}

	work := func(want StageID, next string) {
		t.Helper()
		var act Action
		p, act, err = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
		if err != nil || act.Kind != ActionRunVerification {
			t.Fatalf("%s turn: %v %+v", want, err, act)
		}
		p, act, err = Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: &Verification{Passed: true}}, t0)
		if err != nil || !strings.Contains(act.Prompt, next) {
			t.Fatalf("%s verification: %v %q", want, err, act.Prompt)
		}
	}
	work("research", "Start the report stage")
	work("report", "Start the check stage")
	if p.Current != "check" || p.CurrentKind() != StageReview {
		t.Fatalf("current = %s (%s)", p.Current, p.CurrentKind())
	}
	g = Guidance(p)
	if !strings.Contains(g, "Check the report against the notes") || !strings.Contains(g, "<focus-findings>") || !strings.Contains(g, "Do not edit files in this turn") {
		t.Fatalf("check guidance:\n%s", g)
	}

	// A finding, accepted: the fix turn gets the template's fix instructions.
	p, _, err = Apply(p, Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{Findings: []Finding{{ID: "f1", Severity: "high", File: "report.md", Line: 3, Title: "no source"}}}}, t0)
	if err != nil || p.OpenGate != GateTriage {
		t.Fatalf("review turn: %v gate=%q", err, p.OpenGate)
	}
	p, act, err = SetCardState(p, p.Review.Triage[0], CardDecided, DecisionFix, t0)
	if err != nil || act.Kind != ActionSendTurn {
		t.Fatalf("triage: %v %+v", err, act)
	}
	if g = Guidance(p); !strings.Contains(g, "Correct only what the listed findings name") {
		t.Fatalf("fix guidance:\n%s", g)
	}
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 5, Blocks: Blocks{Report: &Report{Summary: "fixed"}}}, t0)
	p, act, _ = Apply(p, Event{Kind: EventVerification, Turn: 5, Verification: &Verification{Passed: true}}, t0)
	if !strings.Contains(act.Prompt, "round 2 of 2") {
		t.Fatalf("after the fix: %q", act.Prompt)
	}
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 6, Blocks: Blocks{Findings: []Finding{}}}, t0)
	p, act, err = Apply(p, Event{Kind: EventVerification, Turn: 6, Verification: &Verification{Passed: true}}, t0)
	if err != nil || !strings.Contains(act.Prompt, "The pipeline is complete") {
		t.Fatalf("last verification: %v %q", err, act.Prompt)
	}
	if !Finished(p) || p.FinishedAt == nil || Releasable(p) || p.PendingTurn != "" {
		t.Fatalf("finished=%v releasable=%v pending=%q", Finished(p), Releasable(p), p.PendingTurn)
	}
}

func TestTemplatePlanGuidance(t *testing.T) {
	g := Guidance(research(t))
	for _, want := range []string{
		`"stages":["plan","research","report","check"]`,
		`{"title":"…","done":"…","stage":"research"}`,
		`"limits":{"research":3,"report":3,"check":2}`,
		"Do not start the research yet",
	} {
		if !strings.Contains(g, want) {
			t.Fatalf("plan guidance lacks %q:\n%s", want, g)
		}
	}
	if strings.Contains(g, "make test") {
		t.Fatalf("a non-development plan format suggests make test:\n%s", g)
	}
	writing, _ := FindTemplate("", "writing")
	g = Guidance(NewFromTemplate("s", "a novel", writing, t0))
	if !strings.Contains(g, `"tasks":[{"title":"…","done":"…"}]`) || !strings.Contains(g, `"limits":{"draft":3,"revise":2}`) {
		t.Fatalf("writing plan guidance:\n%s", g)
	}
	// The development template keeps its format word for word.
	if g = Guidance(New("s", "g", t0)); !strings.Contains(g, planFormat) {
		t.Fatalf("dev plan guidance:\n%s", g)
	}
}

func TestLoadTemplates(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("blog.yaml", `
name: Blog post
description: Outline, write, edit.
stages:
  - id: plan
    label: Outline
  - id: write
    kind: build
    label: Write
    instructions: Write the post.
  - id: edit
    kind: review
`)
	write("essay.json", `{"id":"essay","name":"Essay","stages":[{"id":"plan"},{"id":"draft","kind":"build"}]}`)
	write("broken.yml", "name: [unclosed")
	write("noname.yaml", "stages:\n  - id: plan\n  - id: build\n")
	write("clash.yaml", "id: research\nname: Mine\nstages:\n  - id: plan\n  - id: build\n")
	write("notes.txt", "not a template")
	if err := os.Mkdir(filepath.Join(dir, "folder.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}

	templates, diags := LoadTemplates(dir)
	ids := map[string]Template{}
	for _, tpl := range templates {
		ids[tpl.ID] = tpl
	}
	if len(templates) != len(BuiltinTemplates())+2 {
		t.Fatalf("templates = %v", ids)
	}
	blog := ids["blog"]
	if blog.Name != "Blog post" || blog.Builtin || blog.Source != "blog.yaml" || len(blog.Stages) != 3 || blog.Stages[1].Instructions != "Write the post." {
		t.Fatalf("blog = %+v", blog)
	}
	if _, ok := ids["essay"]; !ok || !ids["research"].Builtin {
		t.Fatalf("templates = %v", ids)
	}
	if len(diags) != 3 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	for _, d := range diags {
		if d.Source == "" || d.Error == "" {
			t.Fatalf("diagnostic = %+v", d)
		}
	}

	if tpl, ok := FindTemplate(dir, " blog "); !ok || tpl.ID != "blog" {
		t.Fatalf("FindTemplate blog = %+v %v", tpl, ok)
	}
	if tpl, ok := FindTemplate(dir, ""); !ok || tpl.ID != DevTemplateID {
		t.Fatalf("FindTemplate default = %+v %v", tpl, ok)
	}
	if _, ok := FindTemplate(dir, "nope"); ok {
		t.Fatal("found an unknown template")
	}
	if templates, diags := LoadTemplates(filepath.Join(dir, "missing")); len(templates) != len(BuiltinTemplates()) || diags != nil {
		t.Fatalf("missing folder: %d %+v", len(templates), diags)
	}
	if templates, diags := LoadTemplates(filepath.Join(dir, "notes.txt")); len(templates) != len(BuiltinTemplates()) || len(diags) != 1 {
		t.Fatalf("a file as the folder: %d %+v", len(templates), diags)
	}
	big := filepath.Join(dir, "big.yaml")
	if err := os.WriteFile(big, []byte("name: "+strings.Repeat("a", maxTemplateFileBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTemplateFile(big); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("oversized file: %v", err)
	}
}

func novelTemplate(id string) Template {
	return Template{ID: id, Name: "Novel", Description: "d", Stages: []TemplateStage{
		{ID: StagePlan},
		{ID: "draft", Kind: StageBuild, Instructions: "write"},
	}}
}

func TestSaveTemplateRoundTripsThroughLoadTemplates(t *testing.T) {
	dir := t.TempDir()
	saved, err := SaveTemplate(dir, "", novelTemplate("novel"))
	if err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if saved.Builtin || saved.Source != "novel.yaml" {
		t.Fatalf("saved = %+v", saved)
	}
	path := filepath.Join(dir, "novel.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("a leftover .tmp file: %v", entries)
	}
	tpl, ok := FindTemplate(dir, "novel")
	if !ok || tpl.Name != "Novel" || len(tpl.Stages) != 2 || tpl.Stages[1].Instructions != "write" {
		t.Fatalf("round trip = %+v %v", tpl, ok)
	}
}

func TestSaveTemplateRejectsBuiltinID(t *testing.T) {
	dir := t.TempDir()
	if _, err := SaveTemplate(dir, "", novelTemplate("research")); !errors.Is(err, ErrInvalidTemplate) || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("err = %v", err)
	}
}

func TestSaveTemplateRejectsTakenID(t *testing.T) {
	dir := t.TempDir()
	if _, err := SaveTemplate(dir, "", novelTemplate("novel")); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTemplate(dir, "", novelTemplate("novel")); !errors.Is(err, ErrInvalidTemplate) || !strings.Contains(err.Error(), "already taken") {
		t.Fatalf("second save with a different originalID: %v", err)
	}
	// Editing the same template back (originalID == its own id) is fine.
	if _, err := SaveTemplate(dir, "novel", novelTemplate("novel")); err != nil {
		t.Fatalf("re-saving under its own originalID: %v", err)
	}
}

func TestSaveTemplateRenameRemovesThePreviousFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := SaveTemplate(dir, "", novelTemplate("novel")); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTemplate(dir, "novel", novelTemplate("novella")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "novel.yaml")); !os.IsNotExist(err) {
		t.Fatalf("old file still there: %v", err)
	}
	if _, ok := FindTemplate(dir, "novel"); ok {
		t.Fatal("renamed template still found under its old id")
	}
	if tpl, ok := FindTemplate(dir, "novella"); !ok || tpl.Name != "Novel" {
		t.Fatalf("novella = %+v %v", tpl, ok)
	}
}

func TestSaveTemplateRejectsInvalidID(t *testing.T) {
	dir := t.TempDir()
	// The id pattern (no "/", no "..") is what keeps a saved template's
	// path inside dir; Validate rejects these before any file is touched.
	for _, id := range []string{"../escape", "a/b", "Has Spaces"} {
		if _, err := SaveTemplate(dir, "", novelTemplate(id)); !errors.Is(err, ErrInvalidTemplate) {
			t.Fatalf("id %q: err = %v", id, err)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a file escaped validation: %v", entries)
	}
}

func TestSaveTemplateRejectsOversizedTemplate(t *testing.T) {
	dir := t.TempDir()
	tpl := novelTemplate("novel")
	tpl.Stages[1].Instructions = strings.Repeat("a", maxTemplateInstruction)
	for i := 0; i < 20; i++ {
		tpl.Stages = append(tpl.Stages, TemplateStage{ID: StageID(fmt.Sprintf("s%d", i)), Kind: StageBuild, Instructions: strings.Repeat("b", maxTemplateInstruction)})
	}
	if _, err := SaveTemplate(dir, "", tpl); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteTemplate(t *testing.T) {
	dir := t.TempDir()
	if _, err := SaveTemplate(dir, "", novelTemplate("novel")); err != nil {
		t.Fatal(err)
	}
	if err := DeleteTemplate(dir, "novel"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	if _, ok := FindTemplate(dir, "novel"); ok {
		t.Fatal("deleted template still found")
	}
	if err := DeleteTemplate(dir, "novel"); !errors.Is(err, ErrInvalidTemplate) || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("deleting again: %v", err)
	}
	if err := DeleteTemplate(dir, "research"); !errors.Is(err, ErrInvalidTemplate) || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("deleting a built-in: %v", err)
	}
}

func TestRenderTemplateYAML(t *testing.T) {
	raw, err := RenderTemplateYAML(novelTemplate("novel"))
	if err != nil {
		t.Fatalf("RenderTemplateYAML: %v", err)
	}
	var back Template
	if err := yaml.Unmarshal(raw, &back); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if back.ID != "novel" || back.Name != "Novel" || len(back.Stages) != 2 {
		t.Fatalf("back = %+v", back)
	}
	if strings.Contains(string(raw), "builtin") || strings.Contains(string(raw), "source") {
		t.Fatalf("rendered yaml leaks internal fields:\n%s", raw)
	}
}

// writeRaw hand-authors a template file whose name does not match the id
// declared inside it — LoadTemplates has always allowed this (the id
// comes from the file's own "id" field, falling back to the file name
// only when that field is empty).
func writeRaw(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestSaveTemplateOfMismatchedFilenameRemovesTheOldFile covers a save
// (same id, no rename) of a template whose file name never matched its
// id: SaveTemplate must remove that old file once the new "<id>.yaml" is
// written, or the old one lingers forever as a same-id diagnostic.
func TestSaveTemplateOfMismatchedFilenameRemovesTheOldFile(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, dir, "my-blog.yaml", "id: blog\nname: Blog\nstages:\n  - id: plan\n  - id: draft\n    kind: build\n")

	tpl, ok := FindTemplate(dir, "blog")
	if !ok {
		t.Fatal("blog not found before edit")
	}
	tpl.Name = "Blog v2"
	if _, err := SaveTemplate(dir, tpl.ID, tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "my-blog.yaml")); !os.IsNotExist(err) {
		t.Fatalf("the mismatched-name file was not removed: %v", err)
	}
	templates, diags := LoadTemplates(dir)
	if len(diags) != 0 {
		t.Fatalf("a stale file produced a diagnostic: %+v", diags)
	}
	got, ok := FindTemplate(dir, "blog")
	if !ok || got.Name != "Blog v2" || got.Source != "blog.yaml" {
		t.Fatalf("blog = %+v %v (templates=%v)", got, ok, templates)
	}
}

// TestDeleteTemplateOfMismatchedFilenameRemovesTheRealFile covers deleting
// a never-edited, hand-authored template directly: the file removed must
// be the one it actually loads from, not a guessed "<id>.yaml".
func TestDeleteTemplateOfMismatchedFilenameRemovesTheRealFile(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, dir, "my-blog.yaml", "id: blog\nname: Blog\nstages:\n  - id: plan\n  - id: draft\n    kind: build\n")

	if err := DeleteTemplate(dir, "blog"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "my-blog.yaml")); !os.IsNotExist(err) {
		t.Fatalf("the real file was not removed: %v", err)
	}
	if _, ok := FindTemplate(dir, "blog"); ok {
		t.Fatal("deleted template still found")
	}
}

// TestDeleteTemplateAfterEditingAMismatchedFilenameDoesNotResurrectIt is
// the full regression for the finding: editing, then deleting, a
// mismatched-filename template must leave nothing behind that can load
// again as "blog" with the stale pre-edit content.
func TestDeleteTemplateAfterEditingAMismatchedFilenameDoesNotResurrectIt(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, dir, "my-blog.yaml", "id: blog\nname: Blog Original\nstages:\n  - id: plan\n  - id: draft\n    kind: build\n")

	tpl, _ := FindTemplate(dir, "blog")
	tpl.Name = "Blog Edited"
	if _, err := SaveTemplate(dir, tpl.ID, tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if err := DeleteTemplate(dir, "blog"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}

	templates, diags := LoadTemplates(dir)
	for _, got := range templates {
		if got.ID == "blog" {
			t.Fatalf("deleted template resurrected: %+v", got)
		}
	}
	if len(diags) != 0 {
		t.Fatalf("a stale file left a diagnostic: %+v", diags)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("files left behind: %v", entries)
	}
}

// TestRemoveTemplateFilesRejectsAnUnsafeID documents the guard CodeQL's
// path-injection query asked for: removeTemplateFiles never builds a
// filesystem path from an id that is not already known-safe, even though
// every public caller already goes through Template.Validate first.
func TestRemoveTemplateFilesRejectsAnUnsafeID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "..yaml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../escape", "a/b", "..", ""} {
		if removeTemplateFiles(dir, id) {
			t.Fatalf("id %q: removeTemplateFiles touched the filesystem", id)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "..yaml")); err != nil {
		t.Fatalf("an unrelated file was removed: %v", err)
	}
}
