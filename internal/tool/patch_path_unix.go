//go:build !windows

package tool

// defaultPatchPath keeps the previous hardcoded location, so resolution
// normally stops here and unix behaviour is unchanged.
const defaultPatchPath = "/usr/bin/patch"

// bundledPatchPaths has nothing to add on unix: patch is a system utility, not
// something shipped inside another tool's installation.
func bundledPatchPaths() []string { return nil }
