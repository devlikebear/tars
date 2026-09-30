// Package protocol registers the shell as the handler for tars:// links.
//
// macOS reads the scheme from the app bundle's Info.plist, so there is
// nothing to do at run time. Windows and Linux have no bundle manifest the
// OS reads, so the shell registers itself for the current user on start:
// the registry under HKCU on Windows, a .desktop file plus xdg-mime on
// Linux. Registering again after the app moves points the link at the new
// location.
package protocol

import (
	"strings"
)

// Scheme is the URL scheme.
const Scheme = "tars"

// DesktopFileName is the Linux desktop entry's file name.
const DesktopFileName = "tars-desktop.desktop"

// DesktopEntry is the Linux desktop entry that opens tars:// links with exe.
func DesktopEntry(exe string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=TARS",
		"Comment=TARS console",
		"Exec=" + quoteExecArg(exe) + " %u",
		"Terminal=false",
		"Categories=Development;Utility;",
		"MimeType=x-scheme-handler/" + Scheme + ";",
		"StartupWMClass=tars-desktop",
		"",
	}, "\n")
}

// quoteExecArg quotes one Exec argument per the Desktop Entry spec: inside
// double quotes, ", `, $, and \ are escaped with a backslash, and the
// backslash itself is escaped once more because the whole value is a
// string that un-escapes \\ first.
func quoteExecArg(arg string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '"', '`', '$':
			b.WriteString(`\\`)
			b.WriteRune(r)
		case '\\':
			b.WriteString(`\\\\`)
		case '%':
			b.WriteString("%%")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// WindowsCommand is the shell\open\command value that opens a link with exe.
func WindowsCommand(exe string) string {
	return `"` + exe + `" "%1"`
}
