(function () {
const { cloma } = window;

const params = new URLSearchParams(window.location.search);
const name = params.get('name') || 'sandbox';

const output = document.getElementById('log-output');
const status = document.getElementById('log-status');
const title = document.getElementById('log-title');
const clearBtn = document.getElementById('clear-btn');

title.textContent = `Logs — ${name}`;

let atBottom = true;

output.addEventListener('scroll', () => {
  atBottom = output.scrollHeight - output.scrollTop - output.clientHeight < 30;
});

function appendLine(text, stream) {
  const span = document.createElement('span');
  span.className = stream === 'stderr' ? 'log-line-stderr' : '';
  span.textContent = text;
  output.appendChild(span);
  if (atBottom) {
    output.scrollTop = output.scrollHeight;
  }
}

function setStatus(text, cls) {
  status.textContent = text;
  status.className = 'log-status ' + (cls || '');
}

clearBtn.addEventListener('click', () => {
  output.innerHTML = '';
});

// Listen for log data.
cloma.onLogData((data) => {
  if (data.name !== name) return;
  appendLine(data.data, data.stream);
  setStatus('streaming', 'connected');
});

cloma.onLogError((data) => {
  if (data.name !== name) return;
  appendLine(`\n[error] ${data.error}\n`, 'stderr');
  setStatus('error: ' + data.error, 'error');
});

cloma.onLogClose((data) => {
  if (data.name !== name) return;
  appendLine(`\n[stream closed, exit code ${data.code}]\n`, 'stderr');
  setStatus('disconnected', 'disconnected');
});

// Start streaming.
(async () => {
  setStatus('connecting…', 'disconnected');
  try {
    const res = await cloma.startLogStream(name);
    if (!res.ok) {
      setStatus('failed to start stream', 'error');
    }
  } catch (e) {
    setStatus('error: ' + (e && e.message ? e.message : e), 'error');
  }
})();

// Stop the stream when the window is closed.
window.addEventListener('beforeunload', () => {
  cloma.stopLogStream(name);
});
})();