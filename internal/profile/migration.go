package profile

import (
	"encoding/json"
	"fmt"
)

// MigrateResult is returned by Migrate to signal whether any
// schema migration ran and (if so) which deprecation message
// the caller should surface.
type MigrateResult struct {
	// Migrated is true when the caller should re-write the file
	// with the migrated content (typically on next save).
	Migrated bool
	// Warning is non-empty when the loaded profile is on an older
	// readable version; print it to stderr.
	Warning string
}

// Migrate inspects raw JSON bytes, detects the schema version,
// and produces a current-version *Profile.
//
// V1 (current): pass-through.
// V0 (placeholder): no real V0 ever shipped; the wiring is here so
//
//	the FIRST breaking change has a forward-compatible reader and
//	doesn't need to retrofit the deserializer (the expensive bit).
func Migrate(raw []byte) (*Profile, MigrateResult, error) {
	// Peek the version field without committing to a full struct,
	// so a future V2 with renamed fields can still report a clean
	// "version newer than CLI" message.
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, MigrateResult{}, fmt.Errorf("profile: parse version: %w", err)
	}

	switch {
	case probe.Version == 0:
		// No `version` field at all OR explicit version=0. Treated as
		// pre-V1 prototype; refuse with clear migration hint.
		return nil, MigrateResult{}, fmt.Errorf(
			"profile: missing or zero schema version — this profile predates 8l-cli V1; delete and re-run `8l join`")
	case probe.Version == SchemaVersion:
		// Current path.
		var p Profile
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, MigrateResult{}, fmt.Errorf("profile: decode v%d: %w", probe.Version, err)
		}
		return &p, MigrateResult{Migrated: false}, nil
	case probe.Version >= MinReadableVersion && probe.Version < SchemaVersion:
		// Readable but deprecated. There are no such versions today;
		// when SchemaVersion bumps to 2, add a v1 → v2 transform here.
		var p Profile
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, MigrateResult{}, fmt.Errorf("profile: decode v%d: %w", probe.Version, err)
		}
		// PLACEHOLDER: real per-version transforms slot in here:
		//   if probe.Version < 2 { migrateV1ToV2(&p) }
		// At V1 there's nothing to do.
		warn := fmt.Sprintf("profile schema version %d is deprecated; will be re-written as v%d on next save",
			probe.Version, SchemaVersion)
		return &p, MigrateResult{Migrated: true, Warning: warn}, nil
	default:
		// probe.Version > SchemaVersion or < MinReadableVersion-by-some-other-rule.
		return nil, MigrateResult{}, fmt.Errorf(
			"profile: schema version %d not readable by this 8l-cli (supports v%d..v%d) — upgrade 8l-cli",
			probe.Version, MinReadableVersion, SchemaVersion)
	}
}
