package cli

import "strings"

// ExitError wraps an error with a process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return e.Err.Error()
}

// IsFlagError returns true if the error looks like a CLI flag parsing error.
func IsFlagError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "requires at least")
}
