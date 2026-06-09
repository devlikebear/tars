package launchagent

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

func DefaultDomainForUID(uid int) string {
	return "gui/" + strconv.Itoa(uid)
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

func ProgramArgumentsFromPlist(data []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	args := []string{}
	wantProgramArguments := false
	inProgramArguments := false

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return args, nil
		}
		if err != nil {
			return nil, err
		}
		switch tok := token.(type) {
		case xml.StartElement:
			switch tok.Name.Local {
			case "key":
				var key string
				if err := decoder.DecodeElement(&key, &tok); err != nil {
					return nil, err
				}
				wantProgramArguments = strings.TrimSpace(key) == "ProgramArguments"
			case "array":
				if wantProgramArguments {
					inProgramArguments = true
					wantProgramArguments = false
				}
			case "string":
				if !inProgramArguments {
					wantProgramArguments = false
					continue
				}
				var value string
				if err := decoder.DecodeElement(&value, &tok); err != nil {
					return nil, err
				}
				args = append(args, strings.TrimSpace(value))
			default:
				if wantProgramArguments {
					wantProgramArguments = false
				}
			}
		case xml.EndElement:
			if tok.Name.Local == "array" && inProgramArguments {
				return args, nil
			}
		}
	}
}

func ArgumentValue(args []string, flag string) (string, bool) {
	want := strings.TrimSpace(flag)
	if want == "" {
		return "", false
	}
	for i := 0; i < len(args)-1; i++ {
		if strings.TrimSpace(args[i]) != want {
			continue
		}
		value := strings.TrimSpace(args[i+1])
		if value == "" {
			return "", false
		}
		return value, true
	}
	return "", false
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
