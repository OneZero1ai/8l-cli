package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteOptions control profile-write behaviour.
type WriteOptions struct {
	// Force allows overwriting an existing profile that was NOT written
	// by `8l join` (managed_by mismatch). Without --force the writer
	// refuses to clobber user-edited or third-party-tool profiles.
	Force bool
}

// Write atomically persists a profile to <configDir>/<name>.json.
//
// Behaviour:
//   - Creates configDir (mode 0700) if missing.
//   - Stamps managed_at with the current UTC time and ensures
//     managed_by + version are set.
//   - Writes to a tempfile in the same directory then renames, so
//     readers never see a half-written file.
//   - Refuses to overwrite a profile that exists, has a different
//     binding, AND is not managed by 8l, unless opts.Force is set.
//   - Refuses to overwrite a profile with a DIFFERENT binding even
//     when 8l-managed unless opts.Force is set (idempotency rule).
//
// Returns the absolute path written.
func Write(configDir, name string, p *Profile, opts WriteOptions) (string, error) {
	if p == nil {
		return "", errors.New("profile: nil profile")
	}

	// Stamp metadata.
	if p.Version == 0 {
		p.Version = SchemaVersion
	}
	if p.ManagedAt == "" {
		p.ManagedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if err := p.Validate(); err != nil {
		return "", err
	}

	path, err := Path(configDir, name)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("profile: mkdir %s: %w", dir, err)
	}

	// Inspect existing file (if any) for the overwrite policy.
	existing, exists, _, err := Read(configDir, name)
	switch {
	case err != nil && exists:
		// File exists but is corrupt. Allow --force to clobber, otherwise refuse.
		if !opts.Force {
			return "", fmt.Errorf("profile: existing %s is unreadable (%v) — re-run with --force to overwrite", path, err)
		}
	case exists:
		// File exists and parsed cleanly. Apply the policy.
		if !existing.Binding.Equal(p.Binding) && !opts.Force {
			return "", fmt.Errorf("profile: %s already bound to %s — refusing to rebind to %s without --force",
				path, existing.Binding, p.Binding)
		}
		if !existing.IsManagedBy8l() && !opts.Force {
			return "", fmt.Errorf("profile: %s was not written by 8l (managed_by=%q) — refusing to overwrite without --force",
				path, existing.ManagedBy)
		}
	}

	// Atomic write: tempfile in same dir, fsync, rename.
	tmp, err := os.CreateTemp(dir, "."+name+".*.json.tmp")
	if err != nil {
		return "", fmt.Errorf("profile: create tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("profile: encode: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("profile: fsync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("profile: close tempfile: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		cleanup()
		return "", fmt.Errorf("profile: chmod: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return "", fmt.Errorf("profile: rename: %w", err)
	}
	return path, nil
}

// Delete removes a profile file. Missing-file is not an error.
func Delete(configDir, name string) (string, bool, error) {
	path, err := Path(configDir, name)
	if err != nil {
		return "", false, err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return path, false, nil
		}
		return path, false, fmt.Errorf("profile: delete %s: %w", path, err)
	}
	return path, true, nil
}
