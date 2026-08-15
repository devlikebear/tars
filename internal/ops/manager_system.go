//go:build !windows

package ops

import (
	"bufio"
	"os/exec"
	"strings"
)

// processCount counts system-wide processes by parsing `ps`.
func processCount() (int, error) {
	out, err := exec.Command("/bin/ps", "-A", "-o", "pid=").Output()
	if err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}
