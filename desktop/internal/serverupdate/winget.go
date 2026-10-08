package serverupdate

import (
	"errors"
	"strings"
)

// WingetPackage is the winget identifier of the tars server.
const WingetPackage = "devlikebear.TARS"

// ErrManagedByWinget is a tars installed by winget. `tars update` refuses it,
// since replacing the executable in place would leave winget's record of the
// installed version stale, so the user upgrades it with winget.
var ErrManagedByWinget = errors.New("tars was installed with winget; run: winget upgrade " + WingetPackage)

// ManagedByWinget reports whether path is inside winget's portable package
// directory or its Links alias folder. Backslashes are folded so the check
// does not depend on the OS it runs on.
func ManagedByWinget(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
	return strings.Contains(p, "/microsoft/winget/packages/") || strings.Contains(p, "/microsoft/winget/links/")
}
