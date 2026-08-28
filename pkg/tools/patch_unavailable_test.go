package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestApplyPatchReportsMissingPatchUtility covers the branch that turns a
// missing dependency into its own message. Without it the tool reported
// "patch apply failed", which reads as though the patch text were bad.
func TestApplyPatchReportsMissingPatchUtility(t *testing.T) {
	previous := patchExecutable
	patchExecutable = func() (string, error) {
		return "", fmt.Errorf("%w: nothing installed", ErrPatchUnavailable)
	}
	t.Cleanup(func() { patchExecutable = previous })

	tool := NewApplyPatchTool(t.TempDir(), true)
	params := json.RawMessage(`{"patch":"--- a/x.txt\n+++ b/x.txt\n@@ -0,0 +1 @@\n+hello\n"}`)
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("execute apply_patch: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected an error result when patch is unavailable")
	}
	if text := result.Text(); !strings.Contains(text, ErrPatchUnavailable.Error()) {
		t.Fatalf("expected the message to name the missing utility, got %q", text)
	}
	if errors.Is(err, ErrPatchUnavailable) {
		t.Fatal("the tool reports through its result, not a Go error")
	}
}
