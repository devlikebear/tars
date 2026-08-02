package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	defaultMaxRequestBytes  int64 = 256 * 1024
	defaultMaxResponseBytes int64 = 2 * 1024 * 1024
)

var safeTaskID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)

type TokenProvider interface {
	Token(context.Context) (string, error)
}

type TokenProviderFunc func(context.Context) (string, error)

func (provider TokenProviderFunc) Token(ctx context.Context) (string, error) {
	return provider(ctx)
}

type ClientOptions struct {
	HTTPClient        *http.Client
	AllowLoopbackHTTP bool
	AllowPrivateHosts bool
	AllowedHosts      []string
	TokenProvider     TokenProvider
	MaxRequestBytes   int64
	MaxResponseBytes  int64
}

type Client struct {
	endpoint         *url.URL
	interfaceConfig  AgentInterface
	httpClient       *http.Client
	policy           EndpointPolicy
	tokenProvider    TokenProvider
	maxRequestBytes  int64
	maxResponseBytes int64
}

func NewClient(endpoint AgentInterface, options ClientOptions) (*Client, error) {
	if endpoint.ProtocolBinding != BindingHTTPJSON || endpoint.ProtocolVersion != ProtocolVersion {
		return nil, fmt.Errorf("%w: HTTP+JSON 1.0 interface is required", ErrProtocol)
	}
	target, err := url.Parse(strings.TrimSpace(endpoint.URL))
	if err != nil {
		return nil, fmt.Errorf("%w: parse interface URL", ErrUnsafeEndpoint)
	}
	policy := EndpointPolicy{
		AllowLoopbackHTTP: options.AllowLoopbackHTTP,
		AllowPrivateHosts: options.AllowPrivateHosts,
		AllowedHosts:      options.AllowedHosts,
	}
	if err := validateEndpointSyntax(target, policy); err != nil {
		return nil, err
	}
	maxRequestBytes := options.MaxRequestBytes
	if maxRequestBytes <= 0 {
		maxRequestBytes = defaultMaxRequestBytes
	}
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	return &Client{
		endpoint: target, interfaceConfig: endpoint, httpClient: noRedirectClient(options.HTTPClient),
		policy: policy, tokenProvider: options.TokenProvider,
		maxRequestBytes: maxRequestBytes, maxResponseBytes: maxResponseBytes,
	}, nil
}

func (client *Client) Interface() AgentInterface {
	return client.interfaceConfig
}

func (client *Client) SendMessage(ctx context.Context, request SendMessageRequest) (SendMessageResponse, error) {
	if err := validateOutboundMessage(request.Message); err != nil {
		return SendMessageResponse{}, err
	}
	if request.Tenant == "" {
		request.Tenant = client.interfaceConfig.Tenant
	}
	body, err := json.Marshal(request)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("%w: encode send request", ErrInvalidMessage)
	}
	if int64(len(body)) > client.maxRequestBytes {
		return SendMessageResponse{}, ErrRequestLimit
	}
	var response SendMessageResponse
	if err := client.doJSON(ctx, http.MethodPost, "/message:send", body, &response); err != nil {
		return SendMessageResponse{}, err
	}
	if (response.Task == nil) == (response.Message == nil) {
		return SendMessageResponse{}, fmt.Errorf("%w: send response must contain exactly one task or message", ErrInvalidResponse)
	}
	if response.Task != nil {
		if err := validateTask(*response.Task); err != nil {
			return SendMessageResponse{}, err
		}
	}
	if response.Message != nil {
		if err := validateInboundMessage(*response.Message); err != nil {
			return SendMessageResponse{}, err
		}
	}
	return response, nil
}

func (client *Client) GetTask(ctx context.Context, taskID string) (Task, error) {
	if !safeTaskID.MatchString(taskID) {
		return Task{}, fmt.Errorf("%w: invalid task id", ErrInvalidMessage)
	}
	var task Task
	if err := client.doJSON(ctx, http.MethodGet, "/tasks/"+taskID, nil, &task); err != nil {
		return Task{}, err
	}
	if err := validateTask(task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (client *Client) CancelTask(ctx context.Context, taskID string) (Task, error) {
	if !safeTaskID.MatchString(taskID) {
		return Task{}, fmt.Errorf("%w: invalid task id", ErrInvalidMessage)
	}
	var task Task
	if err := client.doJSON(ctx, http.MethodPost, "/tasks/"+taskID+":cancel", []byte(`{}`), &task); err != nil {
		return Task{}, err
	}
	if err := validateTask(task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (client *Client) doJSON(ctx context.Context, method, suffix string, body []byte, output any) error {
	if client == nil || client.endpoint == nil || client.httpClient == nil {
		return fmt.Errorf("a2a: client is not configured")
	}
	if err := validateNetworkEndpoint(ctx, client.endpoint, client.policy); err != nil {
		return err
	}
	target := *client.endpoint
	target.Path = strings.TrimRight(target.Path, "/") + suffix
	target.RawPath = ""
	target.RawQuery = ""
	target.Fragment = ""
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return fmt.Errorf("a2a: build request: %w", err)
	}
	request.Header.Set("Accept", MediaType)
	request.Header.Set("A2A-Version", ProtocolVersion)
	if body != nil {
		request.Header.Set("Content-Type", MediaType)
	}
	if client.tokenProvider != nil {
		token, tokenErr := client.tokenProvider.Token(ctx)
		if tokenErr != nil {
			return ErrAuthentication
		}
		token = strings.TrimSpace(token)
		if token == "" || strings.ContainsAny(token, "\r\n") {
			return ErrAuthentication
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, errRedirectBlocked) {
			return fmt.Errorf("a2a: redirect is blocked")
		}
		return fmt.Errorf("a2a: request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("a2a: remote endpoint returned HTTP %d", response.StatusCode)
	}
	if err := validateJSONContentType(response.Header.Get("Content-Type"), false); err != nil {
		return err
	}
	responseBody, err := readBounded(response.Body, client.maxResponseBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("%w: decode response", ErrInvalidResponse)
	}
	return nil
}

func validateOutboundMessage(message Message) error {
	if err := validateMessageCommon(message); err != nil {
		return err
	}
	if message.Role != RoleUser {
		return fmt.Errorf("%w: outbound role must be ROLE_USER", ErrInvalidMessage)
	}
	for _, part := range message.Parts {
		if part.URL != "" || part.Raw != "" {
			return fmt.Errorf("%w: url and raw parts are disabled", ErrInvalidMessage)
		}
	}
	return nil
}

func validateInboundMessage(message Message) error {
	if err := validateMessageCommon(message); err != nil {
		return err
	}
	if message.Role != RoleAgent {
		return fmt.Errorf("%w: inbound role must be ROLE_AGENT", ErrInvalidResponse)
	}
	return nil
}

func validateMessageCommon(message Message) error {
	if !safeTaskID.MatchString(message.MessageID) || len(message.Parts) == 0 {
		return fmt.Errorf("%w: message id and parts are required", ErrInvalidMessage)
	}
	for _, part := range message.Parts {
		contents := 0
		if part.Text != nil {
			contents++
		}
		if part.Raw != "" {
			contents++
		}
		if part.URL != "" {
			contents++
		}
		if len(part.Data) > 0 {
			contents++
			if !json.Valid(part.Data) {
				return fmt.Errorf("%w: data part is invalid JSON", ErrInvalidMessage)
			}
		}
		if contents != 1 {
			return fmt.Errorf("%w: each part requires exactly one content field", ErrInvalidMessage)
		}
	}
	return nil
}

func validateTask(task Task) error {
	if !safeTaskID.MatchString(task.ID) {
		return fmt.Errorf("%w: task id is required", ErrInvalidResponse)
	}
	if task.ContextID != "" && !safeTaskID.MatchString(task.ContextID) {
		return fmt.Errorf("%w: context id is invalid", ErrInvalidResponse)
	}
	switch task.Status.State {
	case TaskStateSubmitted, TaskStateWorking, TaskStateCompleted, TaskStateFailed, TaskStateCanceled,
		TaskStateInputRequired, TaskStateRejected, TaskStateAuthRequired:
	default:
		return fmt.Errorf("%w: unknown task state", ErrInvalidResponse)
	}
	for _, artifact := range task.Artifacts {
		if strings.TrimSpace(artifact.ArtifactID) == "" || len(artifact.Parts) == 0 {
			return fmt.Errorf("%w: artifact id and parts are required", ErrInvalidResponse)
		}
		for _, part := range artifact.Parts {
			if err := validateMessageCommon(Message{MessageID: "artifact", Parts: []Part{part}}); err != nil {
				return fmt.Errorf("%w: invalid artifact part", ErrInvalidResponse)
			}
		}
	}
	return nil
}
