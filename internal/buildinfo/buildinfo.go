package buildinfo

import (
	"fmt"
	"strings"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Summary() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "dev"
	}
	commit := strings.TrimSpace(Commit)
	if commit == "" {
		commit = "unknown"
	}
	date := strings.TrimSpace(Date)
	if date == "" {
		date = "unknown"
	}
	return fmt.Sprintf("tars %s (%s, %s)", version, commit, date)
}
