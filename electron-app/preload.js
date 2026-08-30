const { contextBridge, ipcRenderer } = require('electron');

// Forward renderer errors/logs to the main process debug log.
contextBridge.exposeInMainWorld('clomaLog', {
  log: (msg) => { try { ipcRenderer.send('renderer-log', String(msg)); } catch (e) {} },
  error: (msg) => { try { ipcRenderer.send('renderer-error', String(msg)); } catch (e) {} },
});

contextBridge.exposeInMainWorld('cloma', {
  listSandboxes: () => ipcRenderer.invoke('list-sandboxes'),
  checkPrereqs: () => ipcRenderer.invoke('check-prereqs'),
  startSandbox: (name) => ipcRenderer.invoke('start-sandbox', name),
  stopSandbox: (name) => ipcRenderer.invoke('stop-sandbox', name),
  deleteSandbox: (name, force) => ipcRenderer.invoke('delete-sandbox', name, force),
  openLogs: (name) => ipcRenderer.invoke('open-logs', name),
  openTerminal: (name) => ipcRenderer.invoke('open-terminal', name),
  refresh: () => ipcRenderer.invoke('refresh'),
  onSandboxesUpdated: (cb) => {
    const listener = (event, data) => cb(data);
    ipcRenderer.on('sandboxes-updated', listener);
    return () => ipcRenderer.removeListener('sandboxes-updated', listener);
  },
  // Log streaming (used by logs.html).
  startLogStream: (name) => ipcRenderer.invoke('start-log-stream', name),
  stopLogStream: (name) => ipcRenderer.invoke('stop-log-stream', name),
  onLogData: (cb) => {
    const listener = (event, data) => cb(data);
    ipcRenderer.on('log-data', listener);
    return () => ipcRenderer.removeListener('log-data', listener);
  },
  onLogError: (cb) => {
    const listener = (event, data) => cb(data);
    ipcRenderer.on('log-error', listener);
    return () => ipcRenderer.removeListener('log-error', listener);
  },
  onLogClose: (cb) => {
    const listener = (event, data) => cb(data);
    ipcRenderer.on('log-close', listener);
    return () => ipcRenderer.removeListener('log-close', listener);
  },
});