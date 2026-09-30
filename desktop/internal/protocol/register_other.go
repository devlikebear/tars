//go:build !linux && !windows

package protocol

// Register does nothing: the macOS bundle declares the scheme in Info.plist.
func Register(string) (bool, error) { return false, nil }
