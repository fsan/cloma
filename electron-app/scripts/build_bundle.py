#!/usr/bin/env python3
"""Assemble a macOS .app bundle for the cloma Electron app.

This script replaces electron-builder. It downloads the official Electron
release for darwin-arm64, extracts the pre-built Electron.app, swaps in our
own main.js / preload.js / renderer files and icons, and writes a minimal
Info.plist. No npm dependencies are required.

Usage:
    python3 scripts/build_bundle.py [--electron-version 31.7.7]

Outputs:
    dist/mac-arm64/Cloma.app
"""

import argparse
import json
import os
import plistlib
import shutil
import stat
import sys
import tarfile
import tempfile
import urllib.request
import zipfile

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
APP_DIR = os.path.dirname(SCRIPT_DIR)
DIST_DIR = os.path.join(APP_DIR, "dist")
BUILD_DIR = os.path.join(APP_DIR, "build")

APP_NAME = "Cloma"
APP_BUNDLE_ID = "com.fsan.cloma"
ELECTRON_VERSION_DEFAULT = "31.7.7"
ARCH = "arm64"

# Files from our app that go into the bundle's Resources/app/ directory.
APP_FILES = ["main.js", "preload.js"]
APP_DIRS = ["renderer"]
ICON_FILES = ["trayIconTemplate.png", "trayIconTemplate@2x.png"]


def log(msg):
    print(msg, flush=True)


def die(msg, code=1):
    print(f"ERROR: {msg}", file=sys.stderr, flush=True)
    sys.exit(code)


def download(url, dest):
    """Download a URL to a file with a simple progress indicator."""
    log(f"Downloading {url} ...")
    req = urllib.request.Request(url, headers={"User-Agent": "cloma-build/1.0"})
    with urllib.request.urlopen(req) as resp, open(dest, "wb") as out:
        total = int(resp.headers.get("Content-Length", 0))
        done = 0
        chunk = 1024 * 256
        while True:
            data = resp.read(chunk)
            if not data:
                break
            out.write(data)
            done += len(data)
            if total:
                pct = done * 100 // total
                sys.stdout.write(f"\r  {done // 1024} KiB / {total // 1024} KiB ({pct}%)")
                sys.stdout.flush()
        sys.stdout.write("\n")


def _is_symlink_zipinfo(info):
    """Return True if a ZipInfo entry represents a Unix symlink."""
    # The upper 16 bits of external_attr hold the Unix st_mode.
    mode = (info.external_attr >> 16) & 0xFFFF
    return stat.S_ISLNK(mode)


def _extract_zip_with_symlinks(zf, dest):
    """Extract a zip archive preserving Unix symlinks.

    Python's zipfile.extractall() writes symlink entries as regular files
    containing the link target as text, which corrupts macOS .app bundles
    (framework versioned directories rely on symlinks like
    Versions/Current -> A). This extractor detects symlink entries and
    creates real symlinks instead.
    """
    # First pass: extract all non-symlink entries (dirs and files).
    symlinks = []
    for info in zf.infolist():
        if info.is_dir():
            # A directory entry — create it.
            dir_path = os.path.join(dest, info.filename)
            os.makedirs(dir_path, exist_ok=True)
            continue
        if _is_symlink_zipinfo(info):
            symlinks.append(info)
            continue
        target_path = os.path.join(dest, info.filename)
        os.makedirs(os.path.dirname(target_path), exist_ok=True)
        with zf.open(info) as src, open(target_path, "wb") as out:
            shutil.copyfileobj(src, out)
        # Preserve permissions for executable files.
        mode = (info.external_attr >> 16) & 0o777
        if mode:
            os.chmod(target_path, mode)

    # Second pass: create symlinks (after their parent dirs exist).
    for info in symlinks:
        link_path = os.path.join(dest, info.filename)
        os.makedirs(os.path.dirname(link_path), exist_ok=True)
        with zf.open(info) as f:
            target = f.read().decode("utf-8").strip()
        # Remove any stale regular file/dir left by a previous broken extract.
        if os.path.lexists(link_path):
            if os.path.isdir(link_path) and not os.path.islink(link_path):
                shutil.rmtree(link_path)
            else:
                os.remove(link_path)
        os.symlink(target, link_path)


def _cache_is_valid(app_path):
    """Check whether the cached Electron.app has intact framework symlinks.

    A previous version of this script used zipfile.extractall(), which writes
    symlink entries as regular files containing the target path as text. That
    corrupts the framework's versioned directory structure. We detect that
    corruption here so we can re-extract from the zip.
    """
    fw = os.path.join(
        app_path, "Contents", "Frameworks", "Electron Framework.framework"
    )
    # These top-level entries must be symlinks into Versions/A/.
    for entry in ("Electron Framework", "Helpers", "Libraries", "Resources"):
        p = os.path.join(fw, entry)
        if not os.path.lexists(p) or not os.path.islink(p):
            return False
    # Versions/Current must be a symlink to A.
    current = os.path.join(fw, "Versions", "Current")
    if not os.path.lexists(current) or not os.path.islink(current):
        return False
    return True


def get_electron(version, cache_dir):
    """Download and cache the Electron darwin-arm64 zip. Returns path to the
    extracted Electron.app directory."""
    cache_key = f"electron-v{version}-darwin-{ARCH}"
    cache_path = os.path.join(cache_dir, cache_key)
    app_path = os.path.join(cache_path, "Electron.app")

    if os.path.isdir(app_path):
        if _cache_is_valid(app_path):
            log(f"Using cached Electron {version} at {cache_path}")
            return app_path
        log("Cached Electron.app has broken symlinks; re-extracting...")
        shutil.rmtree(cache_path)

    os.makedirs(cache_path, exist_ok=True)
    zip_path = os.path.join(cache_path, "electron.zip")

    if not os.path.isfile(zip_path):
        url = (
            f"https://github.com/electron/electron/releases/download/v{version}/"
            f"electron-v{version}-darwin-{ARCH}.zip"
        )
        download(url, zip_path)

    log("Extracting Electron archive (preserving symlinks)...")
    with zipfile.ZipFile(zip_path) as zf:
        _extract_zip_with_symlinks(zf, cache_path)

    if not os.path.isdir(app_path):
        die(f"Electron.app not found in archive after extraction at {app_path}")
    if not _cache_is_valid(app_path):
        die("Extracted Electron.app still has broken symlinks; cannot continue.")
    return app_path


def make_info_plist(dest_path, version):
    """Write a minimal Info.plist for the app."""
    plist = {
        "CFBundleDisplayName": APP_NAME,
        "CFBundleExecutable": "Electron",
        "CFBundleIconFile": "icon.icns",
        "CFBundleIdentifier": APP_BUNDLE_ID,
        "CFBundleInfoDictionaryVersion": "6.0",
        "CFBundleName": APP_NAME,
        "CFBundleDisplayName": APP_NAME,
        "CFBundlePackageType": "APPL",
        "CFBundleShortVersionString": version,
        "CFBundleVersion": version,
        "LSMinimumSystemVersion": "11.0",
        "LSUIElement": True,  # hide from Dock — menu bar app
        "NSHighResolutionCapable": True,
        "NSMicrophoneUsageDescription": "Microphone access is not used.",
        "NSCameraUsageDescription": "Camera access is not used.",
        "ElectronTeamID": "",
        "DTSDKName": "macosx",
        "DTSDKBuild": "12.3",
        "DTPlatformName": "macosx",
        "DTPlatformVersion": "12.3",
        "DTPlatformBuild": "",
        "DTXcode": "1331",
        "DTXcodeBuild": "13C100",
        "BuildMachineOSBuild": "",
    }
    with open(dest_path, "wb") as f:
        plistlib.dump(plist, f)


def png_to_icns(png_path, icns_path):
    """Create a minimal .icns file from a PNG using the icns header + PNG
    payload. macOS accepts PNG data inside an icns container with the 'ic09'
    (512x512) or 'ic10' (1024x1024) OSType. We use 'ic10' for 1024x1024 PNGs
    and 'ic09' for 512x512."""
    from PIL import Image

    img = Image.open(png_path)
    w, h = img.size

    # Determine the appropriate OSType based on size.
    if w >= 1024:
        ostype = b"ic10"  # 1024x1024 (retina 512x512)
        # Resize to 1024 if larger.
        if w != 1024 or h != 1024:
            img = img.resize((1024, 1024), Image.LANCZOS)
            w, h = 1024, 1024
    elif w >= 512:
        ostype = b"ic09"  # 512x512
        if w != 512 or h != 512:
            img = img.resize((512, 512), Image.LANCZOS)
            w, h = 512, 512
    else:
        ostype = b"ic07"  # 128x128
        if w != 128 or h != 128:
            img = img.resize((128, 128), Image.LANCZOS)
            w, h = 128, 128

    # Save PNG bytes.
    import io

    png_buf = io.BytesIO()
    img.save(png_buf, format="PNG")
    png_bytes = png_buf.getvalue()

    # icns element: 4-byte OSType + 4-byte length (includes these 8 bytes) + data
    elem_len = 8 + len(png_bytes)
    element = ostype + elem_len.to_bytes(4, "big") + png_bytes

    # icns file header: 'icns' + 4-byte total length (includes 8-byte header)
    total_len = 8 + len(element)
    header = b"icns" + total_len.to_bytes(4, "big")

    with open(icns_path, "wb") as f:
        f.write(header)
        f.write(element)


def get_app_version():
    """Read the version from package.json."""
    pkg_path = os.path.join(APP_DIR, "package.json")
    with open(pkg_path) as f:
        pkg = json.load(f)
    return pkg.get("version", "0.1.0")


def build(electron_version, cache_dir):
    version = get_app_version()
    os.makedirs(DIST_DIR, exist_ok=True)

    # 1. Get Electron.app
    electron_app = get_electron(electron_version, cache_dir)

    # 2. Determine output path
    out_dir = os.path.join(DIST_DIR, f"mac-{ARCH}")
    os.makedirs(out_dir, exist_ok=True)
    dest_app = os.path.join(out_dir, f"{APP_NAME}.app")

    # 3. Remove previous build
    if os.path.exists(dest_app):
        shutil.rmtree(dest_app)

    # 4. Copy the entire Electron.app as our base
    log(f"Copying Electron.app -> {dest_app}")
    shutil.copytree(electron_app, dest_app, symlinks=True)

    # 5. Replace Info.plist
    contents_dir = os.path.join(dest_app, "Contents")
    make_info_plist(os.path.join(contents_dir, "Info.plist"), version)

    # 6. Replace the app icon
    icon_png = os.path.join(BUILD_DIR, "icon.png")
    if os.path.isfile(icon_png):
        resources_dir = os.path.join(contents_dir, "Resources")
        icns_path = os.path.join(resources_dir, "icon.icns")
        try:
            png_to_icns(icon_png, icns_path)
            log(f"Wrote icon.icns from {icon_png}")
        except Exception as e:
            log(f"Warning: could not create icon.icns ({e}); copying PNG instead")
            shutil.copy2(icon_png, os.path.join(resources_dir, "icon.png"))

    # 7. Create Resources/app/ with our JS files
    app_resources = os.path.join(contents_dir, "Resources", "app")
    os.makedirs(app_resources, exist_ok=True)

    for fname in APP_FILES:
        src = os.path.join(APP_DIR, fname)
        if not os.path.isfile(src):
            die(f"Missing app file: {src}")
        shutil.copy2(src, os.path.join(app_resources, fname))

    for dname in APP_DIRS:
        src = os.path.join(APP_DIR, dname)
        if not os.path.isdir(src):
            die(f"Missing app directory: {src}")
        dest = os.path.join(app_resources, dname)
        if os.path.exists(dest):
            shutil.rmtree(dest)
        shutil.copytree(src, dest)

    # Copy tray icons into Resources/app/build/
    tray_dir = os.path.join(app_resources, "build")
    os.makedirs(tray_dir, exist_ok=True)
    for icon in ICON_FILES:
        src = os.path.join(BUILD_DIR, icon)
        if os.path.isfile(src):
            shutil.copy2(src, os.path.join(tray_dir, icon))
        else:
            log(f"Warning: tray icon not found: {src}")

    # Copy the signing script and entitlements into Resources/app/scripts/
    # so the user can re-sign the bundle on the host if needed.
    scripts_src = os.path.join(APP_DIR, "scripts")
    scripts_dest = os.path.join(app_resources, "scripts")
    if os.path.isdir(scripts_dest):
        shutil.rmtree(scripts_dest)
    os.makedirs(scripts_dest, exist_ok=True)
    for sname in ("sign-app.sh", "entitlements.plist"):
        ssrc = os.path.join(scripts_src, sname)
        if os.path.isfile(ssrc):
            shutil.copy2(ssrc, os.path.join(scripts_dest, sname))
            if sname.endswith(".sh"):
                st = os.stat(os.path.join(scripts_dest, sname))
                os.chmod(os.path.join(scripts_dest, sname), st.st_mode | stat.S_IEXEC)

    # 8. Write a minimal package.json into Resources/app so Electron knows the
    # entry point.
    pkg = {
        "name": "cloma",
        "version": version,
        "main": "main.js",
    }
    with open(os.path.join(app_resources, "package.json"), "w") as f:
        json.dump(pkg, f, indent=2)

    log(f"\nBuilt: {dest_app}")
    return dest_app


def main():
    parser = argparse.ArgumentParser(description="Build the cloma .app bundle")
    parser.add_argument(
        "--electron-version",
        default=ELECTRON_VERSION_DEFAULT,
        help=f"Electron release version (default: {ELECTRON_VERSION_DEFAULT})",
    )
    parser.add_argument(
        "--cache-dir",
        default=os.path.join(os.path.expanduser("~"), ".cache", "cloma-electron"),
        help="Directory to cache the Electron download",
    )
    args = parser.parse_args()

    build(args.electron_version, args.cache_dir)


if __name__ == "__main__":
    main()