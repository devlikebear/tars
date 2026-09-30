package llm

import (
	"errors"
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

// UpstreamSessionError marks a failed call whose upstream session was
// saved anyway — a CLI provider that timed out, was cancelled, or failed
// after it had started and stored a resumable session. The caller can keep
// SessionID and pass it back as ChatOptions.ResumeSessionID next turn, so
// the work done before the failure is not lost. The message and error chain
// are Err's.
type UpstreamSessionError struct {
	SessionID string
	Err       error
}

func (e *UpstreamSessionError) Error() string { return e.Err.Error() }

func (e *UpstreamSessionError) Unwrap() error { return e.Err }

// UpstreamSessionIDFromError returns the resumable upstream session a failed
// call left behind, or "" when there is none.
func UpstreamSessionIDFromError(err error) string {
	var sessErr *UpstreamSessionError
	if errors.As(err, &sessErr) {
		return strings.TrimSpace(sessErr.SessionID)
	}
	return ""
}

// withUpstreamSession attaches a saved session's id to err. Providers call
// it only for sessions they know are on disk.
func withUpstreamSession(err error, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if err == nil || sessionID == "" || UpstreamSessionIDFromError(err) != "" {
		return err
	}
	return &UpstreamSessionError{SessionID: sessionID, Err: err}
}

// PartialUsageError marks a failed call that had already spent tokens — a
// CLI provider cut off by a timeout or cancel, or one whose run ended in an
// error. Usage is what the call spent; CostUSD is set only when the
// provider reported a cost itself. ByModel splits Usage by the upstream
// model id that spent it (a CLI can call more than one model in a call), so
// a caller can price the spend without a reported cost. Nil when the
// provider named no model. The message and error chain are Err's.
type PartialUsageError struct {
	Usage   Usage
	ByModel map[string]Usage
	Err     error
}

func (e *PartialUsageError) Error() string { return e.Err.Error() }

func (e *PartialUsageError) Unwrap() error { return e.Err }

// PartialUsageFromError returns what a failed call spent, if its provider
// reported it.
func PartialUsageFromError(err error) (*PartialUsageError, bool) {
	var usageErr *PartialUsageError
	if errors.As(err, &usageErr) {
		return usageErr, true
	}
	return nil, false
}

// withPartialUsage attaches what a failed call spent to err. Nothing spent,
// nothing attached.
func withPartialUsage(err error, spent Usage, byModel map[string]Usage) error {
	if err == nil || spent == (Usage{}) {
		return err
	}
	if _, ok := PartialUsageFromError(err); ok {
		return err
	}
	return &PartialUsageError{Usage: spent, ByModel: byModel, Err: err}
}
