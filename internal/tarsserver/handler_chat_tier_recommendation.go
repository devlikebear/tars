package tarsserver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

type chatTierRecommendationPayload struct {
	TaskType        string  `json:"task_type,omitempty"`
	RecommendedTier string  `json:"recommended_tier,omitempty"`
	ChosenTier      string  `json:"chosen_tier,omitempty"`
	Reason          string  `json:"reason,omitempty"`
	Confidence      float64 `json:"confidence,omitempty"`
	Accepted        bool    `json:"accepted"`
	Source          string  `json:"source,omitempty"`
	// Pin keeps ChosenTier for every later turn of the session
	// (session.Session.TierPin): the console sets it when the user picks a
	// tier, in the status bar or on the first-turn recommendation card.
	Pin bool `json:"pin,omitempty"`
}

// chatTierSourceSessionPin marks a turn that took the session's pinned tier.
const chatTierSourceSessionPin = "session_pin"

type chatTierRecommendationState struct {
	TaskType        string
	RecommendedTier llm.Tier
	ChosenTier      llm.Tier
	Reason          string
	Confidence      float64
	Accepted        bool
	Source          string
	FirstTurn       bool
}

// resolveChatTierRecommendation picks the turn's tier: the request's choice,
// else the session's pinned tier, else (first turn only) a guess from the
// message. pinned is "" when the session has no pin.
func resolveChatTierRecommendation(input *chatTierRecommendationPayload, message string, firstTurn bool, pinned llm.Tier) (chatTierRecommendationState, error) {
	if input == nil && pinned != "" {
		return chatTierRecommendationState{
			TaskType:        "user_selected",
			RecommendedTier: pinned,
			ChosenTier:      pinned,
			Reason:          "Tier pinned for this session.",
			Confidence:      1,
			Accepted:        true,
			Source:          chatTierSourceSessionPin,
			FirstTurn:       firstTurn,
		}, nil
	}
	if input == nil {
		if !firstTurn {
			return chatTierRecommendationState{}, nil
		}
		rec := llm.RecommendTierForTask(message)
		return chatTierRecommendationState{
			TaskType:        rec.TaskType,
			RecommendedTier: rec.RecommendedTier,
			ChosenTier:      rec.RecommendedTier,
			Reason:          rec.Reason,
			Confidence:      rec.Confidence,
			Accepted:        true,
			Source:          "server",
			FirstTurn:       true,
		}, nil
	}

	recommended, err := llm.ParseTier(input.RecommendedTier)
	if err != nil {
		return chatTierRecommendationState{}, fmt.Errorf("tier recommendation recommended_tier: %w", err)
	}
	chosenRaw := strings.TrimSpace(input.ChosenTier)
	if chosenRaw == "" {
		chosenRaw = recommended.String()
	}
	chosen, err := llm.ParseTier(chosenRaw)
	if err != nil {
		return chatTierRecommendationState{}, fmt.Errorf("tier recommendation chosen_tier: %w", err)
	}
	taskType := strings.TrimSpace(input.TaskType)
	if taskType == "" {
		taskType = "general"
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "Tier selected by request payload."
	}
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "client"
	}
	return chatTierRecommendationState{
		TaskType:        taskType,
		RecommendedTier: recommended,
		ChosenTier:      chosen,
		Reason:          reason,
		Confidence:      input.Confidence,
		Accepted:        input.Accepted,
		Source:          source,
		FirstTurn:       firstTurn,
	}, nil
}

// sessionTierPin is the session's pinned tier, or "" when there is none or it
// cannot be read. A pin that no longer parses is ignored rather than failing
// every turn of the session.
func sessionTierPin(sess session.Session, err error) llm.Tier {
	if err != nil || strings.TrimSpace(sess.TierPin) == "" {
		return ""
	}
	tier, parseErr := llm.ParseTier(sess.TierPin)
	if parseErr != nil {
		return ""
	}
	return tier
}

// pinChatTierIfAsked saves the turn's chosen tier as the session's pin when
// the request asked for it (tier_recommendation.pin).
func pinChatTierIfAsked(store *session.Store, sess *session.Session, input *chatTierRecommendationPayload, state chatTierRecommendationState) error {
	if input == nil || !input.Pin || !state.enabled() {
		return nil
	}
	tier := state.ChosenTier.String()
	if sess.TierPin == tier {
		return nil
	}
	if err := store.SetTierPin(sess.ID, tier); err != nil {
		return fmt.Errorf("pin session tier: %w", err)
	}
	sess.TierPin = tier
	return nil
}

func (s chatTierRecommendationState) enabled() bool {
	return s.RecommendedTier != "" && s.ChosenTier != ""
}

func (s chatTierRecommendationState) contextPayload() map[string]any {
	if !s.enabled() {
		return nil
	}
	return map[string]any{
		"task_type":        s.TaskType,
		"recommended_tier": s.RecommendedTier.String(),
		"chosen_tier":      s.ChosenTier.String(),
		"reason":           s.Reason,
		"confidence":       s.Confidence,
		"accepted":         s.Accepted,
		"source":           s.Source,
		"first_turn":       s.FirstTurn,
	}
}

func recordTierRecommendationSignal(tracker *usage.Tracker, state chatRunState, outcome string, responseUsage llm.Usage) {
	if tracker == nil || !state.tierRecommendation.enabled() {
		return
	}
	dimensions := map[string]string{
		"task_type":        state.tierRecommendation.TaskType,
		"recommended_tier": state.tierRecommendation.RecommendedTier.String(),
		"chosen_tier":      state.tierRecommendation.ChosenTier.String(),
		"accepted":         strconv.FormatBool(state.tierRecommendation.Accepted),
		"source":           state.tierRecommendation.Source,
		"outcome":          outcome,
	}
	if state.llmResolution.Provider != "" {
		dimensions["provider"] = state.llmResolution.Provider
	}
	if state.llmResolution.Model != "" {
		dimensions["model"] = state.llmResolution.Model
	}
	if responseUsage.InputTokens > 0 || responseUsage.OutputTokens > 0 {
		dimensions["input_tokens"] = strconv.Itoa(responseUsage.InputTokens)
		dimensions["output_tokens"] = strconv.Itoa(responseUsage.OutputTokens)
		cost, pricingKnown, costSource := tracker.CallCost(state.llmResolution.Provider, state.llmResolution.Model, responseUsage)
		dimensions["estimated_cost_usd"] = fmt.Sprintf("%.6f", cost)
		dimensions["pricing_known"] = strconv.FormatBool(pricingKnown)
		if costSource != "" {
			dimensions["cost_source"] = costSource
		}
	}
	_ = tracker.RecordSignal(usage.SignalEntry{
		Name:       "llm_tier_recommendation",
		Count:      1,
		Source:     "chat",
		SessionID:  state.sessionID,
		Dimensions: dimensions,
	})
}
