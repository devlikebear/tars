package links

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	const console = "http://127.0.0.1:43180"
	ok := map[string]string{
		Prefix + "https://github.com/devlikebear/tars": "https://github.com/devlikebear/tars",
		Prefix + "http://example.com/a?b=c#d":          "http://example.com/a?b=c#d",
		Prefix + "mailto:someone@example.com":          "mailto:someone@example.com",
	}
	for msg, want := range ok {
		if got, accepted := Parse(msg, console+"/", console); !accepted || got != want {
			t.Errorf("Parse(%q) = %q, %v", msg, got, accepted)
		}
		if _, accepted := Parse(msg, console+"/console/chat/s1", console); !accepted {
			t.Errorf("Parse(%q) from a page URL was refused", msg)
		}
	}
	bad := []struct{ msg, origin string }{
		{Prefix + "https://example.com", "http://127.0.0.1:9999/console"},
		{Prefix + "https://example.com", "https://127.0.0.1:43180"},
		{Prefix + "https://example.com", "%zz"},
		{"wails:foo", console},
		{Prefix + "https://example.com", "http://evil.example"},
		{Prefix + "https://example.com", ""},
		{Prefix + "file:///etc/passwd", console},
		{Prefix + "javascript:alert(1)", console},
		{Prefix + "https:///nohost", console},
		{Prefix + "%zz", console},
	}
	for _, c := range bad {
		if got, accepted := Parse(c.msg, c.origin, console); accepted {
			t.Errorf("Parse(%q from %q) accepted %q", c.msg, c.origin, got)
		}
	}
}

func TestScriptUsesPrefix(t *testing.T) {
	if !strings.Contains(Script, `"`+Prefix+`"`) {
		t.Fatal("the script must send the prefix Parse expects")
	}
}
