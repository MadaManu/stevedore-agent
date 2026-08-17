#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-MadaManu/stevedore-agent}"
SERVICE_NAME="${SERVICE_NAME:-stevedore-agent}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"

usage() {
  cat <<'EOF'
Usage: install.sh [--version TAG] [--install-dir DIR] [--repo OWNER/REPO]

Examples:
  sudo ./install.sh
  sudo ./install.sh --version v0.1.0
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  echo "run install.sh as root, for example: curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sudo bash" >&2
  exit 1
fi

if [ "$(uname -s)" != "Linux" ]; then
  echo "install.sh only supports Linux" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

resolve_latest_version() {
  curl -fsIL "https://github.com/${REPO}/releases/latest" \
    | awk 'tolower($1) == "location:" { location=$2 } END { gsub("\r", "", location); sub(".*/tag/", "", location); print location }'
}

if [ "$VERSION" = "latest" ]; then
  VERSION="$(resolve_latest_version)"
fi

if [ -z "$VERSION" ]; then
  echo "failed to resolve release version" >&2
  exit 1
fi

TARGET_BIN="${INSTALL_DIR}/${SERVICE_NAME}"
CURRENT_VERSION=""
if [ -x "$TARGET_BIN" ]; then
  CURRENT_VERSION="$("$TARGET_BIN" version 2>/dev/null | awk 'NR == 1 { print $2 }')"
fi

if [ "$CURRENT_VERSION" = "$VERSION" ]; then
  echo "stevedore-agent ${VERSION} is already installed at ${TARGET_BIN}"
else
  TMP_DIR="$(mktemp -d)"
  cleanup() {
    rm -rf "$TMP_DIR"
  }
  trap cleanup EXIT

  ASSET_NAME="stevedore-agent-linux-${ARCH}"
  BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

  curl -fsSL -o "${TMP_DIR}/${ASSET_NAME}" "${BASE_URL}/${ASSET_NAME}"
  curl -fsSL -o "${TMP_DIR}/checksums.txt" "${BASE_URL}/checksums.txt"

  (cd "$TMP_DIR" && sha256sum -c checksums.txt --ignore-missing --quiet)
  install -d -m 0755 "$INSTALL_DIR"
  install -m 0755 "${TMP_DIR}/${ASSET_NAME}" "$TARGET_BIN"
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl stop "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
fi

"$TARGET_BIN" install-service
echo "installed stevedore-agent ${VERSION} to ${TARGET_BIN}"
