#!/usr/bin/env bash
set -euo pipefail

REPO="jholhewres/anchored"
BINARY_NAME="anchored"
INSTALL_DIR="${ANCHORED_INSTALL_DIR:-$HOME/.anchored}"
BIN_DIR="$INSTALL_DIR/bin"
INSTALL_PATH="$BIN_DIR/$BINARY_NAME"

VERSION=""

usage() {
  cat <<'EOF'
Anchored Installer

Usage:
  anchored.sh [options]

Options:
  --version VERSION      Install a specific version (default: latest)
  --help                 Show this help message

Examples:
  curl -fsSL https://raw.githubusercontent.com/jholhewres/anchored_oss/main/install/anchored.sh | sh
  ./install/anchored.sh --version v0.4.10
EOF
  exit 0
}

info()  { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
ok()    { printf "\033[1;32m==>\033[0m %s\n" "$*"; }
error() { printf "\033[1;31mError:\033[0m %s\n" "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      [[ -n "$VERSION" ]] || error "--version requires a value"
      shift 2
      ;;
    --help|-h)
      usage
      ;;
    *)
      error "Unknown option: $1"
      ;;
  esac
done

command -v curl >/dev/null 2>&1 || error "curl is required but not installed."
command -v tar >/dev/null 2>&1 || error "tar is required but not installed."

case "$(uname -s)" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)      error "Unsupported operating system: $(uname -s). Only linux and darwin are supported." ;;
esac

case "$(uname -m)" in
  x86_64|amd64)  arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)             error "Unsupported architecture: $(uname -m). Only amd64 and arm64 are supported." ;;
esac

if [[ -z "$VERSION" ]]; then
  info "Fetching latest Anchored release..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/') \
    || error "Could not determine latest version."
fi

version_no_v="${VERSION#v}"
archive="anchored_${version_no_v}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${VERSION}/${archive}"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

info "Downloading Anchored ${VERSION} (${os}/${arch})..."
curl -fsSL "$url" -o "$tmpdir/$archive" || error "Download failed: $url"

tar -xzf "$tmpdir/$archive" -C "$tmpdir"
binary=""
while IFS= read -r candidate; do
  if [[ -x "$candidate" ]]; then
    binary="$candidate"
    break
  fi
done < <(find "$tmpdir" -type f -name "$BINARY_NAME")
[[ -n "$binary" ]] || error "Archive did not contain an executable named $BINARY_NAME."

mkdir -p "$BIN_DIR"
cp "$binary" "$INSTALL_PATH"
chmod 755 "$INSTALL_PATH"

ok "Installed Anchored ${VERSION} to $INSTALL_PATH"
echo ""
echo "Add this to your shell profile if needed:"
echo "  export PATH=\"$BIN_DIR:\$PATH\""
echo ""
echo "Next:"
echo "  anchored --help"
