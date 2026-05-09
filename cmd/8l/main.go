// Command 8l is the 8th-Layer.ai management CLI.
//
// V1 subcommands: join, status, unjoin, doctor, rotate-key. See
// docs/decisions/29-join-cli-design.md (on OneZero1ai/8th-layer-agent)
// for the contract.
//
// This file is a scaffold; subcommand implementations land in the
// V1 implementation PR.
package main

import (
	"fmt"

	"github.com/OneZero1ai/8l-cli/pkg/version"
)

func main() {
	fmt.Println(version.String())
	fmt.Println("scaffold-only build — V1 implementation pending")
}
