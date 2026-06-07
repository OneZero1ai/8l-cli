package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
	"github.com/OneZero1ai/8l-cli/internal/resolver"
	"github.com/OneZero1ai/8l-cli/pkg/version"
)

// joinFlags collects the flag values for the join command. Encapsulating
// them makes the implementation easier to unit-test (we can call
// runJoin(joinFlags{...}) without invoking cobra).
type joinFlags struct {
	Enterprise     string
	L2             string
	Persona        string
	APIKey         string
	Profile        string
	ConfigDir      string
	NonInteractive bool
	NoSmoke        bool
	Force          bool
	Verbose        bool
	// CQCommand is the stdio command stamped into mcpServers.cq.command.
	// Defaults to "cq" — operators can override for cq-cli installs in
	// non-PATH locations via --cq-command.
	CQCommand string
}

func newJoinCmd() *cobra.Command {
	f := &joinFlags{}
	cmd := &cobra.Command{
		Use:   "join",
		Short: "Bind this session to an 8th-Layer L2 group",
		Long: `Bind this session to an 8th-Layer L2 group by writing a claude-mux profile and smoke-testing the binding.

The CLI is idempotent: rerunning with the same (enterprise, l2, persona, profile)
quadruple is a no-op when the existing profile already matches.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJoin(cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}

	cmd.Flags().StringVar(&f.Enterprise, "enterprise", "", "Enterprise ID, e.g. 8th-layer-corp")
	cmd.Flags().StringVar(&f.L2, "l2", "", "L2 / group ID, e.g. engineering")
	cmd.Flags().StringVar(&f.Persona, "persona", "", "Persona name within the L2")
	cmd.Flags().StringVar(&f.APIKey, "api-key", "", "API key (cqa.v1.…) or $ENV_VAR")
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name to write")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().BoolVar(&f.NonInteractive, "non-interactive", false, "Skip prompts; fail if any required value is missing")
	cmd.Flags().BoolVar(&f.NoSmoke, "no-smoke", false, "Skip the post-config smoke test")
	cmd.Flags().BoolVar(&f.Force, "force", false, "Overwrite existing profile if present")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	cmd.Flags().StringVar(&f.CQCommand, "cq-command", "cq", "Stdio command stamped into mcpServers.cq.command")

	return cmd
}

func runJoin(stdout, stderr io.Writer, f *joinFlags) error {
	// In non-interactive mode (or when stdin isn't a TTY) we treat any
	// missing required arg as a fatal exit-10. Interactive prompting is
	// not implemented in V1; the wiring is here so a future iteration
	// can plug in survey/promptui without changing the contract.
	for _, p := range []struct {
		name, val string
	}{
		{"enterprise", f.Enterprise},
		{"l2", f.L2},
		{"persona", f.Persona},
		{"api-key", f.APIKey},
	} {
		if p.val == "" {
			return wrapCoded(ExitMissingArg, fmt.Errorf("missing required --%s", p.name))
		}
	}

	if f.CQCommand == "" {
		f.CQCommand = "cq"
	}
	apiKey, err := resolver.ResolveAPIKey(f.APIKey)
	if err != nil {
		return wrapCoded(ExitInvalidKey, err)
	}
	endpoint, err := resolveEndpoint(stdout, stderr, f, apiKey)
	if err != nil {
		return err
	}

	desired := &profile.Profile{
		Version:   profile.SchemaVersion,
		ManagedBy: version.ManagedBy(),
		ManagedAt: time.Now().UTC().Format(time.RFC3339),
		Binding: profile.Binding{
			Enterprise: f.Enterprise,
			L2:         f.L2,
			Persona:    f.Persona,
		},
		MCPServers: map[string]profile.MCPServer{
			"cq": {
				Type:    "stdio",
				Command: f.CQCommand,
				Env: map[string]string{
					"CQ_ADDR":    endpoint,
					"CQ_API_KEY": apiKey,
				},
			},
		},
	}
	if err := desired.Validate(); err != nil {
		return wrapCoded(ExitMissingArg, err)
	}

	// Idempotency check: if existing profile matches binding + endpoint
	// + key, return early without re-writing.
	existing, exists, _, readErr := profile.Read(f.ConfigDir, f.Profile)
	if readErr != nil && exists {
		// Corrupt or schema-mismatch existing file. Block unless --force.
		if !f.Force {
			return wrapCoded(ExitProfileConflict,
				fmt.Errorf("existing profile unreadable: %w (use --force to overwrite)", readErr))
		}
	}
	if existing != nil && exists {
		// Same binding + same env values → idempotent.
		if existing.Binding.Equal(desired.Binding) &&
			existing.MCPServers["cq"].Env["CQ_ADDR"] == endpoint &&
			existing.MCPServers["cq"].Env["CQ_API_KEY"] == apiKey {
			fmt.Fprintf(stdout, "8l: profile %q already bound to %s — no-op\n",
				f.Profile, desired.Binding)
			if !f.NoSmoke {
				return runSmoke(stdout, stderr, f, endpoint, apiKey)
			}
			return nil
		}
		// Different binding. Block unless --force.
		if !existing.Binding.Equal(desired.Binding) && !f.Force {
			return wrapCoded(ExitProfileConflict,
				fmt.Errorf("profile %q bound to %s; refusing to rebind to %s without --force",
					f.Profile, existing.Binding, desired.Binding))
		}
	}

	// Pre-flight smoke BEFORE writing: avoids leaving a partially-applied
	// profile when auth fails. We probe /auth/me first; if it succeeds,
	// write profile, then run the propose smoke. If propose fails, roll
	// back the profile to its prior state.
	logger := newVerboseLogger(stderr, f.Verbose)
	client := l2client.New(endpoint, apiKey)
	client.Verbose = logger

	if !f.NoSmoke {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		me, err := client.AuthMe(ctx)
		if err != nil {
			return classifyAuthError(err)
		}
		if me.EnterpriseID != "" && me.EnterpriseID != f.Enterprise {
			return wrapCoded(ExitAuthFail, fmt.Errorf(
				"auth/me enterprise_id=%q does not match --enterprise=%q (key bound to a different tenant)",
				me.EnterpriseID, f.Enterprise))
		}
		if me.GroupID != "" && me.GroupID != f.L2 {
			return wrapCoded(ExitAuthFail, fmt.Errorf(
				"auth/me group_id=%q does not match --l2=%q (key bound to a different L2)",
				me.GroupID, f.L2))
		}
	}

	// Snapshot existing profile bytes so we can roll back on smoke failure.
	var rollback []byte
	if exists && readErr == nil {
		path, perr := profile.Path(f.ConfigDir, f.Profile)
		if perr == nil {
			if data, rerr := os.ReadFile(path); rerr == nil {
				rollback = data
			}
		}
	}

	written, err := profile.Write(f.ConfigDir, f.Profile, desired, profile.WriteOptions{Force: f.Force})
	if err != nil {
		// Distinguish profile conflict from generic write errors.
		if strings.Contains(err.Error(), "already bound") || strings.Contains(err.Error(), "not written by 8l") {
			return wrapCoded(ExitProfileConflict, err)
		}
		return wrapCoded(ExitUnexpected, err)
	}
	fmt.Fprintf(stdout, "8l: wrote profile %s\n", written)

	if f.NoSmoke {
		fmt.Fprintln(stdout, "8l: --no-smoke set; skipping post-write probe")
		return nil
	}

	if err := runSmoke(stdout, stderr, f, endpoint, apiKey); err != nil {
		// Roll back on smoke failure so we don't leave the user in a
		// half-applied state.
		if rollback != nil {
			path, _ := profile.Path(f.ConfigDir, f.Profile)
			_ = os.WriteFile(path, rollback, 0o600)
			fmt.Fprintf(stderr, "8l: smoke failed, restored prior profile at %s\n", path)
		} else {
			path, _ := profile.Path(f.ConfigDir, f.Profile)
			_ = os.Remove(path)
			fmt.Fprintf(stderr, "8l: smoke failed, removed half-applied profile at %s\n", path)
		}
		return err
	}
	return nil
}

// resolveEndpoint determines the L2 base URL (issue #204). It prefers the
// directory's recorded URL, then the known URL schemes (route53 edge, then
// legacy), and VALIDATES each candidate with the API key against /auth/me — the
// key is ground truth, so a wrong or stale directory value can never bind us to
// the wrong host. CQ_ADDR_OVERRIDE short-circuits to a single candidate.
func resolveEndpoint(stdout, stderr io.Writer, f *joinFlags, apiKey string) (string, error) {
	cands, err := resolver.Candidates(f.Enterprise, f.L2)
	if err != nil {
		return "", wrapCoded(ExitMissingArg, err)
	}
	// Best-effort: the directory's recorded URL goes first (authoritative once
	// the L2 has announced). No-op under CQ_ADDR_OVERRIDE or on any failure.
	dctx, dcancel := context.WithTimeout(context.Background(), 10*time.Second)
	d := resolver.DirectoryEndpoint(dctx, f.Enterprise)
	dcancel()
	if d != "" && d != cands[0] {
		cands = append([]string{d}, cands...)
	}

	// Without a smoke probe we cannot validate; use the most-likely candidate.
	if f.NoSmoke {
		return cands[0], nil
	}

	logger := newVerboseLogger(stderr, f.Verbose)
	var lastErr error
	for _, base := range cands {
		client := l2client.New(base, apiKey)
		client.Verbose = logger
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		me, err := client.AuthMe(ctx)
		cancel()
		if err == nil {
			if me.EnterpriseID != "" && me.EnterpriseID != f.Enterprise {
				return "", wrapCoded(ExitAuthFail, fmt.Errorf(
					"auth/me enterprise_id=%q does not match --enterprise=%q at %s (key bound to a different tenant)",
					me.EnterpriseID, f.Enterprise, base))
			}
			fmt.Fprintf(stderr, "8l: resolved L2 endpoint %s\n", base)
			return base, nil
		}
		// An auth rejection means we reached a real L2 (it served /auth/me) but
		// the key/tenant is wrong — stop probing and report it, don't fall through
		// to a different host with the same (bad) key.
		if l2client.IsAuth(err) {
			return "", classifyAuthError(err)
		}
		lastErr = err
	}
	return "", wrapCoded(ExitDNSFail, fmt.Errorf(
		"could not reach the L2 for %s/%s at any known URL (last error: %v); "+
			"set CQ_ADDR_OVERRIDE to the L2's real https URL and retry",
		f.Enterprise, f.L2, lastErr))
}

// runSmoke does the post-write probe sequence.
func runSmoke(stdout, stderr io.Writer, f *joinFlags, endpoint, apiKey string) error {
	logger := newVerboseLogger(stderr, f.Verbose)
	client := l2client.New(endpoint, apiKey)
	client.Verbose = logger

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	me, err := client.AuthMe(ctx)
	if err != nil {
		return classifyAuthError(err)
	}
	fmt.Fprintf(stdout, "8l: /auth/me ok (enterprise=%s l2=%s persona=%s)\n",
		valueOr(me.EnterpriseID, f.Enterprise),
		valueOr(me.GroupID, f.L2),
		valueOr(me.Persona, f.Persona),
	)

	resp, err := client.SmokePropose(ctx, f.Persona)
	if err != nil {
		return wrapCoded(ExitUnexpected, fmt.Errorf("propose smoke failed: %w", err))
	}
	if !resp.SmokeOK() {
		return wrapCoded(ExitSmokeLocalTier, fmt.Errorf(
			"propose smoke returned tier=%q (expected 'private') — binding not in effect", resp.Tier))
	}
	fmt.Fprintf(stdout, "8l: smoke ok — propose tier=%s unit_id=%s\n", resp.Tier, resp.UnitID)
	fmt.Fprintf(stdout, "8l: bound %s/%s/%s via %s\n", f.Enterprise, f.L2, f.Persona, endpoint)
	token := encodeQuickToken(quickPayload{
		Enterprise: f.Enterprise,
		L2:         f.L2,
		Persona:    f.Persona,
		APIKey:     apiKey,
	})
	fmt.Fprintf(stdout, "8l: quick-join token (key-equivalent, share like the api-key):\n    %s\n", token)
	return nil
}

func classifyAuthError(err error) error {
	if l2client.IsAuth(err) {
		return wrapCoded(ExitAuthFail, fmt.Errorf("/auth/me rejected: %w", err))
	}
	// Network / DNS classification: cobra-friendly error mapping.
	msg := err.Error()
	if strings.Contains(msg, "no such host") || strings.Contains(msg, "DNS") {
		return wrapCoded(ExitDNSFail, err)
	}
	return wrapCoded(ExitAuthFail, err)
}

func valueOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// newVerboseLogger returns a logger that writes to stderr if --verbose,
// else nil (which the l2client treats as silence).
func newVerboseLogger(w io.Writer, verbose bool) l2client.VerboseLogger {
	if !verbose {
		return nil
	}
	return log.New(w, "8l: ", 0)
}
