package resolver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeOrigin(t *testing.T) {
	ok := map[string]string{
		"https://Eng.Acme.8th-layer.ai/":        "https://eng.acme.8th-layer.ai",        // lowercased, slash dropped
		"https://acme.enterprise.8th-layer.ai.": "https://acme.enterprise.8th-layer.ai", // terminal dot
		"https://l2.corp:443":                   "https://l2.corp",                      // default https port dropped
		"https://l2.corp:8443":                  "https://l2.corp:8443",                 // non-default port kept
	}
	for in, want := range ok {
		got, valid := normalizeOrigin(in, false)
		if !valid || got != want {
			t.Fatalf("normalizeOrigin(%q) = %q,%v; want %q,true", in, got, valid, want)
		}
	}
	// IP literals rejected when not on the dev path; rejected entirely with allowLoopbackHTTP=false.
	for _, bad := range []string{
		"https://203.0.113.5", "https://x.example.com/p", "https://x.example.com?q=1",
		"https://u:p@x.example.com", "ftp://x.example.com", "http://x.example.com", "https://",
	} {
		if _, valid := normalizeOrigin(bad, false); valid {
			t.Fatalf("normalizeOrigin(%q) should be invalid", bad)
		}
	}
	// loopback http allowed only on the dev path; loopback IP allowed there.
	if _, valid := normalizeOrigin("http://127.0.0.1:8080", true); !valid {
		t.Fatal("loopback http should be valid with allowLoopbackHTTP")
	}
	if _, valid := normalizeOrigin("http://127.0.0.1:8080", false); valid {
		t.Fatal("loopback http must be invalid without allowLoopbackHTTP")
	}
}

func TestDirectoryEndpointBestEffort(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/directory/enterprises/acme/l2-endpoint":
			_, _ = w.Write([]byte(`{"enterprise_id":"acme","l2_id":"acme/primary","endpoint_url":"https://acme.enterprise.8th-layer.ai/"}`))
		case r.URL.Path == "/api/v1/directory/enterprises/badjson/l2-endpoint":
			_, _ = w.Write([]byte(`{not json`))
		case r.URL.Path == "/api/v1/directory/enterprises/huge/l2-endpoint":
			_, _ = w.Write([]byte(`{"endpoint_url":"https://x.8th-layer.ai","pad":"` + strings.Repeat("A", 20000) + `"}`))
		case r.URL.Path == "/api/v1/directory/enterprises/iporigin/l2-endpoint":
			_, _ = w.Write([]byte(`{"endpoint_url":"https://203.0.113.9"}`)) // non-normalizable → ""
		default:
			http.NotFound(w, r)
		}
	}))
	defer good.Close()
	t.Setenv(DirectoryURLEnv, good.URL)

	if got := DirectoryEndpoint(context.Background(), "acme"); got != "https://acme.enterprise.8th-layer.ai" {
		t.Fatalf("acme recommendation = %q (want normalized origin)", got)
	}
	for _, ent := range []string{"unknown404", "badjson", "iporigin"} {
		if got := DirectoryEndpoint(context.Background(), ent); got != "" {
			t.Fatalf("DirectoryEndpoint(%q) = %q; want \"\" (best-effort)", ent, got)
		}
	}
	// oversized body is truncated → JSON decode fails → "" (never a join failure)
	if got := DirectoryEndpoint(context.Background(), "huge"); got != "" && got != "https://x.8th-layer.ai" {
		t.Fatalf("huge body handling = %q", got)
	}
}

func TestDirectoryEndpointSkippedUnderOverride(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "https://pinned.example.com")
	t.Setenv(DirectoryURLEnv, "https://should-not-be-queried.invalid")
	if got := DirectoryEndpoint(context.Background(), "acme"); got != "" {
		t.Fatalf("directory must not be queried under override; got %q", got)
	}
}

func TestDirectoryEndpointRefusesRedirect(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")
	attackerHit := false
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerHit = true
		_, _ = w.Write([]byte(`{"endpoint_url":"https://evil.example.com"}`))
	}))
	defer attacker.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+r.URL.Path, http.StatusFound)
	}))
	defer redir.Close()
	t.Setenv(DirectoryURLEnv, redir.URL)
	if got := DirectoryEndpoint(context.Background(), "acme"); got != "" {
		t.Fatalf("redirected directory must yield \"\"; got %q", got)
	}
	if attackerHit {
		t.Fatal("directory redirect was followed to the attacker origin")
	}
}

func TestDirectoryEndpointRejectsBadBaseURL(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")
	t.Setenv(DirectoryURLEnv, "http://evil.example.com") // non-loopback http → rejected
	if got := DirectoryEndpoint(context.Background(), "acme"); got != "" {
		t.Fatalf("bad CQ_DIRECTORY_URL must yield \"\"; got %q", got)
	}
}
