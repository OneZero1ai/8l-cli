package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// fixturePath resolves a testdata file from the repo root, regardless of
// which package's tests are running.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	// Walk up until we find the testdata directory.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "testdata", "profiles", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate testdata/profiles/%s", name)
	return ""
}

func TestFixtureV1Valid(t *testing.T) {
	raw, err := os.ReadFile(fixturePath(t, "v1-valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, mr, err := Migrate(raw)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if mr.Migrated {
		t.Fatal("v1 fixture should not need migration")
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestFixtureV0Refused(t *testing.T) {
	raw, err := os.ReadFile(fixturePath(t, "v0-missing-version.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Migrate(raw); err == nil {
		t.Fatal("v0 fixture should be refused")
	}
}

func TestFixtureV99Refused(t *testing.T) {
	raw, err := os.ReadFile(fixturePath(t, "v99-future.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Migrate(raw); err == nil {
		t.Fatal("v99 fixture should be refused")
	}
}
