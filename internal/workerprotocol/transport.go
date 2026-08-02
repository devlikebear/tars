package workerprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrTransportConfig = errors.New("workerprotocol: transport configuration is invalid")
	ErrTransportLimit  = errors.New("workerprotocol: transport payload exceeds limit")
	ErrWireContract    = errors.New("workerprotocol: wire contract is invalid")
)

type WireLimits struct {
	MaxRequestBytes  int64 `json:"max_request_bytes"`
	MaxResponseBytes int64 `json:"max_response_bytes"`
}

func DefaultWireLimits() WireLimits {
	return WireLimits{MaxRequestBytes: 320 << 20, MaxResponseBytes: 64 << 20}
}

func (limits WireLimits) Validate() error {
	if limits.MaxRequestBytes <= 0 || limits.MaxResponseBytes <= 0 {
		return ErrTransportConfig
	}
	return nil
}

type WireRequest struct {
	ProtocolVersion string           `json:"protocol_version"`
	RequestID       string           `json:"request_id"`
	Envelope        Envelope         `json:"envelope"`
	Workspace       *WorkspaceBundle `json:"workspace,omitempty"`
}

type WireArtifact struct {
	Name      string `json:"name"`
	Digest    string `json:"digest"`
	MediaType string `json:"media_type,omitempty"`
	Data      []byte `json:"data"`
}

type WireResponse struct {
	ProtocolVersion string             `json:"protocol_version"`
	RequestID       string             `json:"request_id"`
	Accepted        bool               `json:"accepted"`
	ErrorCode       string             `json:"error_code,omitempty"`
	Error           string             `json:"error,omitempty"`
	Payload         json.RawMessage    `json:"payload,omitempty"`
	Artifacts       []WireArtifact     `json:"artifacts,omitempty"`
	Checkpoint      *CheckpointPayload `json:"checkpoint,omitempty"`
}

type WorkerTransport interface {
	Exchange(context.Context, WireRequest) (WireResponse, error)
}

type WireHandler interface {
	Handle(context.Context, WireRequest) (WireResponse, error)
}

func (request WireRequest) Validate() error {
	if request.ProtocolVersion != ProtocolVersionV1 || !validProtocolIdentifier(request.RequestID) || request.RequestID != request.Envelope.MessageID {
		return ErrWireContract
	}
	if err := request.Envelope.Validate(); err != nil {
		return errors.Join(ErrWireContract, err)
	}
	if request.Workspace == nil {
		return nil
	}
	if request.Envelope.Type != MessageSync {
		return fmt.Errorf("%w: workspace bundle requires sync message", ErrWireContract)
	}
	if err := VerifyWorkspaceBundle(*request.Workspace, DefaultWorkspaceBundleLimits()); err != nil {
		return errors.Join(ErrWireContract, err)
	}
	var payload SyncPayload
	if err := decodePayload(request.Envelope.Payload, &payload); err != nil || payload.Digest != request.Workspace.Manifest.Digest || payload.Mode != request.Workspace.Manifest.Mode {
		return fmt.Errorf("%w: sync payload does not bind workspace manifest", ErrWireContract)
	}
	return nil
}

func (response WireResponse) Validate(requestID string) error {
	if response.ProtocolVersion != ProtocolVersionV1 || !validProtocolIdentifier(response.RequestID) || response.RequestID != requestID {
		return ErrWireContract
	}
	if !response.Accepted && strings.TrimSpace(response.ErrorCode) == "" {
		return ErrWireContract
	}
	if len(response.Payload) > 0 && !json.Valid(response.Payload) {
		return ErrWireContract
	}
	for _, artifact := range response.Artifacts {
		if !safeWorkspaceRelativePath(artifact.Name) || strings.TrimSpace(artifact.Digest) == "" {
			return ErrWireContract
		}
	}
	if response.Checkpoint != nil && (!validProtocolIdentifier(response.Checkpoint.ID) || strings.TrimSpace(response.Checkpoint.Digest) == "") {
		return ErrWireContract
	}
	return nil
}

func encodeWireRequest(request WireRequest, limits WireLimits) ([]byte, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: encode wire request: %w", err)
	}
	if int64(len(raw)+1) > limits.MaxRequestBytes {
		return nil, ErrTransportLimit
	}
	return append(raw, '\n'), nil
}

func decodeWireRequest(raw []byte, limits WireLimits) (WireRequest, error) {
	if err := limits.Validate(); err != nil {
		return WireRequest{}, err
	}
	if int64(len(raw)) > limits.MaxRequestBytes {
		return WireRequest{}, ErrTransportLimit
	}
	var request WireRequest
	if err := decodeSingleJSONLine(raw, &request); err != nil {
		return WireRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return WireRequest{}, err
	}
	return request, nil
}

func encodeWireResponse(response WireResponse, requestID string, limits WireLimits) ([]byte, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if err := response.Validate(requestID); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: encode wire response: %w", err)
	}
	if int64(len(raw)+1) > limits.MaxResponseBytes {
		return nil, ErrTransportLimit
	}
	return append(raw, '\n'), nil
}

func decodeWireResponse(raw []byte, requestID string, limits WireLimits) (WireResponse, error) {
	if err := limits.Validate(); err != nil {
		return WireResponse{}, err
	}
	if int64(len(raw)) > limits.MaxResponseBytes {
		return WireResponse{}, ErrTransportLimit
	}
	var response WireResponse
	if err := decodeSingleJSONLine(raw, &response); err != nil {
		return WireResponse{}, err
	}
	if err := response.Validate(requestID); err != nil {
		return WireResponse{}, err
	}
	return response, nil
}

func decodeSingleJSONLine(raw []byte, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.ContainsRune(trimmed, '\n') {
		return ErrWireContract
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrWireContract, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrWireContract
	}
	return nil
}

type InProcessTransport struct {
	handler WireHandler
	limits  WireLimits
}

func NewInProcessTransport(handler WireHandler, limits WireLimits) (*InProcessTransport, error) {
	if handler == nil {
		return nil, ErrTransportConfig
	}
	if limits == (WireLimits{}) {
		limits = DefaultWireLimits()
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return &InProcessTransport{handler: handler, limits: limits}, nil
}

func (transport *InProcessTransport) Exchange(ctx context.Context, request WireRequest) (WireResponse, error) {
	if transport == nil || transport.handler == nil {
		return WireResponse{}, ErrTransportConfig
	}
	rawRequest, err := encodeWireRequest(request, transport.limits)
	if err != nil {
		return WireResponse{}, err
	}
	wireRequest, err := decodeWireRequest(rawRequest, transport.limits)
	if err != nil {
		return WireResponse{}, err
	}
	response, err := transport.handler.Handle(ctx, wireRequest)
	if err != nil {
		return WireResponse{}, err
	}
	rawResponse, err := encodeWireResponse(response, request.RequestID, transport.limits)
	if err != nil {
		return WireResponse{}, err
	}
	return decodeWireResponse(rawResponse, request.RequestID, transport.limits)
}

var _ WorkerTransport = (*InProcessTransport)(nil)
