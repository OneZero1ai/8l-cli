package profile

import (
	"strings"
	"testing"
)

func TestMigrateCurrent(t *testing.T) {
	raw := []byte(`{
        "version": 1,
        "managed_by": "8l join v0.1.0",
        "binding": {"enterprise":"8th-layer-corp","l2":"engineering","persona":"alice"},
        "mcpServers": {
            "cq": {"type":"stdio","command":"cq","env":{"CQ_ADDR":"https://x","CQ_API_KEY":"y"}}
        }
    }`)
	p, mr, err := Migrate(raw)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if mr.Migrated {
		t.Fatalf("v1 should not be marked migrated")
	}
	if p.Binding.Enterprise != "8th-layer-corp" {
		t.Fatalf("enterprise round-trip failed")
	}
}

func TestMigrateMissingVersion(t *testing.T) {
	raw := []byte(`{"managed_by":"8l join v0"}`)
	if _, _, err := Migrate(raw); err == nil {
		t.Fatal("expected error on missing version")
	} else if !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("expected schema-version error, got: %v", err)
	}
}

func TestMigrateFutureVersion(t *testing.T) {
	raw := []byte(`{"version": 99, "managed_by":"8l join v9"}`)
	if _, _, err := Migrate(raw); err == nil {
		t.Fatal("expected error on future version")
	}
}

func TestMigrateMalformedJSON(t *testing.T) {
	raw := []byte(`{not json`)
	if _, _, err := Migrate(raw); err == nil {
		t.Fatal("expected parse error")
	}
}
