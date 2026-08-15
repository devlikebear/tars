//go:build windows

package ops

import "golang.org/x/sys/windows"

// diskUsage reports the total and user-available bytes of the volume holding
// path. GetDiskFreeSpaceEx is the Windows analogue of statfs; its
// freeBytesAvailableToCaller output honours per-user disk quotas the same way
// statfs' Bavail excludes root-reserved blocks, so both platforms report the
// space this process could actually use.
func diskUsage(path string) (total uint64, free uint64, err error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeToCaller, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeToCaller, &totalBytes, &totalFree); err != nil {
		return 0, 0, err
	}
	return totalBytes, freeToCaller, nil
}
