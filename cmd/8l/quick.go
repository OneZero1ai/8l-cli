package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/profile"
)

// quickTokenPrefix is the magic header that identifies an 8l quick-join
// bundle. The version byte after the dot lets us evolve the payload
// schema without colliding with cqa.v1.* api keys.
const quickTokenPrefix = "cqq.v1."

// quickPayload carries the four runJoin inputs in a single bundle so an
// operator can hand off "the one thing you need to paste" instead of
// four flags. The api-key is embedded verbatim, so a quick token must be
// treated as key-equivalent — see README §Quick join.
type quickPayload struct {
	Enterprise string
	L2         string
	Persona    string
	APIKey     string
}

// encodeQuickToken serializes the four join inputs as
//
//	cqq.v1.<base64url(enterprise \t l2 \t persona \t apikey)>
//
// Tab is safe as a separator: enterprise/l2/persona are DNS-label-shaped
// in canonical use, and the cqa.v1 key shape (resolver.keyShape) cannot
// contain tabs.
func encodeQuickToken(p quickPayload) string {
	raw := strings.Join([]string{p.Enterprise, p.L2, p.Persona, p.APIKey}, "\t")
	return quickTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeQuickToken is the inverse of encodeQuickToken. It validates the
// prefix and field count but leaves api-key shape validation to
// resolver.ResolveAPIKey downstream — same path as the four-flag join.
func decodeQuickToken(token string) (quickPayload, error) {
	if !strings.HasPrefix(token, quickTokenPrefix) {
		return quickPayload{}, fmt.Errorf("quick token must start with %q", quickTokenPrefix)
	}
	body := strings.TrimPrefix(token, quickTokenPrefix)
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return quickPayload{}, fmt.Errorf("quick token base64 invalid: %w", err)
	}
	parts := strings.Split(string(raw), "\t")
	if len(parts) != 4 {
		return quickPayload{}, fmt.Errorf("quick token payload must have 4 fields (got %d)", len(parts))
	}
	p := quickPayload{
		Enterprise: parts[0],
		L2:         parts[1],
		Persona:    parts[2],
		APIKey:     parts[3],
	}
	if p.Enterprise == "" || p.L2 == "" || p.Persona == "" || p.APIKey == "" {
		return quickPayload{}, fmt.Errorf("quick token payload has empty field")
	}
	return p, nil
}

// quickFlags holds the optional flags that quick still exposes. The
// four "what to bind to" values come from the positional token argument.
type quickFlags struct {
	Profile   string
	ConfigDir string
	NoSmoke   bool
	Force     bool
	Verbose   bool
	CQCommand string
}

func newQuickCmd() *cobra.Command {
	f := &quickFlags{}
	cmd := &cobra.Command{
		Use:   "quick <token>",
		Short: "Bind this session from a single quick-join token",
		Long: `Bind this session using a single quick-join token (cqq.v1.…) that
encodes enterprise, l2, persona, and api-key together. A token is
printed after a successful "8l join" so it can be handed to a teammate
who only needs to paste one thing.

A quick token embeds the raw api-key — treat it as key-equivalent.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuick(cmd.OutOrStdout(), cmd.ErrOrStderr(), f, args[0])
		},
	}
	cmd.Flags().StringVar(&f.Profile, "profile", "8l-cq", "claude-mux profile name to write")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", profile.DefaultConfigDir, "Profile directory")
	cmd.Flags().BoolVar(&f.NoSmoke, "no-smoke", false, "Skip the post-config smoke test")
	cmd.Flags().BoolVar(&f.Force, "force", false, "Overwrite existing profile if present")
	cmd.Flags().BoolVar(&f.Verbose, "verbose", false, "Print HTTP request/response previews to stderr")
	cmd.Flags().StringVar(&f.CQCommand, "cq-command", "cq", "Stdio command stamped into mcpServers.cq.command")
	return cmd
}

func runQuick(stdout, stderr io.Writer, f *quickFlags, token string) error {
	payload, err := decodeQuickToken(token)
	if err != nil {
		return wrapCoded(ExitInvalidKey, err)
	}
	return runJoin(stdout, stderr, &joinFlags{
		Enterprise: payload.Enterprise,
		L2:         payload.L2,
		Persona:    payload.Persona,
		APIKey:     payload.APIKey,
		Profile:    f.Profile,
		ConfigDir:  f.ConfigDir,
		NoSmoke:    f.NoSmoke,
		Force:      f.Force,
		Verbose:    f.Verbose,
		CQCommand:  f.CQCommand,
		// Quick never prompts — the token carries everything.
		NonInteractive: true,
	})
}
