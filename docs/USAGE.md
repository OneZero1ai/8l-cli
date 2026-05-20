# 8l-cli usage

## Install

```sh
make build && sudo install -m 0755 ./8l /usr/local/bin/8l
8l --version
```

## Join an L2 group

```sh
export CQ_TEST_API_KEY=cqa.v1.…   # provided by the L2 admin
8l join \
  --enterprise 8th-layer-corp \
  --l2         engineering \
  --persona    alice \
  --api-key    '$CQ_TEST_API_KEY' \
  --non-interactive
```

The CLI:
1. Validates the API key shape (`cqa.v1.<32hex>.<64chars>`)
2. Probes `https://engineering.8th-layer-corp.8th-layer.ai/api/v1/auth/me`
3. Atomically writes `~/.claude-mux/profiles/8l-cq.json`
4. Posts a smoke KU to `/api/v1/propose` and verifies `tier: private`

If smoke fails the CLI rolls back the profile — you never end up in a
half-applied state.

## Status

```sh
8l status                        # default profile name "8l-cq"
8l status --profile demo         # alternate profile
8l status --no-probe             # skip live HTTP checks
```

## Unjoin

```sh
8l unjoin                                # local cleanup only
8l unjoin --revoke --yes                 # also revoke the key on the L2
```

`--revoke` requires `--yes` because it is destructive on the L2 side.

## Doctor

```sh
8l doctor
```

Runs every check, then summarises pass/fail. Useful when `status`
returns a confusing error — doctor surfaces DNS, TLS, auth, tenant
match, OpenAPI reachability, and `cq` binary on PATH separately.

## Rotate key

```sh
8l rotate-key                            # mint new, swap, revoke old
8l rotate-key --revoke-old=false         # keep the old key alive (audit period)
8l rotate-key --label "rotation 2026Q2"  # custom label on the new key
```

The new key is persisted into the profile **before** the old key is
revoked, so a transient L2 outage never locks you out.

## Exit codes

| Code | Meaning |
|---|---|
| 0  | Success |
| 10 | Missing required arg in non-interactive mode |
| 11 | Invalid API key format |
| 12 | DNS resolution failed for derived endpoint |
| 13 | `/auth/me` non-200 (key invalid, expired, wrong tenant) |
| 14 | Smoke `propose` returned `tier: local` (binding not in effect) |
| 15 | Profile conflict; existing differs and `--force` not set |
| 1  | Unexpected error |

## Endpoint override

For customer Enterprises with non-canonical DNS (private endpoints, etc.):

```sh
export CQ_ADDR_OVERRIDE=https://l2.customer.example.com
8l join --enterprise customer --l2 ops --persona alice --api-key '$KEY'
```

The override applies to every subcommand for the duration of that shell.

## Knowledge-unit subcommands (ported from cq, Decision 35)

Once `8l join` has provisioned a profile, the following subcommands
operate against the bound L2. They reuse the stored `cqa.v1.…` API
key and `CQ_ADDR` from `~/.claude-mux/profiles/8l-cq.json` — no need
to re-pass them.

If you run a knowledge-unit subcommand before `8l join`, it exits
with code 10 and points at `8l join`.

### Propose a knowledge unit

```sh
8l propose \
  --summary "rollup S3 buckets need DeleteObject on the role policy" \
  --detail  "Otherwise the lifecycle rule silently fails." \
  --action  "Add s3:DeleteObject to the IAM policy on the role." \
  --domain  aws --domain s3 --domain test-fleet \
  --language terraform \
  --pattern  iam-policy
```

Server stamps `id`, `created_by`, `tier`, and timestamps.

If the L2 is unreachable (DNS / 5xx / TLS error) the propose lands in
the local outbox at `~/.claude-mux/8l-outbox.jsonl` and is replayed by
`8l drain`. Auth failures (401/403) do **not** queue — they exit with
code 13.

### Query

```sh
8l query --domain aws --domain s3
8l query --domain test-fleet --limit 20
8l query --domain react --language typescript --pattern hook
8l query --domain aws --format json | jq '.[].id'
```

### Confirm

```sh
8l confirm ku_abc123def…
```

Boosts the unit's confidence by one confirmation.

### Flag

```sh
8l flag ku_abc123def… --reason stale
8l flag ku_abc123def… --reason incorrect --detail "broke after v2 release"
8l flag ku_abc123def… --reason duplicate --duplicate-of ku_other…
```

Valid reasons: `stale`, `incorrect`, `duplicate`. Duplicate requires
`--duplicate-of`.

### Status (knowledge-unit aggregate counts)

```sh
8l status                # default: binding + smoke probe
8l status --ku-stats     # adds total_units / tiers / domains
8l status --format json  # JSON payload (implies --ku-stats)
```

The default `8l status` continues to print the binding probe (the
existing `--no-probe`/`--verbose` flags still work). `--ku-stats`
extends it with the L2's per-Enterprise aggregate counts that the
old `cq status` printed.

### Drain

```sh
8l drain --dry-run        # count without pushing
8l drain                  # replay the outbox into the L2
8l drain --format json    # JSON {pushed, failed, pending, warnings}
```

Successfully-pushed entries are removed from the outbox. Auth failure
aborts the drain and preserves the queue (exit 13). Non-auth errors
leave the failing entries queued and exit non-zero so cron/CI can
notice.

### Prompt

```sh
8l prompt reflect            # /cq:reflect slash-command body
8l prompt skill              # cq agent skill prompt body
8l prompt skill --format json
```

Pure-local — no L2 contact, no auth needed. Use these when wiring cq
into agent frameworks that don't have the plugin installed.

## Exit codes (extended)

The codes from V1 still apply. New ones used by the cq subcommands:

| Code | Meaning |
|---|---|
| 10 | Missing required arg (also: unsupported `--format`, invalid `--reason`, missing `--duplicate-of`, missing profile) |
| 13 | Auth failed against `/api/v1/*` |
| 1  | Server 5xx / unexpected error / partial drain failure |
