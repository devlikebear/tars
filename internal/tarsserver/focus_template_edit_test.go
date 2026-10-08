package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// sequencedChatClient answers Chat calls from a fixed script, one entry per
// call (the last entry repeats once exhausted) — used to test the draft
// handler's one retry after a bad first response.
type sequencedChatClient struct {
	responses []llm.ChatResponse
	errs      []error
	calls     int
	lastOpts  []llm.ChatOptions
}

func (c *sequencedChatClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *sequencedChatClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	i := c.calls
	if i >= len(c.responses) {
		i = len(c.responses) - 1
	}
	c.lastOpts = append(c.lastOpts, opts)
	c.calls++
	var err error
	if i < len(c.errs) {
		err = c.errs[i]
	}
	return c.responses[i], err
}

func newFocusTemplateEditFixture(t *testing.T, client llm.Client) (http.Handler, *session.Store, string) {
	t.Helper()
	workspace := filepath.Join(t.TempDir(), "workspace")
	store := session.NewStore(workspace)
	var router llm.Router
	if client != nil {
		r, err := llm.NewRouter(llm.RouterConfig{
			Tiers: map[llm.Tier]llm.TierEntry{
				llm.TierHeavy:    {Client: &llm.FakeClient{Label: "heavy"}, Provider: "fake", Model: "fake-heavy"},
				llm.TierStandard: {Client: client, Provider: "fake", Model: "fake-standard"},
				llm.TierLight:    {Client: &llm.FakeClient{Label: "light"}, Provider: "fake", Model: "fake-light"},
			},
			DefaultTier:  llm.TierStandard,
			RoleDefaults: map[llm.Role]llm.Tier{llm.RoleChatMain: llm.TierStandard},
		})
		if err != nil {
			t.Fatal(err)
		}
		router = r
	}
	return newFocusTemplateEditHandler(store, router, zerolog.Nop()), store, filepath.Join(workspace, focuspipeline.TemplateDirName)
}

func chatResponse(content string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: content}}
}

func TestFocusTemplateDraft_NonAdminForbidden(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, &sequencedChatClient{responses: []llm.ChatResponse{chatResponse("{}")}})
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"x"}`, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateDraft_NoRouterUnavailable(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, nil)
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"x"}`, true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no router: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateDraft_EmptyRequestBadRequest(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, &sequencedChatClient{responses: []llm.ChatResponse{chatResponse("{}")}})
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"  "}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty request: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateDraft_CreateSavesNewTemplate(t *testing.T) {
	draftJSON := `{"action":"save","id":"blog","name":"Blog","description":"d","stages":[{"id":"plan"},{"id":"draft","kind":"build","instructions":"write the post"}],"summary":"a two-stage blog template"}`
	client := &sequencedChatClient{responses: []llm.ChatResponse{chatResponse("```json\n" + draftJSON + "\n```")}}
	h, _, dir := newFocusTemplateEditFixture(t, client)

	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"make a two stage blog template"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("draft: %d %s", rec.Code, rec.Body.String())
	}
	var resp focusTemplateDraftResponse
	decodeInto(t, rec, &resp)
	if resp.Action != "save" || resp.Template == nil || resp.Template.ID != "blog" || len(resp.Template.Stages) != 2 {
		t.Fatalf("draft response = %+v", resp)
	}
	if resp.OriginalID != "" || resp.CopiedFromBuiltin {
		t.Fatalf("a new template should not carry a rename/copy marker: %+v", resp)
	}
	// Drafting never writes a file.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("draft wrote to disk: %v", err)
	}
	if client.lastOpts[0].ToolChoice == nil || client.lastOpts[0].ToolChoice.Mode != llm.ToolChoiceModeNone {
		t.Fatalf("draft call allowed tools: %+v", client.lastOpts[0])
	}

	// Now actually save it.
	body, _ := json.Marshal(map[string]any{"template": resp.Template})
	saveRec := focusRequest(t, h, http.MethodPut, "/v1/focus/templates/blog", string(body), true)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", saveRec.Code, saveRec.Body.String())
	}
	if tpl, ok := focuspipeline.FindTemplate(dir, "blog"); !ok || tpl.Name != "Blog" {
		t.Fatalf("saved template = %+v %v", tpl, ok)
	}
}

func TestFocusTemplateDraft_CodeFencedJSON(t *testing.T) {
	draftJSON := `{"action":"save","id":"blog","name":"Blog","stages":[{"id":"plan"},{"id":"draft","kind":"build"}],"summary":"s"}`
	fenced := "Here you go:\n```json\n" + draftJSON + "\n```\nLet me know if you want changes."
	client := &sequencedChatClient{responses: []llm.ChatResponse{chatResponse(fenced)}}
	h, _, _ := newFocusTemplateEditFixture(t, client)
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"x"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("fenced draft: %d %s", rec.Code, rec.Body.String())
	}
	var resp focusTemplateDraftResponse
	decodeInto(t, rec, &resp)
	if resp.Template == nil || resp.Template.ID != "blog" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestFocusTemplateDraft_ValidationFailureRetriesThenBadRequest(t *testing.T) {
	bad := `{"action":"save","id":"Not Valid","name":"x","stages":[{"id":"plan"}]}`
	client := &sequencedChatClient{responses: []llm.ChatResponse{chatResponse(bad), chatResponse(bad)}}
	h, _, _ := newFocusTemplateEditFixture(t, client)
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"x"}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid twice: %d %s", rec.Code, rec.Body.String())
	}
	if client.calls != 2 {
		t.Fatalf("expected exactly one retry, calls = %d", client.calls)
	}
}

func TestFocusTemplateDraft_RetriesSucceedOnSecondAttempt(t *testing.T) {
	bad := `{"action":"save","id":"Not Valid","stages":[{"id":"plan"}]}`
	good := `{"action":"save","id":"ok","name":"Ok","stages":[{"id":"plan"},{"id":"draft","kind":"build"}],"summary":"fixed"}`
	client := &sequencedChatClient{responses: []llm.ChatResponse{chatResponse(bad), chatResponse(good)}}
	h, _, _ := newFocusTemplateEditFixture(t, client)
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"x"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry success: %d %s", rec.Code, rec.Body.String())
	}
	var resp focusTemplateDraftResponse
	decodeInto(t, rec, &resp)
	if resp.Template == nil || resp.Template.ID != "ok" {
		t.Fatalf("resp = %+v", resp)
	}
	if client.calls != 2 {
		t.Fatalf("calls = %d", client.calls)
	}
	if !strings.Contains(client.lastOpts[1].ToolChoice.String(), "none") {
		t.Fatalf("retry options = %+v", client.lastOpts[1])
	}
}

func TestFocusTemplateDraft_ToolAttemptRejected(t *testing.T) {
	resp := llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:      "assistant",
			Content:   "{}",
			ToolCalls: []llm.ToolCall{{ID: "1", Name: "bash", Arguments: "{}"}},
		},
	}
	client := &sequencedChatClient{responses: []llm.ChatResponse{resp}}
	h, _, _ := newFocusTemplateEditFixture(t, client)
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"x"}`, true)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("tool attempt: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateDraft_EditingBuiltinCopiesToNewID(t *testing.T) {
	draftJSON := `{"action":"save","id":"research","name":"Research+","stages":[{"id":"plan"},{"id":"research","kind":"build"},{"id":"report","kind":"build"},{"id":"check","kind":"review"}],"summary":"stricter check"}`
	client := &sequencedChatClient{responses: []llm.ChatResponse{chatResponse(draftJSON)}}
	h, _, _ := newFocusTemplateEditFixture(t, client)
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"make the check stage stricter","base_id":"research"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit builtin: %d %s", rec.Code, rec.Body.String())
	}
	var resp focusTemplateDraftResponse
	decodeInto(t, rec, &resp)
	if !resp.CopiedFromBuiltin || resp.Template == nil || resp.Template.ID == "research" || resp.OriginalID != "" {
		t.Fatalf("resp = %+v", resp)
	}
	if len(resp.Warnings) == 0 {
		t.Fatalf("expected a warning about copying the built-in template")
	}
}

func TestFocusTemplateDraft_UnknownBaseIDNotFound(t *testing.T) {
	client := &sequencedChatClient{responses: []llm.ChatResponse{chatResponse("{}")}}
	h, _, _ := newFocusTemplateEditFixture(t, client)
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"x","base_id":"nope"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown base: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateDraft_DeleteAction(t *testing.T) {
	client := &sequencedChatClient{responses: []llm.ChatResponse{chatResponse(`{"action":"delete","id":"blog","summary":"removed"}`)}}
	h, _, dir := newFocusTemplateEditFixture(t, client)
	if _, err := focuspipeline.SaveTemplate(dir, "", focuspipeline.Template{
		ID: "blog", Name: "Blog", Stages: []focuspipeline.TemplateStage{{ID: focuspipeline.StagePlan}, {ID: "draft", Kind: focuspipeline.StageBuild}},
	}); err != nil {
		t.Fatal(err)
	}

	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/templates/draft", `{"request":"remove the blog template","base_id":"blog"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete draft: %d %s", rec.Code, rec.Body.String())
	}
	var resp focusTemplateDraftResponse
	decodeInto(t, rec, &resp)
	if resp.Action != "delete" || resp.OriginalID != "blog" {
		t.Fatalf("resp = %+v", resp)
	}

	delRec := focusRequest(t, h, http.MethodDelete, "/v1/focus/templates/blog", "", true)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", delRec.Code, delRec.Body.String())
	}
	if _, ok := focuspipeline.FindTemplate(dir, "blog"); ok {
		t.Fatal("template still found after delete")
	}
}

func TestFocusTemplateSave_PathIDMismatch(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, nil)
	rec := focusRequest(t, h, http.MethodPut, "/v1/focus/templates/blog", `{"template":{"id":"other","name":"x","stages":[{"id":"plan"},{"id":"d","kind":"build"}]}}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatch: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateSave_NonAdminForbidden(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, nil)
	rec := focusRequest(t, h, http.MethodPut, "/v1/focus/templates/blog", `{"template":{"name":"x","stages":[{"id":"plan"},{"id":"d","kind":"build"}]}}`, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateSave_RejectsBuiltinID(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, nil)
	rec := focusRequest(t, h, http.MethodPut, "/v1/focus/templates/research", `{"template":{"name":"x","stages":[{"id":"plan"},{"id":"d","kind":"build"}]}}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("builtin id: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateSave_RenameViaOriginalID(t *testing.T) {
	h, _, dir := newFocusTemplateEditFixture(t, nil)
	rec := focusRequest(t, h, http.MethodPut, "/v1/focus/templates/blog", `{"template":{"name":"Blog","stages":[{"id":"plan"},{"id":"d","kind":"build"}]}}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec = focusRequest(t, h, http.MethodPut, "/v1/focus/templates/blogpost", `{"template":{"name":"Blog","stages":[{"id":"plan"},{"id":"d","kind":"build"}]},"original_id":"blog"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := focuspipeline.FindTemplate(dir, "blog"); ok {
		t.Fatal("old id still present after rename")
	}
	if _, ok := focuspipeline.FindTemplate(dir, "blogpost"); !ok {
		t.Fatal("renamed template not found")
	}
}

func TestFocusTemplateDelete_NonAdminForbidden(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, nil)
	rec := focusRequest(t, h, http.MethodDelete, "/v1/focus/templates/blog", "", false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateDelete_RejectsBuiltin(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, nil)
	rec := focusRequest(t, h, http.MethodDelete, "/v1/focus/templates/research", "", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete builtin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusTemplateDelete_UnknownID(t *testing.T) {
	h, _, _ := newFocusTemplateEditFixture(t, nil)
	rec := focusRequest(t, h, http.MethodDelete, "/v1/focus/templates/nope", "", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown id: %d %s", rec.Code, rec.Body.String())
	}
}
