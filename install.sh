#!/bin/sh
# install.sh — the official installer served at https://install.8th-layer.ai
#
# Installs the two 8th-Layer.ai CLIs side-by-side under ~/.local/bin
# (or $PREFIX):
#
#   8l       — operator CLI built from this repo (OneZero1ai/8l-cli).
#              Subcommands: join, quick, status, unjoin, doctor, rotate-key.
#   8l-cq    — agent CLI built from OneZero1ai/8th-layer-agent (the `cq`
#              binary, renamed at install time so it doesn't collide
#              with `8l`). Subcommands: prompt, propose, query, mcp,
#              confirm, drain, flag, status.
#
# Usage (default = install both):
#   curl -fsSL https://install.8th-layer.ai | sh
#   curl -fsSL https://install.8th-layer.ai | sh -s -- --operator-only
#   curl -fsSL https://install.8th-layer.ai | sh -s -- --agent-only
#   curl -fsSL https://install.8th-layer.ai | sh -s -- --prefix /usr/local/bin
#   curl -fsSL https://install.8th-layer.ai | sh -s -- --uninstall
#   curl -fsSL https://install.8th-layer.ai | sh -s -- --uninstall --revoke
#
# Local equivalent (against a checkout of OneZero1ai/8l-cli):
#   ./install.sh [flags]
#
# Env overrides:
#   INSTALL_PREFIX        target dir for both binaries (default: ~/.local/bin)
#   INSTALL_VERSION_8L    8l tag, e.g. v0.1.0           (default: pinned below)
#   INSTALL_VERSION_CQ    cq tag, e.g. v0.9.1           (default: pinned below)
#   INSTALL_BASE_URL      origin for tarballs           (default: S3 direct)
#
# Verification: SHA256SUMS is fetched alongside each tarball and the
# matching line is checked with `sha256sum -c`. If a SHA256SUMS file
# is absent (currently true for the cq bucket — tracked as a follow-up
# in 8th-layer-agent/cli/RELEASING.md), the script warns and continues
# instead of failing hard, so the install path doesn't break the day
# a release goes out before its checksums.
#
# This script is source-controlled at OneZero1ai/8l-cli:install.sh and
# uploaded to the install.8th-layer.ai S3 origin by
# ci/buildspecs/install-publish.yml on every push to main. Do NOT edit
# the served copy directly — changes there will be overwritten.

set -eu

# ----- defaults --------------------------------------------------------------

VERSION_8L_DEFAULT="v0.1.0"
VERSION_CQ_DEFAULT="v0.9.1"

INSTALL_PREFIX_DEFAULT="${HOME}/.local/bin"
INSTALL_BASE_URL_DEFAULT="https://8l-cli-releases-124074140789-us-east-1.s3.us-east-1.amazonaws.com"

INSTALL_PREFIX="${INSTALL_PREFIX:-${INSTALL_PREFIX_DEFAULT}}"
INSTALL_VERSION_8L="${INSTALL_VERSION_8L:-${VERSION_8L_DEFAULT}}"
INSTALL_VERSION_CQ="${INSTALL_VERSION_CQ:-${VERSION_CQ_DEFAULT}}"
INSTALL_BASE_URL="${INSTALL_BASE_URL:-${INSTALL_BASE_URL_DEFAULT}}"

MODE="both"        # both | operator | agent | uninstall
REVOKE=0
KEEP_PROFILE=0
NO_VERIFY=0

# ----- helpers ---------------------------------------------------------------

err()  { printf 'install: error: %s\n' "$*" >&2; }
info() { printf 'install: %s\n' "$*"; }
warn() { printf 'install: warning: %s\n' "$*" >&2; }

usage() {
  sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { err "$1 not found on PATH"; exit 1; }
}

# ----- flag parsing ----------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --operator-only)  MODE="operator";  shift ;;
    --agent-only)     MODE="agent";     shift ;;
    --uninstall)      MODE="uninstall"; shift ;;
    --revoke)         REVOKE=1;         shift ;;
    --keep-profile)   KEEP_PROFILE=1;   shift ;;
    --no-verify)      NO_VERIFY=1;      shift ;;
    --prefix)         INSTALL_PREFIX="$2"; shift 2 ;;
    --prefix=*)       INSTALL_PREFIX="${1#*=}"; shift ;;
    --version-8l)     INSTALL_VERSION_8L="$2"; shift 2 ;;
    --version-8l=*)   INSTALL_VERSION_8L="${1#*=}"; shift ;;
    --version-cq)     INSTALL_VERSION_CQ="$2"; shift 2 ;;
    --version-cq=*)   INSTALL_VERSION_CQ="${1#*=}"; shift ;;
    -h|--help)        usage; exit 0 ;;
    *)                err "unknown flag: $1 (try --help)"; exit 64 ;;
  esac
done

case "$MODE" in
  uninstall)
    if [ "$NO_VERIFY" -eq 1 ]; then
      warn "--no-verify is irrelevant for uninstall"
    fi
    ;;
  *)
    if [ "$REVOKE" -eq 1 ] || [ "$KEEP_PROFILE" -eq 1 ]; then
      err "--revoke and --keep-profile only apply to --uninstall"
      exit 64
    fi
    ;;
esac

# ----- platform detection ----------------------------------------------------

case "$(uname -s)" in
  Darwin) OS=Darwin ;;
  Linux)  OS=Linux ;;
  *) err "unsupported OS: $(uname -s)"; exit 1 ;;
esac

case "$(uname -m)" in
  arm64|aarch64) ARCH=arm64 ;;
  x86_64|amd64)  ARCH=x86_64 ;;
  *) err "unsupported arch: $(uname -m)"; exit 1 ;;
esac

# ----- uninstall mode (runs early — no downloads) ----------------------------

if [ "$MODE" = "uninstall" ]; then
  TARGET_8L="${INSTALL_PREFIX}/8l"
  TARGET_CQ="${INSTALL_PREFIX}/8l-cq"

  if [ -x "$TARGET_8L" ]; then
    info "unjoining via $TARGET_8L"
    if [ "$KEEP_PROFILE" -eq 1 ]; then
      info "  --keep-profile set; skipping unjoin"
    elif [ "$REVOKE" -eq 1 ]; then
      "$TARGET_8L" unjoin --revoke --yes 2>/dev/null || \
        warn "unjoin --revoke failed; the L2 key may still be active"
    else
      "$TARGET_8L" unjoin 2>/dev/null || \
        warn "unjoin failed; ~/.claude-mux/profiles/8l-cq.json may need manual cleanup"
    fi
  else
    info "no 8l binary at $TARGET_8L; skipping unjoin"
    if [ "$REVOKE" -eq 1 ]; then
      warn "--revoke requested but no 8l binary present; cannot revoke remotely"
    fi
  fi

  for f in "$TARGET_8L" "$TARGET_CQ"; do
    [ -e "$f" ] && { info "removing $f"; rm -f "$f"; }
  done

  info "uninstall complete"
  exit 0
fi

# ----- install mode preflight ------------------------------------------------

need_cmd curl
need_cmd tar
need_cmd mkdir
if [ "$NO_VERIFY" -eq 0 ]; then
  # `sha256sum` on Linux; on macOS we'd use `shasum -a 256` — both exist
  # in default installs so we just check that one is present.
  if ! command -v sha256sum >/dev/null 2>&1 \
     && ! command -v shasum >/dev/null 2>&1; then
    err "neither sha256sum nor shasum found; rerun with --no-verify to skip"
    exit 1
  fi
fi

mkdir -p "$INSTALL_PREFIX"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# verify_tarball <tarball-path> <sums-path> <basename>
# Returns 0 on match, 1 on mismatch, 2 if sums file unusable (caller decides).
verify_tarball() {
  tarball="$1"; sums="$2"; base="$3"
  if [ ! -s "$sums" ]; then
    return 2
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    ( cd "$(dirname "$tarball")" && grep " ${base}\$" "$sums" | sha256sum -c - ) >/dev/null 2>&1
  else
    expected=$(grep " ${base}\$" "$sums" | awk '{print $1}')
    [ -n "$expected" ] || return 2
    actual=$(shasum -a 256 "$tarball" | awk '{print $1}')
    [ "$expected" = "$actual" ]
  fi
}

# install_one <name> <s3-prefix> <version> <tarball-base> <install-as>
install_one() {
  name="$1"; prefix="$2"; ver="$3"; base="$4"; as="$5"
  url="${INSTALL_BASE_URL}/${prefix}/${ver}/${base}"
  sums_url="${INSTALL_BASE_URL}/${prefix}/${ver}/SHA256SUMS"

  info "→ downloading ${name} ${ver} (${OS}/${ARCH})"
  if ! curl -fsSL "$url" -o "${TMP}/${base}"; then
    err "download failed: ${url}"
    err "  if you're testing a tag that hasn't been published yet, set INSTALL_VERSION_${name##*-} or wait for the release pipeline."
    exit 1
  fi

  if [ "$NO_VERIFY" -eq 1 ]; then
    warn "skipping checksum verify for ${base} (--no-verify)"
  else
    if ! curl -fsSL "$sums_url" -o "${TMP}/SHA256SUMS.${name}" 2>/dev/null; then
      warn "no SHA256SUMS at ${sums_url} — skipping verify for ${base}"
    else
      case "$(verify_tarball "${TMP}/${base}" "${TMP}/SHA256SUMS.${name}" "${base}"; echo $?)" in
        0) info "  checksum ok" ;;
        1) err "checksum mismatch for ${base} — refusing to install"; exit 1 ;;
        2) warn "SHA256SUMS at ${sums_url} did not contain an entry for ${base} — skipping verify" ;;
      esac
    fi
  fi

  info "→ extracting ${base}"
  tar -xzf "${TMP}/${base}" -C "$TMP"

  # The tarball ships the binary at its source name (e.g. 'cq' or '8l').
  # `as` is the on-disk name we want. They match for 8l; for cq they don't.
  src_name=$(echo "$base" | sed 's/_[A-Za-z]*_[A-Za-z0-9_]*\.tar\.gz$//')
  if [ ! -f "${TMP}/${src_name}" ]; then
    err "tarball ${base} did not contain expected binary '${src_name}'"
    ls -la "$TMP" >&2
    exit 1
  fi

  install -m 0755 "${TMP}/${src_name}" "${INSTALL_PREFIX}/${as}"
  info "→ installed ${as} to ${INSTALL_PREFIX}/${as}"
}

# ----- install operator (8l) -------------------------------------------------

if [ "$MODE" = "both" ] || [ "$MODE" = "operator" ]; then
  install_one "8l" "8l" "$INSTALL_VERSION_8L" "8l_${OS}_${ARCH}.tar.gz" "8l"
fi

# ----- install agent (cq → 8l-cq) --------------------------------------------

if [ "$MODE" = "both" ] || [ "$MODE" = "agent" ]; then
  install_one "8l-cq" "cli" "$INSTALL_VERSION_CQ" "cq_${OS}_${ARCH}.tar.gz" "8l-cq"
fi

# ----- footer ---------------------------------------------------------------

case ":$PATH:" in
  *":${INSTALL_PREFIX}:"*) ON_PATH=1 ;;
  *)                       ON_PATH=0 ;;
esac

printf '\n'
if [ "$ON_PATH" -eq 1 ]; then
  case "$MODE" in
    operator) info "✓ done — run: 8l --version" ;;
    agent)    info "✓ done — run: 8l-cq --version" ;;
    both)     info "✓ done — run: 8l --version && 8l-cq --version" ;;
  esac
else
  warn "${INSTALL_PREFIX} is not on PATH."
  warn "  add this to your shell rc:"
  printf '      export PATH="%s:$PATH"\n' "$INSTALL_PREFIX"
fi

case "$MODE" in
  operator|both)
    cat <<EOF

Next — bind your session to an L2 (single curl-friendly command):
    8l join --enterprise X --l2 Y --persona Z --api-key cqa.v1.…

Or, if a teammate has shared a quick-join token:
    8l quick cqq.v1.…
EOF
    ;;
esac
