# Cloma

A macOS menu bar app for [cloma](https://github.com/fsan/cloma).

## What it does

Cloma lives in the macOS menu bar and lets you manage your cloma
Docker sandboxes without opening a terminal:

- **View** all cloma-managed sandboxes with their status, workspace path, agent, and creation time
- **Group** sandboxes by workspace or by agent
- **Sort** sandboxes by name or by creation time
- **Status colors** — running (green), stopped (gray), other (amber) shown as colored badges
- **Start** a stopped sandbox
- **Stop** a running sandbox
- **View logs** in a streaming log window
- **Delete** a sandbox (with confirmation)
- **Force delete** a sandbox (removes even if running)

It shells out to the `cloma` and `docker` CLIs for all operations, so both
must be on your `PATH`.

## Architecture

```
electron-app/
├── main.js                 # Electron main process: tray, window, IPC, lifecycle
├── preload.js              # Context-isolated IPC bridge (window.cloma API)
├── renderer/
│   ├── index.html          # Main dropdown UI
│   ├── app.js              # Main UI logic (list, actions, refresh)
│   ├── styles.css          # Main UI styles
│   ├── logs.html           # Log viewer window
│   ├── logs.js             # Log streaming logic
│   └── log-styles.css       # Log viewer styles
├── build/
│   ├── icon.png            # 1024×1024 app icon (generated)
│   ├── trayIconTemplate.png        # 22×22 menu bar template icon (generated)
│   └── trayIconTemplate@2x.png     # 44×44 menu bar template icon (generated)
├── scripts/
│   ├── generate_icon.py    # Python (Pillow) icon generator
│   └── build_bundle.py     # Python .app bundle assembler (replaces electron-builder)
├── build-app.sh            # Build + install script
└── package.json            # App metadata (no npm dependencies)
```

The app uses a template tray icon (black "C" with alpha) so macOS renders it
correctly in both light and dark menu bars.

## Building

The build does **not** use npm or electron-builder. It downloads the official
Electron release directly and assembles the `.app` bundle by hand, avoiding
deprecated transitive npm dependencies.

```bash
# From the electron-app directory:
./build-app.sh              # build (generates icons + assembles .app)
./build-app.sh --install    # build and install to /Applications
./build-app.sh --uninstall  # remove the app from /Applications
./build-app.sh --clean      # remove build artifacts

# Or from the cloma root:
make build-app      # build only
make install-app    # build and install
make clean-app      # remove build artifacts
make uninstall      # remove both the CLI and the app
```

Requirements: Python 3 (with Pillow for icon generation) and curl.

## macOS “cannot be opened” / “Launch failed” / Gatekeeper

The bundle is assembled from the official Electron release and then modified
(swapped `Info.plist`, app files, and icon), which invalidates Electron's
original code signature. On Apple Silicon the kernel kills unsigned or
invalidly-signed executables on launch, producing errors like
`"Cloma" cannot be opened` or `Launchd job spawn failed` (POSIX 111).

Two things are required for the bundle to launch:

1. **Intact framework symlinks.** macOS frameworks use a versioned directory
   layout with symlinks (`Versions/Current -> A`, and top-level `Resources`,
   `Helpers`, etc. are symlinks into `Versions/Current/`). The build script
   extracts the Electron zip with a custom symlink-preserving extractor. If
   the symlinks are broken (e.g. extracted with a tool that writes symlinks as
   regular text files), `codesign` reports "embedded framework contains
   modified or invalid version" and macOS refuses to launch. Run
   `./build-app.sh --clean` (or `make clean-app`) to clear the cache and
   rebuild if you ever see that error.

2. **Inside-out ad-hoc signing with JIT entitlements.** A simple
   `codesign --force --deep --sign -` is **not sufficient** for Electron on
   Apple Silicon because:
   - `--deep` does not apply entitlements to nested code.
   - Electron's V8 engine requires JIT entitlements
     (`com.apple.security.cs.allow-jit`,
      `com.apple.security.cs.allow-unsigned-executable-memory`) — without them
      the kernel kills the process immediately on spawn.
   - Frameworks and helper executables must be signed individually (inside-out).

The build/install script handles signing automatically when run on macOS by
signing each nested framework/helper first, then the main bundle, all with the
entitlements plist (`scripts/entitlements.plist`). If you build inside the
Linux sandbox and copy the bundle to the host manually, run on the host:

```bash
# Inside-out ad-hoc signing with Electron JIT entitlements:
./scripts/sign-app.sh "/Applications/Cloma.app"
# Clear the quarantine attribute:
xattr -cr "/Applications/Cloma.app"
```

## Debugging

The app writes a debug log to `~/Library/Logs/cloma-app.log` on macOS. It
records:

- every command invocation (with pid, timeout, exit code, and truncated stderr)
- JSON parse errors and the resolved `cloma`/`docker` binary paths at startup
- renderer console messages (`renderer-console[...]`) and load failures
  (`renderer did-fail-load` / `renderer preload-error`)
- renderer-side log/error messages forwarded via the preload bridge
  (`renderer: ...` / `renderer ERROR: ...`), including uncaught exceptions and
  unhandled promise rejections

If the app is stuck on "Loading sandboxes…", check this log first —
right-click the tray icon → **Open Debug Log** to reveal it in Finder. The
renderer log lines will show exactly where `app.js` is failing (e.g. a missing
preload bridge, a thrown error before `refresh()`, or a failed IPC call).

## Development

To run the app in dev mode you need Electron installed locally (e.g. via
`npm install electron`), then:

```bash
npx electron .    # launch the app in dev mode
```