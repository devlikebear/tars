package pulse

import "testing"

func TestSeverityString(t *testing.T) {
	cases := []struct {
		s    Severity
		want string
	}{
		{SeverityInfo, "info"},
		{SeverityWarn, "warn"},
		{SeverityError, "error"},
		{SeverityCritical, "critical"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("Severity(%d).String() = %q, want %q", int(c.s), got, c.want)
		}
	}
}

func TestParseSeverity(t *testing.T) {
	cases := []struct {
		in      string
		want    Severity
		wantErr bool
	}{
		{"info", SeverityInfo, false},
		{"warn", SeverityWarn, false},
		{"warning", SeverityWarn, false},
		{"error", SeverityError, false},
		{"critical", SeverityCritical, false},
		{"crit", SeverityCritical, false},
		{"bogus", SeverityInfo, true},
		{"", SeverityInfo, true},
	}
	for _, c := range cases {
		got, err := ParseSeverity(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseSeverity(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("ParseSeverity(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSeverityAtLeast(t *testing.T) {
	if !SeverityError.AtLeast(SeverityWarn) {
		t.Error("error should be >= warn")
	}
	if SeverityInfo.AtLeast(SeverityWarn) {
		t.Error("info should NOT be >= warn")
	}
	if !SeverityCritical.AtLeast(SeverityCritical) {
		t.Error("critical should be >= critical")
	}
}

func TestParseAction(t *testing.T) {
	cases := []struct {
		in      string
		want    Action
		wantErr bool
	}{
		{"ignore", ActionIgnore, false},
		{"notify", ActionNotify, false},
		{"autofix", ActionAutofix, false},
		{"IGNORE", "", true}, // case sensitive
		{"", "", true},
		{"run", "", true},
	}
	for _, c := range cases {
		got, err := ParseAction(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseAction(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("ParseAction(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
