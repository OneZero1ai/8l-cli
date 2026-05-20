package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/OneZero1ai/8l-cli/internal/l2client"
	"github.com/OneZero1ai/8l-cli/internal/profile"
)

// seedProfile writes a minimally-valid profile pointing at baseURL with
// validKey. Returns the absolute config dir used.
//
// The cq subcommands don't go through `8l join` — they just read the
// profile — so tests bypass the join smoke step entirely.
func seedProfile(t *testing.T, baseURL string) string {
	t.Helper()
	dir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339)
	p := &profile.Profile{
		Version:   profile.SchemaVersion,
		ManagedBy: "8l join test",
		ManagedAt: now,
		Binding: profile.Binding{
			Enterprise: "test-enterprise",
			L2:         "test-l2",
			Persona:    "alice",
		},
		MCPServers: map[string]profile.MCPServer{
			"cq": {
				Type:    "stdio",
				Command: "cq",
				Env: map[string]string{
					"CQ_ADDR":    baseURL,
					"CQ_API_KEY": validKey,
				},
			},
		},
	}
	if _, err := profile.Write(dir, "test", p, profile.WriteOptions{Force: true}); err != nil {
		t.Fatalf("seedProfile: %v", err)
	}
	return dir
}

// newCQMockServer is a generic mock L2 server. Routes:
//   - GET  /api/v1/auth/me              → 200, fixed binding
//   - POST /api/v1/propose              → 201, echoes a synthetic KU
//   - GET  /api/v1/query                → 200, returns h.queryUnits
//   - POST /api/v1/confirm/{unit_id}    → 200, echoes confirmed KU
//   - POST /api/v1/flag/{unit_id}       → 200, echoes flagged KU
//   - GET  /api/v1/stats                → 200, returns h.stats
//
// Any field on the handle can be overridden before the server is used.
type cqMockHandle struct {
	queryUnits  []l2client.KnowledgeUnit
	stats       l2client.StatsResponse
	proposeKU   l2client.KnowledgeUnit
	confirmKU   l2client.KnowledgeUnit
	flagKU      l2client.KnowledgeUnit
	statusCodes map[string]int // path → status code override
}

func newCQMock(t *testing.T, h *cqMockHandle) *httptest.Server {
	t.Helper()
	if h == nil {
		h = &cqMockHandle{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(l2client.AuthMeResponse{
			EnterpriseID: "test-enterprise",
			GroupID:      "test-l2",
			Persona:      "alice",
		})
	})
	mux.HandleFunc("/api/v1/propose", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := h.statusCodes["/api/v1/propose"]; ok && code >= 400 {
			http.Error(w, `{"detail":"forced"}`, code)
			return
		}
		w.WriteHeader(http.StatusCreated)
		ku := h.proposeKU
		if ku.ID == "" {
			ku = l2client.KnowledgeUnit{
				ID:       "ku_proposed",
				Tier:     "private",
				Evidence: l2client.Evidence{Confidence: 0.5, Confirmations: 1},
				Insight:  l2client.Insight{Summary: "test"},
			}
		}
		_ = json.NewEncoder(w).Encode(ku)
	})
	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := h.statusCodes["/api/v1/query"]; ok && code >= 400 {
			http.Error(w, `{"detail":"forced"}`, code)
			return
		}
		_ = json.NewEncoder(w).Encode(h.queryUnits)
	})
	mux.HandleFunc("/api/v1/confirm/", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := h.statusCodes["/api/v1/confirm/"]; ok && code >= 400 {
			http.Error(w, `{"detail":"forced"}`, code)
			return
		}
		ku := h.confirmKU
		if ku.ID == "" {
			ku = l2client.KnowledgeUnit{
				ID:       filepath.Base(r.URL.Path),
				Evidence: l2client.Evidence{Confidence: 0.75, Confirmations: 2},
			}
		}
		_ = json.NewEncoder(w).Encode(ku)
	})
	mux.HandleFunc("/api/v1/flag/", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := h.statusCodes["/api/v1/flag/"]; ok && code >= 400 {
			http.Error(w, `{"detail":"forced"}`, code)
			return
		}
		ku := h.flagKU
		if ku.ID == "" {
			ku = l2client.KnowledgeUnit{
				ID:       filepath.Base(r.URL.Path),
				Evidence: l2client.Evidence{Confidence: 0.25, Confirmations: 1},
			}
		}
		_ = json.NewEncoder(w).Encode(ku)
	})
	mux.HandleFunc("/api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		if code, ok := h.statusCodes["/api/v1/stats"]; ok && code >= 400 {
			http.Error(w, `{"detail":"forced"}`, code)
			return
		}
		if h.stats.Tiers == nil {
			h.stats = l2client.StatsResponse{
				TotalUnits: 3,
				Tiers:      map[string]int{"private": 3},
				Domains:    map[string]int{"test-fleet": 2, "go": 1},
			}
		}
		_ = json.NewEncoder(w).Encode(h.stats)
	})
	return httptest.NewServer(mux)
}

// bufs is shorthand for stdout/stderr capture buffers.
type bufs struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
}
