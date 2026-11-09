package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/apptool"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/rs/zerolog"
)

// TestFocusPlanEditGoalModeLocksAgentChecks covers #1290: goal mode on/off
// × the three ways in × each field. In goal mode the agent's tool and block
// may change the goal but not a check; the developer's endpoint may change
// anything.
func TestFocusPlanEditGoalModeLocksAgentChecks(t *testing.T) {
	fields := []struct {
		name  string
		json  string
		value func(focuspipeline.Plan) string
		want  string
	}{
		{"goal", `{"goal":"a narrower goal"}`, func(p focuspipeline.Plan) string { return p.Goal }, "a narrower goal"},
		{"verify", `{"verify":["true"]}`, func(p focuspipeline.Plan) string { return strings.Join(p.Verify, ",") }, "true"},
		{"e2e", `{"e2e":[]}`, func(p focuspipeline.Plan) string { return strings.Join(p.E2E, ",") }, ""},
		{"e2e_setup", `{"e2e_setup":["make build"]}`, func(p focuspipeline.Plan) string { return strings.Join(p.E2ESetup, ",") }, "make build"},
		{"e2e_teardown", `{"e2e_teardown":["pkill app"]}`, func(p focuspipeline.Plan) string { return strings.Join(p.E2ETeardown, ",") }, "pkill app"},
	}
	for _, goalMode := range []bool{false, true} {
		for _, entry := range []string{"api", "tool", "block"} {
			for _, f := range fields {
				name := entry + "/" + f.name
				if goalMode {
					name = "goal_mode/" + name
				}
				t.Run(name, func(t *testing.T) {
					d, _, store, _ := testFocusDriver(t, func(int, string) string { return focusNoFindings }, &fakeVerifier{result: passAll})
					var audited []ops.AutomationAuditEntry
					d.audit = func(e ops.AutomationAuditEntry) { audited = append(audited, e) }
					sess := reviewingFocusSession(t, store, "", "")
					if goalMode {
						if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
							return focuspipeline.StartGoal(p, 0, time.Now()), nil
						}); err != nil {
							t.Fatal(err)
						}
					}
					before := *pipelineOf(t, store, sess.ID).Plan
					cardsBefore := len(pipelineOf(t, store, sess.ID).Cards)

					var toolResult apptool.Result
					switch entry {
					case "api":
						h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
						if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+sess.ID+"/plan", f.json, true); rec.Code != http.StatusOK {
							t.Fatalf("api: %d %s", rec.Code, rec.Body.String())
						}
					case "tool":
						tool := apptool.NewFocusPlanEditTool(newFocusPlanEditor(store, sess.ID, d, zerolog.Nop()))
						var err error
						if toolResult, err = tool.Execute(context.Background(), json.RawMessage(f.json)); err != nil {
							t.Fatal(err)
						}
					case "block":
						reply := "as asked\n<focus-plan-edit>" + f.json + "</focus-plan-edit>\n" + focusNoFindings
						if _, _, ok := focusAfterTurn(store, sess.ID, store.TranscriptPath(sess.ID), reply, currentFocusMark(t, store, sess.ID), time.Now(), d.audit, zerolog.Nop()); !ok {
							t.Fatal("turn not applied")
						}
					}

					p := pipelineOf(t, store, sess.ID)
					locked := goalMode && entry != "api" && f.name != "goal"
					if !locked {
						if got := f.value(*p.Plan); got != f.want {
							t.Fatalf("%s = %q, want %q", f.name, got, f.want)
						}
						if toolResult.IsError {
							t.Fatalf("tool refused: %+v", toolResult)
						}
						// The block path has never been audited; the others are.
						wantAudits := 1
						if entry == "block" {
							wantAudits = 0
						}
						if len(audited) != wantAudits || (wantAudits == 1 && audited[0].Result != "edited") {
							t.Fatalf("audit = %+v", audited)
						}
						return
					}

					if got, was := f.value(*p.Plan), f.value(before); got != was {
						t.Fatalf("goal mode let the agent change %s: %q → %q", f.name, was, got)
					}
					if len(audited) != 1 || audited[0].Action != "focus_plan_edit" || audited[0].Result != focusPlanEditResultGoalMode ||
						audited[0].SessionID != sess.ID {
						t.Fatalf("audit = %+v", audited)
					}
					if fs, _ := audited[0].Details["fields"].([]string); strings.Join(fs, ",") != f.name {
						t.Fatalf("audited fields = %v", audited[0].Details["fields"])
					}
					for _, c := range p.Cards {
						if c.Title == focuspipeline.NoticePlanEdited {
							t.Fatalf("a refused edit left a plan edited card: %+v", c)
						}
					}
					switch entry {
					case "tool":
						if !toolResult.IsError || !strings.Contains(toolResult.Text(), "goal mode") {
							t.Fatalf("tool result = %+v", toolResult)
						}
						if len(p.Cards) != cardsBefore {
							t.Fatalf("the tool's refusal is its result, not a card: %+v", p.Cards[cardsBefore:])
						}
					case "block":
						refused := false
						for _, c := range p.Cards[cardsBefore:] {
							refused = refused || (c.Kind == focuspipeline.CardNotice && c.Title == focuspipeline.NoticeFormatMissing &&
								strings.Contains(string(c.Payload), focuspipeline.TagPlanEdit) && strings.Contains(string(c.Payload), "goal mode"))
						}
						if !refused {
							t.Fatalf("no notice for the refused block: %+v", p.Cards[cardsBefore:])
						}
					}
				})
			}
		}
	}
}
