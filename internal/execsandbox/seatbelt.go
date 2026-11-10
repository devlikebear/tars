package execsandbox

import "strings"

// seatbeltKeychainServices are the Mach services behind the macOS keychain.
// Reading a saved password goes through them, not through a file the Hidden
// list could cover.
var seatbeltKeychainServices = []string{
	"com.apple.SecurityServer",
	"com.apple.securityd.xpc",
	"com.apple.security.keychaind",
}

// seatbeltDeviceWrites are the device nodes an ordinary command writes to.
const seatbeltDeviceWrites = `(literal "/dev/null") (literal "/dev/zero") (literal "/dev/dtracehelper") (regex #"^/dev/tty") (regex #"^/dev/fd/") (literal "/dev/stdout") (literal "/dev/stderr")`

// SeatbeltProfile renders p as a sandbox-exec profile.
//
// The profile starts from (allow default) and takes away what the policy
// names, rather than starting from (deny default): a deny-default profile has
// to list every Mach service, sysctl and IPC a compiler or package manager
// touches, and breaks whenever macOS renames one. What the sandbox is for —
// where a command can write, what it can read, whether it can reach the
// network — is all expressed as denials. Later rules win, so the order below
// is the precedence: writable < read-only < hidden.
func SeatbeltProfile(p Policy) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")

	b.WriteString("(deny file-write*)\n")
	b.WriteString("(allow file-write* ")
	b.WriteString(seatbeltDeviceWrites)
	for _, w := range p.Writable {
		b.WriteString(" (subpath " + seatbeltString(w.Path) + ")")
	}
	b.WriteString(")\n")

	if len(p.ReadOnly) > 0 {
		b.WriteString("(deny file-write*" + seatbeltSubpaths(p.ReadOnly) + ")\n")
	}
	if len(p.Hidden) > 0 {
		subpaths := seatbeltSubpaths(p.Hidden)
		b.WriteString("(deny file-read* file-write*" + subpaths + ")\n")
		// A socket under a hidden path (the ssh agent, gpg-agent) is reached
		// by connect, which file rules do not cover.
		b.WriteString("(deny network-outbound (remote unix-socket" + subpaths + "))\n")
	}

	b.WriteString("(deny mach-lookup")
	for _, name := range seatbeltKeychainServices {
		b.WriteString(" (global-name " + seatbeltString(name) + ")")
	}
	b.WriteString(")\n")

	if !p.Network {
		// Loopback stays open: tests start local servers and dev tools talk
		// to their own daemons.
		b.WriteString("(deny network-outbound (remote ip))\n")
		b.WriteString("(allow network-outbound (remote ip \"localhost:*\"))\n")
	}
	return b.String()
}

func seatbeltSubpaths(paths []Path) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(" (subpath " + seatbeltString(p.Path) + ")")
	}
	return b.String()
}

// seatbeltString quotes s as an SBPL string literal.
func seatbeltString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
