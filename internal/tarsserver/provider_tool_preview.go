package tarsserver

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/secrets"
)

// providerToolLabelKeys are the arguments a tool card is labelled by —
// what command ran, which file, what was searched — in the order they go
// first in the preview. claude-code-cli encodes tool input with sorted
// keys, so without this a Write's file_path comes after its whole content
// and is the first thing cut.
var providerToolLabelKeys = []string{
	"file_path", "notebook_path", "command", "pattern", "path", "glob",
	"url", "query", "description", "subagent_type",
}

var providerToolSecretKey = regexp.MustCompile(`(?i)key|token|secret|private|password|passwd`)

type previewEntry struct {
	key   string
	label bool
	// str is set for string values; raw holds any other value, compacted.
	str   *string
	raw   string
	runes int
}

// providerToolArgsPreview shortens the arguments of a tool a CLI provider
// ran into a JSON object of at most maxLen runes. Cutting the JSON text,
// as statusPreview does, leaves an unparseable fragment the console can
// only show escaped; this shortens the values instead, label arguments
// last, so the preview always parses and still names the command or file.
// Arguments that are not a JSON object get the plain preview.
func providerToolArgsPreview(toolName, args string, maxLen int) string {
	entries, ok := parsePreviewEntries(args)
	if !ok || maxLen <= 0 {
		return statusPreviewForTool(toolName, args, maxLen)
	}
	const unlimited = 1 << 30
	fits := func(labelCap, otherCap, keep int) (string, bool) {
		out := renderPreviewEntries(entries, labelCap, otherCap, keep)
		return out, utf8.RuneCountInString(out) <= maxLen
	}
	if out, ok := fits(unlimited, unlimited, len(entries)); ok {
		return out
	}
	// Largest cap in [1, maxLen] for which render fits, or 0 if none.
	search := func(render func(int) (string, bool)) (string, bool) {
		lo, hi, best, found := 1, maxLen, "", false
		for lo <= hi {
			mid := (lo + hi) / 2
			if out, ok := render(mid); ok {
				best, found, lo = out, true, mid+1
			} else {
				hi = mid - 1
			}
		}
		return best, found
	}
	// Shorten the other arguments first, dropping them from the end when
	// even elided values do not fit; only then shorten the label arguments
	// (sorted first, so they are the last to go).
	labels := 0
	for _, e := range entries {
		if e.label {
			labels++
		}
	}
	for keep := len(entries); keep >= max(labels, 1); keep-- {
		if out, ok := search(func(c int) (string, bool) { return fits(unlimited, c, keep) }); ok {
			return out
		}
	}
	for keep := max(labels, 1); keep >= 1; keep-- {
		if out, ok := search(func(c int) (string, bool) { return fits(c, 1, keep) }); ok {
			return out
		}
	}
	return statusPreviewForTool(toolName, args, maxLen)
}

// parsePreviewEntries reads a JSON object's members in order, label keys
// first, with secrets redacted value by value (redacting the JSON text as
// a whole can eat a closing quote and break it).
func parsePreviewEntries(args string) ([]previewEntry, bool) {
	trimmed := strings.TrimSpace(args)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	var labels, others []previewEntry
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, _ := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, false
		}
		entry := previewEntry{key: key, label: isProviderToolLabelKey(key)}
		var s string
		switch {
		case json.Unmarshal(raw, &s) == nil:
			if providerToolSecretKey.MatchString(key) {
				s = "***"
			} else {
				s = secrets.RedactText(s)
			}
			entry.str = &s
			entry.runes = utf8.RuneCountInString(s)
		default:
			var compact bytes.Buffer
			if err := json.Compact(&compact, raw); err != nil {
				return nil, false
			}
			redacted := secrets.RedactText(compact.String())
			if json.Valid([]byte(redacted)) {
				entry.raw = redacted
			} else {
				entry.str = &redacted
			}
			entry.runes = utf8.RuneCountInString(redacted)
		}
		if entry.label {
			labels = append(labels, entry)
		} else {
			others = append(others, entry)
		}
	}
	if _, err := dec.Token(); err != nil {
		return nil, false
	}
	// Label keys in their own order, the rest as they came.
	ordered := make([]previewEntry, 0, len(labels)+len(others))
	for _, key := range providerToolLabelKeys {
		for _, e := range labels {
			if e.key == key {
				ordered = append(ordered, e)
			}
		}
	}
	return append(ordered, others...), true
}

func isProviderToolLabelKey(key string) bool {
	return slices.Contains(providerToolLabelKeys, key)
}

func renderPreviewEntries(entries []previewEntry, labelCap, otherCap, keep int) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, e := range entries[:keep] {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(marshalPreviewString(e.key))
		b.WriteByte(':')
		limit := otherCap
		if e.label {
			limit = labelCap
		}
		switch {
		case e.str != nil:
			b.WriteString(marshalPreviewString(shortenRunes(*e.str, limit)))
		case e.runes <= limit:
			b.WriteString(e.raw)
		default:
			b.WriteString(marshalPreviewString(shortenRunes(e.raw, limit)))
		}
	}
	b.WriteByte('}')
	return b.String()
}

func shortenRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	if limit <= 1 {
		return "…"
	}
	runes := []rune(s)
	return string(runes[:limit-1]) + "…"
}

// marshalPreviewString encodes s as a JSON string without HTML escaping,
// so `<` stays `<` in the card.
func marshalPreviewString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}
