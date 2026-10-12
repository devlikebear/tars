package initiative

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// telegramLiveRuntime is liveRuntime (live_speak_test.go) plus an opt-in
// Config.Telegram flag.
func telegramLiveRuntime(t *testing.T, now *time.Time, speaker Speaker, telegram bool) *Runtime {
	t.Helper()
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	deps := Dependencies{
		Observer: &fakeObserver{}, Ledger: ledger, Logger: zerolog.Nop(),
		Now: func() time.Time { return *now }, Speaker: speaker,
	}
	return NewRuntime(Config{
		Enabled: true, Mode: ModeLive, Location: seoul, DailySpeakCalls: 12, Telegram: telegram,
	}, deps)
}

// TestTelegramOptOutByteForByteUnchanged (requirement #1): with
// Config.Telegram false, deliverLocked must behave exactly as before this
// feature existed — unreachable/no_console, no compose call, no Channel —
// even when TelegramReady is true (opt-in is what gates it, not readiness).
func TestTelegramOptOutByteForByteUnchanged(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{}
	r := telegramLiveRuntime(t, &now, speaker, false)
	r.deps.Observer = &fakeObserver{obs: Observation{
		TelegramPaired: true,
		TelegramReady:  true,
		RecentUser:     []UserMessage{{At: now.Add(-25 * time.Hour)}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliveryUnreachable || e.DeliveryReason != deliveryReasonNoConsole {
		t.Fatalf("entry = %+v, want unreachable/no_console unchanged by TelegramReady", e)
	}
	if e.Channel != "" || e.ChannelError != "" {
		t.Fatalf("entry = %+v, want no channel fields when telegram is off", e)
	}
	if speaker.callCount() != 0 {
		t.Fatalf("speaker calls = %d, want 0", speaker.callCount())
	}
}

// TestTelegramChannelChosenWhenConsoleAbsent (requirement #2): opted in,
// console absent, a default chat resolves — the runtime picks telegram and
// Speak is asked for that channel.
func TestTelegramChannelChosenWhenConsoleAbsent(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}
	r := telegramLiveRuntime(t, &now, speaker, true)
	r.deps.Observer = &fakeObserver{obs: Observation{
		TelegramPaired: true,
		TelegramReady:  true,
		RecentUser:     []UserMessage{{At: now.Add(-25 * time.Hour)}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliveryDelivered || e.Channel != ChannelTelegram {
		t.Fatalf("entry = %+v, want delivered over telegram", e)
	}
	calls := speaker.calls
	if len(calls) != 1 || calls[0].Channel != ChannelTelegram {
		t.Fatalf("speaker calls = %+v, want one call with Channel=telegram", calls)
	}
}

// TestConsoleWinsOverTelegramWhenBothAvailable (requirement #2): never
// both — console always wins when connected, even with telegram opted in
// and ready.
func TestConsoleWinsOverTelegramWhenBothAvailable(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}
	r := telegramLiveRuntime(t, &now, speaker, true)
	r.deps.Observer = &fakeObserver{obs: Observation{
		ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-time.Hour),
		TelegramReady: true,
		RecentUser:    []UserMessage{{At: now.Add(-25 * time.Hour)}},
	}}
	e := r.RunOnce(context.Background())
	if e.Channel != ChannelConsole {
		t.Fatalf("entry channel = %q, want console", e.Channel)
	}
	if len(speaker.calls) != 1 || speaker.calls[0].Channel != ChannelConsole {
		t.Fatalf("speaker calls = %+v, want Channel=console", speaker.calls)
	}
}

// TestTelegramUnavailableWithoutReadyChatNeverComposes (requirement #1):
// opted in but no single default chat resolves — unreachable with its own
// reason code, still no compose call.
func TestTelegramUnavailableWithoutReadyChatNeverComposes(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{}
	r := telegramLiveRuntime(t, &now, speaker, true)
	r.deps.Observer = &fakeObserver{obs: Observation{
		TelegramPaired: true, // paired, but not resolvable to exactly one chat
		TelegramReady:  false,
		RecentUser:     []UserMessage{{At: now.Add(-25 * time.Hour)}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliveryUnreachable || e.DeliveryReason != deliveryReasonTelegramUnavailable {
		t.Fatalf("entry = %+v, want unreachable/telegram_unavailable", e)
	}
	if speaker.callCount() != 0 {
		t.Fatalf("speaker calls = %d, want 0", speaker.callCount())
	}
}

// TestTelegramChannelErrorStaysDelivered (requirement #3): a channel send
// failure after the main-session write succeeded must not change Delivery
// — the entry stays delivered, only ChannelError records the fixed code.
func TestTelegramChannelErrorStaysDelivered(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true, ChannelError: "send_failed"}}}
	r := telegramLiveRuntime(t, &now, speaker, true)
	r.deps.Observer = &fakeObserver{obs: Observation{
		TelegramPaired: true,
		TelegramReady:  true,
		RecentUser:     []UserMessage{{At: now.Add(-25 * time.Hour)}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliveryDelivered {
		t.Fatalf("entry delivery = %q, want delivered despite the channel error", e.Delivery)
	}
	if e.Channel != ChannelTelegram || e.ChannelError != "send_failed" {
		t.Fatalf("entry = %+v, want channel=telegram, channel_error=send_failed", e)
	}
	// Cooldown/daily cap must still have advanced like any delivered speak.
	now = now.Add(time.Minute)
	e2 := r.RunOnce(context.Background())
	if e2.Reason != "cooldown" {
		t.Fatalf("second tick reason = %q, want cooldown (delivery counted despite channel error)", e2.Reason)
	}
}

// TestTelegramSupersededNeverGetsChannel (sanity for requirement #3): a
// superseded outcome (claim lost / user spoke first) must never record a
// Channel, since nothing was actually delivered over it.
func TestTelegramSupersededNeverGetsChannel(t *testing.T) {
	now := at(9, 0)
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Composed: true, Superseded: true, Reason: "busy"}}}
	r := telegramLiveRuntime(t, &now, speaker, true)
	r.deps.Observer = &fakeObserver{obs: Observation{
		TelegramPaired: true,
		TelegramReady:  true,
		RecentUser:     []UserMessage{{At: now.Add(-25 * time.Hour)}},
	}}
	e := r.RunOnce(context.Background())
	if e.Delivery != DeliverySkipped || e.Channel != "" {
		t.Fatalf("entry = %+v, want skipped with no channel", e)
	}
}
