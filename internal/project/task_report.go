package project

import (
	"bufio"
	"strings"
)

type TaskReport struct {
	Status  string
	Summary string
	Tests   string
	Build   string
	Issue   string
	Branch  string
	PR      string
	Notes   string
}

func ParseTaskReport(raw string) TaskReport {
	report := TaskReport{}
	body := strings.TrimSpace(raw)
	start := strings.Index(body, "<task-report>")
	end := strings.Index(body, "</task-report>")
	if start != -1 && end != -1 && end > start {
		body = body[start+len("<task-report>") : end]
	}

	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		trimmedKey := strings.ToLower(strings.TrimSpace(key))
		trimmedValue := strings.TrimSpace(value)
		switch trimmedKey {
		case "status":
			report.Status = strings.ToLower(trimmedValue)
		case "summary":
			report.Summary = trimmedValue
		case "tests":
			report.Tests = trimmedValue
		case "build":
			report.Build = trimmedValue
		case "issue":
			report.Issue = trimmedValue
		case "branch":
			report.Branch = trimmedValue
		case "pr":
			report.PR = trimmedValue
		case "notes":
			report.Notes = trimmedValue
		}
	}
	return report
}
