package llm

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	zlog "github.com/rs/zerolog/log"
)

// codexImageInstructions keeps the routing model to one tool call. When the
// tool fails (a moderation block), the model otherwise calls it again on its
// own, which doubles the wait and the ChatGPT quota spent for the same
// refusal.
const codexImageInstructions = "You are an illustrator. Call the image generation tool exactly once to draw what the user describes. " +
	"If the tool fails, do not call it again; reply with one short sentence."

// The hosted image tool ignores the size parameter on this backend, so the
// aspect is asked for in words. These sentences moved the result from
// 1536x1024 to 1024x1536 where size alone did not.
var codexImageAspectInstructions = map[ImageAspect]string{
	ImageAspectSquare:    "Square composition, 1:1 aspect ratio.",
	ImageAspectPortrait:  "Vertical portrait orientation, 2:3 aspect ratio (taller than wide).",
	ImageAspectLandscape: "Horizontal landscape orientation, 3:2 aspect ratio (wider than tall).",
}

// codexImageMaxLine bounds one SSE line. The line carrying the finished image
// holds the whole base64 result: about 225 KB as JPEG, over 2 MB as WebP or
// as a partial image, which the 1 MiB chat scanner would reject.
const codexImageMaxLine = 16 * 1024 * 1024

// GenerateImage draws one image with the Responses API's hosted
// image_generation tool, signed in with the ChatGPT account. The client's
// model is the routing model that calls the tool, not the image model; a
// ChatGPT account accepts only the models its Codex model list shows.
// The result is JPEG.
func (c *OpenAICodexClient) GenerateImage(ctx context.Context, req ImageRequest) (ImageResult, error) {
	aspect, err := req.validate(openAICodexProviderLabel)
	if err != nil {
		return ImageResult{}, err
	}
	cred, err := c.getCredential()
	if err != nil {
		return ImageResult{}, newProviderError(openAICodexProviderLabel, "auth", err)
	}
	body := map[string]any{
		"model":        c.model,
		"instructions": codexImageInstructions + " " + codexImageAspectInstructions[aspect],
		"store":        false,
		"stream":       true,
		"input": []any{map[string]any{
			"type":    "message",
			"role":    "user",
			"content": toCodexUserContent(ChatMessage{Role: "user", Content: req.Prompt, ContentBlocks: req.References}),
		}},
		"tools":       []any{map[string]any{"type": "image_generation", "output_format": "jpeg"}},
		"tool_choice": map[string]any{"type": "image_generation"},
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return ImageResult{}, newProviderError(openAICodexProviderLabel, "parse", fmt.Errorf("marshal request: %w", err))
	}
	// The body carries the reference images, so it is not logged whole.
	zlog.Debug().Str("provider", openAICodexProviderLabel).Int("payload_len", len(rawBody)).
		Int("references", len(req.References)).Msg("llm image request")

	resp, _, _, err := c.postResponses(ctx, cred, rawBody, true)
	if err != nil {
		return ImageResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkHTTPStatus(resp, openAICodexProviderLabel); err != nil {
		return ImageResult{}, err
	}
	return parseOpenAICodexImageSSE(resp.Body)
}

// parseOpenAICodexImageSSE returns the first finished image_generation_call.
// A failed call is a refusal; so is a response that completes without one.
func parseOpenAICodexImageSSE(body io.Reader) (ImageResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), codexImageMaxLine)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return ImageResult{}, newProviderError(openAICodexProviderLabel, "parse", fmt.Errorf("decode stream event: %w", err))
		}
		switch strings.TrimSpace(getStringAny(event["type"])) {
		case "response.output_item.done":
			item := castMapAny(event["item"])
			if getStringAny(item["type"]) != "image_generation_call" {
				continue
			}
			return codexImageFromItem(item)
		case "response.completed", "response.done", "response.incomplete":
			return ImageResult{}, imageRefused(openAICodexProviderLabel, "the response finished without an image")
		case "response.failed", "error":
			return ImageResult{}, newProviderError(openAICodexProviderLabel, "request", errors.New(extractOpenAICodexErrorMessage(event)))
		}
	}
	if err := scanner.Err(); err != nil {
		return ImageResult{}, newProviderError(openAICodexProviderLabel, "stream", fmt.Errorf("read stream response: %w", err))
	}
	return ImageResult{}, newProviderError(openAICodexProviderLabel, "stream", errors.New("stream ended before the image arrived"))
}

func codexImageFromItem(item map[string]any) (ImageResult, error) {
	if status := getStringAny(item["status"]); status != "completed" {
		errObj := castMapAny(item["error"])
		reason := firstNonEmptyAny(errObj["code"], errObj["message"], status)
		return ImageResult{}, imageRefused(openAICodexProviderLabel, reason)
	}
	data, err := base64.StdEncoding.DecodeString(getStringAny(item["result"]))
	if err != nil || len(data) == 0 {
		return ImageResult{}, newProviderError(openAICodexProviderLabel, "parse", fmt.Errorf("decode image result: %v", err))
	}
	mediaType := http.DetectContentType(data)
	if !strings.HasPrefix(mediaType, "image/") {
		return ImageResult{}, newProviderError(openAICodexProviderLabel, "parse", fmt.Errorf("image result is %s, not an image", mediaType))
	}
	return ImageResult{
		Data:          data,
		MediaType:     mediaType,
		RevisedPrompt: strings.TrimSpace(getStringAny(item["revised_prompt"])),
	}, nil
}
