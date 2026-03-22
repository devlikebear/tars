package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/spf13/cobra"
)

func newMCPCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP servers from the trusted TARS Hub",
	}
	cmd.AddCommand(newMCPSearchCommand(stdout))
	cmd.AddCommand(newMCPInstallCommand(stdout, stderr))
	cmd.AddCommand(newMCPUninstallCommand(stdout, stderr))
	cmd.AddCommand(newMCPListCommand(stdout))
	cmd.AddCommand(newMCPUpdateCommand(stdout, stderr))
	cmd.AddCommand(newMCPInfoCommand(stdout))
	return cmd
}

func newMCPSearchCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "search [query]",
		Short: "Search the MCP registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			return runMCPSearch(cmd.Context(), stdout, query)
		},
	}
}

func runMCPSearch(ctx context.Context, stdout io.Writer, query string) error {
	reg := skillhub.NewRegistry()
	results, err := reg.SearchMCPServers(ctx, query)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		fmt.Fprintln(stdout, "No MCP servers found.")
		return nil
	}
	for _, entry := range results {
		tags := ""
		if len(entry.Tags) > 0 {
			tags = " (" + strings.Join(entry.Tags, ", ") + ")"
		}
		fmt.Fprintf(stdout, "  %s@%s%s\n    %s\n", entry.Name, entry.Version, tags, entry.Description)
	}
	fmt.Fprintf(stdout, "\n%d MCP server(s) found.\n", len(results))
	return nil
}

func newMCPInstallCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install an MCP server from the hub",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runMCPInstall(cmd.Context(), stdout, stderr, dir, args[0])
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runMCPInstall(ctx context.Context, stdout, _ io.Writer, workspaceDir, name string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	if err := inst.InstallMCP(ctx, name); err != nil {
		return fmt.Errorf("install mcp server %q: %w", name, err)
	}
	fmt.Fprintf(stdout, "Installed MCP server %q to %s/mcp-servers/%s\n", name, workspaceDir, name)
	return nil
}

func newMCPUninstallCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Uninstall a managed MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runMCPUninstall(stdout, stderr, dir, args[0])
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runMCPUninstall(stdout, _ io.Writer, workspaceDir, name string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	if err := inst.UninstallMCP(name); err != nil {
		return fmt.Errorf("uninstall mcp server %q: %w", name, err)
	}
	fmt.Fprintf(stdout, "Uninstalled MCP server %q\n", name)
	return nil
}

func newMCPListCommand(stdout io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed hub MCP servers",
		RunE: func(_ *cobra.Command, _ []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runMCPList(stdout, dir)
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runMCPList(stdout io.Writer, workspaceDir string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	mcps, err := inst.ListMCPs()
	if err != nil {
		return err
	}
	if len(mcps) == 0 {
		fmt.Fprintln(stdout, "No hub MCP servers installed.")
		return nil
	}
	for _, mcp := range mcps {
		fmt.Fprintf(stdout, "  %s@%s  (%s)  %s\n", mcp.Name, mcp.Version, mcp.Source, mcp.Dir)
	}
	fmt.Fprintf(stdout, "\n%d MCP server(s) installed.\n", len(mcps))
	return nil
}

func newMCPUpdateCommand(stdout, stderr io.Writer) *cobra.Command {
	var workspaceDir string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update all installed hub MCP servers to latest",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := resolveWorkspaceDir(workspaceDir)
			if err != nil {
				return err
			}
			return runMCPUpdate(cmd.Context(), stdout, stderr, dir)
		},
	}
	cmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	return cmd
}

func runMCPUpdate(ctx context.Context, stdout, _ io.Writer, workspaceDir string) error {
	inst := skillhub.NewInstaller(workspaceDir)
	updated, err := inst.UpdateMCPs(ctx)
	if err != nil {
		return err
	}
	if len(updated) == 0 {
		fmt.Fprintln(stdout, "All MCP servers are up to date.")
		return nil
	}
	for _, name := range updated {
		fmt.Fprintf(stdout, "  Updated: %s\n", name)
	}
	fmt.Fprintf(stdout, "\n%d MCP server(s) updated.\n", len(updated))
	return nil
}

func newMCPInfoCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show detailed info about an MCP server in the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPInfo(cmd.Context(), stdout, args[0])
		},
	}
}

func runMCPInfo(ctx context.Context, stdout io.Writer, name string) error {
	reg := skillhub.NewRegistry()
	entry, err := reg.FindMCPByName(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Name:        %s\n", entry.Name)
	fmt.Fprintf(stdout, "Version:     %s\n", entry.Version)
	fmt.Fprintf(stdout, "Author:      %s\n", entry.Author)
	fmt.Fprintf(stdout, "Description: %s\n", entry.Description)
	fmt.Fprintf(stdout, "Manifest:    %s\n", entry.Manifest)
	if len(entry.Tags) > 0 {
		fmt.Fprintf(stdout, "Tags:        %s\n", strings.Join(entry.Tags, ", "))
	}
	if len(entry.Files) > 0 {
		fileNames := make([]string, 0, len(entry.Files))
		for _, file := range entry.Files {
			fileNames = append(fileNames, file.Path)
		}
		fmt.Fprintf(stdout, "Files:       %s\n", strings.Join(fileNames, ", "))
	}
	return nil
}
