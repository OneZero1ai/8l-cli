package l2client

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// ProposeRequest is the body for /api/v1/propose. We hand-model only
// the fields needed for the smoke probe; the server may accept more.
type ProposeRequest struct {
	Summary    string   `json:"summary"`
	Detail     string   `json:"detail"`
	Action     string   `json:"action"`
	Domains    []string `json:"domains"`
	Frameworks []string `json:"frameworks,omitempty"`
	Languages  []string `json:"languages,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
}

// ProposeResponse is the relevant slice of /propose's reply.
type ProposeResponse struct {
	UnitID string `json:"unit_id"`
	// Tier is the critical field for smoke: "private" means the binding
	// is taking effect; "local" means it isn't (token didn't authenticate
	// against an L2 group, fell back to local-only proposal).
	Tier      string `json:"tier"`
	CreatedAt string `json:"created_at,omitempty"`
}

// SmokeOK is true when Tier == "private".
func (r *ProposeResponse) SmokeOK() bool { return r.Tier == "private" }

// SmokePropose posts a synthetic onboarding KU and returns the response.
// The KU is tagged with `onboarding-smoke` so admins can filter it out
// of the live commons easily.
func (c *Client) SmokePropose(ctx context.Context, persona string) (*ProposeResponse, error) {
	if persona == "" {
		return nil, fmt.Errorf("l2client: SmokePropose persona required")
	}
	body := ProposeRequest{
		Summary: fmt.Sprintf("join smoke from %s", persona),
		Detail: fmt.Sprintf(
			"Synthetic KU posted by `8l join` smoke probe at %s. Safe to delete or filter from views.",
			time.Now().UTC().Format(time.RFC3339),
		),
		Action:  "Filter `domains contains onboarding-smoke` to suppress smoke KUs in the L2 view.",
		Domains: []string{"onboarding-smoke"},
		Pattern: "join-smoke",
	}
	var out ProposeResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/propose", body, &out); err != nil {
		return nil, err
	}
	if out.Tier == "" {
		return nil, fmt.Errorf("l2client: propose response missing tier field")
	}
	return &out, nil
}
