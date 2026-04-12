package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// hubResourceSpec describes a hub-managed resource type (skill, plugin, or MCP
// server) and the callbacks needed to wire the standard search/install/
// uninstall/list/update/info subcommand tree.
type hubResourceSpec struct {
	// Use and Short for the parent cobra command.
	Use   string
	Short string
	// Singular noun for messages (e.g. "skill", "plugin", "MCP server").
	Noun string

	// Operation callbacks. Each receives the standard context/writer/args.
	Search    func(ctx context.Context, stdout io.Writer, query string) error
	Install   func(ctx context.Context, stdout, stderr io.Writer, workspaceDir, name string) error
	Uninstall func(stdout, stderr io.Writer, workspaceDir, name string) error
	List      func(stdout io.Writer, workspaceDir string) error
	Update    func(ctx context.Context, stdout, stderr io.Writer, workspaceDir string) error
	Info      func(ctx context.Context, stdout io.Writer, name string) error
}

// newHubResourceCommand builds the full search/install/uninstall/list/update/
// info subcommand tree from a hubResourceSpec.
func newHubResourceCommand(spec hubResourceSpec, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "search [query]",
		Short: fmt.Sprintf("Search the %s registry", spec.Noun),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			return spec.Search(cmd.Context(), stdout, query)
		},
	})

	installCmd := &cobra.Command{
		Use:   "install <name>",
		Short: fmt.Sprintf("Install a %s from the hub", spec.Noun),
		Args:  cobra.ExactArgs(1),
	}
	var installWorkspaceDir string
	installCmd.Flags().StringVar(&installWorkspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	installCmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir, err := resolveWorkspaceDir(installWorkspaceDir)
		if err != nil {
			return err
		}
		return spec.Install(cmd.Context(), stdout, stderr, dir, args[0])
	}
	cmd.AddCommand(installCmd)

	uninstallCmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: fmt.Sprintf("Uninstall a %s", spec.Noun),
		Args:  cobra.ExactArgs(1),
	}
	var uninstallWorkspaceDir string
	uninstallCmd.Flags().StringVar(&uninstallWorkspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	uninstallCmd.RunE = func(_ *cobra.Command, args []string) error {
		dir, err := resolveWorkspaceDir(uninstallWorkspaceDir)
		if err != nil {
			return err
		}
		return spec.Uninstall(stdout, stderr, dir, args[0])
	}
	cmd.AddCommand(uninstallCmd)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List installed hub %ss", spec.Noun),
	}
	var listWorkspaceDir string
	listCmd.Flags().StringVar(&listWorkspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	listCmd.RunE = func(_ *cobra.Command, _ []string) error {
		dir, err := resolveWorkspaceDir(listWorkspaceDir)
		if err != nil {
			return err
		}
		return spec.List(stdout, dir)
	}
	cmd.AddCommand(listCmd)

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: fmt.Sprintf("Update all installed hub %ss to latest", spec.Noun),
	}
	var updateWorkspaceDir string
	updateCmd.Flags().StringVar(&updateWorkspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	updateCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		dir, err := resolveWorkspaceDir(updateWorkspaceDir)
		if err != nil {
			return err
		}
		return spec.Update(cmd.Context(), stdout, stderr, dir)
	}
	cmd.AddCommand(updateCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "info <name>",
		Short: fmt.Sprintf("Show detailed info about a %s in the registry", spec.Noun),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return spec.Info(cmd.Context(), stdout, args[0])
		},
	})

	return cmd
}
