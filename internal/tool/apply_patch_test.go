package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func hasPatchBinary() bool {
	_, err := exec.LookPath("patch")
	return err == nil
}

func TestApplyPatchTool_Disabled(t *testing.T) {
	root := t.TempDir()
	t1 := NewApplyPatchTool(root, false)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"patch":"--- a.txt\n+++ a.txt\n"}`))
	if err != nil {
		t.Fatalf("execute apply_patch: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text(), "disabled") {
		t.Fatalf("expected disabled error, got %s", res.Text())
	}
}

func TestApplyPatchTool_AppliesUnifiedDiff(t *testing.T) {
	if !hasPatchBinary() {
		t.Skip("patch binary is not available")
	}
	root := t.TempDir()
	path := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	patch := `--- hello.txt
+++ hello.txt
@@ -1 +1 @@
-hello
+world
`
	t1 := NewApplyPatchTool(root, true)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"patch":`+jsonQuote(patch)+`}`))
	if err != nil {
		t.Fatalf("execute apply_patch: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %s", res.Text())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	if string(body) != "world\n" {
		t.Fatalf("unexpected patched content: %q", string(body))
	}
}

func TestApplyPatchTool_RejectsEscapedPath(t *testing.T) {
	root := t.TempDir()
	patch := `--- ../escape.txt
+++ ../escape.txt
@@ -0,0 +1 @@
+boom
`
	t1 := NewApplyPatchTool(root, true)
	res, err := t1.Execute(context.Background(), json.RawMessage(`{"patch":`+jsonQuote(patch)+`}`))
	if err != nil {
		t.Fatalf("execute apply_patch: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected escaped-path error")
	}
}

func jsonQuote(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}
