package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/spf13/cobra"
)

// initiativeToolOptions are the flags the offline tuning commands share
// (tars#1003). They read the ledger and print; none of them talks to a
// running server or changes the config.
type initiativeToolOptions struct {
	configPath string
	ledgerPath string
}

func newInitiativeCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := &initiativeToolOptions{}
	cmd := &cobra.Command{
		Use:   "initiative",
		Short: "Offline tuning tools for the speak-first loop (export, scenarios, calibrate, adapt)",
		Long: "Reads the initiative ledger (<workspace>/_shared/initiative/ledger.jsonl) to build a\n" +
			"labeled dataset, turn corrections into evaluation scenarios, and suggest pacing and\n" +
			"jev threshold values. Nothing here changes the config; see docs/initiative-tuning.md.",
		SilenceUsage: true,
	}
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "config file path")
	cmd.PersistentFlags().StringVar(&opts.ledgerPath, "ledger", "", "ledger path (default <workspace>/_shared/initiative/ledger.jsonl)")
	cmd.AddCommand(newInitiativeExportCommand(opts, stdout, stderr))
	cmd.AddCommand(newInitiativeScenariosCommand(stdout, stderr))
	cmd.AddCommand(newInitiativeCalibrateCommand(opts, stdout, stderr))
	cmd.AddCommand(newInitiativeAdaptCommand(opts, stdout, stderr))
	return cmd
}

func newInitiativeExportCommand(opts *initiativeToolOptions, stdout, stderr io.Writer) *cobra.Command {
	var out, labelsPath, since string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write the ledger as a JSONL dataset with an empty label slot per tick",
		Long: "One line per tick: signals, decision, delivery, and the reply outcome joined in.\n" +
			"No user or assistant text is in the ledger, so none is in the export. --labels\n" +
			"merges the labels of an earlier export, so corrections survive a fresh export.",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			ledger, _, err := opts.resolve(stderr)
			if err != nil {
				return err
			}
			var cutoff time.Time
			if strings.TrimSpace(since) != "" {
				d, err := time.ParseDuration(since)
				if err != nil || d <= 0 {
					return fmt.Errorf("--since: want a positive duration such as 336h, got %q", since)
				}
				cutoff = time.Now().Add(-d)
			}
			labels := map[string]initiative.DatasetLabel{}
			if labelsPath != "" {
				prev, err := readDatasetFile(labelsPath)
				if err != nil {
					return err
				}
				labels = initiative.LabelsFrom(prev)
			}
			entries, err := initiative.ReadLedgerEntries(ledger)
			if err != nil {
				return fmt.Errorf("read %s: %w", ledger, err)
			}
			records := initiative.BuildDataset(entries, cutoff, labels)
			if err := writeInitiativeOutput(out, stdout, func(w io.Writer) error { return initiative.WriteDataset(w, records) }); err != nil {
				return err
			}
			labeled := 0
			for _, r := range records {
				if !r.Label.Empty() {
					labeled++
				}
			}
			fprintf(stderr, "exported %d records (%d labeled) from %s\n", len(records), labeled, ledger)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "-", "output file (- for stdout)")
	cmd.Flags().StringVar(&labelsPath, "labels", "", "earlier export whose labels to carry over")
	cmd.Flags().StringVar(&since, "since", "", "only ticks newer than this duration (e.g. 336h)")
	return cmd
}

func newInitiativeScenariosCommand(stdout, stderr io.Writer) *cobra.Command {
	var in, out string
	var all bool
	cmd := &cobra.Command{
		Use:   "scenarios",
		Short: "Turn labeled export records into Decide evaluation scenarios",
		Long: "Writes lines in the shape of internal/initiative/testdata/eval/decide.jsonl. Records\n" +
			"whose label sets intent or speak become scenarios expecting that correction; --all\n" +
			"also pins every other record's recorded decision. Scenarios the current policy\n" +
			"fails are listed on stderr.",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(in) == "" {
				return fmt.Errorf("--in is required")
			}
			records, err := readDatasetFile(in)
			if err != nil {
				return err
			}
			scenarios := initiative.ScenariosFromDataset(records, all)
			if err := writeInitiativeOutput(out, stdout, func(w io.Writer) error {
				enc := json.NewEncoder(w)
				enc.SetEscapeHTML(false)
				for _, s := range scenarios {
					if err := enc.Encode(s); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
			failing := 0
			for _, s := range scenarios {
				if d, err := s.Run(); err != nil {
					failing++
					fprintf(stderr, "  current policy fails %s: %v (decided %s/%s)\n", s.Name, err, d.Intent, d.Reason)
				}
			}
			fprintf(stderr, "%d scenarios from %d records; the current policy fails %d\n", len(scenarios), len(records), failing)
			return nil
		},
	}
	cmd.Flags().StringVar(&in, "in", "", "export file to read")
	cmd.Flags().StringVar(&out, "out", "-", "output file (- for stdout)")
	cmd.Flags().BoolVar(&all, "all", false, "also pin the recorded decision of unlabeled records")
	return cmd
}

func newInitiativeCalibrateCommand(opts *initiativeToolOptions, stdout, stderr io.Writer) *cobra.Command {
	var in string
	var minPerClass int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "calibrate",
		Short: "Suggest initiative.thresholds for the jev backend from labeled readings",
		Long: "Scores the configured initiative.thresholds against jev probabilities whose\n" +
			"label says what the user's words meant, and proposes the threshold with the best\n" +
			"balanced accuracy. Only the jev backend reports probabilities; the default llm\n" +
			"backend answers booleans and has nothing to calibrate. Prints, never writes config.",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(in) == "" {
				return fmt.Errorf("--in is required")
			}
			records, err := readDatasetFile(in)
			if err != nil {
				return err
			}
			cfg := opts.initiativeConfig(stderr)
			reports := initiative.CalibrateThresholds(records, cfg.Thresholds, minPerClass)
			if asJSON {
				return writeIndentedJSON(stdout, reports)
			}
			printCalibration(stdout, reports, minPerClass)
			return nil
		},
	}
	cmd.Flags().StringVar(&in, "in", "", "labeled export file to read")
	cmd.Flags().IntVar(&minPerClass, "min-per-class", 5, "labeled readings of each class needed to propose a threshold")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newInitiativeAdaptCommand(opts *initiativeToolOptions, stdout, stderr io.Writer) *cobra.Command {
	var window time.Duration
	var minOutcomes int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "adapt",
		Short: "Suggest daily_cap and cooldown from how often speaking first got a reply",
		Long: "Reads reply outcomes (live mode) over a window. Mostly ignored or asked to stop:\n" +
			"back off. Mostly replied and a limit actually held TARS back: lean in. The runtime\n" +
			"already doubles the cooldown per ignored speak in a row; this looks at the rate.\n" +
			"Prints, never writes config.",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			ledger, cfg, err := opts.resolve(stderr)
			if err != nil {
				return err
			}
			entries, err := initiative.ReadLedgerEntries(ledger)
			if err != nil {
				return fmt.Errorf("read %s: %w", ledger, err)
			}
			s := initiative.SuggestPacing(entries, time.Now(), initiative.PacingInput{
				DailyCap: cfg.DailyCap, Cooldown: cfg.Cooldown, Location: cfg.Location,
				Window: window, MinOutcomes: minOutcomes,
			})
			if asJSON {
				return writeIndentedJSON(stdout, s)
			}
			printPacing(stdout, s)
			return nil
		},
	}
	cmd.Flags().DurationVar(&window, "window", 14*24*time.Hour, "how far back outcomes count")
	cmd.Flags().IntVar(&minOutcomes, "min-outcomes", 10, "resolved outcomes needed to suggest anything")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

// resolve returns the ledger path and the initiative config. With --ledger
// the config is optional (defaults are used when it does not load); without
// it the config names the workspace, so it must load.
func (o *initiativeToolOptions) resolve(stderr io.Writer) (string, initiative.Config, error) {
	cfg, err := o.loadConfig()
	if strings.TrimSpace(o.ledgerPath) != "" {
		if err != nil {
			fprintf(stderr, "note: config not loaded (%v); using default initiative settings\n", err)
		}
		return o.ledgerPath, initiativeConfigFrom(cfg), nil
	}
	if err != nil {
		return "", initiative.Config{}, fmt.Errorf("%w (or pass --ledger)", err)
	}
	return filepath.Join(cfg.WorkspaceDir, "_shared", "initiative", "ledger.jsonl"), initiativeConfigFrom(cfg), nil
}

func (o *initiativeToolOptions) initiativeConfig(stderr io.Writer) initiative.Config {
	cfg, err := o.loadConfig()
	if err != nil {
		fprintf(stderr, "note: config not loaded (%v); comparing against the default thresholds\n", err)
	}
	return initiativeConfigFrom(cfg)
}

func (o *initiativeToolOptions) loadConfig() (config.Config, error) {
	path, err := resolveConfigPath(o.configPath, "")
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, fmt.Errorf("load %s: %w", path, err)
	}
	return cfg, nil
}

// initiativeConfigFrom maps the pacing and threshold fields the tuning
// tools read, the same way the server builds its runtime config, with the
// runtime's defaults for anything unset.
func initiativeConfigFrom(cfg config.Config) initiative.Config {
	c := cfg.Initiative
	loc := time.Local
	if tz := strings.TrimSpace(c.Timezone); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	return initiative.Config{
		Location: loc,
		DailyCap: c.DailyCap,
		Cooldown: initiativeReplyCooldown(c.Cooldown),
		Thresholds: initiative.Thresholds{
			QuietRequested: c.QuietRequestedThreshold,
			UserStrained:   c.UserStrainedThreshold,
			SpecialDay:     c.SpecialDayThreshold,
		},
	}.WithDefaults()
}

func readDatasetFile(path string) ([]initiative.DatasetRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	records, err := initiative.ReadDataset(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return records, nil
}

func writeInitiativeOutput(path string, stdout io.Writer, write func(io.Writer) error) error {
	if path == "" || path == "-" {
		return write(stdout)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeIndentedJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printCalibration(w io.Writer, reports []initiative.ThresholdReport, minPerClass int) {
	for _, r := range reports {
		fprintf(w, "%s: %d labeled readings (%d true), AUC %.2f\n", r.Question, r.Samples, r.Positives, r.AUC)
		fprintf(w, "  current   %.3f  balanced accuracy %.2f  (tp %d fp %d tn %d fn %d)\n",
			r.Current.Threshold, r.Current.BalancedAccuracy, r.Current.TP, r.Current.FP, r.Current.TN, r.Current.FN)
		if r.Candidate == nil {
			fprintf(w, "  candidate none: needs at least %d labeled readings of each class\n", minPerClass)
			continue
		}
		c := r.Candidate
		fprintf(w, "  candidate %.3f  balanced accuracy %.2f  (tp %d fp %d tn %d fn %d)\n",
			c.Threshold, c.BalancedAccuracy, c.TP, c.FP, c.TN, c.FN)
		if c.Threshold != r.Current.Threshold && c.BalancedAccuracy > r.Current.BalancedAccuracy {
			fprintf(w, "  to apply: initiative.thresholds.%s: %.3f\n", r.Question, c.Threshold)
		}
	}
}

func printPacing(w io.Writer, s initiative.PacingSuggestion) {
	fprintf(w, "outcomes: %d replied, %d ignored, %d stopped\n", s.Outcomes.Replied, s.Outcomes.Ignored, s.Outcomes.Stopped)
	fprintf(w, "daily cap reached on %d day(s); cooldown held a decision back %d time(s)\n", s.CapHitDays, s.CooldownBlocks)
	fprintf(w, "verdict: %s (%s)\n", s.Verdict, s.Reason)
	fprintf(w, "  initiative.daily_cap: %d -> %d\n", s.CurrentDailyCap, s.SuggestedDailyCap)
	fprintf(w, "  initiative.cooldown:  %s -> %s\n", s.CurrentCooldown, s.SuggestedCooldown)
}
