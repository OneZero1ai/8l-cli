package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
)

type flagFlags struct {
	Profile     string
	ConfigDir   string
	Verbose     bool
	Reason      string
	Detail      string
	DuplicateOf string
	Format      string
}

func newFlagCmd() *cobra.Command {
	f := &flagFlags{}
	cmd := &cobra.Command{
		Use:   "flag <unit_id>",
		Short: "Flag a knowledge unit as problematic, reducing its confidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFlag(cmd.OutOrStdout(), cmd.ErrOrStderr(), f, args[0])
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	cmd.Flags().StringVar(&f.Reason, "reason", "", fmt.Sprintf("Flag reason (one of: %s)", strings.Join(l2client.AllFlagReasons(), ", ")))
	cmd.Flags().StringVar(&f.Detail, "detail", "", "Optional detail explaining the flag")
	cmd.Flags().StringVar(&f.DuplicateOf, "duplicate-of", "", "Original unit id when reason is duplicate")
	cmd.Flags().StringVar(&f.Format, "format", "text", "Output format: text or json")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func runFlag(stdout, stderr io.Writer, f *flagFlags, unitID string) error {
	if f.Format != "text" && f.Format != "json" {
		return wrapCoded(ExitMissingArg, fmt.Errorf("unsupported format %q: must be text or json", f.Format))
	}
	reason := strings.ToLower(strings.TrimSpace(f.Reason))
	if !validFlagReason(reason) {
		return wrapCoded(ExitMissingArg, fmt.Errorf("invalid reason %q; must be one of: %s",
			f.Reason, strings.Join(l2client.AllFlagReasons(), ", ")))
	}
	if reason == l2client.FlagReasonDuplicate && f.DuplicateOf == "" {
		return wrapCoded(ExitMissingArg, fmt.Errorf("--duplicate-of is required when --reason=duplicate"))
	}

	client, _, err := loadClient(f.Profile, f.ConfigDir, f.Verbose, stderr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ku, err := client.FlagUnit(ctx, unitID, l2client.FlagRequest{
		Reason:      reason,
		Detail:      f.Detail,
		DuplicateOf: f.DuplicateOf,
	})
	if err != nil {
		if l2client.IsAuth(err) {
			return wrapCoded(ExitAuthFail, err)
		}
		return wrapCoded(ExitUnexpected, err)
	}

	if f.Format == "json" {
		return writeJSON(stdout, ku)
	}
	fmt.Fprintf(stdout, "Flagged %s as %s (confidence: %.0f%%)\n", ku.ID, reason, ku.Evidence.Confidence*100)
	return nil
}

func validFlagReason(r string) bool {
	for _, v := range l2client.AllFlagReasons() {
		if v == r {
			return true
		}
	}
	return false
}
