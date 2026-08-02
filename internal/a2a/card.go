package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultMaxCardBytes int64 = 256 * 1024

type DiscoveryOptions struct {
	HTTPClient        *http.Client
	AllowLoopbackHTTP bool
	AllowPrivateHosts bool
	AllowedHosts      []string
	MaxResponseBytes  int64
}

func Discover(ctx context.Context, baseURL string, options DiscoveryOptions) (AgentCard, AgentInterface, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return AgentCard{}, AgentInterface{}, fmt.Errorf("%w: parse discovery URL", ErrUnsafeEndpoint)
	}
	policy := EndpointPolicy{
		AllowLoopbackHTTP: options.AllowLoopbackHTTP,
		AllowPrivateHosts: options.AllowPrivateHosts,
		AllowedHosts:      options.AllowedHosts,
	}
	if err := validateNetworkEndpoint(ctx, base, policy); err != nil {
		return AgentCard{}, AgentInterface{}, err
	}
	discoveryURL := *base
	discoveryURL.Path = AgentCardPath
	discoveryURL.RawPath = ""
	discoveryURL.RawQuery = ""
	discoveryURL.Fragment = ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL.String(), nil)
	if err != nil {
		return AgentCard{}, AgentInterface{}, fmt.Errorf("a2a: build discovery request: %w", err)
	}
	request.Header.Set("Accept", "application/json, "+MediaType)
	response, err := noRedirectClient(options.HTTPClient).Do(request)
	if err != nil {
		if errors.Is(err, errRedirectBlocked) {
			return AgentCard{}, AgentInterface{}, fmt.Errorf("a2a: discovery redirect is blocked")
		}
		return AgentCard{}, AgentInterface{}, fmt.Errorf("a2a: discover agent card: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return AgentCard{}, AgentInterface{}, fmt.Errorf("a2a: discovery returned HTTP %d", response.StatusCode)
	}
	if err := validateJSONContentType(response.Header.Get("Content-Type"), true); err != nil {
		return AgentCard{}, AgentInterface{}, err
	}
	limit := options.MaxResponseBytes
	if limit <= 0 {
		limit = defaultMaxCardBytes
	}
	body, err := readBounded(response.Body, limit)
	if err != nil {
		return AgentCard{}, AgentInterface{}, err
	}
	var card AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return AgentCard{}, AgentInterface{}, fmt.Errorf("%w: decode card", ErrInvalidCard)
	}
	endpoint, err := SelectHTTPJSONV1(card, policy)
	if err != nil {
		return AgentCard{}, AgentInterface{}, err
	}
	selected, _ := url.Parse(endpoint.URL)
	if !sameHost(selected.Hostname(), base.Hostname()) {
		if len(options.AllowedHosts) == 0 {
			return AgentCard{}, AgentInterface{}, fmt.Errorf("%w: card endpoint changed discovery host", ErrUnsafeEndpoint)
		}
	}
	if err := validateNetworkEndpoint(ctx, selected, policy); err != nil {
		return AgentCard{}, AgentInterface{}, err
	}
	return card, endpoint, nil
}

func SelectHTTPJSONV1(card AgentCard, policy EndpointPolicy) (AgentInterface, error) {
	if strings.TrimSpace(card.Name) == "" || strings.TrimSpace(card.Description) == "" ||
		strings.TrimSpace(card.Version) == "" || card.Capabilities == nil ||
		len(card.DefaultInputModes) == 0 || len(card.DefaultOutputModes) == 0 || len(card.Skills) == 0 {
		return AgentInterface{}, fmt.Errorf("%w: name, description, version, capabilities, modes, and skills are required", ErrInvalidCard)
	}
	for _, skill := range card.Skills {
		if strings.TrimSpace(skill.ID) == "" || strings.TrimSpace(skill.Name) == "" ||
			strings.TrimSpace(skill.Description) == "" || len(skill.Tags) == 0 {
			return AgentInterface{}, fmt.Errorf("%w: skills require id, name, description, and tags", ErrInvalidCard)
		}
	}
	var sawHTTPJSON bool
	var sawV1 bool
	for _, endpoint := range card.SupportedInterfaces {
		if endpoint.ProtocolBinding == BindingHTTPJSON {
			sawHTTPJSON = true
		}
		if endpoint.ProtocolVersion == ProtocolVersion {
			sawV1 = true
		}
		if endpoint.ProtocolBinding != BindingHTTPJSON || endpoint.ProtocolVersion != ProtocolVersion {
			continue
		}
		target, err := url.Parse(strings.TrimSpace(endpoint.URL))
		if err != nil {
			return AgentInterface{}, fmt.Errorf("%w: parse interface URL", ErrInvalidCard)
		}
		if err := validateEndpointSyntax(target, policy); err != nil {
			return AgentInterface{}, err
		}
		return endpoint, nil
	}
	if !sawHTTPJSON {
		return AgentInterface{}, fmt.Errorf("%w: HTTP+JSON interface is required", ErrProtocol)
	}
	if !sawV1 {
		return AgentInterface{}, fmt.Errorf("%w: protocol 1.0 interface is required", ErrProtocol)
	}
	return AgentInterface{}, fmt.Errorf("%w: no HTTP+JSON 1.0 interface", ErrProtocol)
}

func validateJSONContentType(value string, allowApplicationJSON bool) error {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("%w: invalid response content type", ErrInvalidResponse)
	}
	if mediaType == MediaType || allowApplicationJSON && mediaType == "application/json" {
		return nil
	}
	return fmt.Errorf("%w: unexpected response content type", ErrInvalidResponse)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, ErrResponseLimit
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("a2a: read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, ErrResponseLimit
	}
	return body, nil
}

var errRedirectBlocked = errors.New("a2a redirect blocked")

func noRedirectClient(source *http.Client) *http.Client {
	if source == nil {
		source = &http.Client{Timeout: 15 * time.Second}
	}
	clone := *source
	if clone.Timeout <= 0 {
		clone.Timeout = 15 * time.Second
	}
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errRedirectBlocked
	}
	return &clone
}
