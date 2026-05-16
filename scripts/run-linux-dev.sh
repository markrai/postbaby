#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
DESKTOP_DIR="${REPO_ROOT}/desktop"

APT_HINT="sudo apt update && sudo apt install -y libglib2.0-dev libgtk-3-dev libwebkit2gtk-4.1-dev libayatana-appindicator3-dev librsvg2-dev libxdo-dev libssl-dev pkg-config build-essential curl wget file"

fail() {
  echo "[run-linux-dev] $1" >&2
  exit 1
}

ensure_linux_system_deps() {
  command -v pkg-config >/dev/null 2>&1 || fail "pkg-config is required for Linux source builds. On Ubuntu/Debian run: ${APT_HINT}"

  for pkg in glib-2.0 gtk+-3.0 webkit2gtk-4.1 ayatana-appindicator3-0.1 librsvg-2.0 openssl; do
    pkg-config --exists "${pkg}" || fail "Missing Linux source-build dependency '${pkg}'. On Ubuntu/Debian run: ${APT_HINT}"
  done
}

if [[ "$(uname -s)" != "Linux" ]]; then
  fail "This helper is intended for Linux only. Use the documented desktop commands directly on Windows or macOS."
fi

[[ -f "${DESKTOP_DIR}/package.json" ]] || fail "Missing ${DESKTOP_DIR}/package.json. Run this from a Postbaby repo checkout."

command -v node >/dev/null 2>&1 || fail "Node.js is required. Install Node.js first."
command -v npm >/dev/null 2>&1 || fail "npm is required. Install Node.js/npm first."
command -v cargo >/dev/null 2>&1 || fail "Rust/Cargo is required. Install Rust first."

ensure_linux_system_deps

if [[ ! -d "${DESKTOP_DIR}/node_modules/@tauri-apps/cli" ]]; then
  echo "[run-linux-dev] Installing desktop npm dependencies..."
  (cd "${DESKTOP_DIR}" && npm install)
fi

cd "${DESKTOP_DIR}"
npm run check
npm run tauri dev
