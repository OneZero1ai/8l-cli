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

type queryFlags struct {
	Profile    string
	ConfigDir  string
	Verbose    bool
	Domains    []string
	Languages  []string
	Frameworks []string
	Pattern    string
	Limit      int
	Format     string
}

func newQueryCmd() *cobra.Command {
	f := &queryFlags{}
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Search knowledge units by domain tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	cmd.Flags().StringArrayVar(&f.Domains, "domain", nil, "Domain tag (repeatable; at least one required)")
	cmd.Flags().StringArrayVar(&f.Languages, "language", nil, "Filter by language (repeatable)")
	cmd.Flags().StringArrayVar(&f.Frameworks, "framework", nil, "Filter by framework (repeatable)")
	cmd.Flags().StringVar(&f.Pattern, "pattern", "", "Filter by pattern")
	cmd.Flags().IntVar(&f.Limit, "limit", 5, "Maximum results (server caps at 50)")
	cmd.Flags().StringVar(&f.Format, "format", "text", "Output format: text or json")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func runQuery(stdout, stderr io.Writer, f *queryFlags) error {
	if f.Format != "text" && f.Format != "json" {
		return wrapCoded(ExitMissingArg, fmt.Errorf("unsupported format %q: must be text or json", f.Format))
	}
	client, _, err := loadClient(f.Profile, f.ConfigDir, f.Verbose, stderr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	units, err := client.Query(ctx, l2client.QueryParams{
		Domains:    f.Domains,
		Languages:  f.Languages,
		Frameworks: f.Frameworks,
		Pattern:    f.Pattern,
		Limit:      f.Limit,
	})
	if err != nil {
		if l2client.IsAuth(err) {
			return wrapCoded(ExitAuthFail, err)
		}
		return wrapCoded(ExitUnexpected, err)
	}

	if f.Format == "json" {
		// Always emit an array, even if empty, so jq pipelines don't break.
		if units == nil {
			units = []l2client.KnowledgeUnit{}
		}
		return writeJSON(stdout, units)
	}
	if len(units) == 0 {
		fmt.Fprintln(stdout, "No matching knowledge units found.")
		return nil
	}
	for _, ku := range units {
		fmt.Fprintf(stdout, "[%s] (%.0f%%) %s\n", ku.ID, ku.Evidence.Confidence*100, ku.Insight.Summary)
		fmt.Fprintf(stdout, "  %s\n", ku.Insight.Detail)
		fmt.Fprintf(stdout, "  Action: %s\n\n", ku.Insight.Action)
	}
	return nil
}
