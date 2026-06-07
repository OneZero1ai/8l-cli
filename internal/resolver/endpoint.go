// Package resolver derives endpoint URLs and parses --api-key inputs.
package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// EndpointEnvOverride lets a customer Enterprise with non-canonical DNS
// (e.g. a private endpoint) override the derived URL. Set in the user's
// environment before running `8l join`.
const EndpointEnvOverride = "CQ_ADDR_OVERRIDE"

// enterpriseEdgeSuffix is the route53 per-enterprise edge suffix (Decision 43):
// a route53 L2 lives at https://<enterprise>.enterprise.8th-layer.ai — one
// ALB/cert/FQDN per enterprise slug; the L2 group is an internal partition, so
// it is NOT part of the hostname (issue #204).
const enterpriseEdgeSuffix = "enterprise.8th-layer.ai"

// DirectoryURLEnv overrides the directory base URL the resolver queries.
const DirectoryURLEnv = "CQ_DIRECTORY_URL"

// defaultDirectoryURL is the directory the CLI resolves L2 URLs from when
// CQ_DIRECTORY_URL is unset (matches the marketplace CQ_DIRECTORY_URL default).
const defaultDirectoryURL = "https://directory.8th-layer.ai"

// Candidates returns the ordered base URLs to try for an (enterprise, l2) pair,
// most-likely first, so a caller can probe each with the real API key and use
// whichever authenticates (the key is ground truth — see issue #204). If
// CQ_ADDR_OVERRIDE is set it is the sole candidate.
//
//  1. https://<enterprise>.enterprise.8th-layer.ai  (route53 edge, current default)
//  2. https://<l2>.<enterprise>.8th-layer.ai         (legacy cloudflare)
func Candidates(enterprise, l2 string) ([]string, error) {
	if v := os.Getenv(EndpointEnvOverride); v != "" {
		if _, err := url.Parse(v); err != nil {
			return nil, fmt.Errorf("resolver: %s=%q invalid: %w", EndpointEnvOverride, v, err)
		}
		return []string{strings.TrimRight(v, "/")}, nil
	}
	if enterprise == "" {
		return nil, fmt.Errorf("resolver: enterprise required")
	}
	if l2 == "" {
		return nil, fmt.Errorf("resolver: l2 required")
	}
	return []string{
		fmt.Sprintf("https://%s.%s", enterprise, enterpriseEdgeSuffix),
		fmt.Sprintf("https://%s.%s.8th-layer.ai", l2, enterprise),
	}, nil
}

// Endpoint returns the most-likely canonical URL for an (enterprise, l2) pair
// (route53 edge first), or the override URL if CQ_ADDR_OVERRIDE is set. Callers
// that hold an API key should prefer Candidates + an authenticated probe; this
// is the no-probe default (e.g. doctor, --no-smoke).
func Endpoint(enterprise, l2 string) (string, error) {
	cands, err := Candidates(enterprise, l2)
	if err != nil {
		return "", err
	}
	return cands[0], nil
}

// DirectoryEndpoint best-effort resolves the enterprise's real L2 base URL from
// the directory (issue #204). Returns "" on ANY failure — the caller falls back
// to Candidates, and always validates the chosen URL with the API key, so a
// stale/missing directory answer is harmless.
func DirectoryEndpoint(ctx context.Context, enterprise string) string {
	if enterprise == "" || os.Getenv(EndpointEnvOverride) != "" {
		return ""
	}
	base := os.Getenv(DirectoryURLEnv)
	if base == "" {
		base = defaultDirectoryURL
	}
	u := strings.TrimRight(base, "/") + "/api/v1/directory/enterprises/" + url.PathEscape(enterprise) + "/l2-endpoint"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "8l-cli/0.1")
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out struct {
		EndpointURL string `json:"endpoint_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ""
	}
	return strings.TrimRight(out.EndpointURL, "/")
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
