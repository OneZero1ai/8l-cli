#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/OneZero1ai/8l-cli.git"
MIN_GO_VERSION="1.23"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="8l"

die() { echo "error: $*" >&2; exit 1; }

check_go() {
  command -v go >/dev/null 2>&1 || die "Go is not installed. Install Go ${MIN_GO_VERSION}+ from https://go.dev/dl/"
  local ver
  ver=$(go version | grep -oP '\d+\.\d+' | head -1)
  local major minor
  major=$(echo "$ver" | cut -d. -f1)
  minor=$(echo "$ver" | cut -d. -f2)
  local req_major req_minor
  req_major=$(echo "$MIN_GO_VERSION" | cut -d. -f1)
  req_minor=$(echo "$MIN_GO_VERSION" | cut -d. -f2)
  if [ "$major" -lt "$req_major" ] || { [ "$major" -eq "$req_major" ] && [ "$minor" -lt "$req_minor" ]; }; then
    die "Go ${ver} found, but ${MIN_GO_VERSION}+ is required"
  fi
}

check_git() {
  command -v git >/dev/null 2>&1 || die "git is not installed"
}

main() {
  echo "8l-cli installer"
  echo "────────────────"

  check_git
  check_go
  echo "✓ go $(go version | grep -oP '\d+\.\d+(\.\d+)?')"

  local tmpdir
  tmpdir=$(mktemp -d)
  trap 'rm -rf "$tmpdir"' EXIT

  echo "Cloning ${REPO}..."
  git clone --depth 1 "$REPO" "$tmpdir/8l-cli" 2>&1 | tail -1

  echo "Building..."
  cd "$tmpdir/8l-cli"
  make build

  echo "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
  if [ -w "$INSTALL_DIR" ]; then
    install -m 0755 "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
  else
    echo "(requires sudo)"
    sudo install -m 0755 "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
  fi

  echo ""
  echo "✓ $(${INSTALL_DIR}/${BINARY_NAME} --version)"
  echo ""
  echo "Next: 8l join --enterprise <SLUG> --l2 <GROUP> --persona <NAME> --api-key <KEY>"
}

main "$@"
