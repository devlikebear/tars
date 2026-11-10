package tools

import (
	"fmt"

	"github.com/devlikebear/tars/internal/execsandbox"
)

// ExecSandbox runs exec commands inside an OS sandbox: Seatbelt on macOS,
// bubblewrap on Linux. A command may write only to the tool's folders (its
// PathPolicy), temp dirs, the git common dir of a repository it works in, and
// the Go and npm caches; git hooks and config, and the .tars, .claude and
// .mcp.json settings in those folders, stay read-only; credential folders
// (~/.ssh, ~/.aws, …), the ssh agent and the macOS keychain are out of reach.
//
// It fails closed: when the host has no sandbox (Windows, or the program is
// missing) the command is refused, never run unsandboxed.
type ExecSandbox struct {
	// Network allows outbound connections. Loopback is always allowed.
	Network bool
	// WritablePaths are extra writable paths. A leading ~/ is the home dir.
	WritablePaths []string
	// ReadOnlyPaths stay unwritable even inside a writable path.
	ReadOnlyPaths []string
	// DenyReadPaths can be neither read nor written, on top of the default
	// credential folders.
	DenyReadPaths []string
}

// shellSandbox is an ExecSandbox bound to one tool's folders.
type shellSandbox struct {
	config ExecSandbox
	dirs   []string
}

func newShellSandbox(config *ExecSandbox, policy PathPolicy) *shellSandbox {
	if config == nil {
		return nil
	}
	dirs := []string{policy.PrimaryDir}
	dirs = append(dirs, policy.AllowedDirs...)
	return &shellSandbox{config: *config, dirs: dirs}
}

// detectSandbox is swappable in tests.
var detectSandbox = execsandbox.Detect

// wrap returns the program and arguments that run argv in dir inside the
// sandbox, or an error when this host has none.
func (s *shellSandbox) wrap(dir string, argv []string) (string, []string, error) {
	backend, err := detectSandbox()
	if err != nil {
		return "", nil, fmt.Errorf("%v; the command was not run", err)
	}
	policy := execsandbox.Resolve(execsandbox.Options{
		Dirs:     s.dirs,
		Writable: s.config.WritablePaths,
		ReadOnly: s.config.ReadOnlyPaths,
		Hidden:   s.config.DenyReadPaths,
		Network:  s.config.Network,
	})
	name, args := backend.Command(policy, dir, argv)
	return name, args, nil
}

// sandboxDescription is appended to the exec tool's description so the model
// reads a permission error as the sandbox, not as something to work around.
func (s *shellSandbox) description() string {
	if s == nil {
		return ""
	}
	text := " Commands run in an OS sandbox: writing outside the session folders, temp dirs and build caches fails, and credential folders (~/.ssh, ~/.aws, ...) cannot be read"
	if !s.config.Network {
		text += "; network access other than localhost is blocked"
	}
	return text + ". Report such a failure instead of retrying it another way."
}
