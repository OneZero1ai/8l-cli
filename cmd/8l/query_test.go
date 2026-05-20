package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
)

func TestQueryHappyPath(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{
		queryUnits: []l2client.KnowledgeUnit{
			{
				ID:       "ku_one",
				Insight:  l2client.Insight{Summary: "alpha", Detail: "ddd", Action: "aaa"},
				Evidence: l2client.Evidence{Confidence: 0.75},
			},
		},
	})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	var b bufs
	err := runQuery(&b.stdout, &b.stderr, &queryFlags{
		Profile:   "test",
		ConfigDir: dir,
		Domains:   []string{"test-fleet"},
		Limit:     5,
		Format:    "text",
	})
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}
	if !strings.Contains(b.stdout.String(), "[ku_one]") {
		t.Fatalf("expected ku_one in output: %s", b.stdout.String())
	}
}

func TestQueryEmptyResult(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{queryUnits: nil})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	var b bufs
	err := runQuery(&b.stdout, &b.stderr, &queryFlags{
		Profile:   "test",
		ConfigDir: dir,
		Domains:   []string{"nonesuch"},
		Limit:     5,
		Format:    "text",
	})
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}
	if !strings.Contains(b.stdout.String(), "No matching") {
		t.Fatalf("expected empty-result message: %s", b.stdout.String())
	}
}

func TestQueryJSONFormat(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{
		queryUnits: []l2client.KnowledgeUnit{
			{ID: "ku_x", Insight: l2client.Insight{Summary: "x"}},
		},
	})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	var b bufs
	err := runQuery(&b.stdout, &b.stderr, &queryFlags{
		Profile:   "test",
		ConfigDir: dir,
		Domains:   []string{"d"},
		Limit:     5,
		Format:    "json",
	})
	if err != nil {
		t.Fatalf("runQuery json: %v", err)
	}
	var units []l2client.KnowledgeUnit
	if err := json.Unmarshal(b.stdout.Bytes(), &units); err != nil {
		t.Fatalf("json decode: %v\n%s", err, b.stdout.String())
	}
	if len(units) != 1 || units[0].ID != "ku_x" {
		t.Fatalf("unexpected payload: %+v", units)
	}
}

func TestQueryServer500(t *testing.T) {
	srv := newCQMock(t, &cqMockHandle{statusCodes: map[string]int{"/api/v1/query": 500}})
	defer srv.Close()
	dir := seedProfile(t, srv.URL)

	err := runQuery(&strings_Builder, &strings_Builder, &queryFlags{
		Profile:   "test",
		ConfigDir: dir,
		Domains:   []string{"d"},
		Limit:     5,
		Format:    "text",
	})
	if err == nil {
		t.Fatal("expected error on 500")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitUnexpected {
		t.Fatalf("expected ExitUnexpected got %v", err)
	}
}

func TestQueryNoProfile(t *testing.T) {
	dir := t.TempDir()
	err := runQuery(&strings_Builder, &strings_Builder, &queryFlags{
		Profile:   "absent",
		ConfigDir: dir,
		Domains:   []string{"d"},
		Format:    "text",
	})
	if err == nil {
		t.Fatal("expected missing-profile error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitMissingArg {
		t.Fatalf("expected ExitMissingArg got %v", err)
	}
}
