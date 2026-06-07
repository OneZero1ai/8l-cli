// Package resolver derives endpoint URLs and parses --api-key inputs.
package resolver

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// EndpointEnvOverride lets a customer Enterprise with non-canonical DNS
// (e.g. a private endpoint) override the derived URL. Set in the user's
// environment before running `8l join`. It is the ONLY way the CLI will send
// the API key to a non-8th-Layer-owned host — directory-driven discovery is
// deliberately not a credential destination (issue #204; codex security review).
const EndpointEnvOverride = "CQ_ADDR_OVERRIDE"

// enterpriseEdgeSuffix is the route53 per-enterprise edge suffix (Decision 43):
// a route53 L2 lives at https://<enterprise>.enterprise.8th-layer.ai — one
// ALB/cert/FQDN per enterprise slug; the L2 group is an internal partition, so
// it is NOT part of the hostname (issue #204).
const enterpriseEdgeSuffix = "enterprise.8th-layer.ai"

// dnsLabel matches one DNS label (RFC 1123): lowercase alnum + internal hyphen,
// 1–63 chars. Enterprise/L2 slugs are validated against this BEFORE they are
// interpolated into a candidate host, so a crafted slug can't smuggle a path,
// port, or extra host labels into the URL the API key is sent to.
var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Candidates returns the ordered base URLs to try for an (enterprise, l2) pair,
// most-likely first. These are DETERMINISTIC, 8th-Layer-owned hosts only — a
// caller probes each with the API key and binds to whichever authenticates, so
// the key is only ever sent to a host the project controls. If CQ_ADDR_OVERRIDE
// is set it is the sole candidate (validated origin-only).
//
//  1. https://<enterprise>.enterprise.8th-layer.ai  (route53 edge, current default)
//  2. https://<l2>.<enterprise>.8th-layer.ai         (legacy cloudflare)
func Candidates(enterprise, l2 string) ([]string, error) {
	if v := os.Getenv(EndpointEnvOverride); v != "" {
		origin, err := validateOverride(v)
		if err != nil {
			return nil, err
		}
		return []string{origin}, nil
	}
	// The enterprise slug IS the route53 hostname label — it must be DNS-safe.
	if !dnsLabel.MatchString(enterprise) {
		return nil, fmt.Errorf("resolver: enterprise %q is not a valid DNS label", enterprise)
	}
	cands := []string{fmt.Sprintf("https://%s.%s", enterprise, enterpriseEdgeSuffix)}
	// The legacy scheme puts the group in the hostname, so add that candidate
	// ONLY when the group is itself a DNS label. In route53 mode the group is an
	// internal partition and may be broader than a DNS label — that's fine; the
	// route53 candidate above doesn't use it (codex). Group equality is still
	// enforced against /auth/me by the caller.
	if dnsLabel.MatchString(l2) {
		cands = append(cands, fmt.Sprintf("https://%s.%s.8th-layer.ai", l2, enterprise))
	}
	return cands, nil
}

// validateOverride enforces that CQ_ADDR_OVERRIDE is an ORIGIN-only URL the API
// key may be sent to: https (or http for loopback only), a non-empty host, and
// no userinfo / query / fragment / path. Returns the normalized origin.
func validateOverride(v string) (string, error) {
	u, err := url.Parse(v)
	if err != nil {
		return "", fmt.Errorf("resolver: %s=%q invalid: %w", EndpointEnvOverride, v, err)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("resolver: %s=%q has no host", EndpointEnvOverride, v)
	}
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	switch u.Scheme {
	case "https":
		// ok
	case "http":
		if !loopback {
			return "", fmt.Errorf("resolver: %s=%q — http is allowed only for loopback (localhost) dev", EndpointEnvOverride, v)
		}
	default:
		return "", fmt.Errorf("resolver: %s=%q must be https (http://localhost allowed for dev)", EndpointEnvOverride, v)
	}
	if u.User != nil {
		return "", fmt.Errorf("resolver: %s must not contain credentials (userinfo)", EndpointEnvOverride)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("resolver: %s must be an origin (no query/fragment)", EndpointEnvOverride)
	}
	if p := strings.Trim(u.Path, "/"); p != "" {
		return "", fmt.Errorf("resolver: %s must be an origin (no path), got path %q", EndpointEnvOverride, u.Path)
	}
	return strings.TrimRight(strings.TrimSuffix(v, "/"), "/"), nil
}

// Endpoint returns the most-likely canonical URL for an (enterprise, l2) pair
// (route53 edge first), or the override URL if CQ_ADDR_OVERRIDE is set. Callers
// that hold an API key should prefer Candidates + an authenticated probe; this
// is the no-probe default (e.g. doctor).
func Endpoint(enterprise, l2 string) (string, error) {
	cands, err := Candidates(enterprise, l2)
	if err != nil {
		return "", err
	}
	return cands[0], nil
}

// Host returns just the hostname for an (enterprise, l2) pair. Used by
// the doctor command for DNS probing.
func Host(enterprise, l2 string) (string, error) {
	endpoint, err := Endpoint(enterprise, l2)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("resolver: parse endpoint: %w", err)
	}
	return u.Hostname(), nil
}

// keyShape matches `cqa.v1.<32hex>.<52chars>` — the shape the L2 server
// actually mints (POST /auth/api-keys) and the cq plugin validates
// (cq_setup.py API_KEY_RE). The 52-char tail is a url-safe base64 of
// 39 secret bytes (secrets.token_urlsafe in api_keys.py).
//
// The 32hex is the key ID (lookup key, safe to log); the 52chars is the
// secret (NEVER log). The split allows the L2 to validate the key shape
// before doing a DB lookup.
var keyShape = regexp.MustCompile(`^cqa\.v1\.[0-9a-f]{32}\.[A-Za-z0-9_-]{52}$`)

// ResolveAPIKey accepts either a literal key or `$VAR` indirection.
//
// Indirection:
//   - "$FOO"         → os.Getenv("FOO")
//   - "${FOO}"       → os.Getenv("FOO")
//   - "cqa.v1.…"     → returned as-is
//
// The resolved key is then format-checked against the cqa.v1 shape.
func ResolveAPIKey(in string) (string, error) {
	if in == "" {
		return "", fmt.Errorf("resolver: api key required")
	}
	resolved := in
	if strings.HasPrefix(in, "$") {
		name := strings.TrimPrefix(in, "$")
		name = strings.TrimPrefix(name, "{")
		name = strings.TrimSuffix(name, "}")
		if name == "" {
			return "", fmt.Errorf("resolver: empty env var name in %q", in)
		}
		v, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("resolver: env %s not set (referenced by --api-key)", name)
		}
		if v == "" {
			return "", fmt.Errorf("resolver: env %s is empty", name)
		}
		resolved = v
	}
	if !keyShape.MatchString(resolved) {
		// We deliberately do NOT echo the key in the error.
		return "", fmt.Errorf("resolver: api key shape invalid (expected cqa.v1.<32hex>.<52chars>)")
	}
	return resolved, nil
}

// KeyID extracts the 32-hex key id from a cqa.v1 key for safe logging.
// Returns the full string only if it looks like a valid key.
func KeyID(key string) string {
	if !keyShape.MatchString(key) {
		return "<invalid>"
	}
	parts := strings.Split(key, ".")
	if len(parts) < 4 {
		return "<invalid>"
	}
	return parts[2]
}
