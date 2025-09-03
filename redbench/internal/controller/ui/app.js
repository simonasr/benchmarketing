async function fetchJSON(path, options = {}) {
  const res = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...options });
  if (!res.ok) {
    const err = await parseErrorResponse(res);
    throw err;
  }
  return res.json();
}

async function parseErrorResponse(res) {
  let bodyText = '';
  let bodyJson = null;
  try {
    const ct = res.headers.get('Content-Type') || '';
    if (ct.includes('application/json')) {
      bodyJson = await res.json();
    } else {
      bodyText = await res.text();
    }
  } catch (_) {
    // ignore
  }

  const details = bodyJson || (bodyText ? { message: bodyText } : null);
  const message = deriveErrorMessage(details, res.status, res.statusText);
  const error = new Error(message);
  error.status = res.status;
  error.statusText = res.statusText;
  error.details = details;
  return error;
}

function deriveErrorMessage(details, status, statusText) {
  if (details && typeof details === 'object') {
    if (typeof details.error === 'string' && details.error) return details.error;
    if (typeof details.message === 'string' && details.message) return details.message;
  }
  return `${status} ${statusText}`;
}

function el(tag, attrs = {}, children = []) {
  const e = document.createElement(tag);
  Object.entries(attrs).forEach(([k, v]) => {
    if (k === 'class') e.className = v; else if (k === 'text') e.textContent = v; else e.setAttribute(k, v);
  });
  (Array.isArray(children) ? children : [children]).filter(Boolean).forEach(c => e.appendChild(typeof c === 'string' ? document.createTextNode(c) : c));
  return e;
}

async function loadWorkers() {
  try {
    const data = await fetchJSON('/workers');
    const tbody = document.querySelector('#workersTable tbody');
    tbody.innerHTML = '';
    data.workers.forEach(w => {
      const row = el('tr', {}, [
        el('td', { text: w.id }),
        el('td', { text: w.address }),
        el('td', { text: String(w.port) }),
        el('td', {}, el('span', { class: w.status, text: w.status })),
        el('td', { text: new Date(w.lastSeen).toLocaleString() }),
        el('td', { text: w.currentJob || '' }),
      ]);
      tbody.appendChild(row);
    });
  } catch (e) {
    console.error(e);
    setStatus(`Failed to load workers: ${e.message}`, 'error', e.details);
  }
}

function serializeJobForm() {
  const targets = [];
  document.querySelectorAll('#targets .target').forEach(t => {
    const redisUrl = t.querySelector('input[name="redisUrl"]').value.trim();
    const redisClusterUrl = t.querySelector('input[name="redisClusterUrl"]').value.trim();
    const workerCount = parseInt(t.querySelector('input[name="workerCount"]').value, 10) || 1;
    if (!redisUrl && !redisClusterUrl) return;
    targets.push({ redisUrl, redisClusterUrl, workerCount });
  });

  const config = {
    test: {
      minClients: parseInt(document.getElementById('minClients').value, 10),
      maxClients: parseInt(document.getElementById('maxClients').value, 10),
      stageIntervalMs: parseInt(document.getElementById('stageIntervalMs').value, 10),
      requestDelayMs: parseInt(document.getElementById('requestDelayMs').value, 10),
      keySize: parseInt(document.getElementById('keySize').value, 10),
      valueSize: parseInt(document.getElementById('valueSize').value, 10),
    },
    redis: {
      operationTimeoutMs: parseInt(document.getElementById('operationTimeoutMs').value, 10),
      expiration: parseInt(document.getElementById('expiration').value, 10),
    }
  };

  return { targets, config };
}

async function startJob(evt) {
  evt.preventDefault();
  const payload = serializeJobForm();
  if (!payload.targets.length) {
    setStatus('Please provide at least one target.');
    return;
  }
  try {
    const job = await fetchJSON('/job/start', { method: 'POST', body: JSON.stringify(payload) });
    setStatus(`Job started: ${job.id}`);
    await refreshJobStatus();
  } catch (e) {
    setStatus(`Failed to start job: ${e.message}`, 'error', e.details);
  }
}

async function stopJob() {
  try {
    const job = await fetchJSON('/job/stop', { method: 'DELETE' });
    setStatus(`Job stopped: ${job.id}`);
    await refreshJobStatus();
  } catch (e) {
    setStatus(`Failed to stop job: ${e.message}`, 'error', e.details);
  }
}

async function refreshJobStatus() {
  try {
    const res = await fetchJSON('/job/status');
    const box = document.getElementById('jobStatus');
    box.textContent = typeof res === 'object' ? JSON.stringify(res, null, 2) : String(res);
  } catch (e) {
    setStatus(`Failed to fetch job status: ${e.message}`, 'error', e.details);
  }
}

function setStatus(msg, level = 'info', details) {
  const box = document.getElementById('jobStatus');
  box.classList.remove('info', 'success', 'error');
  box.classList.add(level || 'info');
  let text = msg || '';
  if (details && typeof details === 'object') {
    try {
      const pretty = JSON.stringify(details, null, 2);
      if (pretty && pretty !== '""') {
        text += `\n${pretty}`;
      }
    } catch (_) { /* ignore */ }
  }
  box.textContent = text;
}

function addTargetRow() {
  const tpl = document.querySelector('#targets .target');
  const clone = tpl.cloneNode(true);
  clone.querySelectorAll('input').forEach(i => i.value = i.name === 'workerCount' ? '1' : '');
  document.getElementById('targets').appendChild(clone);
}

function removeTargetRow(btn) {
  const rows = document.querySelectorAll('#targets .target');
  if (rows.length <= 1) return;
  btn.closest('.target').remove();
}

document.addEventListener('click', (e) => {
  if (e.target && e.target.id === 'addTarget') { addTargetRow(); }
  if (e.target && e.target.classList.contains('removeTarget')) { removeTargetRow(e.target); }
  if (e.target && e.target.id === 'refreshWorkers') { loadWorkers(); }
  if (e.target && e.target.id === 'stopJob') { stopJob(); }
});

document.addEventListener('DOMContentLoaded', () => {
  loadWorkers();
  refreshJobStatus();
  document.getElementById('jobForm').addEventListener('submit', startJob);
  initAutoRefresh();
});

// --- Auto-refresh every 1s with visibility pause and simple backoff ---
const REFRESH_MS = 1000;
let refreshTimer = null;
let failureCount = 0;

async function tickRefresh() {
  try {
    await Promise.all([loadWorkers(), refreshJobStatus()]);
    failureCount = 0;
  } catch (_) {
    failureCount += 1;
  }
}

function startAutoRefresh(intervalMs = REFRESH_MS) {
  stopAutoRefresh();
  refreshTimer = setInterval(tickRefresh, intervalMs);
}

function stopAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
}

function initAutoRefresh() {
  startAutoRefresh(REFRESH_MS);
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) stopAutoRefresh(); else startAutoRefresh(REFRESH_MS);
  });
}


