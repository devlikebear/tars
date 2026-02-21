package tool

import "testing"

func TestCanonicalToolName(t *testing.T) {
	if got := CanonicalToolName("shell_execute"); got != "exec" {
		t.Fatalf("expected shell_execute -> exec, got %q", got)
	}
	if got := CanonicalToolName("  EXEC "); got != "exec" {
		t.Fatalf("expected exec normalization, got %q", got)
	}
	if got := CanonicalToolName("read_file"); got != "read_file" {
		t.Fatalf("expected unknown name to stay normalized, got %q", got)
	}
}

func TestIsExecToolName(t *testing.T) {
	if !IsExecToolName("exec") {
		t.Fatal("expected exec to be recognized")
	}
	if !IsExecToolName("shell_execute") {
		t.Fatal("expected shell_execute alias to be recognized")
	}
	if IsExecToolName("read_file") {
		t.Fatal("did not expect read_file to be recognized as exec")
	}
}
