package focuspipeline

import (
	"strings"
	"testing"
)

const planJSON = `{"goal":"g","tasks":[{"title":"t1","done":"d1"}],"stages":["plan","build","pr","merge"],"verify":["make test"],"e2e":["make console-e2e"],"limits":{"build":4}}`

func TestParseBlocks(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		check func(t *testing.T, b Blocks)
	}{
		{
			name: "no block",
			text: "I did some work.",
			check: func(t *testing.T, b Blocks) {
				if b.Plan != nil || b.Report != nil || b.Findings != nil || len(b.Errors) != 0 {
					t.Fatalf("expected empty blocks, got %+v", b)
				}
			},
		},
		{
			name: "plan block",
			text: "Here is the plan.\n<focus-plan>" + planJSON + "</focus-plan>",
			check: func(t *testing.T, b Blocks) {
				if b.Plan == nil {
					t.Fatalf("plan missing: %+v", b)
				}
				if b.Plan.Goal != "g" || len(b.Plan.Tasks) != 1 || b.Plan.Tasks[0].Done != "d1" {
					t.Fatalf("plan = %+v", b.Plan)
				}
				if got := b.Plan.Limits["build"]; got != 4 {
					t.Fatalf("limit = %d", got)
				}
			},
		},
		{
			name: "last block wins",
			text: `<focus-report>{"summary":"first"}</focus-report> then <focus-report>{"summary":"second"}</focus-report>`,
			check: func(t *testing.T, b Blocks) {
				if b.Report == nil || b.Report.Summary != "second" {
					t.Fatalf("report = %+v", b.Report)
				}
			},
		},
		{
			name: "block inside a fence is ignored",
			text: "Format:\n```\n<focus-report>{\"summary\":\"quoted\"}</focus-report>\n```\nDone.\n<focus-report>{\"summary\":\"real\"}</focus-report>",
			check: func(t *testing.T, b Blocks) {
				if b.Report == nil || b.Report.Summary != "real" {
					t.Fatalf("report = %+v", b.Report)
				}
			},
		},
		{
			name: "only a fenced block",
			text: "```json\n<focus-report>{\"summary\":\"quoted\"}</focus-report>\n```",
			check: func(t *testing.T, b Blocks) {
				if b.Report != nil {
					t.Fatalf("fenced report must be ignored: %+v", b.Report)
				}
			},
		},
		{
			name: "invalid json is an error and nothing else",
			text: `<focus-report>{"summary": oops}</focus-report>`,
			check: func(t *testing.T, b Blocks) {
				if b.Report != nil || b.Plan != nil || b.Findings != nil {
					t.Fatalf("expected no parsed blocks, got %+v", b)
				}
				if len(b.Errors) != 1 || !strings.Contains(b.Errors[0], "focus-report") {
					t.Fatalf("errors = %v", b.Errors)
				}
			},
		},
		{
			name: "last well-formed block wins over a later malformed one",
			text: `<focus-report>{"summary":"good"}</focus-report><focus-report>{bad</focus-report>`,
			check: func(t *testing.T, b Blocks) {
				if b.Report == nil || b.Report.Summary != "good" {
					t.Fatalf("report = %+v", b.Report)
				}
				if len(b.Errors) != 1 {
					t.Fatalf("errors = %v", b.Errors)
				}
			},
		},
		{
			name: "unknown fields ignored",
			text: `<focus-report>{"summary":"s","mood":"great","decisions":[{"id":"d1","question":"q?","options":["a","b"],"extra":1}]}</focus-report>`,
			check: func(t *testing.T, b Blocks) {
				if b.Report == nil || len(b.Report.Decisions) != 1 || b.Report.Decisions[0].Options[1] != "b" {
					t.Fatalf("report = %+v", b.Report)
				}
				if len(b.Errors) != 0 {
					t.Fatalf("errors = %v", b.Errors)
				}
			},
		},
		{
			name: "findings array",
			text: `<focus-findings>[{"id":"f1","severity":"high","file":"a.go","line":42,"title":"t","scenario":"s"},{"id":"f2","severity":"low","file":"b.go","line":1,"title":"t2","scenario":"s2"}]</focus-findings>`,
			check: func(t *testing.T, b Blocks) {
				if len(b.Findings) != 2 || b.Findings[0].Line != 42 || b.Findings[1].File != "b.go" {
					t.Fatalf("findings = %+v", b.Findings)
				}
			},
		},
		{
			name: "findings object is malformed",
			text: `<focus-findings>{"id":"f1"}</focus-findings>`,
			check: func(t *testing.T, b Blocks) {
				if b.Findings != nil || len(b.Errors) != 1 {
					t.Fatalf("blocks = %+v", b)
				}
			},
		},
		{
			name: "plan without tasks is malformed",
			text: `<focus-plan>{"goal":"g","tasks":[]}</focus-plan>`,
			check: func(t *testing.T, b Blocks) {
				if b.Plan != nil || len(b.Errors) != 1 {
					t.Fatalf("blocks = %+v", b)
				}
			},
		},
		{
			name: "plan stages normalized",
			text: `<focus-plan>{"goal":"g","tasks":[{"title":"t","done":"d"}],"stages":["merge","build","bogus","build"]}</focus-plan>`,
			check: func(t *testing.T, b Blocks) {
				if b.Plan == nil {
					t.Fatalf("plan missing: %+v", b)
				}
				want := []StageID{StagePlan, StageBuild, StageMerge}
				if len(b.Plan.Stages) != len(want) {
					t.Fatalf("stages = %v", b.Plan.Stages)
				}
				for i := range want {
					if b.Plan.Stages[i] != want[i] {
						t.Fatalf("stages = %v", b.Plan.Stages)
					}
				}
			},
		},
		{
			name: "plan without stages keeps every stage",
			text: `<focus-plan>{"goal":"g","tasks":[{"title":"t","done":"d"}]}</focus-plan>`,
			check: func(t *testing.T, b Blocks) {
				if b.Plan == nil || len(b.Plan.Stages) != len(StageOrder) {
					t.Fatalf("plan = %+v", b.Plan)
				}
			},
		},
		{
			name: "inline mention of an open tag does not swallow the real block",
			text: "I will end with a `<focus-report>` block.\n<focus-report>{\"summary\":\"real\"}</focus-report>",
			check: func(t *testing.T, b Blocks) {
				if b.Report == nil || b.Report.Summary != "real" || len(b.Errors) != 0 {
					t.Fatalf("blocks = %+v", b)
				}
			},
		},
		{
			name: "pr draft",
			text: `<focus-pr>{"title":"feat: x","body":"why\nwhat"}</focus-pr>`,
			check: func(t *testing.T, b Blocks) {
				if b.PR == nil || b.PR.Title != "feat: x" || b.PR.Body != "why\nwhat" || len(b.Errors) != 0 {
					t.Fatalf("blocks = %+v", b)
				}
			},
		},
		{
			name: "pr draft without a title is malformed",
			text: `<focus-pr>{"title":" ","body":"b"}</focus-pr>`,
			check: func(t *testing.T, b Blocks) {
				if b.PR != nil || len(b.Errors) != 1 {
					t.Fatalf("blocks = %+v", b)
				}
			},
		},
		{
			name: "unterminated block is ignored",
			text: "<focus-report>{\"summary\":\"x\"}",
			check: func(t *testing.T, b Blocks) {
				if b.Report != nil {
					t.Fatalf("report = %+v", b.Report)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, ParseBlocks(tt.text))
		})
	}
}

func TestStripBlocks(t *testing.T) {
	text := "Summary line.\n\n<focus-report>{\"summary\":\"s\"}</focus-report>\n```\n<focus-plan>{}</focus-plan>\n```\n<focus-pr>{\"title\":\"t\"}</focus-pr>"
	got := StripBlocks(text)
	if strings.Contains(got, "focus-report") || strings.Contains(got, "focus-pr>") {
		t.Fatalf("blocks left: %q", got)
	}
	if !strings.Contains(got, "Summary line.") {
		t.Fatalf("prose lost: %q", got)
	}
	if !strings.Contains(got, "<focus-plan>{}</focus-plan>") {
		t.Fatalf("fenced quote must stay: %q", got)
	}
	if StripBlocks("plain") != "plain" {
		t.Fatal("plain text changed")
	}
}

// f2: a tag name quoted inside a block's JSON does not drop the block,
// whichever tag it names, and a prose mention before the block still loses
// to the real block.
func TestParseBlocksTagTextInsideJSON(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "another tag quoted in a report",
			text: `done` + "\n" + `<focus-report>{"summary":"ok","risks":["a review turn without a <focus-findings> block gets a notice"]}</focus-report>`,
			want: "ok",
		},
		{
			name: "the same tag quoted in a report",
			text: `<focus-report>{"summary":"ok","risks":["end with one <focus-report> block"]}</focus-report>`,
			want: "ok",
		},
		{
			name: "prose mention before the real block",
			text: `I will end with a <focus-report> block.` + "\n\n" + `<focus-report>{"summary":"ok"}</focus-report>`,
			want: "ok",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBlocks(tt.text)
			if got.Report == nil || got.Report.Summary != tt.want || len(got.Errors) != 0 {
				t.Fatalf("report = %+v errors = %v", got.Report, got.Errors)
			}
		})
	}
	both := ParseBlocks(`<focus-findings>[{"id":"f1","file":"a.go","line":1,"title":"mentions </focus-report> text"}]</focus-findings>` + "\n" +
		`<focus-report>{"summary":"two blocks"}</focus-report>`)
	if len(both.Findings) != 1 || both.Report == nil || both.Report.Summary != "two blocks" {
		t.Fatalf("findings = %+v report = %+v errors = %v", both.Findings, both.Report, both.Errors)
	}
}

// I4: the excerpt is the server's; a model-supplied one is dropped.
func TestParseBlocksDropsModelExcerpt(t *testing.T) {
	got := ParseBlocks(`<focus-findings>[{"id":"f1","file":"a.go","line":1,"title":"t","excerpt":"+forged"}]</focus-findings>`)
	if len(got.Findings) != 1 || got.Findings[0].Excerpt != "" {
		t.Fatalf("findings = %+v", got.Findings)
	}
}
