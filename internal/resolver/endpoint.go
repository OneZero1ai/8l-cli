// Package resolver derives endpoint URLs and parses --api-key inputs.
package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// EndpointEnvOverride lets a customer Enterprise with non-canonical DNS
// (e.g. a private endpoint) override the derived URL. Set in the user's
// environment before running `8l join`. It is the ONLY way the CLI will send
// the API key to a non-8th-Layer-owned host — directory-driven discovery is
// deliberately not a credential destination (issue #204; codex security review).
const EndpointEnvOverride = "CQ_ADDR_OVERRIDE"

// DirectoryURLEnv overrides the directory base URL the resolver QUERIES (never
// with the L2 bearer) for a candidate-ordering recommendation.
const DirectoryURLEnv = "CQ_DIRECTORY_URL"

// defaultDirectoryURL matches the marketplace CQ_DIRECTORY_URL default.
const defaultDirectoryURL = "https://directory.8th-layer.ai"

// directoryRespCap bounds the directory response body (best-effort recommendation).
const directoryRespCap = 8 << 10 // 8 KiB

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

// normalizeOrigin parses a URL and returns its canonical ORIGIN — the only thing
// the API key may ever be sent to. It enforces: https (or http for loopback when
// allowLoopbackHTTP), a non-empty lowercased host with one terminal dot stripped,
// NO userinfo / query / fragment / path (incl. "//" and %2f), IP literals rejected,
// and default ports (443/80) dropped so `host` and `host:443` compare equal. The
// reconstructed origin is returned (never the raw input) so two inputs that mean
// the same origin normalize identically — that exact-equality is the security hinge
// for accepting a directory recommendation. ok=false on any violation.
func normalizeOrigin(raw string, allowLoopbackHTTP bool) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	// Origin-only: reject ANY path, including "//", "///", and percent-encoded
	// segments — exact equality, not a slash-trim (which would accept "//evil").
	if u.Path != "" && u.Path != "/" {
		return "", false
	}
	if u.RawPath != "" {
		return "", false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return "", false
	}
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	// Reject IP literals (origin must be a name) — EXCEPT loopback on the dev path,
	// where 127.0.0.1/::1 are how local/test servers are reached.
	if net.ParseIP(host) != nil && !(allowLoopbackHTTP && loopback) {
		return "", false
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !(allowLoopbackHTTP && loopback) {
			return "", false
		}
	default:
		return "", false
	}
	// Bracket IPv6 literals (e.g. ::1) so the reconstructed origin is valid.
	hostpart := host
	if strings.Contains(host, ":") {
		hostpart = "[" + host + "]"
	}
	out := u.Scheme + "://" + hostpart
	if port := u.Port(); port != "" &&
		!((u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80")) {
		out += ":" + port
	}
	return out, true
}

// validateOverride enforces that CQ_ADDR_OVERRIDE is an ORIGIN-only URL the API
// key may be sent to (https, or http loopback for dev). Returns the normalized origin.
func validateOverride(v string) (string, error) {
	origin, ok := normalizeOrigin(v, true)
	if !ok {
		return "", fmt.Errorf("resolver: %s=%q must be an https origin (no path/query/fragment/userinfo/IP; http allowed only for localhost)", EndpointEnvOverride, v)
	}
	return origin, nil
}

// DirectoryEndpoint asks the directory for the enterprise's recommended L2 origin.
// It is a BEST-EFFORT, BOUNDED hint — NEVER a credential destination: the L2 bearer
// is not sent here, and the caller acts on the result ONLY when it exactly equals a
// candidate it independently derives (codex). Returns "" on ANY failure (override
// set, bad CQ_DIRECTORY_URL, timeout, redirect, non-200, oversized/malformed body,
// or a non-normalizable origin) so a join never fails because of the directory.
func DirectoryEndpoint(ctx context.Context, enterprise string) string {
	if enterprise == "" || os.Getenv(EndpointEnvOverride) != "" {
		return "" // never override-path; never query when the operator pinned a URL
	}
	base := os.Getenv(DirectoryURLEnv)
	if base == "" {
		base = defaultDirectoryURL
	}
	// The directory URL receives no bearer, but still validate it origin-only +
	// refuse redirects so the CLI isn't turned into a network oracle / spoof target.
	baseOrigin, ok := normalizeOrigin(base, true)
	if !ok {
		return ""
	}
	u := baseOrigin + "/api/v1/directory/enterprises/" + url.PathEscape(enterprise) + "/l2-endpoint"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "8l-cli/0.1")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{
		// Short, capped: this is a non-authoritative ordering hint — a directory
		// outage must NOT delay trying the already-known route53 host (codex).
		Timeout: 2 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("resolver: refusing directory redirect")
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	// Read at most cap+1 and REJECT if over the cap (don't let json.Decode accept a
	// valid prefix of an oversized/trailing-JSON body). Then require ONE complete
	// document whose binding fields match the request (codex).
	body, err := io.ReadAll(io.LimitReader(resp.Body, directoryRespCap+1))
	if err != nil || len(body) > directoryRespCap {
		return ""
	}
	var out struct {
		EnterpriseID string `json:"enterprise_id"`
		L2ID         string `json:"l2_id"`
		EndpointURL  string `json:"endpoint_url"`
	}
	if json.Unmarshal(body, &out) != nil {
		return "" // trailing bytes / non-single-object → invalid
	}
	if out.EnterpriseID != enterprise || out.L2ID == "" || out.EndpointURL == "" {
		return "" // recommendation must be bound to the requested enterprise
	}
	origin, ok := normalizeOrigin(out.EndpointURL, false) // a real directory answer is https
	if !ok {
		return ""
	}
	return origin
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
