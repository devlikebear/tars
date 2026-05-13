package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// skillInstallerFactory builds an Installer for a given workspace dir.
// Tests inject their own factory so the hub source can be pointed at
// httptest servers; production code uses newSkillInstaller.
type skillInstallerFactory func(workspaceDir string) *skillhub.Installer

func newSkillCommand(stdout, stderr io.Writer) *cobra.Command {
	return newSkillCommandWithFactory(stdout, stderr, newSkillInstaller)
}

func newSkillCommandWithFactory(stdout, stderr io.Writer, factory skillInstallerFactory) *cobra.Command {
	return newHubResourceCommand(hubResourceSpec{
		Use:        "skill",
		Short:      "Manage skills from the TARS Hub or external skill hubs",
		Noun:       "skill",
		PluralNoun: "skills",

		Search: func(ctx context.Context, stdout io.Writer, query, from string) error {
			inst := factory("")
			results, err := inst.SearchAllSkills(ctx, query, from)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			if len(results) == 0 {
				fmt.Fprintln(stdout, "No skills found.")
				return nil
			}
			for _, r := range results {
				entry := r.Entry
				invocable := ""
				if entry.UserInvocable {
					invocable = " [invocable]"
				}
				tags := ""
				if len(entry.Tags) > 0 {
					tags = " (" + strings.Join(entry.Tags, ", ") + ")"
				}
				fprintf(stdout, "  [%s] %s@%s%s%s\n    %s\n",
					r.SourceID, entry.Name, entry.Version, invocable, tags, entry.Description)
			}
			fprintf(stdout, "\n%d skill(s) found.\n", len(results))
			return nil
		},

		Install: func(ctx context.Context, stdout, stderr io.Writer, workspaceDir, name string, opts HubInstallOptions) error {
			inst := factory(workspaceDir)
			ref := buildSkillRef(opts.From, name)

			format := strings.ToLower(strings.TrimSpace(opts.Format))
			if format == "" {
				format = "text"
			}
			if format != "text" && format != "json" {
				return fmt.Errorf("--format must be text or json, got %q", opts.Format)
			}

			// In dry-run mode the preview is the deliverable. The
			// OnPreview hook does the rendering so we don't print it
			// twice when --dry-run is combined with confirm flows.
			renderPreview := func(p *skillhub.DryRunResult) {
				if format == "json" {
					renderDryRunJSON(stdout, p)
					return
				}
				renderDryRunText(stdout, p, opts.DryRun)
			}

			// Non-TTY callers must use --yes or --dry-run; otherwise the
			// prompt cannot complete and we'd hang or silently abort.
			if !opts.DryRun && !opts.Yes && !stdinIsTerminal() {
				return fmt.Errorf("non-interactive shell cannot confirm external-hub install of %q; re-run with --yes or --dry-run", name)
			}

			confirm := func(p *skillhub.DryRunResult) (bool, error) {
				// Preview already rendered via OnPreview.
				fprint(stdout, "\nProceed with install? [y/N] ")
				reader := bufio.NewReader(os.Stdin)
				resp, err := reader.ReadString('\n')
				if err != nil && err != io.EOF {
					return false, err
				}
				resp = strings.TrimSpace(strings.ToLower(resp))
				return resp == "y" || resp == "yes", nil
			}

			result, err := inst.InstallWithOptions(ctx, ref, skillhub.InstallOptions{
				Yes:       opts.Yes,
				Confirm:   confirm,
				DryRun:    opts.DryRun,
				OnPreview: renderPreview,
			})
			if err != nil {
				if err == skillhub.ErrInstallAborted {
					fprintln(stderr, "Install aborted.")
					return nil
				}
				return fmt.Errorf("install %q: %w", ref, err)
			}
			if opts.DryRun {
				// Preview already printed via OnPreview; nothing else to do.
				return nil
			}
			fprintf(stdout, "Installed skill %q to %s/skills/%s\n", name, workspaceDir, name)
			if result.RequiresPlugin != "" {
				fprintf(stderr, "⚠ This skill requires plugin %q. Install it with: tars plugin install %s\n", result.RequiresPlugin, result.RequiresPlugin)
			}
			return nil
		},

		Uninstall: func(stdout, _ io.Writer, workspaceDir, name string) error {
			inst := factory(workspaceDir)
			if err := inst.Uninstall(name); err != nil {
				return fmt.Errorf("uninstall %q: %w", name, err)
			}
			fmt.Fprintf(stdout, "Uninstalled skill %q\n", name)
			return nil
		},

		List: func(stdout io.Writer, workspaceDir string) error {
			inst := factory(workspaceDir)
			skills, err := inst.List()
			if err != nil {
				return err
			}
			if len(skills) == 0 {
				fmt.Fprintln(stdout, "No hub skills installed.")
				return nil
			}
			for _, s := range skills {
				fmt.Fprintf(stdout, "  %s@%s  (%s)  %s\n", s.Name, s.Version, s.Source, s.Dir)
			}
			fmt.Fprintf(stdout, "\n%d skill(s) installed.\n", len(skills))
			return nil
		},

		Update: func(ctx context.Context, stdout, _ io.Writer, workspaceDir string) error {
			inst := factory(workspaceDir)
			result, err := inst.Update(ctx)
			if err != nil && hubUpdateResultEmpty(result) {
				return err
			}
			printHubUpdateResult(stdout, "skill", result)
			return err
		},

		Info: func(ctx context.Context, stdout io.Writer, name, from string) error {
			inst := factory("")
			ref := buildSkillRef(from, name)
			entry, sourceID, err := inst.LookupSkill(ctx, ref)
			if err != nil {
				return err
			}
			fprintf(stdout, "Source:      %s\n", sourceID)
			fprintf(stdout, "Name:        %s\n", entry.Name)
			fprintf(stdout, "Version:     %s\n", entry.Version)
			fprintf(stdout, "Author:      %s\n", entry.Author)
			fprintf(stdout, "Description: %s\n", entry.Description)
			fprintf(stdout, "Invocable:   %v\n", entry.UserInvocable)
			if len(entry.Tags) > 0 {
				fprintf(stdout, "Tags:        %s\n", strings.Join(entry.Tags, ", "))
			}
			if entry.RequiresPlugin != "" {
				fprintf(stdout, "Plugin:      %s\n", entry.RequiresPlugin)
			}
			return nil
		},
	}, stdout, stderr)
}

// buildSkillRef combines an optional `--from` flag and a bare name into the
// "<source>:<name>" ref the installer understands. If from is empty the
// name is returned as-is so `tars-hub:foo` calls keep working.
func buildSkillRef(from, name string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return name
	}
	return from + ":" + strings.TrimSpace(name)
}

// stdinIsTerminal reports whether stdin is attached to a real terminal.
// /dev/null and pipes look like character devices to os.Stdin.Stat() and
// would otherwise be misclassified as interactive, so the check delegates
// to go-isatty which uses the platform ioctl.
func stdinIsTerminal() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// renderDryRunText prints a human-readable preview of an external-hub
// install. dryRunOnly tweaks the trailing line so the user understands
// the workspace is untouched.
func renderDryRunText(w io.Writer, p *skillhub.DryRunResult, dryRunOnly bool) {
	fprintln(w, "")
	fprintln(w, "────────── Install preview ──────────")
	fprintf(w, "Source     : %s\n", p.SourceID)
	fprintf(w, "Skill      : %s\n", p.OriginalName)
	if p.OriginalURL != "" {
		fprintf(w, "Source URL : %s\n", p.OriginalURL)
	} else if p.OriginalPath != "" {
		fprintf(w, "Source path: %s\n", p.OriginalPath)
	}
	fprintf(w, "Target dir : %s\n", p.TargetDir)
	if p.LicenseLabel != "" {
		fprintf(w, "License    : %s\n", p.LicenseLabel)
	}
	fprintln(w, "")
	fprintln(w, "Converted frontmatter:")
	fprintf(w, "  name        : %s\n", p.ConvertedSkill.Name)
	fprintf(w, "  description : %s\n", p.ConvertedSkill.Description)
	if p.ConvertedSkill.Version != "" {
		fprintf(w, "  version     : %s\n", p.ConvertedSkill.Version)
	}
	if p.ConvertedSkill.Author != "" {
		fprintf(w, "  author      : %s\n", p.ConvertedSkill.Author)
	}
	if len(p.ConvertedSkill.Tags) > 0 {
		fprintf(w, "  tags        : [%s]\n", strings.Join(p.ConvertedSkill.Tags, ", "))
	}
	fprintln(w, "")
	fprintf(w, "Files (%d):\n", len(p.Files))
	for _, fp := range p.Files {
		mismatch := ""
		if fp.ExpectedSHA256 != "" && !strings.EqualFold(fp.ExpectedSHA256, fp.SHA256) {
			mismatch = "  ⚠ checksum mismatch"
		}
		fprintf(w, "  %s  %dB  sha256:%s%s\n", fp.Path, fp.Size, shortHash(fp.SHA256), mismatch)
	}
	if len(p.AdapterWarnings) > 0 {
		fprintln(w, "")
		fprintln(w, "Adapter warnings:")
		for _, msg := range p.AdapterWarnings {
			fprintf(w, "  ! %s\n", msg)
		}
	}
	if len(p.ChecksumWarnings) > 0 {
		fprintln(w, "")
		fprintln(w, "Checksum warnings:")
		for _, msg := range p.ChecksumWarnings {
			fprintf(w, "  ⚠ %s\n", msg)
		}
	}
	if p.LicenseSource != "" {
		fprintln(w, "")
		fprintf(w, "ATTRIBUTION.md will be created (License: %s).\n", p.LicenseLabel)
	}
	if dryRunOnly {
		fprintln(w, "")
		fprintln(w, "Dry run: no files were written.")
	}
}

// renderDryRunJSON marshals the preview for machine consumption. The CLI
// emits json.MarshalIndent so the output is grep-able and diffable.
func renderDryRunJSON(w io.Writer, p *skillhub.DryRunResult) {
	body, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		fprintf(w, `{"error": "marshal preview: %s"}\n`, err)
		return
	}
	fprintf(w, "%s\n", body)
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}
