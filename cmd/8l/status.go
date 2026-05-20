package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
	"github.com/OneZero1ai/8l-cli/internal/resolver"
)

type statusFlags struct {
	Profile   string
	ConfigDir string
	NoProbe   bool
	Verbose   bool
	KUStats   bool
	Format    string
}

func newStatusCmd() *cobra.Command {
	f := &statusFlags{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current binding and probe its health",
		Long: `Show the current binding and probe its health.

Default output is the binding + live /auth/me + smoke propose check.
Pass --ku-stats to also pull the L2's per-Enterprise knowledge-unit
aggregate counts (the cq CLI's old ` + "`cq status`" + ` surface).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name to inspect")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().BoolVar(&f.NoProbe, "no-probe", false, "Skip the live /auth/me + propose probe")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	cmd.Flags().BoolVar(&f.KUStats, "ku-stats", false, "Also fetch /api/v1/stats (KU tier + domain counts)")
	cmd.Flags().StringVar(&f.Format, "format", "text", "Output format: text or json (json implies --ku-stats)")
	return cmd
}

func runStatus(stdout, stderr io.Writer, f *statusFlags) error {
	p, exists, mr, err := profile.Read(f.ConfigDir, f.Profile)
	if err != nil {
		return wrapCoded(ExitUnexpected, err)
	}
	if !exists {
		path, _ := profile.Path(f.ConfigDir, f.Profile)
		fmt.Fprintf(stdout, "8l: no profile at %s — run `8l join` first\n", path)
		return wrapCoded(ExitMissingArg, fmt.Errorf("no profile bound"))
	}
	if mr.Warning != "" {
		fmt.Fprintf(stderr, "8l: warning: %s\n", mr.Warning)
	}

	path, _ := profile.Path(f.ConfigDir, f.Profile)
	fmt.Fprintf(stdout, "Profile: %s\n", path)
	fmt.Fprintf(stdout, "  schema version: %d\n", p.Version)
	fmt.Fprintf(stdout, "  managed_by:     %s\n", p.ManagedBy)
	fmt.Fprintf(stdout, "  managed_at:     %s\n", p.ManagedAt)
	fmt.Fprintf(stdout, "  binding:        %s\n", p.Binding)
	cq := p.MCPServers["cq"]
	fmt.Fprintf(stdout, "  cq endpoint:    %s\n", cq.Env["CQ_ADDR"])
	fmt.Fprintf(stdout, "  cq key id:      %s\n", resolver.KeyID(cq.Env["CQ_API_KEY"]))

	if f.NoProbe {
		return nil
	}

	client := l2client.New(cq.Env["CQ_ADDR"], cq.Env["CQ_API_KEY"])
	if f.Verbose {
		client.Verbose = newVerboseLogger(stderr, true)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	me, err := client.AuthMe(ctx)
	if err != nil {
		fmt.Fprintf(stdout, "  /auth/me:       FAIL (%s)\n", err)
		return wrapCoded(ExitAuthFail, err)
	}
	fmt.Fprintf(stdout, "  /auth/me:       OK (enterprise=%s l2=%s persona=%s)\n",
		valueOr(me.EnterpriseID, p.Binding.Enterprise),
		valueOr(me.GroupID, p.Binding.L2),
		valueOr(me.Persona, p.Binding.Persona),
	)

	// Smoke propose, same shape as join.
	resp, err := client.SmokePropose(ctx, p.Binding.Persona)
	if err != nil {
		fmt.Fprintf(stdout, "  propose smoke:  FAIL (%s)\n", err)
		return wrapCoded(ExitUnexpected, err)
	}
	if !resp.SmokeOK() {
		fmt.Fprintf(stdout, "  propose smoke:  WRONG TIER (got %q want \"private\")\n", resp.Tier)
		return wrapCoded(ExitSmokeLocalTier, fmt.Errorf("smoke tier=%s", resp.Tier))
	}
	fmt.Fprintf(stdout, "  propose smoke:  OK (tier=%s unit_id=%s)\n", resp.Tier, resp.UnitID)

	if f.KUStats || f.Format == "json" {
		stats, err := client.Stats(ctx)
		if err != nil {
			fmt.Fprintf(stdout, "  /api/v1/stats:  FAIL (%s)\n", err)
			return wrapCoded(ExitUnexpected, err)
		}
		if f.Format == "json" {
			return writeJSON(stdout, stats)
		}
		fmt.Fprintf(stdout, "  total units:    %d\n", stats.TotalUnits)
		if len(stats.Tiers) > 0 {
			fmt.Fprintln(stdout, "  tiers:")
			tierNames := make([]string, 0, len(stats.Tiers))
			for t := range stats.Tiers {
				tierNames = append(tierNames, t)
			}
			sort.Strings(tierNames)
			for _, t := range tierNames {
				fmt.Fprintf(stdout, "    %-12s %d\n", t, stats.Tiers[t])
			}
		}
		if len(stats.Domains) > 0 {
			fmt.Fprintln(stdout, "  domains:")
			domNames := make([]string, 0, len(stats.Domains))
			for d := range stats.Domains {
				domNames = append(domNames, d)
			}
			sort.Strings(domNames)
			for _, d := range domNames {
				fmt.Fprintf(stdout, "    %-20s %d\n", d, stats.Domains[d])
			}
		}
	}
	return nil
}
