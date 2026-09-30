package initiative

import (
	"strings"
	"testing"
	"time"
)

func TestRenderStateIncludesRecentTextWhenLocal(t *testing.T) {
	cfg := Config{Location: seoul}.WithDefaults()
	now := at(9, 0)
	obs := Observation{
		Now:                now,
		ConsoleConnectedAt: now.Add(-2 * time.Minute),
		RecentUser: []UserMessage{
			{At: now.Add(-5 * time.Hour), Text: "too old to include"},
			{At: now.Add(-5 * time.Minute), Text: "오늘은 메시지 그만 보내줘"},
		},
		Profile: "# USER.md\nbirthday: Sep 29",
	}
	state, key := RenderState(cfg, obs, true)
	want := strings.Join([]string{
		"now: Tue 2026-09-29 09:00",
		"console: connected 2m ago",
		"last_user_message: 5m ago",
		"recent_user_messages (newest last):",
		`- "오늘은 메시지 그만 보내줘" (5m ago)`,
		"profile:",
		"# USER.md",
		"birthday: Sep 29",
	}, "\n")
	if state != want {
		t.Fatalf("state =\n%s\nwant\n%s", state, want)
	}
	if key == "" {
		t.Fatal("expected a text key when text is present")
	}
	later := obs
	later.Now = now.Add(time.Minute)
	if _, key2 := RenderState(cfg, later, true); key2 != key {
		t.Fatal("text key must not change when only the clock moves")
	}
	nextDay := obs
	nextDay.Now = now.Add(24 * time.Hour)
	nextDay.RecentUser = []UserMessage{{At: nextDay.Now.Add(-5 * time.Minute), Text: "오늘은 메시지 그만 보내줘"}}
	if _, key3 := RenderState(cfg, nextDay, true); key3 == key {
		t.Fatal("text key must change with the date so special_day is re-read")
	}
}

func TestRenderStateOmitsTextWhenRemote(t *testing.T) {
	cfg := Config{Location: seoul}.WithDefaults()
	now := at(9, 0)
	state, key := RenderState(cfg, Observation{
		Now:        now,
		RecentUser: []UserMessage{{At: now.Add(-5 * time.Minute), Text: "secret plans"}},
		Profile:    "birthday: Sep 29",
	}, false)
	if strings.Contains(state, "secret plans") || strings.Contains(state, "birthday") || key != "" {
		t.Fatalf("remote state leaked text: %q key=%q", state, key)
	}
	if !strings.Contains(state, "console: not connected") || !strings.Contains(state, "last_user_message: 5m ago") {
		t.Fatalf("remote state lost metadata: %q", state)
	}
}

func TestRenderStateWithoutTextHasNoKey(t *testing.T) {
	cfg := Config{Location: seoul}.WithDefaults()
	state, key := RenderState(cfg, Observation{Now: at(9, 0)}, true)
	if key != "" || !strings.Contains(state, "last_user_message: none") {
		t.Fatalf("state=%q key=%q", state, key)
	}
}

func TestRenderStateTruncatesLongText(t *testing.T) {
	cfg := Config{Location: seoul}.WithDefaults()
	now := at(9, 0)
	state, _ := RenderState(cfg, Observation{Now: now, RecentUser: []UserMessage{{At: now, Text: strings.Repeat("가", 500)}}}, true)
	if strings.Count(state, "가") != maxMessageRunes {
		t.Fatalf("message not truncated to %d runes", maxMessageRunes)
	}
}
