package resolver

import "testing"

func TestEndpoint(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")
	got, err := Endpoint("8th-layer-corp", "engineering")
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	// route53 edge is the current default (issue #204): per-enterprise host.
	want := "https://8th-layer-corp.enterprise.8th-layer.ai"
	if got != want {
		t.Fatalf("Endpoint = %q want %q", got, want)
	}
}

func TestCandidates(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")
	got, err := Candidates("8th-layer-corp", "engineering")
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	want := []string{
		"https://8th-layer-corp.enterprise.8th-layer.ai",  // route53 first
		"https://engineering.8th-layer-corp.8th-layer.ai", // legacy second
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Candidates = %v want %v", got, want)
	}
}

func TestCandidatesNonDNSLabelL2DropsLegacy(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")
	// A group that is not a DNS label (route53 groups can be broader) must NOT
	// error — it just drops the legacy candidate, keeping the route53 one.
	got, err := Candidates("acme", "team_alpha")
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(got) != 1 || got[0] != "https://acme.enterprise.8th-layer.ai" {
		t.Fatalf("non-DNS l2 candidates = %v; want route53-only", got)
	}
}

func TestCandidatesEnterpriseMustBeDNSLabel(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")
	for _, ent := range []string{"", "UPPER", "has_underscore", "ends-", "a..b", "white space"} {
		if _, err := Candidates(ent, "default"); err == nil {
			t.Fatalf("enterprise %q should be rejected as a non-DNS label", ent)
		}
	}
}

func TestCandidatesOverrideValidation(t *testing.T) {
	ok := []string{
		"https://customer.example.com/",      // trailing slash trimmed
		"https://l2.internal.corp:8443",      // explicit port ok
		"http://localhost:8080",              // loopback http dev exception
		"http://127.0.0.1",                   // loopback http dev exception
	}
	for _, v := range ok {
		t.Setenv(EndpointEnvOverride, v)
		got, err := Candidates("ent", "l2")
		if err != nil {
			t.Fatalf("override %q should be accepted: %v", v, err)
		}
		if len(got) != 1 {
			t.Fatalf("override %q -> %v; want single candidate", v, got)
		}
	}
	bad := []string{
		"http://evil.example.com",         // non-loopback http rejected
		"ftp://x.example.com",             // wrong scheme
		"https://user:pass@x.example.com", // userinfo rejected
		"https://x.example.com/some/path", // path rejected (origin-only)
		"https://x.example.com//evil",     // double-slash path must NOT be trimmed-away
		"https://x.example.com/%2f..",     // encoded path segment rejected
		"https://x.example.com/?q=1",      // query rejected
		"https://x.example.com/#frag",     // fragment rejected
		"https://",                        // no host
		"not-a-url and spaces",            // unparseable / no scheme
	}
	for _, v := range bad {
		t.Setenv(EndpointEnvOverride, v)
		if _, err := Candidates("ent", "l2"); err == nil {
			t.Fatalf("override %q should be REJECTED", v)
		}
	}
}

func TestOverrideReturnsNormalizedOrigin(t *testing.T) {
	// The returned candidate is a RECONSTRUCTED origin (scheme://host[:port]),
	// not the raw input — no trailing slash, no smuggled path.
	t.Setenv(EndpointEnvOverride, "https://l2.corp:8443/")
	got, err := Candidates("ent", "l2")
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(got) != 1 || got[0] != "https://l2.corp:8443" {
		t.Fatalf("normalized override = %v; want [https://l2.corp:8443]", got)
	}
}

func TestEndpointOverride(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "https://customer.example.com/")
	got, err := Endpoint("ignored", "ignored")
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	if got != "https://customer.example.com" {
		t.Fatalf("Endpoint override = %q", got)
	}
}

func TestEndpointMissing(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")
	// A non-DNS-label enterprise is fatal (it IS the route53 hostname label).
	if _, err := Endpoint("", "x"); err == nil {
		t.Fatal("expected enterprise-required error")
	}
	// An empty/non-DNS group is NOT fatal: route53 doesn't put the group in the
	// hostname, so Endpoint still yields the route53 host (group equality is
	// enforced later against /auth/me).
	got, err := Endpoint("acme", "")
	if err != nil {
		t.Fatalf("Endpoint(acme, \"\") should succeed (route53-only): %v", err)
	}
	if got != "https://acme.enterprise.8th-layer.ai" {
		t.Fatalf("Endpoint(acme, \"\") = %q", got)
	}
}

func TestResolveAPIKey(t *testing.T) {
	literal := "cqa.v1.0123456789abcdef0123456789abcdef." + repeat("a", 52)
	got, err := ResolveAPIKey(literal)
	if err != nil {
		t.Fatalf("literal: %v", err)
	}
	if got != literal {
		t.Fatalf("literal round-trip failed")
	}

	t.Setenv("MY_KEY", literal)
	got, err = ResolveAPIKey("$MY_KEY")
	if err != nil {
		t.Fatalf("$VAR: %v", err)
	}
	if got != literal {
		t.Fatalf("$VAR round-trip failed: %q", got)
	}

	got, err = ResolveAPIKey("${MY_KEY}")
	if err != nil {
		t.Fatalf("${VAR}: %v", err)
	}
	if got != literal {
		t.Fatalf("${VAR} round-trip failed")
	}
}

func TestResolveAPIKeyErrors(t *testing.T) {
	if _, err := ResolveAPIKey(""); err == nil {
		t.Fatal("empty key should error")
	}
	if _, err := ResolveAPIKey("not-a-cqa-key"); err == nil {
		t.Fatal("malformed key should error")
	}
	if _, err := ResolveAPIKey("$NONEXISTENT_VAR_xyz"); err == nil {
		t.Fatal("missing env should error")
	}
	// Regression: the secret tail is exactly 52 url-safe chars (the L2
	// server mints token_urlsafe(39)). Reject the off-by-N shapes — a
	// 64-char tail was the original wrong validator and broke every join.
	pfx := "cqa.v1.0123456789abcdef0123456789abcdef."
	for _, n := range []int{51, 53, 64} {
		if _, err := ResolveAPIKey(pfx + repeat("a", n)); err == nil {
			t.Fatalf("tail of %d chars should be rejected", n)
		}
	}
}

func TestKeyID(t *testing.T) {
	literal := "cqa.v1.0123456789abcdef0123456789abcdef." + repeat("a", 52)
	if got := KeyID(literal); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("KeyID = %q", got)
	}
	if got := KeyID("nope"); got != "<invalid>" {
		t.Fatalf("KeyID(invalid) = %q", got)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
