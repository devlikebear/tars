package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/devlikebear/tars/desktop/internal/activity"
	"github.com/devlikebear/tars/desktop/internal/deeplink"
	"github.com/devlikebear/tars/desktop/internal/icon"
	"github.com/devlikebear/tars/desktop/internal/links"
	"github.com/devlikebear/tars/desktop/internal/server"
	"github.com/devlikebear/tars/desktop/internal/tray"
	"github.com/devlikebear/tars/desktop/internal/winstate"
)

const (
	// pollEvery is how often the tray re-reads the server when nothing
	// prompts it sooner; the event stream prompts it on each new question.
	pollEvery = 3 * time.Second
	// sessionsEvery is how often the recent-chats submenu is refreshed.
	sessionsEvery = 20 * time.Second
	// recentChats is how many chats the submenu lists.
	recentChats = 8
	// notificationPrefix keeps approval notification IDs apart from others.
	notificationPrefix = "tars-approval-"
)

type shell struct {
	cfg    server.Config
	client *activity.Client
	probe  *http.Client

	app      *application.App
	window   *application.WebviewWindow
	tray     *application.SystemTray
	notifier *notifications.NotificationService
	updates  bool

	quitting  atomic.Bool
	pollNow   chan struct{}
	approvals tray.Approvals

	mu           sync.Mutex
	status       tray.Status
	statusSet    bool
	sessions     []activity.Session
	sessionsAt   time.Time
	menuKey      string
	onConsole    bool
	pendingPath  string
	winStatePath string
	lastErrors   map[string]string
}

func newShell(cfg server.Config) *shell {
	s := &shell{
		cfg:     cfg,
		client:  activity.NewClient(cfg, nil),
		probe:   &http.Client{Timeout: 2 * time.Second},
		pollNow: make(chan struct{}, 1),
	}
	if path, err := winstate.DefaultPath(); err == nil {
		s.winStatePath = path
	}
	return s
}

// createWindow makes the console window. It starts on the bundled offline
// page and moves to the console once the server answers.
func (s *shell) createWindow(show bool) {
	opts := application.WebviewWindowOptions{
		Name:             "console",
		Title:            "TARS",
		Width:            1280,
		Height:           840,
		MinWidth:         winstate.MinWidth,
		MinHeight:        winstate.MinHeight,
		URL:              "/",
		Hidden:           true,
		BackgroundColour: application.NewRGB(0x16, 0x18, 0x1b),
		JS:               links.Script,
	}
	s.window = s.app.Window.NewWithOptions(opts)

	// Closing the window hides it: the console's streams and the tray keep
	// running, and the hotkey brings it straight back.
	s.window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.quitting.Load() {
			return
		}
		s.saveWindow()
		s.window.Hide()
		e.Cancel()
	})
	if show {
		s.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			s.restoreWindow()
			s.window.Show()
			s.window.Focus()
		})
	}
}

func (s *shell) restoreWindow() {
	if s.winStatePath == "" {
		return
	}
	saved, ok := winstate.Load(s.winStatePath)
	if !ok {
		s.window.Center()
		return
	}
	var screens []winstate.Rect
	for _, sc := range s.app.Screen.GetAll() {
		screens = append(screens, winstate.Rect{X: sc.WorkArea.X, Y: sc.WorkArea.Y, Width: sc.WorkArea.Width, Height: sc.WorkArea.Height})
	}
	fit, ok := winstate.Fit(saved.Bounds, screens)
	if !ok {
		s.window.Center()
		return
	}
	s.window.SetBounds(application.Rect{X: fit.X, Y: fit.Y, Width: fit.Width, Height: fit.Height})
	if saved.Maximised {
		s.window.Maximise()
	}
}

func (s *shell) saveWindow() {
	if s.winStatePath == "" || s.window == nil || !s.window.IsVisible() || s.window.IsMinimised() {
		return
	}
	b := s.window.Bounds()
	st := winstate.State{Bounds: winstate.Rect{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height}, Maximised: s.window.IsMaximised()}
	if st.Maximised {
		// Keep the size it had before it was maximised.
		if old, ok := winstate.Load(s.winStatePath); ok {
			st.Bounds = old.Bounds
		}
	}
	if err := winstate.Save(s.winStatePath, st); err != nil {
		log.Printf("save window position: %v", err)
	}
}

func (s *shell) toggleWindow() {
	if s.window.IsVisible() && s.window.IsFocused() {
		s.saveWindow()
		s.window.Hide()
		return
	}
	s.showConsole("")
}

// showConsole brings the window up on path ("" keeps the current page). If
// the server is not up yet, the path is opened once it is.
func (s *shell) showConsole(path string) {
	s.mu.Lock()
	onConsole := s.onConsole
	if path != "" && !onConsole {
		s.pendingPath = path
	}
	s.mu.Unlock()
	if path != "" && onConsole {
		s.window.SetURL(s.cfg.ConsoleURL(path))
	}
	if !s.window.IsVisible() {
		s.restoreWindow()
	}
	s.window.Show()
	if s.window.IsMinimised() {
		s.window.UnMinimise()
	}
	s.window.Focus()
}

func (s *shell) createTray() {
	s.tray = s.app.SystemTray.New()
	s.applyTrayIcon(tray.StatusOf(tray.Observation{}))
	s.tray.OnClick(func() {
		if runtime.GOOS == "darwin" {
			s.tray.OpenMenu()
			return
		}
		s.toggleWindow()
	})
	s.rebuildMenu(tray.StatusOf(tray.Observation{}), nil)
}

func (s *shell) applyTrayIcon(st tray.Status) {
	if runtime.GOOS == "darwin" {
		s.tray.SetTemplateIcon(icon.Template(44))
		s.tray.SetLabel(st.Label())
	} else {
		s.tray.SetIcon(icon.Tray(st.State, 64))
	}
	s.tray.SetTooltip(st.Tooltip())
}

func (s *shell) rebuildMenu(st tray.Status, sessions []activity.Session) {
	items := tray.Menu(st, sessions)
	key := menuKey(items)
	s.mu.Lock()
	if key == s.menuKey {
		s.mu.Unlock()
		return
	}
	s.menuKey = key
	s.mu.Unlock()

	menu := s.app.NewMenu()
	s.addItems(menu, items)
	s.tray.SetMenu(menu)
}

func menuKey(items []tray.Item) string {
	var b strings.Builder
	var walk func([]tray.Item, int)
	walk = func(items []tray.Item, depth int) {
		for _, it := range items {
			fmt.Fprintf(&b, "%d|%s|%v|%v|%d|%s|%s|%s\n", depth, it.Label, it.Separator, it.Disabled, it.Action.Kind, it.Action.Path, it.Action.Approval.RequestID, it.Action.Decision)
			walk(it.Children, depth+1)
		}
	}
	walk(items, 0)
	return b.String()
}

func (s *shell) addItems(menu *application.Menu, items []tray.Item) {
	for _, it := range items {
		switch {
		case it.Separator:
			menu.AddSeparator()
		case len(it.Children) > 0:
			s.addItems(menu.AddSubmenu(it.Label), it.Children)
		default:
			action := it.Action
			entry := menu.Add(it.Label).SetEnabled(!it.Disabled && action.Kind != tray.NoAction)
			entry.OnClick(func(*application.Context) { go s.do(action) })
		}
	}
}

func (s *shell) do(a tray.Action) {
	switch a.Kind {
	case tray.ShowConsole:
		s.showConsole(a.Path)
	case tray.Decide:
		s.answer(a.Approval, a.Decision)
	case tray.NewSessionInFolder:
		dir, err := s.app.Dialog.OpenFile().
			SetTitle("Start a chat in a folder").
			CanChooseDirectories(true).
			CanChooseFiles(false).
			PromptForSingleSelection()
		if err != nil || dir == "" {
			return
		}
		s.startChatIn(dir)
	case tray.StartServer:
		s.startServer()
	case tray.CheckUpdates:
		s.checkUpdates()
	case tray.Quit:
		s.quitting.Store(true)
		s.saveWindow()
		s.app.Quit()
	}
}

func (s *shell) answer(a activity.Approval, decision string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.client.Answer(ctx, a, decision); err != nil {
		s.showError("Could not answer the approval", err)
	}
	s.kick()
}

// start runs once the app is up: it looks at the launch arguments, starts
// the poll loop and the event stream, and asks for notification permission.
func (s *shell) start(args []string) {
	s.registerNotifications()
	go s.pollLoop()
	go s.streamLoop()
	s.handleArgs(args)
}

func (s *shell) registerNotifications() {
	go func() {
		if ok, err := s.notifier.CheckNotificationAuthorization(); err == nil && !ok {
			if _, err := s.notifier.RequestNotificationAuthorization(); err != nil {
				log.Printf("notifications: %v", err)
			}
		}
	}()
	if err := s.notifier.RegisterNotificationCategory(notifications.NotificationCategory{
		ID: tray.NotificationCategory,
		Actions: []notifications.NotificationAction{
			{ID: tray.ActionAllowOnce, Title: tray.DecisionLabel(tray.ActionAllowOnce)},
			{ID: tray.ActionAllowSession, Title: tray.DecisionLabel(tray.ActionAllowSession)},
			{ID: tray.ActionDeny, Title: tray.DecisionLabel(tray.ActionDeny), Destructive: true},
		},
	}); err != nil {
		log.Printf("notification actions: %v", err)
	}
	s.notifier.OnNotificationResponse(func(result notifications.NotificationResult) {
		if result.Error != nil {
			log.Printf("notification response: %v", result.Error)
			return
		}
		requestID, ok := strings.CutPrefix(result.Response.ID, notificationPrefix)
		if !ok {
			return
		}
		approval, known := s.approvals.Lookup(requestID)
		decision, isDecision := tray.DecisionFor(result.Response.ActionIdentifier)
		switch {
		case known && isDecision:
			go s.answer(approval, decision)
		case known:
			go s.showConsole("/console/chat/" + url.PathEscape(approval.SessionID))
		default:
			go s.showConsole("")
		}
	})
}

func (s *shell) kick() {
	select {
	case s.pollNow <- struct{}{}:
	default:
	}
}

func (s *shell) pollLoop() {
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		s.poll()
		select {
		case <-ticker.C:
		case <-s.pollNow:
		}
	}
}

func (s *shell) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var obs tray.Observation
	health, err := server.Probe(ctx, s.probe, s.cfg)
	if err == nil {
		obs.Reachable = true
		obs.NeedsSetup = health.NeedsSetup
	}
	var sessions []activity.Session
	if obs.Reachable && !obs.NeedsSetup {
		snap, err := s.client.Activity(ctx)
		switch {
		case activity.IsUnauthorized(err):
			obs.Unauthorized = true
		case err != nil:
			s.logOnce("activity", err)
		default:
			obs.Snapshot = snap
		}
		sessions = s.recentSessions(ctx, obs.Unauthorized)
	}
	s.apply(tray.StatusOf(obs), sessions)
}

// logOnce logs an error from a poll the first time it appears, not on
// every poll: a server without /v1/chat/activity (before #1014) answers 404
// every few seconds.
func (s *shell) logOnce(what string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastErrors == nil {
		s.lastErrors = map[string]string{}
	}
	if s.lastErrors[what] == err.Error() {
		return
	}
	s.lastErrors[what] = err.Error()
	log.Printf("%s: %v", what, err)
}

func (s *shell) recentSessions(ctx context.Context, locked bool) []activity.Session {
	s.mu.Lock()
	cached, at := s.sessions, s.sessionsAt
	s.mu.Unlock()
	if locked || time.Since(at) < sessionsEvery {
		return cached
	}
	list, err := s.client.RecentSessions(ctx, recentChats)
	if err != nil {
		// No admin token on a server that wants one: the menu goes
		// without recent chats rather than failing.
		list = nil
	}
	s.mu.Lock()
	s.sessions, s.sessionsAt = list, time.Now()
	s.mu.Unlock()
	return list
}

func (s *shell) apply(st tray.Status, sessions []activity.Session) {
	s.mu.Lock()
	changed := !s.statusSet || s.status.State != st.State || s.status.Running != st.Running || s.status.Pending != st.Pending
	s.status, s.statusSet = st, true
	goConsole := st.State != tray.Offline && !s.onConsole
	path := ""
	if goConsole {
		s.onConsole = true
		path, s.pendingPath = s.pendingPath, ""
	}
	s.mu.Unlock()

	if changed {
		s.applyTrayIcon(st)
	}
	s.rebuildMenu(st, sessions)
	if goConsole {
		s.window.SetURL(s.cfg.ConsoleURL(path))
	}

	if st.State == tray.Offline {
		for _, gone := range s.approvals.Forget() {
			_ = s.notifier.RemoveNotification(notificationPrefix + gone.RequestID)
		}
		return
	}
	added, removed := s.approvals.Update(st.Snapshot.Pending)
	for _, a := range added {
		title, subtitle, body := tray.NotificationText(a)
		err := s.notifier.SendNotificationWithActions(notifications.NotificationOptions{
			ID:         notificationPrefix + a.RequestID,
			Title:      title,
			Subtitle:   subtitle,
			Body:       body,
			CategoryID: tray.NotificationCategory,
			ThreadID:   a.SessionID,
			Data:       map[string]any{"request_id": a.RequestID, "session_id": a.SessionID},
		})
		if err != nil {
			log.Printf("notify: %v", err)
		}
	}
	for _, a := range removed {
		_ = s.notifier.RemoveNotification(notificationPrefix + a.RequestID)
	}
}

// streamLoop listens to the server's event stream so a new approval shows
// at once instead of on the next poll. It reconnects after a drop.
func (s *shell) streamLoop() {
	for {
		err := s.client.Stream(context.Background(), func(e activity.Event) {
			if e.Category == "approval" || e.RequestID != "" {
				s.kick()
			}
		})
		if activity.IsUnauthorized(err) {
			time.Sleep(time.Minute)
			continue
		}
		time.Sleep(5 * time.Second)
	}
}

func (s *shell) onSecondInstance(data application.SecondInstanceData) {
	go func() {
		s.showConsole("")
		s.handleArgs(data.Args)
	}()
}

// onRawMessage opens console links that leave the app in the browser.
func (s *shell) onRawMessage(_ application.Window, message string, origin *application.OriginInfo) {
	from := ""
	if origin != nil {
		from = origin.Origin
	}
	target, ok := links.Parse(message, from, s.cfg.URL)
	if !ok {
		return
	}
	if err := s.app.Browser.OpenURL(target); err != nil {
		log.Printf("open %s: %v", target, err)
	}
}

// handleArgs acts on a tars:// link or a folder given on the command line.
func (s *shell) handleArgs(args []string) {
	if link, ok := deeplink.FromArgs(args); ok {
		s.handleLink(link)
		return
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if info, err := os.Stat(arg); err == nil && info.IsDir() {
			s.proposeChatIn(arg)
			return
		}
	}
}

func (s *shell) handleLink(raw string) {
	link, err := deeplink.Parse(raw)
	if err != nil {
		log.Printf("%v", err)
		s.showConsole("")
		return
	}
	switch link.Kind {
	case deeplink.Open:
		s.showConsole(link.Path)
	case deeplink.NewChat:
		s.proposeChatIn(link.Dir)
	}
}

// proposeChatIn asks before starting a chat in dir: a link or a dropped
// folder can come from anywhere, and a chat gets to work in its folder.
func (s *shell) proposeChatIn(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return
	}
	s.showConsole("")
	dialog := s.app.Dialog.Question().
		SetTitle("Start a TARS chat").
		SetMessage(fmt.Sprintf("Start a new chat working in\n%s ?", abs))
	start := dialog.AddButton("Start chat")
	start.OnClick(func() { go s.startChatIn(abs) })
	cancel := dialog.AddButton("Cancel")
	dialog.SetDefaultButton(start).SetCancelButton(cancel).AttachToWindow(s.window).Show()
}

func (s *shell) startChatIn(dir string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := s.client.NewSessionIn(ctx, filepath.Base(dir), dir)
	if id == "" {
		s.showError("Could not start a chat", err)
		return
	}
	if err != nil {
		// The session exists but kept its default folder.
		s.showError("The chat started, but not in that folder", err)
	}
	s.showConsole("/console/chat/" + url.PathEscape(id))
}

func (s *shell) startServer() {
	exe, _ := os.Executable()
	bin, err := server.FindTARS(exe, exec.LookPath)
	if err != nil {
		s.showError("Could not start the TARS server", err)
		return
	}
	plan := server.PlanStart(runtime.GOOS, bin, s.cfg)
	cmd := exec.Command(plan.Binary, plan.Args...)
	if plan.Detached {
		logFile, logErr := openServerLog()
		if logErr == nil {
			cmd.Stdout, cmd.Stderr = logFile, logFile
			defer func() { _ = logFile.Close() }()
		}
		detach(cmd)
		err = cmd.Start()
		if err == nil {
			_ = cmd.Process.Release()
		}
	} else {
		var out []byte
		out, err = cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	if err != nil {
		s.showError("Could not start the TARS server", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := server.WaitReady(ctx, s.probe, s.cfg, 500*time.Millisecond); err != nil {
		s.showError("The TARS server did not come up", err)
	}
	s.kick()
}

// openServerLog is where a detached `tars serve` writes, next to the
// window state: the shell has no terminal to show it in.
func openServerLog() (*os.File, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "tars-desktop")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

func (s *shell) checkUpdates() {
	if !s.updates {
		s.app.Dialog.Info().
			SetTitle("Updates").
			SetMessage(fmt.Sprintf("tars-desktop %s is a development build and does not update itself.", version)).
			Show()
		return
	}
	if err := s.app.Updater.CheckAndInstall(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("update: %v", err)
	}
}

func (s *shell) showError(title string, err error) {
	msg := "unknown error"
	if err != nil {
		msg = err.Error()
	}
	log.Printf("%s: %s", title, msg)
	s.app.Dialog.Error().SetTitle(title).SetMessage(msg).Show()
}
