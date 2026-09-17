package computeruse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	// CuaDriverPathEnv overrides where the cua-driver binary is found.
	CuaDriverPathEnv     = "CUA_DRIVER_PATH"
	cuaDriverWaitDelay   = 3 * time.Second
	cuaDriverMaxElements = 2000
)

// FindCuaDriverPath resolves the binary: explicit config, then
// CUA_DRIVER_PATH, then PATH.
func FindCuaDriverPath(configured string) (string, error) {
	for _, candidate := range []string{strings.TrimSpace(configured), strings.TrimSpace(os.Getenv(CuaDriverPathEnv))} {
		if candidate == "" {
			continue
		}
		path, err := exec.LookPath(candidate)
		if err != nil {
			return "", fmt.Errorf("%w: cua-driver not found at %s", ErrDriverUnavailable, candidate)
		}
		return path, nil
	}
	path, err := exec.LookPath("cua-driver")
	if err != nil {
		return "", fmt.Errorf("%w: cua-driver not found in PATH; install it or set %s", ErrDriverUnavailable, CuaDriverPathEnv)
	}
	return path, nil
}

// CuaDriver drives a GUI window through the cua-driver CLI, one subprocess per
// tool call. The daemon holds the accessibility permissions; this process only
// speaks JSON to the CLI.
type CuaDriver struct {
	path           string
	timeout        time.Duration
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

// NewCuaDriver returns a driver that runs the binary at path, bounding every
// call by timeout.
func NewCuaDriver(path string, timeout time.Duration) *CuaDriver {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &CuaDriver{path: path, timeout: timeout, commandContext: exec.CommandContext}
}

var _ Driver = (*CuaDriver)(nil)

func buildCuaArgs(tool string, args map[string]any) ([]string, error) {
	if args == nil {
		args = map[string]any{}
	}
	body, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("computeruse: marshal %s args: %w", tool, err)
	}
	return []string{tool, string(body)}, nil
}

// call runs one cua-driver tool and returns raw stdout. A non-zero exit whose
// stderr mentions the daemon is mapped to ErrDriverUnavailable so the engine
// can report "unavailable" instead of "error".
func (d *CuaDriver) call(ctx context.Context, tool string, args map[string]any) ([]byte, error) {
	argv, err := buildCuaArgs(tool, args)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	cmd := d.commandContext(ctx, d.path, argv...)
	configureCuaDriverProcess(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("computeruse: cua-driver %s timed out after %s", tool, d.timeout)
	}
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "daemon") || strings.Contains(lower, "not running") || strings.Contains(lower, "permission") {
			return nil, fmt.Errorf("%w: %s", ErrDriverUnavailable, msg)
		}
		return nil, fmt.Errorf("computeruse: cua-driver %s failed: %s", tool, msg)
	}
	return stdout.Bytes(), nil
}

// payload unwraps {"structuredContent": {...}} when present. The CLI prints the
// bare payload today; the MCP surface wraps it, and both shapes are accepted.
func payload(raw []byte) (map[string]any, error) {
	var top map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &top); err != nil {
		return nil, fmt.Errorf("computeruse: decode cua-driver output: %w", err)
	}
	if sc, ok := top["structuredContent"].(map[string]any); ok {
		return sc, nil
	}
	if errText, ok := top["error"].(string); ok && errText != "" {
		return nil, cuaPayloadError(errText, "")
	}
	// A refused tool call exits 0 and prints a bare {"code":…,"suggestion":…}
	// envelope, which would otherwise read as an empty snapshot. Only treat it
	// as an error when no real content rode along with it.
	if code := asString(top["code"]); code != "" && !hasAnyKey(top, "elements", "windows", "effect") {
		return nil, cuaPayloadError(code, asString(top["suggestion"]))
	}
	return top, nil
}

func hasAnyKey(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

// cuaPayloadError renders a tool-level failure, routing a dead daemon or a
// refused permission to ErrDriverUnavailable so the engine reports
// "unavailable" rather than "error" — exit status alone cannot tell them apart,
// since a refused call still exits 0.
func cuaPayloadError(code, suggestion string) error {
	msg := code
	if suggestion != "" {
		msg = code + ": " + suggestion
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "daemon") || strings.Contains(lower, "not running") || strings.Contains(lower, "permission") {
		return fmt.Errorf("%w: %s", ErrDriverUnavailable, msg)
	}
	return fmt.Errorf("computeruse: cua-driver error: %s", msg)
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func parseWindowState(raw []byte) (Snapshot, error) {
	p, err := payload(raw)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Degraded: asString(p["degraded_reason"]), TotalElements: asInt(p["element_count"])}
	// total_element_count is the pre-truncation count; element_count reports the
	// same number when the walk was not bounded.
	if total := asInt(p["total_element_count"]); total > snap.TotalElements {
		snap.TotalElements = total
	}
	items, _ := p["elements"].([]any)
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		role := asString(m["role"])
		secure, _ := m["secure"].(bool)
		secure = secure || strings.Contains(strings.ToLower(role), "secure")
		el := Element{
			// Renumbered 1-based and sequential: the driver's own element_index
			// is 0-based and skips non-actionable nodes.
			Index:   len(snap.Elements) + 1,
			Token:   asString(m["element_token"]),
			Role:    role,
			Label:   asString(m["label"]),
			Enabled: true,
			Secure:  secure,
		}
		if enabled, ok := m["enabled"].(bool); ok {
			el.Enabled = enabled
		}
		if sel, ok := m["selected"].(bool); ok {
			el.Selected = &sel
		}
		if !secure {
			el.Value = asString(m["value"])
		}
		snap.Elements = append(snap.Elements, el)
	}
	if snap.TotalElements < len(snap.Elements) {
		snap.TotalElements = len(snap.Elements)
	}
	return snap, nil
}

func parseWindows(items []any) []Window {
	out := []Window{}
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, Window{PID: asInt(m["pid"]), WindowID: asInt(m["window_id"]), App: asString(m["app_name"]), Title: asString(m["title"])})
	}
	return out
}

func parseListWindows(raw []byte) ([]Window, error) {
	p, err := payload(raw)
	if err != nil {
		return nil, err
	}
	items, _ := p["windows"].([]any)
	// Highest z_index first so callers can take [0] as the frontmost. z_index is
	// null when WindowServer cannot report stacking order; those sort last and
	// keep their original relative order.
	type ranked struct {
		w Window
		z float64
	}
	rs := []ranked{}
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		z, ok := m["z_index"].(float64)
		if !ok {
			z = -1
		}
		rs = append(rs, ranked{w: parseWindows([]any{it})[0], z: z})
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].z > rs[j].z })
	out := make([]Window, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.w)
	}
	return out, nil
}

func parseEffect(raw []byte) (Effect, error) {
	p, err := payload(raw)
	if err != nil {
		return "", err
	}
	eff := Effect(asString(p["effect"]))
	if eff == "" {
		eff = EffectUnverifiable
	}
	return eff, nil
}

// --- Driver implementation ---

func (d *CuaDriver) Ping(ctx context.Context) error {
	_, err := d.call(ctx, "list_windows", map[string]any{"on_screen_only": true})
	return err
}

func (d *CuaDriver) ResolveWindow(ctx context.Context, app string) (Window, error) {
	app = strings.TrimSpace(app)
	if app != "" {
		args := map[string]any{"name": app}
		if strings.Contains(app, ".") {
			args = map[string]any{"bundle_id": app}
		}
		raw, err := d.call(ctx, "launch_app", args)
		if err != nil {
			return Window{}, err
		}
		p, err := payload(raw)
		if err != nil {
			return Window{}, err
		}
		wins, _ := p["windows"].([]any)
		parsed := parseWindows(wins)
		if len(parsed) == 0 {
			// App launched without a ready window yet; fall through to a pid-filtered list.
			pid := asInt(p["pid"])
			raw, err := d.call(ctx, "list_windows", map[string]any{"pid": pid})
			if err != nil {
				return Window{}, err
			}
			parsed, err = parseListWindows(raw)
			if err != nil {
				return Window{}, err
			}
		}
		if len(parsed) == 0 {
			return Window{}, fmt.Errorf("computeruse: app %q has no window", app)
		}
		w := parsed[0]
		if w.App == "" {
			w.App = asString(p["name"])
		}
		return w, nil
	}
	raw, err := d.call(ctx, "list_windows", map[string]any{"on_screen_only": true})
	if err != nil {
		return Window{}, err
	}
	wins, err := parseListWindows(raw)
	if err != nil {
		return Window{}, err
	}
	if len(wins) == 0 {
		return Window{}, errors.New("computeruse: no on-screen window")
	}
	return wins[0], nil
}

func (d *CuaDriver) Snapshot(ctx context.Context, w Window, opts SnapshotOpts) (Snapshot, error) {
	args := map[string]any{"pid": w.PID, "window_id": w.WindowID, "include_screenshot": false, "max_elements": cuaDriverMaxElements}
	if opts.Query != "" {
		args["query"] = opts.Query
	}
	if opts.MaxDepth > 0 {
		args["max_depth"] = opts.MaxDepth
	}
	raw, err := d.call(ctx, "get_window_state", args)
	if err != nil {
		return Snapshot{}, err
	}
	snap, err := parseWindowState(raw)
	if err != nil {
		return Snapshot{}, err
	}
	snap.Window = w
	return snap, nil
}

func (d *CuaDriver) action(ctx context.Context, tool string, args map[string]any) (Effect, error) {
	raw, err := d.call(ctx, tool, args)
	if err != nil {
		return "", err
	}
	return parseEffect(raw)
}

func (d *CuaDriver) Click(ctx context.Context, w Window, token string) (Effect, error) {
	return d.action(ctx, "click", map[string]any{"pid": w.PID, "element_token": token})
}

func (d *CuaDriver) SetValue(ctx context.Context, w Window, token, value string) (Effect, error) {
	return d.action(ctx, "set_value", map[string]any{"pid": w.PID, "element_token": token, "value": value})
}

func (d *CuaDriver) TypeText(ctx context.Context, w Window, token, text string) (Effect, error) {
	return d.action(ctx, "type_text", map[string]any{"pid": w.PID, "element_token": token, "text": text})
}

func (d *CuaDriver) PressKey(ctx context.Context, w Window, key string) (Effect, error) {
	return d.action(ctx, "press_key", map[string]any{"pid": w.PID, "window_id": w.WindowID, "key": key})
}

func (d *CuaDriver) Scroll(ctx context.Context, w Window, direction string) (Effect, error) {
	return d.action(ctx, "scroll", map[string]any{"pid": w.PID, "window_id": w.WindowID, "direction": direction})
}
