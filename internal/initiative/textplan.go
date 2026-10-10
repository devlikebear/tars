package initiative

// PlanInput is the resolved facts a text-signal plan decides from. The
// caller (tarsserver) resolves the configured backend — a jev client or an
// LLM role — before building this; PlanText itself touches no network and
// no config file, so every branch is table-tested.
type PlanInput struct {
	// Backend is "llm" or "jev"; anything else is treated as "llm" (the
	// default) by PlanText.
	Backend string

	// Jev fields, read only when Backend == "jev".
	JevConfigured bool
	JevLoopback   bool
	JevHost       string
	JevModel      string

	// LLM fields, read only when Backend != "jev".
	//
	// LLMResolved is false when the initiative role's tier failed to
	// resolve (missing alias, tier, model — config.ResolveLLMTier error).
	LLMResolved bool
	// LLMSupportsDecisionOnly is false for a provider kind that cannot be
	// forced into a tool-free, strict-JSON call (antigravity-cli). See
	// llm.SupportsDecisionOnly.
	LLMSupportsDecisionOnly bool
	// LLMProviderAlias/LLMChatProviderAlias are the provider pool aliases
	// (internal/config.ResolvedLLMTier.ProviderAlias) backing the
	// initiative role and the chat role (RoleChatMain, or the default tier
	// when chat has no explicit mapping) respectively. User text already
	// went to the chat provider during the turn that produced it; sending
	// it again to a *different* credential/endpoint would be a new
	// disclosure, so the two aliases must match exactly — same kind and
	// base URL is not enough, since two providers of the same kind can
	// hold different credentials.
	LLMProviderAlias     string
	LLMChatProviderAlias string
	LLMKind              string
	LLMModel             string
	LLMTier              string
}

// BackendPlan is what a resolved text-signal backend will do this tick:
// whether it can be called at all, whether it reads the user's own words,
// and why — surfaced verbatim by GET /v1/initiative/status and `tars
// doctor`.
type BackendPlan struct {
	Backend string `json:"backend"`
	// Usable reports whether the backend can be called at all. False means
	// no text-signal call happens this tick regardless of gating — the
	// runtime treats this exactly like no backend configured.
	Usable bool `json:"usable"`
	// SendsText reports whether user text (recent messages, USER.md) goes
	// into the state sent to the backend, given Usable. False still allows
	// the call; it just asks over metadata only.
	SendsText bool `json:"sends_text"`
	// Reason is one of: jev_loopback, jev_remote, jev_not_configured,
	// llm_same_provider, llm_other_provider, llm_unavailable,
	// llm_unsupported_provider.
	Reason   string `json:"reason"`
	Kind     string `json:"kind,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Tier     string `json:"tier,omitempty"`
}

// PlanText decides, from already-resolved facts, whether a text-signal
// backend can be called this tick and whether the user's own words may go
// with that call. It never performs I/O.
func PlanText(in PlanInput) BackendPlan {
	if in.Backend == "jev" {
		return planJevText(in)
	}
	return planLLMText(in)
}

func planJevText(in PlanInput) BackendPlan {
	plan := BackendPlan{Backend: "jev", Kind: "jev", Provider: in.JevHost, Model: in.JevModel}
	if !in.JevConfigured {
		plan.Reason = "jev_not_configured"
		return plan
	}
	plan.Usable = true
	if in.JevLoopback {
		plan.SendsText = true
		plan.Reason = "jev_loopback"
		return plan
	}
	plan.Reason = "jev_remote"
	return plan
}

func planLLMText(in PlanInput) BackendPlan {
	plan := BackendPlan{Backend: "llm", Kind: in.LLMKind, Provider: in.LLMProviderAlias, Model: in.LLMModel, Tier: in.LLMTier}
	if !in.LLMResolved {
		plan.Reason = "llm_unavailable"
		return plan
	}
	if !in.LLMSupportsDecisionOnly {
		plan.Reason = "llm_unsupported_provider"
		return plan
	}
	plan.Usable = true
	if in.LLMProviderAlias != "" && in.LLMProviderAlias == in.LLMChatProviderAlias {
		plan.SendsText = true
		plan.Reason = "llm_same_provider"
		return plan
	}
	plan.Reason = "llm_other_provider"
	return plan
}
