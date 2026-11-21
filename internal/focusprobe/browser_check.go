package focusprobe

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

// AgentBrowserPathEnv names the agent-browser binary to use instead of the
// one the lookup finds, like TARS_FOCUS_GH_PATH does for gh.
const AgentBrowserPathEnv = "TARS_FOCUS_AGENT_BROWSER_PATH"

// repoLocalAgentBrowser is where `npm install agent-browser` in the working
// folder puts the CLI, as a shell there names it.
const repoLocalAgentBrowser = "./node_modules/.bin/" + focuspipeline.BrowserCheckCLIName

// FindAgentBrowser finds the scripted-browser CLI a focus pipeline working
// in dir can use for checks in verify (focuspipeline's browser_check.go).
// invocation is how a shell in dir runs it — the bare name when it is on
// PATH, a path relative to dir for the repository's own copy, an absolute
// path otherwise — and path is the file. Both are empty when there is none,
// which is not an error: nothing needs the CLI, a pipeline is only not told
// about it.
//
// It is looked up on every call, in this order: TARS_FOCUS_AGENT_BROWSER_PATH,
// PATH, dir's node_modules/.bin, then the places a server started by launchd
// cannot see by name (~/.local/bin, /opt/homebrew/bin, /usr/local/bin).
func FindAgentBrowser(dir string) (invocation, path string) {
	name := focuspipeline.BrowserCheckCLIName
	if p := strings.TrimSpace(os.Getenv(AgentBrowserPathEnv)); p != "" {
		// An explicit path that is wrong is not replaced by a guess.
		return absoluteInvocation(p)
	}
	if p, err := exec.LookPath(name); err == nil && filepath.IsAbs(p) {
		return name, p
	}
	if dir = strings.TrimSpace(dir); dir != "" {
		// The folder comes from a request: only a regular file that is
		// still inside it once symlinks are resolved counts.
		if p, ok := focusInsideRepo(dir, filepath.Join(dir, filepath.FromSlash(repoLocalAgentBrowser))); ok {
			return repoLocalAgentBrowser, p
		}
	}
	for _, dir := range agentBrowserKnownDirs() {
		if inv, path := absoluteInvocation(filepath.Join(dir, name)); inv != "" {
			return inv, path
		}
	}
	return "", ""
}

// agentBrowserKnownDirs lists the install folders a server started by
// launchd does not have on PATH. A variable so tests do not read the host's.
var agentBrowserKnownDirs = func() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return append(dirs, "/opt/homebrew/bin", "/usr/local/bin")
}

// BrowserCheckCLI is FindAgentBrowser's invocation: what a new pipeline
// records as Pipeline.BrowserCheck.
func BrowserCheckCLI(dir string) string {
	invocation, _ := FindAgentBrowser(dir)
	return invocation
}

// absoluteInvocation accepts p when it is an existing file at an absolute
// path that can be written between single quotes in a shell command — the
// plan guidance shows it that way.
func absoluteInvocation(p string) (invocation, path string) {
	if !filepath.IsAbs(p) || strings.ContainsAny(p, "'\n") || !isRegularFile(p) {
		return "", ""
	}
	return p, p
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
