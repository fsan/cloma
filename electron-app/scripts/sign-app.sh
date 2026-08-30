#!/usr/bin/env bash
# Ad-hoc sign a macOS Electron .app bundle with the entitlements Electron
# requires on Apple Silicon (JIT, unsigned executable memory, etc.).
#
# A simple `codesign --force --deep --sign -` is NOT enough for Electron on
# Apple Silicon: the V8 engine needs JIT entitlements, and nested frameworks
# and helpers must be signed individually (inside-out). Without this, macOS
# kills the process on launch with "Launchd job spawn failed" (POSIX 111) or
# "<app> cannot be opened".
#
# Usage:
#   ./sign-app.sh "/path/to/Cloma.app"
#
set -euo pipefail

BUNDLE="${1:-}"
if [ -z "${BUNDLE}" ] || [ ! -d "${BUNDLE}" ]; then
  echo "Usage: $0 <path-to-Cloma.app>" >&2
  exit 1
fi

if ! command -v codesign >/dev/null 2>&1; then
  echo "ERROR: codesign not found. This script must be run on macOS." >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENTITLEMENTS="${SCRIPT_DIR}/entitlements.plist"

if [ ! -f "${ENTITLEMENTS}" ]; then
  echo "ERROR: entitlements plist not found at ${ENTITLEMENTS}" >&2
  exit 1
fi

echo "Signing ${BUNDLE} (inside-out with entitlements)…"

CONTENTS="${BUNDLE}/Contents"
FW_DIR="${CONTENTS}/Frameworks"

# 1. Sign nested helper binaries inside Electron Framework (deepest first).
EF_HELPERS="${FW_DIR}/Electron Framework.framework/Versions/A/Helpers"
if [ -d "${EF_HELPERS}" ]; then
  for h in "${EF_HELPERS}"/*; do
    [ -f "${h}" ] || continue
    codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${h}" 2>&1 || true
  done
fi

# 2. Sign helper apps that live directly in Contents/Frameworks/
#    (Electron Helper.app, Electron Helper (GPU).app, etc.)
if [ -d "${FW_DIR}" ]; then
  for h in "${FW_DIR}"/*.app; do
    [ -d "${h}" ] || continue
    codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${h}" 2>&1 || true
  done
fi

# 3. Sign all nested frameworks.
if [ -d "${FW_DIR}" ]; then
  for fw in "${FW_DIR}"/*.framework; do
    [ -d "${fw}" ] || continue
    codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${fw}" 2>&1 || true
  done
fi

# 4. Sign helper apps in Library/LoginItems (if present).
HELPERS_DIR="${CONTENTS}/Library/LoginItems"
if [ -d "${HELPERS_DIR}" ]; then
  for h in "${HELPERS_DIR}"/*.app; do
    [ -d "${h}" ] || continue
    codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${h}" 2>&1 || true
  done
fi

# 5. Sign the main bundle with entitlements (outermost, last).
codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${BUNDLE}" 2>&1

# 6. Verify.
if codesign --verify --deep --strict "${BUNDLE}" 2>&1; then
  echo "Signed and verified: ${BUNDLE}"
else
  echo "Warning: signature verification reported issues (may still work)." >&2
fi

# 7. Clear quarantine.
if command -v xattr >/dev/null 2>&1; then
  xattr -cr "${BUNDLE}" 2>/dev/null || true
  echo "Cleared quarantine attributes."
fi

echo "Done. You can now open ${BUNDLE}"