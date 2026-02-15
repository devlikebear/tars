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
