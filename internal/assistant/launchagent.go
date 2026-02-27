package assistant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultLaunchAgentLabel = "io.tars.assistant"

type LaunchAgentConfig struct {
	Label            string
	ProgramArguments []string
	WorkingDirectory string
	StdoutPath       string
	StderrPath       string
	KeepAlive        bool
	RunAtLoad        bool
}

func BuildLaunchAgentPlist(cfg LaunchAgentConfig) string {
	label := strings.TrimSpace(cfg.Label)
	if label == "" {
		label = DefaultLaunchAgentLabel
	}
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
			_, _ = fmt.Fprintf(&b, "    <string>%s</string>\n", xmlEscape(strings.TrimSpace(arg)))
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

func DefaultLaunchAgentPath(label string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(label)
	if name == "" {
		name = DefaultLaunchAgentLabel
	}
	return filepath.Join(home, "Library", "LaunchAgents", name+".plist"), nil
}

func InstallLaunchAgent(path string, content string) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return fmt.Errorf("launchagent path is required")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
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
