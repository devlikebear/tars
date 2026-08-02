// Package a2a implements TARS' bounded interoperability edge for the
// Agent2Agent protocol. It intentionally does not model TARS workers or
// replace the durable Work Ledger.
package a2a

import (
	"encoding/json"
	"errors"
)

const (
	ProtocolVersion = "1.0"
	BindingHTTPJSON = "HTTP+JSON"
	AgentCardPath   = "/.well-known/agent-card.json"
	MediaType       = "application/a2a+json"
)

var (
	ErrInvalidCard     = errors.New("a2a: invalid agent card")
	ErrUnsafeEndpoint  = errors.New("a2a: unsafe endpoint")
	ErrProtocol        = errors.New("a2a: unsupported protocol")
	ErrRequestLimit    = errors.New("a2a: request limit exceeded")
	ErrResponseLimit   = errors.New("a2a: response limit exceeded")
	ErrInvalidMessage  = errors.New("a2a: invalid message")
	ErrInvalidResponse = errors.New("a2a: invalid agent response")
	ErrAuthentication  = errors.New("a2a: authentication failed")
)

type AgentCard struct {
	Name                 string                     `json:"name"`
	Description          string                     `json:"description"`
	SupportedInterfaces  []AgentInterface           `json:"supportedInterfaces"`
	Provider             *AgentProvider             `json:"provider,omitempty"`
	Version              string                     `json:"version"`
	DocumentationURL     string                     `json:"documentationUrl,omitempty"`
	Capabilities         *AgentCapabilities         `json:"capabilities"`
	SecuritySchemes      map[string]json.RawMessage `json:"securitySchemes,omitempty"`
	SecurityRequirements []SecurityRequirement      `json:"securityRequirements,omitempty"`
	DefaultInputModes    []string                   `json:"defaultInputModes"`
	DefaultOutputModes   []string                   `json:"defaultOutputModes"`
	Skills               []AgentSkill               `json:"skills"`
	Signatures           []AgentCardSignature       `json:"signatures,omitempty"`
	IconURL              string                     `json:"iconUrl,omitempty"`
}

type AgentInterface struct {
	URL             string `json:"url"`
	ProtocolBinding string `json:"protocolBinding"`
	ProtocolVersion string `json:"protocolVersion"`
	Tenant          string `json:"tenant,omitempty"`
}

type AgentProvider struct {
	URL          string `json:"url"`
	Organization string `json:"organization"`
}

type AgentCapabilities struct {
	Streaming         *bool            `json:"streaming,omitempty"`
	PushNotifications *bool            `json:"pushNotifications,omitempty"`
	ExtendedAgentCard *bool            `json:"extendedAgentCard,omitempty"`
	Extensions        []AgentExtension `json:"extensions,omitempty"`
}

type AgentExtension struct {
	URI         string          `json:"uri"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Params      json.RawMessage `json:"params,omitempty"`
}

type AgentSkill struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Description          string                `json:"description"`
	Tags                 []string              `json:"tags"`
	Examples             []string              `json:"examples,omitempty"`
	InputModes           []string              `json:"inputModes,omitempty"`
	OutputModes          []string              `json:"outputModes,omitempty"`
	SecurityRequirements []SecurityRequirement `json:"securityRequirements,omitempty"`
}

type SecurityRequirement struct {
	Schemes map[string]StringList `json:"schemes"`
}

type StringList struct {
	List []string `json:"list"`
}

type AgentCardSignature struct {
	Protected string          `json:"protected"`
	Signature string          `json:"signature"`
	Header    json.RawMessage `json:"header,omitempty"`
}

type Role string

const (
	RoleUnspecified Role = "ROLE_UNSPECIFIED"
	RoleUser        Role = "ROLE_USER"
	RoleAgent       Role = "ROLE_AGENT"
)

type Part struct {
	Text      *string         `json:"text,omitempty"`
	Raw       string          `json:"raw,omitempty"`
	URL       string          `json:"url,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Filename  string          `json:"filename,omitempty"`
	MediaType string          `json:"mediaType,omitempty"`
}

func NewTextPart(text string) Part {
	return Part{Text: &text, MediaType: "text/plain"}
}

type Message struct {
	MessageID        string          `json:"messageId"`
	ContextID        string          `json:"contextId,omitempty"`
	TaskID           string          `json:"taskId,omitempty"`
	Role             Role            `json:"role"`
	Parts            []Part          `json:"parts"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
	Extensions       []string        `json:"extensions,omitempty"`
	ReferenceTaskIDs []string        `json:"referenceTaskIds,omitempty"`
}

type SendMessageConfiguration struct {
	AcceptedOutputModes []string `json:"acceptedOutputModes,omitempty"`
	HistoryLength       *int32   `json:"historyLength,omitempty"`
	ReturnImmediately   bool     `json:"returnImmediately,omitempty"`
}

type SendMessageRequest struct {
	Tenant        string                   `json:"tenant,omitempty"`
	Message       Message                  `json:"message"`
	Configuration SendMessageConfiguration `json:"configuration,omitempty"`
	Metadata      json.RawMessage          `json:"metadata,omitempty"`
}

type SendMessageResponse struct {
	Task    *Task    `json:"task,omitempty"`
	Message *Message `json:"message,omitempty"`
}

type TaskState string

const (
	TaskStateUnspecified   TaskState = "TASK_STATE_UNSPECIFIED"
	TaskStateSubmitted     TaskState = "TASK_STATE_SUBMITTED"
	TaskStateWorking       TaskState = "TASK_STATE_WORKING"
	TaskStateCompleted     TaskState = "TASK_STATE_COMPLETED"
	TaskStateFailed        TaskState = "TASK_STATE_FAILED"
	TaskStateCanceled      TaskState = "TASK_STATE_CANCELED"
	TaskStateInputRequired TaskState = "TASK_STATE_INPUT_REQUIRED"
	TaskStateRejected      TaskState = "TASK_STATE_REJECTED"
	TaskStateAuthRequired  TaskState = "TASK_STATE_AUTH_REQUIRED"
)

func (state TaskState) Terminal() bool {
	switch state {
	case TaskStateCompleted, TaskStateFailed, TaskStateCanceled, TaskStateRejected:
		return true
	default:
		return false
	}
}

func (state TaskState) Interrupted() bool {
	return state == TaskStateInputRequired || state == TaskStateAuthRequired
}

type TaskStatus struct {
	State     TaskState `json:"state"`
	Message   *Message  `json:"message,omitempty"`
	Timestamp string    `json:"timestamp,omitempty"`
}

type Task struct {
	ID        string          `json:"id"`
	ContextID string          `json:"contextId,omitempty"`
	Status    TaskStatus      `json:"status"`
	Artifacts []Artifact      `json:"artifacts,omitempty"`
	History   []Message       `json:"history,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type Artifact struct {
	ArtifactID  string          `json:"artifactId"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parts       []Part          `json:"parts"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Extensions  []string        `json:"extensions,omitempty"`
}
