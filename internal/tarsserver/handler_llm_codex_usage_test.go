package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
)

type stubCodexClient struct {
	*llm.FakeClient
	snap    llm.CodexRateLimitSnapshot
	present bool
}

func (s *stubCodexClient) LastCodexRateLimit() (llm.CodexRateLimitSnapshot, bool) {
	return s.snap, s.present
}

func mustNewRouter(t *testing.T, tiers map[llm.Tier]llm.TierEntry) llm.Router {
	t.Helper()
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers:       tiers,
		DefaultTier: llm.TierStandard,
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	return router
}

func TestCodexRateLimitAPIHandler_ReturnsSnapshotsPerTier(t *testing.T) {
	heavyClient := &stubCodexClient{
		FakeClient: &llm.FakeClient{Label: "heavy"},
		snap: llm.CodexRateLimitSnapshot{
			Primary:    &llm.CodexRateLimitWindow{UsedPercent: 25, WindowMinutes: 300},
			RawHeaders: map[string]string{"x-codex-credits-remaining": "9001"},
		},
		present: true,
	}
	standardClient := &llm.FakeClient{Label: "standard"} // not a Codex source
	lightClient := &stubCodexClient{
		FakeClient: &llm.FakeClient{Label: "light"},
		// present=false: tier listed but no snapshot yet
	}

	router := mustNewRouter(t, map[llm.Tier]llm.TierEntry{
		llm.TierHeavy:    {Client: heavyClient, Provider: "openai-codex", Model: "gpt-5.3-codex"},
		llm.TierStandard: {Client: standardClient, Provider: "anthropic", Model: "claude"},
		llm.TierLight:    {Client: lightClient, Provider: "openai-codex", Model: "gpt-5.3-codex-mini"},
	})

	handler := newCodexRateLimitAPIHandler(router, "off")
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/llm/codex/usage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var got codexUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(got.Tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(got.Tiers))
	}

	byTier := map[string]codexUsageTierResponse{}
	for _, t := range got.Tiers {
		byTier[t.Tier] = t
	}
	heavy := byTier["heavy"]
	if heavy.Provider != "openai-codex" || heavy.Model != "gpt-5.3-codex" {
		t.Errorf("heavy meta: %+v", heavy)
	}
	if heavy.Snapshot == nil || heavy.Snapshot.Primary == nil || heavy.Snapshot.Primary.UsedPercent != 25 {
		t.Errorf("heavy snapshot: %+v", heavy.Snapshot)
	}
	if heavy.Snapshot.RawHeaders["x-codex-credits-remaining"] != "9001" {
		t.Errorf("heavy raw headers: %+v", heavy.Snapshot.RawHeaders)
	}

	std := byTier["standard"]
	if std.Snapshot != nil {
		t.Errorf("standard tier should have nil snapshot (non-codex inner): %+v", std.Snapshot)
	}

	light := byTier["light"]
	if light.Snapshot != nil {
		t.Errorf("light tier should have nil snapshot (no request seen): %+v", light.Snapshot)
	}
	if light.Provider != "openai-codex" {
		t.Errorf("light provider should still be exposed: %+v", light)
	}
}

func TestCodexRateLimitAPIHandler_ForbidsNonAdminWhenAuthRequired(t *testing.T) {
	router := mustNewRouter(t, map[llm.Tier]llm.TierEntry{
		llm.TierHeavy:    {Client: &llm.FakeClient{}, Provider: "openai-codex", Model: "m"},
		llm.TierStandard: {Client: &llm.FakeClient{}, Provider: "openai-codex", Model: "m"},
		llm.TierLight:    {Client: &llm.FakeClient{}, Provider: "openai-codex", Model: "m"},
	})
	handler := newCodexRateLimitAPIHandler(router, "required")

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/llm/codex/usage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCodexRateLimitAPIHandler_RejectsNonGET(t *testing.T) {
	router := mustNewRouter(t, map[llm.Tier]llm.TierEntry{
		llm.TierHeavy:    {Client: &llm.FakeClient{}, Provider: "x", Model: "m"},
		llm.TierStandard: {Client: &llm.FakeClient{}, Provider: "x", Model: "m"},
		llm.TierLight:    {Client: &llm.FakeClient{}, Provider: "x", Model: "m"},
	})
	handler := newCodexRateLimitAPIHandler(router, "off")
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/llm/codex/usage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestCodexRateLimitAPIHandler_NilRouter(t *testing.T) {
	handler := newCodexRateLimitAPIHandler(nil, "off")
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/llm/codex/usage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}
