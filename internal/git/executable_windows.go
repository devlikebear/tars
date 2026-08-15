//go:build windows

package git

// Windows has no canonical install location for git — Git for Windows defaults
// to C:\Program Files\Git\cmd\git.exe, scoop/winget/portable installs land
// elsewhere, and none of them are guaranteed. Leave the default empty so
// resolution goes straight to PATH.
const defaultGitPath = ""
