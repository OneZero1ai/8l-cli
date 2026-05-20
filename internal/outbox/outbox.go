// Package outbox is the local fallback queue for `8l propose` and
// `8l drain`.
//
// When the L2 is unreachable, `8l propose` appends a JSON line to the
// outbox file instead of dropping the KU. `8l drain` replays those
// lines through the L2 in order; successfully-pushed lines are
// removed from the file.
//
// Design notes — divergence from upstream cq:
//
//   - The upstream cq Go SDK uses an embedded SQLite store via
//     modernc.org/libc. That brings in ~5MB of CGO-free indirect deps
//     we'd rather not ship for a CLI whose remote path is the common
//     case. 8l-cli is HTTP-only by design; an outbox is the smallest
//     possible local-state primitive that still gives us
//     "don't lose work when the L2 is down".
//   - File format is JSONL — one Entry per line. Writes are append +
//     fsync; drain rewrites the surviving lines on success. Concurrent
//     drain runs are not protected; both writers would just no-op on
//     each other (the second drain finds an empty/already-replayed
//     file). For a single-operator CLI that's enough.
//   - We deliberately do not retain confidence / evidence / tier on
//     queued entries — propose-replay just resends the request body
//     fields. The server stamps everything else.
package outbox

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/OneZero1ai/8l-cli/internal/profile"
)

// DefaultPath is the on-disk JSONL file. Sibling to claude-mux's
// profiles so all 8l local state lives under one root.
const DefaultPath = "~/.claude-mux/8l-outbox.jsonl"

// Entry is one queued propose. JSON shape matches l2client.ProposeRequest
// closely so the entries can be cat'd by humans for debugging.
type Entry struct {
	// EnqueuedAt records when the propose was first attempted and
	// fell into the outbox.
	EnqueuedAt time.Time `json:"enqueued_at"`
	Domains    []string  `json:"domains"`
	Summary    string    `json:"summary"`
	Detail     string    `json:"detail"`
	Action     string    `json:"action"`
	Languages  []string  `json:"languages,omitempty"`
	Frameworks []string  `json:"frameworks,omitempty"`
	Pattern    string    `json:"pattern,omitempty"`
}

// ResolvePath expands ~ and returns the absolute path used by the
// outbox. Callers may pass "" to mean DefaultPath.
func ResolvePath(p string) (string, error) {
	if p == "" {
		p = DefaultPath
	}
	if len(p) > 0 && p[0] == '~' {
		// Reuse profile.ExpandConfigDir's home-expansion semantics so
		// the prefix rules stay in lockstep.
		base, err := profile.ExpandConfigDir(p)
		if err != nil {
			return "", err
		}
		return base, nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("outbox: abspath %q: %w", p, err)
	}
	return abs, nil
}

// Append adds one entry to the end of the outbox file. The parent
// directory is created if absent. fsync runs before close.
func Append(path string, e Entry) error {
	abs, err := ResolvePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return fmt.Errorf("outbox: mkdir: %w", err)
	}
	f, err := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("outbox: open: %w", err)
	}
	defer f.Close()
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("outbox: marshal: %w", err)
	}
	line = append(line, '\n')
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("outbox: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("outbox: fsync: %w", err)
	}
	return nil
}

// Read returns every entry currently queued (oldest-first). Missing
// file → ([]Entry{}, nil), NOT an error.
func Read(path string) ([]Entry, error) {
	abs, err := ResolvePath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("outbox: open: %w", err)
	}
	defer f.Close()
	var out []Entry
	scan := bufio.NewScanner(f)
	// Long detail bodies routinely exceed bufio's default 64KB cap.
	// 1MB is enough headroom for any realistic propose.
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scan.Scan() {
		line := scan.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("outbox: parse line: %w", err)
		}
		out = append(out, e)
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("outbox: scan: %w", err)
	}
	return out, nil
}

// Rewrite atomically replaces the outbox file with the given entries.
// Empty slice ⇒ file is removed entirely (so `8l status` doesn't
// keep an empty file around).
func Rewrite(path string, entries []Entry) error {
	abs, err := ResolvePath(path)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		if err := os.Remove(abs); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("outbox: remove: %w", err)
		}
		return nil
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("outbox: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".outbox-*.tmp")
	if err != nil {
		return fmt.Errorf("outbox: tempfile: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			tmp.Close()
			return fmt.Errorf("outbox: marshal: %w", err)
		}
		line = append(line, '\n')
		if _, err := tmp.Write(line); err != nil {
			tmp.Close()
			return fmt.Errorf("outbox: write tmp: %w", err)
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("outbox: fsync tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("outbox: close tmp: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("outbox: chmod tmp: %w", err)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return fmt.Errorf("outbox: rename: %w", err)
	}
	return nil
}
