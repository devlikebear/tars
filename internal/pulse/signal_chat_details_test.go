package pulse

import (
	"reflect"
	"testing"
)

func TestNewChatSignalDetailsRecordsSharedPrimaryFields(t *testing.T) {
	candidates := []StalledChatCandidate{
		{
			SessionID:         "sess_primary",
			Title:             "Primary",
			LastMessageID:     "msg_primary",
			AgeMinutes:        45,
			AutoResumeEnabled: true,
			CanAutoResume:     false,
			ResumeMode:        "record_assumption_and_proceed",
		},
		{
			SessionID:         "sess_resume",
			Title:             "Can resume",
			LastMessageID:     "msg_resume",
			AgeMinutes:        60,
			AutoResumeEnabled: true,
			CanAutoResume:     true,
		},
	}

	details := newChatSignalDetails(
		"stalled_count",
		candidates,
		"auto_continue_chat",
		map[string]any{"resume_mode": candidates[0].ResumeMode},
	)

	want := map[string]any{
		"stalled_count":             2,
		"session_id":                "sess_primary",
		"session_title":             "Primary",
		"last_message_id":           "msg_primary",
		"age_minutes":               45,
		"can_auto_resume":           false,
		"auto_resume_enabled":       true,
		"block_reason":              "",
		"autofix_candidate":         "auto_continue_chat",
		"sessions":                  candidates,
		"has_auto_resume_candidate": true,
		"resume_mode":               "record_assumption_and_proceed",
	}
	if !reflect.DeepEqual(details, want) {
		t.Fatalf("details = %#v, want %#v", details, want)
	}
}

func TestNewChatSignalDetailsOmitsAggregateAutoResumeWhenAbsent(t *testing.T) {
	candidates := []FailedChatCandidate{
		{
			SessionID:         "sess_failed",
			Title:             "Failed",
			LastMessageID:     "msg_tool_err",
			AgeMinutes:        55,
			FailureKind:       FailedChatKindToolError,
			FailingToolName:   "exec",
			AutoResumeEnabled: true,
			CanAutoResume:     false,
			BlockReason:       "high_risk_failure",
		},
	}

	details := newChatSignalDetails(
		"failed_count",
		candidates,
		"auto_resume_failed_chat",
		map[string]any{
			"failure_kind": FailedChatKindToolError,
			"failing_tool": "exec",
		},
	)

	if _, ok := details["has_auto_resume_candidate"]; ok {
		t.Fatalf("has_auto_resume_candidate should be absent when no candidate can auto-resume: %#v", details)
	}
	if details["failed_count"] != 1 {
		t.Fatalf("failed_count = %#v, want 1", details["failed_count"])
	}
	if details["block_reason"] != "high_risk_failure" {
		t.Fatalf("block_reason = %#v", details["block_reason"])
	}
}
