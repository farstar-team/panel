#!/usr/bin/env bash
set -Eeuo pipefail

REPO="farstar-team/panel"
APP_NAME="farstar"
DATA_DIR="/etc/farstar"
BIN_PATH="/usr/local/bin/farstar"
SERVICE_FILE="/etc/systemd/system/farstar.service"
DEFAULT_PANEL_PORT="8088"

info() { printf '\033[1;34m[Farstar]\033[0m %s\n' "$1"; }
success() { printf '\033[1;32m[Success]\033[0m %s\n' "$1"; }
warn() { printf '\033[1;33m[Warning]\033[0m %s\n' "$1"; }
fail() { printf '\033[1;31m[Error]\033[0m %s\n' "$1" >&2; exit 1; }

cleanup() {
  [[ -z "${TMP_DIR:-}" ]] || rm -rf -- "$TMP_DIR"
}
trap cleanup EXIT

[[ "$(id -u)" -eq 0 ]] || fail "Installer must run as root."
command -v systemctl >/dev/null || fail "systemd is required."

install_packages() {
  if command -v apt-get >/dev/null; then
    apt-get update -y
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl
  elif command -v dnf >/dev/null; then
    dnf install -y ca-certificates curl
  elif command -v yum >/dev/null; then
    yum install -y ca-certificates curl
  else
    fail "Supported package manager not found (apt, dnf, or yum)."
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    aarch64|arm64) printf 'arm64' ;;
    *) fail "Unsupported CPU architecture: $(uname -m)" ;;
  esac
}

download_release_binary() {
  local arch="$1"
  local tag asset checksums base expected actual
  tag="${FARSTAR_VERSION:-}"
  if [[ -z "$tag" ]]; then
    tag="$(curl --fail --silent --show-error \
      -H 'Accept: application/vnd.github+json' \
      "https://api.github.com/repos/${REPO}/releases/latest" |
      sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 || true)"
  fi
  [[ -n "$tag" ]] || return 1
  asset="farstar-linux-${arch}"
  checksums="SHA256SUMS"
  base="https://github.com/${REPO}/releases/download/${tag}"
  info "Downloading Farstar ${tag} for linux/${arch}..."
  curl --fail --location --retry 3 "${base}/${asset}" -o "$TMP_DIR/farstar" || return 1
  curl --fail --location --retry 3 "${base}/${checksums}" -o "$TMP_DIR/SHA256SUMS" || return 1
  expected="$(
    tr -d '\r' < "$TMP_DIR/SHA256SUMS" |
      awk -v file="$asset" '$2 == file {print $1; exit}'
  )"
  [[ "$expected" =~ ^[a-fA-F0-9]{64}$ ]] || fail "Release checksum is missing or invalid."
  actual="$(sha256sum "$TMP_DIR/farstar" | awk '{print $1}')"
  [[ "$actual" == "$expected" ]] || fail "Farstar binary checksum verification failed."
  chmod 0755 "$TMP_DIR/farstar"
  return 0
}

validate_port() {
  [[ "$1" =~ ^[0-9]+$ ]] && (( "$1" >= 1 && "$1" <= 65535 ))
}

prompt_configuration() {
  if [[ -n "${FARSTAR_PANEL_PORT:-}" ]]; then
    PANEL_PORT="$FARSTAR_PANEL_PORT"
  else
    read -r -p "Panel port [${DEFAULT_PANEL_PORT}]: " PANEL_PORT
    PANEL_PORT="${PANEL_PORT:-$DEFAULT_PANEL_PORT}"
  fi
  validate_port "$PANEL_PORT" || fail "Panel port must be between 1 and 65535."

  PANEL_BIND="${FARSTAR_PANEL_BIND:-}"
  if [[ -z "$PANEL_BIND" ]]; then
    read -r -p "Panel bind address [127.0.0.1]: " PANEL_BIND
    PANEL_BIND="${PANEL_BIND:-127.0.0.1}"
  fi
  [[ "$PANEL_BIND" == "127.0.0.1" || "$PANEL_BIND" == "0.0.0.0" ]] ||
    fail "Panel bind must be 127.0.0.1 or 0.0.0.0."
  if [[ "$PANEL_BIND" != "127.0.0.1" ]]; then
    warn "The panel will be reachable over the network. Configure HTTPS immediately."
  fi

  ADMIN_USER="${FARSTAR_ADMIN_USER:-}"
  if [[ -z "$ADMIN_USER" ]]; then
    read -r -p "Admin username [admin]: " ADMIN_USER
    ADMIN_USER="${ADMIN_USER:-admin}"
  fi

  ADMIN_PASS="${FARSTAR_ADMIN_PASSWORD:-}"
  while [[ ${#ADMIN_PASS} -lt 10 ]]; do
    read -r -s -p "Strong admin password (minimum 10 characters): " ADMIN_PASS
    printf '\n'
    [[ ${#ADMIN_PASS} -ge 10 ]] || warn "Password is too short."
  done
}

write_service() {
  cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Farstar Tunnel Panel
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=farstar
Group=farstar
Environment=FARSTAR_DATA_DIR=${DATA_DIR}
Environment=FARSTAR_LISTEN=${PANEL_BIND}:${PANEL_PORT}
ExecStart=${BIN_PATH} serve
Restart=on-failure
RestartSec=3
TimeoutStopSec=15
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR}
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
LockPersonality=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

[Install]
WantedBy=multi-user.target
EOF
}

main() {
  local arch existing_install=false
  arch="$(detect_arch)"
  TMP_DIR="$(mktemp -d)"
  install_packages

  if [[ -f "$DATA_DIR/farstar.db" ]]; then
    existing_install=true
    info "Existing Farstar data detected; configuration will be preserved."
  fi

  download_release_binary "$arch" ||
    fail "No installable GitHub Release was found. Publish a release containing farstar-linux-amd64, farstar-linux-arm64, and SHA256SUMS."
  install -m 0755 "$TMP_DIR/farstar" "$BIN_PATH"

  if ! getent group farstar >/dev/null; then groupadd --system farstar; fi
  if ! id farstar >/dev/null 2>&1; then
    useradd --system --gid farstar --home-dir "$DATA_DIR" --shell /usr/sbin/nologin farstar
  fi
  install -d -m 0750 -o farstar -g farstar "$DATA_DIR" "$DATA_DIR/logs"

  if $existing_install; then
    PANEL_PORT="${FARSTAR_PANEL_PORT:-$DEFAULT_PANEL_PORT}"
    PANEL_BIND="${FARSTAR_PANEL_BIND:-127.0.0.1}"
    if [[ -f "$SERVICE_FILE" ]]; then
      PANEL_PORT="$(sed -n 's/.*FARSTAR_LISTEN=.*:\([0-9][0-9]*\)$/\1/p' "$SERVICE_FILE" | head -n1)"
      PANEL_PORT="${PANEL_PORT:-$DEFAULT_PANEL_PORT}"
      PANEL_BIND="$(sed -n 's/.*FARSTAR_LISTEN=\(.*\):[0-9][0-9]*$/\1/p' "$SERVICE_FILE" | head -n1)"
      PANEL_BIND="${PANEL_BIND:-127.0.0.1}"
    fi
  else
    prompt_configuration
    printf '%s' "$ADMIN_PASS" |
      FARSTAR_DATA_DIR="$DATA_DIR" "$BIN_PATH" setup --username "$ADMIN_USER" --password-stdin
    unset ADMIN_PASS FARSTAR_ADMIN_PASSWORD
  fi

  chown -R farstar:farstar "$DATA_DIR"
  chmod 0750 "$DATA_DIR" "$DATA_DIR/logs"
  [[ ! -f "$DATA_DIR/master.key" ]] || chmod 0600 "$DATA_DIR/master.key"
  write_service

  systemctl daemon-reload
  systemctl enable --now farstar.service
  sleep 2
  systemctl is-active --quiet farstar.service || {
    journalctl -u farstar.service -n 80 --no-pager
    fail "Farstar service failed to start."
  }

  success "Farstar Tunnel Panel is installed and running."
  if [[ "$PANEL_BIND" == "127.0.0.1" ]]; then
    printf 'Local panel: http://127.0.0.1:%s\n' "$PANEL_PORT"
    printf 'Secure access: ssh -L %s:127.0.0.1:%s root@SERVER_IP\n' "$PANEL_PORT" "$PANEL_PORT"
  else
    printf 'Panel: http://SERVER_IP:%s\n' "$PANEL_PORT"
  fi
  printf 'Service logs: journalctl -u farstar -f\n'
}

main "$@"
