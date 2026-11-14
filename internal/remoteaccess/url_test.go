package remoteaccess

import "testing"

func TestStatusURL(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status Status
		want   string
	}{
		{"default port", Status{TailnetURL: "mac.tail1.ts.net", ServeActive: true, OwnedByTARS: true, ServePort: 443}, "https://mac.tail1.ts.net"},
		{"no port", Status{TailnetURL: "mac.tail1.ts.net.", ServeActive: true, OwnedByTARS: true}, "https://mac.tail1.ts.net"},
		{"other port", Status{TailnetURL: "mac.tail1.ts.net", ServeActive: true, OwnedByTARS: true, ServePort: 8443}, "https://mac.tail1.ts.net:8443"},
		{"serve off", Status{TailnetURL: "mac.tail1.ts.net", OwnedByTARS: true}, ""},
		{"serve for another target", Status{TailnetURL: "mac.tail1.ts.net", ServeActive: true}, ""},
		{"no name", Status{ServeActive: true, OwnedByTARS: true}, ""},
	} {
		if got := tc.status.URL(); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
