package pulse

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/devlikebear/tars/internal/pulse/autofix"
)

// DecideByRules classifies a tick without a model. Every signal already
// carries the facts a decision needs — its severity against the configured
// thresholds, the autofix that would resolve it, and whether that autofix
// may run — so the classification is a function of them:
//
//  1. a signal naming an allowed autofix that is ready to run → autofix
//  2. disk usage at warn with log compression allowed → autofix
//  3. any signal at or above the minimum severity → notify
//  4. otherwise → ignore
func DecideByRules(signals []Signal, policy DeciderPolicy) Decision {
	for _, s := range signals {
		name, _ := s.Details["autofix_candidate"].(string)
		if name == "" || !slices.Contains(policy.AllowedAutofixes, name) {
			continue
		}
		if detailTrue(s, "has_auto_resume_candidate") || detailTrue(s, "has_auto_continue_candidate") {
			return Decision{Action: ActionAutofix, Severity: s.Severity, Title: s.Summary, Summary: s.Summary, AutofixName: name}
		}
	}
	for _, s := range signals {
		if s.Kind == SignalKindDiskUsage && s.Severity <= SeverityWarn && slices.Contains(policy.AllowedAutofixes, autofix.CompressOldLogsName) {
			return Decision{Action: ActionAutofix, Severity: s.Severity, Title: s.Summary, Summary: s.Summary, AutofixName: autofix.CompressOldLogsName}
		}
	}
	var worst *Signal
	var summaries []string
	for i := range signals {
		s := &signals[i]
		if !s.Severity.AtLeast(policy.MinSeverity) {
			continue
		}
		summaries = append(summaries, s.Summary)
		if worst == nil || s.Severity > worst.Severity {
			worst = s
		}
	}
	if worst == nil {
		return Decision{Action: ActionIgnore, Severity: SeverityInfo}
	}
	return Decision{Action: ActionNotify, Severity: worst.Severity, Title: worst.Summary, Summary: strings.Join(summaries, "; ")}
}

func detailTrue(s Signal, key string) bool {
	v, _ := s.Details[key].(bool)
	return v
}

// Fingerprint identifies what a set of signals is about, so a tick that
// observes the same situation as the last one can be told apart from a new
// one. Numbers are left out: ages, counts and percentages drift every
// minute while the situation stays the same. What identifies it — kinds,
// severities, session and job ids, timestamps of the last activity, the
// flags that say whether an autofix may run — is text or boolean.
func Fingerprint(signals []Signal) string {
	h := sha256.New()
	for _, s := range signals {
		_, _ = fmt.Fprintf(h, "%s|%s|", s.Kind, s.Severity)
		if raw, err := json.Marshal(s.Details); err == nil {
			var details any
			if json.Unmarshal(raw, &details) == nil {
				if stable, err := json.Marshal(withoutNumbers(details)); err == nil {
					_, _ = h.Write(stable)
				}
			}
		}
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func withoutNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			if _, number := child.(float64); number {
				continue
			}
			out[k] = withoutNumbers(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, child := range t {
			if _, number := child.(float64); number {
				continue
			}
			out = append(out, withoutNumbers(child))
		}
		return out
	default:
		return v
	}
}
