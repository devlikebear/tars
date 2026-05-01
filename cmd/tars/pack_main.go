package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/spf13/cobra"
)

func newPackCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Manage domain packs from the TARS Hub",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "search [query]",
		Short: "Search domain packs in the hub",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			return searchPacks(cmd.Context(), stdout, query)
		},
	})

	installCmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a domain pack from the hub",
		Args:  cobra.ExactArgs(1),
	}
	var workspaceDir string
	var assumeYes bool
	installCmd.Flags().StringVar(&workspaceDir, "workspace-dir", defaultWorkspaceDir(), "workspace directory")
	installCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "install after printing the plan without prompting")
	installCmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir, err := resolveWorkspaceDir(workspaceDir)
		if err != nil {
			return err
		}
		return installPack(cmd.Context(), stdin, stdout, stderr, dir, args[0], assumeYes)
	}
	cmd.AddCommand(installCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "info <name>",
		Short: "Show detailed info about a domain pack",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showPackInfo(cmd.Context(), stdout, args[0])
		},
	})

	return cmd
}

func searchPacks(ctx context.Context, stdout io.Writer, query string) error {
	reg := skillhub.NewRegistry()
	results, err := reg.SearchPacks(ctx, query)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		fmt.Fprintln(stdout, "No packs found.")
		return nil
	}
	for _, entry := range results {
		tags := ""
		if len(entry.Tags) > 0 {
			tags = " (" + strings.Join(entry.Tags, ", ") + ")"
		}
		fmt.Fprintf(stdout, "  %s@%s%s\n    %s\n", entry.Name, entry.Version, tags, entry.Description)
	}
	fmt.Fprintf(stdout, "\n%d pack(s) found.\n", len(results))
	return nil
}

func showPackInfo(ctx context.Context, stdout io.Writer, name string) error {
	reg := skillhub.NewRegistry()
	entry, err := reg.FindPackByName(ctx, name)
	if err != nil {
		return err
	}
	printPackEntry(stdout, *entry)
	return nil
}

func installPack(ctx context.Context, stdin io.Reader, stdout, _ io.Writer, workspaceDir, name string, assumeYes bool) error {
	inst := skillhub.NewInstaller(workspaceDir)
	plan, err := inst.PlanPackInstall(ctx, name)
	if err != nil {
		return fmt.Errorf("plan pack %q: %w", name, err)
	}
	printPackInstallPlan(stdout, plan)
	if !assumeYes {
		ok, err := confirmPackInstall(stdin, stdout)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(stdout, "Installation cancelled.")
			return nil
		}
	}
	result, err := inst.InstallPack(ctx, name)
	if err != nil {
		return fmt.Errorf("install pack %q: %w", name, err)
	}
	printPackInstallResult(stdout, result)
	return nil
}

func printPackEntry(stdout io.Writer, entry skillhub.PackEntry) {
	fmt.Fprintf(stdout, "Name:        %s\n", entry.Name)
	fmt.Fprintf(stdout, "Version:     %s\n", entry.Version)
	fmt.Fprintf(stdout, "Author:      %s\n", entry.Author)
	fmt.Fprintf(stdout, "Description: %s\n", entry.Description)
	if len(entry.Tags) > 0 {
		fmt.Fprintf(stdout, "Tags:        %s\n", strings.Join(entry.Tags, ", "))
	}
	if len(entry.Plugins) > 0 {
		fmt.Fprintf(stdout, "Plugins:     %s\n", strings.Join(entry.Plugins, ", "))
	}
	if len(entry.MCPServers) > 0 {
		fmt.Fprintf(stdout, "MCP Servers: %s\n", strings.Join(entry.MCPServers, ", "))
	}
	if len(entry.Skills) > 0 {
		fmt.Fprintf(stdout, "Skills:      %s\n", strings.Join(entry.Skills, ", "))
	}
}

func printPackInstallPlan(stdout io.Writer, plan skillhub.PackInstallPlan) {
	fmt.Fprintf(stdout, "Pack: %s@%s\n", plan.Pack.Name, plan.Pack.Version)
	if strings.TrimSpace(plan.Pack.Description) != "" {
		fmt.Fprintf(stdout, "%s\n", plan.Pack.Description)
	}
	fmt.Fprintln(stdout, "\nInstall plan:")
	for _, item := range plan.Items {
		fmt.Fprintf(stdout, "  [%s] %s %s@%s\n", item.Action, item.Type, item.Name, item.Version)
		if strings.TrimSpace(item.Description) != "" {
			fmt.Fprintf(stdout, "      %s\n", item.Description)
		}
	}
}

func printPackInstallResult(stdout io.Writer, result *skillhub.PackInstallResult) {
	if len(result.Installed) > 0 {
		fmt.Fprintln(stdout, "\nInstalled:")
		for _, item := range result.Installed {
			fmt.Fprintf(stdout, "  %s %s@%s\n", item.Type, item.Name, item.Version)
		}
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintln(stdout, "\nSkipped:")
		for _, item := range result.Skipped {
			fmt.Fprintf(stdout, "  %s %s@%s (%s)\n", item.Type, item.Name, item.Version, item.Action)
		}
	}
	if len(result.SandboxReports) > 0 {
		fmt.Fprintln(stdout, "\nSandbox:")
		for _, report := range result.SandboxReports {
			fmt.Fprintf(stdout, "  %s %s: %s\n", report.PackageType, report.PackageName, packSandboxSummary(report))
		}
	}
	fmt.Fprintf(stdout, "\nPack %q complete.\n", result.Plan.Pack.Name)
}

func confirmPackInstall(stdin io.Reader, stdout io.Writer) (bool, error) {
	fmt.Fprint(stdout, "\nInstall this pack? [y/N] ")
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

func packSandboxSummary(report skillhub.SandboxReport) string {
	total := len(report.Checks)
	passed := 0
	for _, check := range report.Checks {
		if check.Status == "passed" {
			passed++
		}
	}
	return fmt.Sprintf("%d/%d checks", passed, total)
}
