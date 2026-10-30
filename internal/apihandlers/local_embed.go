package apihandlers

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/httpapi"
	"github.com/devlikebear/tars/internal/localembed"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/rs/zerolog"
)

const localEmbedLogLines = 30

// LocalEmbedOptions configures NewLocalEmbedHandler.
type LocalEmbedOptions struct {
	ConfigPath string
	// Running is the config the server started with: semantic memory is wired
	// at start-up, so a change on disk needs a restart to take effect.
	Running config.Config
	// Semantic is the semantic memory config the server started with.
	Semantic memory.SemanticConfig
	// NewManager overrides how the sidecar manager is built (tests).
	NewManager func(localembed.Options) (*localembed.Manager, error)
	Logger     zerolog.Logger
}

// localEmbedJob is the long action in flight or last finished. Installing and
// reindexing outlive a request, so the console polls this.
type localEmbedJob struct {
	Action  string   `json:"action"`
	Running bool     `json:"running"`
	Error   string   `json:"error,omitempty"`
	Log     []string `json:"log,omitempty"`
	Done    int      `json:"done,omitempty"`
	Total   int      `json:"total,omitempty"`
}

type localEmbedState struct {
	Enabled         bool              `json:"enabled"`
	RestartRequired bool              `json:"restart_required"`
	Status          localembed.Status `json:"status"`
	ReindexPending  int               `json:"reindex_pending"`
	Job             *localEmbedJob    `json:"job,omitempty"`
}

type localEmbedHandler struct {
	opts LocalEmbedOptions

	mu     sync.Mutex
	job    *localEmbedJob
	cancel context.CancelFunc
}

// NewLocalEmbedHandler serves /v1/admin/local-embed: the state of the local
// embedding model, and the actions on it.
func NewLocalEmbedHandler(opts LocalEmbedOptions) http.Handler {
	if opts.NewManager == nil {
		opts.NewManager = localembed.New
	}
	h := &localEmbedHandler{opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/admin/local-embed", func(w http.ResponseWriter, r *http.Request) {
		if !httpapi.RequireMethod(w, r, http.MethodGet) {
			return
		}
		cfg, mgr, ok := h.load(w)
		if ok {
			httpapi.WriteJSON(w, http.StatusOK, h.state(r.Context(), cfg, mgr))
		}
	})
	mux.HandleFunc("/v1/admin/local-embed/", func(w http.ResponseWriter, r *http.Request) {
		if !httpapi.RequireMethod(w, r, http.MethodPost) {
			return
		}
		h.action(w, r, strings.TrimPrefix(r.URL.Path, "/v1/admin/local-embed/"))
	})
	return mux
}

func (h *localEmbedHandler) load(w http.ResponseWriter) (config.Config, *localembed.Manager, bool) {
	cfg, err := config.Load(h.opts.ConfigPath)
	if err != nil {
		h.opts.Logger.Error().Err(err).Msg("load config for local embed failed")
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "load config failed")
		return cfg, nil, false
	}
	mgr, err := h.opts.NewManager(localembed.OptionsForConfig(cfg))
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return cfg, nil, false
	}
	return cfg, mgr, true
}

func (h *localEmbedHandler) state(ctx context.Context, cfg config.Config, mgr *localembed.Manager) localEmbedState {
	state := localEmbedState{
		Enabled:         localembed.Enabled(cfg),
		RestartRequired: localembed.Enabled(cfg) != localembed.Enabled(h.opts.Running),
		Status:          mgr.Status(ctx),
	}
	if h.reindexReady(cfg) {
		pending, err := h.service().ReindexPending()
		if err != nil {
			h.opts.Logger.Warn().Err(err).Msg("count reindex backlog failed")
		}
		state.ReindexPending = pending
	}
	h.mu.Lock()
	if h.job != nil {
		job := *h.job
		job.Log = append([]string(nil), h.job.Log...)
		state.Job = &job
	}
	h.mu.Unlock()
	return state
}

func (h *localEmbedHandler) reindexReady(cfg config.Config) bool {
	return localembed.Enabled(cfg) && localembed.Enabled(h.opts.Running)
}

func (h *localEmbedHandler) service() *memory.Service {
	return memory.NewService(h.opts.Running.WorkspaceDir, memory.ServiceOptions{Config: h.opts.Semantic})
}

func (h *localEmbedHandler) action(w http.ResponseWriter, r *http.Request, action string) {
	if action == "cancel" {
		h.mu.Lock()
		if h.cancel != nil {
			h.cancel()
		}
		h.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cfg, mgr, ok := h.load(w)
	if !ok {
		return
	}
	var err error
	switch action {
	case "install":
		// Installing while on (say, after a removal) brings the sidecar back
		// without waiting for a restart.
		start := localembed.Enabled(h.opts.Running) && localembed.Enabled(cfg)
		if !h.startJob(w, action, func(ctx context.Context, job *jobWriter) error {
			if err := mgr.Install(ctx, job); err != nil {
				return err
			}
			if start {
				return mgr.Start(ctx)
			}
			return nil
		}) {
			return
		}
		httpapi.WriteJSON(w, http.StatusAccepted, h.state(r.Context(), cfg, mgr))
		return
	case "reindex":
		if !h.reindexReady(cfg) {
			httpapi.WriteError(w, http.StatusConflict, "restart_required", "enable local embeddings and restart TARS first")
			return
		}
		if !mgr.Status(r.Context()).Running {
			httpapi.WriteError(w, http.StatusConflict, "not_running", "the local embedding model is not running")
			return
		}
		svc := h.service()
		if !h.startJob(w, action, func(ctx context.Context, job *jobWriter) error {
			_, err := svc.Reindex(ctx, job.progress)
			return err
		}) {
			return
		}
		httpapi.WriteJSON(w, http.StatusAccepted, h.state(r.Context(), cfg, mgr))
		return
	case "remove", "remove-all", "enable", "disable":
		if h.jobRunning() {
			httpapi.WriteError(w, http.StatusConflict, "busy", "another local embedding action is running")
			return
		}
	default:
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "unknown action")
		return
	}
	switch action {
	case "remove":
		err = mgr.RemoveModel()
	case "remove-all":
		err = mgr.Uninstall()
	case "enable":
		err = config.PatchYAML(h.opts.ConfigPath, localembed.EnableUpdates(cfg))
	case "disable":
		err = config.PatchYAML(h.opts.ConfigPath, localembed.DisableUpdates())
		mgr.Stop()
	}
	if err != nil {
		h.opts.Logger.Error().Err(err).Str("action", action).Msg("local embed action failed")
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if cfg, mgr, ok = h.load(w); ok {
		httpapi.WriteJSON(w, http.StatusOK, h.state(r.Context(), cfg, mgr))
	}
}

func (h *localEmbedHandler) jobRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.job != nil && h.job.Running
}

// startJob runs fn in the background unless a job is running. It is detached
// from the request: the console leaves and comes back. A job writes only to
// h.job, under h.mu.
func (h *localEmbedHandler) startJob(w http.ResponseWriter, action string, fn func(context.Context, *jobWriter) error) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.job != nil && h.job.Running {
		httpapi.WriteError(w, http.StatusConflict, "busy", "another local embedding action is running")
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &localEmbedJob{Action: action, Running: true}
	h.job = job
	h.cancel = cancel
	go func() {
		defer cancel()
		err := fn(ctx, &jobWriter{h: h, job: job})
		h.mu.Lock()
		defer h.mu.Unlock()
		job.Running = false
		h.cancel = nil
		if err != nil {
			job.Error = err.Error()
			// A killed subprocess reports "signal: killed", not ctx.Err().
			if ctx.Err() != nil {
				job.Error = "cancelled"
			}
			h.opts.Logger.Warn().Err(err).Str("action", action).Msg("local embed job failed")
		}
	}()
	return true
}

// jobWriter keeps the last lines of a job's output. Progress bars rewrite
// their line with \r, so that ends a line too.
type jobWriter struct {
	h       *localEmbedHandler
	job     *localEmbedJob
	partial string
}

func (j *jobWriter) Write(p []byte) (int, error) {
	j.h.mu.Lock()
	defer j.h.mu.Unlock()
	text := j.partial + string(p)
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' })
	j.partial = ""
	if len(lines) > 0 && !strings.HasSuffix(text, "\n") && !strings.HasSuffix(text, "\r") {
		j.partial = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 200 {
			line = line[:200]
		}
		j.job.Log = append(j.job.Log, line)
	}
	if extra := len(j.job.Log) - localEmbedLogLines; extra > 0 {
		j.job.Log = j.job.Log[extra:]
	}
	return len(p), nil
}

func (j *jobWriter) progress(done, total int) {
	j.h.mu.Lock()
	defer j.h.mu.Unlock()
	j.job.Done, j.job.Total = done, total
}
