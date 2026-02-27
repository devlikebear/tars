package assistant

import (
	"fmt"
	"strings"
)

type hotkeySpec struct {
	Modifiers []string
	Key       string
}

var supportedHotkeyKeys = func() map[string]struct{} {
	keys := map[string]struct{}{
		"space":  {},
		"enter":  {},
		"return": {},
		"esc":    {},
		"escape": {},
		"tab":    {},
		"up":     {},
		"down":   {},
		"left":   {},
		"right":  {},
	}
	for ch := 'a'; ch <= 'z'; ch++ {
		keys[string(ch)] = struct{}{}
	}
	for ch := '0'; ch <= '9'; ch++ {
		keys[string(ch)] = struct{}{}
	}
	for i := 1; i <= 20; i++ {
		keys[fmt.Sprintf("f%d", i)] = struct{}{}
	}
	return keys
}()

func parseHotkeySpec(raw string) (hotkeySpec, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return hotkeySpec{}, fmt.Errorf("hotkey is required")
	}
	parts := strings.Split(input, "+")
	if len(parts) < 2 {
		return hotkeySpec{}, fmt.Errorf("hotkey must include modifiers and key (example: Ctrl+Option+Space)")
	}
	spec := hotkeySpec{Modifiers: make([]string, 0, len(parts)-1)}
	seenMod := map[string]struct{}{}
	for _, part := range parts {
		token := normalizeHotkeyToken(part)
		if token == "" {
			continue
		}
		if mod, ok := normalizeModifier(token); ok {
			if _, exists := seenMod[mod]; exists {
				continue
			}
			seenMod[mod] = struct{}{}
			spec.Modifiers = append(spec.Modifiers, mod)
			continue
		}
		if _, ok := supportedHotkeyKeys[token]; !ok {
			return hotkeySpec{}, fmt.Errorf("unsupported hotkey token: %s", strings.TrimSpace(part))
		}
		if spec.Key != "" {
			return hotkeySpec{}, fmt.Errorf("hotkey must include exactly one key")
		}
		spec.Key = token
	}
	if len(spec.Modifiers) == 0 {
		return hotkeySpec{}, fmt.Errorf("hotkey must include at least one modifier")
	}
	if spec.Key == "" {
		return hotkeySpec{}, fmt.Errorf("hotkey key is required")
	}
	return spec, nil
}

func normalizeHotkeyToken(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeModifier(token string) (string, bool) {
	switch normalizeHotkeyToken(token) {
	case "ctrl", "control":
		return "ctrl", true
	case "option", "alt":
		return "option", true
	case "shift":
		return "shift", true
	case "cmd", "command", "meta":
		return "cmd", true
	default:
		return "", false
	}
}
