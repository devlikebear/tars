package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// Natural-language focus template editing (ADR §4.1 follow-up):
//
//	POST   /v1/focus/templates/draft   {request, base_id?, draft?} → {action, template?, original_id?, summary, warnings?, copied_from_builtin?}
//	PUT    /v1/focus/templates/{id}    {template, original_id?}     → the saved template
//	DELETE /v1/focus/templates/{id}                                 → {deleted: true}
//
// The draft route only asks an LLM for a proposal and validates it; it
// never writes a file. The user reviews the draft (the console shows a
// preview), then PUT or DELETE actually changes
// <workspace>/focus-templates/. Editing a built-in template always drafts
// a copy under a new id — a built-in template is never replaced or
// deleted. All three routes need the admin token, like starting a
// pipeline in a folder (focus_pipeline.go's create).
type focusTemplateEditAPI struct {
	sessions *session.Store
	router   llm.Router
	logger   zerolog.Logger
}

func newFocusTemplateEditHandler(sessions *session.Store, router llm.Router, logger zerolog.Logger) http.Handler {
	api := &focusTemplateEditAPI{sessions: sessions, router: router, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/focus/templates/draft", api.draft)
	mux.HandleFunc("PUT /v1/focus/templates/{id}", api.save)
	mux.HandleFunc("DELETE /v1/focus/templates/{id}", api.delete)
	return mux
}

func (a *focusTemplateEditAPI) dir() string { return focusTemplateDir(a.sessions) }

type focusTemplateDraftRequest struct {
	// Request is the natural-language instruction: what to create, change
	// or remove.
	Request string `json:"request"`
	// BaseID is the template being edited or deleted; empty asks for a
	// new template.
	BaseID string `json:"base_id,omitempty"`
	// Draft, when set, is a previous draft response's template: a
	// follow-up refinement ("make the review stage stricter") adjusts it
	// instead of BaseID's saved template.
	Draft *focuspipeline.Template `json:"draft,omitempty"`
}

type focusTemplateDraftResponse struct {
	// Action is "save" (Template holds the proposed template) or
	// "delete" (OriginalID names what would be removed).
	Action            string                  `json:"action"`
	Template          *focuspipeline.Template `json:"template,omitempty"`
	OriginalID        string                  `json:"original_id,omitempty"`
	Summary           string                  `json:"summary,omitempty"`
	Warnings          []string                `json:"warnings,omitempty"`
	CopiedFromBuiltin bool                    `json:"copied_from_builtin,omitempty"`
}

const focusTemplateDraftSystemPrompt = `You edit TARS focus-mode pipeline templates from a natural-language request.
Return only a json object, no prose and no code fence: {"action":"save"|"delete","id":"...","name":"...","description":"...","stages":[{"id":"...","kind":"plan|build|review|pr|pr_review|merge","label":"...","instructions":"...","fix_instructions":"..."}],"summary":"one sentence of what changed"}.
Rules: the first stage must have id "plan" and kind "plan". A stage's kind is one of plan, build, review, pr, pr_review, merge; omit kind only when the id already is one of those words. The kinds plan, pr, pr_review and merge may each appear at most once and must keep their own id as their id; build and review may repeat under any other lower-case id. Ids are lower-case ASCII letters, digits, "_" or "-", at most 32 characters, and unique within the template. At most 12 stages total. instructions and fix_instructions must never contain the text "<focus-" or "</focus-". Never call a tool, never claim you ran one, and never describe taking an action outside this json object.
Action "delete" removes an existing template: set id to the one being deleted and omit name, description and stages.
Action "save" creates a new template or replaces the "base" given to you; when "base" is a built-in template (its "builtin" field is true), you may not reuse its id — choose a new one, since built-in templates are never changed in place. When a "previous_draft" is given, refine that draft instead of starting over.`

func (a *focusTemplateEditAPI) draft(w http.ResponseWriter, r *http.Request) {
	if serverauth.RoleFromRequest(r) != serverauth.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "editing focus templates needs the admin token"})
		return
	}
	var req focusTemplateDraftRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	request := strings.TrimSpace(req.Request)
	if request == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request is required"})
		return
	}
	if a.router == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no LLM configured for template drafting"})
		return
	}
	dir := a.dir()
	templates, _ := focuspipeline.LoadTemplates(dir)
	var base *focuspipeline.Template
	baseID := strings.TrimSpace(req.BaseID)
	if baseID != "" {
		for i := range templates {
			if templates[i].ID == baseID {
				base = &templates[i]
				break
			}
		}
		if base == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
			return
		}
	}
	resp, err := draftFocusTemplateWithLLM(r.Context(), a.router, req, request, dir, templates, base)
	if err != nil {
		if errors.Is(err, errFocusTemplateDraftInvalid) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type focusTemplateSaveRequest struct {
	Template focuspipeline.Template `json:"template"`
	// OriginalID is the template being replaced (a draftFocusTemplateWithLLM
	// response's original_id); empty saves a new template.
	OriginalID string `json:"original_id,omitempty"`
}

func (a *focusTemplateEditAPI) save(w http.ResponseWriter, r *http.Request) {
	if serverauth.RoleFromRequest(r) != serverauth.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "editing focus templates needs the admin token"})
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var req focusTemplateSaveRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	tpl := req.Template
	if strings.TrimSpace(tpl.ID) == "" {
		tpl.ID = id
	} else if strings.TrimSpace(tpl.ID) != id {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the path id and the template's id must match"})
		return
	}
	saved, err := focuspipeline.SaveTemplate(a.dir(), req.OriginalID, tpl)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (a *focusTemplateEditAPI) delete(w http.ResponseWriter, r *http.Request) {
	if serverauth.RoleFromRequest(r) != serverauth.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "editing focus templates needs the admin token"})
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := focuspipeline.DeleteTemplate(a.dir(), id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// errFocusTemplateDraftInvalid marks a draft that is still not a valid,
// savable (or deletable) template after one retry — the caller's request
// was understood but the result fails Template.Validate or the id rules,
// so a 400 (not a 502) is the right answer.
var errFocusTemplateDraftInvalid = errors.New("focus template draft: invalid")

// focusTemplateLLMDraft is the json object the model is asked to return.
type focusTemplateLLMDraft struct {
	Action      string                        `json:"action"`
	ID          string                        `json:"id"`
	Name        string                        `json:"name"`
	Description string                        `json:"description"`
	Stages      []focuspipeline.TemplateStage `json:"stages"`
	Summary     string                        `json:"summary"`
}

// draftFocusTemplateWithLLM asks the model for one draft, retrying once
// with the previous failure's message when the response cannot be decoded
// or does not pass normalizeFocusTemplateDraft. The call never allows the
// model to use a tool: ToolChoiceNone, an empty Claude Code harness tool
// list, and the read-only "plan" permission mode, mirroring
// internal/computeruse's decision-only calls.
func draftFocusTemplateWithLLM(
	ctx context.Context,
	router llm.Router,
	req focusTemplateDraftRequest,
	request, dir string,
	templates []focuspipeline.Template,
	base *focuspipeline.Template,
) (focusTemplateDraftResponse, error) {
	client, _, err := router.ClientFor(llm.RoleChatMain)
	if err != nil {
		return focusTemplateDraftResponse{}, fmt.Errorf("focus template draft: %w", err)
	}
	ids := make([]string, 0, len(templates))
	for _, t := range templates {
		ids = append(ids, t.ID)
	}
	basePayload := map[string]any{"request": request, "existing_ids": ids}
	if base != nil {
		basePayload["base"] = base
	}
	if req.Draft != nil {
		basePayload["previous_draft"] = req.Draft
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		payload := basePayload
		if attempt > 0 && lastErr != nil {
			retry := map[string]any{}
			for k, v := range basePayload {
				retry[k] = v
			}
			retry["previous_attempt_error"] = lastErr.Error()
			payload = retry
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return focusTemplateDraftResponse{}, fmt.Errorf("focus template draft: encode request: %w", err)
		}
		messages := []llm.ChatMessage{
			{Role: "system", Content: focusTemplateDraftSystemPrompt},
			{Role: "user", Content: string(raw)},
		}
		callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		resp, err := client.Chat(callCtx, messages, llm.ChatOptions{
			ToolChoice:               llm.ToolChoiceNone(),
			ResponseFormat:           &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject},
			ClaudeCodePermissionMode: "plan",
			ClaudeCodeHarness:        &llm.ClaudeCodeHarnessOptions{Tools: []string{}, SafeMode: true, StrictMCP: true, DisableChrome: true, MaxTurns: 1},
		})
		cancel()
		if err != nil {
			return focusTemplateDraftResponse{}, fmt.Errorf("focus template draft: %w", err)
		}
		if len(resp.Message.ToolCalls) > 0 || len(resp.ProviderExecutedTools) > 0 {
			return focusTemplateDraftResponse{}, fmt.Errorf("focus template draft: the model attempted to use a tool")
		}
		var llmDraft focusTemplateLLMDraft
		if err := json.Unmarshal([]byte(extractJSONObjectText(resp.Message.Content)), &llmDraft); err != nil {
			lastErr = fmt.Errorf("decode draft: %w", err)
			continue
		}
		result, verr := normalizeFocusTemplateDraft(dir, templates, base, llmDraft)
		if verr != nil {
			lastErr = verr
			continue
		}
		return result, nil
	}
	return focusTemplateDraftResponse{}, fmt.Errorf("%w: %v", errFocusTemplateDraftInvalid, lastErr)
}

// normalizeFocusTemplateDraft turns the model's json object into a
// focusTemplateDraftResponse, validating a "save" action with
// focuspipeline.PrepareTemplateSave (no file is written) and a "delete"
// action against the loaded templates. Editing a built-in template always
// assigns a new id — base.Builtin is never the originalID a save reports.
func normalizeFocusTemplateDraft(dir string, templates []focuspipeline.Template, base *focuspipeline.Template, d focusTemplateLLMDraft) (focusTemplateDraftResponse, error) {
	action := strings.ToLower(strings.TrimSpace(d.Action))
	id := strings.TrimSpace(d.ID)
	if id == "" && base != nil {
		id = base.ID
	}
	switch action {
	case "delete":
		if id == "" {
			return focusTemplateDraftResponse{}, fmt.Errorf("delete needs an id")
		}
		found, builtin := false, false
		for _, t := range templates {
			if t.ID == id {
				found, builtin = true, t.Builtin
			}
		}
		if !found {
			return focusTemplateDraftResponse{}, fmt.Errorf("id %q not found", id)
		}
		if builtin {
			return focusTemplateDraftResponse{}, fmt.Errorf("id %q is a built-in template and cannot be deleted", id)
		}
		return focusTemplateDraftResponse{Action: "delete", OriginalID: id, Summary: strings.TrimSpace(d.Summary)}, nil
	case "save":
		tpl := focuspipeline.Template{ID: id, Name: d.Name, Description: d.Description, Stages: d.Stages}
		originalID, copied := "", false
		if base != nil {
			originalID = base.ID
			if base.Builtin {
				originalID, copied = "", true
				if tpl.ID == "" || tpl.ID == base.ID {
					tpl.ID = uniqueFocusTemplateID(templates, base.ID)
				}
			}
		}
		saved, err := focuspipeline.PrepareTemplateSave(dir, originalID, tpl)
		if err != nil {
			return focusTemplateDraftResponse{}, err
		}
		var warnings []string
		if copied {
			warnings = append(warnings, fmt.Sprintf("%q is a built-in template, so the edit was saved as a new template %q instead of replacing it.", base.ID, saved.ID))
		}
		return focusTemplateDraftResponse{
			Action: "save", Template: &saved, OriginalID: originalID,
			Summary: strings.TrimSpace(d.Summary), Warnings: warnings, CopiedFromBuiltin: copied,
		}, nil
	default:
		return focusTemplateDraftResponse{}, fmt.Errorf("action must be save or delete, got %q", d.Action)
	}
}

// uniqueFocusTemplateID returns an id derived from base that none of
// templates already use, for a draft that must copy rather than replace a
// built-in template.
func uniqueFocusTemplateID(templates []focuspipeline.Template, base string) string {
	taken := map[string]bool{}
	for _, t := range templates {
		taken[t.ID] = true
	}
	clamp := func(id string) string {
		if len(id) <= 32 {
			return id
		}
		return id[:32]
	}
	candidate := clamp(base + "-custom")
	for n := 2; taken[candidate]; n++ {
		candidate = clamp(fmt.Sprintf("%s-custom-%d", base, n))
	}
	return candidate
}

// extractJSONObjectText pulls a json object out of s, tolerating a
// ```json-fenced block or stray text around it the model added despite
// being asked not to.
func extractJSONObjectText(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			head := strings.TrimSpace(s[:nl])
			if head == "" || strings.EqualFold(head, "json") {
				s = s[nl+1:]
			}
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	if start := strings.IndexByte(s, '{'); start > 0 {
		s = s[start:]
	}
	if end := strings.LastIndexByte(s, '}'); end >= 0 && end < len(s)-1 {
		s = s[:end+1]
	}
	return strings.TrimSpace(s)
}
