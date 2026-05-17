package embodiment

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rs/zerolog"
)

func TestRouteCapabilityAwareDelivery(t *testing.T) {
	dispatcher := &recordingActionDispatcher{}
	router := NewRouter(dispatcher, zerolog.New(io.Discard))
	provider := ProviderDescriptor{
		Name:         "mock",
		Enabled:      true,
		Transport:    TransportMCP,
		Capabilities: []Capability{CapabilitySpeech},
	}

	delivered := router.Route(context.Background(), BodyAction{
		Kind:    ActionSpeak,
		Payload: map[string]any{"text": "hello"},
	}, provider)
	if !delivered.Delivered || delivered.Dropped || delivered.RequiredCapability != CapabilitySpeech {
		t.Fatalf("speak route = %+v", delivered)
	}
	if len(dispatcher.actions) != 1 || dispatcher.actions[0].Payload["text"] != "hello" {
		t.Fatalf("dispatched actions = %+v", dispatcher.actions)
	}

	dropped := router.Route(context.Background(), BodyAction{
		Kind:    ActionExpress,
		Payload: map[string]any{"emotion": "happy"},
	}, provider)
	if dropped.Delivered || !dropped.Dropped || dropped.Reason != RouteReasonCapabilityNotSupported {
		t.Fatalf("express route = %+v", dropped)
	}
	if len(dispatcher.actions) != 1 {
		t.Fatalf("unsupported expression should not dispatch: %+v", dispatcher.actions)
	}
}

func TestRouteGracefulDrops(t *testing.T) {
	router := NewRouter(&recordingActionDispatcher{err: errors.New("send failed")}, zerolog.New(io.Discard))
	provider := ProviderDescriptor{
		Name:         "mock",
		Enabled:      true,
		Transport:    TransportMCP,
		Capabilities: []Capability{CapabilitySpeech},
	}

	failed := router.Route(context.Background(), BodyAction{Kind: ActionSpeak, Payload: map[string]any{"text": "hello"}}, provider)
	if failed.Delivered || !failed.Dropped || failed.Reason != RouteReasonDispatchFailed {
		t.Fatalf("dispatch failure route = %+v", failed)
	}

	disabled := router.Route(context.Background(), BodyAction{Kind: ActionSpeak, Payload: map[string]any{"text": "hello"}}, ProviderDescriptor{
		Name:         "mock",
		Enabled:      false,
		Capabilities: []Capability{CapabilitySpeech},
	})
	if disabled.Delivered || !disabled.Dropped || disabled.Reason != RouteReasonProviderDisabled {
		t.Fatalf("disabled provider route = %+v", disabled)
	}
}

func TestRouteAllKeepsPartialDelivery(t *testing.T) {
	dispatcher := &recordingActionDispatcher{}
	router := NewRouter(dispatcher, zerolog.New(io.Discard))
	results := router.RouteAll(context.Background(), []BodyAction{
		{Kind: ActionSpeak, Payload: map[string]any{"text": "hello"}},
		{Kind: ActionExpress, Payload: map[string]any{"emotion": "happy"}},
	}, ProviderDescriptor{
		Name:         "mock",
		Enabled:      true,
		Transport:    TransportMCP,
		Capabilities: []Capability{CapabilitySpeech},
	})
	if len(results) != 2 || !results[0].Delivered || !results[1].Dropped {
		t.Fatalf("RouteAll results = %+v", results)
	}
	if len(dispatcher.actions) != 1 {
		t.Fatalf("delivered actions = %+v, want one", dispatcher.actions)
	}
}

type recordingActionDispatcher struct {
	actions []BodyAction
	err     error
}

func (d *recordingActionDispatcher) Dispatch(_ context.Context, _ ProviderDescriptor, action BodyAction) error {
	d.actions = append(d.actions, action)
	return d.err
}
