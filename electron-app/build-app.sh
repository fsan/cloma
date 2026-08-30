#!/usr/bin/env bash
# Build the cloma Electron app for macOS.
#
# Produces a packaged .app bundle under electron-app/dist/mac-arm64/.
# The build runs inside this Linux sandbox but targets macOS (arm64),
# so the resulting bundle can be installed on the macOS host.
#
# This script does NOT use npm or electron-builder. It downloads the official
# Electron release directly and assembles the .app bundle by hand, avoiding
# deprecated transitive npm dependencies.
#
# The resulting bundle is ad-hoc signed (codesign --sign -) when the macOS
# `codesign` tool is available, and the macOS quarantine attribute is cleared
# on install. On Apple Silicon an unsigned/modified Electron bundle is killed
# by the kernel on launch ("<app> cannot be opened"), so ad-hoc signing is
# required for the app to start.
#
# Usage:
#   ./build-app.sh            # build the app
#   ./build-app.sh --install   # build and install to /Applications on the host
#   ./build-app.sh --uninstall # remove the app from /Applications on the host
#   ./build-app.sh --clean     # remove build artifacts (dist, icons, cache)
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="${SCRIPT_DIR}"
HOST_ROOT="$(cd "${APP_DIR}/.." && pwd)"

# ---- helpers ----

color_green() { printf '\033[32m%s\033[0m\n' "$1"; }
color_yellow() { printf '\033[33m%s\033[0m\n' "$1"; }
color_red() { printf '\033[31m%s\033[0m\n' "$1"; }

need() {
  command -v "$1" >/dev/null 2>&1 || { color_red "missing dependency: $1"; exit 1; }
}

# ---- optional clean step (handled early so we skip the build) ----

clean_app() {
  color_yellow "Removing build artifacts…"
  rm -rf "${APP_DIR}/dist"
  rm -f "${APP_DIR}/build/icon.png"
  rm -f "${APP_DIR}/build/trayIconTemplate.png"
  rm -f "${APP_DIR}/build/trayIconTemplate@2x.png"
  # Also clear the Electron download cache. Older builds extracted the zip
  # with zipfile.extractall(), which corrupted framework symlinks; clearing
  # the cache ensures a clean re-extract with the symlink-preserving extractor.
  local cache_dir="${HOME}/.cache/cloma-electron"
  if [ -d "${cache_dir}" ]; then
    rm -rf "${cache_dir}"
  fi
  color_green "Cleaned cloma app build artifacts."
}

# ---- optional uninstall step (handled early so we skip the build) ----

uninstall_app() {
  local app_name="Cloma.app"
  local removed=0

  # Candidate install locations (mirrors install_app).
  local host_home=""
  for u in /Users/*; do
    if [ -d "${u}" ]; then host_home="${u}"; break; fi
  done

  local candidates=("/Applications/${app_name}")
  if [ -n "${host_home}" ]; then
    candidates+=("${host_home}/Applications/${app_name}")
  fi

  for dest in "${candidates[@]}"; do
    if [ -d "${dest}" ]; then
      color_yellow "Removing ${dest}…"
      rm -rf "${dest}"
      removed=1
    fi
  done

  if [ "${removed}" -eq 1 ]; then
    color_green "Removed cloma app from Applications."
  else
    color_yellow "cloma app is not installed in /Applications (nothing to remove)."
  fi
}

if [ "${1:-}" = "--uninstall" ]; then
  uninstall_app
  exit 0
fi

if [ "${1:-}" = "--clean" ]; then
  clean_app
  exit 0
fi

# ---- prerequisites ----

need uv
need curl

# ---- generate icons if missing ----
# Pillow is not installed globally; uv run --with fetches it into an
# ephemeral environment for the duration of the script run.
if [ ! -f "${APP_DIR}/build/icon.png" ] || [ ! -f "${APP_DIR}/build/trayIconTemplate.png" ]; then
  color_yellow "Generating icons…"
  (cd "${APP_DIR}" && uv run --with pillow scripts/generate_icon.py)
fi

# ---- build the app ----

color_yellow "Building Electron app…"
(cd "${APP_DIR}" && uv run --with pillow scripts/build_bundle.py)

# Locate the produced .app bundle.
APP_BUNDLE="$(find "${APP_DIR}/dist" -maxdepth 3 -name '*.app' -print -quit 2>/dev/null || true)"
if [ -z "${APP_BUNDLE}" ]; then
  color_red "Build succeeded but no .app bundle was found under ${APP_DIR}/dist"
  exit 1
fi

color_green "Built: ${APP_BUNDLE}"

# ---- ad-hoc signing (required on Apple Silicon) ----
#
# We modified the official Electron.app (swapped Info.plist, added our app
# files and icon), which invalidates its original signature. On Apple Silicon
# the kernel kills any unsigned or invalidly-signed executable on launch,
# producing the "<app> cannot be opened" / "Launchd job spawn failed" error.
#
# A simple `codesign --force --deep --sign -` is NOT sufficient for Electron on
# Apple Silicon because:
#   1. `--deep` does not apply entitlements to nested code.
#   2. Electron's V8 engine requires JIT entitlements
#      (com.apple.security.cs.allow-jit,
#       com.apple.security.cs.allow-unsigned-executable-memory) — without them
#      the kernel kills the process immediately on spawn (POSIX error 111,
#      "Launchd job spawn failed").
#   3. Frameworks and helper executables must be signed individually
#      (inside-out) so each nested component has a valid signature.
#
# The fix is to sign every nested framework/helper first, then sign the main
# bundle, all with the entitlements plist. This is what electron-builder and
# @electron/osx-sign do under the hood.
#
# `codesign` only exists on macOS, so inside the Linux sandbox we skip this and
# print the commands for the user to run on the host instead.
ENTITLEMENTS="${APP_DIR}/scripts/entitlements.plist"

sign_bundle() {
  local bundle="$1"
  if ! command -v codesign >/dev/null 2>&1; then
    color_yellow "codesign not available in this sandbox; skipping signing."
    echo "On the macOS host, run the signing script before launching:"
    echo "  \"${APP_DIR}/scripts/sign-app.sh\" \"${bundle}\""
    return 0
  fi

  color_yellow "Ad-hoc signing ${bundle} (inside-out with entitlements)…"
  local contents="${bundle}/Contents"
  local fw_dir="${contents}/Frameworks"

  # 1. Sign nested helper binaries inside Electron Framework (deepest first).
  local ef_helpers="${fw_dir}/Electron Framework.framework/Versions/A/Helpers"
  if [ -d "${ef_helpers}" ]; then
    for h in "${ef_helpers}"/*; do
      [ -f "${h}" ] || continue
      codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${h}" 2>&1 || true
    done
  fi

  # 2. Sign helper apps that live directly in Contents/Frameworks/
  #    (Electron Helper.app, Electron Helper (GPU).app, etc.)
  if [ -d "${fw_dir}" ]; then
    for h in "${fw_dir}"/*.app; do
      [ -d "${h}" ] || continue
      codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${h}" 2>&1 || true
    done
  fi

  # 3. Sign all nested frameworks.
  if [ -d "${fw_dir}" ]; then
    for fw in "${fw_dir}"/*.framework; do
      [ -d "${fw}" ] || continue
      codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${fw}" 2>&1 || true
    done
  fi

  # 4. Sign helper apps in Library/LoginItems (if present).
  local loginitems="${contents}/Library/LoginItems"
  if [ -d "${loginitems}" ]; then
    for h in "${loginitems}"/*.app; do
      [ -d "${h}" ] || continue
      codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${h}" 2>&1 || true
    done
  fi

  # 5. Sign the main bundle with entitlements (outermost, last).
  codesign --force --sign - --entitlements "${ENTITLEMENTS}" "${bundle}" 2>&1 || {
    color_red "Ad-hoc signing failed; the app may be blocked by macOS on launch."
    color_yellow "On the host, run: \"${APP_DIR}/scripts/sign-app.sh\" \"${bundle}\""
    return 1
  }

  # 6. Verify the signature.
  if codesign --verify --deep --strict "${bundle}" 2>&1; then
    color_green "Signed and verified."
  else
    color_yellow "Warning: signature verification reported issues (may still work)."
  fi
}

sign_bundle "${APP_BUNDLE}"

# ---- optional install step ----

# install_app copies the built .app bundle to /Applications on the macOS host.
# Inside the Linux sandbox /Applications is not mounted, so we detect the host
# Applications path via the /Users mount and copy there. If no writable host
# location is found, we print instructions for manual installation.
install_app() {
  local bundle="$1"
  local app_name
  app_name="$(basename "${bundle}")"

  # The /Users/<user> path is the host home directory mounted into the sandbox.
  local host_home=""
  for u in /Users/*; do
    if [ -d "${u}" ]; then host_home="${u}"; break; fi
  done

  local dest_dir=""
  if [ -d "/Applications" ] && [ -w "/Applications" ]; then
    dest_dir="/Applications"
  elif [ -n "${host_home}" ] && [ -d "${host_home}/Applications" ] && [ -w "${host_home}/Applications" ]; then
    dest_dir="${host_home}/Applications"
  fi

  if [ -n "${dest_dir}" ]; then
    color_yellow "Installing ${app_name} to ${dest_dir}…"
    rm -rf "${dest_dir}/${app_name}"
    cp -R "${bundle}" "${dest_dir}/${app_name}"
    local installed="${dest_dir}/${app_name}"

    # Re-sign the installed copy (inside-out with entitlements) and clear the
    # macOS quarantine attribute so Gatekeeper does not block it. These are
    # no-ops on Linux (the sandbox) but run for real on a macOS host.
    sign_bundle "${installed}"
    if command -v xattr >/dev/null 2>&1; then
      xattr -cr "${installed}" 2>/dev/null || true
    fi

    color_green "Installed: ${installed}"
  else
    # No writable host Applications directory is visible from this sandbox.
    # Print the bundle path so the user can copy it manually on the host.
    color_yellow "Could not find a writable /Applications from this sandbox."
    color_green "Built bundle: ${APP_BUNDLE}"
    echo ""
    echo "To install on the host, run:"
    echo "  cp -R \"${APP_BUNDLE}\" /Applications/"
    if [ -n "${host_home}" ]; then
      echo "  cp -R \"${APP_BUNDLE}\" \"${host_home}/Applications/\""
    fi
    echo ""
    echo "Then, on the host, sign and clear quarantine so macOS does not block it:"
    echo "  \"${APP_DIR}/scripts/sign-app.sh\" \"/Applications/${app_name}\""
    echo "  xattr -cr \"/Applications/${app_name}\""
    echo ""
    echo "Or open it directly:"
    echo "  open \"${APP_BUNDLE}\""
  fi
}

if [ "${1:-}" = "--install" ]; then
  install_app "${APP_BUNDLE}"
fi