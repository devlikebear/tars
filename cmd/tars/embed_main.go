package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/localembed"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/spf13/cobra"
)

type embedOptions struct {
	configPath string
}

// embedManagerFactory builds the sidecar manager; tests swap it.
var embedManagerFactory = localembed.New

func newEmbedCommand(stdout io.Writer) *cobra.Command {
	opts := &embedOptions{}
	cmd := &cobra.Command{
		Use:   "embed",
		Short: "Manage the local embedding model for semantic memory (off by default)",
		Long: "Installs and runs a local embedding model (default " + config.DefaultLocalEmbedModel + ") for\n" +
			"semantic memory recall, as memory_embed_provider: local. Everything is stored in\n" +
			"<workspace>/_shared/local-embed; `tars serve` runs the model while it is enabled.",
		SilenceUsage: true,
	}
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "config file path")

	cmd.AddCommand(&cobra.Command{
		Use:          "status",
		Short:        "Show whether the local embedding model is enabled, installed and running",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, mgr, err := loadEmbedManager(opts)
			if err != nil {
				return err
			}
			printEmbedStatus(stdout, cfg, mgr.Status(cmd.Context()))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:          "install",
		Short:        "Create the Python environment and download the model (re-run to resume or repair)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _, mgr, err := loadEmbedManager(opts)
			if err != nil {
				return err
			}
			if err := mgr.Install(cmd.Context(), stdout); err != nil {
				return err
			}
			fprintln(stdout, "Installed. Enable it with `tars embed enable`.")
			return nil
		},
	})

	var removeAll bool
	remove := &cobra.Command{
		Use:          "remove",
		Short:        "Delete the downloaded model (--all also deletes the Python environment)",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, _, mgr, err := loadEmbedManager(opts)
			if err != nil {
				return err
			}
			if removeAll {
				if err := mgr.Uninstall(); err != nil {
					return err
				}
				fprintln(stdout, "Removed the local embedding model and its Python environment.")
				return nil
			}
			if err := mgr.RemoveModel(); err != nil {
				return err
			}
			fprintln(stdout, "Removed the local embedding model. `tars embed install` downloads it again.")
			return nil
		},
	}
	remove.Flags().BoolVar(&removeAll, "all", false, "also delete the Python environment")
	cmd.AddCommand(remove)

	cmd.AddCommand(&cobra.Command{
		Use:          "enable",
		Short:        "Turn on semantic memory with the local embedding model",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, configPath, mgr, err := loadEmbedManager(opts)
			if err != nil {
				return err
			}
			updates := map[string]any{
				"memory_semantic_enabled": true,
				"memory_embed_provider":   "local",
			}
			// Another provider's endpoint, model and size would not fit the
			// local model; clearing them lets the local defaults apply.
			if memory.NormalizeEmbedProvider(cfg.MemoryEmbedProvider) != "local" {
				updates["memory_embed_base_url"] = ""
				updates["memory_embed_model"] = ""
				updates["memory_embed_dimensions"] = config.DefaultLocalEmbedDimensions
			}
			if err := config.PatchYAML(configPath, updates); err != nil {
				return fmt.Errorf("update %s: %w", configPath, err)
			}
			fprintf(stdout, "Enabled local semantic memory in %s.\n", configPath)
			if !mgr.Status(cmd.Context()).ModelInstalled {
				fprintln(stdout, "The model is not installed yet: run `tars embed install`.")
			}
			fprintln(stdout, "Restart TARS to apply (`tars service stop && tars service start`, or restart `tars serve`).")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:          "disable",
		Short:        "Turn off semantic memory and stop the local embedding model (files are kept)",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, configPath, mgr, err := loadEmbedManager(opts)
			if err != nil {
				return err
			}
			if err := config.PatchYAML(configPath, map[string]any{"memory_semantic_enabled": false}); err != nil {
				return fmt.Errorf("update %s: %w", configPath, err)
			}
			mgr.Stop()
			fprintf(stdout, "Disabled semantic memory in %s. Memory recall uses keywords only.\n", configPath)
			fprintln(stdout, "Restart TARS to apply. `tars embed remove` deletes the downloaded model.")
			return nil
		},
	})
	return cmd
}

// loadEmbedManager loads the config and builds the manager for the model it
// names, or for the local defaults when another provider is configured.
func loadEmbedManager(opts *embedOptions) (config.Config, string, *localembed.Manager, error) {
	configPath, err := resolveConfigPath(opts.configPath, "")
	if err != nil {
		return config.Config{}, "", nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return config.Config{}, "", nil, fmt.Errorf("load %s: %w", configPath, err)
	}
	model, baseURL := config.DefaultLocalEmbedModel, config.DefaultLocalEmbedBaseURL
	if memory.NormalizeEmbedProvider(cfg.MemoryEmbedProvider) == "local" {
		model, baseURL = cfg.MemoryEmbedModel, cfg.MemoryEmbedBaseURL
	}
	mgr, err := embedManagerFactory(localembed.Options{
		Dir:     localembed.DirFor(cfg.WorkspaceDir),
		Model:   model,
		BaseURL: baseURL,
	})
	if err != nil {
		return config.Config{}, "", nil, err
	}
	return cfg, configPath, mgr, nil
}

func localEmbedEnabled(cfg config.Config) bool {
	return cfg.MemorySemanticEnabled && memory.NormalizeEmbedProvider(cfg.MemoryEmbedProvider) == "local"
}

func printEmbedStatus(w io.Writer, cfg config.Config, st localembed.Status) {
	yesNo := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}
	fprintf(w, "enabled:   %s\n", yesNo(localEmbedEnabled(cfg)))
	fprintf(w, "model:     %s\n", st.Model)
	fprintf(w, "installed: %s", yesNo(st.ModelInstalled))
	if st.ModelInstalled {
		fprintf(w, " (%s)", humanBytes(st.ModelBytes))
	}
	fprintln(w)
	fprintf(w, "python:    %s\n", map[bool]string{true: "ready", false: "not set up"}[st.EnvReady])
	running := yesNo(st.Running)
	if !st.Running && st.RunningModel != "" {
		running = fmt.Sprintf("no (port serves %s)", st.RunningModel)
	}
	fprintf(w, "running:   %s (%s)\n", running, st.BaseURL)
	if st.UVPath == "" && !st.EnvReady {
		fprintln(w, "uv:        not found — install it first (macOS: brew install uv)")
	}
	fprintf(w, "dir:       %s\n", st.Dir)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %sB", float64(n)/float64(div), strings.Split("K M G T", " ")[exp])
}
