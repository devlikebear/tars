package computeruse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DriverCua and DriverAgentDesktop are the tools.computer_use.driver values.
	DriverCua          = "cua-driver"
	DriverAgentDesktop = "agent-desktop"

	// AgentDesktopPathEnv overrides where the agent-desktop binary is found.
	AgentDesktopPathEnv = "AGENT_DESKTOP_PATH"
	// agentDesktopSession scopes refs: a ref is only valid in the session that
	// took its snapshot, so every call of this process names the same one.
	agentDesktopSession = "tars-computer-use"
	// agentDesktopDeadlineMargin leaves room for the CLI to answer with the
	// partial tree it has before the process timeout kills it.
	agentDesktopDeadlineMargin = 2 * time.Second
)

// FindAgentDesktopPath resolves the binary: explicit config, then
// AGENT_DESKTOP_PATH, then PATH, then known install locations.
func FindAgentDesktopPath(configured string) (string, error) {
	for _, candidate := range []string{strings.TrimSpace(configured), strings.TrimSpace(os.Getenv(AgentDesktopPathEnv))} {
		if candidate == "" {
			continue
		}
		path, err := exec.LookPath(candidate)
		if err != nil {
			return "", fmt.Errorf("%w: agent-desktop not found at %s", ErrDriverUnavailable, candidate)
		}
		return path, nil
	}
	if path, err := exec.LookPath("agent-desktop"); err == nil {
		return path, nil
	}
	for _, candidate := range agentDesktopFallbacks() {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: agent-desktop not found in PATH; install it or set %s", ErrDriverUnavailable, AgentDesktopPathEnv)
}

// agentDesktopFallbacks is a variable so tests do not depend on what the host
// happens to have installed.
var agentDesktopFallbacks = defaultAgentDesktopFallbacks

func defaultAgentDesktopFallbacks() []string {
	out := []string{}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, filepath.Join(home, ".local", "bin", "agent-desktop"))
	}
	return append(out, "/opt/homebrew/bin/agent-desktop", "/usr/local/bin/agent-desktop")
}

// adTarget is what an action needs to know about a ref from its snapshot: the
// CLI has separate commands for what the Driver interface calls one action.
type adTarget struct {
	role    string
	actions map[string]bool
}

// AgentDesktopDriver drives a GUI window through the agent-desktop CLI, one
// subprocess per call. There is no daemon: the Accessibility grant belongs to
// the app that launched the process.
type AgentDesktopDriver struct {
	path           string
	configured     string
	timeout        time.Duration
	commandContext func(context.Context, string, ...string) *exec.Cmd

	mu      sync.Mutex
	targets map[string]adTarget
	// scrollers is the ref to scroll per window: the CLI scrolls an element,
	// the Driver interface a window.
	scrollers map[int]string
}

// NewAgentDesktopDriver returns a driver that runs the binary at path,
// bounding every call by timeout.
func NewAgentDesktopDriver(path string, timeout time.Duration) *AgentDesktopDriver {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &AgentDesktopDriver{path: path, timeout: timeout, commandContext: exec.CommandContext,
		targets: map[string]adTarget{}, scrollers: map[int]string{}}
}

// NewLazyAgentDesktopDriver looks the binary up on each call, so installing it
// needs no restart. A missing binary surfaces as ErrDriverUnavailable from Ping.
func NewLazyAgentDesktopDriver(configured string, timeout time.Duration) *AgentDesktopDriver {
	d := NewAgentDesktopDriver("", timeout)
	d.configured = configured
	return d
}

var _ Driver = (*AgentDesktopDriver)(nil)

// SetupHint is the guidance the engine shows when Ping fails.
func (d *AgentDesktopDriver) SetupHint() string {
	return "install agent-desktop (https://github.com/vercel-labs/agent-desktop) and grant Accessibility to the app that runs TARS"
}

type adEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *adError        `json:"error"`
}

type adError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// adRefused marks a call the CLI turned down before delivering anything
// because of what the element is, not because anything went wrong.
var adRefused = map[string]bool{"POLICY_DENIED": true, "ACTION_NOT_SUPPORTED": true}

// call runs one subcommand and returns its data. A failure exits non-zero but
// still prints the JSON envelope, so stdout is read before the exit status.
func (d *AgentDesktopDriver) call(ctx context.Context, command string, args ...string) (json.RawMessage, error) {
	path := d.path
	if path == "" {
		var err error
		if path, err = FindAgentDesktopPath(d.configured); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	// Options go first: everything after "--" is a positional.
	argv := append([]string{command, "--session", agentDesktopSession}, args...)
	cmd := d.commandContext(ctx, path, argv...)
	configureCuaDriverProcess(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("computeruse: agent-desktop %s timed out after %s", command, d.timeout)
	}
	var env adEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &env); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if runErr != nil {
			return nil, fmt.Errorf("computeruse: agent-desktop %s failed: %v: %s", command, runErr, msg)
		}
		return nil, fmt.Errorf("computeruse: decode agent-desktop %s output: %w", command, err)
	}
	if env.OK {
		return env.Data, nil
	}
	if env.Error == nil {
		return nil, fmt.Errorf("computeruse: agent-desktop %s failed without an error", command)
	}
	if strings.HasPrefix(env.Error.Code, "PERM") {
		return nil, fmt.Errorf("%w: %s: %s", ErrDriverUnavailable, env.Error.Code, env.Error.Message)
	}
	return nil, &adCallError{code: env.Error.Code, message: env.Error.Message}
}

type adCallError struct{ code, message string }

func (e *adCallError) Error() string {
	return fmt.Sprintf("computeruse: agent-desktop error: %s: %s", e.code, e.message)
}

// Ping proves the binary runs and holds the Accessibility grant, which a
// snapshot would otherwise only reveal on the first step.
func (d *AgentDesktopDriver) Ping(ctx context.Context) error {
	raw, err := d.call(ctx, "permissions")
	if err != nil {
		return err
	}
	var p struct {
		Accessibility struct {
			State string `json:"state"`
		} `json:"accessibility"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("computeruse: decode agent-desktop permissions: %w", err)
	}
	if p.Accessibility.State != "granted" {
		return fmt.Errorf("%w: agent-desktop accessibility permission is %q", ErrDriverUnavailable, p.Accessibility.State)
	}
	return nil
}

type adWindow struct {
	ID         string   `json:"id"`
	PID        int      `json:"pid"`
	App        string   `json:"app_name"`
	Title      string   `json:"title"`
	Accessible bool     `json:"accessible"`
	Visible    bool     `json:"visible"`
	Focused    bool     `json:"is_focused"`
	Bounds     adBounds `json:"bounds"`
}

type adBounds struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// listed reports a window that can be observed on screen. The CLI also lists
// an app's off-screen and helper surfaces, and strips too thin to hold a UI.
func (w adWindow) listed() bool {
	return w.Accessible && w.Visible && w.Bounds.Width >= minDrivableWindowEdge && w.Bounds.Height >= minDrivableWindowEdge
}

func (w adWindow) window() (Window, error) {
	id, err := parseADWindowID(w.ID)
	if err != nil {
		return Window{}, err
	}
	return Window{PID: w.PID, WindowID: id, App: w.App, Title: w.Title}, nil
}

// parseADWindowID turns "w-6758" into 6758, the only shape seen on macOS.
func parseADWindowID(id string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "w-"))
	if err != nil || !strings.HasPrefix(id, "w-") {
		return 0, fmt.Errorf("computeruse: unsupported agent-desktop window id %q", id)
	}
	return n, nil
}

func (d *AgentDesktopDriver) listWindows(ctx context.Context) ([]adWindow, error) {
	raw, err := d.call(ctx, "list-windows")
	if err != nil {
		return nil, err
	}
	var all []adWindow
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("computeruse: decode agent-desktop windows: %w", err)
	}
	out := make([]adWindow, 0, len(all))
	for _, w := range all {
		if w.listed() {
			out = append(out, w)
		}
	}
	return out, nil
}

// pickWindow prefers the focused window: the CLI gives no stacking order, so
// that is the only frontmost signal there is.
func pickWindow(wins []adWindow, match func(adWindow) bool) (adWindow, bool) {
	var first *adWindow
	for i := range wins {
		if !match(wins[i]) {
			continue
		}
		if wins[i].Focused {
			return wins[i], true
		}
		if first == nil {
			first = &wins[i]
		}
	}
	if first == nil {
		return adWindow{}, false
	}
	return *first, true
}

func (d *AgentDesktopDriver) ResolveWindow(ctx context.Context, app string) (Window, error) {
	app = strings.TrimSpace(app)
	wins, err := d.listWindows(ctx)
	if err != nil {
		return Window{}, err
	}
	if app == "" {
		w, ok := pickWindow(wins, func(w adWindow) bool { return w.Focused })
		if !ok {
			if len(wins) != 1 {
				return Window{}, errors.New("computeruse: no focused window; pass app explicitly")
			}
			w = wins[0]
		}
		return w.window()
	}
	if w, ok := pickWindow(wins, func(w adWindow) bool { return strings.EqualFold(strings.TrimSpace(w.App), app) }); ok {
		return w.window()
	}
	return d.launchWindow(ctx, app)
}

// launchWindow also attaches to a running app without focusing it, which is
// what makes "Calculator" work where the app reports its localized name.
func (d *AgentDesktopDriver) launchWindow(ctx context.Context, app string) (Window, error) {
	target := app
	if bundle, ok := appBundleAliases[strings.ToLower(app)]; ok {
		target = bundle
	}
	raw, err := d.call(ctx, "launch", "--", target)
	if err != nil {
		return Window{}, err
	}
	var launched struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(raw, &launched); err != nil {
		return Window{}, fmt.Errorf("computeruse: decode agent-desktop launch: %w", err)
	}
	// launch returns once the process runs, which can be before it has a window.
	for attempt := 0; attempt <= cuaLaunchPollAttempts; attempt++ {
		wins, err := d.listWindows(ctx)
		if err != nil {
			return Window{}, err
		}
		if w, ok := pickWindow(wins, func(w adWindow) bool { return w.PID == launched.PID }); ok {
			return w.window()
		}
		if err := sleepCtx(ctx, cuaLaunchPollInterval); err != nil {
			return Window{}, err
		}
	}
	return Window{}, fmt.Errorf("computeruse: app %q has no window after %s", app, cuaLaunchPollAttempts*cuaLaunchPollInterval)
}

// adRoleNames maps the CLI's normalized roles to the AX names the rest of the
// package reads: menu filtering, label adoption and Compatible all go by them.
var adRoleNames = map[string]string{
	"button": "AXButton", "cell": "AXCell", "checkbox": "AXCheckBox", "column": "AXColumn",
	"combobox": "AXComboBox", "group": "AXGroup", "handle": "AXHandle", "heading": "AXHeading",
	"image": "AXImage", "link": "AXLink", "list": "AXList", "menu": "AXMenu", "menubar": "AXMenuBar",
	"menubaritem": "AXMenuBarItem", "menubutton": "AXMenuButton", "menuitem": "AXMenuItem",
	"outline": "AXOutline", "popupbutton": "AXPopUpButton", "radiobutton": "AXRadioButton",
	"row": "AXRow", "scrollarea": "AXScrollArea", "scrollbar": "AXScrollBar",
	"searchfield": "AXSearchField", "slider": "AXSlider", "splitter": "AXSplitter",
	"statictext": "AXStaticText", "switch": "AXSwitch", "tab": "AXTab", "table": "AXTable",
	"tabgroup": "AXTabGroup", "textarea": "AXTextArea", "textfield": "AXTextField",
	"toolbar": "AXToolbar", "webarea": "AXWebArea", "window": "AXWindow",
}

func adRole(role string) string {
	key := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(role))
	if name, ok := adRoleNames[key]; ok {
		return name
	}
	if key == "" {
		return "AXUnknown"
	}
	return "AX" + strings.ToUpper(key[:1]) + key[1:]
}

type adNode struct {
	Ref      string   `json:"ref_id"`
	Role     string   `json:"role"`
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	States   []string `json:"states"`
	Actions  []string `json:"available_actions"`
	Children []adNode `json:"children"`
}

func (n adNode) has(state string) bool {
	for _, s := range n.States {
		if s == state {
			return true
		}
	}
	return false
}

type adSnapshot struct {
	Complete *bool  `json:"complete"`
	Tree     adNode `json:"tree"`
}

// adWalk flattens one tree into a Snapshot and the per-ref facts actions need.
type adWalk struct {
	snap     Snapshot
	seen     map[string]struct{}
	targets  map[string]adTarget
	scroller string
	// scrollerIsArea stops a scroll area losing to a list or bar found earlier.
	scrollerIsArea bool
}

func (a *adWalk) addText(role, text string) {
	if text == "" || len(a.snap.Texts) >= maxReadOnlyTexts {
		return
	}
	line := truncateRunes(fmt.Sprintf("%s = %q", role, text), maxReadOnlyTextRunes)
	if _, dup := a.seen[line]; dup {
		return
	}
	a.seen[line] = struct{}{}
	a.snap.Texts = append(a.snap.Texts, line)
}

// isElement decides which nodes the model can target. Static text with no
// action is read-only even though the CLI gives it a ref.
func (n adNode) isElement() bool {
	if n.Ref == "" {
		return false
	}
	return !strings.EqualFold(n.Role, "statictext") || len(n.Actions) > 0
}

func (a *adWalk) visit(n adNode, parent, depth int) {
	role := adRole(n.Role)
	// A menu subtree can hold recent file names and is never a click target.
	if IsMenuRole(role) {
		return
	}
	self := parent
	switch {
	case n.isElement():
		self = a.addElement(n, role, parent, depth)
	case depth > 0 && !n.has("secure"):
		// The root is skipped: its name is the window title, which RenderState
		// leaves out on purpose.
		text := n.Value
		if text == "" {
			text = n.Name
		}
		a.addText(role, text)
	}
	for _, c := range n.Children {
		a.visit(c, self, depth+1)
	}
}

func (a *adWalk) addElement(n adNode, role string, parent, depth int) int {
	actions := make(map[string]bool, len(n.Actions))
	for _, name := range n.Actions {
		actions[name] = true
	}
	secure := n.has("secure")
	el := Element{
		Index:       len(a.snap.Elements) + 1,
		Token:       n.Ref,
		Role:        role,
		Label:       n.Name,
		Enabled:     !n.has("disabled"),
		Secure:      secure,
		ParentIndex: parent,
		Depth:       depth,
	}
	switch {
	case secure:
		// Never read: the value of a secure field stays out of the snapshot.
	case actions["Toggle"] || strings.Contains(strings.ToLower(role), "radio"):
		// The value is the CLI's "0"/"1" for the same fact.
		checked := n.has("checked") || n.has("selected")
		el.Selected = &checked
	default:
		el.Value = n.Value
	}
	if el.Label == "" && labelAdoptingRoles[strings.ToLower(role)] {
		for _, c := range n.Children {
			if !strings.EqualFold(c.Role, "statictext") {
				continue
			}
			text := c.Name
			if text == "" {
				text = c.Value
			}
			if text != "" {
				if r := []rune(text); len(r) > maxAdoptedLabelRunes {
					text = string(r[:maxAdoptedLabelRunes])
				}
				el.Label, el.LabelAdopted = text, true
				break
			}
		}
	}
	a.snap.Elements = append(a.snap.Elements, el)
	a.targets[n.Ref] = adTarget{role: role, actions: actions}
	if isArea := role == "AXScrollArea"; actions["Scroll"] && (a.scroller == "" || (isArea && !a.scrollerIsArea)) {
		a.scroller, a.scrollerIsArea = n.Ref, isArea
	}
	return el.Index
}

func parseADSnapshot(raw json.RawMessage) (Snapshot, map[string]adTarget, string, error) {
	var parsed adSnapshot
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Snapshot{}, nil, "", fmt.Errorf("computeruse: decode agent-desktop snapshot: %w", err)
	}
	walk := adWalk{seen: map[string]struct{}{}, targets: map[string]adTarget{}}
	walk.visit(parsed.Tree, 0, 0)
	snap := walk.snap
	snap.TotalElements = len(snap.Elements)
	snap.Partial = parsed.Complete != nil && !*parsed.Complete
	if snap.Partial && len(snap.Elements) == 0 {
		snap.Degraded = "agent-desktop returned no elements before its deadline"
	}
	return snap, walk.targets, walk.scroller, nil
}

func (d *AgentDesktopDriver) Snapshot(ctx context.Context, w Window, opts SnapshotOpts) (Snapshot, error) {
	// -i is not used: it drops static text, the only place an action's outcome
	// shows in an app like Calculator. opts.Query has no equivalent here; the
	// caller just gets the whole window.
	args := []string{"--window-id", "w-" + strconv.Itoa(w.WindowID), "--compact"}
	if opts.MaxDepth > 0 {
		args = append(args, "--max-depth", strconv.Itoa(opts.MaxDepth))
	}
	// The default 3s observation deadline cuts large trees short, while the
	// step budget allows far more.
	if deadline := d.timeout - agentDesktopDeadlineMargin; deadline > 0 {
		args = append(args, "--timeout-ms", strconv.FormatInt(deadline.Milliseconds(), 10))
	}
	raw, err := d.call(ctx, "snapshot", args...)
	if err != nil {
		return Snapshot{}, err
	}
	snap, targets, scroller, err := parseADSnapshot(raw)
	if err != nil {
		return Snapshot{}, err
	}
	snap.Window = w
	d.mu.Lock()
	defer d.mu.Unlock()
	d.targets = targets
	d.scrollers[w.WindowID] = scroller
	return snap, nil
}

func (d *AgentDesktopDriver) target(token string) adTarget {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.targets[token]
}

// parseADEffect reads the CLI's own delivery report. delivered_unverified is
// the normal result of a click: the next snapshot is what judges it.
func parseADEffect(raw json.RawMessage) (Effect, error) {
	var p struct {
		Disposition struct {
			Delivery string `json:"delivery"`
		} `json:"disposition"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("computeruse: decode agent-desktop result: %w", err)
	}
	switch p.Disposition.Delivery {
	case "delivered_verified":
		return EffectConfirmed, nil
	case "not_delivered":
		// A success that delivered nothing: the element was already in that state.
		return EffectConfirmed, nil
	default:
		return EffectUnverifiable, nil
	}
}

func (d *AgentDesktopDriver) action(ctx context.Context, command string, args ...string) (Effect, error) {
	raw, err := d.call(ctx, command, args...)
	var callErr *adCallError
	if errors.As(err, &callErr) && adRefused[callErr.code] {
		return EffectRefused, nil
	}
	if err != nil {
		return "", err
	}
	return parseADEffect(raw)
}

func (d *AgentDesktopDriver) Click(ctx context.Context, _ Window, token string) (Effect, error) {
	return d.action(ctx, "click", "--", token)
}

func (d *AgentDesktopDriver) SetValue(ctx context.Context, _ Window, token, value string) (Effect, error) {
	t := d.target(token)
	role := strings.ToLower(t.role)
	switch {
	case t.actions["Toggle"] && value == "true":
		return d.action(ctx, "check", "--", token)
	case t.actions["Toggle"] && value == "false":
		return d.action(ctx, "uncheck", "--", token)
	case strings.Contains(role, "radio"):
		return d.action(ctx, "click", "--", token)
	case strings.Contains(role, "popup") || strings.Contains(role, "combobox"):
		return d.action(ctx, "select", "--", token, value)
	}
	return d.action(ctx, "set-value", "--", token, value)
}

// TypeText falls back to set-value on a field that offers no text insertion,
// which is how a secure field reports itself: `type` refuses it.
func (d *AgentDesktopDriver) TypeText(ctx context.Context, _ Window, token, text string) (Effect, error) {
	t := d.target(token)
	if t.actions["SetValue"] && !t.actions["TypeText"] {
		return d.action(ctx, "set-value", "--", token, text)
	}
	return d.action(ctx, "type", "--", token, text)
}

// PressKey targets the process by name: --window-id would demand that the
// window is focused, and this driver does not focus anything.
func (d *AgentDesktopDriver) PressKey(ctx context.Context, w Window, key string) (Effect, error) {
	return d.action(ctx, "press", "--app", w.App, "--", key)
}

func (d *AgentDesktopDriver) Scroll(ctx context.Context, w Window, direction string) (Effect, error) {
	d.mu.Lock()
	token := d.scrollers[w.WindowID]
	d.mu.Unlock()
	if token == "" {
		return EffectRefused, nil
	}
	return d.action(ctx, "scroll", "--direction", direction, "--", token)
}
