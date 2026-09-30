package tarsserver

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func tierPinTestRouter(t *testing.T) llm.Router {
	t.Helper()
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy:    {Client: &llm.FakeClient{Label: "heavy"}, Model: "heavy-model", Provider: "fake"},
			llm.TierStandard: {Client: &llm.FakeClient{Label: "standard"}, Model: "standard-model", Provider: "fake"},
			llm.TierLight:    {Client: &llm.FakeClient{Label: "light"}, Model: "light-model", Provider: "fake"},
		},
		DefaultTier:  llm.TierStandard,
		RoleDefaults: map[llm.Role]llm.Tier{llm.RoleChatMain: llm.TierStandard},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return router
}

func tierPinTestStore(t *testing.T) (string, *session.Store) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	return root, session.NewStore(root)
}

func appendTierPinHistory(t *testing.T, store *session.Store, id string) {
	t.Helper()
	path := store.TranscriptPath(id)
	for _, role := range []string{"user", "assistant"} {
		if err := session.AppendMessage(path, session.Message{Role: role, Content: "earlier " + role}); err != nil {
			t.Fatalf("append message: %v", err)
		}
	}
}

func buildTierPinState(t *testing.T, root string, store *session.Store, id string, input *chatTierRecommendationPayload) chatRunState {
	t.Helper()
	deps := chatHandlerDeps{workspaceDir: root, store: store, router: tierPinTestRouter(t), logger: zerolog.Nop()}
	state, err := buildSessionChatRunState(root, "", store, id, "keep going", nil, nil, input, true, "", deps)
	if err != nil {
		t.Fatalf("buildSessionChatRunState: %v", err)
	}
	return state
}

func TestResolveChatTierRecommendation_SessionPinAppliesWhenRequestHasNoTier(t *testing.T) {
	rec, err := resolveChatTierRecommendation(nil, "hello", false, llm.TierHeavy)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rec.ChosenTier != llm.TierHeavy || rec.Source != chatTierSourceSessionPin {
		t.Fatalf("rec = %+v, want the session pin", rec)
	}
}

func TestResolveChatTierRecommendation_SessionPinBeatsFirstTurnGuess(t *testing.T) {
	rec, err := resolveChatTierRecommendation(nil, "Implement the next issue and verify the release.", true, llm.TierLight)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rec.ChosenTier != llm.TierLight {
		t.Fatalf("chosen = %q, want the pinned light tier", rec.ChosenTier)
	}
}

func TestResolveChatTierRecommendation_RequestTierBeatsSessionPin(t *testing.T) {
	rec, err := resolveChatTierRecommendation(&chatTierRecommendationPayload{RecommendedTier: "light", ChosenTier: "light"}, "hi", false, llm.TierHeavy)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rec.ChosenTier != llm.TierLight {
		t.Fatalf("chosen = %q, want the request's light", rec.ChosenTier)
	}
}

// The reported bug: a tier accepted on the first turn only lasted that turn.
func TestBuildSessionChatRunState_PinnedChoiceHoldsForLaterTurns(t *testing.T) {
	root, store := tierPinTestStore(t)
	sess, err := store.Create("coding")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	first := buildTierPinState(t, root, store, sess.ID, &chatTierRecommendationPayload{
		TaskType: "coding", RecommendedTier: "heavy", ChosenTier: "heavy", Accepted: true, Source: "console", Pin: true,
	})
	if first.llmResolution.Tier != llm.TierHeavy {
		t.Fatalf("first turn tier = %q, want heavy", first.llmResolution.Tier)
	}
	saved, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if saved.TierPin != "heavy" {
		t.Fatalf("TierPin = %q, want heavy saved on the session", saved.TierPin)
	}

	appendTierPinHistory(t, store, sess.ID)
	next := buildTierPinState(t, root, store, sess.ID, nil)
	if next.llmResolution.Tier != llm.TierHeavy {
		t.Fatalf("later turn tier = %q, want the pinned heavy", next.llmResolution.Tier)
	}
	if next.tierRecommendation.Source != chatTierSourceSessionPin {
		t.Fatalf("later turn source = %q, want %q", next.tierRecommendation.Source, chatTierSourceSessionPin)
	}
}

func TestBuildSessionChatRunState_UnpinnedChoiceDoesNotStick(t *testing.T) {
	root, store := tierPinTestStore(t)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	buildTierPinState(t, root, store, sess.ID, &chatTierRecommendationPayload{RecommendedTier: "heavy", ChosenTier: "heavy", Accepted: true})
	saved, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if saved.TierPin != "" {
		t.Fatalf("TierPin = %q, want none without pin:true", saved.TierPin)
	}
	appendTierPinHistory(t, store, sess.ID)
	next := buildTierPinState(t, root, store, sess.ID, nil)
	if next.llmResolution.Tier != llm.TierStandard {
		t.Fatalf("later turn tier = %q, want the default standard", next.llmResolution.Tier)
	}
}

func TestSessionAPIPatchTierPin(t *testing.T) {
	_, store := tierPinTestStore(t)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	other, err := store.Create("other")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.Nop())

	pinned := patchSessionJSON(t, handler, sess.ID, `{"tier_pin":"Heavy"}`)
	if pinned.TierPin != "heavy" {
		t.Fatalf("TierPin = %q, want heavy", pinned.TierPin)
	}
	if got, _ := store.Get(other.ID); got.TierPin != "" {
		t.Fatalf("other session got pin %q", got.TierPin)
	}

	status, body := patchSessionRaw(t, handler, sess.ID, `{"tier_pin":"turbo"}`)
	if status != http.StatusBadRequest || !strings.Contains(string(body), "tier") {
		t.Fatalf("unknown tier: status %d body %q, want 400", status, body)
	}

	cleared := patchSessionJSON(t, handler, sess.ID, `{"tier_pin":""}`)
	if cleared.TierPin != "" {
		t.Fatalf("TierPin = %q after clearing", cleared.TierPin)
	}
}
