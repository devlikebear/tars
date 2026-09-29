package proofverifier

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProcessRunnerKeepsStdoutFreeOfShellStartupOutput pins the property the
// proof record depends on: a verified command's stdout is the command's own
// output and nothing else.
//
// Run used to invoke the shell with -lc. A login shell sources the startup
// files — /etc/profile, /etc/profile.d/*, ~/.profile — before the command
// runs, and anything they print lands on the same stdout the command writes
// to. That is not hypothetical: on an image whose /bin/sh is dash,
// /etc/profile.d/nvm.sh prints "nvm" when a non-bash shell sources it, so
// `printf success` returned "nvm\nsuccess".
//
// stdout is hashed into the proof record as stdout_digest and excerpted as
// stdout_excerpt, so startup noise makes the same command digest differently
// on different machines — it quietly breaks the reproducibility the record
// exists to provide. Dropping -l is what keeps it out.
//
// The test plants a noisy ~/.profile, which a login shell would source and a
// non-login shell ignores. It cannot write to /etc/profile.d, but the sourcing
// step it exercises is the same one.
func TestProcessRunnerKeepsStdoutFreeOfShellStartupOutput(t *testing.T) {
	// t.Setenv forbids t.Parallel.

	home := t.TempDir()
	profile := "printf 'startup-noise\\n'\n"
	for _, name := range []string{".profile", ".bash_profile", ".bashrc"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(profile), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Both are consulted for the user's home depending on shell and platform.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	output, err := (processCommandRunner{}).Run(context.Background(), t.TempDir(), "printf success", commandTestBudget)
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if output.Stdout != "success" {
		t.Fatalf("stdout = %q, want %q — shell startup output leaked into the verified command's stdout", output.Stdout, "success")
	}
	if strings.Contains(output.Stderr, "startup-noise") {
		t.Fatalf("stderr = %q, want no startup output", output.Stderr)
	}
}
