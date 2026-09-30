// Package deeplink parses tars:// links.
//
// Any web page or document can open a tars:// link, so a link only ever
// navigates the console or proposes a new chat that the person confirms.
// It never answers an approval or sends a message.
package deeplink

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Scheme is the URL scheme the shell registers.
const Scheme = "tars"

// Kind is what a link asks for.
type Kind int

const (
	// Open shows the console on Path.
	Open Kind = iota
	// NewChat proposes a chat working in Dir; the shell asks first.
	NewChat
	// OpenWindow shows chat SessionID in a window of its own.
	OpenWindow
)

// Link is a parsed tars:// URL.
type Link struct {
	Kind Kind
	// Path is the console path for Open.
	Path string
	// Dir is the absolute folder for NewChat.
	Dir string
	// SessionID is the chat for OpenWindow.
	SessionID string
}

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// Parse reads one tars:// URL. Accepted forms:
//
//	tars://                     the console
//	tars://open                 the console
//	tars://session/<id>         a chat (tars://chat/<id> too)
//	tars://session/<id>?window=new
//	                            a chat in a window of its own
//	tars://new?cwd=<abs path>   a new chat in a folder
func Parse(raw string) (Link, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Link{}, fmt.Errorf("deep link: %w", err)
	}
	if !strings.EqualFold(u.Scheme, Scheme) {
		return Link{}, fmt.Errorf("deep link %q: not a %s:// link", raw, Scheme)
	}
	// tars://session/abc parses as host "session", path "/abc"; tars:session/abc
	// as opaque "session/abc". Read both as one slash-separated route.
	route := u.Opaque
	if route == "" {
		route = u.Host + u.Path
	}
	parts := strings.FieldsFunc(route, func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return Link{Kind: Open, Path: "/console"}, nil
	}
	switch strings.ToLower(parts[0]) {
	case "open":
		if len(parts) == 1 {
			return Link{Kind: Open, Path: "/console"}, nil
		}
	case "session", "chat":
		return parseSession(raw, parts, u.Query().Get("window"))
	case "new":
		if len(parts) == 1 {
			return parseNewChat(u.Query().Get("cwd"))
		}
	}
	return Link{}, fmt.Errorf("deep link %q: unknown route", raw)
}

// parseSession reads tars://session/<id>, optionally ?window=new.
func parseSession(raw string, parts []string, window string) (Link, error) {
	if len(parts) != 2 || !ValidSessionID(parts[1]) {
		return Link{}, fmt.Errorf("deep link %q: bad session id", raw)
	}
	switch window {
	case "":
		return Link{Kind: Open, Path: "/console/chat/" + parts[1]}, nil
	case "new":
		return Link{Kind: OpenWindow, Path: "/console/chat/" + parts[1], SessionID: parts[1]}, nil
	}
	return Link{}, fmt.Errorf("deep link %q: window must be \"new\"", raw)
}

// parseNewChat reads tars://new?cwd=<abs path>.
func parseNewChat(dir string) (Link, error) {
	if dir == "" {
		return Link{}, errors.New("deep link: tars://new needs ?cwd=<folder>")
	}
	if !filepath.IsAbs(dir) {
		return Link{}, fmt.Errorf("deep link: cwd %q is not an absolute path", dir)
	}
	return Link{Kind: NewChat, Dir: filepath.Clean(dir)}, nil
}

// ValidSessionID reports whether id can name a chat in a console path: no
// slashes, dots-only traversal, spaces, or escapes to smuggle in.
func ValidSessionID(id string) bool {
	return sessionIDPattern.MatchString(id) && strings.Trim(id, ".") != ""
}

// FromArgs finds the first tars:// URL among command-line arguments: how
// Windows and Linux hand a link to the app (macOS sends an event instead).
func FromArgs(args []string) (string, bool) {
	for _, arg := range args {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(arg)), Scheme+":") {
			return strings.TrimSpace(arg), true
		}
	}
	return "", false
}
