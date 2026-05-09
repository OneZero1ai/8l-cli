// Package version exposes build-time version metadata.
//
// Values are populated via -ldflags at build time; defaults match
// what `go install` against an untagged tree would produce.
package version

import "fmt"

var (
	// Version is the semver tag, e.g. "v0.1.0".
	Version = "v0.1.0-dev"
	// Commit is the short git SHA.
	Commit = "unknown"
	// BuildDate is the RFC3339 build timestamp.
	BuildDate = "unknown"
)

// String returns a human-readable build banner.
func String() string {
	return fmt.Sprintf("8l %s (commit %s, built %s)", Version, Commit, BuildDate)
}

// ManagedBy is the value written to profile.managed_by for forensic auditability.
func ManagedBy() string {
	return fmt.Sprintf("8l join %s", Version)
}
