#!/usr/bin/env bash
set -euo pipefail

REPO="jholhewres/anchored_oss"
BINARY_NAME="anchored-oss"
INSTALL_DIR="/usr/local/bin"
INSTALL_PATH="${INSTALL_DIR}/${BINARY_NAME}"

VERSION=""
NON_INTERACTIVE=false

usage() {
  cat <<'EOF'
Anchored OSS Installer

Usage:
  install.sh [options]

Options:
  --version VERSION      Install a specific version (default: latest)
  --non-interactive      Skip wizard, just install the binary
  --help                 Show this help message

Examples:
  install.sh                        # Install latest
  install.sh --version v0.1.0       # Install specific version
  install.sh --non-interactive      # Install latest without wizard
EOF
  exit 0
}

info()  { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
error() { printf "\033[1;31mError:\033[0m %s\n" "$*" >&2; exit 1; }

cleanup() {
  [[ -n "${TMPDIR:-}" ]] && rm -rf "$TMPDIR"
}
trap cleanup EXIT

# --- Argument parsing ---
for arg in "$@"; do
  case "$arg" in
    --version)
      shift; VERSION="${1:-}"; shift || error "--version requires a value"
      ;;
    --non-interactive)
      NON_INTERACTIVE=true; shift
      ;;
    --help|-h)
      usage
      ;;
    *)
      error "Unknown option: $arg"
      ;;
  esac
done

# --- Prerequisites ---
command -v curl >/dev/null 2>&1 || error "curl is required but not installed."

# --- OS detection ---
os="$(uname -s)"
case "$os" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)      error "Unsupported operating system: ${os}. Only linux and darwin are supported." ;;
esac

# --- Arch detection ---
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64)  arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)             error "Unsupported architecture: ${arch}. Only amd64 and arm64 are supported." ;;
esac

# --- Resolve version ---
if [[ -z "$VERSION" ]]; then
  info "Fetching latest version from GitHub..."
  VERSION_URL="https://api.github.com/repos/${REPO}/releases/latest"
  VERSION_RESPONSE=$(curl -fsSL "$VERSION_URL" 2>/dev/null) \
    || error "Failed to fetch latest version from GitHub. Check your network or specify a version with --version."

  VERSION=$(printf '%s\n' "$VERSION_RESPONSE" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/') \
    || error "Could not parse latest version from GitHub API response."
fi

info "Installing ${BINARY_NAME} ${VERSION} (${os}/${arch})..."

# --- Download ---
BINARY_FILE="${BINARY_NAME}-${os}-${arch}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_FILE}"

TMPDIR=$(mktemp -d)
DOWNLOAD_PATH="${TMPDIR}/${BINARY_FILE}"

info "Downloading from ${DOWNLOAD_URL}..."
HTTP_STATUS=$(curl -fsSL -w '%{http_code}' -o "$DOWNLOAD_PATH" "$DOWNLOAD_URL") || true

if [[ ! -f "$DOWNLOAD_PATH" ]]; then
  error "Download failed (HTTP ${HTTP_STATUS}). Ensure version ${VERSION} exists for ${os}/${arch} at ${DOWNLOAD_URL}"
fi

if [[ ! -s "$DOWNLOAD_PATH" ]]; then
  rm -f "$DOWNLOAD_PATH"
  error "Downloaded file is empty. Ensure version ${VERSION} exists for ${os}/${arch}."
fi

# --- Install ---
if [[ "$(id -u)" -ne 0 ]]; then
  if ! command -v sudo >/dev/null 2>&1; then
    error "Root privileges required to install to ${INSTALL_DIR}. Run with sudo or as root."
  fi
  sudo cp "$DOWNLOAD_PATH" "$INSTALL_PATH"
  sudo chmod 755 "$INSTALL_PATH"
else
  cp "$DOWNLOAD_PATH" "$INSTALL_PATH"
  chmod 755 "$INSTALL_PATH"
fi

# --- Done ---
info "Installed to ${INSTALL_PATH}"
echo ""
echo "Anchored OSS installed successfully!"
echo ""
if [[ "$NON_INTERACTIVE" == false ]]; then
  echo "Run the setup wizard:"
  echo "  ${BINARY_NAME} -setup"
  echo ""
  echo "Or start directly with an existing config:"
  echo "  ${BINARY_NAME}"
else
  echo "Ready to use:"
  echo "  ${BINARY_NAME}"
fi
