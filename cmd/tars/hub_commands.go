package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/spf13/cobra"
)

// HubInstallOptions carries the install-time flag values into the
// hubResourceSpec.Install callback so each resource (skill / plugin / mcp)
// can implement its own external-hub flow.
type HubInstallOptions struct {
	From string // --from <hub-id>; empty means "any registered source"
	Yes  bool   // --yes auto-approves external-hub installs
}

// hubResourceSpec describes a hub-managed resource type (skill, plugin, or MCP
// server) and the callbacks needed to wire the standard search/install/
// uninstall/list/update/info subcommand tree.
type hubResourceSpec struct {
	// Use and Short for the parent cobra command.
	Use   string
	Short string
	// Singular and plural nouns for messages (e.g. "skill" / "skills").
	Noun       string
	PluralNoun string

	// Operation callbacks. Each receives the standard context/writer/args.
	Search    func(ctx context.Context, stdout io.Writer, query, from string) error
	Install   func(ctx context.Context, stdout, stderr io.Writer, workspaceDir, name string, opts HubInstallOptions) error
	Uninstall func(stdout, stderr io.Writer, workspaceDir, name string) error
	List      func(stdout io.Writer, workspaceDir string) error
	Update    func(ctx context.Context, stdout, stderr io.Writer, workspaceDir string) error
	Info      func(ctx context.Context, stdout io.Writer, name, from string) error
}

// newHubResourceCommand builds the full search/install/uninstall/list/update/
// info subcommand tree from a hubResourceSpec.
func newHubResourceCommand(spec hubResourceSpec, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
	}
	pluralNoun := spec.plural()

	searchCmd := &cobra.Command{
		Use:   "search [query]",
		Short: fmt.Sprintf("Search the %s registry", spec.Noun),
		Args:  cobra.MaximumNArgs(1),
	}
	var searchFrom string
	searchCmd.Flags().StringVar(&searchFrom, "from", "", "restrict to a specific hub source (e.g. openclaw)")
	searchCmd.RunE = func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = args[0]
		}
		return spec.Search(cmd.Context(), stdout, query, searchFrom)
	}
	cmd.AddCommand(searchCmd)

	installCmd := &cobra.Command{
		Use:   "install <name>",
		Short: fmt.Sprintf("Install a %s from the hub", spec.Noun),
		Args:  cobra.ExactArgs(1),
	}
	var installWorkspaceDir string
	var installFrom string
	var installYes bool
	installCmd.Flags().StringVar(&installWorkspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	installCmd.Flags().StringVar(&installFrom, "from", "", "install from a specific hub source (e.g. openclaw)")
	installCmd.Flags().BoolVarP(&installYes, "yes", "y", false, "auto-approve external-hub installs (skip the confirmation prompt)")
	installCmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir, err := resolveWorkspaceDir(installWorkspaceDir)
		if err != nil {
			return err
		}
		return spec.Install(cmd.Context(), stdout, stderr, dir, args[0], HubInstallOptions{
			From: installFrom,
			Yes:  installYes,
		})
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
		Short: fmt.Sprintf("List installed hub %s", pluralNoun),
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
		Short: fmt.Sprintf("Update all installed hub %s to latest", pluralNoun),
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

	infoCmd := &cobra.Command{
		Use:   "info <name>",
		Short: fmt.Sprintf("Show detailed info about a %s in the registry", spec.Noun),
		Args:  cobra.ExactArgs(1),
	}
	var infoFrom string
	infoCmd.Flags().StringVar(&infoFrom, "from", "", "look up in a specific hub source (e.g. openclaw)")
	infoCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return spec.Info(cmd.Context(), stdout, args[0], infoFrom)
	}
	cmd.AddCommand(infoCmd)

	return cmd
}

func (spec hubResourceSpec) plural() string {
	if plural := strings.TrimSpace(spec.PluralNoun); plural != "" {
		return plural
	}
	return strings.TrimSpace(spec.Noun) + "s"
}

func printHubUpdateResult(stdout io.Writer, noun string, result skillhub.UpdateResult) {
	if hubUpdateResultEmpty(result) {
		fmt.Fprintf(stdout, "All %ss are up to date.\n", noun)
		return
	}
	for _, name := range result.Updated {
		fmt.Fprintf(stdout, "  Updated: %s\n", name)
	}
	for _, item := range result.Skipped {
		fmt.Fprintf(stdout, "  Skipped: %s%s\n", item.Name, formatUpdateDetail(item.Detail()))
	}
	for _, item := range result.Failed {
		fmt.Fprintf(stdout, "  Failed: %s%s\n", item.Name, formatUpdateDetail(item.Detail()))
	}
	fmt.Fprintf(stdout, "\n%d %s(s) updated, %d skipped, %d failed.\n", len(result.Updated), noun, len(result.Skipped), len(result.Failed))
}

func hubUpdateResultEmpty(result skillhub.UpdateResult) bool {
	return len(result.Updated) == 0 && len(result.Skipped) == 0 && len(result.Failed) == 0
}

func formatUpdateDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return ""
	}
	return " (" + detail + ")"
}
