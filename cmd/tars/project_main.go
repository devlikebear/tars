package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	protocol "github.com/devlikebear/tars/pkg/tarsclient"
	"github.com/spf13/cobra"
)

type projectCommandOptions struct {
	client    clientOptions
	action    string
	projectID string
	limit     int
}

var projectCommandRunner = runProjectCommand

func newProjectCommand(stdout, stderr io.Writer) *cobra.Command {
	clientOpts := defaultClientOptions()
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Run one-shot project operations",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return projectCommandRunner(cmd.Context(), stdout, stderr, projectCommandOptions{
				client: clientOpts,
				action: "list",
			})
		},
	}
	bindReadOnlyRuntimeFlags(listCmd, &clientOpts)

	getCmd := &cobra.Command{
		Use:   "get {project_id}",
		Short: "Show project details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return projectCommandRunner(cmd.Context(), stdout, stderr, projectCommandOptions{
				client:    clientOpts,
				action:    "get",
				projectID: strings.TrimSpace(args[0]),
			})
		},
	}
	bindReadOnlyRuntimeFlags(getCmd, &clientOpts)

	activityCmd := &cobra.Command{
		Use:   "activity {project_id} [limit]",
		Short: "Show recent project activity",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			limit := 10
			if len(args) == 2 {
				value, err := strconv.Atoi(strings.TrimSpace(args[1]))
				if err != nil || value < 0 {
					return fmt.Errorf("limit must be a non-negative integer")
				}
				limit = value
			}
			return projectCommandRunner(cmd.Context(), stdout, stderr, projectCommandOptions{
				client:    clientOpts,
				action:    "activity",
				projectID: strings.TrimSpace(args[0]),
				limit:     limit,
			})
		},
	}
	bindReadOnlyRuntimeFlags(activityCmd, &clientOpts)

	autopilotCmd := &cobra.Command{
		Use:   "autopilot",
		Short: "Run project autopilot actions",
	}
	for _, action := range []struct {
		use    string
		action string
		short  string
	}{
		{use: "start {project_id}", action: "autopilot-start", short: "Start project autopilot"},
		{use: "status {project_id}", action: "autopilot-status", short: "Show project autopilot status"},
		{use: "advance {project_id}", action: "autopilot-advance", short: "Run one autopilot step"},
	} {
		action := action
		sub := &cobra.Command{
			Use:   action.use,
			Short: action.short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return projectCommandRunner(cmd.Context(), stdout, stderr, projectCommandOptions{
					client:    clientOpts,
					action:    action.action,
					projectID: strings.TrimSpace(args[0]),
				})
			},
		}
		bindReadOnlyRuntimeFlags(sub, &clientOpts)
		autopilotCmd.AddCommand(sub)
	}

	cmd.AddCommand(listCmd, getCmd, activityCmd, autopilotCmd)
	return cmd
}

func runProjectCommand(ctx context.Context, stdout, _ io.Writer, opts projectCommandOptions) error {
	client := newProtocolClient(opts.client)
	switch strings.TrimSpace(opts.action) {
	case "list":
		items, err := client.ListProjects(ctx)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			_, err = fmt.Fprintln(stdout, "SYSTEM > (no projects)")
			return err
		}
		if _, err := fmt.Fprintln(stdout, "SYSTEM > projects"); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(stdout, "- %s name=%s type=%s status=%s\n",
				strings.TrimSpace(item.ID),
				strings.TrimSpace(item.Name),
				strings.TrimSpace(item.Type),
				strings.TrimSpace(item.Status),
			); err != nil {
				return err
			}
		}
		return nil
	case "get":
		item, err := client.GetProject(ctx, opts.projectID)
		if err != nil {
			return err
		}
		return printProject(stdout, item)
	case "activity":
		items, err := client.ListProjectActivity(ctx, opts.projectID, opts.limit)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "SYSTEM > project activity %s count=%d\n", strings.TrimSpace(opts.projectID), len(items)); err != nil {
			return err
		}
		if len(items) == 0 {
			_, err = fmt.Fprintln(stdout, "(no recent activity)")
			return err
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(stdout, "- %s source=%s kind=%s status=%s task=%s agent=%s message=%s\n",
				strings.TrimSpace(item.Timestamp),
				strings.TrimSpace(item.Source),
				strings.TrimSpace(item.Kind),
				strings.TrimSpace(item.Status),
				strings.TrimSpace(item.TaskID),
				strings.TrimSpace(item.Agent),
				strings.TrimSpace(item.Message),
			); err != nil {
				return err
			}
		}
		return nil
	case "autopilot-start":
		item, err := client.StartProjectAutopilot(ctx, opts.projectID)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "SYSTEM > autopilot started project_id=%s run_id=%s status=%s iterations=%d\n",
			strings.TrimSpace(item.ProjectID),
			strings.TrimSpace(item.RunID),
			strings.TrimSpace(item.Status),
			item.Iterations,
		)
		return err
	case "autopilot-status":
		item, err := client.GetProjectAutopilot(ctx, opts.projectID)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "SYSTEM > project autopilot %s run_id=%s status=%s iterations=%d phase=%s phase_status=%s next_action=%s message=%s\n",
			strings.TrimSpace(item.ProjectID),
			strings.TrimSpace(item.RunID),
			strings.TrimSpace(item.Status),
			item.Iterations,
			strings.TrimSpace(item.Phase),
			strings.TrimSpace(item.PhaseStatus),
			strings.TrimSpace(item.NextAction),
			strings.TrimSpace(item.Message),
		)
		return err
	case "autopilot-advance":
		item, err := client.AdvanceProjectAutopilot(ctx, opts.projectID)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "SYSTEM > project autopilot advance %s phase=%s status=%s run_status=%s next_action=%s message=%s\n",
			strings.TrimSpace(item.ProjectID),
			strings.TrimSpace(item.Name),
			strings.TrimSpace(item.Status),
			strings.TrimSpace(item.RunStatus),
			strings.TrimSpace(item.NextAction),
			strings.TrimSpace(item.Message),
		)
		return err
	default:
		return fmt.Errorf("unsupported project action: %s", strings.TrimSpace(opts.action))
	}
}

func printProject(stdout io.Writer, item protocol.Project) error {
	if _, err := fmt.Fprintf(stdout, "SYSTEM > project %s\n", strings.TrimSpace(item.ID)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "name=%s type=%s status=%s\n",
		strings.TrimSpace(item.Name),
		strings.TrimSpace(item.Type),
		strings.TrimSpace(item.Status),
	); err != nil {
		return err
	}
	if strings.TrimSpace(item.GitRepo) != "" {
		if _, err := fmt.Fprintf(stdout, "git_repo=%s\n", strings.TrimSpace(item.GitRepo)); err != nil {
			return err
		}
	}
	if strings.TrimSpace(item.Objective) != "" {
		if _, err := fmt.Fprintf(stdout, "objective=%s\n", strings.TrimSpace(item.Objective)); err != nil {
			return err
		}
	}
	if body := strings.TrimSpace(item.Body); body != "" {
		if _, err := fmt.Fprintln(stdout, body); err != nil {
			return err
		}
	}
	return nil
}
