package tarsserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

type chatHandlerDeps struct {
	workspaceDir   string
	store          *session.Store
	client         llm.Client
	router         llm.Router
	logger         zerolog.Logger
	maxIters       int
	chatLimiter    *inflightLimiter
	activity       *runtimeActivity
	mainSessionID  string
	tooling        chatToolingOptions
	extraTools     []tool.Tool
	cancelRegistry *chatCancelRegistry
	// turnFeeds keeps each running turn's events for consoles that attach
	// later (GET /v1/chat/stream).
	turnFeeds    *chatTurnFeeds
	chatActivity *chatActivity
	permissions  *chatPermissionBroker
}

func (d chatHandlerDeps) resolveChatClient() (llm.Client, llm.TierResolution, error) {
	return d.resolveChatClientForTier("")
}

func (d chatHandlerDeps) resolveChatClientForTier(rawTier string) (llm.Client, llm.TierResolution, error) {
	if d.router != nil {
		if strings.TrimSpace(rawTier) != "" {
			tier, err := llm.ParseTier(rawTier)
			if err != nil {
				return nil, llm.TierResolution{}, err
			}
			return d.router.ClientForTier(tier)
		}
		return d.router.ClientFor(llm.RoleChatMain)
	}
	if d.client != nil {
		return d.client, llm.TierResolution{Role: llm.RoleChatMain, Source: "legacy"}, nil
	}
	return nil, llm.TierResolution{}, fmt.Errorf("llm router is not configured")
}

type chatAttachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64-encoded content
}

type chatRequestPayload struct {
	SessionID          string                         `json:"session_id"`
	Message            string                         `json:"message"`
	Attachments        []chatAttachment               `json:"attachments,omitempty"`
	Mentions           []chatFileMentionRequest       `json:"mentions,omitempty"`
	SubagentMentions   []chatSubagentMentionRequest   `json:"subagent_mentions,omitempty"`
	TierRecommendation *chatTierRecommendationPayload `json:"tier_recommendation,omitempty"`
	// ReviewNotes are comments on, and reverts of, earlier turns' changes;
	// they are appended to the message (see appendReviewNotes).
	ReviewNotes []chatReviewNote `json:"review_notes,omitempty"`
	// ConsoleContext is guidance the console adds to this turn without the
	// user typing it (the companion handoff); it is appended to the message
	// as a tagged block the console hides (see appendConsoleContext).
	ConsoleContext string `json:"console_context,omitempty"`
	// InteractivePermissions says the client will answer permission_request
	// events, so tool prompts wait for it instead of failing the call.
	InteractivePermissions bool `json:"interactive_permissions,omitempty"`
}

func handleChatRequest(w http.ResponseWriter, r *http.Request, deps chatHandlerDeps) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	if deps.chatLimiter != nil {
		release, ok := deps.chatLimiter.tryAcquire()
		if !ok {
			writeError(w, http.StatusTooManyRequests, "overloaded", "overloaded")
			return
		}
		// The limit counts consoles streaming a turn. A turn whose console
		// went away keeps running (#971) but gives its slot back, so turns
		// left waiting on an approval cannot lock everyone out of chat.
		var once sync.Once
		releaseOnce := func() { once.Do(release) }
		defer releaseOnce()
		go func() {
			<-r.Context().Done()
			releaseOnce()
		}()
	}

	req, ok := decodeChatRequestPayload(w, r)
	if !ok {
		return
	}
	// A person's turn wins over the focus driver: its pending step (a
	// verification, a queued turn) stops, and this turn's post-turn hook
	// starts a new run.
	deps.tooling.Focus.cancel(chatClaimKey(req.SessionID, deps.mainSessionID))
	_, _ = runChatTurn(w, r, req, deps, chatTurnOrigin{})
}

// chatClaimKey is the session a chat request will run in, as far as it is
// known before the turn is prepared: "main" and "" name the main session,
// "new" names none yet (a new session cannot be busy).
func chatClaimKey(requested, mainSessionID string) string {
	id := strings.TrimSpace(requested)
	switch {
	case strings.EqualFold(id, "new"):
		return ""
	case id == "" || strings.EqualFold(id, "main"):
		return strings.TrimSpace(mainSessionID)
	}
	return id
}

// chatTurnOrigin says who started a turn.
type chatTurnOrigin struct {
	// unattended names the server-side source of a turn no console started
	// ("focus"). Its tool prompts follow the session's permission mode
	// through the ops approval queue, like cron turns, and its context is
	// the caller's (the server lifetime), not detached from a request.
	unattended string
	// readOnly turns (focus Q&A, in plan mode) never take the repository's
	// write lease, so they never push the session holding it, or
	// themselves, into a worktree.
	readOnly bool
}

// errChatTurnRejected is a turn that never started: its error answer was
// written to w.
type errChatTurnRejected struct {
	status int
	msg    string
	// cause is what made preparing the turn fail. The HTTP answer carries
	// only msg; the server's own callers (the focus driver's blocked gate)
	// get the cause too, so "auto compaction failed" says why (#1231). It is
	// an error from reading or building the session — a path, a decode
	// position — never message text.
	cause error
}

func (e *errChatTurnRejected) Error() string {
	if e.cause == nil {
		return e.msg
	}
	detail := strings.TrimSpace(e.cause.Error())
	if detail == "" || detail == e.msg {
		return e.msg
	}
	return e.msg + ": " + detail
}

func (e *errChatTurnRejected) Unwrap() error { return e.cause }

// runChatTurn is one chat turn from the prepared request to "done": the
// console request and the server's own turns (runServerChatTurn) share it,
// so both go through the same turn feed, cancel registry, worktree lease,
// chat activity, checkpoint, persistence and post-turn focus hook. It
// returns the reply, or the error that ended the turn.
func runChatTurn(w http.ResponseWriter, r *http.Request, req chatRequestPayload, deps chatHandlerDeps, origin chatTurnOrigin) (llm.ChatResponse, error) {
	// Claim the session before anything else: the transcript, feed and
	// cancel belong to one turn at a time. A second turn — a person typing
	// while the focus driver's turn is being prepared, say — is refused.
	claimKey := chatClaimKey(req.SessionID, deps.mainSessionID)
	claim, release, ok := deps.cancelRegistry.Claim(claimKey)
	if !ok {
		writeError(w, http.StatusConflict, "turn_running", errChatTurnBusy.Error())
		return llm.ChatResponse{}, errChatTurnBusy
	}
	// The claim ends just before the turn's last event (done, cancelled,
	// error): a console that sends its next message on that event — a
	// queued follow-up after Stop — finds the session free, not a 409.
	var releaseOnce sync.Once
	releases := []func(){release}
	endClaim := func() {
		releaseOnce.Do(func() {
			for _, r := range releases {
				r()
			}
		})
	}
	defer endClaim()
	endBusy := deps.activity.beginChat()
	defer endBusy()
	var worktreeMoved *worktreeNotice
	if !origin.readOnly {
		moved, endLease := deps.tooling.Worktrees.beginTurn(r.Context(), strings.TrimSpace(req.SessionID), false)
		defer endLease()
		worktreeMoved = moved
	}
	deps.logger.Debug().
		Str("path", r.URL.Path).
		Str("session_id", strings.TrimSpace(req.SessionID)).
		Int("message_len", len(strings.TrimSpace(req.Message))).
		Msg("chat request accepted")

	state, status, errMessage, err := prepareChatRunState(r, req, deps)
	if err != nil {
		writeError(w, status, "", errMessage)
		return llm.ChatResponse{}, &errChatTurnRejected{status: status, msg: errMessage, cause: err}
	}
	if state.sessionID != claimKey {
		// The request named no live session; it runs in a new one.
		var releaseNew func()
		if claim, releaseNew, ok = deps.cancelRegistry.Claim(state.sessionID); !ok {
			writeError(w, http.StatusConflict, "turn_running", errChatTurnBusy.Error())
			return llm.ChatResponse{}, errChatTurnBusy
		}
		releases = append(releases, releaseNew)
	}

	state.interactivePermissions = req.InteractivePermissions && origin.unattended == ""
	state.unattendedSource = origin.unattended
	stream := newChatStreamWriter(w, state.sessionID, deps.logger)
	feed, endFeed := deps.turnFeeds.begin(state.sessionID)
	defer endFeed()
	stream.feed = feed

	stream.activity = deps.chatActivity
	defer deps.chatActivity.begin(state.sessionID)()
	stream.status("stream_open", "stream connected", "", "", "", "")
	if worktreeMoved != nil {
		stream.worktree(*worktreeMoved)
	}
	if state.turnID != "" {
		stream.turnStarted(state.turnID)
	}
	if state.invokedSkill != nil {
		stream.skillSelected(state.invokedSkill.Name, state.invokedSkillReason)
	}
	if state.invokedCommand != nil {
		stream.commandSelected(state.invokedCommand.Name, state.invokedCommandReason)
	}

	// Emit context info for frontend monitoring
	stream.contextInfo(map[string]any{
		"system_prompt_tokens":            systemPromptTokens(state.llmMessages),
		"history_tokens":                  sumHistoryTokens(state.history),
		"history_messages":                len(state.history),
		"tool_count":                      len(state.injectedSchemas),
		"tool_names":                      toolNamesFromSchemas(state.injectedSchemas),
		"skill_count":                     len(state.availableSkillNames),
		"skill_names":                     state.availableSkillNames,
		"command_count":                   len(state.availableCommandNames),
		"command_names":                   state.availableCommandNames,
		"memory_count":                    state.relevantMemoryCount,
		"memory_tokens":                   state.relevantMemoryTokens,
		"compaction_trigger_tokens":       deps.tooling.Compaction.TriggerTokens,
		"compaction_keep_recent_tokens":   deps.tooling.Compaction.KeepRecentTokens,
		"compaction_keep_recent_fraction": deps.tooling.Compaction.KeepRecentFraction,
		"compaction_last_mode":            state.compaction.Mode,
		"used_tool_names":                 []string{},
		"selected_skill_name":             skillNameOrEmpty(state.invokedSkill),
		"selected_skill_reason":           state.invokedSkillReason,
		"selected_capability_version_ids": state.capabilityVersionIDs,
		"selected_command_name":           skillNameOrEmpty(state.invokedCommand),
		"selected_command_reason":         state.invokedCommandReason,
		"mentioned_path_count":            len(state.mentionedPaths),
		"mentioned_paths":                 state.mentionedPaths,
		"mentioned_subagent_count":        len(state.mentionedSubagents),
		"mentioned_subagents":             chatSubagentMentionNames(state.mentionedSubagents),
		"llm_tier":                        state.llmResolution.Tier.String(),
		"llm_provider":                    state.llmResolution.Provider,
		"llm_model":                       state.llmResolution.Model,
		"llm_tier_source":                 state.llmResolution.Source,
		"tier_recommendation":             state.tierRecommendation.contextPayload(),
		"style_effective":                 state.sessionStyle,
	})
	if state.compaction.Applied {
		stream.compactionApplied(map[string]any{
			"mode":                    state.compaction.Mode,
			"original_count":          state.compaction.OriginalCount,
			"final_count":             state.compaction.FinalCount,
			"compacted_count":         state.compaction.CompactedCount,
			"trigger_tokens":          state.compaction.TriggerTokens,
			"estimated_tokens_before": state.compaction.EstimatedTokensBefore,
		})
	}

	// Detached from the request: the turn keeps running when the console
	// goes away, and POST /v1/chat/cancel is what stops it. A server turn's
	// context is already the server's, so shutdown stops it too.
	parentCtx := r.Context()
	if origin.unattended == "" {
		parentCtx = context.WithoutCancel(parentCtx)
	}
	baseCtx := usage.WithCallMeta(parentCtx, usage.CallMeta{
		Source:               "chat",
		SessionID:            state.sessionID,
		CapabilityVersionIDs: state.capabilityVersionIDs,
	})
	chatCtx, cancelChat := context.WithCancel(baseCtx)
	defer cancelChat()
	claim.setCancel(cancelChat)

	recordTierRecommendationSignal(deps.tooling.UsageTracker, state, "requested", llm.Usage{})
	checkpointTurn := beginChatCheckpoint(chatCtx, deps, state, req.Message)
	state.turnText = newChatTurnText()
	chatResp, deltaSent, toolCalls, err := executeChatLoop(chatCtx, deps, state, stream)
	// Before the outcome branches: an error or a cancel leaves edits on disk too.
	endChatCheckpoint(chatCtx, checkpointTurn, stream, deps.logger, state.sessionID)
	if err != nil {
		if chatCtx.Err() == context.Canceled {
			persistInterruptedTurn(state, req.Message, chatResp, toolCalls, deps.logger)
			endClaim()
			stream.cancelled()
			recordTierRecommendationSignal(deps.tooling.UsageTracker, state, "cancelled", chatResp.Usage)
			deps.logger.Debug().Str("session_id", state.sessionID).Msg("chat request cancelled")
			return chatResp, err
		}
		persistInterruptedTurn(state, req.Message, llm.ChatResponse{}, toolCalls, deps.logger)
		endClaim()
		stream.error(err)
		recordTierRecommendationSignal(deps.tooling.UsageTracker, state, "error", llm.Usage{})
		return llm.ChatResponse{}, err
	}
	if !deltaSent && chatResp.Message.Content != "" {
		deps.logger.Debug().
			Str("session_id", state.sessionID).
			Int("assistant_len", len(chatResp.Message.Content)).
			Msg("emit fallback delta from non-streaming llm response")
		stream.delta(chatResp.Message.Content)
	}

	persistChatResult(state, req.Message, chatResp, toolCalls, deps.logger)
	recordTierRecommendationSignal(deps.tooling.UsageTracker, state, "completed", chatResp.Usage)

	// Fire-and-forget: warm cache for next turn based on current user message
	startMemoryPrefetchForNextTurn(
		state.requestWorkspaceDir,
		req.Message,
		state.sessionID,
		deps.tooling.MemorySemanticConfig,
		deps.tooling.MemoryCache,
		deps.tooling.PlanClarifyMode,
	)

	if p, act, ok := focusAfterTurn(state.store, state.sessionID, state.transcriptPath, chatResp.Message.Content, state.focusMark, time.Now(), deps.logger); ok {
		stream.pipeline(p, focusNextPrompt(act))
		deps.tooling.Focus.afterTurn(r.Context(), state.sessionID, act, serverauth.RoleFromRequest(r))
	}

	endClaim()
	stream.done(chatResp.Usage)
	deps.logger.Debug().Str("session_id", state.sessionID).Msg("chat request complete")
	return chatResp, nil
}
