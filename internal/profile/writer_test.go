package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func makeProfile() *Profile {
	return &Profile{
		Version:   SchemaVersion,
		ManagedBy: "8l join v0.1.0",
		Binding: Binding{
			Enterprise: "8th-layer-corp",
			L2:         "engineering",
			Persona:    "alice",
		},
		MCPServers: map[string]MCPServer{
			"cq": {
				Type:    "stdio",
				Command: "cq",
				Env: map[string]string{
					"CQ_ADDR":    "https://engineering.8th-layer-corp.8th-layer.ai",
					"CQ_API_KEY": "cqa.v1.0123456789abcdef0123456789abcdef.test",
				},
			},
		},
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := makeProfile()

	path, err := Write(dir, "test", p, WriteOptions{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if filepath.Base(path) != "test.json" {
		t.Fatalf("unexpected basename: %s", path)
	}

	got, exists, _, err := Read(dir, "test")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}
	if !got.Binding.Equal(p.Binding) {
		t.Fatalf("binding mismatch")
	}
	if got.ManagedAt == "" {
		t.Fatal("expected managed_at to be stamped")
	}
}

func TestWriteIdempotentAndForce(t *testing.T) {
	dir := t.TempDir()
	p := makeProfile()
	if _, err := Write(dir, "test", p, WriteOptions{}); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// Same binding, no force → ok (writer overwrites because it's 8l-managed
	// and the binding matches).
	if _, err := Write(dir, "test", p, WriteOptions{}); err != nil {
		t.Fatalf("idempotent write: %v", err)
	}

	// Different binding, no force → refused.
	p2 := makeProfile()
	p2.Binding.Persona = "bob"
	if _, err := Write(dir, "test", p2, WriteOptions{}); err == nil {
		t.Fatal("expected error rebinding without --force")
	}

	// Different binding, --force → ok.
	if _, err := Write(dir, "test", p2, WriteOptions{Force: true}); err != nil {
		t.Fatalf("force rebind: %v", err)
	}
}

func TestWriteRefusesNonManagedProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manual.json")
	manual := makeProfile()
	manual.ManagedBy = "hand-edited"
	manual.ManagedAt = "2026-01-01T00:00:00Z"
	raw, _ := json.MarshalIndent(manual, "", "  ")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := Write(dir, "manual", makeProfile(), WriteOptions{}); err == nil {
		t.Fatal("expected refusal of non-managed overwrite")
	}
	if _, err := Write(dir, "manual", makeProfile(), WriteOptions{Force: true}); err != nil {
		t.Fatalf("force overwrite of non-managed: %v", err)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "to-delete", makeProfile(), WriteOptions{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, deleted, err := Delete(dir, "to-delete")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Fatal("expected deleted=true")
	}
	// Second delete = noop.
	_, deleted, err = Delete(dir, "to-delete")
	if err != nil {
		t.Fatalf("Delete (missing): %v", err)
	}
	if deleted {
		t.Fatal("expected deleted=false on missing")
	}
}

func TestReadMissing(t *testing.T) {
	dir := t.TempDir()
	p, exists, _, err := Read(dir, "nope")
	if err != nil {
		t.Fatalf("Read missing: %v", err)
	}
	if exists || p != nil {
		t.Fatalf("expected (nil, false), got (%v, %v)", p, exists)
	}
}

func TestPathRejectsTraversal(t *testing.T) {
	if _, err := Path("/tmp", "../etc/passwd"); err == nil {
		t.Fatal("expected error on path traversal")
	}
}
