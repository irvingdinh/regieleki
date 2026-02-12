#!/usr/bin/env bash

# Regieleki DNS Server - Automated Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/irvingdinh/regieleki/main/install.sh | sudo bash
#
# This script will:
#   1. Download the latest regieleki binary
#   2. Install it to /usr/local/bin
#   3. Create /var/lib/regieleki for data
#   4. Install and start the systemd service
#   5. Disable systemd-resolved if it conflicts on port 53

{

set -euo pipefail

REPO="irvingdinh/regieleki"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/var/lib/regieleki"
SERVICE_FILE="/etc/systemd/system/regieleki.service"
BINARY="regieleki"

# -------------------------------------------------------------------
# Helpers
# -------------------------------------------------------------------

info()  { printf "\033[1;34m[info]\033[0m  %s\n" "$*"; }
ok()    { printf "\033[1;32m[ok]\033[0m    %s\n" "$*"; }
warn()  { printf "\033[1;33m[warn]\033[0m  %s\n" "$*"; }
die()   { printf "\033[1;31m[error]\033[0m %s\n" "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "'$1' is required but not installed."
}

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    die "This script must be run as root (use sudo)."
  fi
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    *) die "Unsupported architecture: $arch. Only x86_64/amd64 is supported." ;;
  esac
}

detect_os() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux) echo "linux" ;;
    *) die "Unsupported OS: $os. Only Linux is supported." ;;
  esac
}

get_latest_version() {
  local url="https://api.github.com/repos/${REPO}/releases/latest"
  local version
  version="$(curl -fsSL "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')" || true
  if [ -z "$version" ]; then
    die "Failed to fetch latest release from GitHub. Check https://github.com/${REPO}/releases"
  fi
  echo "$version"
}

# -------------------------------------------------------------------
# Installation steps
# -------------------------------------------------------------------

install_binary() {
  local version="$1"
  local bin_url="https://github.com/${REPO}/releases/download/${version}/${BINARY}"
  local checksum_url="https://github.com/${REPO}/releases/download/${version}/${BINARY}.sha256"
  TMPDIR_CLEANUP="$(mktemp -d)" || die "Failed to create temporary directory"
  trap 'rm -rf "$TMPDIR_CLEANUP"' EXIT

  info "Downloading regieleki ${version}..."
  curl -fsSL -o "${TMPDIR_CLEANUP}/${BINARY}" "$bin_url" || die "Download failed. Check https://github.com/${REPO}/releases"

  info "Verifying checksum..."
  curl -fsSL -o "${TMPDIR_CLEANUP}/${BINARY}.sha256" "$checksum_url" || die "Checksum download failed. Check https://github.com/${REPO}/releases"
  (cd "$TMPDIR_CLEANUP" && sha256sum -c "${BINARY}.sha256") || die "Checksum verification failed. The downloaded binary may be corrupted or tampered with."

  chmod 755 "${TMPDIR_CLEANUP}/${BINARY}"

  # Quick sanity check
  if ! file "${TMPDIR_CLEANUP}/${BINARY}" | grep -q "ELF.*64-bit"; then
    die "Downloaded file is not a valid Linux binary."
  fi

  mv "${TMPDIR_CLEANUP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  ok "Installed ${INSTALL_DIR}/${BINARY} (${version})"
}

create_data_dir() {
  if [ ! -d "$DATA_DIR" ]; then
    mkdir -p "$DATA_DIR"
    ok "Created ${DATA_DIR}"
  fi
}

handle_port53_conflict() {
  # Known DNS services that commonly occupy port 53
  local svc
  for svc in systemd-resolved dnsmasq named bind9 unbound; do
    if systemctl is-active --quiet "$svc" 2>/dev/null; then
      warn "${svc} is running and conflicts on port 53."
      info "Stopping and disabling ${svc}..."
      systemctl stop "$svc"
      systemctl disable "$svc"
      ok "Disabled ${svc}"
    fi
  done

  # Fix resolv.conf if systemd-resolved was the stub resolver
  if [ -L /etc/resolv.conf ] && readlink /etc/resolv.conf | grep -q "systemd"; then
    rm -f /etc/resolv.conf
    printf "nameserver 8.8.8.8\nnameserver 1.1.1.1\n" > /etc/resolv.conf
    ok "Updated /etc/resolv.conf with public nameservers"
  fi

  # Final check: if something unknown still holds port 53, bail with a clear message
  sleep 1
  if ss -tulnp 'sport = 53' 2>/dev/null | grep -q ":53"; then
    warn "Port 53 is still in use:"
    ss -tulnp 'sport = 53' >&2
    die "Could not free port 53. Stop the process above and re-run the installer."
  fi
}

generate_token() {
  if [ ! -f "${DATA_DIR}/token" ]; then
    "${INSTALL_DIR}/${BINARY}" access-token -token "${DATA_DIR}/token" >/dev/null
    ok "API token generated at ${DATA_DIR}/token"
  fi
}

install_service() {
  cat > "$SERVICE_FILE" <<'EOF'
[Unit]
Description=Regieleki DNS Server
After=network.target

[Service]
Type=simple
DynamicUser=yes
StateDirectory=regieleki
ExecStart=/usr/local/bin/regieleki -dns :53 -http :13860 -data /var/lib/regieleki/records.tsv -token /var/lib/regieleki/token
Restart=always
RestartSec=3
LimitNOFILE=65535
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/regieleki

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable regieleki
  ok "Systemd service installed and enabled"
}

start_service() {
  systemctl restart regieleki

  # Wait briefly and verify
  sleep 1
  if systemctl is-active --quiet regieleki; then
    ok "regieleki is running"
  else
    warn "regieleki may have failed to start. Check: journalctl -u regieleki"
  fi
}

print_summary() {
  local ip
  ip="$(hostname -I 2>/dev/null | awk '{print $1}')" || ip="<server-ip>"

  printf "\n"
  printf "\033[1;32m========================================\033[0m\n"
  printf "\033[1;32m  Regieleki DNS Server is ready!\033[0m\n"
  printf "\033[1;32m========================================\033[0m\n"
  printf "\n"
  printf "  Admin UI:  http://%s:13860\n" "$ip"
  printf "  DNS:       %s:53\n" "$ip"
  printf "\n"
  printf "  Data file: %s/records.tsv\n" "$DATA_DIR"
  printf "  API Token: regieleki access-token -token %s/token\n" "$DATA_DIR"
  printf "  Service:   systemctl {status|stop|restart} regieleki\n"
  printf "  Logs:      journalctl -u regieleki -f\n"
  printf "\n"
  printf "  To use this as your DNS server, set your\n"
  printf "  client's DNS to: %s\n" "$ip"
  printf "\n"
}

# -------------------------------------------------------------------
# Main
# -------------------------------------------------------------------

main() {
  info "Regieleki DNS Server - Installer"
  printf "\n"

  require_root
  require_cmd curl
  require_cmd systemctl

  detect_os   >/dev/null
  detect_arch >/dev/null

  local version
  version="$(get_latest_version)"

  install_binary "$version"
  create_data_dir
  generate_token
  handle_port53_conflict
  install_service
  start_service
  print_summary
}

main "$@"

}
