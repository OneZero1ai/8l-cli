package outbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendThenRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	if err := Append(path, Entry{
		EnqueuedAt: time.Now().UTC(),
		Summary:    "one",
		Detail:     "d", Action: "a",
		Domains: []string{"d1"},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := Append(path, Entry{
		EnqueuedAt: time.Now().UTC(),
		Summary:    "two",
		Detail:     "d", Action: "a",
		Domains: []string{"d1"},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	entries, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Summary != "one" || entries[1].Summary != "two" {
		t.Fatalf("order wrong: %+v", entries)
	}
}

func TestReadMissingFile(t *testing.T) {
	entries, err := Read(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("Read missing: %v (should be nil err)", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty, got %d", len(entries))
	}
}

func TestRewriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")
	for _, s := range []string{"a", "b", "c"} {
		if err := Append(path, Entry{Domains: []string{"d"}, Summary: s, Detail: "x", Action: "y"}); err != nil {
			t.Fatalf("Append %s: %v", s, err)
		}
	}
	// Drop "b".
	cur, _ := Read(path)
	keep := []Entry{cur[0], cur[2]}
	if err := Rewrite(path, keep); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	after, _ := Read(path)
	if len(after) != 2 || after[0].Summary != "a" || after[1].Summary != "c" {
		t.Fatalf("rewrite mismatch: %+v", after)
	}
}

func TestRewriteEmptyRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")
	if err := Append(path, Entry{Domains: []string{"d"}, Summary: "x", Detail: "x", Action: "y"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := Rewrite(path, nil); err != nil {
		t.Fatalf("Rewrite empty: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be removed, got %v", err)
	}
}

func TestResolvePathTildeExpands(t *testing.T) {
	t.Setenv("HOME", "/tmp/somewhere")
	abs, err := ResolvePath("~/x.jsonl")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if abs != "/tmp/somewhere/x.jsonl" {
		t.Fatalf("tilde not expanded: %q", abs)
	}
}
