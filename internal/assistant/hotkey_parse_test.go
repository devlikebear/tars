package assistant

import "testing"

func TestParseHotkeySpec(t *testing.T) {
	spec, err := parseHotkeySpec("Ctrl+Option+Space")
	if err != nil {
		t.Fatalf("parse hotkey: %v", err)
	}
	if spec.Key != "space" {
		t.Fatalf("expected key=space, got %q", spec.Key)
	}
	if len(spec.Modifiers) != 2 || spec.Modifiers[0] != "ctrl" || spec.Modifiers[1] != "option" {
		t.Fatalf("unexpected modifiers: %+v", spec.Modifiers)
	}
}

func TestParseHotkeySpec_Invalid(t *testing.T) {
	if _, err := parseHotkeySpec("Ctrl+Option"); err == nil {
		t.Fatalf("expected parse error for missing key")
	}
	if _, err := parseHotkeySpec("Ctrl+Option+UnknownKey"); err == nil {
		t.Fatalf("expected parse error for unknown key")
	}
}
