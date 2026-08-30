const { app, BrowserWindow, Tray, Menu, ipcMain, nativeImage, shell } = require('electron');
const path = require('path');
const fs = require('fs');
const os = require('os');
const { spawn } = require('child_process');

let tray = null;
let window = null;
let logWindows = new Map(); // name -> BrowserWindow

// ---- Debug logging ----
// Write to ~/Library/Logs/cloma-app.log on macOS so issues can be diagnosed.
const LOG_FILE = path.join(
  process.env.HOME || os.homedir() || '/tmp',
  'Library', 'Logs', 'cloma-app.log'
);
let _logStream = null;
function logFile() {
  if (_logStream) return _logStream;
  try {
    fs.mkdirSync(path.dirname(LOG_FILE), { recursive: true });
    _logStream = fs.createWriteStream(LOG_FILE, { flags: 'a' });
  } catch (e) {
    _logStream = null;
  }
  return _logStream;
}
function debug(msg) {
  const line = `[${new Date().toISOString()}] ${msg}`;
  try { const s = logFile(); if (s) s.write(line + '\n'); } catch (e) {}
  if (process.env.CLOMA_DEBUG) console.log(line);
}

// Common install locations for cloma and docker on macOS. GUI apps get a
// minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin) that excludes /usr/local/bin
// and /opt/homebrew/bin, so we search these explicitly.
const CLOMA_SEARCH_PATHS = [
  '/usr/local/bin/cloma',
  '/opt/homebrew/bin/cloma',
  path.join(process.env.HOME || '', '.local', 'bin', 'cloma'),
  path.join(process.resourcesPath || '', 'bin', 'cloma'),
];
const DOCKER_SEARCH_PATHS = [
  '/usr/local/bin/docker',
  '/opt/homebrew/bin/docker',
  '/Applications/Docker.app/Contents/Resources/bin/docker',
];

let _clomaBin = null;
let _dockerBin = null;

// Resolve the cloma binary path. Search known locations, then PATH.
function clomaBin() {
  if (_clomaBin) return _clomaBin;
  const dev = process.env.CLOMA_BIN;
  if (dev) { _clomaBin = dev; return dev; }
  for (const p of CLOMA_SEARCH_PATHS) {
    try { if (fs.existsSync(p) && fs.statSync(p).isFile()) { _clomaBin = p; return p; } } catch (e) {}
  }
  _clomaBin = 'cloma'; // last resort: rely on PATH
  return 'cloma';
}

// Resolve the docker binary path. Search known locations, then PATH.
function dockerBin() {
  if (_dockerBin) return _dockerBin;
  const dev = process.env.DOCKER_BIN;
  if (dev) { _dockerBin = dev; return dev; }
  for (const p of DOCKER_SEARCH_PATHS) {
    try { if (fs.existsSync(p) && fs.statSync(p).isFile()) { _dockerBin = p; return p; } } catch (e) {}
  }
  _dockerBin = 'docker';
  return 'docker';
}

// Run a command and return { stdout, stderr, code }. Times out after 15s.
//
// Uses spawn with detached: true so the child runs in its own process group.
// This is critical because cloma spawns docker as a grandchild — if we only
// kill cloma on timeout, the orphaned docker process keeps the stdout pipe
// open and Node's execFile callback never fires, hanging the UI forever.
// With detached mode we can kill the entire process group (-pid) on timeout.
function runCmd(bin, args, opts = {}) {
  return new Promise((resolve) => {
    const timeout = opts.timeout || 15000;
    const env = augmentedEnv();
    let settled = false;
    let stdout = '';
    let stderr = '';
    let timer = null;

    debug(`runCmd: ${bin} ${args.join(' ')} (timeout=${timeout}ms)`);
    const child = spawn(bin, args, {
      env,
      detached: true,
      stdio: ['ignore', 'pipe', 'pipe'],
      cwd: opts.cwd || undefined,
    });
    debug(`runCmd: spawned pid=${child.pid}`);

    child.stdout.on('data', (data) => { stdout += data.toString(); });
    child.stderr.on('data', (data) => { stderr += data.toString(); });

    const done = (code, err) => {
      if (settled) return;
      settled = true;
      if (timer) { clearTimeout(timer); timer = null; }
      debug(`runCmd: ${bin} ${args.join(' ')} -> code=${code} stderr=${(stderr || '').slice(0, 200)}`);
      resolve({ stdout: stdout || '', stderr: stderr || '', code: code != null ? code : (err ? 1 : 0), err });
    };

    child.on('error', (err) => {
      // ENOENT etc.
      debug(`runCmd: ${bin} error: ${err.message}`);
      done(1, err);
    });

    child.on('close', (code, signal) => {
      if (signal) {
        done(1, new Error(`process killed by ${signal}`));
      } else {
        done(code, null);
      }
    });

    timer = setTimeout(() => {
      if (settled) return;
      debug(`runCmd: TIMEOUT killing process group -${child.pid}`);
      // Kill the entire process group so grandchild processes (docker) die too.
      try { process.kill(-child.pid, 'SIGKILL'); } catch (e) {
        try { child.kill('SIGKILL'); } catch (e2) {}
      }
      // Give the kill a moment to close stdio, then resolve regardless.
      setTimeout(() => {
        done(1, new Error(`command timed out after ${timeout}ms: ${bin} ${args.join(' ')}`));
      }, 500);
    }, timeout);
  });
}

// Build an environment with a PATH that includes common macOS bin locations.
// GUI apps inherit a minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin) that excludes
// /usr/local/bin, /opt/homebrew/bin, and Docker's bundled bin directory. Both
// cloma and docker (and cloma's own `exec.Command("docker", ...)`) need these.
function augmentedEnv() {
  const extra = [
    '/usr/local/bin',
    '/opt/homebrew/bin',
    '/Applications/Docker.app/Contents/Resources/bin',
    path.join(process.env.HOME || '', '.local', 'bin'),
  ];
  const base = process.env.PATH || '/usr/bin:/bin:/usr/sbin:/sbin';
  const parts = base.split(':').filter(Boolean);
  for (const e of extra) {
    if (!parts.includes(e)) parts.push(e);
  }
  return { ...process.env, PATH: parts.join(':') };
}

// Run cloma with the given args.
async function cloma(args, opts = {}) {
  const bin = clomaBin();
  const res = await runCmd(bin, args, opts);
  return res;
}

// List cloma-managed sandboxes. Returns [] on error.
async function listSandboxes() {
  const res = await cloma(['list', '--json']);
  if (res.code !== 0) {
    debug(`listSandboxes: cloma list failed code=${res.code} stderr=${(res.stderr || '').slice(0, 300)}`);
    return { error: res.stderr || res.stdout || 'failed to list sandboxes', sandboxes: [] };
  }
  try {
    const arr = JSON.parse(res.stdout);
    debug(`listSandboxes: parsed ${Array.isArray(arr) ? arr.length : 0} sandboxes`);
    return { sandboxes: Array.isArray(arr) ? arr : [], error: null };
  } catch (e) {
    debug(`listSandboxes: JSON parse error: ${e.message} stdout=${(res.stdout || '').slice(0, 300)}`);
    return { error: 'failed to parse cloma list output: ' + e.message, sandboxes: [] };
  }
}

// Check whether the cloma binary is available.
async function checkCloma() {
  const bin = clomaBin();
  const res = await runCmd(bin, ['version']);
  debug(`checkCloma: bin=${bin} code=${res.code} stderr=${(res.stderr || '').slice(0, 200)}`);
  return { ok: res.code === 0, path: bin, error: res.code !== 0 ? (res.stderr || res.stdout || 'failed to run') : null };
}

// Check whether docker is available.
async function checkDocker() {
  const bin = dockerBin();
  const res = await runCmd(bin, ['version', '--format', '{{.Server.Version}}']);
  debug(`checkDocker: bin=${bin} code=${res.code} stderr=${(res.stderr || '').slice(0, 200)}`);
  return { ok: res.code === 0, path: bin, error: res.code !== 0 ? (res.stderr || res.stdout || 'failed to run') : null };
}

// Start a sandbox: `docker sandbox exec` wakes a stopped sandbox. We run a
// trivial command so the sandbox starts without launching an agent.
async function startSandbox(name) {
  const res = await runCmd(dockerBin(), ['sandbox', 'exec', name, 'true']);
  return res;
}

// Stop a sandbox.
async function stopSandbox(name) {
  const res = await runCmd(dockerBin(), ['sandbox', 'stop', name]);
  return res;
}

// Remove a sandbox (force or not).
async function removeSandbox(name, force) {
  const args = ['sandbox', 'rm'];
  if (force) args.push('--force');
  args.push(name);
  const res = await runCmd(dockerBin(), args);
  return res;
}

// Get logs for a sandbox. `docker sandbox logs` is not a real command, so we
// stream the container logs via `docker logs`. The sandbox container name is
// the sandbox name prefixed with the docker-sandbox namespace.
async function getLogProcess(name) {
  const env = augmentedEnv();
  // Try the common sandbox container naming first.
  const candidates = [name, `docker-sandbox-${name}`, `sandbox-${name}`];
  // We'll spawn `docker logs -f <candidate>` and let the renderer pick the one
  // that works. For simplicity, try each in order.
  for (const candidate of candidates) {
    const check = await runCmd(dockerBin(), ['inspect', candidate]);
    if (check.code === 0) {
      return spawn(dockerBin(), ['logs', '-f', '--tail', '500', candidate], { env });
    }
  }
  // Fallback: use `docker sandbox logs` if the plugin supports it.
  return spawn(dockerBin(), ['sandbox', 'logs', '-f', name], { env });
}

function createTrayIcon() {
  const iconPath = path.join(__dirname, 'build', 'trayIconTemplate.png');
  const icon2x = path.join(__dirname, 'build', 'trayIconTemplate@2x.png');
  let image = nativeImage.createFromPath(iconPath);
  if (process.platform === 'darwin') {
    image.setTemplateImage(true);
  }
  return image;
}

function createWindow() {
  window = new BrowserWindow({
    width: 420,
    height: 560,
    show: false,
    frame: false,
    resizable: false,
    movable: false,
    minimizable: false,
    maximizable: false,
    fullscreenable: false,
    skipTaskbar: true,
    transparent: true,
    hasShadow: true,
    backgroundColor: '#00000000',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  window.loadFile(path.join(__dirname, 'renderer', 'index.html'));

  // Capture renderer console messages and load failures in the debug log.
  window.webContents.on('console-message', (_e, level, message, line, source) => {
    const lvl = ['LOG', 'WARN', 'ERROR'][level] || `L${level}`;
    debug(`renderer-console[${lvl}] ${source}:${line} ${message}`);
  });
  window.webContents.on('did-fail-load', (_e, code, desc, url) => {
    debug(`renderer did-fail-load: code=${code} desc=${desc} url=${url}`);
  });
  window.webContents.on('preload-error', (_e, preloadPath, error) => {
    debug(`renderer preload-error: ${preloadPath} ${error}`);
  });

  window.on('blur', () => {
    if (window && window.isVisible()) {
      window.hide();
    }
  });

  window.on('closed', () => {
    window = null;
  });
}

function showWindow() {
  if (!window) createWindow();
  const trayBounds = tray.getBounds();
  const screen = require('electron').screen;
  const display = screen.getDisplayNearestPoint({ x: trayBounds.x, y: trayBounds.y });
  const workArea = display.workArea;

  const width = 420;
  const height = 560;
  let x = Math.round(trayBounds.x + trayBounds.width / 2 - width / 2);
  let y = Math.round(trayBounds.y + trayBounds.height + 4);

  // Clamp to work area.
  if (x < workArea.x) x = workArea.x;
  if (x + width > workArea.x + workArea.width) x = workArea.x + workArea.width - width;
  if (y + height > workArea.y + workArea.height) {
    // If it would overflow the bottom, show above the tray icon.
    y = Math.round(trayBounds.y - height - 4);
  }

  window.setBounds({ x, y, width, height });
  window.show();
  window.focus();
}

function hideWindow() {
  if (window) window.hide();
}

function toggleWindow() {
  if (window && window.isVisible()) {
    hideWindow();
  } else {
    showWindow();
  }
}

function createTray() {
  const icon = createTrayIcon();
  tray = new Tray(icon);
  tray.setToolTip('cloma');

  const menu = Menu.buildFromTemplate([
    { label: 'Show cloma', click: () => showWindow() },
    { type: 'separator' },
    {
      label: 'Open Debug Log',
      click: () => {
        // Ensure the log file exists, then reveal it in Finder.
        try { fs.closeSync(fs.openSync(LOG_FILE, 'a')); } catch (e) {}
        shell.showItemInFolder(LOG_FILE);
      },
    },
    { type: 'separator' },
    {
      label: 'Quit',
      accelerator: 'Command+Q',
      click: () => {
        app.quit();
      },
    },
  ]);
  tray.setContextMenu(menu);
  tray.on('click', toggleWindow);
}

// ---- IPC handlers ----

// Renderer → main debug logging (forwarded from preload's clomaLog bridge).
ipcMain.on('renderer-log', (_event, msg) => debug(`renderer: ${msg}`));
ipcMain.on('renderer-error', (_event, msg) => debug(`renderer ERROR: ${msg}`));

ipcMain.handle('list-sandboxes', async () => {
  const { sandboxes, error } = await listSandboxes();
  return { sandboxes, error };
});

ipcMain.handle('check-prereqs', async () => {
  const clomaCheck = await checkCloma();
  const dockerCheck = await checkDocker();
  return { cloma: clomaCheck, docker: dockerCheck };
});

ipcMain.handle('start-sandbox', async (event, name) => {
  const res = await startSandbox(name);
  return { ok: res.code === 0, error: res.stderr || res.stdout };
});

ipcMain.handle('stop-sandbox', async (event, name) => {
  const res = await stopSandbox(name);
  return { ok: res.code === 0, error: res.stderr || res.stdout };
});

ipcMain.handle('delete-sandbox', async (event, name, force) => {
  const res = await removeSandbox(name, force);
  return { ok: res.code === 0, error: res.stderr || res.stdout };
});

ipcMain.handle('open-logs', async (event, name) => {
  openLogWindow(name);
  return { ok: true };
});

ipcMain.handle('refresh', async () => {
  const { sandboxes, error } = await listSandboxes();
  return { sandboxes, error };
});

ipcMain.handle('open-terminal', async (event, name) => {
  // Open a terminal running `cloma shell <name>` via macOS `open`.
  const script = `tell application "Terminal" to do script "cloma shell ${name.replace(/"/g, '\\"')}"`;
  const res = await runCmd('osascript', ['-e', script]);
  return { ok: res.code === 0, error: res.stderr };
});

// ---- Log windows ----

function openLogWindow(name) {
  if (logWindows.has(name)) {
    const w = logWindows.get(name);
    if (!w.isDestroyed()) {
      w.show();
      w.focus();
      return;
    }
    logWindows.delete(name);
  }

  const logWin = new BrowserWindow({
    width: 800,
    height: 500,
    title: `Logs — ${name}`,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      additionalArguments: [`--log-window=${name}`],
    },
  });

  logWin.loadFile(path.join(__dirname, 'renderer', 'logs.html'), { query: { name } });

  logWin.on('closed', () => {
    logWindows.delete(name);
  });

  logWindows.set(name, logWin);
}

// The log renderer asks for a log stream; we spawn it and pipe output.
const logStreams = new Map(); // name -> ChildProcess

ipcMain.handle('start-log-stream', async (event, name) => {
  // Kill any existing stream for this name.
  if (logStreams.has(name)) {
    try { logStreams.get(name).kill(); } catch (e) {}
    logStreams.delete(name);
  }

  const proc = await getLogProcess(name);
  logStreams.set(name, proc);

  const sender = event.sender;

  proc.stdout.on('data', (data) => {
    if (!sender.isDestroyed()) {
      sender.send('log-data', { name, data: data.toString(), stream: 'stdout' });
    }
  });
  proc.stderr.on('data', (data) => {
    if (!sender.isDestroyed()) {
      sender.send('log-data', { name, data: data.toString(), stream: 'stderr' });
    }
  });
  proc.on('error', (err) => {
    if (!sender.isDestroyed()) {
      sender.send('log-error', { name, error: err.message });
    }
  });
  proc.on('close', (code) => {
    if (!sender.isDestroyed()) {
      sender.send('log-close', { name, code });
    }
    logStreams.delete(name);
  });

  return { ok: true };
});

ipcMain.handle('stop-log-stream', async (event, name) => {
  if (logStreams.has(name)) {
    try { logStreams.get(name).kill(); } catch (e) {}
    logStreams.delete(name);
  }
  return { ok: true };
});

// ---- App lifecycle ----

app.whenReady().then(() => {
  debug('app ready — creating tray and window');
  debug(`clomaBin=${clomaBin()} dockerBin=${dockerBin()}`);
  debug(`PATH=${augmentedEnv().PATH}`);
  createTray();
  createWindow();

  // Refresh periodically in the background.
  setInterval(async () => {
    if (window && window.isVisible()) {
      const { sandboxes, error } = await listSandboxes();
      if (window && window.webContents) {
        window.webContents.send('sandboxes-updated', { sandboxes, error });
      }
    }
  }, 5000);
});

app.on('window-all-closed', (e) => {
  // Prevent quitting when all windows are closed — we live in the tray.
  e.preventDefault();
});

app.on('before-quit', () => {
  // Kill any lingering log streams.
  for (const [, proc] of logStreams) {
    try { proc.kill(); } catch (e) {}
  }
});