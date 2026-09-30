//go:build !darwin && !linux

package sessionworktree

// cloneFile has no copy-on-write clone to use here; files are copied.
func cloneFile(src, dst string) error {
	return errCloneUnsupported
}
