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
	return newHubResourceCommand(hubResourceSpec{
		Use:   "mcp",
		Short: "Manage MCP servers from the TARS Hub",
		Noun:  "MCP server",

		Search: func(ctx context.Context, stdout io.Writer, query string) error {
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
		},

		Install: func(ctx context.Context, stdout, _ io.Writer, workspaceDir, name string) error {
			inst := skillhub.NewInstaller(workspaceDir)
			if err := inst.InstallMCP(ctx, name); err != nil {
				return fmt.Errorf("install mcp server %q: %w", name, err)
			}
			fmt.Fprintf(stdout, "Installed MCP server %q to %s/mcp-servers/%s\n", name, workspaceDir, name)
			return nil
		},

		Uninstall: func(stdout, _ io.Writer, workspaceDir, name string) error {
			inst := skillhub.NewInstaller(workspaceDir)
			if err := inst.UninstallMCP(name); err != nil {
				return fmt.Errorf("uninstall mcp server %q: %w", name, err)
			}
			fmt.Fprintf(stdout, "Uninstalled MCP server %q\n", name)
			return nil
		},

		List: func(stdout io.Writer, workspaceDir string) error {
			inst := skillhub.NewInstaller(workspaceDir)
			mcps, err := inst.ListMCPs()
			if err != nil {
				return err
			}
			if len(mcps) == 0 {
				fmt.Fprintln(stdout, "No hub MCP servers installed.")
				return nil
			}
			for _, m := range mcps {
				fmt.Fprintf(stdout, "  %s@%s  (%s)  %s\n", m.Name, m.Version, m.Source, m.Dir)
			}
			fmt.Fprintf(stdout, "\n%d MCP server(s) installed.\n", len(mcps))
			return nil
		},

		Update: func(ctx context.Context, stdout, _ io.Writer, workspaceDir string) error {
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
		},

		Info: func(ctx context.Context, stdout io.Writer, name string) error {
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
		},
	}, stdout, stderr)
}
