package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/OneZero1ai/8l-cli/internal/profile"
	"github.com/OneZero1ai/8l-cli/internal/resolver"
)

func TestRunStatusHappyPath(t *testing.T) {
	srv := newL2(t, "private")
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	if err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		NoSmoke:    true,
		CQCommand:  "cq",
	}); err != nil {
		t.Fatalf("seed join: %v", err)
	}

	var stdout bytes.Buffer
	if err := runStatus(&stdout, &bytes.Buffer{}, &statusFlags{
		Profile:   "test",
		ConfigDir: dir,
	}); err != nil {
		t.Fatalf("runStatus: %v\n%s", err, stdout.String())
	}
	out := stdout.String()
	for _, want := range []string{"binding:", "8th-layer-corp/engineering/alice", "/auth/me:       OK", "propose smoke:  OK"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunStatusNoProfile(t *testing.T) {
	dir := t.TempDir()
	err := runStatus(&bytes.Buffer{}, &bytes.Buffer{}, &statusFlags{
		Profile:   "missing",
		ConfigDir: dir,
	})
	if err == nil {
		t.Fatal("expected error on missing profile")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitMissingArg {
		t.Fatalf("expected ExitMissingArg, got %v", err)
	}
}

func TestRunUnjoin(t *testing.T) {
	srv := newL2(t, "private")
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	if err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		NoSmoke:    true,
		CQCommand:  "cq",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var stdout bytes.Buffer
	if err := runUnjoin(&stdout, &bytes.Buffer{}, &unjoinFlags{
		Profile:   "test",
		ConfigDir: dir,
	}); err != nil {
		t.Fatalf("runUnjoin: %v", err)
	}
	if !strings.Contains(stdout.String(), "removed profile") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}

	// Profile gone.
	if _, exists, _, _ := profile.Read(dir, "test"); exists {
		t.Fatal("profile still present after unjoin")
	}
}

func TestRunUnjoinRevokeRequiresYes(t *testing.T) {
	srv := newL2(t, "private")
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	if err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		NoSmoke:    true,
		CQCommand:  "cq",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := runUnjoin(&bytes.Buffer{}, &bytes.Buffer{}, &unjoinFlags{
		Profile:   "test",
		ConfigDir: dir,
		Revoke:    true,
	})
	if err == nil {
		t.Fatal("expected --yes-required error")
	}
}
