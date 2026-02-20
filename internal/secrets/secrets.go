package secrets

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

var (
	sensitiveKeywords = map[string]struct{}{
		"KEY":      {},
		"TOKEN":    {},
		"SECRET":   {},
		"PRIVATE":  {},
		"PASSWORD": {},
		"PASSWD":   {},
	}

	jsonSecretPattern = regexp.MustCompile(`(?i)"([a-z0-9_]*(?:key|token|secret|private|password|passwd)[a-z0-9_]*)"\s*:\s*"[^"]*"`)
	bareSecretPattern = regexp.MustCompile(`(?i)\b([a-z0-9_]*(?:key|token|secret|private|password|passwd)[a-z0-9_]*)\b\s*([:=])\s*[^,\}\n]+`)
	authBearerPattern = regexp.MustCompile(`(?i)\bauthorization\b\s*([:=])\s*bearer\s+[a-z0-9._\-+=]+`)
	bearerPattern     = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._\-+=]+`)
	cliFlagPattern    = regexp.MustCompile(`(?i)(--(?:api[-_]?key|token|secret|password|passwd)\s+)(?:"[^"]*"|'[^']*'|[^\s]+)`)
	pemBlockPattern   = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
)

// Registry stores known secret values and redaction rules.
type Registry struct {
	mu          sync.RWMutex
	values      map[string]struct{}
	sorted      []string
	sortedDirty bool
}

var globalRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		values:      map[string]struct{}{},
		sorted:      []string{},
		sortedDirty: false,
	}
}

func ResetForTests() {
	globalRegistry = NewRegistry()
}

func LooksSensitiveKey(name string) bool {
	key := strings.ToUpper(strings.TrimSpace(name))
	if key == "" {
		return false
	}
	for keyword := range sensitiveKeywords {
		if strings.HasPrefix(key, keyword) || strings.HasSuffix(key, keyword) {
			return true
		}
		if strings.HasPrefix(key, keyword+"_") || strings.HasSuffix(key, "_"+keyword) {
			return true
		}
	}
	for _, token := range splitKeyTokens(key) {
		if _, ok := sensitiveKeywords[token]; ok {
			return true
		}
	}
	return false
}

func RegisterNamed(name, value string) {
	globalRegistry.RegisterNamed(name, value)
}

func RegisterForced(name, value string) {
	globalRegistry.RegisterForced(name, value)
}

func RegisterMapNamed(values map[string]string) {
	globalRegistry.RegisterMapNamed(values)
}

func RegisterMapForced(values map[string]string) {
	globalRegistry.RegisterMapForced(values)
}

func RegisterOSEnv() {
	for _, line := range os.Environ() {
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		RegisterNamed(line[:idx], line[idx+1:])
	}
}

func RedactText(text string) string {
	return globalRegistry.RedactText(text)
}

func RedactPreview(text string, maxLen int) string {
	return globalRegistry.RedactPreview(text, maxLen)
}

func (r *Registry) RegisterMapNamed(values map[string]string) {
	for k, v := range values {
		r.RegisterNamed(k, v)
	}
}

func (r *Registry) RegisterMapForced(values map[string]string) {
	for k, v := range values {
		r.RegisterForced(k, v)
	}
}

func (r *Registry) RegisterNamed(name, value string) {
	if !LooksSensitiveKey(name) {
		return
	}
	r.registerValue(value)
}

func (r *Registry) RegisterForced(_ string, value string) {
	r.registerValue(value)
}

func (r *Registry) registerValue(value string) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 4 || trimmed == "***" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.values[trimmed]; ok {
		return
	}
	r.values[trimmed] = struct{}{}
	r.sortedDirty = true
}

func (r *Registry) RedactPreview(text string, maxLen int) string {
	trimmed := strings.TrimSpace(r.RedactText(text))
	if trimmed == "" {
		return ""
	}
	normalized := strings.Join(strings.Fields(trimmed), " ")
	runes := []rune(normalized)
	if maxLen <= 0 || len(runes) <= maxLen {
		return normalized
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func (r *Registry) RedactText(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	redacted := jsonSecretPattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := jsonSecretPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return `"` + sub[1] + `":"***"`
	})
	redacted = bareSecretPattern.ReplaceAllString(redacted, "${1}${2}***")
	redacted = authBearerPattern.ReplaceAllString(redacted, "authorization${1}***")
	redacted = bearerPattern.ReplaceAllString(redacted, "Bearer ***")
	redacted = cliFlagPattern.ReplaceAllString(redacted, "${1}***")
	redacted = pemBlockPattern.ReplaceAllString(redacted, "-----BEGIN PRIVATE KEY-----\n…redacted…\n-----END PRIVATE KEY-----")

	for _, value := range r.sortedValues() {
		redacted = strings.ReplaceAll(redacted, value, maskValue(value))
	}
	return redacted
}

func (r *Registry) sortedValues() []string {
	r.mu.RLock()
	needsSort := r.sortedDirty
	r.mu.RUnlock()
	if needsSort {
		r.mu.Lock()
		if r.sortedDirty {
			values := make([]string, 0, len(r.values))
			for value := range r.values {
				values = append(values, value)
			}
			sort.Slice(values, func(i, j int) bool {
				return len(values[i]) > len(values[j])
			})
			r.sorted = values
			r.sortedDirty = false
		}
		r.mu.Unlock()
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.sorted...)
}

func maskValue(value string) string {
	if len(value) < 12 {
		return "***"
	}
	return value[:4] + "…" + value[len(value)-3:]
}

func splitKeyTokens(key string) []string {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	return strings.FieldsFunc(key, func(r rune) bool {
		return !(unicode.IsDigit(r) || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	})
}
