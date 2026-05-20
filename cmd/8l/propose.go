package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/outbox"
	"github.com/OneZero1ai/8l-cli/internal/profile"
)

type proposeFlags struct {
	Profile    string
	ConfigDir  string
	OutboxPath string
	Verbose    bool

	Summary    string
	Detail     string
	Action     string
	Domains    []string
	Languages  []string
	Frameworks []string
	Pattern    string
	Format     string
}

func newProposeCmd() *cobra.Command {
	f := &proposeFlags{}
	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Propose a new knowledge unit to the L2",
		Long: `Propose a new knowledge unit to the bound L2 group.

The unit's tier is set server-side (defaults to "private" — the L2
admin can flip via CQ_DEFAULT_KU_TIER). On L2 unreachable the propose
is appended to the local outbox (` + outbox.DefaultPath + `) and
drained later via ` + "`8l drain`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPropose(cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().StringVar(&f.OutboxPath, "outbox", outbox.DefaultPath, "Local outbox file (used on L2 unreachable)")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	cmd.Flags().StringVar(&f.Summary, "summary", "", "Brief summary of the insight (required)")
	cmd.Flags().StringVar(&f.Detail, "detail", "", "Detailed explanation (required)")
	cmd.Flags().StringVar(&f.Action, "action", "", "Recommended action (required)")
	cmd.Flags().StringArrayVar(&f.Domains, "domain", nil, "Domain tag (repeatable; at least one required)")
	cmd.Flags().StringArrayVar(&f.Languages, "language", nil, "Programming language context (repeatable)")
	cmd.Flags().StringArrayVar(&f.Frameworks, "framework", nil, "Framework context (repeatable)")
	cmd.Flags().StringVar(&f.Pattern, "pattern", "", "Pattern context")
	cmd.Flags().StringVar(&f.Format, "format", "text", "Output format: text or json")
	_ = cmd.MarkFlagRequired("summary")
	_ = cmd.MarkFlagRequired("detail")
	_ = cmd.MarkFlagRequired("action")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func runPropose(stdout, stderr io.Writer, f *proposeFlags) error {
	if f.Format != "text" && f.Format != "json" {
		return wrapCoded(ExitMissingArg, fmt.Errorf("unsupported format %q: must be text or json", f.Format))
	}

	client, _, err := loadClient(f.Profile, f.ConfigDir, f.Verbose, stderr)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	params := l2client.ProposeParams{
		Summary:    f.Summary,
		Detail:     f.Detail,
		Action:     f.Action,
		Domains:    f.Domains,
		Languages:  f.Languages,
		Frameworks: f.Frameworks,
		Pattern:    f.Pattern,
	}

	ku, err := client.Propose(ctx, params)
	if err != nil {
		// Auth errors: don't enqueue — they'd just fail again.
		// Other unreachable / 5xx: queue to outbox.
		if l2client.IsAuth(err) {
			return wrapCoded(ExitAuthFail, err)
		}
		if isUnreachable(err) || is5xx(err) {
			entry := outbox.Entry{
				EnqueuedAt: time.Now().UTC(),
				Domains:    f.Domains,
				Summary:    f.Summary,
				Detail:     f.Detail,
				Action:     f.Action,
				Languages:  f.Languages,
				Frameworks: f.Frameworks,
				Pattern:    f.Pattern,
			}
			if qerr := outbox.Append(f.OutboxPath, entry); qerr != nil {
				return wrapCoded(ExitUnexpected, errors.Join(err, qerr))
			}
			fmt.Fprintf(stderr, "8l: warning: L2 unreachable (%s) — queued to %s; run `8l drain` to retry\n", err, f.OutboxPath)
			if f.Format == "json" {
				return writeJSON(stdout, map[string]any{
					"queued":  true,
					"reason":  err.Error(),
					"outbox":  f.OutboxPath,
					"summary": f.Summary,
				})
			}
			fmt.Fprintf(stdout, "Queued to %s (L2 unreachable)\n", f.OutboxPath)
			return nil
		}
		return wrapCoded(ExitUnexpected, err)
	}

	if f.Format == "json" {
		return writeJSON(stdout, ku)
	}
	fmt.Fprintf(stdout, "Proposed: %s\n", ku.ID)
	return nil
}

// writeJSON renders v as indented JSON to w. Shared by every cq
// subcommand for the --format=json path.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// isUnreachable reports whether err looks like a transport-level
// failure (DNS, dial, TLS, EOF) — i.e. the server never spoke. The
// l2client.do() wraps these via fmt.Errorf so we test by absence of
// HTTPError.
func isUnreachable(err error) bool {
	var he *l2client.HTTPError
	return !errors.As(err, &he)
}

// is5xx reports whether err is an HTTPError with a 5xx status.
func is5xx(err error) bool {
	var he *l2client.HTTPError
	if !errors.As(err, &he) {
		return false
	}
	return he.StatusCode >= 500
}
