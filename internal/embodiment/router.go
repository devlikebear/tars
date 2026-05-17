package embodiment

import (
	"context"
	"strings"

	"github.com/rs/zerolog"
)

type ActionDispatcher interface {
	Dispatch(context.Context, ProviderDescriptor, BodyAction) error
}

type Router struct {
	dispatcher ActionDispatcher
	logger     zerolog.Logger
}

func NewRouter(dispatcher ActionDispatcher, logger zerolog.Logger) *Router {
	return &Router{dispatcher: dispatcher, logger: logger}
}

func (r *Router) Route(ctx context.Context, action BodyAction, provider ProviderDescriptor) RouteResult {
	normalized, err := NormalizeBodyAction(action)
	if err != nil {
		return routeDrop(normalized, provider, "", RouteReasonInvalidAction, err)
	}
	required, ok := RequiredCapability(normalized.Kind)
	if !ok {
		return routeDrop(normalized, provider, "", RouteReasonInvalidAction, nil)
	}
	if !provider.Enabled {
		return routeDrop(normalized, provider, required, RouteReasonProviderDisabled, nil)
	}
	if !providerHasCapability(provider, required) {
		result := routeDrop(normalized, provider, required, RouteReasonCapabilityNotSupported, nil)
		r.logRoute(result)
		return result
	}
	if r == nil || r.dispatcher == nil {
		return routeDrop(normalized, provider, required, RouteReasonNoDispatcher, nil)
	}
	if err := r.dispatcher.Dispatch(ctx, provider, normalized); err != nil {
		result := routeDrop(normalized, provider, required, RouteReasonDispatchFailed, err)
		r.logRoute(result)
		return result
	}
	result := RouteResult{
		Action:             normalized,
		Provider:           normalizeName(provider.Name),
		RequiredCapability: required,
		Delivered:          true,
		Reason:             RouteReasonDelivered,
	}
	r.logRoute(result)
	return result
}

func (r *Router) RouteAll(ctx context.Context, actions []BodyAction, provider ProviderDescriptor) []RouteResult {
	results := make([]RouteResult, 0, len(actions))
	for _, action := range actions {
		results = append(results, r.Route(ctx, action, provider))
	}
	return results
}

func RequiredCapability(kind ActionKind) (Capability, bool) {
	switch ActionKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case ActionSpeak:
		return CapabilitySpeech, true
	case ActionExpress:
		return CapabilityExpression, true
	case ActionMove:
		return CapabilityMotion, true
	case ActionLED:
		return CapabilityLED, true
	default:
		return "", false
	}
}

func providerHasCapability(provider ProviderDescriptor, required Capability) bool {
	for _, capability := range provider.Capabilities {
		if Capability(strings.ToLower(strings.TrimSpace(string(capability)))) == required {
			return true
		}
	}
	return false
}

func routeDrop(action BodyAction, provider ProviderDescriptor, required Capability, reason string, err error) RouteResult {
	result := RouteResult{
		Action:             action,
		Provider:           normalizeName(provider.Name),
		RequiredCapability: required,
		Dropped:            true,
		Reason:             reason,
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func (r *Router) logRoute(result RouteResult) {
	if r == nil {
		return
	}
	event := r.logger.Info().
		Str("provider", result.Provider).
		Str("action", string(result.Action.Kind)).
		Str("capability", string(result.RequiredCapability)).
		Str("reason", result.Reason)
	if result.Error != "" {
		event = event.Str("error", result.Error)
	}
	if result.Delivered {
		event.Msg("embodiment action delivered")
		return
	}
	event.Msg("embodiment action dropped")
}
