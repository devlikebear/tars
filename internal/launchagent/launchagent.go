package launchagent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultServerLabel    = "io.tars.server"
	DefaultAssistantLabel = "io.tars.assistant"
	ServiceLabelEnv       = "TARS_LAUNCHD_LABEL"
	ServiceDomainEnv      = "TARS_LAUNCHD_DOMAIN"
)

type Config struct {
	Label            string
	DefaultLabel     string
	ProgramArguments []string
	WorkingDirectory string
	StdoutPath       string
	StderrPath       string
	KeepAlive        bool
	RunAtLoad        bool
	Environment      map[string]string
}

func BuildPlist(cfg Config) string {
	label := effectiveLabel(cfg.Label, cfg.DefaultLabel)
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n")
	b.WriteString("<dict>\n")
	b.WriteString("  <key>Label</key>\n")
	_, _ = fmt.Fprintf(&b, "  <string>%s</string>\n", xmlEscape(label))
	if len(cfg.ProgramArguments) > 0 {
		b.WriteString("  <key>ProgramArguments</key>\n")
		b.WriteString("  <array>\n")
		for _, arg := range cfg.ProgramArguments {
			_, _ = fmt.Fprintf(&b, "    <string>%s</string>\n", xmlEscape(arg))
		}
		b.WriteString("  </array>\n")
	}
	if v := strings.TrimSpace(cfg.WorkingDirectory); v != "" {
		b.WriteString("  <key>WorkingDirectory</key>\n")
		_, _ = fmt.Fprintf(&b, "  <string>%s</string>\n", xmlEscape(v))
	}
	if v := strings.TrimSpace(cfg.StdoutPath); v != "" {
		b.WriteString("  <key>StandardOutPath</key>\n")
		_, _ = fmt.Fprintf(&b, "  <string>%s</string>\n", xmlEscape(v))
	}
	if v := strings.TrimSpace(cfg.StderrPath); v != "" {
		b.WriteString("  <key>StandardErrorPath</key>\n")
		_, _ = fmt.Fprintf(&b, "  <string>%s</string>\n", xmlEscape(v))
	}
	writeEnvironment(&b, cfg.Environment)
	b.WriteString("  <key>RunAtLoad</key>\n")
	if cfg.RunAtLoad {
		b.WriteString("  <true/>\n")
	} else {
		b.WriteString("  <false/>\n")
	}
	b.WriteString("  <key>KeepAlive</key>\n")
	if cfg.KeepAlive {
		b.WriteString("  <true/>\n")
	} else {
		b.WriteString("  <false/>\n")
	}
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")
	return b.String()
}

func PathForHome(home string, label string, defaultLabel string) string {
	return filepath.Join(strings.TrimSpace(home), "Library", "LaunchAgents", effectiveLabel(label, defaultLabel)+".plist")
}

func DefaultPath(label string, defaultLabel string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return PathForHome(home, label, defaultLabel), nil
}

func Install(path string, content string) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return fmt.Errorf("launchagent path is required")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

func ResolveServiceIdentity(defaultLabel string, defaultDomain string) (string, string) {
	label := strings.TrimSpace(os.Getenv(ServiceLabelEnv))
	if label == "" {
		label = strings.TrimSpace(defaultLabel)
	}
	domain := strings.TrimSpace(os.Getenv(ServiceDomainEnv))
	if domain == "" {
		domain = strings.TrimSpace(defaultDomain)
	}
	return label, domain
}

func writeEnvironment(b *strings.Builder, env map[string]string) {
	keys := make([]string, 0, len(env))
	for key, value := range env {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return
	}
	sort.Strings(keys)
	b.WriteString("  <key>EnvironmentVariables</key>\n")
	b.WriteString("  <dict>\n")
	for _, key := range keys {
		b.WriteString("    <key>")
		b.WriteString(xmlEscape(key))
		b.WriteString("</key>\n")
		_, _ = fmt.Fprintf(b, "    <string>%s</string>\n", xmlEscape(env[key]))
	}
	b.WriteString("  </dict>\n")
}

func effectiveLabel(label string, defaultLabel string) string {
	if trimmed := strings.TrimSpace(label); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(defaultLabel); trimmed != "" {
		return trimmed
	}
	return DefaultServerLabel
}

func xmlEscape(v string) string {
	out := strings.TrimSpace(v)
	out = strings.ReplaceAll(out, "&", "&amp;")
	out = strings.ReplaceAll(out, "<", "&lt;")
	out = strings.ReplaceAll(out, ">", "&gt;")
	out = strings.ReplaceAll(out, `"`, "&quot;")
	out = strings.ReplaceAll(out, "'", "&apos;")
	return out
}
