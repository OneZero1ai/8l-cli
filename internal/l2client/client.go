// Package l2client is a small HTTP client for the 8th-Layer L2 API.
//
// Scope V1: just enough surface to support `8l join`, `status`,
// `doctor`, and `rotate-key`. We deliberately do NOT import the
// 8th-layer-agent server code — the contract is fetched via OpenAPI
// at test time and hand-modeled here.
package l2client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a thin authenticated HTTP wrapper.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	UserAgent  string
	// Verbose dumps requests/responses to stderr-attached logger when set.
	Verbose VerboseLogger
}

// VerboseLogger is the minimal subset of log.Logger we need. Tests can
// supply a buffer; CLI passes log.New(os.Stderr, ...).
type VerboseLogger interface {
	Printf(format string, args ...any)
}

// New returns a Client with sensible defaults.
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			// Every request here carries the API key as a Bearer token. NEVER
			// follow a redirect: a 3xx to another origin would resend the
			// Authorization header to a host we didn't intend to trust, leaking
			// the credential (issue #204 / codex security review). Returning an
			// error means net/http does NOT issue the redirected request, so the
			// target receives nothing; Do returns the (closed) 3xx response plus
			// this error, which the caller treats as a non-auth failure.
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				return fmt.Errorf("l2client: refusing redirect to %s (credential confidentiality)", req.URL.Redacted())
			},
		},
		UserAgent: "8l-cli/0.1",
	}
}

// HTTPError surfaces non-2xx responses without leaking the body to the
// terminal by default. The Body is captured for diagnostic verbose logs.
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	body := e.Body
	if len(body) > 200 {
		body = body[:200] + "…"
	}
	return fmt.Sprintf("%s %s → %s: %s", e.Method, e.Path, e.Status, body)
}

// IsAuth returns true if the error is 401/403.
func IsAuth(err error) bool {
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	return he.StatusCode == http.StatusUnauthorized || he.StatusCode == http.StatusForbidden
}

// do issues a JSON-bodied request and decodes the JSON response into out
// (out may be nil). Non-2xx returns *HTTPError with the body captured.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	if c.BaseURL == "" {
		return errors.New("l2client: BaseURL empty")
	}
	if c.APIKey == "" {
		return errors.New("l2client: APIKey empty")
	}

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("l2client: marshal: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("l2client: build req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Verbose != nil {
		c.Verbose.Printf("→ %s %s", method, c.BaseURL+path)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("l2client: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("l2client: read body: %w", err)
	}

	if c.Verbose != nil {
		preview := string(respBody)
		if len(preview) > 400 {
			preview = preview[:400] + "…"
		}
		c.Verbose.Printf("← %d %s", resp.StatusCode, preview)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(respBody),
		}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("l2client: decode: %w", err)
		}
	}
	return nil
}
