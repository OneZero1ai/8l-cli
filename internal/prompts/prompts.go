// Package prompts provides the canonical cq agent prompts as
// compiled-in strings for `8l prompt reflect` and `8l prompt skill`.
//
// The body text is copied from
// 8th-layer-agent/sdk/go/prompts/{reflect.md, SKILL.md}; any future
// drift gets resynced by `make sync-prompts` (see Makefile target).
//
// The `8l prompt` subcommand is pure-local — it never contacts the L2
// — so we keep this package free of HTTP / auth concerns.
package prompts

import _ "embed"

//go:embed reflect.md
var reflectBody string

//go:embed SKILL.md
var skillBody string

// Reflect returns the /cq:reflect slash-command prompt body.
func Reflect() string { return reflectBody }

// Skill returns the cq agent skill prompt body.
func Skill() string { return skillBody }
