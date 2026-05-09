package l2client

import (
	"context"
	"fmt"
	"net/http"
)

// AuthMeResponse is what /api/v1/auth/me returns on success.
//
// The shape is intentionally narrow — extra fields are ignored. This
// keeps us forward-compatible with server additions.
type AuthMeResponse struct {
	EnterpriseID string `json:"enterprise_id"`
	GroupID      string `json:"group_id"`
	Persona      string `json:"persona"`
	KeyID        string `json:"key_id,omitempty"`
	Tier         string `json:"tier,omitempty"`
}

// AuthMe probes /auth/me and returns the binding the L2 server thinks
// this token is for.
func (c *Client) AuthMe(ctx context.Context) (*AuthMeResponse, error) {
	var out AuthMeResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MintAPIKeyRequest is the body for /auth/api-keys POST. Used by
// `8l rotate-key`. Mint authority is enforced server-side; we just
// pass a label.
type MintAPIKeyRequest struct {
	Label   string `json:"label"`
	Persona string `json:"persona,omitempty"`
}

// MintAPIKeyResponse contains the freshly minted key. The full secret
// is returned ONCE — the caller must persist it immediately (in our
// case, into the profile JSON).
type MintAPIKeyResponse struct {
	APIKey    string `json:"api_key"`
	KeyID     string `json:"key_id"`
	CreatedAt string `json:"created_at"`
}

// MintAPIKey calls /api/v1/auth/api-keys to generate a fresh key.
//
// Authentication uses the EXISTING client APIKey. After a successful
// rotation the caller should swap c.APIKey to the new value.
func (c *Client) MintAPIKey(ctx context.Context, req MintAPIKeyRequest) (*MintAPIKeyResponse, error) {
	if req.Label == "" {
		return nil, fmt.Errorf("l2client: MintAPIKey label required")
	}
	var out MintAPIKeyResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/auth/api-keys", req, &out); err != nil {
		return nil, err
	}
	if out.APIKey == "" {
		return nil, fmt.Errorf("l2client: mint returned empty api_key")
	}
	return &out, nil
}

// RevokeAPIKey calls DELETE /api/v1/auth/api-keys/<key_id>. Used by
// `8l unjoin --revoke` (V2) and `8l rotate-key` (cleanup of old key).
func (c *Client) RevokeAPIKey(ctx context.Context, keyID string) error {
	if keyID == "" {
		return fmt.Errorf("l2client: RevokeAPIKey key_id required")
	}
	return c.do(ctx, http.MethodDelete, "/api/v1/auth/api-keys/"+keyID, nil, nil)
}
