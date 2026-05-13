package skillhub

import (
	"fmt"
	"strings"
	"time"
)

// AttributionFilename is the filename written alongside imported skills.
const AttributionFilename = "ATTRIBUTION.md"

// AttributionInput collects everything BuildAttribution needs.
type AttributionInput struct {
	SourceID       string    // e.g. "openclaw"
	OriginalName   string    // skill name as published by the source
	OriginalURL    string    // permalink (with commit SHA if available)
	CommitSHA      string    // optional
	OriginalAuthor string    // resolved author (frontmatter or hub default)
	LicenseLabel   string    // "MIT", "Apache-2.0", ...
	LicenseBody    []byte    // raw license text from the source repo
	NoticeBody     []byte    // optional NOTICE file body (Apache-2.0 §4)
	ImportedAt     time.Time // when the import was performed
}

// LicenseLabels recognised by BuildAttribution.
const (
	LicenseMIT         = "MIT"
	LicenseApache2     = "Apache-2.0"
	LicenseProprietary = "Proprietary"
	LicenseUnknown     = "Unknown"
)

// BuildAttribution renders the ATTRIBUTION.md body. It returns an error for
// licenses we refuse to materialize (Proprietary, Unknown, empty body).
func BuildAttribution(in AttributionInput) ([]byte, error) {
	switch strings.TrimSpace(in.LicenseLabel) {
	case LicenseProprietary:
		return nil, fmt.Errorf("attribution: refusing to materialize a proprietary skill (%s/%s)", in.SourceID, in.OriginalName)
	case "", LicenseUnknown:
		return nil, fmt.Errorf("attribution: license for %s/%s is unknown; refusing to import", in.SourceID, in.OriginalName)
	}
	if len(in.LicenseBody) == 0 {
		return nil, fmt.Errorf("attribution: license body for %s/%s is empty", in.SourceID, in.OriginalName)
	}

	when := in.ImportedAt
	if when.IsZero() {
		when = time.Now().UTC()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Attribution\n\nThis skill was imported from **%s** into TARS.\n\n", in.SourceID)
	fmt.Fprintf(&b, "- **Original name**: %s\n", in.OriginalName)
	if u := strings.TrimSpace(in.OriginalURL); u != "" {
		fmt.Fprintf(&b, "- **Original source**: %s\n", u)
	}
	if sha := strings.TrimSpace(in.CommitSHA); sha != "" {
		fmt.Fprintf(&b, "- **Commit**: %s\n", sha)
	}
	fmt.Fprintf(&b, "- **Imported at**: %s\n", when.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- **License**: %s\n", in.LicenseLabel)
	if a := strings.TrimSpace(in.OriginalAuthor); a != "" {
		fmt.Fprintf(&b, "- **Original author**: %s\n", a)
	}
	b.WriteString("\n")

	if in.LicenseLabel == LicenseApache2 {
		b.WriteString("## NOTICE\n\n")
		if len(in.NoticeBody) > 0 {
			b.Write(normalizeTrailingNewline(in.NoticeBody))
		} else {
			b.WriteString("No NOTICE file provided in source.\n")
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "## %s\n\n", licenseSectionTitle(in.LicenseLabel))
	b.Write(normalizeTrailingNewline(in.LicenseBody))
	b.WriteString("\n---\n")
	fmt.Fprintf(&b, "_Do not delete this file. It satisfies the %s attribution requirement._\n", in.LicenseLabel)

	return []byte(b.String()), nil
}

func licenseSectionTitle(label string) string {
	switch label {
	case LicenseMIT:
		return "MIT License"
	case LicenseApache2:
		return "Apache License 2.0"
	default:
		return label
	}
}

func normalizeTrailingNewline(body []byte) []byte {
	if len(body) == 0 {
		return []byte{'\n'}
	}
	if body[len(body)-1] == '\n' {
		return body
	}
	out := make([]byte, len(body)+1)
	copy(out, body)
	out[len(body)] = '\n'
	return out
}

// DetectLicenseLabel returns one of LicenseMIT / LicenseApache2 /
// LicenseProprietary / LicenseUnknown based on simple textual markers in a
// license body. Used by adapters that don't get SPDX metadata from their
// upstream.
func DetectLicenseLabel(body []byte) string {
	text := strings.ToLower(string(body))
	if strings.Contains(text, "all rights reserved") || strings.Contains(text, "proprietary") {
		return LicenseProprietary
	}
	if strings.Contains(text, "apache license") && strings.Contains(text, "version 2.0") {
		return LicenseApache2
	}
	if strings.Contains(text, "mit license") {
		return LicenseMIT
	}
	// "Permission is hereby granted, free of charge..." also appears in MIT.
	if strings.Contains(text, "permission is hereby granted, free of charge") &&
		strings.Contains(text, "without restriction") {
		return LicenseMIT
	}
	return LicenseUnknown
}
