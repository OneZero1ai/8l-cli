package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPromptReflectTextNonEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writePrompt(&buf, "text", "prompt", "hello body"); err != nil {
		t.Fatalf("writePrompt text: %v", err)
	}
	if buf.String() != "hello body" {
		t.Fatalf("unexpected text: %q", buf.String())
	}
}

func TestPromptJSONEncoding(t *testing.T) {
	var buf bytes.Buffer
	if err := writePrompt(&buf, "json", "prompt", "hello body"); err != nil {
		t.Fatalf("writePrompt json: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if out["prompt"] != "hello body" {
		t.Fatalf("unexpected payload: %+v", out)
	}
}

func TestPromptInvalidFormat(t *testing.T) {
	if err := writePrompt(&bytes.Buffer{}, "yaml", "prompt", "x"); err == nil {
		t.Fatal("expected unsupported-format error")
	}
}

// Verify the embedded prompt bodies aren't empty — the cq agent
// instructions are useless if the embed got stripped.
func TestPromptBodiesEmbedded(t *testing.T) {
	cmd := newPromptReflectCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute reflect: %v", err)
	}
	if !strings.Contains(buf.String(), "cq") {
		t.Fatalf("reflect body looks wrong: %s", buf.String()[:min(200, len(buf.String()))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
