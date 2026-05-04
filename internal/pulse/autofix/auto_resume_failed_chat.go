package autofix

import (
	"context"
	"fmt"
)

const AutoResumeFailedChatName = "auto_resume_failed_chat"

type FailedChatResumeResult struct {
	Resumed    int      `json:"resumed"`
	Skipped    int      `json:"skipped"`
	Escalated  int      `json:"escalated"`
	SessionIDs []string `json:"session_ids,omitempty"`
}

type FailedChatResumer interface {
	ResumeFailedChats(ctx context.Context) (FailedChatResumeResult, error)
}

// AutoResumeFailedChat retries chat turns that halted with a recoverable
// failure (tool error from a non-mutating tool, or a user message the LLM
// never finished responding to). Mutating-tool failures are filtered out at
// detection time, so by the time a candidate reaches this autofix it is safe
// to re-run the turn.
type AutoResumeFailedChat struct {
	Resumer FailedChatResumer
}

func (a *AutoResumeFailedChat) Name() string { return AutoResumeFailedChatName }

func (a *AutoResumeFailedChat) Run(ctx context.Context) (Result, error) {
	if a == nil || a.Resumer == nil {
		return Result{Name: AutoResumeFailedChatName}, fmt.Errorf("failed-chat resumer is not configured")
	}
	result, err := a.Resumer.ResumeFailedChats(ctx)
	if err != nil {
		return Result{Name: AutoResumeFailedChatName}, err
	}
	return Result{
		Name:    AutoResumeFailedChatName,
		Summary: autoResumeFailedChatSummary(result),
		Changed: result.Resumed > 0,
		Details: map[string]any{
			"resumed":     result.Resumed,
			"skipped":     result.Skipped,
			"escalated":   result.Escalated,
			"session_ids": result.SessionIDs,
		},
	}, nil
}

func autoResumeFailedChatSummary(result FailedChatResumeResult) string {
	if result.Resumed > 0 {
		return "retried halted chat turns"
	}
	if result.Escalated > 0 {
		return "halted chats need user attention"
	}
	return "no opted-in halted chats were safe to retry"
}
