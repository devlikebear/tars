package textutil

import "strings"

func ValueOrDash(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "-"
	}
	return value
}
