package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/spf13/cobra"
)

func newTemplateCommand(stdout, stderr io.Writer) *cobra.Command {
	return newTemplateCommandWithFactory(stdout, stderr, skillhub.NewInstaller)
}

// newTemplateCommandWithFactory builds `tars template` on installers from
// factory, so tests can point the registry at a local server.
func newTemplateCommandWithFactory(stdout, stderr io.Writer, factory skillInstallerFactory) *cobra.Command {
	cmd := newHubResourceCommand(hubResourceSpec{
		Use:        "template",
		Short:      "Manage focus templates from the TARS Hub",
		Noun:       "focus template",
		PluralNoun: "focus templates",

		Search: func(ctx context.Context, stdout io.Writer, query, _ string) error {
			results, err := factory("").Registry.SearchFocusTemplates(ctx, query)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			if len(results) == 0 {
				fprintln(stdout, "No focus templates found.")
				return nil
			}
			for _, entry := range results {
				tags := ""
				if len(entry.Tags) > 0 {
					tags = " (" + strings.Join(entry.Tags, ", ") + ")"
				}
				fprintf(stdout, "  %s@%s%s\n    %s\n", entry.Name, entry.Version, tags, entry.Description)
			}
			fprintf(stdout, "\n%d focus template(s) found.\n", len(results))
			return nil
		},

		Install: func(ctx context.Context, stdout, _ io.Writer, workspaceDir, name string, _ HubInstallOptions) error {
			inst := factory(workspaceDir)
			if _, err := inst.InstallFocusTemplate(ctx, name); err != nil {
				return fmt.Errorf("install focus template %q: %w", name, err)
			}
			fprintf(stdout, "Installed focus template %q to %s/focus-templates\n", name, workspaceDir)
			return nil
		},

		Uninstall: func(stdout, _ io.Writer, workspaceDir, name string) error {
			inst := factory(workspaceDir)
			if err := inst.UninstallFocusTemplate(name); err != nil {
				return fmt.Errorf("uninstall focus template %q: %w", name, err)
			}
			fprintf(stdout, "Uninstalled focus template %q\n", name)
			return nil
		},

		List: func(stdout io.Writer, workspaceDir string) error {
			inst := factory(workspaceDir)
			rows, err := inst.ListFocusTemplates()
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				fprintln(stdout, "No hub focus templates installed.")
				return nil
			}
			for _, row := range rows {
				fprintf(stdout, "  %s@%s  (%s)  focus-templates/%s\n", row.Name, row.Version, row.Source, row.File)
			}
			fprintf(stdout, "\n%d focus template(s) installed.\n", len(rows))
			return nil
		},

		Update: func(ctx context.Context, stdout, _ io.Writer, workspaceDir string) error {
			inst := factory(workspaceDir)
			result, err := inst.UpdateFocusTemplates(ctx)
			if err != nil && hubUpdateResultEmpty(result) {
				return err
			}
			printHubUpdateResult(stdout, "focus template", result)
			return err
		},

		Info: func(ctx context.Context, stdout io.Writer, name, _ string) error {
			entry, err := factory("").Registry.FindFocusTemplateByName(ctx, name)
			if err != nil {
				return err
			}
			fprintf(stdout, "Name:        %s\n", entry.Name)
			fprintf(stdout, "Version:     %s\n", entry.Version)
			fprintf(stdout, "Author:      %s\n", entry.Author)
			fprintf(stdout, "Description: %s\n", entry.Description)
			if len(entry.Tags) > 0 {
				fprintf(stdout, "Tags:        %s\n", strings.Join(entry.Tags, ", "))
			}
			if len(entry.Files) > 0 {
				fprintf(stdout, "Files:       %s\n", strings.Join(entry.Files.Paths(), ", "))
			}
			return nil
		},
	}, stdout, stderr)
	cmd.Aliases = []string{"focus-template"}
	return cmd
}
