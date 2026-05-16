package resolver

import "testing"

func TestEndpoint(t *testing.T) {
	t.Setenv(EndpointEnvOverride, "")
	got, err := Endpoint("8th-layer-corp", "engineering")
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	want := "https://engineering.8th-layer-corp.8th-layer.ai"
	if got != want {
		t.Fatalf("Endpoint = %q want %q", got, want)
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
	if _, err := Endpoint("", "x"); err == nil {
		t.Fatal("expected enterprise-required error")
	}
	if _, err := Endpoint("x", ""); err == nil {
		t.Fatal("expected l2-required error")
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
