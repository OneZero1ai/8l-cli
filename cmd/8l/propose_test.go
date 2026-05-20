package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OneZero1ai/8l-cli/internal/outbox"
)

func TestProposeHappyPath(t *testing.T) {
	srv := newCQMock(t, nil)
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	var b bufs
	err := runPropose(&b.stdout, &b.stderr, &proposeFlags{
		Profile:   "test",
		ConfigDir: dir,
		Summary:   "S",
		Detail:    "D",
		Action:    "A",
		Domains:   []string{"test-fleet"},
		Format:    "text",
	})
	if err != nil {
		t.Fatalf("runPropose: %v\nstderr:\n%s", err, b.stderr.String())
	}
	if !strings.Contains(b.stdout.String(), "Proposed: ku_proposed") {
		t.Fatalf("unexpected stdout: %s", b.stdout.String())
	}
}

func TestProposeNoProfile(t *testing.T) {
	dir := t.TempDir()
	err := runPropose(&strings_Builder, &strings_Builder, &proposeFlags{
		Profile:   "absent",
		ConfigDir: dir,
		Summary:   "S",
		Detail:    "D",
		Action:    "A",
		Domains:   []string{"test-fleet"},
		Format:    "text",
	})
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitMissingArg {
		t.Fatalf("expected ExitMissingArg got %v", err)
	}
	if !strings.Contains(err.Error(), "8l join") {
		t.Errorf("error should point at `8l join`: %v", err)
	}
}

func TestProposeServer500QueuesToOutbox(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{statusCodes: map[string]int{"/api/v1/propose": 500}})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	outboxPath := filepath.Join(dir, "outbox.jsonl")
	var b bufs
	err := runPropose(&b.stdout, &b.stderr, &proposeFlags{
		Profile:    "test",
		ConfigDir:  dir,
		OutboxPath: outboxPath,
		Summary:    "S",
		Detail:     "D",
		Action:     "A",
		Domains:    []string{"test-fleet"},
		Format:     "text",
	})
	if err != nil {
		t.Fatalf("runPropose: %v (should have queued silently)", err)
	}
	entries, err := outbox.Read(outboxPath)
	if err != nil {
		t.Fatalf("outbox.Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 outbox entry, got %d", len(entries))
	}
	if entries[0].Summary != "S" {
		t.Fatalf("outbox entry summary mismatch: %s", entries[0].Summary)
	}
	if !strings.Contains(b.stdout.String(), "Queued") {
		t.Errorf("stdout should announce queued: %s", b.stdout.String())
	}
}

func TestProposeAuthFail(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{statusCodes: map[string]int{"/api/v1/propose": 401}})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	err := runPropose(&strings_Builder, &strings_Builder, &proposeFlags{
		Profile:   "test",
		ConfigDir: dir,
		Summary:   "S",
		Detail:    "D",
		Action:    "A",
		Domains:   []string{"test-fleet"},
		Format:    "text",
	})
	if err == nil {
		t.Fatal("expected auth error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitAuthFail {
		t.Fatalf("expected ExitAuthFail got %v", err)
	}
	// And nothing should have been queued: outbox file must not exist.
	if _, statErr := os.Stat(filepath.Join(dir, "8l-outbox.jsonl")); !os.IsNotExist(statErr) {
		t.Errorf("auth-fail should not have queued: %v", statErr)
	}
}

// strings_Builder is a stub bytes-buffer alias used when stdout/stderr
// are irrelevant to the test assertions. Using a discard-style buffer
// is enough; we don't need a global var.
var strings_Builder strings.Builder
