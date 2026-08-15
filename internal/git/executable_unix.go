//go:build !windows

package git

// defaultGitPath is where git lives on a stock macOS or Linux install. Trying
// it before PATH keeps the original hardcoded-path behaviour on the platforms
// that had it, so no lookup order changes there.
const defaultGitPath = "/usr/bin/git"
