package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// mockL2 serves /api/v1/auth/me with the given identity + status.
func mockL2(enterprise, group, persona string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" {
			http.NotFound(w, r)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"enterprise_id": enterprise, "group_id": group, "persona": persona,
		})
	}))
}

func joinF() *joinFlags {
	return &joinFlags{Enterprise: "acme", L2: "default", Persona: "agent"}
}

const testKey = "cqa.v1.0123456789abcdef0123456789abcdef." +
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestBindStaleAuthFailThenCorrect(t *testing.T) {
	stale := mockL2("acme", "default", "agent", http.StatusUnauthorized) // 401
	defer stale.Close()
	good := mockL2("acme", "default", "agent", http.StatusOK)
	defer good.Close()

	got, err := bindEndpoint(io.Discard, joinF(), testKey, []string{stale.URL, good.URL})
	if err != nil {
		t.Fatalf("bindEndpoint: %v", err)
	}
	if got != good.URL {
		t.Fatalf("bound %q; want the healthy candidate %q (a stale 401 must not shadow it)", got, good.URL)
	}
}

func TestBindNetworkFailThenCorrect(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // now unreachable → network error on probe
	good := mockL2("acme", "default", "agent", http.StatusOK)
	defer good.Close()

	got, err := bindEndpoint(io.Discard, joinF(), testKey, []string{deadURL, good.URL})
	if err != nil {
		t.Fatalf("bindEndpoint: %v", err)
	}
	if got != good.URL {
		t.Fatalf("bound %q; want %q after a dead first candidate", got, good.URL)
	}
}

func TestBindRejectsIdentityMismatch(t *testing.T) {
	cases := []struct{ ent, grp, per string }{
		{"other", "default", "agent"}, // wrong enterprise
		{"acme", "finance", "agent"},  // wrong group
		{"acme", "default", "admin"},  // wrong persona
		{"", "default", "agent"},      // empty enterprise (no compatibility fallback)
		{"acme", "", "agent"},         // empty group
		{"acme", "default", ""},       // empty persona
	}
	for _, c := range cases {
		srv := mockL2(c.ent, c.grp, c.per, http.StatusOK)
		_, err := bindEndpoint(io.Discard, joinF(), testKey, []string{srv.URL})
		srv.Close()
		if err == nil {
			t.Fatalf("identity (%q,%q,%q) should be rejected against (acme,default,agent)", c.ent, c.grp, c.per)
		}
	}
}

func TestBindNoSmokeStillAuthenticates(t *testing.T) {
	// --no-smoke must NOT bypass endpoint authentication; bindEndpoint has no
	// skip path, so a NoSmoke join still validates the binding here.
	f := joinF()
	f.NoSmoke = true
	good := mockL2("acme", "default", "agent", http.StatusOK)
	defer good.Close()
	got, err := bindEndpoint(io.Discard, f, testKey, []string{good.URL})
	if err != nil || got != good.URL {
		t.Fatalf("no-smoke bind = %q, err=%v; must still authenticate", got, err)
	}
}

func TestBindRefusesRedirectAndNeverLeaksKeyToAttacker(t *testing.T) {
	var attackerHits int32
	var attackerSawAuth int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attackerHits, 1)
		if r.Header.Get("Authorization") != "" {
			atomic.AddInt32(&attackerSawAuth, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	// First candidate 302-redirects /auth/me to the attacker origin.
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/api/v1/auth/me", http.StatusFound)
	}))
	defer redir.Close()
	good := mockL2("acme", "default", "agent", http.StatusOK)
	defer good.Close()

	got, err := bindEndpoint(io.Discard, joinF(), testKey, []string{redir.URL, good.URL})
	if err != nil {
		t.Fatalf("bindEndpoint: %v", err)
	}
	if got != good.URL {
		t.Fatalf("bound %q; want %q (redirect candidate must be skipped)", got, good.URL)
	}
	if atomic.LoadInt32(&attackerHits) != 0 {
		t.Fatalf("attacker origin received %d request(s); the redirect must NOT be followed", attackerHits)
	}
	if atomic.LoadInt32(&attackerSawAuth) != 0 {
		t.Fatal("attacker received the Authorization header — credential leak")
	}
}

func TestBindAuthFailThenNetworkSurfacesAsAuth(t *testing.T) {
	// The exact dummy-key live path: route53 → 401, legacy → DNS/network failure.
	// The result must be an AUTH error (a real L2 rejected the key), not a DNS error.
	rejecting := mockL2("acme", "default", "agent", http.StatusUnauthorized)
	defer rejecting.Close()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // unreachable → network error

	_, err := bindEndpoint(io.Discard, joinF(), testKey, []string{rejecting.URL, deadURL})
	if err == nil {
		t.Fatal("expected an error")
	}
	var ec ExitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != ExitAuthFail {
		t.Fatalf("expected ExitAuthFail (a real L2 rejected the key), got %v (code path must not surface DNS)", err)
	}
}

func TestBindMismatchFirstThenExactMatchSecond(t *testing.T) {
	// A reachable candidate that authenticates but to the WRONG identity must NOT
	// shadow a later candidate that matches exactly — probe-all, bind-first-exact.
	wrong := mockL2("other-tenant", "default", "agent", http.StatusOK)
	defer wrong.Close()
	right := mockL2("acme", "default", "agent", http.StatusOK)
	defer right.Close()

	got, err := bindEndpoint(io.Discard, joinF(), testKey, []string{wrong.URL, right.URL})
	if err != nil {
		t.Fatalf("bindEndpoint: %v", err)
	}
	if got != right.URL {
		t.Fatalf("bound %q; want %q (a mismatched first candidate must not shadow the exact match)", got, right.URL)
	}
}

func TestBindAllFailReturnsAuthWhenAnyRejected(t *testing.T) {
	a := mockL2("acme", "default", "agent", http.StatusUnauthorized)
	defer a.Close()
	b := mockL2("acme", "default", "agent", http.StatusForbidden)
	defer b.Close()
	_, err := bindEndpoint(io.Discard, joinF(), testKey, []string{a.URL, b.URL})
	if err == nil {
		t.Fatal("expected an error when no candidate authenticates")
	}
	// An auth-class failure (a real L2 answered + rejected) should surface as auth,
	// not as a DNS/unreachable error.
	if !strings.Contains(strings.ToLower(err.Error()), "auth") && !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected an auth-class error, got: %v", err)
	}
}
