// Package embodiment defines the provider-neutral body subsystem contract.
//
// Phase 1 intentionally keeps this package dormant: it models perceptions,
// body capabilities, providers, and future actions without registering any
// LLM tools or touching hardware-specific code.
package embodiment

import "time"

type Capability string

const (
	CapabilityVision     Capability = "vision"
	CapabilityHearing    Capability = "hearing"
	CapabilitySpeech     Capability = "speech"
	CapabilityExpression Capability = "expression"
	CapabilityMotion     Capability = "motion"
	CapabilityLED        Capability = "led"
)

type Modality string

const (
	ModalityVision Modality = "vision"
	ModalityAudio  Modality = "audio"
	ModalitySensor Modality = "sensor"
)

type OwnerState string

const (
	OwnerOwner    OwnerState = "owner"
	OwnerStranger OwnerState = "stranger"
	OwnerUnknown  OwnerState = "unknown"
	OwnerNone     OwnerState = "none"
)

type Transport string

const (
	TransportMCP     Transport = "mcp"
	TransportWebhook Transport = "webhook"
)

type ActionKind string

const (
	ActionSpeak   ActionKind = "speak"
	ActionExpress ActionKind = "express"
	ActionMove    ActionKind = "move"
	ActionLED     ActionKind = "led"
)

type Percept struct {
	ID            string
	Provider      string
	Modality      Modality
	Owner         OwnerState
	Summary       string
	Labels        []string
	MediaRef      string
	Trigger       string
	Salience      float64
	SessionID     string
	ThreadID      string
	IsSelfSensory bool
	CapturedAt    time.Time
	Raw           map[string]any
}

type ProviderDescriptor struct {
	Name         string
	Capabilities []Capability
	Enabled      bool
	Transport    Transport
	Endpoint     string
}

type BodyAction struct {
	Kind    ActionKind
	Payload map[string]any
}

type GateMode string

const (
	GateModeDirective   GateMode = "directive"
	GateModeObservation GateMode = "observation"
)

const (
	GateReasonOwnerVoice  = "owner_voice"
	GateReasonObservation = "observation"
	GateReasonExternal    = "external"
	GateReasonDebounce    = "debounce"
	GateReasonRateLimited = "rate_limited"
	GateReasonDisabled    = "disabled"
)

type GateDecision struct {
	Trigger bool     `json:"trigger"`
	Mode    GateMode `json:"mode"`
	Reason  string   `json:"reason,omitempty"`
}

const (
	CognitionReasonTriggered = "triggered"
	CognitionReasonSkipped   = "skipped"
	CognitionReasonInFlight  = "in_flight"
)

type CognitionResult struct {
	Triggered bool   `json:"triggered"`
	RunID     string `json:"run_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type IngestResult struct {
	Percept         Percept         `json:"percept"`
	Decision        GateDecision    `json:"decision"`
	CognitionResult CognitionResult `json:"cognition"`
}
