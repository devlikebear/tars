package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

type approvalCommandOptions struct {
	client     clientOptions
	action     string
	approvalID string
}

var approvalCommandRunner = runApprovalCommand

func newApproveCommand(stdout, stderr io.Writer) *cobra.Command {
	clientOpts := defaultClientOptions()
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Run one-shot approval operations",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List pending approvals",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return approvalCommandRunner(cmd.Context(), stdout, stderr, approvalCommandOptions{
				client: clientOpts,
				action: "list",
			})
		},
	}
	bindReadOnlyRuntimeFlags(listCmd, &clientOpts)

	runCmd := &cobra.Command{
		Use:   "run {approval_id}",
		Short: "Approve and execute a cleanup approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return approvalCommandRunner(cmd.Context(), stdout, stderr, approvalCommandOptions{
				client:     clientOpts,
				action:     "run",
				approvalID: strings.TrimSpace(args[0]),
			})
		},
	}
	bindReadOnlyRuntimeFlags(runCmd, &clientOpts)

	rejectCmd := &cobra.Command{
		Use:   "reject {approval_id}",
		Short: "Reject a cleanup approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return approvalCommandRunner(cmd.Context(), stdout, stderr, approvalCommandOptions{
				client:     clientOpts,
				action:     "reject",
				approvalID: strings.TrimSpace(args[0]),
			})
		},
	}
	bindReadOnlyRuntimeFlags(rejectCmd, &clientOpts)

	cmd.AddCommand(listCmd, runCmd, rejectCmd)
	return cmd
}

func runApprovalCommand(ctx context.Context, stdout, _ io.Writer, opts approvalCommandOptions) error {
	client := newProtocolClient(opts.client)
	switch strings.TrimSpace(opts.action) {
	case "list":
		items, err := client.ListApprovals(ctx)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			_, err = fmt.Fprintln(stdout, "SYSTEM > (no approvals)")
			return err
		}
		if _, err := fmt.Fprintln(stdout, "SYSTEM > approvals"); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(stdout, "- %s type=%s status=%s candidates=%d\n", item.ID, item.Type, item.Status, len(item.Plan.Candidates)); err != nil {
				return err
			}
		}
		return nil
	case "run":
		if err := client.ApproveCleanup(ctx, opts.approvalID); err != nil {
			return err
		}
		result, err := client.ApplyCleanup(ctx, opts.approvalID)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "SYSTEM > cleanup applied approval_id=%s deleted=%d bytes=%d\n", result.ApprovalID, result.DeletedCount, result.DeletedBytes)
		return err
	case "reject":
		if err := client.RejectCleanup(ctx, opts.approvalID); err != nil {
			return err
		}
		_, err := fmt.Fprintf(stdout, "SYSTEM > approval rejected %s\n", opts.approvalID)
		return err
	default:
		return fmt.Errorf("unsupported approval action: %s", strings.TrimSpace(opts.action))
	}
}
