package main

import (
	"errors"
	"strings"
	"testing"
)

func TestConfirmHappyPath(t *testing.T) {
	srv := newCQMock(t, nil)
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	var b bufs
	err := runConfirm(&b.stdout, &b.stderr, &confirmFlags{
		Profile:   "test",
		ConfigDir: dir,
		Format:    "text",
	}, "ku_xyz")
	if err != nil {
		t.Fatalf("runConfirm: %v", err)
	}
	if !strings.Contains(b.stdout.String(), "Confirmed ku_xyz") {
		t.Fatalf("unexpected stdout: %s", b.stdout.String())
	}
}

func TestConfirmServer500(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{statusCodes: map[string]int{"/api/v1/confirm/": 500}})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	err := runConfirm(&strings_Builder, &strings_Builder, &confirmFlags{
		Profile:   "test",
		ConfigDir: dir,
		Format:    "text",
	}, "ku_xyz")
	if err == nil {
		t.Fatal("expected error on 500")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitUnexpected {
		t.Fatalf("expected ExitUnexpected got %v", err)
	}
}

func TestConfirmNoProfile(t *testing.T) {
	dir := t.TempDir()
	err := runConfirm(&strings_Builder, &strings_Builder, &confirmFlags{
		Profile:   "absent",
		ConfigDir: dir,
		Format:    "text",
	}, "ku_xyz")
	if err == nil {
		t.Fatal("expected missing-profile error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitMissingArg {
		t.Fatalf("expected ExitMissingArg got %v", err)
	}
}
