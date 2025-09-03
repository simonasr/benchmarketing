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
        el('td', {}, (() => {
          const btn = el('button', { class: 'exitWorker', 'data-worker-id': w.id }, 'Exit');
          btn.disabled = w.status === 'busy';
          return btn;
        })()),
      ]);
      tbody.appendChild(row);
    });
    // Update available workers info for distribution aid
    const info = document.getElementById('availableInfo');
    if (info && typeof data.available === 'number') {
      info.textContent = `Available workers: ${data.available}`;
      info.dataset.available = String(data.available);
    }
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

async function exitWorker(workerId) {
  if (!workerId) return;
  try {
    await fetchJSON(`/workers/${encodeURIComponent(workerId)}/exit`, { method: 'POST' });
    setStatus(`Exit signaled to worker ${workerId}`, 'success');
    await loadWorkers();
  } catch (e) {
    setStatus(`Failed to exit worker ${workerId}: ${e.message}`, 'error', e.details);
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
  if (e.target && e.target.id === 'distributeWorkers') { distributeWorkers(); }
  if (e.target && e.target.classList.contains('exitWorker')) { exitWorker(e.target.dataset.workerId); }
});

document.addEventListener('DOMContentLoaded', () => {
  loadWorkers();
  refreshJobStatus();
  document.getElementById('jobForm').addEventListener('submit', startJob);
  initAutoRefresh();
  initPersistence();
  initReset();
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
  const toggle = document.getElementById('autoRefreshToggle');
  if (toggle) {
    toggle.addEventListener('change', () => {
      if (toggle.checked) startAutoRefresh(REFRESH_MS); else stopAutoRefresh();
      try { localStorage.setItem(TOGGLE_KEY, toggle.checked ? '1' : '0'); } catch {}
    });
    try { const v = localStorage.getItem(TOGGLE_KEY); if (v !== null) toggle.checked = v === '1'; } catch {}
    if (!toggle.checked) stopAutoRefresh();
  }
}

// --- Persistence of form values & toggle ---
const STORAGE_KEY = 'rb_ui_v1_jobForm';
const TOGGLE_KEY = 'rb_ui_v1_autoRefresh';

function snapshotForm() {
  const targets = [];
  document.querySelectorAll('#targets .target').forEach(t => {
    targets.push({
      redisUrl: t.querySelector('input[name="redisUrl"]').value.trim(),
      redisClusterUrl: t.querySelector('input[name="redisClusterUrl"]').value.trim(),
      workerCount: t.querySelector('input[name="workerCount"]').value.trim(),
    });
  });
  return {
    targets,
    test: {
      minClients: document.getElementById('minClients').value,
      maxClients: document.getElementById('maxClients').value,
      stageIntervalMs: document.getElementById('stageIntervalMs').value,
      requestDelayMs: document.getElementById('requestDelayMs').value,
      keySize: document.getElementById('keySize').value,
      valueSize: document.getElementById('valueSize').value,
    },
    redis: {
      operationTimeoutMs: document.getElementById('operationTimeoutMs').value,
      expiration: document.getElementById('expiration').value,
    }
  };
}

function restoreForm(data) {
  if (!data || !Array.isArray(data.targets)) return;
  const container = document.getElementById('targets');
  const rows = container.querySelectorAll('.target');
  rows.forEach((row, i) => { if (i) row.remove(); });
  function getOrCreateRow(index) {
    // Ensure there are (index+1) rows by clicking Add Target as needed
    while (container.querySelectorAll('.target').length <= index) {
      const btn = document.getElementById('addTarget');
      if (btn) btn.click(); else break;
    }
    return container.querySelectorAll('.target')[index] || container.querySelector('.target');
  }
  data.targets.forEach((t, i) => {
    const row = getOrCreateRow(i);
    row.querySelector('input[name="redisUrl"]').value = t.redisUrl || '';
    row.querySelector('input[name="redisClusterUrl"]').value = t.redisClusterUrl || '';
    row.querySelector('input[name="workerCount"]').value = t.workerCount || '1';
  });
  const set = (id, val) => { const el = document.getElementById(id); if (el && val != null) el.value = val; };
  set('minClients', data.test?.minClients);
  set('maxClients', data.test?.maxClients);
  set('stageIntervalMs', data.test?.stageIntervalMs);
  set('requestDelayMs', data.test?.requestDelayMs);
  set('keySize', data.test?.keySize);
  set('valueSize', data.test?.valueSize);
  set('operationTimeoutMs', data.redis?.operationTimeoutMs);
  set('expiration', data.redis?.expiration);
}

const saveFormDebounced = (() => {
  let t; return () => { clearTimeout(t); t = setTimeout(() => {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshotForm())); } catch {}
  }, 200); };
})();

function initPersistence() {
  try { const raw = localStorage.getItem(STORAGE_KEY); if (raw) restoreForm(JSON.parse(raw)); } catch {}
  document.addEventListener('input', (e) => {
    if (e.target && e.target.closest('#jobForm')) saveFormDebounced();
  });
  document.addEventListener('click', (e) => {
    if (e.target && (e.target.id === 'addTarget' || e.target.classList.contains('removeTarget'))) {
      saveFormDebounced();
    }
  });
}

function initReset() {
  const btn = document.getElementById('resetDefaults');
  if (!btn) return;
  btn.addEventListener('click', () => {
    try { localStorage.removeItem(STORAGE_KEY); } catch {}
    // Reset to hard-coded defaults from the HTML
    document.getElementById('minClients').value = '10';
    document.getElementById('maxClients').value = '100';
    document.getElementById('stageIntervalMs').value = '1000';
    document.getElementById('requestDelayMs').value = '10';
    document.getElementById('keySize').value = '16';
    document.getElementById('valueSize').value = '1024';
    document.getElementById('operationTimeoutMs').value = '250';
    document.getElementById('expiration').value = '45';
    const container = document.getElementById('targets');
    const rows = container.querySelectorAll('.target');
    rows.forEach((row, i) => { if (i) row.remove(); });
    const first = container.querySelector('.target');
    first.querySelector('input[name="redisUrl"]').value = '';
    first.querySelector('input[name="redisClusterUrl"]').value = '';
    first.querySelector('input[name="workerCount"]').value = '1';
    setStatus('Form reset to defaults.', 'success');
  });
}

// --- Worker distribution helper ---
function distributeWorkers() {
  const info = document.getElementById('availableInfo');
  const available = info && info.dataset && info.dataset.available ? parseInt(info.dataset.available, 10) : NaN;
  const targets = Array.from(document.querySelectorAll('#targets .target'));
  const T = targets.length;
  if (!Number.isFinite(available)) {
    setStatus('Available worker count unknown. Click Refresh first.', 'error');
    return;
  }
  if (available < T) {
    // Assign 1 to as many targets as possible, 0 to the rest; user can adjust
    targets.forEach((row, i) => {
      row.querySelector('input[name="workerCount"]').value = i < available ? '1' : '0';
    });
    saveFormDebounced();
    setStatus(`Not enough workers (${available}) for ${T} targets. Assigned 1 to first ${available}.`, 'error');
    return;
  }
  const base = Math.floor(available / T);
  const rem = available % T;
  targets.forEach((row, i) => {
    const v = base + (i < rem ? 1 : 0);
    row.querySelector('input[name="workerCount"]').value = String(v);
  });
  saveFormDebounced();
  setStatus(`Distributed ${available} workers across ${T} targets.`, 'success');
}


