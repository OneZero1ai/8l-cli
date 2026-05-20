package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/OneZero1ai/8l-cli/internal/profile"
	"github.com/OneZero1ai/8l-cli/internal/resolver"
)

func TestQuickTokenRoundTrip(t *testing.T) {
	in := quickPayload{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
	}
	tok := encodeQuickToken(in)
	if !strings.HasPrefix(tok, "cqq.v1.") {
		t.Fatalf("missing prefix: %q", tok)
	}
	out, err := decodeQuickToken(tok)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, in)
	}
}

func TestQuickTokenRejectsGarbage(t *testing.T) {
	cases := []string{
		"",
		"not-a-token",
		"cqa.v1.deadbeef",                                          // looks like an api key, not a quick token
		"cqq.v1.!!!notbase64!!!",
		"cqq.v1." + "Zm9v",                                          // decodes to "foo" — only 1 field
	}
	for _, c := range cases {
		if _, err := decodeQuickToken(c); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}

func TestRunQuickHappyPath(t *testing.T) {
	srv := newL2(t, "private")
	defer srv.Close()
	t.Setenv(resolver.EndpointEnvOverride, srv.URL)

	token := encodeQuickToken(quickPayload{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     validKey,
	})

	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runQuick(&stdout, &stderr, &quickFlags{
		Profile:   "test",
		ConfigDir: dir,
		CQCommand: "cq",
	}, token)
	if err != nil {
		t.Fatalf("runQuick: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "smoke ok") {
		t.Fatalf("expected smoke ok in output:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "quick-join token") {
		t.Fatalf("expected join to re-print a quick token:\n%s", stdout.String())
	}
	p, exists, _, err := profile.Read(dir, "test")
	if err != nil || !exists {
		t.Fatalf("profile missing: err=%v exists=%v", err, exists)
	}
	if p.Binding.Persona != "alice" {
		t.Fatalf("persona mismatch: %s", p.Binding.Persona)
	}
}

func TestRunQuickInvalidToken(t *testing.T) {
	dir := t.TempDir()
	err := runQuick(&bytes.Buffer{}, &bytes.Buffer{}, &quickFlags{
		Profile:   "test",
		ConfigDir: dir,
	}, "not-a-quick-token")
	if err == nil {
		t.Fatal("expected invalid-token error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitInvalidKey {
		t.Fatalf("expected ExitInvalidKey (11), got %v", err)
	}
}

func TestRunQuickInvalidKeyShape(t *testing.T) {
	// Well-formed token envelope but the embedded api-key is not cqa.v1.* —
	// quick should surface the same invalid-key exit (11) as a four-flag join.
	tok := encodeQuickToken(quickPayload{
		Enterprise: "8th-layer-corp",
		L2:         "engineering",
		Persona:    "alice",
		APIKey:     "nope-not-a-key",
	})
	dir := t.TempDir()
	err := runQuick(&bytes.Buffer{}, &bytes.Buffer{}, &quickFlags{
		Profile:   "test",
		ConfigDir: dir,
	}, tok)
	if err == nil {
		t.Fatal("expected invalid-key error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitInvalidKey {
		t.Fatalf("expected ExitInvalidKey (11), got %v", err)
	}
}
