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

# Default listen port. Deliberately off the common 80/443/3000/5000/8000/8080
# lane to avoid colliding with whatever else the host already runs. Override
# with ANCHORED_OSS_PORT=...
PORT="${ANCHORED_OSS_PORT:-8771}"

# Minimum Node major version pm2 needs.
NODE_MIN_MAJOR=18

log() { printf '%s\n' "==> $*"; }
warn() { printf '%s\n' "WARN: $*" >&2; }
fail() { printf '%s\n' "ERROR: $*" >&2; exit 1; }

# SUDO is "sudo" when we are not root and sudo exists; empty when already root;
# unset-capable otherwise (callers check is_privileged).
SUDO=""
detect_sudo() {
  if [ "$(id -u)" -eq 0 ]; then
    SUDO=""
    return 0
  fi
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
    return 0
  fi
  return 1
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

node_major() {
  command -v node >/dev/null 2>&1 || return 1
  node -v 2>/dev/null | sed 's/^v//; s/\..*//'
}

# ensure_node guarantees a Node.js >= NODE_MIN_MAJOR (pm2 runs on Node). It is
# idempotent: a recent enough Node short-circuits. Install strategy, in order:
# Homebrew (macOS) -> NodeSource via apt/dnf (Linux, needs root/sudo) -> nvm
# (user-level fallback, no root). Aborts with guidance if all paths fail.
ensure_node() {
  major="$(node_major || echo 0)"
  if [ "${major:-0}" -ge "$NODE_MIN_MAJOR" ] 2>/dev/null; then
    log "Node $(node -v) detected"
    return 0
  fi
  if [ "${major:-0}" -gt 0 ] 2>/dev/null; then
    warn "Node $(node -v) is older than v${NODE_MIN_MAJOR}; installing a current LTS"
  else
    log "Node.js not found; installing LTS"
  fi

  if [ "$(uname -s)" = "Darwin" ] && command -v brew >/dev/null 2>&1; then
    if brew install node; then return 0; fi
  fi

  if detect_sudo; then
    if command -v apt-get >/dev/null 2>&1; then
      log "Installing Node LTS via NodeSource (apt)"
      if download "https://deb.nodesource.com/setup_lts.x" "${TMPDIR:-/tmp}/nodesource.$$" \
        && $SUDO -E sh "${TMPDIR:-/tmp}/nodesource.$$" \
        && $SUDO apt-get install -y nodejs; then
        rm -f "${TMPDIR:-/tmp}/nodesource.$$"
        return 0
      fi
      rm -f "${TMPDIR:-/tmp}/nodesource.$$" 2>/dev/null || true
    elif command -v dnf >/dev/null 2>&1; then
      log "Installing Node LTS via NodeSource (dnf)"
      if download "https://rpm.nodesource.com/setup_lts.x" "${TMPDIR:-/tmp}/nodesource.$$" \
        && $SUDO -E sh "${TMPDIR:-/tmp}/nodesource.$$" \
        && $SUDO dnf install -y nodejs; then
        rm -f "${TMPDIR:-/tmp}/nodesource.$$"
        return 0
      fi
      rm -f "${TMPDIR:-/tmp}/nodesource.$$" 2>/dev/null || true
    fi
  fi

  # User-level fallback: nvm. Works without root but the resulting node/pm2
  # live under $HOME, so boot persistence (pm2 startup) may need manual setup.
  log "Falling back to nvm (user-level Node install)"
  export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
  if [ ! -s "$NVM_DIR/nvm.sh" ]; then
    download "https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh" "${TMPDIR:-/tmp}/nvm-install.$$"
    PROFILE=/dev/null sh "${TMPDIR:-/tmp}/nvm-install.$$" >/dev/null 2>&1 || true
    rm -f "${TMPDIR:-/tmp}/nvm-install.$$"
  fi
  if [ -s "$NVM_DIR/nvm.sh" ]; then
    # shellcheck disable=SC1090
    . "$NVM_DIR/nvm.sh"
    nvm install --lts >/dev/null 2>&1 || true
    nvm use --lts >/dev/null 2>&1 || true
  fi

  major="$(node_major || echo 0)"
  [ "${major:-0}" -ge "$NODE_MIN_MAJOR" ] 2>/dev/null \
    || fail "Could not install Node >= v${NODE_MIN_MAJOR}. Install it manually and re-run."
  log "Node $(node -v) ready"
}

install_pm2() {
  if command -v pm2 >/dev/null 2>&1; then
    log "pm2 $(pm2 -v 2>/dev/null) detected"
    return 0
  fi
  command -v npm >/dev/null 2>&1 || fail "npm not found after Node install."
  log "Installing pm2 globally"
  if npm install -g pm2 >/dev/null 2>&1; then
    return 0
  fi
  if detect_sudo; then
    $SUDO npm install -g pm2 || fail "Could not install pm2. Install it manually: npm install -g pm2"
  else
    fail "Could not install pm2 (no permission for global npm). Install it manually: npm install -g pm2"
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

write_default_config() {
  cat > "$CONFIG_PATH" <<EOF
server:
  address: "0.0.0.0:${PORT}"
database:
  driver: sqlite
  dsn: "$DATA_DIR/anchored-oss.db"
  max_open_conns: 10
  max_idle_conns: 5
  conn_max_lifetime: 5m
cors:
  allowed_origins:
    - "http://localhost:${PORT}"
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
# no TTY or when ANCHORED_OSS_NONINTERACTIVE=1 is set.
SETUP_RAN=0
configure_database() {
  if [ -f "$CONFIG_PATH" ]; then
    log "Existing config at $CONFIG_PATH — keeping it (delete it to reconfigure)."
    return
  fi

  if [ "${ANCHORED_OSS_NONINTERACTIVE:-0}" != "1" ] && [ -r /dev/tty ] && [ -w /dev/tty ]; then
    log "Launching interactive setup (database configuration)"
    printf '\n'
    if ( cd "$INSTALL_ROOT" && "$INSTALL_BIN" -setup < /dev/tty ); then
      SETUP_RAN=1
      return
    fi
    log "Interactive setup did not complete; falling back to default config."
  fi

  log "Writing default config (sqlite, port ${PORT}). Reconfigure later with: $INSTALL_BIN -setup"
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

# enable_boot_persistence wires pm2 to restart the app on reboot. pm2 startup
# needs root to install the systemd/launchd unit; best-effort, with a clear
# manual hint when we can't elevate.
enable_boot_persistence() {
  if [ "$(uname -s)" != "Linux" ] && [ "$(uname -s)" != "Darwin" ]; then
    return 0
  fi
  if detect_sudo; then
    startup_cmd="$(pm2 startup -u "$(id -un)" --hp "$HOME" 2>/dev/null | grep -E '^\s*sudo ' | tail -1 || true)"
    if [ -n "$startup_cmd" ]; then
      log "Enabling pm2 boot persistence"
      sh -c "$startup_cmd" >/dev/null 2>&1 || warn "pm2 startup needs manual setup; run: $startup_cmd"
    else
      pm2 startup >/dev/null 2>&1 || true
    fi
    pm2 save >/dev/null 2>&1 || true
  else
    warn "No root/sudo: skipping boot persistence. To enable it later, run 'pm2 startup' and follow the printed command."
  fi
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
  ensure_node
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

  log "Starting $APP_NAME with pm2 on port ${PORT}"
  pm2 start "$ECOSYSTEM_PATH" --update-env
  pm2 save
  enable_boot_persistence

  log "Installed Anchored OSS in $INSTALL_ROOT (port ${PORT})"
  if [ "$SETUP_RAN" != "1" ]; then
    log "Finish setup in the dashboard — create your organization, admin login, and projects:"
  fi
  log "  http://localhost:${PORT}"
  log "On a cloud VM, open port ${PORT} in the firewall and use the public IP/hostname."
  log "Logs: pm2 logs $APP_NAME   |   Restart: pm2 restart $APP_NAME"
}

main "$@"
