package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ImageGenerator draws one image from a prompt. It is a separate interface
// from Client because only some providers can do it: assert a Client to
// ImageGenerator to find out. The openai-codex and gemini-native clients
// implement it.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, req ImageRequest) (ImageResult, error)
}

var (
	_ ImageGenerator = (*OpenAICodexClient)(nil)
	_ ImageGenerator = (*GeminiNativeClient)(nil)
)

// ImageAspect is the shape a generated image is asked to have. Providers
// treat it as a request, not a size: read the real dimensions from the
// returned bytes.
type ImageAspect string

const (
	ImageAspectSquare    ImageAspect = "square"    // 1:1
	ImageAspectPortrait  ImageAspect = "portrait"  // 2:3
	ImageAspectLandscape ImageAspect = "landscape" // 3:2
)

// ImageRequest is what GenerateImage sends. The prompt goes to the provider
// as given; nothing is added to it except the provider's own aspect
// instruction.
type ImageRequest struct {
	Prompt string
	// References are images the provider should draw from, such as a
	// character's design sheet. Each is a ContentBlock of Type "image" with
	// base64 Data and its MediaType.
	References []ContentBlock
	// Aspect defaults to ImageAspectSquare.
	Aspect ImageAspect
}

// ImageResult is one generated image.
type ImageResult struct {
	Data      []byte
	MediaType string
	// RevisedPrompt is the prompt the provider says it actually drew from,
	// when it rewrites the one it was given.
	RevisedPrompt string
}

var (
	// ErrImageRefused means the provider declined to draw the prompt, usually
	// a safety or content policy block. Retrying the same prompt will not
	// help; the error text carries the provider's reason.
	ErrImageRefused = errors.New("image generation refused by provider")
	// ErrImageBillingRequired means the credential's account has no image
	// quota at all: a gemini-native key from a free-tier project. A key from
	// a project with billing set up is the fix, not waiting.
	ErrImageBillingRequired = errors.New("image generation needs a billing-enabled account")
)

// maxImageReferences bounds how many reference images one request carries.
// Both providers accept more, but each costs input tokens and request size.
const maxImageReferences = 8

func (r ImageRequest) validate(provider string) (ImageAspect, error) {
	if strings.TrimSpace(r.Prompt) == "" {
		return "", newProviderError(provider, "build_request", errors.New("image prompt is empty"))
	}
	if len(r.References) > maxImageReferences {
		return "", newProviderError(provider, "build_request", fmt.Errorf("at most %d reference images, got %d", maxImageReferences, len(r.References)))
	}
	for i, ref := range r.References {
		if ref.Type != "image" || strings.TrimSpace(ref.Data) == "" || !strings.HasPrefix(ref.MediaType, "image/") {
			return "", newProviderError(provider, "build_request", fmt.Errorf("reference %d is not a base64 image block", i))
		}
	}
	switch r.Aspect {
	case "":
		return ImageAspectSquare, nil
	case ImageAspectSquare, ImageAspectPortrait, ImageAspectLandscape:
		return r.Aspect, nil
	}
	return "", newProviderError(provider, "build_request", fmt.Errorf("unknown image aspect %q", r.Aspect))
}

func imageRefused(provider, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return newProviderError(provider, "image", ErrImageRefused)
	}
	return newProviderError(provider, "image", fmt.Errorf("%w: %s", ErrImageRefused, reason))
}
