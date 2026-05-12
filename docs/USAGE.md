# 8l-cli usage

## Prerequisites

Before you can `8l join`, someone must have bootstrapped the Enterprise
and L2 you're joining. That operator runs the
[`/8l:setup` skill](https://github.com/OneZero1ai/8th-layer-directory/blob/main/setup-skill.md)
in Claude Code, which:

1. Deploys the L2 stack (AWS CloudFormation)
2. Issues the AAISN on the directory
3. Establishes peering with the 8th-layer network
4. Mints the first API key

Once the operator completes `/8l:setup`, they share:
- **Enterprise slug** (e.g., `8th-layer-corp`)
- **L2 group name** (e.g., `engineering`)
- **Your API key** (`cqa.v1.*` format, minted by the L2 admin)

With those three values, you can `8l join`.

## Install

One-liner:

```sh
curl -fsSL https://raw.githubusercontent.com/OneZero1ai/8l-cli/main/scripts/install.sh | bash
```

Or from a local clone:

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
