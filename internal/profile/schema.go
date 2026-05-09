// Package profile reads, writes, and validates claude-mux profile JSON
// files managed by `8l join`.
//
// V1 schema is documented in docs/decisions/29-join-cli-design.md.
package profile

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// SchemaVersion is the current profile schema version emitted by writers.
//
// Versioning policy (Decision 29 §2): readers MUST accept SchemaVersion
// AND SchemaVersion-1 (with deprecation warning). Anything older is
// refused — operators must run a migration.
const SchemaVersion = 1

// MinReadableVersion is the oldest schema version we can read. Bump when
// a breaking change ships and we drop support for N-2.
const MinReadableVersion = 1

// Profile is the on-disk JSON representation of a binding.
//
// All fields are intentionally non-pointer; absent fields decode to zero
// values which Validate() then rejects.
type Profile struct {
	Version    int                  `json:"version"`
	ManagedBy  string               `json:"managed_by"`
	ManagedAt  string               `json:"managed_at"`
	Binding    Binding              `json:"binding"`
	MCPServers map[string]MCPServer `json:"mcpServers"`
}

// Binding identifies the (enterprise, l2, persona) tuple this profile
// is wired to. Used by `8l join` for idempotency checks.
type Binding struct {
	Enterprise string `json:"enterprise"`
	L2         string `json:"l2"`
	Persona    string `json:"persona"`
}

// MCPServer mirrors the claude-mux mcpServers entry shape.
type MCPServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Equal reports whether two bindings target the same (enterprise, l2, persona)
// triple. Used for idempotency.
func (b Binding) Equal(other Binding) bool {
	return b.Enterprise == other.Enterprise &&
		b.L2 == other.L2 &&
		b.Persona == other.Persona
}

// String renders a binding for log output.
func (b Binding) String() string {
	return fmt.Sprintf("%s/%s/%s", b.Enterprise, b.L2, b.Persona)
}

// idRegexp matches DNS-safe enterprise / l2 / persona identifiers.
// Conservative; tightened later if needed.
var idRegexp = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// Validate checks that a profile loaded from disk is well-formed.
//
// Returns nil if the profile passes; an error describing the first
// failed invariant otherwise.
func (p *Profile) Validate() error {
	if p == nil {
		return errors.New("profile: nil")
	}
	if p.Version < MinReadableVersion {
		return fmt.Errorf("profile: schema version %d unsupported (min %d)",
			p.Version, MinReadableVersion)
	}
	if p.Version > SchemaVersion {
		return fmt.Errorf("profile: schema version %d newer than this CLI supports (max %d) — upgrade 8l",
			p.Version, SchemaVersion)
	}
	if p.ManagedBy == "" {
		return errors.New("profile: managed_by required")
	}
	if p.ManagedAt != "" {
		if _, err := time.Parse(time.RFC3339, p.ManagedAt); err != nil {
			return fmt.Errorf("profile: managed_at not RFC3339: %w", err)
		}
	}
	if !idRegexp.MatchString(p.Binding.Enterprise) {
		return fmt.Errorf("profile: invalid enterprise %q", p.Binding.Enterprise)
	}
	if !idRegexp.MatchString(p.Binding.L2) {
		return fmt.Errorf("profile: invalid l2 %q", p.Binding.L2)
	}
	if !idRegexp.MatchString(p.Binding.Persona) {
		return fmt.Errorf("profile: invalid persona %q", p.Binding.Persona)
	}
	if len(p.MCPServers) == 0 {
		return errors.New("profile: mcpServers required (at least one entry)")
	}
	cq, ok := p.MCPServers["cq"]
	if !ok {
		return errors.New("profile: mcpServers.cq entry required")
	}
	if cq.Type == "" {
		return errors.New("profile: mcpServers.cq.type required")
	}
	if cq.Command == "" {
		return errors.New("profile: mcpServers.cq.command required")
	}
	if cq.Env["CQ_ADDR"] == "" {
		return errors.New("profile: mcpServers.cq.env.CQ_ADDR required")
	}
	if cq.Env["CQ_API_KEY"] == "" {
		return errors.New("profile: mcpServers.cq.env.CQ_API_KEY required")
	}
	return nil
}

// IsManagedBy8l reports whether this profile was last written by an `8l`
// CLI invocation. Profiles written by a human or another tool are
// protected from overwrite unless --force is set.
var managedBy8lRegexp = regexp.MustCompile(`^8l join `)

func (p *Profile) IsManagedBy8l() bool {
	return managedBy8lRegexp.MatchString(p.ManagedBy)
}
