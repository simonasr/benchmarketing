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
    // Pre-compute target mismatch vs current form
    const currentTargets = (buildCurrentDtoFromForm()?.targets || []).reduce((m, t) => { const k = t.clusterUrl || t.redisUrl; if (k) m[k] = (m[k]||0) + (Number.isFinite(t.workerCount)?t.workerCount:0); return m; }, {});
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
      // Subtle highlight if busy and likely mismatch: we only know label via current form; if current job target label not in current form, mark
      try {
        if (w.status === 'busy' && w.currentJob) {
          const label = String(w.currentJob).trim();
          if (label && !(label in currentTargets)) {
            row.classList.add('mismatch');
            row.title = 'Worker target likely differs from current form';
          }
        }
      } catch (_) {}
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
    // Immediately re-check once to catch fast-fail and overwrite optimistic message
    await refreshJobStatus(true);
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

async function refreshJobStatus(isImmediateCheck = false) {
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
      // Promote fail/success to status box message
      if (st === 'failed') {
        const msg = res?.errorMessage || 'Job failed';
        setStatus(msg, 'error');
        flashJobPanel('error');
      } else if (st === 'completed') {
        setStatus('Job completed', 'success');
        flashJobPanel('success');
      } else if (st === 'running' && isImmediateCheck) {
        // On immediate recheck, keep the optimistic started message
      } else if (!st || st === 'no_jobs') {
      }
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

// Visual flash cue on panel head for success/error
function flashJobPanel(kind) {
  try {
    const panel = document.getElementById('job');
    if (!panel) return;
    const head = panel.querySelector('.panel-head');
    if (!head) return;
    // No visual flash anymore as per request; leave hook empty for future tweaks
  } catch(_) {}
}

// Enable/disable Start/Stop depending on whether a job is running
function setJobControlsState(isRunning) {
  const startBtn = document.getElementById('startJob');
  const stopBtn = document.getElementById('stopJob');
  const resetBtn = document.getElementById('resetDefaults');

  // Set start button state
  if (startBtn) {
    startBtn.disabled = !!isRunning;
    startBtn.classList.toggle('hidden', !!isRunning);
  }

  // Set stop button state
  if (stopBtn) {
    stopBtn.disabled = !isRunning;
    stopBtn.classList.toggle('hidden', !isRunning);
  }

  // Set reset button state (always enabled)
  if (resetBtn) {
    resetBtn.disabled = false;
  }

  // Optionally lock target inputs while running to avoid confusion
  try {
    const form = document.getElementById('jobForm');
    if (form) {
      const inputs = form.querySelectorAll('input, select, button');
      inputs.forEach(el => {
        // Only disable controls that are not start/stop/reset buttons
        if (el.id === 'startJob' || el.id === 'stopJob' || el.id === 'resetDefaults') {
          return; // Their state is managed above
        }
        el.disabled = !!isRunning;
      });
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
  initRuntimeConfigImport();
  // Live predictions on any change within the form
  const updatePredictionsDebounced = debounce(updatePredictions, 150);
  const updateImportBtnDebounced = debounce(updateImportConfigButtonState, 250);
  document.getElementById('jobForm').addEventListener('input', (e) => {
    if (e && e.target) updatePredictionsDebounced();
    if (e && e.target) updateImportBtnDebounced();
  });
  document.getElementById('jobForm').addEventListener('change', (e) => {
    if (e && e.target) updatePredictionsDebounced();
    if (e && e.target) updateImportBtnDebounced();
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
  Array.from(rows).slice(1).forEach(row => row.remove());
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
  Array.from(rows).slice(1).forEach(row => row.remove());
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

// --- Runtime Config Import (single button flow) ---
const RUNTIME_CACHE_TTL_MS = 2000; // short TTL to avoid excessive calls while staying fresh
let runtimeConfigCache = { dto: null, ts: 0 };

async function checkRuntimeConfigAvailable() {
  try {
    const res = await fetch('/api/v1/runtime-config', { method: 'HEAD' });
    return res.status === 204;
  } catch (_) {
    return false;
  }
}

async function fetchRuntimeConfig(opts) {
  const force = !!(opts && opts.force);
  const now = Date.now();
  if (!force && runtimeConfigCache.dto && (now - runtimeConfigCache.ts) < RUNTIME_CACHE_TTL_MS) {
    return runtimeConfigCache.dto;
  }
  const dto = await fetchJSON('/api/v1/runtime-config');
  runtimeConfigCache = { dto, ts: now };
  return dto;
}

function renderRuntimePreview(dto) {
  const preInline = document.getElementById('runtimeConfigJson');
  const details = document.getElementById('runtimeConfigPreview');
  const modal = document.getElementById('importModal');
  const curPre = document.getElementById('importModalCurrent');
  const incPre = document.getElementById('importModalIncoming');
  const diffPre = document.getElementById('importModalDiff');
  const copy = { version: dto.version, updatedAt: dto.updatedAt, config: dto.config, targets: dto.targets };
  const incomingText = JSON.stringify(copy, null, 2);

  // Build current snapshot from the form
  const current = buildCurrentDtoFromForm();

  const currentText = JSON.stringify(current, null, 2);
  const diffText = prettyKeyDiff(current, copy);

  if (modal && curPre && incPre && diffPre) {
    curPre.textContent = currentText;
    incPre.textContent = incomingText;
    diffPre.textContent = diffText || 'No changes.';
    openImportModal();
  } else if (preInline && details) {
    preInline.textContent = incomingText;
    details.classList.remove('hidden');
  }
}

function applyRuntimeConfigToForm(dto, mode = 'merge') {
  if (!dto || !dto.config) return;
  const cfg = dto.config;
  const setIfNumber = (id, val) => {
    if (Number.isFinite(val)) {
      const el = document.getElementById(id);
      if (el) el.value = String(val);
    }
  };
  setIfNumber('minClients', cfg.test?.minClients);
  setIfNumber('maxClients', cfg.test?.maxClients);
  setIfNumber('stageIntervalMs', cfg.test?.stageIntervalMs);
  setIfNumber('requestDelayMs', cfg.test?.requestDelayMs);
  setIfNumber('keySize', cfg.test?.keySize);
  setIfNumber('valueSize', cfg.test?.valueSize);
  setIfNumber('operationTimeoutMs', cfg.redis?.operationTimeoutMs);
  if (Number.isFinite(cfg.redis?.expiration)) {
    const el = document.getElementById('expiration');
    if (el) el.value = String(cfg.redis.expiration);
  }
  updatePredictions();
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshotForm())); } catch {}
}

function initRuntimeConfigImport() {
  const importBtn = document.getElementById('importConfigBtn');
  const applyBtn = document.getElementById('applyRuntimeConfigBtn');
  const confirmBtn = document.getElementById('confirmImportBtn');
  const cancelBtn = document.getElementById('cancelImportBtn');
  const closeBtn = document.getElementById('closeImportModal');
  const backdrop = document.getElementById('importModalBackdrop');

  (async () => {
    const available = await checkRuntimeConfigAvailable();
    if (importBtn) importBtn.disabled = !available;
    if (available) updateImportConfigButtonState();
  })();

  if (importBtn) {
    importBtn.addEventListener('click', async () => {
      try {
        const dto = await fetchRuntimeConfig({ force: true });
        renderRuntimePreview(dto);
        setStatus('Loaded runtime config preview.', 'success');
      } catch (e) {
        setStatus(`Failed to load runtime config: ${e.message}`, 'error', e.details);
      }
    });
  }

  if (applyBtn) {
    applyBtn.addEventListener('click', async () => {
      try { await performImportAndClose(); } catch (_) {}
    });
  }

  if (confirmBtn) {
    confirmBtn.addEventListener('click', async () => {
      try { await performImportAndClose(); } catch (_) {}
    });
  }

  if (cancelBtn) cancelBtn.addEventListener('click', closeImportModal);
  if (closeBtn) closeBtn.addEventListener('click', closeImportModal);
  if (backdrop) backdrop.addEventListener('click', closeImportModal);
}

function openImportModal() {
  const modal = document.getElementById('importModal');
  if (!modal) return;
  modal.classList.remove('hidden');
  document.body.classList.add('modal-open');
  // Focus the dialog close for accessibility
  const closeBtn = document.getElementById('closeImportModal');
  if (closeBtn) { try { closeBtn.focus(); } catch(_) {} }
  // Focus trap and ESC/Enter handlers
  const focusable = modal.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  function onKeydown(e) {
    if (e.key === 'Escape') { e.preventDefault(); closeImportModal(); return; }
    if (e.key === 'Enter') { const confirm = document.getElementById('confirmImportBtn'); if (confirm && !confirm.disabled) { e.preventDefault(); confirm.click(); } }
    if (e.key === 'Tab') {
      if (focusable.length === 0) return;
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
    }
  }
  modal.addEventListener('keydown', onKeydown);
  modal.dataset.trap = '1';
  // store handler reference for cleanup
  modal._keydownHandler = onKeydown;
}

function closeImportModal() {
  const modal = document.getElementById('importModal');
  if (!modal) return;
  modal.classList.add('hidden');
  document.body.classList.remove('modal-open');
  const trigger = document.getElementById('importConfigBtn');
  if (trigger) { try { trigger.focus(); } catch(_) {} }
  if (modal.dataset.trap === '1') {
    try {
      if (modal._keydownHandler) {
        modal.removeEventListener('keydown', modal._keydownHandler);
        delete modal._keydownHandler;
      }
    } catch(_) {}
    delete modal.dataset.trap;
  }
}

// Compute a human-readable diff focusing on config.test, config.redis and targets workerCount
function prettyKeyDiff(current, incoming) {
  try {
    const lines = [];
    function cmp(path, a, b) {
      if (a === b) return;
      lines.push(`${path}: ${JSON.stringify(a)} -> ${JSON.stringify(b)}`);
    }
    const ct = current?.config?.test || {};
    const it = incoming?.config?.test || {};
    cmp('test.minClients', ct.minClients, it.minClients);
    cmp('test.maxClients', ct.maxClients, it.maxClients);
    cmp('test.stageIntervalMs', ct.stageIntervalMs, it.stageIntervalMs);
    cmp('test.requestDelayMs', ct.requestDelayMs, it.requestDelayMs);
    cmp('test.keySize', ct.keySize, it.keySize);
    cmp('test.valueSize', ct.valueSize, it.valueSize);
    const cr = current?.config?.redis || {};
    const ir = incoming?.config?.redis || {};
    cmp('redis.operationTimeoutMs', cr.operationTimeoutMs, ir.operationTimeoutMs);
    cmp('redis.expiration', cr.expiration, ir.expiration);
    const ctgs = Array.isArray(current?.targets) ? current.targets : [];
    const itgs = Array.isArray(incoming?.targets) ? incoming.targets : [];
    // Map by label for comparison
    const mapT = (arr) => {
      const m = {};
      arr.forEach(t => {
        const key = t.clusterUrl || t.redisUrl || '';
        if (!key) return;
        m[key] = (m[key] || 0) + (Number.isFinite(t.workerCount) ? t.workerCount : 0);
      });
      return m;
    };
    const mC = mapT(ctgs);
    const mI = mapT(itgs);
    const labels = new Set([...Object.keys(mC), ...Object.keys(mI)]);
    labels.forEach(k => cmp(`targets[${k}].workerCount`, mC[k] || 0, mI[k] || 0));
    return lines.join('\n');
  } catch (_) {
    return '';
  }
}

// Determine if there are any meaningful changes between current form state and incoming DTO
async function updateImportConfigButtonState() {
  try {
    const btn = document.getElementById('importConfigBtn');
    if (!btn || btn.disabled) return; // disabled due to availability
    const dto = await fetchRuntimeConfig();
    const current = buildCurrentDtoFromForm();
    const hasDiff = hasMeaningfulDiff(current, dto);
    btn.disabled = !hasDiff;
    btn.title = btn.disabled ? 'No differences to apply' : '';
  } catch (_) {
    // keep button state as-is on error
  }
}

function buildCurrentDtoFromForm() {
  const s = snapshotForm();
  const cfg = {
    test: {
      minClients: parseInt(s.test.minClients, 10),
      maxClients: parseInt(s.test.maxClients, 10),
      stageIntervalMs: parseInt(s.test.stageIntervalMs, 10),
      requestDelayMs: parseInt(s.test.requestDelayMs, 10),
      keySize: parseInt(s.test.keySize, 10),
      valueSize: parseInt(s.test.valueSize, 10),
    },
    redis: {
      operationTimeoutMs: parseInt(s.redis.operationTimeoutMs, 10),
      expiration: parseInt(s.redis.expiration, 10),
    }
  };
  const targets = s.targets.map(t => {
    const parsed = parseInt(t.workerCount, 10);
    const wc = (Number.isFinite(parsed) && parsed > 0) ? parsed : 1;
    return {
      redisUrl: t.redisUrl || '',
      clusterUrl: t.clusterUrl || '',
      workerCount: wc,
    };
  }).filter(t => (t.redisUrl || t.clusterUrl));
  return { version: 'v1', updatedAt: new Date().toISOString(), config: cfg, targets };
}

function hasMeaningfulDiff(current, incoming) {
  const diff = prettyKeyDiff(current, incoming);
  return !!(diff && diff.trim().length);
}

// Apply targets list to the form (clears extra rows, ensures count)
function applyTargetsToForm(targets) {
  if (!Array.isArray(targets) || !targets.length) return;
  const container = document.getElementById('targets');
  const rows = container.querySelectorAll('.target');
  Array.from(rows).slice(1).forEach(row => row.remove());
  targets.forEach((t, i) => {
    if (i > 0) {
      const addBtn = document.getElementById('addTarget');
      if (addBtn) addBtn.click();
    }
    const row = container.querySelectorAll('.target')[i] || container.querySelector('.target');
    if (row) {
      const redisInput = row.querySelector('input[name="redisUrl"]');
      const clusterInput = row.querySelector('input[name="clusterUrl"]');
      const wcInput = row.querySelector('input[name="workerCount"]');
      if (redisInput) redisInput.value = t.redisUrl || '';
      if (clusterInput) clusterInput.value = t.clusterUrl || '';
      if (wcInput) wcInput.value = String(Number.isFinite(t.workerCount) && t.workerCount > 0 ? t.workerCount : 1);
    }
  });
}

// Shared import handler for Apply/Confirm buttons
async function performImportAndClose() {
  try {
    const dto = await fetchRuntimeConfig({ force: true });
    applyRuntimeConfigToForm(dto, 'merge');
    applyTargetsToForm(dto.targets);
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshotForm())); } catch {}
    try {
      const when = new Date().toLocaleString();
      setStatus(`Imported runtime config @ ${when}`, 'success');
    } catch(_) { setStatus('Imported runtime config into form.', 'success'); }
    closeImportModal();
    updateImportConfigButtonState();
  } catch (e) {
    setStatus(`Failed to import runtime config: ${e.message}`, 'error', e.details);
    throw e;
  }
}
