#!/usr/bin/env bash
# setup.sh — all-in-one client installer / uninstaller for the 8l CLI.
#
# Install mode (default): builds 8l from source, installs it to
# /usr/local/bin/8l, and (unless --build-only) runs `8l join` so the
# operator is bound to their L2 in a single invocation.
#
# Uninstall mode (--uninstall): runs `8l unjoin` (optionally with
# --revoke to kill the L2 key server-side), removes the installed
# binary, and cleans the local build artifact.
#
# Examples:
#   ./setup.sh                              # interactive; prompts for everything
#   ./setup.sh --enterprise acme --l2 eng \
#              --persona alice --api-key cqa.v1.…   # fully non-interactive
#   ./setup.sh --api-key-stdin <<<"$KEY"    # api key via stdin (no shell history)
#   ./setup.sh --build-only                 # build + install, skip join
#   ./setup.sh --uninstall                  # unjoin (local) + remove binary
#   ./setup.sh --uninstall --revoke         # also revoke the key on the L2
#   ./setup.sh --uninstall --keep-profile   # leave ~/.claude-mux profile in place
#
# Env fallbacks (used when flag is omitted):
#   EIGHTL_ENTERPRISE  EIGHTL_L2  EIGHTL_PERSONA  EIGHTL_API_KEY
#
# Requires: Go 1.23+, git, make (install mode only — uninstall needs none).
#
# TODO(hosting): host this at get.8th-layer.ai (S3+CloudFront, same
#   pattern as the marketing and signup sites) so the install one-liner
#   becomes:
#     curl -fsSL https://get.8th-layer.ai/setup.sh | bash -s -- \
#       --enterprise X --l2 Y --persona Z --api-key K
#   Until then this script must be run from a local checkout of 8l-cli.
#
# TODO(plugin): this installs only the 8l CLI. The 8l-cq Claude Code
#   plugin still has to be wired up manually inside Claude Code:
#     /plugin marketplace add https://8thlayer.onezero1.ai/marketplace.json
#     /plugin install 8l-cq
#   and CQ_ADDR / CQ_API_KEY set under "env" in ~/.claude/settings.json.
#   A future iteration can merge that block via jq once the plugin
#   surface stabilises.

set -euo pipefail

PREFIX="${PREFIX:-/usr/local/bin}"
BUILD_ONLY=0
API_KEY_STDIN=0
UNINSTALL=0
REVOKE=0
KEEP_PROFILE=0
USE_SUDO=""

ENTERPRISE="${EIGHTL_ENTERPRISE:-}"
L2="${EIGHTL_L2:-}"
PERSONA="${EIGHTL_PERSONA:-}"
API_KEY="${EIGHTL_API_KEY:-}"

err()  { printf 'setup: error: %s\n' "$*" >&2; }
info() { printf 'setup: %s\n' "$*"; }

usage() {
  sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
  case "$1" in
    --enterprise)     ENTERPRISE="$2"; shift 2 ;;
    --enterprise=*)   ENTERPRISE="${1#*=}"; shift ;;
    --l2)             L2="$2"; shift 2 ;;
    --l2=*)           L2="${1#*=}"; shift ;;
    --persona)        PERSONA="$2"; shift 2 ;;
    --persona=*)      PERSONA="${1#*=}"; shift ;;
    --api-key)        API_KEY="$2"; shift 2 ;;
    --api-key=*)      API_KEY="${1#*=}"; shift ;;
    --api-key-stdin)  API_KEY_STDIN=1; shift ;;
    --build-only)     BUILD_ONLY=1; shift ;;
    --uninstall)      UNINSTALL=1; shift ;;
    --revoke)         REVOKE=1; shift ;;
    --keep-profile)   KEEP_PROFILE=1; shift ;;
    --prefix)         PREFIX="$2"; shift 2 ;;
    --prefix=*)       PREFIX="${1#*=}"; shift ;;
    -h|--help)        usage; exit 0 ;;
    *)                err "unknown flag: $1"; exit 64 ;;
  esac
done

# ---- sudo selection (used by both install and uninstall) ---------------------
need_sudo_for() {
  local target="$1"
  if [ "$(id -u)" -eq 0 ]; then return 1; fi
  if [ -w "$(dirname "$target")" ] && { [ ! -e "$target" ] || [ -w "$target" ]; }; then
    return 1
  fi
  return 0
}

# ---- uninstall mode ----------------------------------------------------------
# Runs early — no Go/make required.
if [ "$UNINSTALL" -eq 1 ]; then
  if [ "$BUILD_ONLY" -eq 1 ] || [ -n "$ENTERPRISE$L2$PERSONA$API_KEY" ] \
     || [ "$API_KEY_STDIN" -eq 1 ]; then
    err "--uninstall is incompatible with install/join flags"
    exit 64
  fi

  TARGET="$PREFIX/8l"

  # 1) unjoin (local profile cleanup + optional server-side revoke).
  if [ -x "$TARGET" ]; then
    info "unjoining via $TARGET"
    if [ "$KEEP_PROFILE" -eq 1 ]; then
      info "  --keep-profile set; skipping unjoin"
    elif [ "$REVOKE" -eq 1 ]; then
      "$TARGET" unjoin --revoke --yes || \
        err "unjoin --revoke failed; profile/key may still exist on the L2"
    else
      "$TARGET" unjoin || \
        err "unjoin failed; you may need to remove ~/.claude-mux/profiles/8l-cq.json by hand"
    fi
  else
    info "no 8l binary at $TARGET; skipping unjoin"
    if [ "$REVOKE" -eq 1 ]; then
      err "--revoke requested but no binary present; cannot revoke key remotely"
    fi
    if [ "$KEEP_PROFILE" -eq 0 ] && [ -f "$HOME/.claude-mux/profiles/8l-cq.json" ]; then
      info "removing orphan profile ~/.claude-mux/profiles/8l-cq.json"
      rm -f "$HOME/.claude-mux/profiles/8l-cq.json"
    fi
  fi

  # 2) remove the installed binary.
  if [ -e "$TARGET" ]; then
    info "removing $TARGET"
    if need_sudo_for "$TARGET"; then
      command -v sudo >/dev/null 2>&1 || { err "sudo required to remove $TARGET"; exit 1; }
      sudo rm -f "$TARGET"
    else
      rm -f "$TARGET"
    fi
  fi

  # 3) clean local build artifact.
  SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
  [ -f "$SCRIPT_DIR/8l" ] && rm -f "$SCRIPT_DIR/8l"

  cat >&2 <<EOF

setup: uninstall complete.

Reminder — the 8l-cq Claude Code plugin (if you installed it) is NOT
removed by this script. To clean it up:

  /plugin uninstall 8l-cq
  /plugin marketplace remove 8th-layer

and delete the CQ_ADDR / CQ_API_KEY entries from ~/.claude/settings.json.
EOF
  exit 0
fi

# ---- preflight (install mode) ------------------------------------------------
for cmd in go git make; do
  command -v "$cmd" >/dev/null 2>&1 || { err "$cmd not found on PATH"; exit 1; }
done

GO_VER=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
GO_MAJOR=${GO_VER%%.*}
GO_MINOR=$(printf '%s' "$GO_VER" | awk -F. '{print $2}')
if [ "${GO_MAJOR:-0}" -lt 1 ] \
   || { [ "${GO_MAJOR:-0}" -eq 1 ] && [ "${GO_MINOR:-0}" -lt 23 ]; }; then
  err "Go 1.23+ required, found $GO_VER"
  exit 1
fi

if need_sudo_for "$PREFIX/8l"; then
  if command -v sudo >/dev/null 2>&1; then
    USE_SUDO="sudo"
  else
    err "$PREFIX not writable and sudo unavailable; rerun as root or pass --prefix"
    exit 1
  fi
fi

# ---- build & install ---------------------------------------------------------
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
cd "$SCRIPT_DIR"

info "building 8l ($(git describe --tags --always --dirty 2>/dev/null || echo dev))"
make build

info "installing to $PREFIX/8l"
$USE_SUDO install -m 0755 ./8l "$PREFIX/8l"
info "installed: $("$PREFIX/8l" --version 2>/dev/null || echo unknown)"

if [ "$BUILD_ONLY" -eq 1 ]; then
  info "build-only; skipping join."
  exit 0
fi

# ---- collect join args -------------------------------------------------------
# Flags > env > interactive prompt. If stdin isn't a tty and a value is
# still missing, fail with exit 10 (matches `8l join`'s missing-arg code).
prompt_or_die() {
  local label="$1" current="$2"
  if [ -n "$current" ]; then printf '%s' "$current"; return 0; fi
  if [ ! -t 0 ]; then err "missing --$label (no tty for prompt)"; exit 10; fi
  printf '%s: ' "$label" >&2
  local reply
  IFS= read -r reply
  printf '%s' "$reply"
}

ENTERPRISE=$(prompt_or_die enterprise "$ENTERPRISE")
L2=$(prompt_or_die l2 "$L2")
PERSONA=$(prompt_or_die persona "$PERSONA")

if [ "$API_KEY_STDIN" -eq 1 ]; then
  IFS= read -r API_KEY
fi
if [ -z "$API_KEY" ]; then
  if [ ! -t 0 ]; then
    err "missing --api-key (and no --api-key-stdin / EIGHTL_API_KEY)"
    exit 10
  fi
  printf 'api-key (input hidden): ' >&2
  IFS= read -rs API_KEY
  printf '\n' >&2
fi

# ---- join --------------------------------------------------------------------
info "binding to $L2.$ENTERPRISE as $PERSONA"
"$PREFIX/8l" join \
  --enterprise "$ENTERPRISE" \
  --l2         "$L2" \
  --persona    "$PERSONA" \
  --api-key    "$API_KEY" \
  --non-interactive

cat >&2 <<EOF

setup: done. 8l is installed at $PREFIX/8l and bound to $L2.$ENTERPRISE.

Next — install the Claude Code plugin manually (TODO: setup.sh should
do this for you in a future iteration):

  /plugin marketplace add https://8thlayer.onezero1.ai/marketplace.json
  /plugin install 8l-cq

then add to ~/.claude/settings.json:

  { "env": { "CQ_ADDR":    "https://${L2}.${ENTERPRISE}.8th-layer.ai/cq",
             "CQ_API_KEY": "<same key you just used>" } }
EOF
