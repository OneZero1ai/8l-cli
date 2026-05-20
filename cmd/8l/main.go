// Command 8l is the 8th-Layer.ai management CLI.
//
// V1 subcommands: join, quick, status, unjoin, doctor, rotate-key.
// See docs/decisions/29-join-cli-design.md for the contract.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/pkg/version"
)

// ExitCoder is implemented by errors that map to a specific process
// exit code. Subcommands return these for typed exits per Decision 29
// §"Exit codes".
type ExitCoder interface {
	error
	ExitCode() int
}

// codedError pairs a sentinel exit code with a wrapped cause.
type codedError struct {
	code  int
	cause error
}

func (e *codedError) Error() string { return e.cause.Error() }
func (e *codedError) Unwrap() error { return e.cause }
func (e *codedError) ExitCode() int { return e.code }

func wrapCoded(code int, err error) error {
	if err == nil {
		return nil
	}
	return &codedError{code: code, cause: err}
}

// Exit codes (Decision 29 §"Exit codes").
const (
	ExitOK              = 0
	ExitMissingArg      = 10
	ExitInvalidKey      = 11
	ExitDNSFail         = 12
	ExitAuthFail        = 13
	ExitSmokeLocalTier  = 14
	ExitProfileConflict = 15
	ExitUnexpected      = 1
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "8l",
		Short:         "8th-Layer.ai management CLI",
		Long:          `8l manages bindings between Claude Code sessions and 8th-Layer.ai L2 groups.`,
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newJoinCmd())
	root.AddCommand(newQuickCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newUnjoinCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newRotateKeyCmd())

	// cq subcommands ported from 8th-layer-agent/cli per Decision 35.
	root.AddCommand(newProposeCmd())
	root.AddCommand(newQueryCmd())
	root.AddCommand(newConfirmCmd())
	root.AddCommand(newFlagCmd())
	root.AddCommand(newDrainCmd())
	root.AddCommand(newPromptCmd())

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "8l: %s\n", err)
		var ec ExitCoder
		if errors.As(err, &ec) {
			os.Exit(ec.ExitCode())
		}
		os.Exit(ExitUnexpected)
	}
}
