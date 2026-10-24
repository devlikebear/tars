package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	zlog "github.com/rs/zerolog/log"
)

// geminiImageMaxResponse bounds an image response. Gemini 3 image models
// return a thoughtSignature beside the image that is larger than the image
// itself: a 2K picture came back as 10 MB of JSON, past the 10 MiB the chat
// path reads.
const geminiImageMaxResponse = 32 * 1024 * 1024

var geminiImageAspectRatios = map[ImageAspect]string{
	ImageAspectSquare:    "1:1",
	ImageAspectPortrait:  "2:3",
	ImageAspectLandscape: "3:2",
}

// GenerateImage draws one image with generateContent on an image-output
// model (a "-image" model such as gemini-3.1-flash-image); the client's
// model must be one. The aspect ratio is always sent: without it the
// models drew landscape, not the square the documentation promises.
//
// It ignores the client's HTTP timeout, like streaming chat does, and runs
// until ctx ends; image calls took up to 17 s at 2K.
func (c *GeminiNativeClient) GenerateImage(ctx context.Context, req ImageRequest) (ImageResult, error) {
	const provider = "gemini-native"
	aspect, err := req.validate(provider)
	if err != nil {
		return ImageResult{}, err
	}
	parts := make([]*geminiPart, 0, len(req.References)+1)
	for _, ref := range req.References {
		parts = append(parts, &geminiPart{InlineData: &geminiInlineData{MimeType: ref.MediaType, Data: ref.Data}})
	}
	parts = append(parts, &geminiPart{Text: req.Prompt})
	imageConfig := &geminiImageConfig{AspectRatio: geminiImageAspectRatios[aspect]}
	if geminiImageSizeSupported(c.model) {
		imageConfig.ImageSize = "1K"
	}
	encoded, err := json.Marshal(&geminiGenerateRequest{
		Contents: []*geminiContent{{Role: "user", Parts: parts}},
		GenerationConfig: &geminiGenConfig{
			ResponseModalities: []string{"IMAGE"},
			ImageConfig:        imageConfig,
		},
	})
	if err != nil {
		return ImageResult{}, newProviderError(provider, "request", fmt.Errorf("marshal request: %w", err))
	}
	// The body carries the reference images, so it is not logged whole.
	zlog.Debug().Str("provider", provider).Str("model", c.model).Int("payload_len", len(encoded)).
		Int("references", len(req.References)).Msg("llm image request")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.requestURL(false), bytes.NewReader(encoded))
	if err != nil {
		return ImageResult{}, newProviderError(provider, "request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := transportHTTPClient(c.httpClient, true).Do(httpReq)
	if err != nil {
		return ImageResult{}, wrapGeminiHTTPError("request", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, geminiImageMaxResponse))
	if err != nil {
		return ImageResult{}, newProviderError(provider, "request", fmt.Errorf("read response: %w", err))
	}
	if resp.StatusCode != http.StatusOK {
		return ImageResult{}, geminiImageHTTPError(resp.StatusCode, body)
	}

	var decoded geminiGenerateResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ImageResult{}, newProviderError(provider, "parse", fmt.Errorf("decode response: %w", err))
	}
	if len(decoded.Candidates) == 0 {
		return ImageResult{}, imageRefused(provider, "no candidates")
	}
	candidate := decoded.Candidates[0]
	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if part == nil || part.InlineData == nil || !strings.HasPrefix(part.InlineData.MimeType, "image/") {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if err != nil || len(data) == 0 {
				return ImageResult{}, newProviderError(provider, "parse", fmt.Errorf("decode image data: %v", err))
			}
			return ImageResult{Data: data, MediaType: part.InlineData.MimeType}, nil
		}
	}
	// A blocked prompt answers 200 with finishReason PROHIBITED_CONTENT (or
	// another non-STOP reason) and no parts.
	reason := candidate.FinishReason
	if reason == "" || reason == "STOP" {
		reason = "the response held no image"
	}
	return ImageResult{}, imageRefused(provider, reason)
}

// geminiImageSizeSupported reports whether a model takes imageConfig.imageSize.
// The Gemini 3 image models do; gemini-2.5-flash-image has no size setting.
func geminiImageSizeSupported(model string) bool {
	return !strings.Contains(strings.TrimPrefix(model, "models/"), "gemini-2.")
}

// geminiImageHTTPError maps an error response. A key from a free-tier project
// gets 429 RESOURCE_EXHAUSTED with "limit: 0" on every image model: that is
// not a rate limit to wait out, so it is told apart.
func geminiImageHTTPError(status int, body []byte) error {
	var errResp geminiErrorResponse
	_ = json.Unmarshal(body, &errResp)
	message := strings.TrimSpace(errResp.Error.Message)
	providerErr := &ProviderError{
		Provider:   "gemini-native",
		Operation:  "request",
		StatusCode: status,
		Message:    message,
	}
	if status == http.StatusTooManyRequests && strings.Contains(message, "free_tier") && strings.Contains(message, "limit: 0") {
		providerErr.Cause = ErrImageBillingRequired
	}
	return providerErr
}
