package main

import (
	"context"
	"io"
	"os"
	"strings"

	clientapp "github.com/devlikebear/tarsncase/internal/tarsclient"
	protocol "github.com/devlikebear/tarsncase/pkg/tarsclient"
	"github.com/spf13/cobra"
)

type clientOptions struct {
	serverURL  string
	sessionID  string
	apiToken   string
	adminToken string
	message    string
	verbose    bool
}

func defaultClientOptions() clientOptions {
	serverURL := strings.TrimSpace(os.Getenv("TARS_SERVER_URL"))
	if serverURL == "" {
		serverURL = protocol.DefaultServerURL
	}
	return clientOptions{
		serverURL:  serverURL,
		sessionID:  "",
		apiToken:   strings.TrimSpace(os.Getenv("TARS_API_TOKEN")),
		adminToken: strings.TrimSpace(os.Getenv("TARS_ADMIN_API_TOKEN")),
	}
}

func bindClientFlags(cmd *cobra.Command, opts *clientOptions) {
	if cmd == nil || opts == nil {
		return
	}
	cmd.Flags().StringVar(&opts.serverURL, "server-url", opts.serverURL, "tars server url")
	cmd.Flags().StringVar(&opts.sessionID, "session", opts.sessionID, "session id")
	cmd.Flags().StringVar(&opts.apiToken, "api-token", opts.apiToken, "api token")
	cmd.Flags().StringVar(&opts.adminToken, "admin-api-token", opts.adminToken, "admin api token")
	cmd.Flags().StringVar(&opts.message, "message", opts.message, "send one message and exit")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", opts.verbose, "verbose status output")
}

func runClientCommand(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, opts clientOptions) error {
	return clientapp.Run(ctx, stdin, stdout, stderr, clientapp.Options{
		ServerURL:  strings.TrimSpace(opts.serverURL),
		SessionID:  strings.TrimSpace(opts.sessionID),
		APIToken:   strings.TrimSpace(opts.apiToken),
		AdminToken: strings.TrimSpace(opts.adminToken),
		Message:    strings.TrimSpace(opts.message),
		Verbose:    opts.verbose,
	})
}
