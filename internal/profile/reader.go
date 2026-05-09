package profile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultConfigDir is the V1 location for profiles, matching
// claude-mux's convention.
const DefaultConfigDir = "~/.claude-mux/profiles"

// ExpandConfigDir resolves a `~`-prefixed path to absolute, using $HOME.
func ExpandConfigDir(dir string) (string, error) {
	if dir == "" {
		dir = DefaultConfigDir
	}
	if len(dir) > 0 && dir[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("profile: resolve home: %w", err)
		}
		dir = filepath.Join(home, dir[1:])
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("profile: abspath %q: %w", dir, err)
	}
	return abs, nil
}

// Path returns the on-disk path for a named profile within configDir.
//
// Profile names are restricted to filename-safe identifiers; callers
// should validate before calling.
func Path(configDir, name string) (string, error) {
	abs, err := ExpandConfigDir(configDir)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", errors.New("profile: name required")
	}
	// Defensive: refuse path traversal in profile names.
	if filepath.Base(name) != name {
		return "", fmt.Errorf("profile: invalid profile name %q", name)
	}
	return filepath.Join(abs, name+".json"), nil
}

// Read loads a profile from disk and runs schema validation +
// migration. Returns (nil, false, nil) when the file does not exist —
// that is NOT an error, just an absent profile.
//
// On older but readable schema versions, mr.Warning is set and the
// caller should surface it to the user.
func Read(configDir, name string) (p *Profile, exists bool, mr MigrateResult, err error) {
	path, err := Path(configDir, name)
	if err != nil {
		return nil, false, MigrateResult{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, MigrateResult{}, nil
		}
		return nil, false, MigrateResult{}, fmt.Errorf("profile: read %s: %w", path, err)
	}
	p, mr, err = Migrate(raw)
	if err != nil {
		return nil, true, MigrateResult{}, err
	}
	if err := p.Validate(); err != nil {
		return nil, true, mr, err
	}
	return p, true, mr, nil
}
