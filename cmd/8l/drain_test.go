package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OneZero1ai/8l-cli/internal/outbox"
)

func TestDrainEmpty(t *testing.T) {
	srv := newCQMock(t, nil)
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	var b bufs
	err := runDrain(&b.stdout, &b.stderr, &drainFlags{
		Profile:    "test",
		ConfigDir:  dir,
		OutboxPath: filepath.Join(dir, "outbox.jsonl"),
		Format:     "text",
	})
	if err != nil {
		t.Fatalf("runDrain: %v", err)
	}
	if !strings.Contains(b.stdout.String(), "Outbox empty") {
		t.Fatalf("unexpected stdout: %s", b.stdout.String())
	}
}

func TestDrainDryRun(t *testing.T) {
	srv := newCQMock(t, nil)
	defer srv.Close()
	dir := seedProfile(t, srv.URL)
	obx := filepath.Join(dir, "outbox.jsonl")
	for i := 0; i < 3; i++ {
		if err := outbox.Append(obx, outbox.Entry{
			EnqueuedAt: time.Now().UTC(),
			Domains:    []string{"d"},
			Summary:    "s", Detail: "d", Action: "a",
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	var b bufs
	err := runDrain(&b.stdout, &b.stderr, &drainFlags{
		Profile: "test", ConfigDir: dir, OutboxPath: obx, DryRun: true, Format: "text",
	})
	if err != nil {
		t.Fatalf("runDrain dry: %v", err)
	}
	if !strings.Contains(b.stdout.String(), "Would push 3") {
		t.Fatalf("unexpected stdout: %s", b.stdout.String())
	}
}

func TestDrainHappyPath(t *testing.T) {
	srv := newCQMock(t, nil)
	defer srv.Close()
	dir := seedProfile(t, srv.URL)
	obx := filepath.Join(dir, "outbox.jsonl")
	if err := outbox.Append(obx, outbox.Entry{
		EnqueuedAt: time.Now().UTC(),
		Domains:    []string{"d"}, Summary: "one", Detail: "x", Action: "y",
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := outbox.Append(obx, outbox.Entry{
		EnqueuedAt: time.Now().UTC(),
		Domains:    []string{"d"}, Summary: "two", Detail: "x", Action: "y",
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	var b bufs
	err := runDrain(&b.stdout, &b.stderr, &drainFlags{
		Profile: "test", ConfigDir: dir, OutboxPath: obx, Format: "text",
	})
	if err != nil {
		t.Fatalf("runDrain: %v\nstderr:%s", err, b.stderr.String())
	}
	if !strings.Contains(b.stdout.String(), "Pushed 2 unit(s)") {
		t.Fatalf("unexpected stdout: %s", b.stdout.String())
	}
	// Outbox must now be empty.
	entries, _ := outbox.Read(obx)
	if len(entries) != 0 {
		t.Fatalf("outbox still has %d entries", len(entries))
	}
}

func TestDrainAuthFailKeepsQueue(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{statusCodes: map[string]int{"/api/v1/propose": 401}})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)
	obx := filepath.Join(dir, "outbox.jsonl")
	if err := outbox.Append(obx, outbox.Entry{
		EnqueuedAt: time.Now().UTC(),
		Domains:    []string{"d"}, Summary: "one", Detail: "x", Action: "y",
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	err := runDrain(&strings_Builder, &strings_Builder, &drainFlags{
		Profile: "test", ConfigDir: dir, OutboxPath: obx, Format: "text",
	})
	if err == nil {
		t.Fatal("expected auth error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitAuthFail {
		t.Fatalf("expected ExitAuthFail got %v", err)
	}
	// Queue must still contain the entry — we don't drop on auth fail.
	entries, _ := outbox.Read(obx)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry preserved, got %d", len(entries))
	}
}

func TestDrainServer500MarksFailures(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{statusCodes: map[string]int{"/api/v1/propose": 500}})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)
	obx := filepath.Join(dir, "outbox.jsonl")
	if err := outbox.Append(obx, outbox.Entry{
		EnqueuedAt: time.Now().UTC(),
		Domains:    []string{"d"}, Summary: "one", Detail: "x", Action: "y",
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	err := runDrain(&strings_Builder, &strings_Builder, &drainFlags{
		Profile: "test", ConfigDir: dir, OutboxPath: obx, Format: "text",
	})
	if err == nil {
		t.Fatal("expected error on partial failure")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitUnexpected {
		t.Fatalf("expected ExitUnexpected got %v", err)
	}
	entries, _ := outbox.Read(obx)
	if len(entries) != 1 {
		t.Fatalf("expected entry to remain queued, got %d", len(entries))
	}
}

func TestDrainNoProfile(t *testing.T) {
	dir := t.TempDir()
	obx := filepath.Join(dir, "outbox.jsonl")
	// Put one entry so we get past the empty-outbox short circuit.
	if err := outbox.Append(obx, outbox.Entry{
		EnqueuedAt: time.Now().UTC(),
		Domains:    []string{"d"}, Summary: "x", Detail: "x", Action: "x",
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	err := runDrain(&strings_Builder, &strings_Builder, &drainFlags{
		Profile: "absent", ConfigDir: dir, OutboxPath: obx, Format: "text",
	})
	if err == nil {
		t.Fatal("expected missing-profile error")
	}
}
