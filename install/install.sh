#!/usr/bin/env bash
set -euo pipefail

REPO="jholhewres/anchored_oss"
INSTALL_DIR="$HOME/.anchored-oss"
BIN_DIR="$INSTALL_DIR/bin"
CONFIG_DIR="$INSTALL_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { echo -e "${CYAN}$1${RESET}"; }
ok()    { echo -e "${GREEN}$1${RESET}"; }
warn()  { echo -e "${YELLOW}$1${RESET}"; }
err()   { echo -e "${RED}$1${RESET}" >&2; }

TTY="/dev/tty"

prompt_choice() {
    local msg="$1"
    shift
    local options=("$@")
    echo -e "${BOLD}${msg}${RESET}" > "$TTY"
    for i in "${!options[@]}"; do
        echo -e "  $((i+1)). ${options[$i]}" > "$TTY"
    done
    while true; do
        echo -ne "${BOLD}Enter choice [1-${#options[@]}]:${RESET} " > "$TTY"
        read -r answer < "$TTY"
        answer="${answer:-1}"
        if [[ "$answer" =~ ^[0-9]+$ ]] && [ "$answer" -ge 1 ] && [ "$answer" -le "${#options[@]}" ]; then
            echo "$answer"
            return 0
        fi
    done
}

ARCH=$(uname -m)
case "$(uname -s)" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *)      err "Unsupported OS: $(uname -s)"; exit 1 ;;
esac

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)              err "Unsupported arch: $ARCH"; exit 1 ;;
esac

info "=== Anchored OSS Installer ==="
echo ""

VARIANT_CHOICE=$(prompt_choice "Select server variant:" "Self-hosted (single-tenant, default)" "Cloud (multi-tenant with registration)")
case "$VARIANT_CHOICE" in
    1) VARIANT="selfhosted" ;;
    2) VARIANT="cloud" ;;
    *) VARIANT="selfhosted" ;;
esac

LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"v\(.*\)".*/\1/')
if [ -z "$LATEST" ]; then
    err "Failed to determine latest version"; exit 1
fi

BINARY="anchored-oss-${VARIANT}-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/v${LATEST}/${BINARY}"

mkdir -p "$BIN_DIR" "$CONFIG_DIR"

if [ -x "$BIN_DIR/anchored-oss" ]; then
    CURRENT=$("$BIN_DIR/anchored-oss" --version 2>/dev/null || echo "unknown")
    if [ "$CURRENT" = "$LATEST" ]; then
        ok "anchored-oss v${LATEST} (${VARIANT}) is already up to date."
        exit 0
    fi
    info "Updating anchored-oss ${CURRENT} → v${LATEST} (${VARIANT})..."
fi

info "Downloading anchored-oss v${LATEST} (${VARIANT}, ${OS}/${ARCH})..."
curl -fsSL "$URL" -o "$BIN_DIR/anchored-oss" || {
    err "Download failed."; exit 1
}
chmod +x "$BIN_DIR/anchored-oss"

if ! echo "$PATH" | grep -q "$BIN_DIR"; then
    for rc in .bashrc .zshrc .profile .bash_profile; do
        rcfile="$HOME/$rc"
        [ -f "$rcfile" ] || continue
        if ! grep -q 'anchored-oss/bin' "$rcfile" 2>/dev/null; then
            echo "" >> "$rcfile"
            echo "# Anchored OSS server" >> "$rcfile"
            echo 'export PATH="$HOME/.anchored-oss/bin:$PATH"' >> "$rcfile"
        fi
    done
fi

ok "anchored-oss v${LATEST} (${VARIANT}) installed to $BIN_DIR/anchored-oss"
echo ""

DB_CHOICE=$(prompt_choice "Select database:" "SQLite (local file, no external dependencies)" "PostgreSQL (recommended for production)" "Docker Compose (Postgres + server, all-in-one)")
case "$DB_CHOICE" in
    1)
        DB_DRIVER="sqlite"
        DB_DSN="$INSTALL_DIR/anchored-oss.db"
        ;;
    2)
        DB_DRIVER="postgres"
        echo -ne "${BOLD}PostgreSQL DSN (e.g. postgres://user:pass@localhost:5432/anchored_oss?sslmode=disable):${RESET} " > "$TTY"
        read -r DB_DSN < "$TTY"
        if [ -z "$DB_DSN" ]; then
            err "DSN is required for PostgreSQL"; exit 1
        fi
        ;;
    3)
        DB_DRIVER="postgres"
        DB_DSN="postgres://anchored:anchored@localhost:5433/anchored_oss?sslmode=disable"
        if command -v docker &>/dev/null; then
            info "Starting Postgres via Docker Compose..."
            docker compose up -d postgres 2>/dev/null || {
                warn "docker compose not available. Start Postgres manually."
            }
        else
            warn "Docker not found. Install Docker or configure Postgres manually."
        fi
        ;;
esac

cat > "$CONFIG_DIR/config.yaml" <<EOF
server:
  address: ":8080"
  read_timeout: 30s
  write_timeout: 300s

database:
  driver: "${DB_DRIVER}"
  dsn: "${DB_DSN}"

cors:
  allowed_origins: []

mode:
  type: "${VARIANT}"

quota:
  max_storage_bytes: 0
EOF

ok "Config written to $CONFIG_DIR/config.yaml"
echo ""

RUNTIME_CHOICE=$(prompt_choice "Select runtime:" "Manual (run the binary directly)" "PM2 (recommended, auto-restart)" "Docker (containerized)")
case "$RUNTIME_CHOICE" in
    1)
        ok "Run manually with:"
        echo "  $BIN_DIR/anchored-oss -bootstrap    # first time: create admin + API key"
        echo "  $BIN_DIR/anchored-oss               # start server"
        ;;
    2)
        if command -v pm2 &>/dev/null; then
            "$BIN_DIR/anchored-oss" -bootstrap 2>/dev/null || true
            pm2 start "$BIN_DIR/anchored-oss" --name anchored-oss -- -config "$CONFIG_DIR/config.yaml"
            pm2 save
            ok "Server started with PM2"
            echo "  pm2 logs anchored-oss    # view logs"
            echo "  pm2 stop anchored-oss    # stop"
            echo "  pm2 restart anchored-oss # restart"
        else
            warn "PM2 not found. Install with: npm install -g pm2"
            echo "  Then run: pm2 start $BIN_DIR/anchored-oss --name anchored-oss -- -config $CONFIG_DIR/config.yaml"
        fi
        ;;
    3)
        if command -v docker &>/dev/null; then
            docker compose up -d 2>/dev/null || {
                warn "docker compose failed. Ensure docker-compose.yml is in the project directory."
            }
            ok "Server started with Docker Compose"
        else
            warn "Docker not found. Install Docker first."
        fi
        ;;
esac

echo ""
info "Open a new terminal (or run: source ~/.bashrc)"
echo ""
info "Dashboard: http://localhost:8080"
