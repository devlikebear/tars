package llm

import (
	"fmt"
	"strings"
)

// ProviderError is a structured error for LLM provider failures.
type ProviderError struct {
	Provider   string
	Operation  string
	StatusCode int
	Message    string
	Cause      error
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s status %d: %s", e.Provider, e.StatusCode, e.Message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s %s: %v", e.Provider, e.Operation, e.Cause)
	}
	return fmt.Sprintf("%s %s: %v", e.Provider, e.Operation, e.Message)
}

func (e *ProviderError) Unwrap() error {
	return e.Cause
}

func newProviderError(provider, operation string, cause error) *ProviderError {
	return &ProviderError{
		Provider:  provider,
		Operation: operation,
		Message:   "",
		Cause:     cause,
	}
}

func newHTTPError(provider string, statusCode int, body string) *ProviderError {
	return &ProviderError{
		Provider:   provider,
		Operation:  "request",
		StatusCode: statusCode,
		Message:    strings.TrimSpace(body),
	}
}

// newPDFUnsupportedError flags a PDF-bearing message that the named provider
// cannot ingest. Replaces the previous silent placeholder which let a PDF
// pass through as throwaway text the model would treat as a literal note
// rather than a document.
func newPDFUnsupportedError(provider string) *ProviderError {
	return &ProviderError{
		Provider:  provider,
		Operation: "build_request",
		Message:   "pdf_unsupported_by_provider: this provider does not support PDF document blocks; convert to text or images before sending",
	}
}

// containsPDFDocumentBlock reports whether any message carries a PDF document.
func containsPDFDocumentBlock(messages []ChatMessage) bool {
	for _, msg := range messages {
		for _, block := range msg.ContentBlocks {
			if strings.EqualFold(block.Type, "document") {
				return true
			}
		}
	}
	return false
}
