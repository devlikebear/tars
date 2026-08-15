//go:build !windows

package shellexec

// defaultShellPath is guaranteed by POSIX, so resolution normally stops here
// and behaves exactly as the previous hardcoded /bin/sh did.
const defaultShellPath = "/bin/sh"

var shellLookupNames = []string{"sh"}

// fallbackShellPaths has nothing to add on unix: if /bin/sh is missing and sh
// is not on PATH, the system has no POSIX shell to find.
func fallbackShellPaths() []string { return nil }
