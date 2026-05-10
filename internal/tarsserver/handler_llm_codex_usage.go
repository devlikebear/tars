package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
)

type codexUsageTierResponse struct {
	Tier     string                      `json:"tier"`
	Provider string                      `json:"provider,omitempty"`
	Model    string                      `json:"model,omitempty"`
	Snapshot *llm.CodexRateLimitSnapshot `json:"snapshot,omitempty"`
}

type codexUsageResponse struct {
	Tiers []codexUsageTierResponse `json:"tiers"`
}

// newCodexRateLimitAPIHandler exposes the most recently observed Codex
// `x-codex-*` rate-limit headers (per tier) for admin consumers. Tiers whose
// underlying client is not a Codex client, or which haven't seen a request
// yet, are still listed but with a nil snapshot — that lets the console
// distinguish "no data yet" from "wrong provider" downstream.
func newCodexRateLimitAPIHandler(router llm.Router, authMode string) http.Handler {
	normalizedAuthMode := serverauth.NormalizeMode(strings.TrimSpace(strings.ToLower(authMode)))
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/admin/llm/codex/usage", func(w http.ResponseWriter, r *http.Request) {
		if router == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "llm router is not configured"})
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if normalizedAuthMode != serverauth.ModeOff && strings.TrimSpace(serverauth.RoleFromContext(r.Context())) != serverauth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		out := codexUsageResponse{Tiers: make([]codexUsageTierResponse, 0, len(llm.AllTiers()))}
		for _, tier := range llm.AllTiers() {
			client, resolution, err := router.ClientForTier(tier)
			if err != nil {
				continue
			}
			entry := codexUsageTierResponse{
				Tier:     string(tier),
				Provider: resolution.Provider,
				Model:    resolution.Model,
			}
			if src, ok := client.(llm.CodexRateLimitSource); ok {
				if snap, present := src.LastCodexRateLimit(); present {
					snapCopy := snap
					entry.Snapshot = &snapCopy
				}
			}
			out.Tiers = append(out.Tiers, entry)
		}

		writeJSON(w, http.StatusOK, out)
	})

	return mux
}
