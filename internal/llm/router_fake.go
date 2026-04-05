package llm

import (
	"context"
	"fmt"
)

// FakeClient is a minimal Client implementation for tests. Each call
// records its arguments and returns a canned response so that test code
// can assert which tier/role was resolved.
type FakeClient struct {
	// Label identifies this client in assertions (e.g. "heavy", "light").
	Label string
	// AskResponse is returned from Ask; if empty, a default is synthesized.
	AskResponse string
	// ChatResponse is returned from Chat; if zero-valued, a default is
	// synthesized that echoes Label in the message content.
	ChatResponse ChatResponse
	// AskCalls counts invocations of Ask.
	AskCalls int
	// ChatCalls counts invocations of Chat.
	ChatCalls int
}

// Ask implements llm.Client.
func (f *FakeClient) Ask(_ context.Context, prompt string) (string, error) {
	f.AskCalls++
	if f.AskResponse != "" {
		return f.AskResponse, nil
	}
	return fmt.Sprintf("%s:ask:%s", f.Label, prompt), nil
}

// Chat implements llm.Client.
func (f *FakeClient) Chat(_ context.Context, _ []ChatMessage, _ ChatOptions) (ChatResponse, error) {
	f.ChatCalls++
	if f.ChatResponse.Message.Role != "" || f.ChatResponse.Message.Content != "" {
		return f.ChatResponse, nil
	}
	return ChatResponse{
		Message:    ChatMessage{Role: "assistant", Content: fmt.Sprintf("%s:chat", f.Label)},
		StopReason: "stop",
	}, nil
}

// NewFakeRouter builds a Router backed by FakeClients, one per tier.
// Each tier gets a FakeClient labelled with the tier name. Useful in tests
// that need to assert which tier was used for which call.
func NewFakeRouter(defaultTier Tier, roleDefaults map[Role]Tier) (Router, map[Tier]*FakeClient, error) {
	clients := map[Tier]*FakeClient{
		TierHeavy:    {Label: "heavy"},
		TierStandard: {Label: "standard"},
		TierLight:    {Label: "light"},
	}
	tiers := map[Tier]TierEntry{
		TierHeavy:    {Client: clients[TierHeavy], Provider: "fake", Model: "fake-heavy"},
		TierStandard: {Client: clients[TierStandard], Provider: "fake", Model: "fake-standard"},
		TierLight:    {Client: clients[TierLight], Provider: "fake", Model: "fake-light"},
	}
	router, err := NewRouter(RouterConfig{
		Tiers:        tiers,
		DefaultTier:  defaultTier,
		RoleDefaults: roleDefaults,
	})
	if err != nil {
		return nil, nil, err
	}
	return router, clients, nil
}
