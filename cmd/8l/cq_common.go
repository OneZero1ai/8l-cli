package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
)

// errNoProfile is the sentinel returned when the requested profile has
// not been provisioned by `8l join`. Subcommands surface this as a
// pointer to `8l join`, not a stack trace.
var errNoProfile = errors.New("no profile bound — run `8l join` first")

// loadClient is the common entry path for every cq subcommand
// (propose / query / confirm / flag / status — drain too). It reads
// the profile, wires up an l2client.Client, and applies --verbose if
// set.
//
// On a missing / empty profile it returns errNoProfile wrapped with
// ExitMissingArg so the user gets a clean "run join first" error
// instead of garbage HTTP failures.
func loadClient(profileName, configDir string, verbose bool, stderr io.Writer) (*l2client.Client, *profile.Profile, error) {
	p, exists, mr, err := profile.Read(configDir, profileName)
	if err != nil {
		return nil, nil, wrapCoded(ExitUnexpected, err)
	}
	if !exists {
		path, _ := profile.Path(configDir, profileName)
		return nil, nil, wrapCoded(ExitMissingArg, fmt.Errorf("%w (looked at %s)", errNoProfile, path))
	}
	if mr.Warning != "" {
		fmt.Fprintf(stderr, "8l: warning: %s\n", mr.Warning)
	}
	cq, ok := p.MCPServers["cq"]
	if !ok {
		return nil, nil, wrapCoded(ExitUnexpected, errors.New("profile: missing mcpServers.cq entry"))
	}
	addr := cq.Env["CQ_ADDR"]
	apiKey := cq.Env["CQ_API_KEY"]
	if addr == "" || apiKey == "" {
		return nil, nil, wrapCoded(ExitUnexpected, errors.New("profile: cq.env.CQ_ADDR or CQ_API_KEY empty"))
	}
	client := l2client.New(addr, apiKey)
	if verbose {
		client.Verbose = newVerboseLogger(stderr, true)
	}
	return client, p, nil
}
