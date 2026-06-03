#!/usr/bin/env sh
set -eu

REPO="${ANCHORED_OSS_REPO:-jholhewres/anchored_oss}"
APP_NAME="${ANCHORED_OSS_APP_NAME:-anchored-oss}"
INSTALL_ROOT="${ANCHORED_OSS_HOME:-$HOME/.anchored-oss}"
BIN_DIR="$INSTALL_ROOT/bin"
DATA_DIR="$INSTALL_ROOT/data"
CONFIG_PATH="$INSTALL_ROOT/config.yaml"
ECOSYSTEM_PATH="$INSTALL_ROOT/ecosystem.config.cjs"
RAW_BASE="https://raw.githubusercontent.com/$REPO/main"

log() { printf '%s\n' "==> $*"; }
fail() { printf '%s\n' "ERROR: $*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing required command: $1"
}

download() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --retry-delay 2 "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$out" "$url"
  else
    fail "Install curl or wget first."
  fi
}

detect_platform() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m | tr '[:upper:]' '[:lower:]')"

  case "$os" in
    linux) platform="linux" ;;
    darwin) platform="darwin" ;;
    mingw*|msys*|cygwin*) platform="windows" ;;
    *) fail "Unsupported OS: $os" ;;
  esac

  case "$arch" in
    x86_64|amd64) cpu="amd64" ;;
    arm64|aarch64) cpu="arm64" ;;
    *) fail "Unsupported architecture: $arch" ;;
  esac

  ext=""
  if [ "$platform" = "windows" ]; then
    ext=".exe"
  fi

  ASSET="anchored-oss-selfhosted-$platform-$cpu$ext"
  INSTALL_BIN="$BIN_DIR/anchored-oss$ext"
}

resolve_version() {
  if [ -n "${ANCHORED_OSS_VERSION:-}" ]; then
    VERSION="$ANCHORED_OSS_VERSION"
    return
  fi

  tmp_version="${TMPDIR:-/tmp}/anchored-oss-version.$$"
  if download "$RAW_BASE/VERSION.md" "$tmp_version" >/dev/null 2>&1; then
    VERSION="$(tr -d '[:space:]' < "$tmp_version")"
    rm -f "$tmp_version"
  else
    VERSION="latest"
  fi

  [ -n "$VERSION" ] || VERSION="latest"
}

install_pm2() {
  if command -v pm2 >/dev/null 2>&1; then
    return
  fi

  need_cmd npm
  log "Installing pm2 globally"
  if npm install -g pm2; then
    return
  fi

  if command -v sudo >/dev/null 2>&1; then
    sudo npm install -g pm2
  else
    fail "Could not install pm2. Install it manually with: npm install -g pm2"
  fi
}

write_default_config() {
  cat > "$CONFIG_PATH" <<EOF
server:
  address: ":${ANCHORED_OSS_PORT:-8080}"
database:
  driver: sqlite
  dsn: "$DATA_DIR/anchored-oss.db"
  max_open_conns: 10
  max_idle_conns: 5
  conn_max_lifetime: 5m
cors:
  allowed_origins:
    - "http://localhost:${ANCHORED_OSS_PORT:-8080}"
quota:
  max_storage_bytes: 0
curation:
  worker_enabled: true
  batch_size: 100
  interval: 5s
  near_dup_window: 720h
  near_dup_threshold: 0.85
EOF
}

# configure_database runs the interactive setup wizard when a terminal is
# available (reading from /dev/tty so it works even under `curl | sh`, where
# stdin is the piped script). The wizard writes config.yaml into $INSTALL_ROOT,
# provisions the chosen database, and bootstraps the admin + first API key
# (printed once). Falls back to a non-interactive sqlite default when there is
# no TTY or when ANCHORED_OSS_NONINTERACTIVE=1 is set. Sets SETUP_RAN=1 when the
# wizard handled bootstrap so the caller can skip the manual bootstrap hint.
SETUP_RAN=0
configure_database() {
  if [ -f "$CONFIG_PATH" ]; then
    log "Existing config at $CONFIG_PATH — keeping it (delete it to reconfigure)."
    return
  fi

  if [ "${ANCHORED_OSS_NONINTERACTIVE:-0}" != "1" ] && [ -r /dev/tty ] && [ -w /dev/tty ]; then
    log "Launching interactive setup (database configuration)"
    printf '\n'
    # cd into INSTALL_ROOT so the wizard writes config.yaml exactly where pm2
    # reads it; /dev/tty gives the wizard a real terminal under curl|sh.
    if ( cd "$INSTALL_ROOT" && "$INSTALL_BIN" -setup < /dev/tty ); then
      SETUP_RAN=1
      return
    fi
    log "Interactive setup did not complete; falling back to default config."
  fi

  log "Writing default config (sqlite). Reconfigure later with: $INSTALL_BIN -setup"
  write_default_config
}

write_ecosystem() {
  cat > "$ECOSYSTEM_PATH" <<EOF
module.exports = {
  apps: [
    {
      name: '$APP_NAME',
      cwd: '$INSTALL_ROOT',
      script: '$INSTALL_BIN',
      args: '-config $CONFIG_PATH',
      autorestart: true,
      max_restarts: 10,
      env: {
        NODE_ENV: 'production'
      }
    }
  ]
}
EOF
}

verify_checksum() {
  checksums="$1"
  binary="$2"
  [ -f "$checksums" ] || return 0

  if command -v sha256sum >/dev/null 2>&1; then
    expected="$(grep "  $ASSET$" "$checksums" | awk '{print $1}')"
    actual="$(sha256sum "$binary" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    expected="$(grep "  $ASSET$" "$checksums" | awk '{print $1}')"
    actual="$(shasum -a 256 "$binary" | awk '{print $1}')"
  else
    log "Skipping checksum verification: sha256sum/shasum not found"
    return 0
  fi

  [ -n "$expected" ] || fail "Checksum for $ASSET not found"
  [ "$expected" = "$actual" ] || fail "Checksum verification failed for $ASSET"
}

main() {
  detect_platform
  resolve_version
  install_pm2

  mkdir -p "$BIN_DIR" "$DATA_DIR"

  if [ "$VERSION" = "latest" ]; then
    release_url="https://github.com/$REPO/releases/latest/download"
  else
    release_url="https://github.com/$REPO/releases/download/$VERSION"
  fi

  tmp_bin="${TMPDIR:-/tmp}/$ASSET.$$"
  tmp_checksums="${TMPDIR:-/tmp}/anchored-oss-checksums.$$"

  log "Downloading $ASSET from $REPO@$VERSION"
  download "$release_url/$ASSET" "$tmp_bin"
  download "$release_url/checksums-sha256.txt" "$tmp_checksums" >/dev/null 2>&1 || true
  verify_checksum "$tmp_checksums" "$tmp_bin"

  mv "$tmp_bin" "$INSTALL_BIN"
  chmod +x "$INSTALL_BIN"
  rm -f "$tmp_checksums"

  configure_database
  write_ecosystem

  log "Starting $APP_NAME with pm2"
  pm2 start "$ECOSYSTEM_PATH" --update-env
  pm2 save

  log "Installed Anchored OSS in $INSTALL_ROOT"
  log "Open the dashboard to finish setup — create your organization, admin login, and projects:"
  log "  http://localhost:${ANCHORED_OSS_PORT:-8080}"
}

main "$@"
