// Package fileuri converts between local paths and file: URIs.
//
// It exists because url.URL{Scheme: "file", Path: path} is wrong on Windows:
// `C:\dir\artifact.txt` renders as file://C:%5Cdir%5Cartifact.txt, where the
// drive letter becomes the URI host and every separator is percent-encoded.
// Such a URI is not resolvable — the parser here rejects it, and a naive
// consumer that strips the scheme gets a path that cannot be opened.
//
// A file URI needs forward slashes and a leading one ahead of the drive
// letter: file:///C:/dir/artifact.txt. On unix the absolute path already
// starts with a slash, so both forms agree and existing URIs stay valid.
package fileuri

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// New returns the file: URI for path.
//
// It takes no error because it cannot fail for its callers. path is made
// absolute so the URI identifies the same file from any working directory,
// and filepath.Abs only fails when the working directory cannot be
// determined — unreachable here, since every caller already holds an absolute
// path. Returning an error anyway would put a branch in four callers that no
// test could ever reach.
func New(path string) string {
	resolved := strings.TrimSpace(path)
	if absolute, err := filepath.Abs(resolved); err == nil {
		resolved = absolute
	}
	uriPath := filepath.ToSlash(resolved)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	return (&url.URL{Scheme: "file", Path: uriPath}).String()
}

// Path returns the local path a file: URI refers to.
//
// A URI with a host is rejected rather than guessed at: it is either a remote
// UNC reference this code does not handle, or the malformed shape New exists
// to avoid.
func Path(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("fileuri: parse %q: %w", raw, err)
	}
	if parsed.Scheme != "file" {
		return "", fmt.Errorf("fileuri: %q is not a file URI", raw)
	}
	if parsed.Host != "" {
		return "", fmt.Errorf("fileuri: %q has a host", raw)
	}
	uriPath := parsed.Path
	// file:///C:/dir becomes /C:/dir when parsed; the leading slash is part of
	// the URI grammar, not of the Windows path.
	if len(uriPath) > 2 && uriPath[0] == '/' && uriPath[2] == ':' {
		uriPath = uriPath[1:]
	}
	if uriPath == "" {
		return "", fmt.Errorf("fileuri: %q has no path", raw)
	}
	return filepath.FromSlash(uriPath), nil
}
