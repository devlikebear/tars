//go:build windows

package ops

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processCount counts system-wide processes via a toolhelp snapshot.
//
// The unix path shells out to `ps`, but the Windows equivalent (`tasklist`) is
// slow to start and prints localized headers that would have to be parsed
// around. The snapshot API is both faster and locale-independent.
//
// Processes the caller cannot open still appear in the snapshot, so the count
// covers the whole system rather than just this user's processes, matching
// `ps -A`.
func processCount() (int, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	var entry windows.ProcessEntry32
	// Size must be filled in before the first call or the walk fails.
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return 0, err
	}
	count := 0
	for {
		count++
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			// ERROR_NO_MORE_FILES is the documented end-of-walk signal.
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return count, nil
			}
			return 0, err
		}
	}
}
