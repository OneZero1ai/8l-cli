package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
	"github.com/OneZero1ai/8l-cli/pkg/version"
)

type rotateKeyFlags struct {
	Profile        string
	ConfigDir      string
	Label          string
	RevokeOld      bool
	NonInteractive bool
	Verbose        bool
}

func newRotateKeyCmd() *cobra.Command {
	f := &rotateKeyFlags{}
	cmd := &cobra.Command{
		Use:   "rotate-key",
		Short: "Mint a new API key on the L2 and update the profile in place",
		Long: `Authenticate using the existing key, mint a fresh key on the L2,
write it into the profile (atomic), and optionally revoke the old key.

This command is destructive on the L2 side when --revoke-old is set;
the new key MUST be persisted into the profile before the old key is
revoked, otherwise the session is locked out.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotateKey(cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name to rotate")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().StringVar(&f.Label, "label", "", "Optional label for the new key (defaults to `8l rotate-key <ts>`)")
	cmd.Flags().BoolVar(&f.RevokeOld, "revoke-old", true, "Revoke the previous key after the new one is persisted")
	cmd.Flags().BoolVar(&f.NonInteractive, "non-interactive", false, "Reserved for future TTY confirmation flow; honoured today as a no-op")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	return cmd
}

func runRotateKey(stdout, stderr io.Writer, f *rotateKeyFlags) error {
	p, exists, _, err := profile.Read(f.ConfigDir, f.Profile)
	if err != nil {
		return wrapCoded(ExitUnexpected, err)
	}
	if !exists {
		return wrapCoded(ExitMissingArg, fmt.Errorf("no profile %q to rotate; run `8l join` first", f.Profile))
	}
	cq := p.MCPServers["cq"]
	oldKey := cq.Env["CQ_API_KEY"]
	endpoint := cq.Env["CQ_ADDR"]

	client := l2client.New(endpoint, oldKey)
	if f.Verbose {
		client.Verbose = newVerboseLogger(stderr, true)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Auth probe — fail fast if the existing key is invalid.
	me, err := client.AuthMe(ctx)
	if err != nil {
		return wrapCoded(ExitAuthFail, fmt.Errorf("auth/me with existing key: %w", err))
	}
	oldKeyID := me.KeyID

	// 2. Mint new key.
	label := f.Label
	if label == "" {
		label = fmt.Sprintf("8l rotate-key %s", time.Now().UTC().Format(time.RFC3339))
	}
	mint, err := client.MintAPIKey(ctx, l2client.MintAPIKeyRequest{
		Label:   label,
		Persona: p.Binding.Persona,
	})
	if err != nil {
		return wrapCoded(ExitUnexpected, fmt.Errorf("mint new key: %w", err))
	}
	fmt.Fprintf(stdout, "8l: minted new key id=%s\n", mint.KeyID)

	// 3. Update the profile with the new key BEFORE revoking the old one.
	p.MCPServers["cq"] = profile.MCPServer{
		Type:    cq.Type,
		Command: cq.Command,
		Args:    cq.Args,
		Env: map[string]string{
			"CQ_ADDR":    endpoint,
			"CQ_API_KEY": mint.APIKey,
		},
	}
	p.ManagedBy = version.ManagedBy()
	p.ManagedAt = time.Now().UTC().Format(time.RFC3339)

	written, err := profile.Write(f.ConfigDir, f.Profile, p, profile.WriteOptions{Force: true})
	if err != nil {
		return wrapCoded(ExitUnexpected, fmt.Errorf("write rotated profile: %w", err))
	}
	fmt.Fprintf(stdout, "8l: wrote rotated profile %s\n", written)

	// 4. Verify with the new key.
	newClient := l2client.New(endpoint, mint.APIKey)
	if f.Verbose {
		newClient.Verbose = newVerboseLogger(stderr, true)
	}
	if _, err := newClient.AuthMe(ctx); err != nil {
		return wrapCoded(ExitAuthFail,
			fmt.Errorf("post-rotate auth/me with new key failed: %w (old key not revoked, restore manually)", err))
	}
	fmt.Fprintln(stdout, "8l: post-rotate auth/me ok")

	// 5. Revoke old key (best-effort).
	if f.RevokeOld {
		if oldKeyID == "" {
			fmt.Fprintln(stderr, "8l: warning — old key id unknown, cannot revoke (auth/me did not return key_id)")
		} else {
			if rerr := newClient.RevokeAPIKey(ctx, oldKeyID); rerr != nil {
				fmt.Fprintf(stderr, "8l: warning — revoke old key %s failed: %v (revoke manually via L2 admin)\n", oldKeyID, rerr)
			} else {
				fmt.Fprintf(stdout, "8l: revoked old key id=%s\n", oldKeyID)
			}
		}
	}

	return nil
}
