package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/spf13/cobra"
)

func newSkillCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage skills from the TARS Skill Hub",
	}
	cmd.AddCommand(newSkillSearchCommand(stdout))
	cmd.AddCommand(newSkillInstallCommand(stdout, stderr))
	cmd.AddCommand(newSkillUninstallCommand(stdout, stderr))
	cmd.AddCommand(newSkillListCommand(stdout))
	cmd.AddCommand(newSkillUpdateCommand(stdout, stderr))
	cmd.AddCommand(newSkillInfoCommand(stdout))
	return cmd
}

func newSkillSearchCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "search [query]",
		Short: "Search the skill registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			return runSkillSearch(cmd.Context(), stdout, query)
		},
	}
}

func runSkillSearch(ctx context.Context, stdout io.Writer, query string) error {
	reg := skillhub.NewRegistry()
	results, err := reg.Search(ctx, query)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		fmt.Fprintln(stdout, "No skills found.")
		return nil
	}
	for _, entry := range results {
		invocable := ""
		if entry.UserInvocable {
			invocable = " [invocable]"
		}
		tags := ""
		if len(entry.Tags) > 0 {
			tags = " (" + strings.Join(entry.Tags, ", ") + ")"
		}
		fmt.Fprintf(stdout, "  %s@%s%s%s\n    %s\n", entry.Name, entry.Version, invocable, tags, entry.Description)
	}
	fmt.Fprintf(stdout, "\n%d skill(s) found.\n", len(results))
	return nil
}

func newSkillInstallCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a skill from the hub",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runSkillInstall(cmd.Context(), stdout, stderr, dir, args[0])
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runSkillInstall(ctx context.Context, stdout, stderr io.Writer, workspaceDir, name string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	result, err := inst.Install(ctx, name)
	if err != nil {
		return fmt.Errorf("install %q: %w", name, err)
	}
	fmt.Fprintf(stdout, "Installed skill %q to %s/skills/%s\n", name, workspaceDir, name)
	if result.RequiresPlugin != "" {
		fmt.Fprintf(stderr, "⚠ This skill requires plugin %q. Install it with: tars plugin install %s\n", result.RequiresPlugin, result.RequiresPlugin)
	}
	return nil
}

func newSkillUninstallCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Uninstall a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runSkillUninstall(stdout, stderr, dir, args[0])
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runSkillUninstall(stdout, _ io.Writer, workspaceDir, name string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	if err := inst.Uninstall(name); err != nil {
		return fmt.Errorf("uninstall %q: %w", name, err)
	}
	fmt.Fprintf(stdout, "Uninstalled skill %q\n", name)
	return nil
}

func newSkillListCommand(stdout io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed hub skills",
		RunE: func(_ *cobra.Command, _ []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runSkillList(stdout, dir)
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runSkillList(stdout io.Writer, workspaceDir string) error {
	inst := skillhub.NewInstaller(workspaceDir)
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
}

func newSkillUpdateCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update all installed hub skills to latest",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runSkillUpdate(cmd.Context(), stdout, stderr, dir)
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runSkillUpdate(ctx context.Context, stdout, _ io.Writer, workspaceDir string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	updated, err := inst.Update(ctx)
	if err != nil {
		return err
	}
	if len(updated) == 0 {
		fmt.Fprintln(stdout, "All skills are up to date.")
		return nil
	}
	for _, name := range updated {
		fmt.Fprintf(stdout, "  Updated: %s\n", name)
	}
	fmt.Fprintf(stdout, "\n%d skill(s) updated.\n", len(updated))
	return nil
}

func newSkillInfoCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show detailed info about a skill in the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillInfo(cmd.Context(), stdout, args[0])
		},
	}
}

func runSkillInfo(ctx context.Context, stdout io.Writer, name string) error {
	reg := skillhub.NewRegistry()
	entry, err := reg.FindByName(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Name:        %s\n", entry.Name)
	fmt.Fprintf(stdout, "Version:     %s\n", entry.Version)
	fmt.Fprintf(stdout, "Author:      %s\n", entry.Author)
	fmt.Fprintf(stdout, "Description: %s\n", entry.Description)
	fmt.Fprintf(stdout, "Invocable:   %v\n", entry.UserInvocable)
	if len(entry.Tags) > 0 {
		fmt.Fprintf(stdout, "Tags:        %s\n", strings.Join(entry.Tags, ", "))
	}
	if entry.RequiresPlugin != "" {
		fmt.Fprintf(stdout, "Plugin:      %s\n", entry.RequiresPlugin)
	}
	return nil
}
