package autofix

import (
	"context"
	"fmt"
)

const AutoContinueChatName = "auto_continue_chat"

type ChatAutoContinueResult struct {
	Resumed    int      `json:"resumed"`
	Skipped    int      `json:"skipped"`
	Escalated  int      `json:"escalated"`
	SessionIDs []string `json:"session_ids,omitempty"`
}

type ChatAutoContinuer interface {
	AutoContinueStalledChats(ctx context.Context) (ChatAutoContinueResult, error)
}

type AutoContinueChat struct {
	Continuer ChatAutoContinuer
}

func (a *AutoContinueChat) Name() string { return AutoContinueChatName }

func (a *AutoContinueChat) Run(ctx context.Context) (Result, error) {
	if a == nil || a.Continuer == nil {
		return Result{Name: AutoContinueChatName}, fmt.Errorf("chat auto-continuer is not configured")
	}
	result, err := a.Continuer.AutoContinueStalledChats(ctx)
	if err != nil {
		return Result{Name: AutoContinueChatName}, err
	}
	return Result{
		Name:    AutoContinueChatName,
		Summary: autoContinueChatSummary(result),
		Changed: result.Resumed > 0,
		Details: map[string]any{
			"resumed":     result.Resumed,
			"skipped":     result.Skipped,
			"escalated":   result.Escalated,
			"session_ids": result.SessionIDs,
		},
	}, nil
}

func autoContinueChatSummary(result ChatAutoContinueResult) string {
	if result.Resumed > 0 {
		return "continued stalled chat sessions"
	}
	if result.Escalated > 0 {
		return "stalled chat sessions need user attention"
	}
	return "no opted-in stalled chats were safe to continue"
}
