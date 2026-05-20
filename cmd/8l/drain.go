package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/outbox"
	"github.com/OneZero1ai/8l-cli/internal/profile"
)

type drainFlags struct {
	Profile    string
	ConfigDir  string
	OutboxPath string
	Verbose    bool
	DryRun     bool
	Format     string
}

func newDrainCmd() *cobra.Command {
	f := &drainFlags{}
	cmd := &cobra.Command{
		Use:   "drain",
		Short: "Push outbox-queued knowledge units to the L2",
		Long: `Push knowledge units queued in the local outbox to the bound L2.

` + "`8l propose`" + ` enqueues to ~/.claude-mux/8l-outbox.jsonl when the L2
is unreachable. ` + "`8l drain`" + ` replays those entries in order;
successfully-pushed entries are removed from the file. Failures stay
queued for the next drain.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDrain(cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().StringVar(&f.OutboxPath, "outbox", outbox.DefaultPath, "Local outbox file to drain")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "Count entries without pushing")
	cmd.Flags().StringVar(&f.Format, "format", "text", "Output format: text or json")
	return cmd
}

// drainResult mirrors the upstream cq DrainResult shape closely enough
// for `--format=json` consumers to be portable.
type drainResult struct {
	Pushed   int      `json:"pushed"`
	Failed   int      `json:"failed"`
	Pending  int      `json:"pending"`
	Warnings []string `json:"warnings,omitempty"`
	DryRun   bool     `json:"dry_run,omitempty"`
}

func runDrain(stdout, stderr io.Writer, f *drainFlags) error {
	if f.Format != "text" && f.Format != "json" {
		return wrapCoded(ExitMissingArg, fmt.Errorf("unsupported format %q: must be text or json", f.Format))
	}

	entries, err := outbox.Read(f.OutboxPath)
	if err != nil {
		return wrapCoded(ExitUnexpected, err)
	}

	if f.DryRun {
		res := drainResult{DryRun: true, Pending: len(entries)}
		if f.Format == "json" {
			return writeJSON(stdout, res)
		}
		fmt.Fprintf(stdout, "Would push %d unit(s) from %s.\n", res.Pending, f.OutboxPath)
		return nil
	}

	if len(entries) == 0 {
		if f.Format == "json" {
			return writeJSON(stdout, drainResult{})
		}
		fmt.Fprintln(stdout, "Outbox empty — nothing to drain.")
		return nil
	}

	client, _, err := loadClient(f.Profile, f.ConfigDir, f.Verbose, stderr)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var (
		remaining []outbox.Entry
		res       drainResult
		authFail  error
	)
	for i, e := range entries {
		if ctxErr := ctx.Err(); ctxErr != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("context cancelled: %s", ctxErr))
			remaining = append(remaining, entries[i:]...)
			break
		}
		_, perr := client.Propose(ctx, l2client.ProposeParams{
			Summary:    e.Summary,
			Detail:     e.Detail,
			Action:     e.Action,
			Domains:    e.Domains,
			Languages:  e.Languages,
			Frameworks: e.Frameworks,
			Pattern:    e.Pattern,
		})
		if perr != nil {
			// Auth failures are fatal — every remaining entry will
			// fail the same way. Stop, preserve the rest of the
			// queue (including the current entry), and surface
			// ExitAuthFail to the caller.
			if l2client.IsAuth(perr) {
				res.Failed++
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %s (auth — aborting)", e.Summary, perr))
				remaining = append(remaining, entries[i:]...)
				authFail = perr
				break
			}
			res.Failed++
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %s", e.Summary, perr))
			remaining = append(remaining, e)
			continue
		}
		res.Pushed++
	}

	if err := outbox.Rewrite(f.OutboxPath, remaining); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("rewrite outbox: %s", err))
	}
	res.Pending = len(remaining)

	if authFail != nil {
		if f.Format == "json" {
			_ = writeJSON(stdout, res)
		} else {
			fmt.Fprintf(stdout, "Pushed %d, failed %d, %d still queued (auth fail).\n", res.Pushed, res.Failed, res.Pending)
		}
		return wrapCoded(ExitAuthFail, authFail)
	}

	if f.Format == "json" {
		return writeJSON(stdout, res)
	}
	fmt.Fprintf(stdout, "Pushed %d unit(s); %d failed; %d still queued.\n", res.Pushed, res.Failed, res.Pending)
	for _, w := range res.Warnings {
		fmt.Fprintf(stdout, "  warning: %s\n", w)
	}
	// If we failed any non-auth pushes, mark the run as non-zero so
	// CI / cron consumers can detect partial failure.
	if res.Failed > 0 {
		return wrapCoded(ExitUnexpected, errors.New("one or more outbox entries failed to push"))
	}
	return nil
}
