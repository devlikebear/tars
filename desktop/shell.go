package main

import (
	"bytes"
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
	"github.com/devlikebear/tars/desktop/internal/serverupdate"
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
	// chatWidth and chatHeight are a chat window's size the first time it
	// opens; after that it reopens where it was.
	chatWidth  = 1100
	chatHeight = 780
	// queuedNotificationPrefix marks an unattended run's question (#1033),
	// answered through the ops approvals rather than the chat API.
	queuedNotificationPrefix = "tars-unattended-"
	// updateNotificationPrefix marks the update notifications; a click on
	// one runs Check for updates….
	updateNotificationPrefix = "tars-update-"
	// serverUpdateTimeout bounds one server update: `tars update`'s download
	// and restart (tars waits up to a minute for it), or Homebrew's
	// `brew update` and `brew upgrade`.
	serverUpdateTimeout = 10 * time.Minute
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
	queued    tray.QueuedApprovals

	// windows is where the console window and the chat windows were.
	windows *winstate.Store

	mu          sync.Mutex
	status      tray.Status
	statusSet   bool
	sessions    []activity.Session
	sessionsAt  time.Time
	menuKey     string
	onConsole   bool
	pendingPath string
	chats       map[string]*application.WebviewWindow
	lastErrors  map[string]string
	// warnedServer is the server version last flagged as older than the
	// app, so each one is reported once per run.
	warnedServer *string
	// announced is the release versions whose update notifications were
	// sent this run, keyed by what they are for ("desktop", "server-wait").
	announced map[string]string

	// updateNow wakes the update loop before its timer.
	updateNow chan struct{}
	// updating serialises server updates between the loop and the tray.
	updating sync.Mutex
}

func newShell(cfg server.Config) *shell {
	s := &shell{
		cfg:       cfg,
		client:    activity.NewClient(cfg, nil),
		probe:     &http.Client{Timeout: 2 * time.Second},
		pollNow:   make(chan struct{}, 1),
		updateNow: make(chan struct{}, 1),
		chats:     map[string]*application.WebviewWindow{},
	}
	// Without a config dir the places are kept for this run only.
	path, _ := winstate.DefaultPath()
	s.windows = winstate.Open(path, deeplink.ValidSessionID)
	return s
}

// createWindow makes the console window. It starts on the bundled offline
// page and moves to the console once the server answers. A shown window
// brings back the chat windows that were open when the shell quit; a hidden
// start leaves them for the next one.
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
			open := s.chatWindows()
			for _, sw := range s.windows.Sessions() {
				if open[sw.ID] == nil {
					s.createChatWindow(sw.ID)
				}
			}
		})
	}
}

func (s *shell) restoreWindow() {
	saved, _ := s.windows.Main()
	s.placeWindow(s.window, saved)
}

// placeWindow moves w to its saved place, fitted to the screens now
// attached; a window never saved, or saved off every screen, is centred.
func (s *shell) placeWindow(w *application.WebviewWindow, saved winstate.Window) {
	var screens []winstate.Rect
	for _, sc := range s.app.Screen.GetAll() {
		screens = append(screens, winstate.Rect{X: sc.WorkArea.X, Y: sc.WorkArea.Y, Width: sc.WorkArea.Width, Height: sc.WorkArea.Height})
	}
	fit, ok := winstate.Fit(saved.Bounds, screens)
	if !ok {
		w.Center()
		return
	}
	w.SetBounds(application.Rect{X: fit.X, Y: fit.Y, Width: fit.Width, Height: fit.Height})
	if saved.Maximised {
		w.Maximise()
	}
}

// placeOf is where w is now; ok is false for a hidden or minimised window,
// whose bounds say nothing about where it should reopen.
func placeOf(w *application.WebviewWindow) (winstate.Window, bool) {
	if w == nil || !w.IsVisible() || w.IsMinimised() {
		return winstate.Window{}, false
	}
	b := w.Bounds()
	return winstate.Window{Bounds: winstate.Rect{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height}, Maximised: w.IsMaximised()}, true
}

func (s *shell) saveWindow() {
	place, ok := placeOf(s.window)
	if !ok {
		return
	}
	if err := s.windows.SetMain(place); err != nil {
		log.Printf("save window position: %v", err)
	}
}

// saveWindows records every window's place, for a quit.
func (s *shell) saveWindows() {
	s.saveWindow()
	for id, w := range s.chatWindows() {
		place, ok := placeOf(w)
		if !ok {
			continue
		}
		if err := s.windows.SetSession(id, place); err != nil {
			log.Printf("save window position: %v", err)
		}
	}
}

// onShutdown runs however the app quits, before Wails closes the windows:
// marking the quit first keeps the chat windows' close hooks from dropping
// them from the list to reopen.
func (s *shell) onShutdown() {
	s.quitting.Store(true)
	s.saveWindows()
}

func (s *shell) chatWindows() map[string]*application.WebviewWindow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]*application.WebviewWindow, len(s.chats))
	for id, w := range s.chats {
		out[id] = w
	}
	return out
}

func chatPath(id string) string {
	return "/console/chat/" + url.PathEscape(id)
}

// openChatWindow shows chat id in a window of its own, or brings its window
// forward when it has one. Past winstate.MaxSessions windows the chat opens
// in the console window instead.
func (s *shell) openChatWindow(id string) {
	if !deeplink.ValidSessionID(id) {
		log.Printf("chat window: bad session id %q", id)
		return
	}
	s.mu.Lock()
	w := s.chats[id]
	s.mu.Unlock()
	if w != nil {
		w.Show()
		if w.IsMinimised() {
			w.UnMinimise()
		}
		w.Focus()
		return
	}
	added, err := s.windows.AddSession(id)
	if err != nil {
		log.Printf("save window list: %v", err)
	}
	if !added {
		log.Printf("%d chat windows are open; showing %s in the console window", winstate.MaxSessions, id)
		s.showConsole(chatPath(id))
		return
	}
	s.createChatWindow(id)
}

// createChatWindow makes chat id's window at its saved place. Like the
// console window it waits on the offline page until the server answers.
// Closing it closes it for good and drops it from the list to reopen.
func (s *shell) createChatWindow(id string) {
	s.mu.Lock()
	title := tray.ChatWindowTitle(s.sessions, id)
	s.mu.Unlock()
	w := s.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "chat-" + id,
		Title:            title,
		Width:            chatWidth,
		Height:           chatHeight,
		MinWidth:         winstate.MinWidth,
		MinHeight:        winstate.MinHeight,
		URL:              "/",
		Hidden:           true,
		BackgroundColour: application.NewRGB(0x16, 0x18, 0x1b),
		JS:               links.Script,
	})
	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		if s.quitting.Load() {
			return
		}
		s.mu.Lock()
		delete(s.chats, id)
		s.mu.Unlock()
		if err := s.windows.RemoveSession(id); err != nil {
			log.Printf("save window list: %v", err)
		}
	})
	// Registered and checked under one lock: apply either finds this window
	// when the server comes up, or the server is already up.
	s.mu.Lock()
	s.chats[id] = w
	onConsole := s.onConsole
	s.mu.Unlock()
	if onConsole {
		w.SetURL(s.cfg.ConsoleURL(chatPath(id)))
	}
	saved, _ := s.windows.Session(id)
	s.placeWindow(w, saved.Window)
	w.Show()
	w.Focus()
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
	key := tray.Key(items)
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
	case tray.OpenSessionWindow:
		s.openChatWindow(a.SessionID)
	case tray.Decide:
		s.answer(a.Approval, a.Decision)
	case tray.Review:
		s.review(a.Queued, a.Decision == tray.ReviewApprove)
	case tray.NewSessionInFolder:
		dir, err := s.app.Dialog.OpenFile().
			SetTitle("Start a chat in a folder").
			CanChooseDirectories(true).
			CanChooseFiles(false).
			PromptForSingleSelection()
		if err != nil || dir == "" {
			return
		}
		// The folder was just picked: ask again only to offer a worktree.
		if deeplink.InGitRepository(dir) {
			s.proposeChatIn(dir, false)
			return
		}
		s.startChatIn(dir, false)
	case tray.StartServer:
		s.startServer()
	case tray.CheckUpdates:
		s.checkUpdates()
	case tray.Quit:
		s.onShutdown()
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

// review answers an unattended run's queued question through the ops
// approvals. One answered elsewhere in the meantime (409) needs no dialog.
func (s *shell) review(q activity.QueuedApproval, approve bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.client.Review(ctx, q, approve); err != nil && !activity.IsConflict(err) {
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
	if serverupdate.Enabled(runtime.GOOS) {
		go s.updateLoop()
	}
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
	s.registerNotificationCategories()
	s.notifier.OnNotificationResponse(s.onNotificationResponse)
}

// registerNotificationCategories declares the buttons of both kinds of
// approval notification.
func (s *shell) registerNotificationCategories() {
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
	if err := s.notifier.RegisterNotificationCategory(notifications.NotificationCategory{
		ID: tray.QueuedNotificationCategory,
		Actions: []notifications.NotificationAction{
			{ID: tray.ActionApprove, Title: tray.ReviewLabel(tray.ReviewApprove)},
			{ID: tray.ActionReject, Title: tray.ReviewLabel(tray.ReviewReject), Destructive: true},
		},
	}); err != nil {
		log.Printf("notification actions: %v", err)
	}
}

// onNotificationResponse handles a click on an approval notification: a
// chat question's answer, an unattended run's review, or opening the chat.
func (s *shell) onNotificationResponse(result notifications.NotificationResult) {
	if result.Error != nil {
		log.Printf("notification response: %v", result.Error)
		return
	}
	if what, ok := strings.CutPrefix(result.Response.ID, updateNotificationPrefix); ok {
		if what == "desktop" {
			go s.checkUpdates()
		}
		return
	}
	if approvalID, ok := strings.CutPrefix(result.Response.ID, queuedNotificationPrefix); ok {
		s.onQueuedResponse(approvalID, result.Response.ActionIdentifier)
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
		go s.showConsole(chatPath(approval.SessionID))
	default:
		go s.showConsole("")
	}
}

// onQueuedResponse handles a click on an unattended run's notification:
// approve or reject through the ops approvals, or open the chat.
func (s *shell) onQueuedResponse(approvalID, actionID string) {
	q, known := s.queued.Lookup(approvalID)
	decision, isReview := tray.ReviewFor(actionID)
	switch {
	case known && isReview:
		go s.review(q, decision == tray.ReviewApprove)
	case known:
		go s.showConsole(chatPath(q.SessionID))
	default:
		go s.showConsole("")
	}
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
		s.checkServerVersion(health.Version)
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

// checkServerVersion tells the user, once per server version, when the
// server is older than the app: installing or upgrading the app (the cask
// included) leaves an already installed server as it was.
func (s *shell) checkServerVersion(serverVersion string) {
	if !server.Outdated(serverVersion, version) {
		return
	}
	s.mu.Lock()
	if s.warnedServer != nil && *s.warnedServer == serverVersion {
		s.mu.Unlock()
		return
	}
	s.warnedServer = &serverVersion
	s.mu.Unlock()
	log.Printf("server %q is older than tars-desktop %s", serverVersion, version)
	go s.app.Dialog.Warning().
		SetTitle("The TARS server needs an update").
		SetMessage(server.OutdatedMessage(runtime.GOOS, serverVersion, version)).
		Show()
	if serverupdate.Enabled(runtime.GOOS) {
		s.wakeUpdates()
	}
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
	changed := !s.statusSet || s.status.State != st.State || s.status.Running != st.Running || s.status.Pending != st.Pending || s.status.Queued != st.Queued
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
		for id, w := range s.chatWindows() {
			w.SetURL(s.cfg.ConsoleURL(chatPath(id)))
		}
	}

	if st.State == tray.Offline {
		for _, gone := range s.approvals.Forget() {
			_ = s.notifier.RemoveNotification(notificationPrefix + gone.RequestID)
		}
		for _, gone := range s.queued.Forget() {
			_ = s.notifier.RemoveNotification(queuedNotificationPrefix + gone.ApprovalID)
		}
		return
	}
	s.notifyQueued(st.Snapshot.Queued)
	s.notifyPending(st.Snapshot.Pending)
}

// notifyPending sends one notification per new chat question and withdraws
// those answered elsewhere.
func (s *shell) notifyPending(pending []activity.Approval) {
	added, removed := s.approvals.Update(pending)
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

// notifyQueued sends one notification per new unattended question and
// withdraws those answered elsewhere.
func (s *shell) notifyQueued(queued []activity.QueuedApproval) {
	added, removed := s.queued.Update(queued)
	for _, q := range added {
		title, subtitle, body := tray.QueuedNotificationText(q)
		err := s.notifier.SendNotificationWithActions(notifications.NotificationOptions{
			ID:         queuedNotificationPrefix + q.ApprovalID,
			Title:      title,
			Subtitle:   subtitle,
			Body:       body,
			CategoryID: tray.QueuedNotificationCategory,
			ThreadID:   q.SessionID,
			Data:       map[string]any{"approval_id": q.ApprovalID, "session_id": q.SessionID},
		})
		if err != nil {
			log.Printf("notify: %v", err)
		}
	}
	for _, q := range removed {
		_ = s.notifier.RemoveNotification(queuedNotificationPrefix + q.ApprovalID)
	}
}

// streamLoop listens to the server's event stream so a new approval shows
// at once instead of on the next poll. It reconnects after a drop.
func (s *shell) streamLoop() {
	for {
		err := s.client.Stream(context.Background(), func(e activity.Event) {
			// An unattended run's question and its review arrive as ops
			// events without a request id.
			if e.Category == "approval" || e.Category == "ops" || e.RequestID != "" {
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

// onSecondInstance acts on what a second launch was given; a bare launch
// brings the console window up.
func (s *shell) onSecondInstance(data application.SecondInstanceData) {
	go func() {
		if !s.handleArgs(data.Args) {
			s.showConsole("")
		}
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

// handleArgs acts on a tars:// link or a folder given on the command line
// and reports whether there was one.
func (s *shell) handleArgs(args []string) bool {
	if link, ok := deeplink.FromArgs(args); ok {
		s.handleLink(link)
		return true
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if info, err := os.Stat(arg); err == nil && info.IsDir() {
			s.proposeChatIn(arg, false)
			return true
		}
	}
	return false
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
	case deeplink.OpenWindow:
		s.openChatWindow(link.SessionID)
	case deeplink.NewChat:
		s.proposeChatIn(link.Dir, link.Isolate)
	}
}

// proposeChatIn asks before starting a chat in dir: a link or a dropped
// folder can come from anywhere, and a chat gets to work in its folder. In a
// git repository it also offers a worktree of the chat's own; a link that
// asked for one (isolate) makes that the default.
func (s *shell) proposeChatIn(dir string, isolate bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return
	}
	inRepo := deeplink.InGitRepository(abs)
	isolate = isolate && inRepo
	s.showConsole("")
	message := fmt.Sprintf("Start a new chat working in\n%s ?", abs)
	if isolate {
		message = fmt.Sprintf("Start a new chat in a worktree of its own, made from\n%s ?", abs)
	}
	dialog := s.app.Dialog.Question().
		SetTitle("Start a TARS chat").
		SetMessage(message)
	start := dialog.AddButton("Start chat")
	start.OnClick(func() { go s.startChatIn(abs, false) })
	defaultButton := start
	if inRepo {
		isolated := dialog.AddButton("Start isolated")
		isolated.OnClick(func() { go s.startChatIn(abs, true) })
		if isolate {
			defaultButton = isolated
		}
	}
	cancel := dialog.AddButton("Cancel")
	dialog.SetDefaultButton(defaultButton).SetCancelButton(cancel).AttachToWindow(s.window).Show()
}

func (s *shell) startChatIn(dir string, isolate bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, err := s.client.NewSessionIn(ctx, filepath.Base(dir), dir, isolate)
	if id == "" {
		s.showError("Could not start a chat", err)
		return
	}
	switch {
	case errors.Is(err, activity.ErrIsolateUnsupported):
		s.showError("The chat started in that folder, but not isolated", err)
	case err != nil:
		// The session exists but kept its default folder.
		s.showError("The chat started, but not in that folder", err)
	}
	s.showConsole(chatPath(id))
}

func (s *shell) startServer() {
	exe, _ := os.Executable()
	home, _ := os.UserHomeDir()
	bin, err := server.FindTARS(exe, exec.LookPath, server.InstallDirs(runtime.GOOS, home))
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
		if err != nil && plan.Fallback != nil && server.UnknownFlag(string(out)) {
			// A tars from before the flag: start what is installed.
			out, err = exec.Command(plan.Binary, plan.Fallback...).CombinedOutput()
		}
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

// checkUpdates is the tray's Check for updates…: the server first where the
// shell updates it, then the shell itself, whose updater shows its window.
func (s *shell) checkUpdates() {
	if serverupdate.Enabled(runtime.GOOS) {
		// A download and a restart can take minutes; keep the tray responsive.
		go s.updateServer(true)
	}
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

// updateLoop checks for releases on a timer: the server is updated when idle
// (see serverupdate), and a newer shell is announced, not installed, since
// installing it restarts the app.
func (s *shell) updateLoop() {
	timer := time.NewTimer(serverupdate.FirstCheckAfter)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
		case <-s.updateNow:
		}
		s.announceShellUpdate()
		next := s.updateServer(false)
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(next)
	}
}

func (s *shell) wakeUpdates() {
	select {
	case s.updateNow <- struct{}{}:
	default:
	}
}

// updateServer checks for a server release and installs it when the server
// is idle, returning when to check again. explicit is a click on Check for
// updates…, which reports errors in a dialog instead of the log.
func (s *shell) updateServer(explicit bool) time.Duration {
	s.updating.Lock()
	defer s.updating.Unlock()

	fail := func(title string, err error) time.Duration {
		if explicit {
			s.showError(title, err)
		} else {
			s.logOnce("server update", err)
		}
		return serverupdate.CheckEvery
	}
	exe, _ := os.Executable()
	home, _ := os.UserHomeDir()
	bin, err := server.FindTARS(exe, exec.LookPath, server.InstallDirs(runtime.GOOS, home))
	if err != nil {
		return fail("Could not find the TARS server to update", err)
	}
	resolved, err := filepath.EvalSymlinks(bin)
	if err != nil {
		resolved = bin
	}
	u, err := serverupdate.For(runtime.GOOS, bin, resolved, findBrew, s.cfg, runHidden)
	if err != nil {
		return fail("Could not update the TARS server", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), serverUpdateTimeout)
	defer cancel()
	check, err := u.Check(ctx)
	if err != nil {
		return fail("Could not check for a server update", err)
	}
	snap, activityErr := s.client.Activity(ctx)
	step := serverupdate.Decide(check, snap, activityErr)
	switch step {
	case serverupdate.WaitForIdle:
		if s.firstAnnouncement("server-wait", check.Latest) || explicit {
			s.notify(updateNotificationPrefix+"server-wait",
				fmt.Sprintf("TARS server %s is ready to install", check.Latest),
				"It installs once no chat is running and nothing waits on an approval.")
		}
	case serverupdate.ApplyNow:
		log.Printf("updating the server %s to %s", check.Current, check.Latest)
		res, err := u.Apply(ctx)
		if err != nil {
			return fail("Could not update the TARS server", err)
		}
		body := "The server restarted on the new version."
		if !res.Restarted {
			body = "Start the server again to run it."
			if res.Server == "not_running" {
				body = "It runs the next time the server starts."
			}
		}
		s.notify(updateNotificationPrefix+"server-done", fmt.Sprintf("TARS server updated to %s", res.Latest), body)
		s.kick()
	}
	return serverupdate.Next(step)
}

// announceShellUpdate tells the user, once per release, that a newer
// tars-desktop is available; clicking the notification installs it.
func (s *shell) announceShellUpdate() {
	if !s.updates {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	rel, err := s.app.Updater.Check(ctx)
	if err != nil {
		s.logOnce("desktop update check", err)
		return
	}
	if rel == nil || !s.firstAnnouncement("desktop", rel.Version) {
		return
	}
	s.notify(updateNotificationPrefix+"desktop",
		fmt.Sprintf("tars-desktop %s is available", rel.Version),
		"Click to install it, or choose Check for updates… in the tray menu.")
}

// firstAnnouncement records that what's notification for version is being
// sent and reports whether it is the first this run.
func (s *shell) firstAnnouncement(what, version string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.announced == nil {
		s.announced = map[string]string{}
	}
	if s.announced[what] == version {
		return false
	}
	s.announced[what] = version
	return true
}

func (s *shell) notify(id, title, body string) {
	if err := s.notifier.SendNotification(notifications.NotificationOptions{ID: id, Title: title, Body: body}); err != nil {
		log.Printf("notify: %v", err)
	}
}

// runHidden runs a tars command to completion without a console window.
// findBrew is the brew executable, or "": on PATH, else where Homebrew
// installs it (an app opened from Finder has launchd's PATH).
func findBrew() string {
	if path, err := exec.LookPath("brew"); err == nil {
		return path
	}
	for _, dir := range serverupdate.BrewDirs(runtime.GOOS) {
		candidate := filepath.Join(dir, "brew")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func runHidden(ctx context.Context, bin string, args, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	hideWindow(cmd)
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (s *shell) showError(title string, err error) {
	msg := "unknown error"
	if err != nil {
		msg = err.Error()
	}
	log.Printf("%s: %s", title, msg)
	s.app.Dialog.Error().SetTitle(title).SetMessage(msg).Show()
}
