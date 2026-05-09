# 8l-cli

Management CLI for [8th-Layer.ai](https://8th-layer.ai). Binds Claude Code (and similar MCP-aware) sessions to an L2 group inside an Enterprise tenant.

V1 subcommands:

| Command | Purpose |
|---|---|
| `8l join` | Bind this session — write a claude-mux profile + smoke-test the binding |
| `8l status` | Print the current binding + probe `/auth/me` and `propose` |
| `8l unjoin` | Remove the local binding (optionally revoke the L2 key) |
| `8l doctor` | Diagnose binding issues (DNS, TLS, auth, MCP server) |
| `8l rotate-key` | Mint a new L2 API key and update the profile in place |

See [`docs/USAGE.md`](docs/USAGE.md) for end-to-end examples.

## Build

Requires Go 1.23+.

```sh
make build       # → ./8l
make test        # unit + integration tests with -race
make release-build
```

The build is hermetic — no GitHub Actions, no external CI. Until the
CodeBuild migration in [#168] lands, releases are produced locally and
uploaded as GitHub Release assets.

## Design

Architecture and contract are documented in
[`docs/decisions/29-join-cli-design.md`](https://github.com/OneZero1ai/8th-layer-agent/blob/main/docs/decisions/29-join-cli-design.md)
on the `8th-layer-agent` repo.

Operator-locked decisions resolved during dispatch:

1. **No `--mint-key`** — keys must be pre-minted by the L2 admin. Audit-trail-positive; no path for a leaked persona to elevate via self-service mint.
2. **Explicit profile schema versioning** — the JSON has a `version` field; the CLI reads N-1 with a deprecation warning, refuses N-2.
3. **Dedicated repo** — `OneZero1ai/8l-cli`, independent release cadence from `8th-layer-agent`.

## License

[Apache-2.0](LICENSE).
