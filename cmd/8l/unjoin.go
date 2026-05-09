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

type unjoinFlags struct {
	Profile   string
	ConfigDir string
	Revoke    bool
	Yes       bool
	Verbose   bool
}

func newUnjoinCmd() *cobra.Command {
	f := &unjoinFlags{}
	cmd := &cobra.Command{
		Use:   "unjoin",
		Short: "Remove the local binding (delete the profile)",
		Long: `Delete the claude-mux profile written by 8l join.

By default, only the local profile is removed. With --revoke the CLI also
calls the L2 to revoke the API key the profile is using; the operator
needs the key's revoke authority for that to succeed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnjoin(cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name to remove")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().BoolVar(&f.Revoke, "revoke", false, "Also revoke the API key on the L2 side")
	cmd.Flags().BoolVar(&f.Yes, "yes", false, "Suppress the confirmation prompt (required for --revoke in non-interactive use)")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	return cmd
}

func runUnjoin(stdout, stderr io.Writer, f *unjoinFlags) error {
	p, exists, _, err := profile.Read(f.ConfigDir, f.Profile)
	if err != nil && !exists {
		return wrapCoded(ExitUnexpected, err)
	}

	if f.Revoke {
		if !exists || p == nil {
			return wrapCoded(ExitMissingArg,
				fmt.Errorf("--revoke requires a readable profile to locate the key id"))
		}
		if !f.Yes {
			return wrapCoded(ExitMissingArg,
				fmt.Errorf("--revoke is destructive on the L2; pass --yes to confirm"))
		}
		cq := p.MCPServers["cq"]
		client := l2client.New(cq.Env["CQ_ADDR"], cq.Env["CQ_API_KEY"])
		if f.Verbose {
			client.Verbose = newVerboseLogger(stderr, true)
		}
		// The auth/me probe gives us the canonical key id even when the
		// profile didn't store one (older versions might not have).
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		me, merr := client.AuthMe(ctx)
		if merr != nil {
			return wrapCoded(ExitAuthFail, fmt.Errorf("auth/me before revoke: %w", merr))
		}
		if me.KeyID == "" {
			return wrapCoded(ExitUnexpected,
				fmt.Errorf("auth/me did not return key_id; cannot revoke (delete the profile and ask the L2 admin)"))
		}
		if rerr := client.RevokeAPIKey(ctx, me.KeyID); rerr != nil {
			return wrapCoded(ExitUnexpected, fmt.Errorf("revoke %s: %w", me.KeyID, rerr))
		}
		fmt.Fprintf(stdout, "8l: revoked key id %s\n", me.KeyID)
	}

	path, deleted, derr := profile.Delete(f.ConfigDir, f.Profile)
	if derr != nil {
		return wrapCoded(ExitUnexpected, derr)
	}
	if !deleted {
		fmt.Fprintf(stdout, "8l: no profile at %s — nothing to remove\n", path)
		return nil
	}
	fmt.Fprintf(stdout, "8l: removed profile %s\n", path)
	return nil
}
