package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
)

func TestFlagHappyPath(t *testing.T) {
	srv := newCQMock(t, nil)
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	var b bufs
	err := runFlag(&b.stdout, &b.stderr, &flagFlags{
		Profile:   "test",
		ConfigDir: dir,
		Reason:    l2client.FlagReasonStale,
		Format:    "text",
	}, "ku_xyz")
	if err != nil {
		t.Fatalf("runFlag: %v", err)
	}
	if !strings.Contains(b.stdout.String(), "Flagged ku_xyz as stale") {
		t.Fatalf("unexpected stdout: %s", b.stdout.String())
	}
}

func TestFlagInvalidReason(t *testing.T) {
	srv := newCQMock(t, nil)
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	err := runFlag(&strings_Builder, &strings_Builder, &flagFlags{
		Profile:   "test",
		ConfigDir: dir,
		Reason:    "not-a-real-reason",
		Format:    "text",
	}, "ku_xyz")
	if err == nil {
		t.Fatal("expected invalid-reason error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitMissingArg {
		t.Fatalf("expected ExitMissingArg, got %v", err)
	}
}

func TestFlagDuplicateRequiresOriginal(t *testing.T) {
	srv := newCQMock(t, nil)
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	err := runFlag(&strings_Builder, &strings_Builder, &flagFlags{
		Profile:   "test",
		ConfigDir: dir,
		Reason:    l2client.FlagReasonDuplicate,
		Format:    "text",
	}, "ku_xyz")
	if err == nil {
		t.Fatal("expected duplicate-of requirement")
	}
}

func TestFlagServer500(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{statusCodes: map[string]int{"/api/v1/flag/": 500}})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	err := runFlag(&strings_Builder, &strings_Builder, &flagFlags{
		Profile:   "test",
		ConfigDir: dir,
		Reason:    l2client.FlagReasonStale,
		Format:    "text",
	}, "ku_xyz")
	if err == nil {
		t.Fatal("expected 500 error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitUnexpected {
		t.Fatalf("expected ExitUnexpected got %v", err)
	}
}

func TestFlagNoProfile(t *testing.T) {
	dir := t.TempDir()
	err := runFlag(&strings_Builder, &strings_Builder, &flagFlags{
		Profile:   "absent",
		ConfigDir: dir,
		Reason:    l2client.FlagReasonStale,
		Format:    "text",
	}, "ku_xyz")
	if err == nil {
		t.Fatal("expected missing-profile error")
	}
}
