package tarsserver

import "testing"

func TestVerifierRefusalQuotesTheVerifiersOwnReason(t *testing.T) {
	for _, tc := range []struct{ summary, rationale, want string }{
		{"verification subject is unavailable", `path "src/App.css": no such file`, `verification subject is unavailable: path "src/App.css": no such file`},
		{" verification subject is unavailable ", "", "verification subject is unavailable"},
		{"", "command could not start", "command could not start"},
		{"", " ", "verification provenance was unavailable"},
	} {
		if got := verifierRefusal(tc.summary, tc.rationale); got != tc.want {
			t.Errorf("verifierRefusal(%q, %q) = %q, want %q", tc.summary, tc.rationale, got, tc.want)
		}
	}
}
