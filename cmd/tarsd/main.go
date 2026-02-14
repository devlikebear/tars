package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/devlikebear/tarsncase/internal/cli"
	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/prompt"
	"github.com/devlikebear/tarsncase/internal/session"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	_ = godotenv.Load(".env")
	logger := zerolog.New(stderr).With().Timestamp().Str("component", "tarsd").Logger()
	cmd := newRootCmd(stdout, stderr, logger, time.Now)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		var ex *cli.ExitError
		if errors.As(err, &ex) {
			return ex.Code
		}

		logger.Error().Err(err).Msg("failed to parse flags")
		if cli.IsFlagError(err) {
			return 2
		}
		return 1
	}

	return 0
}

type options struct {
	ConfigPath        string
	Mode              string
	WorkspaceDir      string
	RunOnce           bool
	RunLoop           bool
	ServeAPI          bool
	APIAddr           string
	HeartbeatInterval time.Duration
	MaxHeartbeats     int
}

func newRootCmd(stdout, stderr io.Writer, logger zerolog.Logger, nowFn func() time.Time) *cobra.Command {
	opts := options{}

	cmd := &cobra.Command{
		Use:           "tarsd",
		Short:         "Main daemon for TARS",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if opts.RunOnce && opts.RunLoop {
				return &cli.ExitError{Code: 2, Err: fmt.Errorf("--run-once and --run-loop are mutually exclusive")}
			}

			cfg, err := config.Load(opts.ConfigPath)
			if err != nil {
				logger.Error().Err(err).Msg("failed to load config")
				return &cli.ExitError{Code: 1, Err: err}
			}

			if opts.Mode != "" {
				cfg.Mode = opts.Mode
			}
			if opts.WorkspaceDir != "" {
				cfg.WorkspaceDir = opts.WorkspaceDir
			}

			if err := memory.EnsureWorkspace(cfg.WorkspaceDir); err != nil {
				logger.Error().Err(err).Msg("failed to initialize workspace")
				return &cli.ExitError{Code: 1, Err: err}
			}
			if err := memory.AppendDailyLog(cfg.WorkspaceDir, nowFn(), "tarsd startup complete"); err != nil {
				logger.Error().Err(err).Msg("failed to write daily log")
				return &cli.ExitError{Code: 1, Err: err}
			}

			var ask heartbeat.AskFunc
			var llmClient llm.Client
			needLLM := opts.RunOnce || opts.RunLoop || opts.ServeAPI
			if needLLM {
				client, err := llm.NewProvider(llm.ProviderOptions{
					Provider:      cfg.LLMProvider,
					AuthMode:      cfg.LLMAuthMode,
					OAuthProvider: cfg.LLMOAuthProvider,
					BaseURL:       cfg.LLMBaseURL,
					Model:         cfg.LLMModel,
					APIKey:        cfg.LLMAPIKey,
				})
				if err != nil {
					logger.Error().Err(err).Msg("failed to initialize llm provider")
					return &cli.ExitError{Code: 1, Err: err}
				}
				llmClient = client
				ask = client.Ask
			}

			if opts.ServeAPI {
				store := session.NewStore(cfg.WorkspaceDir)

				mux := http.NewServeMux()
				heartbeatHandler := newHeartbeatAPIHandler(cfg.WorkspaceDir, nowFn, ask, logger)
				mux.Handle("/v1/heartbeat/", heartbeatHandler)
				chatHandler := newChatAPIHandler(cfg.WorkspaceDir, store, llmClient, logger)
				mux.Handle("/v1/chat", chatHandler)

				server := &http.Server{
					Addr:    opts.APIAddr,
					Handler: mux,
				}

				ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
				defer stop()

				go func() {
					<-ctx.Done()
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = server.Shutdown(shutdownCtx)
				}()

				logger.Info().Str("addr", opts.APIAddr).Msg("tarsd api server started")
				if _, err := fmt.Fprintf(stdout, "tarsd api serving on %s\n", opts.APIAddr); err != nil {
					return &cli.ExitError{Code: 1, Err: err}
				}
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error().Err(err).Msg("failed to serve api")
					return &cli.ExitError{Code: 1, Err: err}
				}
				return nil
			}

			if opts.RunOnce {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := heartbeat.RunOnceWithLLM(ctx, cfg.WorkspaceDir, nowFn(), ask); err != nil {
					logger.Error().Err(err).Msg("failed to run heartbeat once")
					return &cli.ExitError{Code: 1, Err: err}
				}
				logger.Info().Msg("heartbeat run-once complete")
			}
			if opts.RunLoop {
				count, err := heartbeat.RunLoopWithLLM(
					context.Background(),
					cfg.WorkspaceDir,
					opts.HeartbeatInterval,
					opts.MaxHeartbeats,
					nowFn,
					ask,
				)
				if err != nil {
					logger.Error().Err(err).Msg("failed to run heartbeat loop")
					return &cli.ExitError{Code: 1, Err: err}
				}
				logger.Info().Int("heartbeat_count", count).Msg("heartbeat run-loop complete")
			}

			logger.Info().
				Str("mode", cfg.Mode).
				Str("workspace_dir", cfg.WorkspaceDir).
				Msg("tarsd startup complete")

			fmt.Fprintf(stdout, "tarsd starting in %s mode\n", cfg.Mode)
			return nil
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "path to config file")
	cmd.Flags().StringVar(&opts.Mode, "mode", "", "runtime mode override")
	cmd.Flags().StringVar(&opts.WorkspaceDir, "workspace-dir", "", "workspace directory override")
	cmd.Flags().BoolVar(&opts.RunOnce, "run-once", false, "run heartbeat once and exit")
	cmd.Flags().BoolVar(&opts.RunLoop, "run-loop", false, "run heartbeat loop")
	cmd.Flags().BoolVar(&opts.ServeAPI, "serve-api", false, "serve tarsd http api")
	cmd.Flags().StringVar(&opts.APIAddr, "api-addr", "127.0.0.1:8080", "http api listen address")
	cmd.Flags().DurationVar(&opts.HeartbeatInterval, "heartbeat-interval", 30*time.Minute, "heartbeat interval (e.g. 30m, 5s)")
	cmd.Flags().IntVar(&opts.MaxHeartbeats, "max-heartbeats", 0, "maximum heartbeat count in loop (0 means unlimited)")

	return cmd
}

func newHeartbeatAPIHandler(workspaceDir string, nowFn func() time.Time, ask heartbeat.AskFunc, logger zerolog.Logger) http.Handler {
	var mu sync.Mutex
	runHeartbeat := func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return heartbeat.RunOnceWithLLMResult(callCtx, workspaceDir, nowFn(), ask)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/heartbeat/run-once", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		response, err := runHeartbeat(r.Context())
		if err != nil {
			logger.Error().Err(err).Msg("heartbeat run-once api failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"response": response})
	})

	return mux
}

func newChatAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if strings.TrimSpace(req.Message) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
			return
		}

		// Resolve or create session
		sessionID := req.SessionID
		if sessionID == "" {
			sess, err := store.Create("chat")
			if err != nil {
				logger.Error().Err(err).Msg("create session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create session failed"})
				return
			}
			sessionID = sess.ID
		} else {
			if _, err := store.Get(sessionID); err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
		}

		transcriptPath := store.TranscriptPath(sessionID)

		// Build system prompt
		systemPrompt := prompt.Build(prompt.BuildOptions{WorkspaceDir: workspaceDir})

		// Load history
		history, err := session.LoadHistory(transcriptPath, 120000)
		if err != nil {
			logger.Error().Err(err).Msg("load history failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load history failed"})
			return
		}

		// Append user message to transcript
		now := time.Now().UTC()
		userMsg := session.Message{Role: "user", Content: req.Message, Timestamp: now}
		if err := session.AppendMessage(transcriptPath, userMsg); err != nil {
			logger.Error().Err(err).Msg("append user message failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save message failed"})
			return
		}

		// Build messages array for LLM
		var llmMessages []llm.ChatMessage
		llmMessages = append(llmMessages, llm.ChatMessage{Role: "system", Content: systemPrompt})
		for _, m := range history {
			llmMessages = append(llmMessages, llm.ChatMessage{Role: m.Role, Content: m.Content})
		}
		llmMessages = append(llmMessages, llm.ChatMessage{Role: "user", Content: req.Message})

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)

		// Helper to send SSE event
		sendSSE := func(data any) {
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			if flusher != nil {
				flusher.Flush()
			}
		}

		// Call LLM with streaming
		chatResp, err := client.Chat(r.Context(), llmMessages, llm.ChatOptions{
			OnDelta: func(text string) {
				sendSSE(map[string]string{"type": "delta", "text": text})
			},
		})
		if err != nil {
			sendSSE(map[string]string{"type": "error", "error": err.Error()})
			return
		}

		// Append assistant message to transcript
		assistantMsg := session.Message{Role: "assistant", Content: chatResp.Message.Content, Timestamp: time.Now().UTC()}
		if err := session.AppendMessage(transcriptPath, assistantMsg); err != nil {
			logger.Error().Err(err).Msg("append assistant message failed")
		}

		// Send done event
		sendSSE(map[string]any{
			"type":       "done",
			"session_id": sessionID,
			"usage": map[string]int{
				"input_tokens":  chatResp.Usage.InputTokens,
				"output_tokens": chatResp.Usage.OutputTokens,
			},
		})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
