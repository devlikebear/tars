package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobTool_ReturnsMatches(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "b.md"), []byte("b"), 0o644)
	t1 := NewGlobTool(root)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"pattern":"*.txt"}`))
	if err != nil {
		t.Fatalf("execute glob: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "a.txt") {
		t.Fatalf("expected txt match, got %s", res.Text())
	}
}
