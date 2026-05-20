// cq.go — user-facing cq subcommand transport.
//
// One method per subcommand surface:
//
//   - Propose  → POST /api/v1/propose
//   - Query    → GET  /api/v1/query?domains=…
//   - Confirm  → POST /api/v1/confirm/{unit_id}
//   - FlagUnit → POST /api/v1/flag/{unit_id}
//   - Stats    → GET  /api/v1/stats
//
// Route shapes were taken from cq_server/app.py (the L2's source of
// truth). The earlier task description used /units/{id}/confirm etc. —
// that's not what the server exposes; we follow the server.
package l2client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ProposeParams describes a new knowledge unit to propose. Fields map
// 1:1 to the cq CLI's --summary / --detail / --action / --domain /
// --language / --framework / --pattern flags.
type ProposeParams struct {
	Summary    string
	Detail     string
	Action     string
	Domains    []string
	Languages  []string
	Frameworks []string
	Pattern    string
	// CreatedBy is sent in the request body but the server overwrites
	// it with the authenticated caller's username. Provided here for
	// drain replay symmetry — for an interactive `8l propose` it's
	// usually empty.
	CreatedBy string
}

// Propose posts a new knowledge unit and returns the server-canonical
// row (the server stamps id / tier / created_by / evidence timestamps).
func (c *Client) Propose(ctx context.Context, p ProposeParams) (*KnowledgeUnit, error) {
	body := ProposeRequest{
		Domains: p.Domains,
		Insight: Insight{
			Summary: p.Summary,
			Detail:  p.Detail,
			Action:  p.Action,
		},
		Context: ProposeContext{
			Languages:  p.Languages,
			Frameworks: p.Frameworks,
			Pattern:    p.Pattern,
		},
		CreatedBy: p.CreatedBy,
	}
	var out KnowledgeUnit
	if err := c.do(ctx, http.MethodPost, "/api/v1/propose", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// QueryParams describes a knowledge-unit search.
type QueryParams struct {
	Domains    []string
	Languages  []string
	Frameworks []string
	Pattern    string
	Limit      int
}

// Query searches for matching KUs. The server enforces tenancy scope
// from the authenticated bearer; this client never sends scope hints.
func (c *Client) Query(ctx context.Context, p QueryParams) ([]KnowledgeUnit, error) {
	if len(p.Domains) == 0 {
		return nil, fmt.Errorf("l2client: Query requires at least one domain")
	}
	q := url.Values{}
	for _, d := range p.Domains {
		q.Add("domains", d)
	}
	for _, l := range p.Languages {
		q.Add("languages", l)
	}
	for _, f := range p.Frameworks {
		q.Add("frameworks", f)
	}
	if p.Pattern != "" {
		q.Set("pattern", p.Pattern)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	path := "/api/v1/query?" + q.Encode()
	var out []KnowledgeUnit
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Confirm boosts a KU's confidence by id.
func (c *Client) Confirm(ctx context.Context, unitID string) (*KnowledgeUnit, error) {
	if strings.TrimSpace(unitID) == "" {
		return nil, fmt.Errorf("l2client: Confirm unit_id required")
	}
	var out KnowledgeUnit
	if err := c.do(ctx, http.MethodPost, "/api/v1/confirm/"+url.PathEscape(unitID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FlagRequest is the body for /api/v1/flag/{unit_id}.
//
// Server-side reason is a FlagReason enum (stale / incorrect /
// duplicate). The detail / duplicate_of fields are accepted by the
// cq Python server's request model but ignored by the L2's
// apply_flag — preserved here for forward compatibility with
// scoring.apply_flag once it carries them through (parity with the
// upstream cq Go CLI).
type FlagRequest struct {
	Reason      string `json:"reason"`
	Detail      string `json:"detail,omitempty"`
	DuplicateOf string `json:"duplicate_of,omitempty"`
}

// FlagUnit posts a flag against a KU.
func (c *Client) FlagUnit(ctx context.Context, unitID string, req FlagRequest) (*KnowledgeUnit, error) {
	if strings.TrimSpace(unitID) == "" {
		return nil, fmt.Errorf("l2client: FlagUnit unit_id required")
	}
	if req.Reason == "" {
		return nil, fmt.Errorf("l2client: FlagUnit reason required")
	}
	var out KnowledgeUnit
	if err := c.do(ctx, http.MethodPost, "/api/v1/flag/"+url.PathEscape(unitID), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StatsResponse mirrors the cq server's StatsResponse shape — see
// app.py:102.
type StatsResponse struct {
	TotalUnits int            `json:"total_units"`
	Tiers      map[string]int `json:"tiers"`
	Domains    map[string]int `json:"domains"`
}

// Stats returns the L2's per-Enterprise aggregate counts.
//
// The endpoint requires auth (SEC-HIGH #39 — pre-fix it was
// unauthenticated and returned global counts).
func (c *Client) Stats(ctx context.Context) (*StatsResponse, error) {
	var out StatsResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/stats", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
