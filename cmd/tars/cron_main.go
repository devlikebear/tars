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

type cronCommandOptions struct {
	client clientOptions
	action string
	jobID  string
	limit  int
}

var cronCommandRunner = runCronCommand

func newCronCommand(stdout, stderr io.Writer) *cobra.Command {
	clientOpts := defaultClientOptions()
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Run one-shot cron operations",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List cron jobs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cronCommandRunner(cmd.Context(), stdout, stderr, cronCommandOptions{
				client: clientOpts,
				action: "list",
			})
		},
	}
	bindReadOnlyRuntimeFlags(listCmd, &clientOpts)

	getCmd := &cobra.Command{
		Use:   "get {job_id}",
		Short: "Show one cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cronCommandRunner(cmd.Context(), stdout, stderr, cronCommandOptions{
				client: clientOpts,
				action: "get",
				jobID:  strings.TrimSpace(args[0]),
			})
		},
	}
	bindReadOnlyRuntimeFlags(getCmd, &clientOpts)

	runsCmd := &cobra.Command{
		Use:   "runs {job_id} [limit]",
		Short: "Show recent cron runs",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			limit := 20
			if len(args) == 2 {
				value, err := strconv.Atoi(strings.TrimSpace(args[1]))
				if err != nil || value <= 0 {
					return fmt.Errorf("usage: cron runs {job_id} [limit]")
				}
				limit = value
			}
			return cronCommandRunner(cmd.Context(), stdout, stderr, cronCommandOptions{
				client: clientOpts,
				action: "runs",
				jobID:  strings.TrimSpace(args[0]),
				limit:  limit,
			})
		},
	}
	bindReadOnlyRuntimeFlags(runsCmd, &clientOpts)

	runCmd := &cobra.Command{
		Use:   "run {job_id}",
		Short: "Run one cron job immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cronCommandRunner(cmd.Context(), stdout, stderr, cronCommandOptions{
				client: clientOpts,
				action: "run",
				jobID:  strings.TrimSpace(args[0]),
			})
		},
	}
	bindReadOnlyRuntimeFlags(runCmd, &clientOpts)

	cmd.AddCommand(listCmd, getCmd, runsCmd, runCmd)
	return cmd
}

func runCronCommand(ctx context.Context, stdout, _ io.Writer, opts cronCommandOptions) error {
	client := newProtocolClient(opts.client)
	switch strings.TrimSpace(opts.action) {
	case "list":
		jobs, err := client.ListCronJobs(ctx)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			_, err = fmt.Fprintln(stdout, "SYSTEM > (no cron jobs)")
			return err
		}
		if _, err := fmt.Fprintln(stdout, "SYSTEM > cron jobs"); err != nil {
			return err
		}
		for _, job := range jobs {
			if _, err := fmt.Fprintf(stdout, "- %s name=%s schedule=%s enabled=%t\n", job.ID, job.Name, job.Schedule, job.Enabled); err != nil {
				return err
			}
		}
		return nil
	case "run":
		response, err := client.RunCronJob(ctx, opts.jobID)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "SYSTEM > %s\n", response)
		return err
	case "get":
		job, err := client.GetCronJob(ctx, opts.jobID)
		if err != nil {
			return err
		}
		if err := printCronJob(stdout, job); err != nil {
			return err
		}
		runs, err := client.ListCronRuns(ctx, opts.jobID, 10)
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			_, err = fmt.Fprintln(stdout, "SYSTEM > (no cron run logs)")
			return err
		}
		if _, err := fmt.Fprintf(stdout, "SYSTEM > cron run logs (latest %d)\n", len(runs)); err != nil {
			return err
		}
		for _, run := range runs {
			if strings.TrimSpace(run.Error) != "" {
				if _, err := fmt.Fprintf(stdout, "- %s error=%s\n", cronValueOrDash(run.RanAt), cronLogText(run.Error)); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprintf(stdout, "- %s response=%s\n", cronValueOrDash(run.RanAt), cronLogText(run.Response)); err != nil {
				return err
			}
		}
		return nil
	case "runs":
		runs, err := client.ListCronRuns(ctx, opts.jobID, opts.limit)
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			_, err = fmt.Fprintln(stdout, "SYSTEM > (no cron runs)")
			return err
		}
		if _, err := fmt.Fprintln(stdout, "SYSTEM > cron runs"); err != nil {
			return err
		}
		for _, run := range runs {
			if strings.TrimSpace(run.Error) != "" {
				if _, err := fmt.Fprintf(stdout, "- %s error=%s\n", cronValueOrDash(run.RanAt), cronLogText(run.Error)); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprintf(stdout, "- %s response=%s\n", cronValueOrDash(run.RanAt), cronLogText(run.Response)); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported cron action: %s", strings.TrimSpace(opts.action))
	}
}

func printCronJob(stdout io.Writer, job protocol.CronJob) error {
	if _, err := fmt.Fprintf(stdout, "SYSTEM > cron job %s\n", job.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "name=%s schedule=%s enabled=%t delete_after_run=%t\n",
		cronValueOrDash(job.Name), cronValueOrDash(job.Schedule), job.Enabled, job.DeleteAfterRun); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "session_target=%s wake_mode=%s delivery_mode=%s\n",
		cronValueOrDash(job.SessionTarget),
		cronValueOrDash(job.WakeMode),
		cronValueOrDash(job.DeliveryMode),
	); err != nil {
		return err
	}
	if strings.TrimSpace(job.LastRunAt) != "" {
		if _, err := fmt.Fprintf(stdout, "last_run_at=%s\n", strings.TrimSpace(job.LastRunAt)); err != nil {
			return err
		}
	}
	if strings.TrimSpace(job.LastRunError) != "" {
		if _, err := fmt.Fprintf(stdout, "last_run_error=%s\n", cronLogText(job.LastRunError)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(stdout, "prompt:"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(stdout, cronPromptText(job.Prompt))
	return err
}

func cronPromptText(v string) string {
	text := strings.TrimSpace(v)
	if text == "" {
		return "-"
	}
	return text
}

func cronLogText(v string) string {
	text := strings.TrimSpace(v)
	if text == "" {
		return "-"
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "\\n")
	return text
}

func cronValueOrDash(v string) string {
	text := strings.TrimSpace(v)
	if text == "" {
		return "-"
	}
	return text
}
