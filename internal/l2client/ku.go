package l2client

import "time"

// KnowledgeUnit mirrors the L2 server's KnowledgeUnit shape.
//
// We re-declare it here instead of importing cq SDK types so the CLI
// stays free of the upstream cq Go SDK and its CGO/SQLite dependency
// tree (see internal/l2client/client.go's package doc).
type KnowledgeUnit struct {
	ID           string   `json:"id"`
	Version      int      `json:"version"`
	Domains      []string `json:"domains"`
	Insight      Insight  `json:"insight"`
	Context      Context  `json:"context"`
	Evidence     Evidence `json:"evidence"`
	Tier         string   `json:"tier"`
	CreatedBy    string   `json:"created_by"`
	SupersededBy string   `json:"superseded_by,omitempty"`
	Flags        []Flag   `json:"flags,omitempty"`
}

// Context mirrors cq Context. Pattern is a plain string per the server schema.
type Context struct {
	Languages  []string `json:"languages,omitempty"`
	Frameworks []string `json:"frameworks,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
}

// Evidence mirrors cq Evidence. Confidence is in [0.0, 1.0].
type Evidence struct {
	Confidence    float64    `json:"confidence"`
	Confirmations int        `json:"confirmations"`
	FirstObserved *time.Time `json:"first_observed,omitempty"`
	LastConfirmed *time.Time `json:"last_confirmed,omitempty"`
}

// Flag is a single problem report against a KU.
type Flag struct {
	Reason      string     `json:"reason"`
	Timestamp   *time.Time `json:"timestamp,omitempty"`
	Detail      string     `json:"detail,omitempty"`
	DuplicateOf string     `json:"duplicate_of,omitempty"`
}

// Valid FlagReason values, matching the cq server's FlagReason enum.
const (
	FlagReasonStale     = "stale"
	FlagReasonIncorrect = "incorrect"
	FlagReasonDuplicate = "duplicate"
)

// AllFlagReasons returns the canonical FlagReason list (display order
// matches the upstream cq CLI).
func AllFlagReasons() []string {
	return []string{FlagReasonStale, FlagReasonIncorrect, FlagReasonDuplicate}
}
