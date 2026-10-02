// Package testutil holds helpers shared by tests across packages.
package testutil

import "testing"

// SetHome points the user's home directory at dir for the rest of the test.
//
// Setting HOME alone is not enough: on Windows os.UserHomeDir reads
// USERPROFILE, so a HOME-only test resolves to the real home and reads or
// overwrites the user's own files. HOMEDRIVE and HOMEPATH are emptied too,
// since some tools fall back to them when USERPROFILE is unset.
func SetHome(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
}
