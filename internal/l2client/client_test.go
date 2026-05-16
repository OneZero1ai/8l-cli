package l2client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newMockL2(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := New(srv.URL, "cqa.v1.test.key")
	return c, srv.Close
}

func TestAuthMeOK(t *testing.T) {
	c, stop := newMockL2(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("missing Bearer header: %q", got)
		}
		_ = json.NewEncoder(w).Encode(AuthMeResponse{
			EnterpriseID: "8th-layer-corp",
			GroupID:      "engineering",
			Persona:      "alice",
			KeyID:        "abc123",
		})
	})
	defer stop()

	me, err := c.AuthMe(context.Background())
	if err != nil {
		t.Fatalf("AuthMe: %v", err)
	}
	if me.EnterpriseID != "8th-layer-corp" || me.GroupID != "engineering" {
		t.Fatalf("decode mismatch: %+v", me)
	}
}

func TestAuthMe401(t *testing.T) {
	c, stop := newMockL2(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
	})
	defer stop()

	_, err := c.AuthMe(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsAuth(err) {
		t.Fatalf("IsAuth=false on 401: %v", err)
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if he.StatusCode != 401 {
		t.Fatalf("StatusCode = %d", he.StatusCode)
	}
}

func TestSmokeProposeTierPrivate(t *testing.T) {
	c, stop := newMockL2(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/propose" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req ProposeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(req.Domains) == 0 || req.Domains[0] != "onboarding-smoke" {
			t.Errorf("expected onboarding-smoke domain, got %v", req.Domains)
		}
		// The server requires the nested `insight` object — a flat body
		// is rejected 422. Assert the probe sends it nested.
		if req.Insight.Summary == "" || req.Insight.Action == "" {
			t.Errorf("expected nested insight {summary,action}, got %+v", req.Insight)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ProposeResponse{
			UnitID: "ku-123",
			Tier:   "private",
		})
	})
	defer stop()

	resp, err := c.SmokePropose(context.Background(), "alice")
	if err != nil {
		t.Fatalf("SmokePropose: %v", err)
	}
	if !resp.SmokeOK() {
		t.Fatalf("SmokeOK=false on tier=private")
	}
}

func TestSmokeProposeTierLocal(t *testing.T) {
	c, stop := newMockL2(t, func(w http.ResponseWriter, r *http.Request) {
		// Server fell back to local-only propose — binding didn't take.
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ProposeResponse{
			UnitID: "ku-local",
			Tier:   "local",
		})
	})
	defer stop()

	resp, err := c.SmokePropose(context.Background(), "alice")
	if err != nil {
		t.Fatalf("SmokePropose: %v", err)
	}
	if resp.SmokeOK() {
		t.Fatal("SmokeOK=true on tier=local — should be false")
	}
}

func TestMintAPIKey(t *testing.T) {
	c, stop := newMockL2(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/api-keys" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(MintAPIKeyResponse{
			APIKey: "cqa.v1.newkey.value",
			KeyID:  "newkeyid",
		})
	})
	defer stop()

	resp, err := c.MintAPIKey(context.Background(), MintAPIKeyRequest{Label: "rotated"})
	if err != nil {
		t.Fatalf("MintAPIKey: %v", err)
	}
	if resp.APIKey != "cqa.v1.newkey.value" {
		t.Fatalf("APIKey mismatch: %q", resp.APIKey)
	}
}

func TestMintAPIKeyEmptyResponse(t *testing.T) {
	c, stop := newMockL2(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(MintAPIKeyResponse{})
	})
	defer stop()

	if _, err := c.MintAPIKey(context.Background(), MintAPIKeyRequest{Label: "x"}); err == nil {
		t.Fatal("expected error on empty api_key")
	}
}

func TestMintAPIKeyRequiresLabel(t *testing.T) {
	c, stop := newMockL2(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit when label is empty")
	})
	defer stop()
	if _, err := c.MintAPIKey(context.Background(), MintAPIKeyRequest{}); err == nil {
		t.Fatal("expected label-required error")
	}
}

func TestRevokeAPIKey(t *testing.T) {
	called := false
	c, stop := newMockL2(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/api/v1/auth/api-keys/abc123" {
			t.Errorf("path = %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	defer stop()
	if err := c.RevokeAPIKey(context.Background(), "abc123"); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}
