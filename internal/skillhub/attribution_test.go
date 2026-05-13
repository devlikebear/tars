package skillhub

import (
	"strings"
	"testing"
	"time"
)

const mitBody = `MIT License

Copyright (c) 2025 Peter Steinberger

Permission is hereby granted, free of charge, to any person obtaining a copy
...`

const apacheBody = `                                 Apache License
                           Version 2.0, January 2004
...`

const proprietaryBody = `© 2025 Anthropic, PBC. All rights reserved.
LICENSE: Use of these materials...`

func TestBuildAttribution_MIT(t *testing.T) {
	body, err := BuildAttribution(AttributionInput{
		SourceID:       "openclaw",
		OriginalName:   "github",
		OriginalURL:    "https://github.com/steipete/openclaw/blob/abc/skills/github/SKILL.md",
		CommitSHA:      "abc1234",
		OriginalAuthor: "Peter Steinberger",
		LicenseLabel:   LicenseMIT,
		LicenseBody:    []byte(mitBody),
		ImportedAt:     time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildAttribution: %v", err)
	}
	s := string(body)
	for _, want := range []string{
		"# Attribution",
		"openclaw",
		"github",
		"abc1234",
		"MIT License",
		"Copyright (c) 2025 Peter Steinberger",
		"2026-05-13T12:00:00Z",
		"Do not delete this file",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("attribution missing %q\n---\n%s", want, s)
		}
	}
}

func TestBuildAttribution_Apache2_WithoutNotice(t *testing.T) {
	body, err := BuildAttribution(AttributionInput{
		SourceID:     "anthropic",
		OriginalName: "skill-creator",
		LicenseLabel: LicenseApache2,
		LicenseBody:  []byte(apacheBody),
	})
	if err != nil {
		t.Fatalf("BuildAttribution: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "## NOTICE") {
		t.Errorf("Apache-2.0 attribution should include NOTICE section")
	}
	if !strings.Contains(s, "No NOTICE file provided in source.") {
		t.Errorf("missing NOTICE fallback message")
	}
}

func TestBuildAttribution_RejectsProprietary(t *testing.T) {
	_, err := BuildAttribution(AttributionInput{
		SourceID:     "anthropic",
		OriginalName: "docx",
		LicenseLabel: LicenseProprietary,
		LicenseBody:  []byte(proprietaryBody),
	})
	if err == nil {
		t.Fatalf("expected error for proprietary license")
	}
	if !strings.Contains(err.Error(), "proprietary") {
		t.Errorf("error %q does not mention proprietary", err)
	}
}

func TestBuildAttribution_RejectsUnknownLicense(t *testing.T) {
	_, err := BuildAttribution(AttributionInput{
		LicenseLabel: LicenseUnknown,
		LicenseBody:  []byte("some text"),
	})
	if err == nil {
		t.Fatalf("expected error for unknown license")
	}
}

func TestBuildAttribution_RejectsEmptyBody(t *testing.T) {
	_, err := BuildAttribution(AttributionInput{
		LicenseLabel: LicenseMIT,
		LicenseBody:  nil,
	})
	if err == nil {
		t.Fatalf("expected error for empty license body")
	}
}

func TestBuildAttribution_Apache2_WithNotice(t *testing.T) {
	body, err := BuildAttribution(AttributionInput{
		SourceID:     "anthropic",
		OriginalName: "skill-creator",
		LicenseLabel: LicenseApache2,
		LicenseBody:  []byte(apacheBody),
		NoticeBody:   []byte("This product includes software developed by Anthropic, PBC."),
	})
	if err != nil {
		t.Fatalf("BuildAttribution: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "This product includes software developed by Anthropic") {
		t.Errorf("NOTICE body should be embedded verbatim: %s", s)
	}
}

func TestLicenseSectionTitle_Default(t *testing.T) {
	got := licenseSectionTitle("BSD-3-Clause")
	if got != "BSD-3-Clause" {
		t.Errorf("default branch should return label as-is, got %q", got)
	}
}

func TestNormalizeTrailingNewline_Cases(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "\n"},
		{"no-newline", "no-newline\n"},
		{"already\n", "already\n"},
	}
	for _, tt := range cases {
		got := string(normalizeTrailingNewline([]byte(tt.in)))
		if got != tt.want {
			t.Errorf("normalizeTrailingNewline(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDetectLicenseLabel_PermissionMarker(t *testing.T) {
	body := []byte("Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files to deal in the Software without restriction")
	if got := DetectLicenseLabel(body); got != LicenseMIT {
		t.Errorf("expected MIT from permission marker, got %q", got)
	}
}

func TestDetectLicenseLabel(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"mit", mitBody, LicenseMIT},
		{"apache2", apacheBody, LicenseApache2},
		{"proprietary", proprietaryBody, LicenseProprietary},
		{"unknown", "BSD 3-Clause License\nCopyright...", LicenseUnknown},
	}
	for _, tt := range cases {
		got := DetectLicenseLabel([]byte(tt.body))
		if got != tt.want {
			t.Errorf("%s: DetectLicenseLabel = %q, want %q", tt.name, got, tt.want)
		}
	}
}
