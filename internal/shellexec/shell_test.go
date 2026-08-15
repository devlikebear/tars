package shellexec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveExecutableReturnsAbsoluteExistingShell calls resolveExecutable
// rather than Executable so the result is not served from the process-wide
// cache, which another test or an earlier call may already have populated.
func TestResolveExecutableReturnsAbsoluteExistingShell(t *testing.T) {
	shell, err := resolveExecutable()
	if err != nil {
		t.Fatalf("resolve shell: %v", err)
	}
	if !filepath.IsAbs(shell) {
		t.Fatalf("expected an absolute path, got %q", shell)
	}
	info, err := os.Stat(shell)
	if err != nil {
		t.Fatalf("stat resolved shell %q: %v", shell, err)
	}
	if info.IsDir() {
		t.Fatalf("resolved shell %q is a directory", shell)
	}
}

// TestResolvedShellRunsPOSIXCommand is the assertion that matters: the point of
// resolving rather than hardcoding is to end up with something that actually
// interprets POSIX command strings, which is what every caller feeds it.
func TestResolvedShellRunsPOSIXCommand(t *testing.T) {
	shell, err := resolveExecutable()
	if err != nil {
		t.Fatalf("resolve shell: %v", err)
	}
	out, err := exec.Command(shell, "-c", "echo tars && exit 0").CombinedOutput()
	if err != nil {
		t.Fatalf("run command through %q: %v (output %q)", shell, err, out)
	}
	if !strings.Contains(string(out), "tars") {
		t.Fatalf("expected command output to contain %q, got %q", "tars", out)
	}
}
