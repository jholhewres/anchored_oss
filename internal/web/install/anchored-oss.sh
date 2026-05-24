#!/usr/bin/env bash
set -euo pipefail

REPO="jholhewres/anchored_oss"
BINARY_NAME="anchored-oss"
INSTALL_DIR="${ANCHORED_OSS_INSTALL_DIR:-$HOME/.anchored-oss}"
BIN_DIR="$INSTALL_DIR/bin"
INSTALL_PATH="$BIN_DIR/$BINARY_NAME"

VERSION=""
VARIANT="selfhosted"

usage() {
  cat <<'EOF'
Anchored OSS Installer

Usage:
  install.sh [options]

Options:
  --version VERSION      Install a specific version (default: latest)
  --variant VARIANT      selfhosted or cloud (default: selfhosted)
  --help                 Show this help message

Examples:
  curl -fsSL https://anchoredoss.dev/install-oss | sh
  curl -fsSL https://anchoredoss.dev/install-oss | sh -s -- --variant cloud
  ./install/install.sh --version v0.1.0
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
    --variant)
      VARIANT="${2:-}"
      [[ "$VARIANT" == "selfhosted" || "$VARIANT" == "cloud" ]] || error "--variant must be selfhosted or cloud"
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
  info "Fetching latest Anchored OSS release..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/') \
    || error "Could not determine latest version."
fi

binary_file="${BINARY_NAME}-${VARIANT}-${os}-${arch}"
url="https://github.com/${REPO}/releases/download/${VERSION}/${binary_file}"

mkdir -p "$BIN_DIR"

info "Downloading Anchored OSS ${VERSION} (${VARIANT}, ${os}/${arch})..."
curl -fsSL "$url" -o "$INSTALL_PATH" || error "Download failed: $url"
chmod 755 "$INSTALL_PATH"

ok "Installed Anchored OSS ${VERSION} (${VARIANT}) to $INSTALL_PATH"
echo ""
echo "Add this to your shell profile if needed:"
echo "  export PATH=\"$BIN_DIR:\$PATH\""
echo ""
echo "Next:"
echo "  anchored-oss -setup"
