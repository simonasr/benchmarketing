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

// Simple debounce helper for readability
function debounce(fn, delayMs) {
  let timerId;
  return (...args) => {
    clearTimeout(timerId);
    timerId = setTimeout(() => fn.apply(null, args), delayMs);
  };
}

// --- Sorting state for Workers table ---
const SORT_STORAGE_KEY = 'rb_ui_v1_workersSort';
let workersSort = { key: 'id', dir: 'asc' }; // dir: 'asc' | 'desc'

function loadWorkersSort() {
  try {
    const raw = localStorage.getItem(SORT_STORAGE_KEY);
    if (raw) {
      const obj = JSON.parse(raw);
      if (obj && typeof obj.key === 'string' && (obj.dir === 'asc' || obj.dir === 'desc')) {
        workersSort = obj;
      }
    }
  } catch {}
}

function saveWorkersSort() {
  try { localStorage.setItem(SORT_STORAGE_KEY, JSON.stringify(workersSort)); } catch {}
}

function compareValues(a, b, key) {
  const va = a?.[key];
  const vb = b?.[key];
  // Numeric compare when both are finite numbers
  if (Number.isFinite(va) && Number.isFinite(vb)) return va - vb;
  // Date compare for lastSeen if parseable
  if (key === 'lastSeen') {
    const da = new Date(va).getTime();
    const db = new Date(vb).getTime();
    if (Number.isFinite(da) && Number.isFinite(db)) return da - db;
  }
  // String compare fallback
  return String(va || '').localeCompare(String(vb || ''));
}

function sortWorkersData(data) {
  if (!workersSort?.key) return data;
  const key = workersSort.key;
  const dir = workersSort.dir === 'desc' ? -1 : 1;
  const copy = data.slice();
  copy.sort((a, b) => compareValues(a, b, key) * dir);
  return copy;
}

function updateHeaderSortIndicators() {
  const headers = document.querySelectorAll('#workersTable thead th.sortable');
  headers.forEach(h => {
    const k = h.getAttribute('data-key');
    const isActive = k === workersSort.key;
    h.setAttribute('aria-sort', isActive ? (workersSort.dir === 'desc' ? 'descending' : 'ascending') : 'none');
  });
}

function initWorkersHeaderSorting() {
  const thead = document.querySelector('#workersTable thead');
  if (!thead) return;
  const onActivate = (th) => {
    const key = th.getAttribute('data-key');
    if (!key) return;
    if (workersSort.key === key) {
      workersSort.dir = workersSort.dir === 'asc' ? 'desc' : 'asc';
    } else {
      workersSort.key = key;
      workersSort.dir = 'asc';
    }
    saveWorkersSort();
    updateHeaderSortIndicators();
    loadWorkers();
  };
  thead.addEventListener('click', (e) => {
    const th = e.target && e.target.closest('th.sortable');
    if (th) onActivate(th);
  });
  thead.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      const th = e.target && e.target.closest('th.sortable');
      if (th) { e.preventDefault(); onActivate(th); }
    }
  });
}

async function loadWorkers() {
  try {
    const data = await fetchJSON('/workers');
    const tbody = document.querySelector('#workersTable tbody');
    tbody.innerHTML = '';
    const workers = sortWorkersData(Array.isArray(data.workers) ? data.workers : []);
    workers.forEach(w => {
      const row = el('tr', {}, [
        el('td', { text: w.id }),
        el('td', { text: w.address }),
        el('td', { text: String(w.port) }),
        el('td', {}, el('span', { class: w.status, text: w.status })),
        el('td', { text: new Date(w.lastSeen).toLocaleString() }),
        el('td', { text: w.currentJob || '' }),
        el('td', {}, (() => {
          const btn = el('button', { class: 'exitWorker', 'data-worker-id': w.id, 'data-worker-status': w.status }, 'Exit');
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
    updateHeaderSortIndicators();
    // Predictions may depend on worker distribution inputs; refresh display
    updatePredictions();
  } catch (e) {
    console.error(e);
    setStatus(`Failed to load workers: ${e.message}`, 'error', e.details);
  }
}

function serializeJobForm() {
  const targets = [];
  document.querySelectorAll('#targets .target').forEach(t => {
    const redisUrl = t.querySelector('input[name="redisUrl"]').value.trim();
    const clusterUrl = t.querySelector('input[name="clusterUrl"]').value.trim();
    const workerCount = parseInt(t.querySelector('input[name="workerCount"]').value, 10) || 1;
    if (!redisUrl && !clusterUrl) return;
    targets.push({ redisUrl, clusterUrl, workerCount });
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
    if (e && e.status === 409) {
      setStatus('Another job is already running. Stop it before starting a new one.', 'error');
      setJobControlsState(true);
    } else {
      setStatus(`Failed to start job: ${e.message}`, 'error', e.details);
    }
  }
}

async function stopJob() {
  try {
    const job = await fetchJSON('/job/stop', { method: 'DELETE' });
    setStatus(`Job stopped: ${job.id}`);
    await refreshJobStatus();
  } catch (e) {
    if (e && e.status === 404) {
      setStatus('No active job to stop.', 'error');
      setJobControlsState(false);
    } else {
      setStatus(`Failed to stop job: ${e.message}`, 'error', e.details);
    }
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
    const pre = document.getElementById('jobStatusJson');
    const text = typeof res === 'object' ? JSON.stringify(res, null, 2) : String(res);
    if (pre) {
      pre.textContent = text;
    } else {
      const box = document.getElementById('jobStatus');
      if (box) box.textContent = text;
    }
    // Toggle controls based on job running state
    try {
      const st = res && typeof res === 'object' ? res.status : undefined;
      setJobControlsState(st === 'running');
    } catch (_) { /* ignore */ }
  } catch (e) {
    const pre = document.getElementById('jobStatusJson');
    if (pre) pre.textContent = '';
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

// Enable/disable Start/Stop depending on whether a job is running
function setJobControlsState(isRunning) {
  const startBtn = document.getElementById('startJob');
  const stopBtn = document.getElementById('stopJob');
  if (startBtn) {
    startBtn.disabled = !!isRunning;
    startBtn.classList.toggle('hidden', !!isRunning);
  }
  if (stopBtn) {
    stopBtn.disabled = !isRunning;
    stopBtn.classList.toggle('hidden', !isRunning);
  }
  // Optionally lock target inputs while running to avoid confusion
  try {
    const form = document.getElementById('jobForm');
    if (form) {
      const inputs = form.querySelectorAll('input, select, button');
      inputs.forEach(el => {
        if (el.id === 'stopJob' || el.id === 'resetDefaults') return; // keep stop/reset usable
        if (el.id === 'startJob') { el.disabled = !!isRunning; return; }
        // Keep other controls editable when not running; disable while running
        el.disabled = !!isRunning;
      });
      // Re-enable non-start controls we want active while running
      const resetBtn = document.getElementById('resetDefaults');
      if (resetBtn) resetBtn.disabled = false;
      if (stopBtn) stopBtn.disabled = !isRunning;
    }
  } catch (_) { /* ignore */ }
}

function addTargetRow() {
  const tpl = document.querySelector('#targets .target');
  const clone = tpl.cloneNode(true);
  clone.querySelectorAll('input').forEach(i => i.value = i.name === 'workerCount' ? '1' : '');
  document.getElementById('targets').appendChild(clone);
  updatePredictions();
}

function removeTargetRow(btn) {
  const rows = document.querySelectorAll('#targets .target');
  if (rows.length <= 1) return;
  btn.closest('.target').remove();
  updatePredictions();
}

document.addEventListener('click', (e) => {
  if (e.target && e.target.id === 'addTarget') { addTargetRow(); }
  if (e.target && e.target.classList.contains('removeTarget')) { removeTargetRow(e.target); }
  if (e.target && e.target.id === 'refreshWorkers') { loadWorkers(); }
  if (e.target && e.target.id === 'stopJob') { stopJob(); }
  if (e.target && e.target.id === 'distributeWorkers') { distributeWorkers(); }
  if (e.target && e.target.classList.contains('exitWorker')) {
    const workerId = e.target.dataset.workerId;
    const status = e.target.dataset.workerStatus;
    if (!workerId) return;
    if (status === 'busy') {
      const ok = window.confirm(`Worker ${workerId} is busy. Force-exit? This may interrupt the job.`);
      if (!ok) return;
    }
    exitWorker(workerId);
  }
});

document.addEventListener('DOMContentLoaded', () => {
  loadWorkersSort();
  loadWorkers();
  refreshJobStatus();
  document.getElementById('jobForm').addEventListener('submit', startJob);
  initAutoRefresh();
  initPersistence();
  initReset();
  initWorkersHeaderSorting();
  // Live predictions on any change within the form
  const updatePredictionsDebounced = debounce(updatePredictions, 150);
  document.getElementById('jobForm').addEventListener('input', (e) => {
    if (e && e.target) updatePredictionsDebounced();
  });
  document.getElementById('jobForm').addEventListener('change', (e) => {
    if (e && e.target) updatePredictionsDebounced();
  });
  updatePredictions();
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
      clusterUrl: t.querySelector('input[name="clusterUrl"]').value.trim(),
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
    },
    assumptions: {
      latencyMs: document.getElementById('assumedLatencyMs')?.value,
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
    row.querySelector('input[name="clusterUrl"]').value = (t.clusterUrl || '') ;
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
  set('assumedLatencyMs', data.assumptions?.latencyMs);
  updatePredictions();
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
    const latencyEl = document.getElementById('assumedLatencyMs');
    if (latencyEl) latencyEl.value = '0.5';
    const container = document.getElementById('targets');
    const rows = container.querySelectorAll('.target');
    rows.forEach((row, i) => { if (i) row.remove(); });
    const first = container.querySelector('.target');
    first.querySelector('input[name="redisUrl"]').value = '';
    first.querySelector('input[name="clusterUrl"]').value = '';
    first.querySelector('input[name="workerCount"]').value = '1';
    setStatus('Form reset to defaults.', 'success');
    updatePredictions();
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
    updatePredictions();
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
  updatePredictions();
}


// --- Predictions (duration and theoretical request rates) ---
// Defaults and numeric floors to avoid magic numbers and division-by-zero
const DEFAULT_ASSUMED_LATENCY_MS = 0.5; // Default avg Redis latency per op when input missing/invalid
const MIN_GOROUTINE_MS = 1;            // Floor for per-goroutine window (ms) to avoid zero
const MIN_PER_ITERATION_MS = 0.000001; // Smallest allowed iteration window to prevent division by zero
const MS_PADDING_LENGTH = 3;           // Zero-padding length for milliseconds in duration formatting
const OPS_PER_GOROUTINE = 2;           // Number of Redis ops per goroutine window (save + get)
const OPS_PER_ITERATION = 2;           // Number of Redis ops per iteration (save + get)
function formatNumber(n) {
  try { return Number.isFinite(n) ? n.toLocaleString(undefined, { maximumFractionDigits: 2 }) : '-'; } catch { return String(n); }
}

function formatDuration(ms) {
  if (!Number.isFinite(ms) || ms < 0) return '-';
  const s = Math.floor(ms / 1000);
  const msRem = ms % 1000;
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}h ${m}m ${sec}s`;
  if (m > 0) return `${m}m ${sec}s`;
  if (s > 0) return `${s}.${String(msRem).padStart(MS_PADDING_LENGTH, '0')}s`;
  return `${ms}ms`;
}

function computePredictionsPerTarget() {
  const minC = parseInt(document.getElementById('minClients').value, 10);
  const maxC = parseInt(document.getElementById('maxClients').value, 10);
  const stageMs = parseInt(document.getElementById('stageIntervalMs').value, 10);
  const reqDelayMs = parseInt(document.getElementById('requestDelayMs').value, 10);
  if (!Number.isFinite(minC) || !Number.isFinite(maxC) || !Number.isFinite(stageMs) || minC < 1 || maxC < 1 || stageMs < 1 || maxC < minC) {
    return { message: 'Provide valid test config to see predictions.' };
  }

  const stages = (maxC - minC + 1);
  const durationMs = stages * stageMs;

  // Upper bound per goroutine time window: use requestDelay only (ignore op timeout)
  const perGoroutineMs = Math.max(MIN_GOROUTINE_MS, (Number.isFinite(reqDelayMs) ? reqDelayMs : 0));
  const perGoroutineSec = perGoroutineMs / 1000;

  const items = [];
  const rows = document.querySelectorAll('#targets .target');
  const assumedLatencyEl = document.getElementById('assumedLatencyMs');
  let assumedOpMs = parseFloat(assumedLatencyEl && assumedLatencyEl.value);
  if (!Number.isFinite(assumedOpMs) || assumedOpMs < 0) assumedOpMs = DEFAULT_ASSUMED_LATENCY_MS;
  rows.forEach((t, idx) => {
    const redisUrl = t.querySelector('input[name="redisUrl"]').value.trim();
    const clusterUrl = t.querySelector('input[name="clusterUrl"]').value.trim();
    const label = clusterUrl || redisUrl;
    const wc = parseInt(t.querySelector('input[name="workerCount"]').value, 10);
    if (!label) return; // skip rows without a target
    if (!Number.isFinite(wc) || wc <= 0) return; // skip non-positive worker counts
    const peakConcurrency = maxC * wc;
    const peakRpsUpperBound = (OPS_PER_GOROUTINE * maxC / perGoroutineSec) * wc;
    // RPS assuming average Redis latency per op (two ops/iteration)
    const perIterationMs = Math.max(
      MIN_PER_ITERATION_MS,
      (assumedOpMs * OPS_PER_ITERATION) + (Number.isFinite(reqDelayMs) ? reqDelayMs : 0)
    );
    const rpsAt05ms = (1000 / perIterationMs) * maxC * wc;
    items.push({ label, workerCount: wc, peakConcurrency, peakRpsUpperBound, rpsAt05ms, assumedOpMs, index: idx + 1 });
  });

  if (!items.length) {
    return { message: 'Add at least one valid target with worker count to see predictions.' };
  }

  return { durationMs, items };
}

function updatePredictions() {
  const box = document.getElementById('predictionInfo');
  if (!box) return;
  const p = computePredictionsPerTarget();
  if (p.message) {
    box.classList.remove('success', 'error');
    box.classList.add('info');
    box.textContent = p.message;
    return;
  }
  const lines = [ `Estimated duration: ${formatDuration(p.durationMs)}` ];
  p.items.forEach(item => {
    const parts = [
      `${item.label} —`,
      `workers: ${formatNumber(item.workerCount)}`,
      `peak concurrency: ${formatNumber(item.peakConcurrency)}`,
      `peak req/s (upper): ${formatNumber(item.peakRpsUpperBound)}`,
      `est req/s @${formatNumber(item.assumedOpMs)}ms: ${formatNumber(item.rpsAt05ms)}`,
    ];
    lines.push(parts.join(' | '));
  });
  box.classList.remove('error', 'info');
  box.classList.add('success');
  box.textContent = lines.join('\n');
}

