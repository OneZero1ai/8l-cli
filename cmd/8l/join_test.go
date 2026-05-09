package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
	"github.com/OneZero1ai/8l-cli/internal/resolver"
)

const validKey = "cqa.v1.0123456789abcdef0123456789abcdef.0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// newL2 spins up a mock that handles /auth/me and /propose in the
// happy-path shape. Override individual handlers by passing a setter.
func newL2(t *testing.T, tier string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(l2client.AuthMeResponse{
			EnterpriseID: "8th-layer-corp",
			GroupID:      "engineering",
			Persona:      "alice",
			KeyID:        "0123456789abcdef0123456789abcdef",
		})
	})
	mux.HandleFunc("/api/v1/propose", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(l2client.ProposeResponse{
			UnitID: "ku-test",
			Tier:   tier,
		})
	})
	return httptest.NewServer(mux)
}

func TestRunJoinHappyPath(t *testing.T) {
	srv := newL2(t, "private")
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runJoin(&stdout, &stderr, &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		CQCommand:  "cq",
	})
	if err != nil {
		t.Fatalf("runJoin: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "smoke ok") {
		t.Fatalf("expected smoke ok in output:\n%s", stdout.String())
	}
	// Verify the profile landed.
	p, exists, _, err := profile.Read(dir, "test")
	if err != nil || !exists {
		t.Fatalf("profile missing: err=%v exists=%v", err, exists)
	}
	if p.Binding.Persona != "alice" {
		t.Fatalf("persona mismatch: %s", p.Binding.Persona)
	}
}

func TestRunJoinTierLocalRollback(t *testing.T) {
	srv := newL2(t, "local")
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runJoin(&stdout, &stderr, &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		CQCommand:  "cq",
	})
	if err == nil {
		t.Fatal("expected error on tier=local")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitSmokeLocalTier {
		t.Fatalf("expected ExitSmokeLocalTier (14), got %v", err)
	}
	// Profile should NOT exist (no prior profile to roll back to).
	if _, exists, _, _ := profile.Read(dir, "test"); exists {
		t.Fatal("profile should have been removed after smoke failure")
	}
}

func TestRunJoinIdempotent(t *testing.T) {
	srv := newL2(t, "private")
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	flags := &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		CQCommand:  "cq",
		NoSmoke:    true,
	}

	if err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, flags); err != nil {
		t.Fatalf("first join: %v", err)
	}
	var stdout bytes.Buffer
	if err := runJoin(&stdout, &bytes.Buffer{}, flags); err != nil {
		t.Fatalf("second join: %v", err)
	}
	if !strings.Contains(stdout.String(), "no-op") {
		t.Fatalf("expected idempotent no-op, got:\n%s", stdout.String())
	}
}

func TestRunJoinRebindRefused(t *testing.T) {
	srv := newL2(t, "private")
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	first := &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		CQCommand:  "cq",
		NoSmoke:    true,
	}
	if err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, first); err != nil {
		t.Fatalf("first: %v", err)
	}

	second := *first
	second.Persona = "bob"
	err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &second)
	if err == nil {
		t.Fatal("expected refusal of rebind without --force")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitProfileConflict {
		t.Fatalf("expected ExitProfileConflict (15), got %v", err)
	}

	second.Force = true
	if err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &second); err != nil {
		t.Fatalf("force rebind: %v", err)
	}
}

func TestRunJoinMissingArg(t *testing.T) {
	dir := t.TempDir()
	err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &joinFlags{
		Enterprise: "",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
	})
	if err == nil {
		t.Fatal("expected missing-arg error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitMissingArg {
		t.Fatalf("expected ExitMissingArg (10), got %v", err)
	}
}

func TestRunJoinInvalidKey(t *testing.T) {
	dir := t.TempDir()
	err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     "nope-not-a-key",
		Profile:    "test",
		ConfigDir:  dir,
	})
	if err == nil {
		t.Fatal("expected invalid-key error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitInvalidKey {
		t.Fatalf("expected ExitInvalidKey (11), got %v", err)
	}
}

func TestRunJoinAuthFailMismatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(l2client.AuthMeResponse{
			EnterpriseID: "OTHER-corp",
			GroupID:      "OTHER",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		CQCommand:  "cq",
	})
	if err == nil {
		t.Fatal("expected auth-mismatch error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitAuthFail {
		t.Fatalf("expected ExitAuthFail (13), got %v", err)
	}
}

func TestRunJoin401(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	dir := t.TempDir()
	err := runJoin(&bytes.Buffer{}, &bytes.Buffer{}, &joinFlags{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
		Profile:    "test",
		ConfigDir:  dir,
		CQCommand:  "cq",
	})
	if err == nil {
		t.Fatal("expected 401 error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitAuthFail {
		t.Fatalf("expected ExitAuthFail (13) on 401, got %v", err)
	}
}
