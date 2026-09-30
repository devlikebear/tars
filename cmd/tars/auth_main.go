package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type authOptions struct {
	workspaceDir string
	password     string
	role         string
	ttl          time.Duration
}

func defaultAuthOptions() authOptions {
	return authOptions{
		workspaceDir: defaultWorkspaceDir(),
		role:         consoleauth.RoleUser,
		ttl:          5 * time.Minute,
	}
}

func newAuthCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage browser login accounts and pairing codes",
	}

	initOpts := defaultAuthOptions()
	initCmd := &cobra.Command{
		Use:          "init",
		Short:        "Create the initial admin browser login",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthInit(cmd.Context(), stdin, stdout, stderr, initOpts)
		},
	}
	initCmd.Flags().StringVar(&initOpts.workspaceDir, "workspace-dir", initOpts.workspaceDir, "workspace directory")
	initCmd.Flags().StringVar(&initOpts.password, "password", "", "admin password (or TARS_INITIAL_ADMIN_PASSWORD)")

	passwdOpts := defaultAuthOptions()
	passwdCmd := &cobra.Command{
		Use:          "passwd [admin|user]",
		Short:        "Set or change a browser login password",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			passwdOpts.role = args[0]
			return runAuthPasswd(cmd.Context(), stdin, stdout, stderr, passwdOpts)
		},
	}
	passwdCmd.Flags().StringVar(&passwdOpts.workspaceDir, "workspace-dir", passwdOpts.workspaceDir, "workspace directory")
	passwdCmd.Flags().StringVar(&passwdOpts.password, "password", "", "password")

	pairingOpts := defaultAuthOptions()
	pairingCmd := &cobra.Command{
		Use:          "pairing-code",
		Short:        "Create a one-time browser pairing code",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthPairingCode(cmd.Context(), stdout, pairingOpts)
		},
	}
	pairingCmd.Flags().StringVar(&pairingOpts.workspaceDir, "workspace-dir", pairingOpts.workspaceDir, "workspace directory")
	pairingCmd.Flags().StringVar(&pairingOpts.role, "role", pairingOpts.role, "role for the pairing code")
	pairingCmd.Flags().DurationVar(&pairingOpts.ttl, "ttl", pairingOpts.ttl, "pairing code TTL")

	cmd.AddCommand(initCmd, passwdCmd, pairingCmd)
	return cmd
}

func runAuthInit(_ context.Context, stdin io.Reader, stdout, stderr io.Writer, opts authOptions) error {
	workspaceDir, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return err
	}
	store := consoleauth.NewStore(workspaceDir)
	hasAdmin, err := store.HasPassword(consoleauth.RoleAdmin)
	if err != nil {
		return err
	}
	if hasAdmin {
		return fmt.Errorf("admin password already configured; use `tars auth passwd admin`")
	}
	password, err := resolveAuthPassword(stdin, stderr, opts.password, os.Getenv("TARS_INITIAL_ADMIN_PASSWORD"))
	if err != nil {
		return err
	}
	if err := store.SetPassword(consoleauth.RoleAdmin, password); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "admin account initialized for %s\n", workspaceDir)
	return err
}

func runAuthPasswd(_ context.Context, stdin io.Reader, stdout, stderr io.Writer, opts authOptions) error {
	role, err := normalizeAuthCommandRole(opts.role)
	if err != nil {
		return err
	}
	workspaceDir, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return err
	}
	password, err := resolveAuthPassword(stdin, stderr, opts.password, "")
	if err != nil {
		return err
	}
	if err := consoleauth.NewStore(workspaceDir).SetPassword(role, password); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s password updated for %s\n", role, workspaceDir)
	return err
}

func runAuthPairingCode(_ context.Context, stdout io.Writer, opts authOptions) error {
	role, err := normalizeAuthCommandRole(opts.role)
	if err != nil {
		return err
	}
	workspaceDir, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		return err
	}
	code, err := consoleauth.NewStore(workspaceDir).CreatePairingCode(role, opts.ttl)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "pairing code (%s, expires %s): %s\n", code.Role, code.ExpiresAt.Format(time.RFC3339), code.Code)
	return err
}

func resolveAuthPassword(stdin io.Reader, stderr io.Writer, explicit, fallback string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback, nil
	}
	if fd, ok := stdinTerminalFd(stdin); ok {
		return promptTerminalPassword(fd, stderr)
	}
	if err := writeAuthPrompt(stderr, "Password: "); err != nil {
		return "", err
	}
	reader := bufio.NewReader(stdin)
	password, err := reader.ReadString('\n')
	if err != nil && len(password) == 0 {
		return "", fmt.Errorf("password is required")
	}
	password = strings.TrimRight(password, "\r\n")
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password is required")
	}
	return password, nil
}

// stdinTerminalFd reports the descriptor of stdin when it is an interactive
// terminal. Pipes, files and in-memory readers (tests) keep the line-based
// path. Swappable so tests can drive the terminal path.
var stdinTerminalFd = func(stdin io.Reader) (int, bool) {
	f, ok := stdin.(*os.File)
	if !ok || f == nil {
		return 0, false
	}
	fd := int(f.Fd())
	return fd, term.IsTerminal(fd)
}

// readTerminalPassword reads one line from a terminal without echoing it.
// Swappable so tests can drive the terminal path.
var readTerminalPassword = readPasswordNoEcho

// promptTerminalPassword asks twice without echo so a typo cannot lock the
// user out of the console. term.ReadPassword swallows the Enter key, so each
// read is followed by a newline to keep the next prompt on its own line.
func promptTerminalPassword(fd int, stderr io.Writer) (string, error) {
	password, err := readTerminalPasswordLine(fd, stderr, "Password: ")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password is required")
	}
	confirm, err := readTerminalPasswordLine(fd, stderr, "Confirm password: ")
	if err != nil {
		return "", err
	}
	if confirm != password {
		return "", fmt.Errorf("passwords do not match")
	}
	return password, nil
}

func readTerminalPasswordLine(fd int, stderr io.Writer, prompt string) (string, error) {
	if err := writeAuthPrompt(stderr, prompt); err != nil {
		return "", err
	}
	raw, err := readTerminalPassword(fd)
	if werr := writeAuthPrompt(stderr, "\n"); werr != nil && err == nil {
		err = werr
	}
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

// readPasswordNoEcho wraps term.ReadPassword so that Ctrl-C while the prompt
// is waiting restores the terminal before the process exits; otherwise the
// shell is left with echo turned off.
func readPasswordNoEcho(fd int) ([]byte, error) {
	state, err := term.GetState(fd)
	if err != nil {
		return nil, err
	}
	stop := onInterrupt(func() { _ = term.Restore(fd, state) })
	defer stop()
	return term.ReadPassword(fd)
}

// exitInterrupted ends the process after Ctrl-C at a password prompt.
// Swappable so tests can observe it.
var exitInterrupted = func() {
	fmt.Fprintln(os.Stderr)
	os.Exit(130)
}

// onInterrupt runs restore and exits if Ctrl-C arrives before stop is called.
func onInterrupt(restore func()) (stop func()) {
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	done := make(chan struct{})
	go func() {
		select {
		case <-interrupted:
			restore()
			exitInterrupted()
		case <-done:
		}
	}()
	return func() {
		signal.Stop(interrupted)
		close(done)
	}
}

func writeAuthPrompt(stderr io.Writer, text string) error {
	if stderr == nil {
		return nil
	}
	_, err := fmt.Fprint(stderr, text)
	return err
}

func normalizeAuthCommandRole(role string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case consoleauth.RoleAdmin:
		return consoleauth.RoleAdmin, nil
	case consoleauth.RoleUser:
		return consoleauth.RoleUser, nil
	default:
		return "", fmt.Errorf("unsupported role %q (expected admin or user)", role)
	}
}
