package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
)

type confirmFlags struct {
	Profile   string
	ConfigDir string
	Verbose   bool
	Format    string
}

func newConfirmCmd() *cobra.Command {
	f := &confirmFlags{}
	cmd := &cobra.Command{
		Use:   "confirm <unit_id>",
		Short: "Confirm a knowledge unit proved correct, boosting its confidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfirm(cmd.OutOrStdout(), cmd.ErrOrStderr(), f, args[0])
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	cmd.Flags().StringVar(&f.Format, "format", "text", "Output format: text or json")
	return cmd
}

func runConfirm(stdout, stderr io.Writer, f *confirmFlags, unitID string) error {
	if f.Format != "text" && f.Format != "json" {
		return wrapCoded(ExitMissingArg, fmt.Errorf("unsupported format %q: must be text or json", f.Format))
	}
	client, _, err := loadClient(f.Profile, f.ConfigDir, f.Verbose, stderr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ku, err := client.Confirm(ctx, unitID)
	if err != nil {
		if l2client.IsAuth(err) {
			return wrapCoded(ExitAuthFail, err)
		}
		return wrapCoded(ExitUnexpected, err)
	}

	if f.Format == "json" {
		return writeJSON(stdout, ku)
	}
	fmt.Fprintf(stdout, "Confirmed %s (confidence: %.0f%%)\n", ku.ID, ku.Evidence.Confidence*100)
	return nil
}
