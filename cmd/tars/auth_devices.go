package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/spf13/cobra"
)

// newAuthDevicesCommand lists and revokes paired mobile devices
// (docs/decisions/mobile-remote.md §4). Pairing itself starts with
// `tars auth pairing-code --role focus`.
func newAuthDevicesCommand(stdout io.Writer) *cobra.Command {
	workspaceDir := defaultWorkspaceDir()
	cmd := &cobra.Command{
		Use:          "devices",
		Short:        "List paired mobile devices",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runAuthDevicesList(stdout, workspaceDir)
		},
	}
	cmd.PersistentFlags().StringVar(&workspaceDir, "workspace-dir", workspaceDir, "workspace directory")

	revokeCmd := &cobra.Command{
		Use:          "revoke <device-id>",
		Short:        "Stop a paired mobile device's token from working",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runAuthDevicesRevoke(stdout, workspaceDir, args[0])
		},
	}
	cmd.AddCommand(revokeCmd)
	return cmd
}

func runAuthDevicesList(stdout io.Writer, workspaceDirFlag string) error {
	workspaceDir, err := resolveWorkspaceDir(workspaceDirFlag)
	if err != nil {
		return err
	}
	devices, err := consoleauth.NewStore(workspaceDir).ListDevices()
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		_, err := fmt.Fprintln(stdout, "no paired devices")
		return err
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tNAME\tROLE\tPAIRED\tSTATUS"); err != nil {
		return err
	}
	for _, device := range devices {
		status := "active"
		if device.RevokedAt != nil {
			status = "revoked " + device.RevokedAt.Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", device.ID, device.Name, device.Role, device.CreatedAt.Format(time.RFC3339), status); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func runAuthDevicesRevoke(stdout io.Writer, workspaceDirFlag, id string) error {
	workspaceDir, err := resolveWorkspaceDir(workspaceDirFlag)
	if err != nil {
		return err
	}
	if err := consoleauth.NewStore(workspaceDir).RevokeDevice(id); err != nil {
		if errors.Is(err, consoleauth.ErrDeviceNotFound) {
			return fmt.Errorf("no paired device with id %q", id)
		}
		return err
	}
	_, err = fmt.Fprintf(stdout, "device %s revoked\n", id)
	return err
}

// normalizePairingCommandRole is normalizeAuthCommandRole plus focus: a
// pairing code can enroll a mobile device, which has no password.
func normalizePairingCommandRole(role string) (string, error) {
	if strings.TrimSpace(strings.ToLower(role)) == consoleauth.RoleFocus {
		return consoleauth.RoleFocus, nil
	}
	if normalized, err := normalizeAuthCommandRole(role); err == nil {
		return normalized, nil
	}
	return "", fmt.Errorf("unsupported role %q (expected admin, user or focus)", role)
}
