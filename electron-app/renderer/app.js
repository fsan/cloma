(function () {
// ---- Renderer debug logging ----
// Forward logs/errors to the main process debug log via the preload bridge.
const rlog = (m) => { try { window.clomaLog && window.clomaLog.log(m); } catch (e) {} };
const rerr = (m) => { try { window.clomaLog && window.clomaLog.error(m); } catch (e) {} };

// Catch uncaught errors so they reach the debug log instead of vanishing.
window.addEventListener('error', (e) => {
  rerr(`uncaught: ${e.message} at ${e.filename}:${e.lineno}:${e.colno}`);
});
window.addEventListener('unhandledrejection', (e) => {
  rerr(`unhandled rejection: ${e.reason && e.reason.message ? e.reason.message : String(e.reason)}`);
});

rlog('renderer app.js loaded');

const { cloma } = window;
if (!cloma) {
  rerr('window.cloma is undefined — preload bridge missing!');
}

const els = {
  list: document.getElementById('sandbox-list'),
  loading: document.getElementById('loading'),
  empty: document.getElementById('empty'),
  error: document.getElementById('error-banner'),
  prereqs: document.getElementById('prereqs'),
  prereqCloma: document.getElementById('prereq-cloma'),
  prereqDocker: document.getElementById('prereq-docker'),
  refreshBtn: document.getElementById('refresh-btn'),
  lastUpdated: document.getElementById('last-updated'),
  controls: document.getElementById('controls'),
  groupBy: document.getElementById('group-by'),
  sortBy: document.getElementById('sort-by'),
};

let busy = new Set(); // names currently being acted upon

// View state: grouping and sorting preferences (persisted across refreshes).
let groupBy = 'none';   // 'none' | 'workspace' | 'agent'
let sortBy = 'name';    // 'name' | 'created'

// Cache of the most recent sandbox data so we can re-render when the user
// changes grouping/sorting without re-fetching from the main process.
let lastSandboxes = [];

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s == null ? '' : String(s);
  return d.innerHTML;
}

function statusClass(status) {
  if (status === 'running') return 'status-running';
  if (status === 'stopped') return 'status-stopped';
  return 'status-other';
}

// Human-readable label for a status value, used in the colored badge.
function statusLabel(status) {
  if (!status) return 'unknown';
  return String(status);
}

// Group key for a sandbox given the current grouping mode. Returns '' for the
// ungrouped mode so everything lands in a single (hidden) group.
function groupKey(sb) {
  if (groupBy === 'workspace') return sb.workspace || '<unknown>';
  if (groupBy === 'agent') return sb.agent || '<unknown>';
  return '';
}

// Comparator for the current sort mode. Running sandboxes always float to the
// top within a group regardless of the sort key, so active instances are easy
// to spot.
function sortSandboxes(a, b) {
  if (a.status === 'running' && b.status !== 'running') return -1;
  if (a.status !== 'running' && b.status === 'running') return 1;
  if (sortBy === 'created') {
    const ac = a.created ? new Date(a.created).getTime() : 0;
    const bc = b.created ? new Date(b.created).getTime() : 0;
    // Newest first.
    return bc - ac;
  }
  return String(a.name).localeCompare(String(b.name));
}

function showError(msg) {
  if (!msg) {
    els.error.classList.add('hidden');
    els.error.textContent = '';
    return;
  }
  els.error.textContent = msg;
  els.error.classList.remove('hidden');
}

async function checkPrereqs() {
  try {
    rlog('checkPrereqs: invoking');
    const { cloma: clomaCheck, docker: dockerCheck } = await cloma.checkPrereqs();
    rlog(`checkPrereqs: cloma.ok=${clomaCheck.ok} docker.ok=${dockerCheck.ok}`);
    let show = false;
    if (!clomaCheck.ok) {
      els.prereqCloma.querySelector('span:last-child').textContent =
        `cloma CLI not found (tried: ${clomaCheck.path})`;
      els.prereqCloma.querySelector('.dot').className = 'dot dot-warn';
      show = true;
    } else {
      els.prereqCloma.querySelector('span:last-child').textContent =
        `cloma CLI found: ${clomaCheck.path}`;
      els.prereqCloma.querySelector('.dot').className = 'dot dot-ok';
    }
    if (!dockerCheck.ok) {
      els.prereqDocker.querySelector('span:last-child').textContent =
        `docker not found / not running (tried: ${dockerCheck.path})`;
      els.prereqDocker.querySelector('.dot').className = 'dot dot-warn';
      show = true;
    } else {
      els.prereqDocker.querySelector('span:last-child').textContent =
        `docker available: ${dockerCheck.path}`;
      els.prereqDocker.querySelector('.dot').className = 'dot dot-ok';
    }
    if (show) {
      els.prereqs.classList.remove('hidden');
    } else {
      els.prereqs.classList.add('hidden');
    }
  } catch (e) {
    rerr(`checkPrereqs failed: ${e && e.message ? e.message : String(e)}`);
  }
}

function renderSandboxItem(sb) {
  const running = sb.status === 'running';
  const name = escapeHtml(sb.name);
  const status = escapeHtml(statusLabel(sb.status));
  const ws = escapeHtml(sb.workspace || '<unknown>');
  const agent = sb.agent ? escapeHtml(sb.agent) : '';
  const created = sb.created ? new Date(sb.created).toLocaleString() : '';
  const isBusy = busy.has(sb.name);

  const metaParts = [];
  if (agent) metaParts.push(`<span class="meta-tag meta-agent">${agent}</span>`);
  if (created) metaParts.push(`<span class="meta-tag meta-created" title="${escapeHtml(sb.created)}">${escapeHtml(created)}</span>`);
  const meta = metaParts.length ? `<div class="sandbox-meta">${metaParts.join(' ')}</div>` : '';

  return `
    <li class="sandbox-item" data-name="${name}">
      <div class="sandbox-row">
        <span class="status-dot ${statusClass(sb.status)}"></span>
        <span class="sandbox-name" title="${name}">${name}</span>
        <span class="sandbox-status status-badge ${statusClass(sb.status)}">${status}</span>
      </div>
      <div class="sandbox-workspace" title="${ws}">${ws}</div>
      ${meta}
      <div class="sandbox-actions">
        <button class="btn btn-start" data-action="start" ${running || isBusy ? 'disabled' : ''}>Start</button>
        <button class="btn btn-stop" data-action="stop" ${!running || isBusy ? 'disabled' : ''}>Stop</button>
        <button class="btn" data-action="logs" ${isBusy ? 'disabled' : ''}>Logs</button>
        <button class="btn btn-delete" data-action="delete" ${isBusy ? 'disabled' : ''}>Delete</button>
        <button class="btn btn-force-delete" data-action="force-delete" ${isBusy ? 'disabled' : ''}>Force</button>
      </div>
    </li>
  `;
}

function renderSandboxes(sandboxes) {
  els.loading.classList.add('hidden');

  if (!sandboxes || sandboxes.length === 0) {
    els.list.innerHTML = '';
    els.empty.classList.remove('hidden');
    els.controls.classList.add('hidden');
    return;
  }
  els.empty.classList.add('hidden');
  els.controls.classList.remove('hidden');

  // Keep a copy for re-rendering on group/sort changes.
  lastSandboxes = sandboxes.slice();

  // Sort a copy according to the current sort mode.
  const sorted = sandboxes.slice().sort(sortSandboxes);

  if (groupBy === 'none') {
    els.list.innerHTML = sorted.map(renderSandboxItem).join('');
    return;
  }

  // Group by the selected key, preserving the sort order within each group.
  const groups = new Map();
  for (const sb of sorted) {
    const key = groupKey(sb);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(sb);
  }

  // Render groups in insertion order (which follows the sorted order of their
  // first member).
  const html = [];
  for (const [key, items] of groups) {
    const count = items.length;
    const running = items.filter((sb) => sb.status === 'running').length;
    html.push(`
      <li class="group-header">
        <span class="group-title" title="${escapeHtml(key)}">${escapeHtml(key)}</span>
        <span class="group-count">${count} sandbox${count === 1 ? '' : 'es'}${running ? ` · ${running} running` : ''}</span>
      </li>
    `);
    html.push(items.map(renderSandboxItem).join(''));
  }
  els.list.innerHTML = html.join('');
}

async function refresh() {
  els.refreshBtn.classList.add('spinning');
  try {
    rlog('refresh: start');
    await checkPrereqs();
    rlog('refresh: calling listSandboxes');
    const { sandboxes, error } = await cloma.listSandboxes();
    rlog(`refresh: got ${sandboxes ? sandboxes.length : 0} sandboxes error=${!!error}`);
    showError(error);
    renderSandboxes(sandboxes);
    els.lastUpdated.textContent = 'Updated ' + new Date().toLocaleTimeString();
  } catch (e) {
    rerr(`refresh failed: ${e && e.message ? e.message : String(e)}`);
    els.loading.classList.add('hidden');
    showError('Unexpected error: ' + (e && e.message ? e.message : String(e)));
  } finally {
    els.refreshBtn.classList.remove('spinning');
  }
}

async function withBusy(name, fn) {
  busy.add(name);
  // Disable buttons for this item immediately.
  const item = els.list.querySelector(`.sandbox-item[data-name="${cssEscape(name)}"]`);
  if (item) {
    item.querySelectorAll('button').forEach((b) => (b.disabled = true));
  }
  try {
    return await fn();
  } finally {
    busy.delete(name);
  }
}

// Minimal CSS escape for attribute selectors.
function cssEscape(s) {
  return String(s).replace(/["\\]/g, '\\$&');
}

async function handleAction(action, name) {
  try {
    switch (action) {
      case 'start': {
        await withBusy(name, () => cloma.startSandbox(name));
        break;
      }
      case 'stop': {
        await withBusy(name, () => cloma.stopSandbox(name));
        break;
      }
      case 'delete': {
        if (!confirm(`Delete sandbox "${name}"?\nThis stops and removes it permanently.`)) return;
        await withBusy(name, () => cloma.deleteSandbox(name, false));
        break;
      }
      case 'force-delete': {
        if (!confirm(`Force delete sandbox "${name}"?\nThis removes it even if it's running.`)) return;
        await withBusy(name, () => cloma.deleteSandbox(name, true));
        break;
      }
      case 'logs': {
        await cloma.openLogs(name);
        return; // don't refresh immediately
      }
    }
  } catch (e) {
    showError(`${action} failed: ` + (e && e.message ? e.message : String(e)));
  }
  await refresh();
}

// Event delegation for action buttons.
els.list.addEventListener('click', (e) => {
  const btn = e.target.closest('button[data-action]');
  if (!btn || btn.disabled) return;
  const item = btn.closest('.sandbox-item');
  if (!item) return;
  const name = item.getAttribute('data-name');
  const action = btn.getAttribute('data-action');
  handleAction(action, name);
});

// Re-render with the current data when the user changes grouping/sorting.
els.groupBy.addEventListener('change', () => {
  groupBy = els.groupBy.value;
  renderSandboxes(lastSandboxes);
});
els.sortBy.addEventListener('change', () => {
  sortBy = els.sortBy.value;
  renderSandboxes(lastSandboxes);
});

els.refreshBtn.addEventListener('click', refresh);

// Listen for background updates.
try {
  cloma.onSandboxesUpdated((data) => {
    showError(data.error);
    renderSandboxes(data.sandboxes);
    els.lastUpdated.textContent = 'Updated ' + new Date().toLocaleTimeString();
  });
  rlog('onSandboxesUpdated listener registered');
} catch (e) {
  rerr(`onSandboxesUpdated failed: ${e && e.message ? e.message : String(e)}`);
}

// Initial load.
rlog('calling initial refresh()');
refresh().catch((e) => rerr(`initial refresh rejected: ${e && e.message ? e.message : String(e)}`));
})();