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
// environment before running `8l join`.
const EndpointEnvOverride = "CQ_ADDR_OVERRIDE"

// Endpoint returns the canonical URL for an (enterprise, l2) pair, or
// the override URL if CQ_ADDR_OVERRIDE is set.
//
// Canonical shape: https://<l2>.<enterprise>.8th-layer.ai
func Endpoint(enterprise, l2 string) (string, error) {
	if v := os.Getenv(EndpointEnvOverride); v != "" {
		if _, err := url.Parse(v); err != nil {
			return "", fmt.Errorf("resolver: %s=%q invalid: %w", EndpointEnvOverride, v, err)
		}
		return strings.TrimRight(v, "/"), nil
	}
	if enterprise == "" {
		return "", fmt.Errorf("resolver: enterprise required")
	}
	if l2 == "" {
		return "", fmt.Errorf("resolver: l2 required")
	}
	return fmt.Sprintf("https://%s.%s.8th-layer.ai", l2, enterprise), nil
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

// keyShape matches `cqa.v1.<32hex>.<64chars>` per Decision 28.
//
// The 32hex is the key ID (lookup key, safe to log); the 64chars is the
// secret (NEVER log). The split allows the L2 to validate the key shape
// before doing a DB lookup.
var keyShape = regexp.MustCompile(`^cqa\.v1\.[0-9a-f]{32}\.[A-Za-z0-9_-]{64}$`)

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
		return "", fmt.Errorf("resolver: api key shape invalid (expected cqa.v1.<32hex>.<64chars>)")
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
