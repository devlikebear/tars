package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skillhub"
)

func TestPrintHubUpdateResultDistinguishesStatuses(t *testing.T) {
	result := skillhub.UpdateResult{
		Updated: []string{"alpha"},
		Skipped: []skillhub.UpdateDiagnostic{
			{Name: "beta", Reason: "up to date"},
		},
		Failed: []skillhub.UpdateDiagnostic{
			{Name: "gamma", Err: errors.New("simulated update failure")},
		},
	}

	var out bytes.Buffer
	printHubUpdateResult(&out, "skill", result)
	got := out.String()
	for _, want := range []string{
		"Updated: alpha",
		"Skipped: beta (up to date)",
		"Failed: gamma (simulated update failure)",
		"1 skill(s) updated, 1 skipped, 1 failed.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}
