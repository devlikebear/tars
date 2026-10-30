// Package localembed installs and runs the embedding sidecar behind
// memory_embed_provider: local — a small Python server that serves a
// Hugging Face sentence-transformers model (Kakao's kanana-2 Korean
// embeddings by default) over the OpenAI-compatible /v1/embeddings API on a
// loopback port.
//
// Everything lives under one directory, <workspace>/_shared/local-embed:
// the Python environment (made with uv), the downloaded model (its own
// HF_HOME, so removing it never touches the user's global Hugging Face
// cache), the server script and its log. TARS never runs the model
// in-process.
package localembed

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed assets/server.py assets/download.py
var assets embed.FS

// pythonVersion and requirements are what the sidecar was verified with
// (macOS arm64, MPS). Changing them reinstalls the environment on the next
// Install.
const pythonVersion = "3.12"

var requirements = []string{
	"torch==2.15.0",
	"transformers==5.22.0",
	"sentence-transformers==6.2.0",
	"huggingface-hub==2.4.0",
	"fastapi==0.162.0",
	"uvicorn==0.54.0",
}

var (
	// ErrUVMissing means uv, used to build the Python environment, is not
	// installed.
	ErrUVMissing = errors.New("uv is not installed (https://docs.astral.sh/uv/ — on macOS: brew install uv)")
	// ErrNotInstalled means Start was called before Install.
	ErrNotInstalled = errors.New("local embedding model is not installed; run `tars embed install`")
)

const (
	healthTimeout = 3 * time.Second
	startTimeout  = 3 * time.Minute
	pollInterval  = 500 * time.Millisecond
)

// Runner runs one install step to completion, streaming its output to out.
type Runner func(ctx context.Context, name string, args, env []string, out io.Writer) error

// ExecRunner runs a step as a subprocess.
func ExecRunner(ctx context.Context, name string, args, env []string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// Options configures a Manager.
type Options struct {
	// Dir holds everything the sidecar needs; normally
	// <workspace>/_shared/local-embed (see DirFor).
	Dir string
	// Model is the Hugging Face model id, e.g. kakaocorp/kanana-2-1.3b-embedding.
	Model string
	// BaseURL is memory_embed_base_url, e.g. http://127.0.0.1:43181/v1. Its
	// host must be loopback: the sidecar serves without authentication.
	BaseURL string
	// UVPath overrides the uv lookup.
	UVPath string
	// Runner overrides how install steps run (tests).
	Runner Runner
	// HTTPClient is used for health checks.
	HTTPClient *http.Client
}

// Manager installs, removes, starts and stops the sidecar.
type Manager struct {
	dir     string
	model   string
	baseURL string
	host    string
	port    int
	uvPath  string
	run     Runner
	client  *http.Client

	// python overrides the environment's interpreter (tests).
	python string

	mu  sync.Mutex
	cmd *exec.Cmd
}

// DirFor is the sidecar directory inside a workspace.
func DirFor(workspaceDir string) string {
	return filepath.Join(workspaceDir, "_shared", "local-embed")
}

// New validates opts and returns a Manager.
func New(opts Options) (*Manager, error) {
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		return nil, errors.New("local embed dir is empty")
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" || strings.Contains(model, "..") || strings.Count(model, "/") != 1 {
		return nil, fmt.Errorf("local embed model %q must be a Hugging Face id like org/name", model)
	}
	host, port, err := loopbackHostPort(opts.BaseURL)
	if err != nil {
		return nil, err
	}
	run := opts.Runner
	if run == nil {
		run = ExecRunner
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: healthTimeout}
	}
	return &Manager{
		dir:     dir,
		model:   model,
		baseURL: strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"),
		host:    host,
		port:    port,
		uvPath:  strings.TrimSpace(opts.UVPath),
		run:     run,
		client:  client,
	}, nil
}

func loopbackHostPort(raw string) (string, int, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "http" || u.Port() == "" {
		return "", 0, fmt.Errorf("local embed base_url %q must be http://<loopback>:<port>/v1", raw)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", 0, fmt.Errorf("local embed base_url %q must point at a loopback address", raw)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("local embed base_url %q has an invalid port", raw)
	}
	return host, port, nil
}

func (m *Manager) venvDir() string    { return filepath.Join(m.dir, "venv") }
func (m *Manager) hfHome() string     { return filepath.Join(m.dir, "hf") }
func (m *Manager) appDir() string     { return filepath.Join(m.dir, "app") }
func (m *Manager) pidPath() string    { return filepath.Join(m.dir, "server.pid") }
func (m *Manager) markerPath() string { return filepath.Join(m.venvDir(), ".tars-requirements") }

// LogPath is the sidecar's log file.
func (m *Manager) LogPath() string { return filepath.Join(m.dir, "server.log") }

func (m *Manager) modelDir() string {
	return filepath.Join(m.hfHome(), "hub", "models--"+strings.ReplaceAll(m.model, "/", "--"))
}

func (m *Manager) pythonPath() string {
	if m.python != "" {
		return m.python
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(m.venvDir(), "Scripts", "python.exe")
	}
	return filepath.Join(m.venvDir(), "bin", "python")
}

// FindUV returns the uv binary: the configured path, PATH, then the usual
// install locations (a launchd-started server has a minimal PATH).
func (m *Manager) FindUV() string {
	if m.uvPath != "" {
		if isFile(m.uvPath) {
			return m.uvPath
		}
		return ""
	}
	if path, err := exec.LookPath("uv"); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	for _, candidate := range []string{
		"/opt/homebrew/bin/uv",
		"/usr/local/bin/uv",
		filepath.Join(home, ".local", "bin", "uv"),
		filepath.Join(home, ".cargo", "bin", "uv"),
	} {
		if isFile(candidate) {
			return candidate
		}
	}
	return ""
}

// Status is what is installed and whether the sidecar answers.
type Status struct {
	Model          string `json:"model"`
	BaseURL        string `json:"base_url"`
	Dir            string `json:"dir"`
	UVPath         string `json:"uv_path,omitempty"`
	EnvReady       bool   `json:"env_ready"`
	ModelInstalled bool   `json:"model_installed"`
	// ModelBytes is the disk the downloaded model(s) take: the whole private
	// HF_HOME.
	ModelBytes int64 `json:"model_bytes"`
	Running    bool  `json:"running"`
	// RunningModel is the model a server on the port reports; it differs
	// from Model when another model (or another program) holds the port.
	RunningModel string `json:"running_model,omitempty"`
}

// Status inspects the directory and probes the port.
func (m *Manager) Status(ctx context.Context) Status {
	st := Status{
		Model:          m.model,
		BaseURL:        m.baseURL,
		Dir:            m.dir,
		UVPath:         m.FindUV(),
		EnvReady:       m.envReady(),
		ModelInstalled: m.modelInstalled(),
		ModelBytes:     dirSize(m.hfHome()),
	}
	if model, ok := m.health(ctx); ok {
		st.RunningModel = model
		st.Running = model == m.model
	}
	return st
}

func (m *Manager) envReady() bool {
	raw, err := os.ReadFile(m.markerPath())
	return err == nil && strings.TrimSpace(string(raw)) == requirementsKey() && isFile(m.pythonPath())
}

func requirementsKey() string {
	return pythonVersion + " " + strings.Join(requirements, " ")
}

func (m *Manager) modelInstalled() bool {
	matches, _ := filepath.Glob(filepath.Join(m.modelDir(), "snapshots", "*", "*.safetensors"))
	return len(matches) > 0
}

// Install builds the Python environment (skipped when it already matches the
// pinned requirements), writes the server script and downloads the model.
// Re-running it resumes an interrupted download.
func (m *Manager) Install(ctx context.Context, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if !m.envReady() {
		uv := m.FindUV()
		if uv == "" {
			return ErrUVMissing
		}
		if err := os.MkdirAll(m.dir, 0o755); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "==> creating Python %s environment in %s\n", pythonVersion, m.venvDir())
		if err := m.run(ctx, uv, []string{"venv", "--allow-existing", "--python", pythonVersion, m.venvDir()}, nil, out); err != nil {
			return fmt.Errorf("create python environment: %w", err)
		}
		_, _ = fmt.Fprintln(out, "==> installing packages")
		args := append([]string{"pip", "install", "--python", m.pythonPath()}, requirements...)
		if err := m.run(ctx, uv, args, nil, out); err != nil {
			return fmt.Errorf("install python packages: %w", err)
		}
		if err := os.MkdirAll(m.venvDir(), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(m.markerPath(), []byte(requirementsKey()+"\n"), 0o644); err != nil {
			return err
		}
	}
	if err := m.writeAssets(); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "==> downloading %s\n", m.model)
	if err := m.run(ctx, m.pythonPath(), []string{"-I", filepath.Join(m.appDir(), "download.py"), m.model}, m.env(false), out); err != nil {
		return fmt.Errorf("download model: %w", err)
	}
	if !m.modelInstalled() {
		return fmt.Errorf("download finished but no weights were found in %s", m.modelDir())
	}
	return nil
}

func (m *Manager) writeAssets() error {
	if err := os.MkdirAll(m.appDir(), 0o755); err != nil {
		return err
	}
	for _, name := range []string{"server.py", "download.py"} {
		data, err := assets.ReadFile("assets/" + name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(m.appDir(), name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) env(offline bool) []string {
	env := []string{
		"HF_HOME=" + m.hfHome(),
		"HF_HUB_DISABLE_TELEMETRY=1",
		"TARS_EMBED_MODEL=" + m.model,
		"TARS_EMBED_HOST=" + m.host,
		"TARS_EMBED_PORT=" + strconv.Itoa(m.port),
	}
	if offline {
		// The server only loads what Install downloaded; it never reaches the
		// network at run time.
		env = append(env, "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1")
	}
	return env
}

// RemoveModel stops the sidecar and deletes the downloaded model, keeping
// the Python environment so a reinstall only downloads again. It deletes the
// whole private HF_HOME, not just the model's folder: newer Hugging Face
// clients keep large weights in content-addressed blobs (hub/blobs/xx/...)
// outside it, and nothing else uses this HF_HOME.
func (m *Manager) RemoveModel() error {
	m.Stop()
	return os.RemoveAll(m.hfHome())
}

// Uninstall stops the sidecar and deletes the whole directory.
func (m *Manager) Uninstall() error {
	m.Stop()
	return os.RemoveAll(m.dir)
}

// Start runs the sidecar and waits until it answers. A sidecar already
// serving the configured model is adopted as is.
func (m *Manager) Start(ctx context.Context) error {
	if model, ok := m.health(ctx); ok {
		if model == m.model {
			return nil
		}
		// Another model on our port: replace it only if it is ours.
		if !m.stopRecorded() {
			return fmt.Errorf("port %d is serving %q, not %q", m.port, model, m.model)
		}
	}
	if !m.envReady() || !m.modelInstalled() {
		return ErrNotInstalled
	}
	if err := m.writeAssets(); err != nil {
		return err
	}
	logFile, err := os.OpenFile(m.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(m.pythonPath(), "-I", filepath.Join(m.appDir(), "server.py"))
	cmd.Env = append(os.Environ(), m.env(true)...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start local embedding server: %w", err)
	}
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		_ = logFile.Close()
	}()
	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()
	_ = os.WriteFile(m.pidPath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)

	deadline := time.NewTimer(startTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case err := <-exited:
			m.clearProcess(cmd)
			return fmt.Errorf("local embedding server exited during start (%v); see %s", err, m.LogPath())
		case <-ctx.Done():
			m.Stop()
			return ctx.Err()
		case <-deadline.C:
			m.Stop()
			return fmt.Errorf("local embedding server did not answer within %s; see %s", startTimeout, m.LogPath())
		case <-tick.C:
			if model, ok := m.health(ctx); ok && model == m.model {
				return nil
			}
		}
	}
}

func (m *Manager) clearProcess(cmd *exec.Cmd) {
	m.mu.Lock()
	if m.cmd == cmd {
		m.cmd = nil
	}
	m.mu.Unlock()
	_ = os.Remove(m.pidPath())
}

// Stop ends the sidecar this or an earlier TARS process started.
func (m *Manager) Stop() {
	m.mu.Lock()
	cmd := m.cmd
	m.cmd = nil
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	m.stopRecorded()
}

// stopRecorded kills the process named in the pid file, if any, and reports
// whether there was one.
func (m *Manager) stopRecorded() bool {
	raw, err := os.ReadFile(m.pidPath())
	if err != nil {
		return false
	}
	_ = os.Remove(m.pidPath())
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return false
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
	return true
}

// health asks the server on the port which model it serves.
func (m *Manager) health(ctx context.Context) (string, bool) {
	root := strings.TrimSuffix(m.baseURL, "/v1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/health", nil)
	if err != nil {
		return "", false
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", false
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		OK    bool   `json:"ok"`
		Model string `json:"model"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&body) != nil || !body.OK {
		return "", false
	}
	return body.Model, true
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}
