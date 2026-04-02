package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/spf13/cobra"
)

func newPluginCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins from the TARS Hub",
	}
	cmd.AddCommand(newPluginSearchCommand(stdout))
	cmd.AddCommand(newPluginInstallCommand(stdout, stderr))
	cmd.AddCommand(newPluginUninstallCommand(stdout, stderr))
	cmd.AddCommand(newPluginListCommand(stdout))
	cmd.AddCommand(newPluginUpdateCommand(stdout, stderr))
	cmd.AddCommand(newPluginInfoCommand(stdout))
	return cmd
}

func newPluginSearchCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "search [query]",
		Short: "Search the plugin registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			return runPluginSearch(cmd.Context(), stdout, query)
		},
	}
}

func runPluginSearch(ctx context.Context, stdout io.Writer, query string) error {
	reg := skillhub.NewRegistry()
	results, err := reg.SearchPlugins(ctx, query)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		fmt.Fprintln(stdout, "No plugins found.")
		return nil
	}
	for _, entry := range results {
		tags := ""
		if len(entry.Tags) > 0 {
			tags = " (" + strings.Join(entry.Tags, ", ") + ")"
		}
		fmt.Fprintf(stdout, "  %s@%s%s\n    %s\n", entry.Name, entry.Version, tags, entry.Description)
	}
	fmt.Fprintf(stdout, "\n%d plugin(s) found.\n", len(results))
	return nil
}

func newPluginInstallCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a plugin from the hub",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runPluginInstall(cmd.Context(), stdout, stderr, dir, args[0])
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runPluginInstall(ctx context.Context, stdout, _ io.Writer, workspaceDir, name string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	if err := inst.InstallPlugin(ctx, name); err != nil {
		return fmt.Errorf("install plugin %q: %w", name, err)
	}
	fmt.Fprintf(stdout, "Installed plugin %q to %s/plugins/%s\n", name, workspaceDir, name)
	return nil
}

func newPluginUninstallCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Uninstall a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runPluginUninstall(stdout, stderr, dir, args[0])
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runPluginUninstall(stdout, _ io.Writer, workspaceDir, name string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	if err := inst.UninstallPlugin(name); err != nil {
		return fmt.Errorf("uninstall plugin %q: %w", name, err)
	}
	fmt.Fprintf(stdout, "Uninstalled plugin %q\n", name)
	return nil
}

func newPluginListCommand(stdout io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed hub plugins",
		RunE: func(_ *cobra.Command, _ []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runPluginList(stdout, dir)
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runPluginList(stdout io.Writer, workspaceDir string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	plugins, err := inst.ListPlugins()
	if err != nil {
		return err
	}
	if len(plugins) == 0 {
		fmt.Fprintln(stdout, "No hub plugins installed.")
		return nil
	}
	for _, p := range plugins {
		fmt.Fprintf(stdout, "  %s@%s  (%s)  %s\n", p.Name, p.Version, p.Source, p.Dir)
	}
	fmt.Fprintf(stdout, "\n%d plugin(s) installed.\n", len(plugins))
	return nil
}

func newPluginUpdateCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update all installed hub plugins to latest",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runPluginUpdate(cmd.Context(), stdout, stderr, dir)
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runPluginUpdate(ctx context.Context, stdout, _ io.Writer, workspaceDir string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	updated, err := inst.UpdatePlugins(ctx)
	if err != nil {
		return err
	}
	if len(updated) == 0 {
		fmt.Fprintln(stdout, "All plugins are up to date.")
		return nil
	}
	for _, name := range updated {
		fmt.Fprintf(stdout, "  Updated: %s\n", name)
	}
	fmt.Fprintf(stdout, "\n%d plugin(s) updated.\n", len(updated))
	return nil
}

func newPluginInfoCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show detailed info about a plugin in the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInfo(cmd.Context(), stdout, args[0])
		},
	}
}

func runPluginInfo(ctx context.Context, stdout io.Writer, name string) error {
	reg := skillhub.NewRegistry()
	entry, err := reg.FindPluginByName(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Name:        %s\n", entry.Name)
	fmt.Fprintf(stdout, "Version:     %s\n", entry.Version)
	fmt.Fprintf(stdout, "Author:      %s\n", entry.Author)
	fmt.Fprintf(stdout, "Description: %s\n", entry.Description)
	if len(entry.Tags) > 0 {
		fmt.Fprintf(stdout, "Tags:        %s\n", strings.Join(entry.Tags, ", "))
	}
	if len(entry.Files) > 0 {
		fmt.Fprintf(stdout, "Files:       %s\n", strings.Join(entry.Files.Paths(), ", "))
	}
	return nil
}
