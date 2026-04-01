#!/usr/bin/env bash
# PookiePaws Installer for macOS and Linux
# Usage: curl -fsSL https://raw.githubusercontent.com/MITPOAI/PookiePaws/main/install.sh | bash

set -euo pipefail

OWNER="MITPOAI"
REPO="PookiePaws"
INSTALL_DIR="/usr/local/bin"
BIN="pookie"

BOLD="\033[1m"
GREEN="\033[1;32m"
CYAN="\033[1;36m"
MAGENTA="\033[1;35m"
RED="\033[1;31m"
DIM="\033[2m"
RESET="\033[0m"

step()  { echo -e "  ${CYAN}-> $1${RESET}"; }
ok()    { echo -e "  ${GREEN}OK${RESET} $1"; }
fail()  { echo -e "  ${RED}ERR $1${RESET}"; exit 1; }

echo ""
echo -e "  ${MAGENTA}${BOLD}PookiePaws Installer${RESET}"
echo -e "  ${DIM}Local-first marketing ops runtime${RESET}"
echo ""

# Detect OS
step "Detecting operating system..."
case "$(uname -s)" in
  Darwin) OS="darwin" ;;
  Linux)  OS="linux"  ;;
  *)      fail "Unsupported OS: $(uname -s). Only macOS and Linux are supported." ;;
esac

# Detect architecture
case "$(uname -m)" in
  x86_64 | amd64)         ARCH="amd64" ;;
  arm64  | aarch64)       ARCH="arm64" ;;
  *)      fail "Unsupported architecture: $(uname -m). Only amd64 and arm64 are supported." ;;
esac
ok "${OS}/${ARCH}"

# Check for required tools
for cmd in curl tar; do
  command -v "$cmd" >/dev/null 2>&1 || fail "Required tool '$cmd' not found. Please install it and try again."
done

# Fetch latest release version
step "Fetching latest release..."
RELEASE_JSON=$(curl -fsSL --retry 3 "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
  -H "User-Agent: pookie-installer") || fail "Could not fetch release info. Check your internet connection."

# Parse version (no jq dependency)
TAG=$(echo "$RELEASE_JSON" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
VERSION="${TAG#v}"
[ -n "$VERSION" ] || fail "Could not determine latest version."
ok "Latest version: $TAG"

# Build download URL
ASSET="pookie_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${OWNER}/${REPO}/releases/download/${TAG}/${ASSET}"
TMP_DIR=$(mktemp -d)
TMP_ARCHIVE="${TMP_DIR}/${ASSET}"

# Download
step "Downloading ${ASSET}..."
curl -fsSL --retry 3 --progress-bar -o "$TMP_ARCHIVE" "$URL" \
  || fail "Download failed: ${URL}"
ok "Downloaded ${ASSET}"

# Extract
step "Extracting..."
tar -xzf "$TMP_ARCHIVE" -C "$TMP_DIR"
ok "Extracted"

# Find binary in extracted files
EXTRACTED_BIN=$(find "$TMP_DIR" -type f -name "$BIN" | head -1)
[ -n "$EXTRACTED_BIN" ] || fail "Could not find '${BIN}' in archive."

# Install
step "Installing to ${INSTALL_DIR}/${BIN}..."
if [ -w "$INSTALL_DIR" ]; then
  cp "$EXTRACTED_BIN" "${INSTALL_DIR}/${BIN}"
  chmod +x "${INSTALL_DIR}/${BIN}"
else
  # Need sudo
  echo -e "  ${DIM}(sudo required to write to ${INSTALL_DIR})${RESET}"
  sudo cp "$EXTRACTED_BIN" "${INSTALL_DIR}/${BIN}"
  sudo chmod +x "${INSTALL_DIR}/${BIN}"
fi
ok "Installed to ${INSTALL_DIR}/${BIN}"

# Cleanup
rm -rf "$TMP_DIR"

# Verify
step "Verifying installation..."
INSTALLED_VER=$("${INSTALL_DIR}/${BIN}" version 2>&1) || fail "Verification failed. Try running: ${INSTALL_DIR}/${BIN} version"
ok "$INSTALLED_VER"

echo ""
echo -e "  ${GREEN}${BOLD}PookiePaws ${TAG} installed successfully!${RESET}"
echo ""
echo -e "  ${BOLD}Next steps:${RESET}"
echo -e "  ${DIM}  pookie init     <- configure your providers${RESET}"
echo -e "  ${DIM}  pookie start    <- launch the console${RESET}"
echo -e "  ${DIM}  open http://127.0.0.1:18800${RESET}"
echo ""
