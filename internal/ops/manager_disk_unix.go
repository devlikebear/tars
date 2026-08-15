//go:build !windows

package ops

import "syscall"

// diskUsage reports the total and user-available bytes of the filesystem
// holding path. Free is the unprivileged-available figure (Bavail), not the
// raw free count, so the used percentage matches what the user can act on.
func diskUsage(path string) (total uint64, free uint64, err error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, 0, err
	}
	return fs.Blocks * uint64(fs.Bsize), fs.Bavail * uint64(fs.Bsize), nil
}
