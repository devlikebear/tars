package focuspipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebAppTemplateIsBuiltinAndValid(t *testing.T) {
	tpl, ok := FindTemplate("", WebAppTemplateID)
	if !ok {
		t.Fatal("no webapp template")
	}
	if err := tpl.Validate(); err != nil {
		t.Fatal(err)
	}
	if !tpl.Builtin || !tpl.WebUI {
		t.Fatalf("builtin %v, web ui %v", tpl.Builtin, tpl.WebUI)
	}
	p := NewFromTemplate("s1", "a sign-up page for a book club", tpl, t0)
	if p.Template != WebAppTemplateID || !p.WebUI || p.Current != StagePlan {
		t.Fatalf("pipeline: template %q, web ui %v, current %q", p.Template, p.WebUI, p.Current)
	}
	want := []struct{ id, kind StageID }{{StagePlan, StagePlan}, {"make", StageBuild}, {"review", StageReview}}
	if len(p.Stages) != len(want) {
		t.Fatalf("stages: %+v", p.Stages)
	}
	for i, w := range want {
		if p.Stages[i].ID != w.id || p.Stages[i].KindOf() != w.kind {
			t.Errorf("stage %d = %s/%s, want %s/%s", i, p.Stages[i].ID, p.Stages[i].KindOf(), w.id, w.kind)
		}
	}
	if p.hasStageKind(StagePR) || p.hasStageKind(StageMerge) {
		t.Fatal("the web app template has no pull request")
	}
}

// The plan is written for someone who cannot read code, on the fixed stack,
// with verify commands as the proof, and nothing is pushed.
func TestWebAppPlanGuidance(t *testing.T) {
	tpl, _ := FindTemplate("", WebAppTemplateID)
	p := NewFromTemplate("s1", "a sign-up page", tpl, t0)
	got := Guidance(p)
	for _, want := range []string{
		"cannot read code", "plain words", "Cloudflare Vite", "Hono", "D1", "Tailwind",
		"`npm run build`", "Do not commit, branch, push or open a pull request",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plan guidance lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, pushPRInWrongStageWarning) {
		t.Errorf("a template with no pr stage is warned about one:\n%s", got)
	}
	if strings.Contains(got, "Scripted browser checks") {
		t.Errorf("a desk without the CLI hears of it:\n%s", got)
	}
}

// A web app pipeline is taught scripted browser checks although it has no
// pull request stage; the writing template still is not.
func TestWebUITemplateGetsBrowserChecks(t *testing.T) {
	tpl, _ := FindTemplate("", WebAppTemplateID)
	p := NewFromTemplate("s1", "a sign-up page", tpl, t0)
	p.BrowserCheck = BrowserCheckCLIName
	if got := Guidance(p); !strings.Contains(got, "Scripted browser checks") || !strings.Contains(got, checkFailedMarker) {
		t.Fatalf("plan guidance:\n%s", got)
	}

	p.Stages[0].Status, p.Stages[1].Status, p.Stages[1].Iteration = StatusDone, StatusActive, 1
	p.Current = "make"
	got := Guidance(p)
	if !strings.Contains(got, "never by loosening the check") || !strings.Contains(got, "Make the approved tasks") {
		t.Fatalf("build guidance:\n%s", got)
	}
	if strings.Contains(got, "Scripted browser checks:") {
		t.Fatalf("the plan paragraph is repeated in build:\n%s", got)
	}
}

// A pipeline from any other built-in template stores nothing new.
func TestWebUIIsNotStoredForOtherTemplates(t *testing.T) {
	for _, tpl := range BuiltinTemplates() {
		if tpl.ID == WebAppTemplateID {
			continue
		}
		raw, err := json.Marshal(NewFromTemplate("s", "goal", tpl, t0))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "web_ui") {
			t.Fatalf("%s stores web_ui:\n%s", tpl.ID, raw)
		}
	}
}

// A user template can mark itself as web UI work.
func TestUserTemplateWebUI(t *testing.T) {
	dir := t.TempDir()
	yaml := "name: Landing page\nweb_ui: true\nstages:\n  - id: plan\n  - id: build\n  - id: review\n"
	if err := os.WriteFile(filepath.Join(dir, "landing.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	tpl, ok := FindTemplate(dir, "landing")
	if !ok || !tpl.WebUI {
		t.Fatalf("found %v, web ui %v", ok, tpl.WebUI)
	}
	p := NewFromTemplate("s", "a landing page", tpl, t0)
	p.BrowserCheck = BrowserCheckCLIName
	if !strings.Contains(Guidance(p), "Scripted browser checks") {
		t.Fatal("a web UI user template is not taught browser checks")
	}
}
