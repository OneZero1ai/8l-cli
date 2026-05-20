package l2client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProposeRoundtrip(t *testing.T) {
	var gotPath string
	var gotBody ProposeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(KnowledgeUnit{
			ID:      "ku_abc",
			Domains: []string{"d"},
			Tier:    "private",
			Insight: Insight{Summary: "s", Detail: "d", Action: "a"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "cqa.v1.test.key")
	ku, err := c.Propose(context.Background(), ProposeParams{
		Summary: "s", Detail: "d", Action: "a",
		Domains: []string{"d"}, Languages: []string{"go"},
		Pattern: "p",
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if gotPath != "/api/v1/propose" {
		t.Errorf("path: %q", gotPath)
	}
	if gotBody.Insight.Summary != "s" || gotBody.Insight.Detail != "d" {
		t.Errorf("body shape wrong: %+v", gotBody)
	}
	if len(gotBody.Context.Languages) != 1 || gotBody.Context.Languages[0] != "go" {
		t.Errorf("context.languages not propagated: %+v", gotBody.Context)
	}
	if gotBody.Context.Pattern != "p" {
		t.Errorf("pattern not propagated: %q", gotBody.Context.Pattern)
	}
	if ku.ID != "ku_abc" {
		t.Errorf("ku id: %q", ku.ID)
	}
}

func TestQueryEncodesAllParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]KnowledgeUnit{})
	}))
	defer srv.Close()

	c := New(srv.URL, "k")
	_, err := c.Query(context.Background(), QueryParams{
		Domains:    []string{"d1", "d2"},
		Languages:  []string{"go", "py"},
		Frameworks: []string{"fastapi"},
		Pattern:    "p",
		Limit:      7,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	// Each repeated param should land as its own key=value.
	for _, want := range []string{"domains=d1", "domains=d2", "languages=go", "languages=py", "frameworks=fastapi", "pattern=p", "limit=7"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query missing %q in %s", want, gotQuery)
		}
	}
}

func TestQueryRequiresDomain(t *testing.T) {
	c := New("http://x", "k")
	if _, err := c.Query(context.Background(), QueryParams{}); err == nil {
		t.Fatal("expected error for empty domains")
	}
}

func TestConfirmAndFlag(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(KnowledgeUnit{ID: "ku_xyz"})
	}))
	defer srv.Close()
	c := New(srv.URL, "k")

	if _, err := c.Confirm(context.Background(), "ku_xyz"); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if gotPath != "/api/v1/confirm/ku_xyz" {
		t.Errorf("confirm path: %q", gotPath)
	}
	if _, err := c.FlagUnit(context.Background(), "ku_xyz", FlagRequest{Reason: FlagReasonStale}); err != nil {
		t.Fatalf("FlagUnit: %v", err)
	}
	if gotPath != "/api/v1/flag/ku_xyz" {
		t.Errorf("flag path: %q", gotPath)
	}
}

func TestStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(StatsResponse{
			TotalUnits: 5,
			Tiers:      map[string]int{"private": 5},
			Domains:    map[string]int{"a": 3, "b": 2},
		})
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	st, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalUnits != 5 || st.Tiers["private"] != 5 || st.Domains["a"] != 3 {
		t.Fatalf("unexpected stats: %+v", st)
	}
}
