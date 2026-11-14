package tarsserver

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type alertFixture struct {
	store    *focuspipeline.Store
	sessions *session.Store
	root     string
	alerts   *focusMobileAlerts
	sent     []string
	paired   bool
	queued   []chatQueuedApproval
}

func newAlertFixture(t *testing.T) *alertFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	sessions := session.NewStore(root)
	f := &alertFixture{store: focusStoreFor(sessions), sessions: sessions, root: root, paired: true}
	f.alerts = &focusMobileAlerts{
		pipelines: f.store,
		devices:   func() (bool, error) { return f.paired, nil },
		queued:    func() ([]chatQueuedApproval, error) { return f.queued, nil },
		send: func(_ context.Context, text string) error {
			f.sent = append(f.sent, text)
			return nil
		},
		baseURL: func(context.Context) string { return "https://mac-mini.tail1.ts.net/" },
		logger:  zerolog.New(io.Discard),
	}
	return f
}

func (f *alertFixture) save(t *testing.T, p focuspipeline.Pipeline) {
	t.Helper()
	if err := f.store.Save(p); err != nil {
		t.Fatal(err)
	}
}

func alertTestPipeline(gate string, cards ...focuspipeline.Card) focuspipeline.Pipeline {
	return focuspipeline.Pipeline{
		SessionID:   "s1",
		QASessionID: "s1-qa",
		Goal:        "add dark mode",
		Current:     focuspipeline.StageReview,
		Stages:      []focuspipeline.Stage{{ID: focuspipeline.StageReview, Status: focuspipeline.StatusActive}},
		OpenGate:    gate,
		Cards:       cards,
	}
}

func TestFocusAlertsSendOncePerNewWait(t *testing.T) {
	f := newAlertFixture(t)
	ctx := context.Background()

	// What already waits at start is recorded, not sent.
	f.save(t, alertTestPipeline(focuspipeline.GatePlan))
	f.alerts.tick(ctx)
	if len(f.sent) != 0 {
		t.Fatalf("a restart must send nothing: %q", f.sent)
	}

	// A new finding sends one message; the plan gate is old news.
	finding := focuspipeline.Card{ID: "f1", Kind: focuspipeline.CardFinding, Stage: focuspipeline.StageReview, Title: "Null check missing", State: focuspipeline.CardUnseen}
	f.save(t, alertTestPipeline(focuspipeline.GatePlan, finding))
	f.alerts.tick(ctx)
	want := "TARS on mac-mini · waiting on you\nadd dark mode\n• Null check missing\nhttps://mac-mini.tail1.ts.net/console/focus/s1"
	if len(f.sent) != 1 || f.sent[0] != want {
		t.Fatalf("sent = %q\nwant %q", f.sent, want)
	}

	// Nothing new: nothing sent, however many ticks.
	f.alerts.tick(ctx)
	f.alerts.tick(ctx)
	if len(f.sent) != 1 {
		t.Fatalf("still waiting must not repeat: %q", f.sent)
	}

	// A queued tool call in the Q&A session is news too.
	f.queued = []chatQueuedApproval{{ApprovalID: "a1", SessionID: "s1-qa", ToolName: "exec"}, {ApprovalID: "a2", SessionID: "elsewhere", ToolName: "exec"}}
	f.alerts.tick(ctx)
	if len(f.sent) != 2 || !strings.Contains(f.sent[1], "• Tool call waits for approval: exec") {
		t.Fatalf("approval alert = %q", f.sent)
	}
}

func TestFocusAlertsNeedAPairedPhone(t *testing.T) {
	f := newAlertFixture(t)
	ctx := context.Background()
	f.paired = false
	f.alerts.tick(ctx)
	f.save(t, alertTestPipeline(focuspipeline.GateMerge))
	f.alerts.tick(ctx)
	if len(f.sent) != 0 {
		t.Fatalf("no phone, no message: %q", f.sent)
	}
	// Pairing later does not replay what waited before.
	f.paired = true
	f.alerts.tick(ctx)
	if len(f.sent) != 0 {
		t.Fatalf("pairing must not replay old waits: %q", f.sent)
	}

	// A device check that fails sends nothing either.
	f.alerts.devices = func() (bool, error) { return true, errors.New("broken") }
	f.save(t, alertTestPipeline(focuspipeline.GateBlocked))
	f.alerts.tick(ctx)
	if len(f.sent) != 0 {
		t.Fatalf("unknown pairing, no message: %q", f.sent)
	}
}

func TestFocusAlertsSendFailureMovesOn(t *testing.T) {
	f := newAlertFixture(t)
	ctx := context.Background()
	calls := 0
	f.alerts.send = func(context.Context, string) error { calls++; return errors.New("telegram down") }
	f.alerts.tick(ctx)
	f.save(t, alertTestPipeline(focuspipeline.GatePR))
	f.alerts.tick(ctx)
	f.alerts.tick(ctx)
	if calls != 1 {
		t.Fatalf("a failed send is not retried every tick: %d", calls)
	}
}

func TestFocusAlertMessageWithoutLink(t *testing.T) {
	f := newAlertFixture(t)
	f.alerts.baseURL = func(context.Context) string { return "" }
	if got := f.alerts.link(context.Background(), "s1"); got != "" {
		t.Fatalf("no address, no link: %q", got)
	}
	f.alerts.baseURL = nil
	if got := f.alerts.link(context.Background(), "s1"); got != "" {
		t.Fatalf("no detector, no link: %q", got)
	}
	got := focusAlertMessage(focuspipeline.Pipeline{Goal: "  "}, []string{"Ready to merge (G4)"}, "")
	if got != "TARS · waiting on you\n• Ready to merge (G4)" {
		t.Fatalf("message = %q", got)
	}
}

func TestFocusAlertsNilAndUnreadable(t *testing.T) {
	var nilAlerts *focusMobileAlerts
	nilAlerts.tick(context.Background())
	nilAlerts.start(context.Background())
	startFocusAlerts(context.Background(), nil)

	// A sessions path that is a file: the listing fails and nothing is sent.
	f := newAlertFixture(t)
	f.alerts.pipelines = focuspipeline.NewStore(filepath.Join(f.root, "AGENTS.md"))
	f.alerts.tick(context.Background())
	if len(f.sent) != 0 || f.alerts.primed {
		t.Fatalf("unreadable pipelines: sent=%q primed=%v", f.sent, f.alerts.primed)
	}
}

type failingRemoteRunner struct{ calls int }

func (r *failingRemoteRunner) Run(context.Context, string, ...string) (string, string, error) {
	r.calls++
	return "", "not running", errors.New("exit 1")
}

func TestStartFocusAlertsUsesRemoteAccessAddress(t *testing.T) {
	f := newAlertFixture(t)
	runner := &failingRemoteRunner{}
	runtime := &serveAPIRuntime{focusAlerts: f.alerts, remoteAccessRunner: runner}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startFocusAlerts(ctx, runtime)
	// Tailscale that does not answer leaves the message without a link.
	if got := f.alerts.link(context.Background(), "s1"); got != "" || runner.calls == 0 {
		t.Fatalf("link = %q, calls = %d", got, runner.calls)
	}
}

func TestCachedRemoteURL(t *testing.T) {
	now := time.Unix(0, 0)
	calls := 0
	get := cachedRemoteURL(func(context.Context) string { calls++; return "https://a" }, time.Minute, func() time.Time { return now })
	ctx := context.Background()
	first, second := get(ctx), get(ctx)
	if first != "https://a" || second != "https://a" || calls != 1 {
		t.Fatalf("cached: %q %q calls=%d", first, second, calls)
	}
	now = now.Add(2 * time.Minute)
	get(ctx)
	if calls != 2 {
		t.Fatalf("expired: calls=%d", calls)
	}
}

func TestPhoneDevicePaired(t *testing.T) {
	store := consoleauth.NewStore(t.TempDir())
	if ok, err := phoneDevicePaired(store); ok || err != nil {
		t.Fatalf("no devices: %v %v", ok, err)
	}
	device, _, err := store.CreateDevice("iPhone", consoleauth.RoleFocus)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := phoneDevicePaired(store); !ok {
		t.Fatal("an active phone is paired")
	}
	if err := store.RevokeDevice(device.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := phoneDevicePaired(store); ok {
		t.Fatal("a revoked phone is not")
	}
}

func TestNewFocusMobileAlertsSendsThroughTelegram(t *testing.T) {
	f := newAlertFixture(t)
	pairings, err := newTelegramPairingStore(filepath.Join(t.TempDir(), "telegram_pairings.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeInitiativeTelegramSender{}
	cfg := config.Config{RuntimeConfig: config.RuntimeConfig{WorkspaceDir: f.root}}

	// The channel off: no sender at all.
	if a := newFocusMobileAlerts(cfg, f.sessions, nil, sender, pairings, zerolog.Nop()); a.send != nil {
		t.Fatal("telegram disabled must not send")
	}

	cfg.ChannelsTelegramEnabled = true
	a := newFocusMobileAlerts(cfg, f.sessions, nil, sender, pairings, zerolog.Nop())
	if err := a.send(context.Background(), "hi"); !errors.Is(err, errFocusAlertNoChat) || len(sender.calls) != 0 {
		t.Fatalf("no paired chat: %v calls=%d", err, len(sender.calls))
	}
	issued, _, err := pairings.issue(telegramPairingIdentity{UserID: 41, ChatID: "4101", Username: "dev"}, telegramPairingTTL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pairings.approve(issued.Code); err != nil {
		t.Fatal(err)
	}
	if err := a.send(context.Background(), "hi"); err != nil || len(sender.calls) != 1 || sender.calls[0].ChatID != "4101" || sender.calls[0].ParseMode != "" {
		t.Fatalf("send: %v %+v", err, sender.calls)
	}
	if paired, err := a.devices(); paired || err != nil {
		t.Fatalf("no phone paired yet: %v %v", paired, err)
	}
}
