const API = '';

async function api(path, opts) {
  const res = await fetch(API + path, opts);
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || (Array.isArray(data.errors) && data.errors[0]) || res.statusText);
  return data;
}

// Party with o11y mode toggle
function togglePartyMode() {
  document.body.classList.toggle('party-mode');
  const isParty = document.body.classList.contains('party-mode');
  localStorage.setItem('partyMode', isParty);
}

// Restore party mode from localStorage on load
document.addEventListener('DOMContentLoaded', () => {
  if (localStorage.getItem('partyMode') === 'true') {
    document.body.classList.add('party-mode');
  }
});

// Favicon hover wiggle
document.addEventListener('DOMContentLoaded', () => {
  const favicon = document.querySelector('link[rel="icon"]');
  if (!favicon) return;
  let wiggleTimer;
  favicon.addEventListener('mouseenter', () => {
    favicon.href = '/assets/favicon.svg#';
    document.documentElement.classList.add('favicon-wiggle');
    clearTimeout(wiggleTimer);
    wiggleTimer = setTimeout(() => {
      document.documentElement.classList.remove('favicon-wiggle');
    }, 600);
  });
});

let scenarios = [];

// --- Modal toggles ---
function openModal(overlayId) {
  document.getElementById(overlayId).classList.add('open');
}
function closeModal(overlayId) {
  document.getElementById(overlayId).classList.remove('open');
}
function openImportModal() {
  document.getElementById('import-file').value = '';
  document.getElementById('import-status').textContent = '';
  openModal('import-modal-overlay');
}
function closeImportModal(e) {
  if (e && e.target !== e.currentTarget) return; // only close when overlay itself is clicked
  closeModal('import-modal-overlay');
}
function openScenarioModal() {
  document.getElementById('scenario-file-input').value = '';
  document.getElementById('scenario-upload-status').textContent = '';
  openModal('scenario-modal-overlay');
}
function closeScenarioModal(e) {
  if (e && e.target !== e.currentTarget) return;
  closeModal('scenario-modal-overlay');
}
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') {
    closeModal('import-modal-overlay');
    closeModal('scenario-modal-overlay');
  }
});

function handleScenarioChange(el) {
  const topo = document.getElementById('topology');
  if (el.value) {
    topo.disabled = true;
    topo.style.opacity = '0.5';
    topo.style.cursor = 'not-allowed';
  } else {
    topo.disabled = false;
    topo.style.opacity = '';
    topo.style.cursor = '';
  }
}

async function loadScenarios() {
  try {
    const data = await api('/api/scenarios');
    scenarios = data.scenarios;
    const sel = document.getElementById('scenario');
    scenarios.forEach(s => {
      const opt = document.createElement('option');
      opt.value = s;
      opt.textContent = s;
      sel.appendChild(opt);
    });
    if (scenarios.includes('microservice')) {
      sel.value = 'microservice';
      handleScenarioChange(sel);
    }
  } catch(e) { console.error('load scenarios:', e); }
}

function statusClass(status) {
  return 'status-' + status;
}

function formatTime(t) {
  if (!t) return '-';
  return new Date(t).toLocaleTimeString();
}

function formatDuration(ns) {
  if (!ns) return '-';
  if (ns < 1000) return ns + 'ns';
  if (ns < 1e6) return (ns / 1e3).toFixed(1) + '&#956;s';
  if (ns < 1e9) return (ns / 1e6).toFixed(1) + 'ms';
  return (ns / 1e9).toFixed(2) + 's';
}

// --- Infinite indicator for unlimited values ---

function updateInfiniteIndicator(id) {
  const input = document.getElementById(id);
  const overlay = document.getElementById(id + '-inf');
  if (!input || !overlay) return;
  // Inactive (other mode active) → show "-" instead of ∞
  if ('inactive' in input.dataset) {
    overlay.textContent = '\u2014';
    overlay.classList.add('visible');
    input.style.color = '#64748b';
    return;
  }
  const val = input.value.trim();
  const isInfinite = val === '' || val === '0' || val.startsWith('0');
  overlay.textContent = '\u221E';
  overlay.classList.toggle('visible', isInfinite);
  if (isInfinite) {
    input.style.color = '#64748b';
  } else {
    input.style.color = '';
  }
}

function focusInfiniteInput(id, activate) {
  const input = document.getElementById(id);
  if (!input) return;
  if (activate !== undefined && (parseInt(input.value) || 0) === 0) {
    input.value = String(activate);
  }
  input.focus();
  hideInfiniteOverlay(id);
  if (activate !== undefined) {
    syncTraceCountDuration(id);
  }
}

function hideInfiniteOverlay(id) {
  const overlay = document.getElementById(id + '-inf');
  if (overlay) overlay.classList.remove('visible');
  const input = document.getElementById(id);
  if (input) input.style.color = '';
}

// --- Soft mutual exclusion: changedId wins ---

function syncTraceCountDuration(changedId) {
  const tc = document.getElementById('trace-count');
  const dur = document.getElementById('duration');
  if (!tc || !dur) return;

  function activate(id) { delete document.getElementById(id).dataset.inactive; }
  function deactivate(id) { document.getElementById(id).dataset.inactive = ''; }

  const tcVal = parseInt(tc.value) || 0;
  const durVal = parseInt(dur.value) || 0;

  // changedId tells us which field was just modified — that field's mode wins
  if (changedId === 'trace-count' && tcVal > 0) {
    dur.value = '';
    deactivate('duration');
    activate('trace-count');
  } else if (changedId === 'duration' && durVal > 0) {
    tc.value = '0';
    deactivate('trace-count');
    activate('duration');
  } else if (tcVal > 0 && durVal > 0) {
    // Both > 0 without a clear changedId — keep both (generator handles it)
  } else if (tcVal > 0) {
    dur.value = '';
    deactivate('duration');
    activate('trace-count');
  } else if (durVal > 0) {
    tc.value = '0';
    deactivate('trace-count');
    activate('duration');
  } else {
    delete tc.dataset.inactive;
    delete dur.dataset.inactive;
  }

  updateInfiniteIndicator('trace-count');
  updateInfiniteIndicator('duration');
}

// --- Signal type UI ---

function getSignals() {
  const checks = document.querySelectorAll('#signal-type input[type="checkbox"]');
  const signals = {
    traces: false,
    logs: false,
    metrics: false
  };
  checks.forEach(c => {
    if (c.checked && signals.hasOwnProperty(c.value)) {
      signals[c.value] = true;
    }
  });
  // Default to traces if nothing selected
  if (!signals.traces && !signals.logs && !signals.metrics) {
    signals.traces = true;
  }
  return signals;
}

function updateAllPillState(containerId) {
  const container = document.getElementById(containerId);
  if (!container) return;
  const checks = container.querySelectorAll('input[type="checkbox"]');
  const allChecked = checks.length > 0 && Array.from(checks).every(c => c.checked);
  const allBtn = container.querySelector('.pill-all');
  if (allBtn) allBtn.classList.toggle('active', allChecked);
}

function selectAllSignals() {
  const checks = document.querySelectorAll('#signal-type input[type="checkbox"]');
  const allChecked = Array.from(checks).every(c => c.checked);
  checks.forEach(c => c.checked = !allChecked);
  onSignalCheckboxChange('traces');
  updateAllPillState('signal-type');
}

// Do NOT preventDefault: it triggers the checkbox activation-revert, undoing
// the manual checked changes below. Let the native toggle commit, then adjust.
function onSignalPillClick(event, value) {
  const multiSelect = event.ctrlKey || event.metaKey;
  const checks = document.querySelectorAll('#signal-type input[type="checkbox"]');
  const clicked = document.querySelector('#signal-type input[value="' + value + '"]');
  if (multiSelect) {
    if (!Array.from(checks).some(c => c.checked)) clicked.checked = true;
  } else {
    checks.forEach(c => { c.checked = (c.value === value); });
  }
  onSignalCheckboxChange(value);
  updateAllPillState('signal-type');
}

function onSignalCheckboxChange(changed) {
  const checks = document.querySelectorAll('#signal-type input[type="checkbox"]');
  // Ensure at least one checkbox is checked
  let anyChecked = false;
  checks.forEach(c => { if (c.checked) anyChecked = true; });
  if (!anyChecked) {
    // Re-check traces — always need at least one signal
    document.querySelector('#signal-type input[value="traces"]').checked = true;
  }
  const hasTraces = document.querySelector('#signal-type input[value="traces"]').checked;
  const hasLogs = document.querySelector('#signal-type input[value="logs"]').checked;
  const hasMetrics = document.querySelector('#signal-type input[value="metrics"]').checked;
  // Show/hide trace config area — only auto-expand when this signal was just toggled on
  var traceArea = document.getElementById('trace-config-area');
  traceArea.style.display = hasTraces ? '' : 'none';
  if (changed === 'traces' && hasTraces && traceArea.classList.contains('collapsed')) {
    traceArea.classList.remove('collapsed');
    try { localStorage.removeItem('o11y-workbench-config-trace-collapsed'); } catch(e) {}
  }
  // Show/hide trace-specific fields
  document.getElementById('trace-fields-group').classList.toggle('hidden', !hasTraces);
  // Show/hide log config area — only auto-expand when this signal was just toggled on
  var logArea = document.getElementById('log-config-area');
  logArea.style.display = hasLogs ? '' : 'none';
  if (changed === 'logs' && hasLogs && logArea.classList.contains('collapsed')) {
    logArea.classList.remove('collapsed');
    try { localStorage.removeItem('o11y-workbench-config-log-collapsed'); } catch(e) {}
  }
  document.getElementById('log-per-span-field').style.display = (hasTraces && hasLogs) ? 'block' : 'none';
  document.getElementById('log-count-field').style.display = hasLogs ? 'block' : 'none';
  // Show/hide metric config area — only auto-expand when this signal was just toggled on
  var metricArea = document.getElementById('metric-config-area');
  metricArea.style.display = hasMetrics ? '' : 'none';
  if (changed === 'metrics' && hasMetrics && metricArea.classList.contains('collapsed')) {
    metricArea.classList.remove('collapsed');
    try { localStorage.removeItem('o11y-workbench-config-metric-collapsed'); } catch(e) {}
  }
  updateSeverityBars();
  updateMetricTypeBars();
  updateToggleAllBtn();
  updateAllPillState('signal-type');
}

function updateSeverityBars() {
  const ids = ['info', 'warn', 'error', 'fatal'];
  ids.forEach(s => {
    const v = parseInt(document.getElementById('sev-' + s).value) || 0;
    updateChunkedBar(document.getElementById('sev-bar-' + s), v);
  });
  const total = ids.reduce((sum, s) => sum + (parseInt(document.getElementById('sev-' + s).value) || 0), 0);
  const el = document.getElementById('severity-total');
  el.innerHTML = 'Total: <span class="' + (total === 100 ? 'ok' : 'over') + '">' + total + '</span> / 100';
}

function redistributeSeverity(changed, newVal) {
  const ids = ['info', 'warn', 'error', 'fatal'];
  const clamped = Math.max(0, Math.min(100, newVal));
  const remaining = 100 - clamped;
  const others = ids.filter(s => s !== changed);
  const otherSum = others.reduce((sum, s) => sum + (parseInt(document.getElementById('sev-' + s).value) || 0), 0);
  let newOthers = {};
  if (otherSum === 0) {
    const each = Math.floor(remaining / 3);
    let leftover = remaining - each * 3;
    others.forEach((s, i) => { newOthers[s] = each + (i < leftover ? 1 : 0); });
  } else {
    let assigned = 0;
    others.forEach((s, i) => {
      const cur = parseInt(document.getElementById('sev-' + s).value) || 0;
      let v = i === others.length - 1
        ? remaining - assigned
        : Math.round(remaining * cur / otherSum);
      newOthers[s] = v;
      assigned += v;
    });
  }
  document.getElementById('sev-' + changed).value = clamped;
  others.forEach(s => { document.getElementById('sev-' + s).value = Math.max(0, Math.min(100, newOthers[s])); });
  updateSeverityBars();
}

function onSeverityInput(changed) {
  const el = document.getElementById('sev-' + changed);
  let v = parseInt(el.value);
  if (isNaN(v) || v < 0) v = 0;
  if (v > 100) v = 100;
  redistributeSeverity(changed, v);
}

// --- Drag-to-set bar values ---
var dragState = null;

function startBarDrag(ev, prefix, id) {
  ev.preventDefault();
  document.activeElement && document.activeElement.blur();
  var track = ev.currentTarget;
  var callback = prefix === 'sev'
    ? function(pct) { redistributeSeverity(id, pct); }
    : function(pct) { redistributeMetricTypes(id, pct); };
  track.classList.add('dragging');
  dragState = { track: track, callback: callback };
  updateBarDrag(ev);
}

function updateBarDrag(ev) {
  if (!dragState) return;
  var rect = dragState.track.getBoundingClientRect();
  var x = ev.clientX;
  // Touch events store coordinates in touches array
  if (x === undefined && ev.touches && ev.touches.length) {
    x = ev.touches[0].clientX;
  }
  if (x === undefined) return;
  var pct = Math.round(((x - rect.left) / rect.width) * 100);
  dragState.callback(Math.max(0, Math.min(100, pct)));
}

function stopBarDrag() {
  if (dragState) {
    dragState.track.classList.remove('dragging');
    dragState = null;
  }
}

function collectLogAttrs() {
  const rows = document.querySelectorAll('#attr-body tr');
  const attrs = [];
  rows.forEach(tr => {
    const key = tr.querySelector('.attr-key').value.trim();
    const val = tr.querySelector('.attr-val').value.trim();
    if (key) attrs.push({ key: key, value: val });
  });
  return attrs;
}

function addLogAttr(key, val) {
  const tbody = document.getElementById('attr-body');
  const tr = document.createElement('tr');
  tr.innerHTML = '<td><input type="text" class="attr-key" placeholder="key" value="' + escapeHtml(key || '') + '"></td>' +
    '<td><input type="text" class="attr-val" placeholder="value" value="' + escapeHtml(val || '') + '"></td>' +
    '<td><button class="btn-danger" style="font-size:14px;padding:2px 6px" onclick="this.closest(\'tr\').remove()">&times;</button></td>';
  tbody.appendChild(tr);
}

function getMetricAttrs() {
  const tbody = document.getElementById('metric-attr-body');
  const attrs = [];
  tbody.querySelectorAll('tr').forEach(tr => {
    const key = tr.querySelector('.attr-key').value.trim();
    const val = tr.querySelector('.attr-val').value.trim();
    if (key) attrs.push({ key: key, value: val });
  });
  return attrs;
}

function addMetricAttr(key, val) {
  const tbody = document.getElementById('metric-attr-body');
  const tr = document.createElement('tr');
  tr.innerHTML = '<td><input type="text" class="attr-key" placeholder="key" value="' + escapeHtml(key || '') + '"></td>' +
    '<td><input type="text" class="attr-val" placeholder="value" value="' + escapeHtml(val || '') + '"></td>' +
    '<td><button class="btn-danger" style="font-size:14px;padding:2px 6px" onclick="this.closest(\'tr\').remove()">&times;</button></td>';
  tbody.appendChild(tr);
}

function updateMetricTypeBars() {
  const ids = ['gauge', 'sum', 'histogram'];
  const total = ids.reduce((sum, s) => sum + (parseFloat(document.getElementById('metric-' + s).value) || 0), 0);
  ids.forEach(s => {
    const v = parseFloat(document.getElementById('metric-' + s).value) || 0;
    updateChunkedBar(document.getElementById('metric-bar-' + s), v);
  });
  const el = document.getElementById('metric-type-total');
  el.innerHTML = 'Total: <span class="' + (Math.abs(total - 100) < 0.5 ? 'ok' : 'over') + '">' + (total / 100).toFixed(1) + '</span>';
}

function redistributeMetricTypes(changed, newVal) {
  const ids = ['gauge', 'sum', 'histogram'];
  const clamped = Math.max(0, Math.min(100, newVal));
  const remaining = 100 - clamped;
  const others = ids.filter(s => s !== changed);
  const otherSum = others.reduce((sum, s) => sum + (parseFloat(document.getElementById('metric-' + s).value) || 0), 0);
  let newOthers = {};
  if (otherSum === 0) {
    const each = Math.floor(remaining / 2);
    let leftover = remaining - each * 2;
    others.forEach((s, i) => { newOthers[s] = each + (i < leftover ? 1 : 0); });
  } else {
    let assigned = 0;
    others.forEach((s, i) => {
      const cur = parseFloat(document.getElementById('metric-' + s).value) || 0;
      let v = i === others.length - 1
        ? remaining - assigned
        : Math.round(remaining * cur / otherSum);
      newOthers[s] = v;
      assigned += v;
    });
  }
  document.getElementById('metric-' + changed).value = clamped;
  others.forEach(s => { document.getElementById('metric-' + s).value = Math.max(0, Math.min(100, newOthers[s])); });
  updateMetricTypeBars();
}



function onMetricTypeInput(type) {
  const el = document.getElementById('metric-' + type);
  let v = parseInt(el.value);
  if (isNaN(v) || v < 0) v = 0;
  if (v > 100) v = 100;
  redistributeMetricTypes(type, v);
}

function initChunkedBars() {
  // Create 20 chunk divs inside each .sev-bar-fill element
  document.querySelectorAll('.sev-bar-fill').forEach(function(fill) {
    fill.innerHTML = '';
    for (var i = 0; i < 20; i++) {
      var chunk = document.createElement('div');
      chunk.className = 'sev-chunk';
      fill.appendChild(chunk);
    }
  });
}

function updateChunkedBar(fillEl, value) {
  var v = Math.max(0, Math.min(100, value));
  var fullChunks = Math.floor(v / 5);
  var remainder = v % 5;
  var chunks = fillEl.children;
  var fillColor = (fillEl.style.getPropertyValue('--chunk-color') || '').trim();
  if (!fillColor) fillColor = '#3b82f6';
  for (var i = 0; i < chunks.length; i++) {
    chunks[i].className = 'sev-chunk';
    if (i < fullChunks) {
      chunks[i].style.background = fillColor;
    } else if (i === fullChunks && remainder > 0) {
      var fillPct = (remainder / 5) * 100;
      chunks[i].style.background = 'linear-gradient(to right, ' + fillColor + ' 0% ' + fillPct + '%, transparent ' + fillPct + '% 100%)';
    } else {
      chunks[i].style.background = '';
    }
  }
}

function toggleAllSignalConfigs() {
  var areas = ['trace-config-area', 'log-config-area', 'metric-config-area'];
  var visibleAreas = areas.map(function(id) { return document.getElementById(id); }).filter(function(el) { return el && el.style.display !== 'none'; });
  var anyCollapsed = visibleAreas.some(function(el) { return el.classList.contains('collapsed'); });
  visibleAreas.forEach(function(el) {
    var signal = el.id.replace('-config-area', '');
    if (anyCollapsed) {
      el.classList.remove('collapsed');
      try { localStorage.removeItem('o11y-workbench-config-' + signal + '-collapsed'); } catch(e) {}
    } else {
      el.classList.add('collapsed');
      try { localStorage.setItem('o11y-workbench-config-' + signal + '-collapsed', '1'); } catch(e) {}
    }
  });
  document.getElementById('toggle-all-btn').textContent = anyCollapsed ? 'Collapse All' : 'Expand All';
}

function toggleSignalConfig(signal) {
  const area = document.getElementById(signal + '-config-area');
  if (!area) return;
  const isCollapsed = area.classList.toggle('collapsed');
  const key = 'o11y-workbench-config-' + signal + '-collapsed';
  try { localStorage.setItem(key, isCollapsed ? '1' : ''); } catch(e) {}
  updateToggleAllBtn();
}

function updateToggleAllBtn() {
  var areas = ['trace-config-area', 'log-config-area', 'metric-config-area'];
  var visibleAreas = areas.map(function(id) { return document.getElementById(id); }).filter(function(el) { return el && el.style.display !== 'none'; });
  var anyCollapsed = visibleAreas.some(function(el) { return el.classList.contains('collapsed'); });
  document.getElementById('toggle-all-btn').textContent = anyCollapsed ? 'Expand All' : 'Collapse All';
}

function resetSignalFilter() {
  const checks = document.querySelectorAll('#run-filter-signals input[type="checkbox"]');
  const allChecked = Array.from(checks).every(c => c.checked);
  if (allChecked) {
    checks.forEach(c => c.checked = c.value === 'traces');
  } else {
    checks.forEach(c => c.checked = true);
  }
  renderRunList();
  updateAllPillState('run-filter-signals');
}

// Do NOT preventDefault: it triggers the checkbox activation-revert, undoing
// the manual checked changes below. Let the native toggle commit, then adjust.
function onFilterSignalPillClick(event, value) {
  const multiSelect = event.ctrlKey || event.metaKey;
  if (!multiSelect) {
    const checks = document.querySelectorAll('#run-filter-signals input[type="checkbox"]');
    checks.forEach(c => { c.checked = (c.value === value); });
  }
  onFilterSignalChange();
}

function onFilterSignalChange() {
  const checks = document.querySelectorAll('#run-filter-signals input[type="checkbox"]');
  const anyChecked = Array.from(checks).some(c => c.checked);
  if (!anyChecked) {
    document.querySelector('#run-filter-signals input[value="traces"]').checked = true;
  }
  renderRunList();
  updateAllPillState('run-filter-signals');
}

let runs = {};
let pollTimer = null;

const ACTIVE_RUN_STATUSES = new Set(['pending', 'running', 'paused', 'streaming']);

function hasActiveRuns(runsMap) {
  return Object.values(runsMap).some(r => ACTIVE_RUN_STATUSES.has(r.status));
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

function mergeListRun(prev, incoming) {
  // List omits OTLP payloads; keep any previously fetched detail blobs.
  if (prev) {
    if (incoming.trace_data == null && prev.trace_data != null) incoming.trace_data = prev.trace_data;
    if (incoming.log_data == null && prev.log_data != null) incoming.log_data = prev.log_data;
    if (incoming.metric_data == null && prev.metric_data != null) incoming.metric_data = prev.metric_data;
  }
  return incoming;
}

function runNeedsPayloadFetch(r) {
  return (!!r.has_trace_data && !r.trace_data) ||
    (!!r.has_log_data && !r.log_data) ||
    (!!r.has_metric_data && !r.metric_data);
}

async function fetchRunDetail(id) {
  const full = await api('/api/runs/' + id);
  const prev = runs[id];
  runs[id] = prev ? { ...prev, ...full } : full;
  return runs[id];
}

async function startRun() {
  const runBtn = document.getElementById('run-btn');
  runBtn.disabled = true;
  const statusEl = document.getElementById('run-status');
  statusEl.textContent = 'Starting...';

  const signals = getSignals();

  var body = {
    seed: parseInt(document.getElementById('seed').value) || 42,
    output: document.getElementById('output').value,
    format: document.getElementById('format').value,
    insecure: document.getElementById('insecure').checked,
    endpoints: document.getElementById('endpoints').value.split(',').map(s => s.trim()).filter(s => s),
    signals: signals
  };

  if (signals.traces) {
    body.trace_count = parseInt(document.getElementById('trace-count').value) || 0;
    body.rate = parseInt(document.getElementById('rate').value) || 0;
    body.duration = parseInt(document.getElementById('duration').value) || 0;
    body.topology = document.getElementById('topology').value;
    body.scenario = document.getElementById('scenario').value;
    body.span_count = parseInt(document.getElementById('span-count').value) || 10;
    body.scenario_span_count = parseInt(document.getElementById('scenario-span-count').value) || 0;
    body.max_depth = parseInt(document.getElementById('max-depth').value) || 5;
    body.service_name = document.getElementById('service-name').value || 'mock-service';
  }

  if (signals.metrics) {
    body.metric_config = {
      enabled: true,
      metric_count: parseInt(document.getElementById('metric-count').value) || 20,
      gauge_ratio: (parseFloat(document.getElementById('metric-gauge').value) || 0) / 100,
      sum_ratio: (parseFloat(document.getElementById('metric-sum').value) || 0) / 100,
      value_min: parseFloat(document.getElementById('metric-value-min').value) || 0,
      value_max: parseFloat(document.getElementById('metric-value-max').value) || 100,
      attributes: getMetricAttrs()
    };
  }

  if (signals.logs) {
    body.log_config = {
      enabled: true,
      logs_per_span: parseInt(document.getElementById('logs-per-span').value) || 3,
      log_count: parseInt(document.getElementById('log-count').value) || 10,
      severity_dist: {
        info: parseInt(document.getElementById('sev-info').value) || 0,
        warn: parseInt(document.getElementById('sev-warn').value) || 0,
        error: parseInt(document.getElementById('sev-error').value) || 0,
        fatal: parseInt(document.getElementById('sev-fatal').value) || 0
      },
      body_template: document.getElementById('body-template').value || '',
      attributes: collectLogAttrs()
    };
  }

  try {
    const run = await api('/api/runs', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body)
    });
    runs[run.id] = run;
    if (!pollTimer) startPolling();
    statusEl.textContent = 'Starting... (ID: ' + run.id + ')';
  } catch(e) {
    statusEl.textContent = 'Error: ' + e.message;
  }
  runBtn.disabled = false;
}

async function importTraces() {
  const fileInput = document.getElementById('import-file');
  const file = fileInput.files[0];
  if (!file) return;
  const btn = document.getElementById('import-btn');
  const status = document.getElementById('import-status');
  btn.disabled = true;
  status.textContent = 'Importing...';
  try {
    const text = await file.text();
    const run = await api('/api/runs/import', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: text
    });
    runs[run.id] = run;
    renderRunList();
    status.innerHTML = '<span style="color:#22c55e">Imported ' + (run.span_count || 0) + ' spans</span>';
    setTimeout(() => closeImportModal(), 800);
  } catch(e) {
    status.innerHTML = '<span style="color:#ef4444">Error: ' + escapeHtml(e.message) + '</span>';
  }
  btn.disabled = false;
}

async function uploadScenarios() {
  const fileInput = document.getElementById('scenario-file-input');
  const file = fileInput.files[0];
  if (!file) return;
  const btn = document.getElementById('scenario-upload-btn');
  const status = document.getElementById('scenario-upload-status');
  btn.disabled = true;
  status.textContent = 'Uploading...';
  try {
    const text = await file.text();
    const ct = file.name.endsWith('.yaml') || file.name.endsWith('.yml') ? 'application/x-yaml' : 'application/json';
    const data = await api('/api/scenarios', {
      method: 'POST',
      headers: {'Content-Type': ct},
      body: text
    });
    status.innerHTML = '<span style="color:#22c55e">Registered: ' + (data.registered || []).join(', ') + '</span>';
    setTimeout(() => closeScenarioModal(), 800);
    // Reload scenario dropdown
    const sel = document.getElementById('scenario');
    while (sel.options.length > 1) sel.remove(1);
    await loadScenarios();
  } catch(e) {
    status.innerHTML = '<span style="color:#ef4444">Error: ' + escapeHtml(e.message) + '</span>';
  }
  btn.disabled = false;
}






async function pauseRun(id) {
  try {
    await api('/api/runs/' + id + '/pause', { method: 'POST' });
    renderRunList();
  } catch(e) {
    console.error('pause run:', e);
  }
}

async function resumeRun(id) {
  try {
    await api('/api/runs/' + id + '/resume', { method: 'POST' });
    renderRunList();
  } catch(e) {
    console.error('resume run:', e);
  }
}

async function stopRun(id) {
  try {
    await api('/api/runs/' + id + '/stop', { method: 'POST' });
    renderRunList();
  } catch(e) {
    console.error('stop run:', e);
  }
}

async function stopAllRuns() {
  const active = Object.values(runs).filter(r => r.status === 'running' || r.status === 'pending' || r.status === 'paused');
  if (active.length === 0) return;
  const btn = document.getElementById('stop-all-btn');
  if (btn) btn.disabled = true;
  try {
    await Promise.all(active.map(r => api('/api/runs/' + r.id + '/stop', { method: 'POST' })));
  } catch(e) {
    console.error('stop all runs:', e);
  }
  if (btn) btn.disabled = false;
  renderRunList();
}

async function pauseAllRuns() {
  const active = Object.values(runs).filter(r => r.status === 'running');
  if (active.length === 0) return;
  const btn = document.getElementById('pause-all-btn');
  if (btn) btn.disabled = true;
  try {
    await Promise.all(active.map(r => api('/api/runs/' + r.id + '/pause', { method: 'POST' })));
  } catch(e) {
    console.error('pause all runs:', e);
  }
  if (btn) btn.disabled = false;
  renderRunList();
}

async function resumeAllRuns() {
  const active = Object.values(runs).filter(r => r.status === 'paused');
  if (active.length === 0) return;
  const btn = document.getElementById('resume-all-btn');
  if (btn) btn.disabled = true;
  try {
    await Promise.all(active.map(r => api('/api/runs/' + r.id + '/resume', { method: 'POST' })));
  } catch(e) {
    console.error('resume all runs:', e);
  }
  if (btn) btn.disabled = false;
  renderRunList();
}

function showOtterToast(run) {
  const existing = document.querySelector('.otter-toast');
  if (existing) existing.remove();

  const toast = document.createElement('div');
  toast.className = 'otter-toast';
  toast.innerHTML = '<span class="toast-icon">🦦</span><span class="toast-text">o11y the Otter finished <strong>' +
    (run.label || 'run #' + run.id.slice(0,8)) + '</strong></span>';
  document.body.appendChild(toast);

  setTimeout(() => {
    toast.classList.add('out');
    setTimeout(() => toast.remove(), 300);
  }, 3000);
}

function startPolling() {
  if (pollTimer) return;
  const announcedCompleted = new Set();
  pollTimer = setInterval(async () => {
    try {
      const data = await api('/api/runs');
      for (const r of data.runs) {
        const prev = runs[r.id];
        const prevStatus = prev?.status;
        runs[r.id] = mergeListRun(prev, r);
        // Show brief "Completed!" when a run transitions to completed
        if (r.status === 'completed' && prevStatus && prevStatus !== 'completed' && !announcedCompleted.has(r.id)) {
          announcedCompleted.add(r.id);
          showOtterToast(r);
          const el = document.getElementById('run-status');
          el.textContent = 'Completed!';
          setTimeout(() => { if (el.textContent === 'Completed!') el.textContent = ''; }, 3000);
        }
      }
      renderRunList();
      // Show detail if viewing one
      const detail = document.getElementById('detail-section');
      if (!detail.classList.contains('hidden')) {
        const id = detail.querySelector('#detail-id').textContent;
        let r = runs[id];
        if (r && runNeedsPayloadFetch(r)) {
          try { r = await fetchRunDetail(id); } catch (e) { console.error('Detail refresh error:', e); }
        }
        if (r) renderDetail(r);
      }
      if (!hasActiveRuns(runs)) stopPolling();
    } catch(e) {
      console.error('Polling error:', e);
      const statusEl = document.getElementById('run-status');
      if (statusEl && statusEl.textContent && statusEl.textContent.includes('Starting')) {
        statusEl.textContent = 'Connection error - retrying...';
      }
    }
  }, 1000);
}

let lastDetailKey = '';

function getRunSignals(r) {
  if (r.output === 'import') {
    return { traces: true, logs: false, metrics: false };
  }
  // Prefer signals object if available, fall back to signal_type for backwards compat
  if (r.config?.signals) {
    return r.config.signals;
  }
  // Default to traces
  return { traces: true, logs: false, metrics: false };
}

function getSignalBadge(signals) {
  const t = signals.traces || false;
  const l = signals.logs || false;
  const m = signals.metrics || false;

  if (t && l && m) return '<span class="signal-badge all">A</span>';
  if (t && l) return '<span class="signal-badge both">TL</span>';
  if (t && m) return '<span class="signal-badge both" style="background:rgba(34,197,94,0.2);color:#4ade80">TM</span>';
  if (l && m) return '<span class="signal-badge both" style="background:rgba(139,92,246,0.2);color:#c084fc">LM</span>';
  if (l) return '<span class="signal-badge logs">L</span>';
  if (m) return '<span class="signal-badge metrics">M</span>';
  return '<span class="signal-badge traces">T</span>';
}

var _emptyStateHTML = null;

function renderRunList() {
  const list = document.getElementById('run-list');
  const search = (document.getElementById('filter-search').value || '').toLowerCase();
  let entries = Object.values(runs).sort((a, b) => new Date(b.started_at) - new Date(a.started_at));

  const checkedSignals = [];
  document.querySelectorAll('#run-filter-signals input[type="checkbox"]:checked').forEach(c => checkedSignals.push(c.value));
  if (checkedSignals.length > 0 && checkedSignals.length < 3) {
    entries = entries.filter(r =>
      (checkedSignals.includes('traces') && !!(r.has_trace_data || r.trace_data)) ||
      (checkedSignals.includes('logs') && !!(r.has_log_data || r.log_data || r.log_count || r.log_count_completed)) ||
      (checkedSignals.includes('metrics') && !!(r.has_metric_data || r.metric_data || r.metric_count || r.metric_count_completed))
    );
  }
  if (search) {
    entries = entries.filter(r => r.id.toLowerCase().includes(search) || (r.scenario || '').toLowerCase().includes(search));
  }

  if (_emptyStateHTML === null) {
    _emptyStateHTML = list.innerHTML;
  }

  if (entries.length === 0) {
    if (Object.keys(runs).length === 0) {
      list.innerHTML = _emptyStateHTML;
      return;
    }
    list.innerHTML = '<div class="empty"><p>No jobs match the current filter.</p></div>';
    return;
  }
  list.innerHTML = entries.map(r => {
    const signals = getRunSignals(r);
    const signalBadge = getSignalBadge(signals);
    const scenario = r.scenario || r.config?.Scenario || '';
    const scenarioTag = scenario ? ' &middot; ' + scenario : '';
    const isImported = r.output === 'import';
    const metaParts = [formatTime(r.started_at)];
    if (isImported) {
      metaParts.push('imported');
    } else {
      if (r.config?.SpanCount) metaParts.push(r.config.SpanCount + ' span budget');
      var tc = r.config?.TraceCount;
      if (tc !== undefined) metaParts.push((tc === 0 ? '\u221e' : tc) + ' traces');
    }
    var statsHtml = '<span>' + (r.span_count || 0) + ' spans</span>';
    if (r.log_count || r.log_count_completed) {
      var lc = r.log_count || r.log_count_completed;
      statsHtml += '<span>' + lc + ' logs</span>';
    }
    if (r.metric_count || r.metric_count_completed) {
      var mc = r.metric_count || r.metric_count_completed;
      statsHtml += '<span>' + mc + ' metrics</span>';
    }
    statsHtml += '<span>' + formatDuration(r.duration_ns) + '</span>';
    return `<div class="run-card${r.status === 'running' ? ' running' : ''}${r.status === 'completed' ? ' completed' : ''}" onclick="showDetail('${r.id}')">
      ${signalBadge}
      <div class="status ${statusClass(r.status)}"></div>
      <div class="info">
        <div class="id">${r.id}${scenarioTag}</div>
        <div class="meta">${metaParts.join(' &middot; ')}</div>
      </div>
      <div class="stats">
        ${statsHtml}
      </div>
      ${r.status === 'running' ? `<button class="btn-pause" onclick="event.stopPropagation();pauseRun('${r.id}')">Pause</button>` : ''}
      ${r.status === 'paused' ? `<button class="btn-pause" onclick="event.stopPropagation();resumeRun('${r.id}')">Resume</button>` : ''}
      ${r.status === 'running' || r.status === 'pending' || r.status === 'paused' ? `<button class="btn-danger" onclick="event.stopPropagation();stopRun('${r.id}')">Stop</button>` : ''}
    </div>`;
  }).join('');
  const pauseAllBtn = document.getElementById('pause-all-btn');
  const resumeAllBtn = document.getElementById('resume-all-btn');
  const stopAllBtn = document.getElementById('stop-all-btn');
  const hasRunning = entries.some(r => r.status === 'running');
  const hasPaused = entries.some(r => r.status === 'paused');
  const hasActive = hasRunning || hasPaused || entries.some(r => r.status === 'pending');
  if (pauseAllBtn) pauseAllBtn.style.display = hasRunning ? '' : 'none';
  if (resumeAllBtn) resumeAllBtn.style.display = hasPaused ? '' : 'none';
  if (stopAllBtn) stopAllBtn.style.display = hasActive ? '' : 'none';
}

async function showDetail(id) {
  const section = document.getElementById('detail-section');
  section.classList.remove('hidden');
  document.getElementById('detail-id').textContent = id;
  lastDetailKey = '';
  section.scrollIntoView({ behavior: 'smooth' });
  try {
    const r = await fetchRunDetail(id);
    renderDetail(r);
  } catch (e) {
    console.error('Failed to load run detail:', e);
    const el = document.getElementById('detail-content');
    if (el) el.innerHTML = '<div class="error">Failed to load run detail</div>';
  }
}

let detailTab = 'traces';

function renderDetail(r) {
  // Build a key from the run's mutable fields. If unchanged since last
  // render, skip DOM replacement to preserve scroll position in <pre>.
  const payloadKey = (r.has_trace_data ? 't' : '') + (r.trace_data ? String(r.trace_data.length || 1) : '0') +
    (r.has_log_data ? 'l' : '') + (r.log_data ? String(r.log_data.length || 1) : '0') +
    (r.has_metric_data ? 'm' : '') + (r.metric_data ? String(r.metric_data.length || 1) : '0');
  const key = r.id + '|' + r.status + '|' + (r.span_count || 0) + '|' + (r.log_count || 0) + '|' + (r.duration_ns || 0) + '|' + (r.error || '') + '|' + payloadKey + '|' + detailTab;
  if (key === lastDetailKey) return;
  lastDetailKey = key;

  const el = document.getElementById('detail-content');
  const signals = getRunSignals(r);
  const hasLogs = r.has_log_data || r.log_data || (r.log_count && r.log_count > 0);

  // Running state
  if (r.status === 'running') {
    const done = r.trace_count_completed || 0;
    const total = r.trace_count;
    const tracesLabel = total === 0 ? `${done} / &infin;` : `${done} / ${total}`;
    var progressHtml = '<div class="stat"><div class="value">' + tracesLabel + '</div><div class="label">Traces</div></div>';
    if (hasLogs || r.config?.log_config?.enabled) {
      var logDone = r.log_count_completed || 0;
      var logTotal = r.log_count || 0;
      progressHtml += '<div class="stat"><div class="value">' + logDone + ' / ' + (logTotal || '?') + '</div><div class="label">Logs</div></div>';
    }
    if (r.metric_count && r.metric_count > 0) {
      var metricDone = r.metric_count_completed || 0;
      progressHtml += '<div class="stat"><div class="value">' + metricDone + ' / ' + r.metric_count + '</div><div class="label">Metrics</div></div>';
    }
    el.innerHTML = '<div class="stat-row"><div class="stat"><div class="value"><span class="status-running" style="display:inline-block;width:12px;height:12px;border-radius:50%;margin-right:8px;vertical-align:middle"></span>Running</div><div class="label">Status</div></div>' + progressHtml + '</div><div style="margin-top:12px"><button class="btn btn-secondary" onclick="pauseRun(\'' + r.id + '\')">Pause</button><button class="btn btn-secondary" onclick="stopRun(\'' + r.id + '\')" style="margin-left:8px">Stop</button></div>';
    return;
  }

  // Paused state
  if (r.status === 'paused') {
    const done = r.trace_count_completed || 0;
    const total = r.trace_count;
    const tracesLabel = total === 0 ? `${done} / &infin;` : `${done} / ${total}`;
    var pausedHtml = '<div class="stat"><div class="value">' + tracesLabel + '</div><div class="label">Traces</div></div>';
    if (hasLogs) {
      pausedHtml += '<div class="stat"><div class="value">' + (r.log_count_completed || 0) + ' / ' + (r.log_count || '?') + '</div><div class="label">Logs</div></div>';
    }
    if (r.metric_count) {
      pausedHtml += '<div class="stat"><div class="value">' + (r.metric_count_completed || 0) + ' / ' + r.metric_count + '</div><div class="label">Metrics</div></div>';
    }
    el.innerHTML = '<div class="stat-row"><div class="stat"><div class="value"><span class="status-paused" style="display:inline-block;width:12px;height:12px;border-radius:50%;margin-right:8px;vertical-align:middle"></span>Paused</div><div class="label">Status</div></div>' + pausedHtml + '</div><div style="margin-top:12px"><button class="btn btn-secondary" onclick="resumeRun(\'' + r.id + '\')">Resume</button><button class="btn btn-secondary" onclick="stopRun(\'' + r.id + '\')" style="margin-left:8px">Stop</button></div>';
    return;
  }

  // Failed state
  if (r.status === 'failed') {
    el.innerHTML = '<div class="stat-row"><div class="stat"><div class="value" style="color:#ef4444">Failed</div><div class="label">Status</div></div><div class="stat"><div class="value">' + formatDuration(r.duration_ns) + '</div><div class="label">Duration</div></div></div><div class="error">' + (r.error || 'Unknown error') + '</div>';
    return;
  }

  // Stopped state
  if (r.status === 'stopped') {
    var stoppedStats = '<div class="stat"><div class="value"><span class="status-stopped" style="display:inline-block;width:12px;height:12px;border-radius:50%;margin-right:8px;vertical-align:middle"></span>Stopped</div><div class="label">Status</div></div><div class="stat"><div class="value">' + formatDuration(r.duration_ns) + '</div><div class="label">Duration</div></div><div class="stat"><div class="value">' + (r.span_count || 0) + '</div><div class="label">Spans</div></div>';
    if (r.log_count_completed) {
      stoppedStats += '<div class="stat"><div class="value">' + r.log_count_completed + '</div><div class="label">Logs</div></div>';
    }
    if (r.metric_count_completed) {
      stoppedStats += '<div class="stat"><div class="value">' + r.metric_count_completed + '</div><div class="label">Metrics</div></div>';
    }
    el.innerHTML = '<div class="stat-row">' + stoppedStats + '</div>';
    return;
  }

  // ===== Completed status =====
  // Counts come pre-computed from the Go backend; do not re-derive client-side.
  var spanCountTotal = r.span_count || 0;
  var traceCount = r.resource_span_count || r.trace_count || 0;
  var logRecordCount = r.log_record_count || 0;
  var logSeverities = r.log_severity_counts || {};
  var metricRecordCount = r.metric_series_count || 0;

  const isImported = r.output === 'import';
  var summaryHtml = '<div class="stat-row">';
  summaryHtml += '<div class="stat"><div class="value" style="color:#22c55e">Completed</div><div class="label">Status</div></div>';
  summaryHtml += '<div class="stat"><div class="value">' + (spanCountTotal || r.span_count || 0) + '</div><div class="label">Spans</div></div>';
  summaryHtml += '<div class="stat"><div class="value">' + (traceCount || r.trace_count || 0) + '</div><div class="label">Traces</div></div>';
  if (logRecordCount > 0) {
    summaryHtml += '<div class="stat"><div class="value">' + logRecordCount + '</div><div class="label">Logs</div></div>';
  }
  if (metricRecordCount > 0) {
    summaryHtml += '<div class="stat"><div class="value">' + metricRecordCount + '</div><div class="label">Metrics</div></div>';
  }
  summaryHtml += '<div class="stat"><div class="value">' + formatDuration(r.duration_ns) + '</div><div class="label">Duration</div></div>';
  summaryHtml += '<div class="stat"><div class="value">' + (isImported ? '-' : (r.config?.Seed || '-')) + '</div><div class="label">Seed</div></div>';
  summaryHtml += '</div>';

  // Severity mini tiles — below stat row, each severity as a small stat tile
  if (logRecordCount > 0) {
    summaryHtml += '<div class="sev-tiles">';
    var sevOrder = ['info', 'warn', 'error', 'fatal'];
    sevOrder.forEach(function(s) {
      var count = logSeverities[s] || 0;
      summaryHtml += '<div class="sev-tile"><div class="count sev-' + s + '">' + count + '</div><div class="lbl sev-' + s + '">' + s.toUpperCase() + '</div></div>';
    });
    summaryHtml += '</div>';
  }

  // Build tab bar
  var hasTraceData = !!(r.has_trace_data || r.trace_data);
  var hasLogData = logRecordCount > 0 || !!(r.has_log_data || r.log_data);
  var hasMetricData = metricRecordCount > 0 || !!(r.has_metric_data || r.metric_data);
  var tabHtml = '<div class="detail-tabs">';
  if (hasTraceData) {
    tabHtml += '<button class="detail-tab' + (detailTab === 'traces' ? ' active' : '') + '" onclick="switchDetailTab(\'traces\')">Spans<span class="tab-badge">' + (spanCountTotal || r.span_count || 0) + '</span></button>';
  }
  if (hasLogData) {
    tabHtml += '<button class="detail-tab' + (detailTab === 'logs' ? ' active' : '') + '" onclick="switchDetailTab(\'logs\')">Logs<span class="tab-badge">' + logRecordCount + '</span></button>';
  }
  if (hasMetricData) {
    tabHtml += '<button class="detail-tab' + (detailTab === 'metrics' ? ' active' : '') + '" onclick="switchDetailTab(\'metrics\')">Metrics<span class="tab-badge">' + metricRecordCount + '</span></button>';
  }
  tabHtml += '</div>';

  el.innerHTML = summaryHtml + tabHtml;

  if (detailTab === 'logs' && hasLogData) {
    el.innerHTML += renderLogTable(r.log_data);
  } else if (detailTab === 'traces' && r.trace_data) {
    const pretty = typeof r.trace_data === 'string' ? r.trace_data : JSON.stringify(r.trace_data, null, 2);
    el.innerHTML += '<h3 style="font-size:14px;font-weight:600;margin-bottom:8px;color:#94a3b8">Trace Data</h3>';
    el.innerHTML += '<pre>' + escapeHtml(pretty) + '</pre>';
  } else if (detailTab === 'metrics' && r.metric_data) {
    el.innerHTML += renderMetricTable(r.metric_data);
  }
}

function switchDetailTab(tab) {
  detailTab = tab;
  // Re-render will fire via polling or we can manually trigger
  const detailId = document.getElementById('detail-id').textContent;
  const r = runs[detailId];
  if (r) renderDetail(r);
}

// Shared table shell for log/metric OTLP renderers. Call sites keep signal-specific
// cell formatting, OTLP unwrapping, and detail-row wiring.
function buildTable(headers, bodyHtml) {
  var html = '<table class="log-table"><thead><tr>';
  for (var i = 0; i < headers.length; i++) {
    var h = headers[i];
    html += '<th' + (h.width ? ' style="width:' + h.width + '"' : '') + '>' + h.label + '</th>';
  }
  html += '</tr></thead><tbody>' + (bodyHtml || '') + '</tbody></table>';
  return html;
}

function renderLogTable(logData) {
  let logs;
  try {
    logs = typeof logData === 'string' ? JSON.parse(logData) : logData;
  } catch(e) { return '<div class="error">Failed to parse log data</div>'; }

  var bodyHtml = '';
  if (logs.resourceLogs) {
    var allRecords = [];
    logs.resourceLogs.forEach(function(rl) {
      if (rl.scopeLogs) rl.scopeLogs.forEach(function(sl) {
        if (sl.logRecords) sl.logRecords.forEach(function(lr) {
          allRecords.push(lr);
        });
      });
    });

    allRecords.slice(0, 200).forEach(function(lr, idx) {
      var sev = (lr.severityText || 'info').toLowerCase();
      var sevClass = sev === 'warn' ? 'warn' : sev === 'error' ? 'error' : sev === 'fatal' ? 'fatal' : 'info';
      var ts = lr.timeUnixNano ? new Date(lr.timeUnixNano / 1e6).toLocaleTimeString() : '-';

      var bodyStr = '';
      if (lr.body) {
        bodyStr = lr.body.stringValue || JSON.stringify(lr.body);
      }

      var traceIdStr = '';
      if (lr.traceId) {
        traceIdStr = typeof lr.traceId === 'string' ? lr.traceId.substring(0, 16) : '';
      }

      bodyHtml += '<tr onclick="toggleLogDetail(this, ' + idx + ')" class="log-row" data-idx="' + idx + '">';
      bodyHtml += '<td><span class="severity-tag ' + sevClass + '">' + (lr.severityText || 'INFO') + '</span></td>';
      bodyHtml += '<td style="white-space:nowrap;color:#64748b;font-family:monospace;font-size:11px">' + ts + '</td>';
      bodyHtml += '<td><div class="log-body">' + escapeHtml(bodyStr) + '</div></td>';
      bodyHtml += '<td style="font-family:monospace;font-size:10px;color:#475569">' + escapeHtml(traceIdStr) + '...</td>';
      bodyHtml += '</tr>';

      // Detail row (hidden)
      bodyHtml += '<tr class="log-detail" id="log-detail-' + idx + '"><td colspan="4">';
      bodyHtml += '<pre>' + escapeHtml(JSON.stringify(lr, null, 2)) + '</pre>';
      if (lr.attributes && lr.attributes.length) {
        bodyHtml += '<div class="attr-list">';
        lr.attributes.forEach(function(attr) {
          var av = '';
          if (attr.value) av = attr.value.stringValue || attr.value.intValue || attr.value.doubleValue || JSON.stringify(attr.value);
          bodyHtml += '<span class="attr-item"><strong>' + escapeHtml(attr.key) + ':</strong> ' + escapeHtml(String(av)) + '</span>';
        });
        bodyHtml += '</div>';
      }
      bodyHtml += '</td></tr>';
    });
  }

  return buildTable([
    { label: 'Severity', width: '52px' },
    { label: 'Timestamp', width: '140px' },
    { label: 'Body' },
    { label: 'Trace ID', width: '80px' }
  ], bodyHtml);
}

function toggleLogDetail(row, idx) {
  var detail = document.getElementById('log-detail-' + idx);
  if (!detail) return;
  var isOpen = detail.classList.contains('open');
  // Close all
  document.querySelectorAll('.log-detail.open').forEach(function(el) { el.classList.remove('open'); });
  document.querySelectorAll('.log-table tr.expanded').forEach(function(el) { el.classList.remove('expanded'); });
  if (!isOpen) {
    detail.classList.add('open');
    row.classList.add('expanded');
  }
}

function renderMetricTable(metricData) {
  let metrics;
  try {
    metrics = typeof metricData === 'string' ? JSON.parse(metricData) : metricData;
  } catch(e) { return '<div class="error">Failed to parse metric data</div>'; }

  var bodyHtml = '';
  if (metrics.resourceMetrics) {
    var allMetrics = [];
    metrics.resourceMetrics.forEach(function(rm) {
      if (rm.scopeMetrics) rm.scopeMetrics.forEach(function(sm) {
        if (sm.metrics) sm.metrics.forEach(function(m) {
          allMetrics.push(m);
        });
      });
    });

    allMetrics.slice(0, 200).forEach(function(m) {
      var type = 'gauge';
      var dataPoints = [];
      if (m.gauge) { type = 'gauge'; dataPoints = m.gauge.dataPoints || []; }
      else if (m.sum) { type = 'sum'; dataPoints = m.sum.dataPoints || []; }
      else if (m.histogram) { type = 'histogram'; dataPoints = m.histogram.dataPoints || []; }

      var typeClass = type === 'sum' ? 'warn' : type === 'histogram' ? 'fatal' : 'info';

      bodyHtml += '<tr class="log-row">';
      bodyHtml += '<td><span class="severity-tag ' + typeClass + '">' + type.toUpperCase() + '</span></td>';
      bodyHtml += '<td><strong>' + escapeHtml(m.name || '') + '</strong>';
      if (m.description) {
        bodyHtml += '<br><span style="font-size:11px;color:#64748b">' + escapeHtml(m.description) + '</span>';
      }
      bodyHtml += '</td>';
      bodyHtml += '<td style="font-family:monospace;font-size:11px">';

      if (type === 'histogram') {
        var hCount = 0, hSum = 0;
        dataPoints.forEach(function(dp) {
          hCount += dp.count || 0;
          hSum += dp.sum || 0;
        });
        bodyHtml += 'count: ' + hCount + ', sum: ' + hSum;
        if (dataPoints.length > 0 && dataPoints[0].bucketCounts) {
          bodyHtml += '<br>buckets: [' + dataPoints[0].bucketCounts.join(', ') + ']';
        }
      } else {
        dataPoints.slice(0, 3).forEach(function(dp, idx) {
          if (idx > 0) bodyHtml += '<br>';
          var val = dp.asInt !== undefined ? dp.asInt : (dp.asDouble !== undefined ? dp.asDouble : '?');
          bodyHtml += val;
        });
        if (dataPoints.length > 3) {
          bodyHtml += '<br><span style="color:#64748b">...and ' + (dataPoints.length - 3) + ' more</span>';
        }
      }

      bodyHtml += '</td>';
      var ts = dataPoints.length > 0 && dataPoints[0].timeUnixNano
        ? new Date(Number(dataPoints[0].timeUnixNano) / 1e6).toLocaleTimeString()
        : '-';
      bodyHtml += '<td style="white-space:nowrap;color:#64748b;font-family:monospace;font-size:11px">' + ts + '</td>';
      bodyHtml += '</tr>';
    });
  }

  return buildTable([
    { label: 'Type', width: '80px' },
    { label: 'Name' },
    { label: 'Value(s)' },
    { label: 'Timestamp', width: '140px' }
  ], bodyHtml);
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

// --- Remotetap (Live Collector Data) ---

let rtPollTimer = null;
let rtUsagePollTimer = null;
let rtRunID = null;

async function connectRemotetap() {
  const endpoint = document.getElementById('remotetap-endpoint').value;
  const connectBtn = document.getElementById('remotetap-connect-btn');
  const statusEl = document.getElementById('remotetap-status');

  const servicesInput = document.getElementById('rt-filters-services');
  const spanKindsSelect = document.getElementById('rt-filters-spankinds');
  const minLogSeveritySelect = document.getElementById('rt-filters-minseverity');
  const samplingTypeSelect = document.getElementById('rt-filters-sampling');
  const samplingRateInput = document.getElementById('rt-filters-samplingrate');
  const maxSpansInput = document.getElementById('rt-filters-maxspans');
  const maxBytesInput = document.getElementById('rt-filters-maxbytes');

  const filters = {
    services: servicesInput ? servicesInput.value.split(',').map(s => s.trim()).filter(s => s) : [],
    spanKinds: spanKindsSelect ? Array.from(spanKindsSelect.selectedOptions).map(o => o.value) : [],
    resourceAttrs: {},
    minLogSeverity: minLogSeveritySelect ? minLogSeveritySelect.value : '',
    samplingType: samplingTypeSelect ? samplingTypeSelect.value : 'none',
    samplingRate: samplingRateInput ? parseFloat(samplingRateInput.value) || 0 : 0,
    maxSpans: maxSpansInput ? parseInt(maxSpansInput.value) || 10000 : 10000,
    maxBytes: maxBytesInput ? parseInt(maxBytesInput.value) || 10485760 : 10485760,
  };

  connectBtn.disabled = true;
  statusEl.textContent = 'Connecting...';
  statusEl.className = 'status-connecting';
  try {
    const res = await fetch('/api/remotetap/connect', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({endpoint, filters})
    });
    if (res.ok) {
      const data = await res.json();
      showRemotetapConnected();
      rtRunID = data.run_id;
      pollRemotetapStatus();
      pollRemotetapUsage();
    } else {
      const err = await res.json().catch(() => ({}));
      statusEl.textContent = 'Connection failed: ' + (err.error || res.statusText);
      statusEl.className = 'status-error';
    }
  } catch (err) {
    statusEl.textContent = 'Connection error: ' + err.message;
    statusEl.className = 'status-error';
  }
  connectBtn.disabled = false;
}

async function disconnectRemotetap() {
  const disconnectBtn = document.getElementById('remotetap-disconnect-btn');
  disconnectBtn.disabled = true;
  try {
    await fetch('/api/remotetap/disconnect', { method: 'POST' });
  } catch (err) {
    /* ignore disconnect errors */
  }
  showRemotetapDisconnected();
  disconnectBtn.disabled = false;
}

async function pollRemotetapStatus() {
  if (rtPollTimer) clearInterval(rtPollTimer);
  rtPollTimer = setInterval(async () => {
    try {
      const res = await fetch('/api/remotetap/status');
      if (!res.ok) {
        showRemotetapDisconnected();
        return;
      }
      const status = await res.json();
      if (!status.connected) {
        showRemotetapDisconnected();
        return;
      }
      document.getElementById('rt-span-count').textContent = status.span_count || 0;
      document.getElementById('rt-log-count').textContent = status.log_count || 0;
      document.getElementById('rt-metric-count').textContent = status.metric_count || 0;
    } catch (err) {
      /* ignore polling errors */
    }
  }, 2000);
}

async function pollRemotetapUsage() {
  if (rtUsagePollTimer) clearInterval(rtUsagePollTimer);
  if (!rtRunID) return;
  rtUsagePollTimer = setInterval(async () => {
    try {
      const res = await fetch('/api/remotetap/usage/' + rtRunID);
      if (!res.ok) return;
      const data = await res.json();

      const usageEl = document.getElementById('remotetap-usage');
      usageEl.style.display = '';

      const spanStr = (data.spanCount || 0).toLocaleString();
      const maxSpanStr = (data.maxSpans || 10000).toLocaleString();
      document.getElementById('rt-usage-spans').textContent = spanStr + ' / ' + maxSpanStr;

      const bytes = data.currentBytes || 0;
      let bytesStr;
      if (bytes >= 1048576) {
        bytesStr = (bytes / 1048576).toFixed(1) + ' MB';
      } else if (bytes >= 1024) {
        bytesStr = (bytes / 1024).toFixed(0) + ' KB';
      } else {
        bytesStr = bytes + ' B';
      }
      const maxBytes = data.maxBytes || 10485760;
      let maxBytesStr;
      if (maxBytes >= 1048576) {
        maxBytesStr = (maxBytes / 1048576).toFixed(0) + ' MB';
      } else {
        maxBytesStr = maxBytes + ' B';
      }
      document.getElementById('rt-usage-bytes').textContent = bytesStr + ' / ' + maxBytesStr;

      document.getElementById('rt-usage-evicted').textContent = data.evictedCount || 0;

      const spanPct = data.maxSpans > 0 ? Math.min((data.spanCount || 0) / data.maxSpans, 1) : 0;
      const bytePct = data.maxBytes > 0 ? Math.min((data.currentBytes || 0) / data.maxBytes, 1) : 0;

      const spansBar = document.getElementById('rt-usage-spans-bar');
      spansBar.style.width = (spanPct * 100) + '%';
      spansBar.className = 'usage-progress-fill' + (spanPct > 0.8 ? ' danger' : spanPct > 0.6 ? ' warning' : '');

      const bytesBar = document.getElementById('rt-usage-bytes-bar');
      bytesBar.style.width = (bytePct * 100) + '%';
      bytesBar.className = 'usage-progress-fill' + (bytePct > 0.8 ? ' danger' : bytePct > 0.6 ? ' warning' : '');

      // Show warning when >80%
      const warningEl = document.getElementById('rt-usage-warning');
      if (spanPct > 0.8 || bytePct > 0.8) {
        warningEl.style.display = '';
      } else {
        warningEl.style.display = 'none';
      }
    } catch (err) {
      /* ignore polling errors */
    }
  }, 2000);
}

function showRemotetapConnected() {
  document.getElementById('remotetap-connect-btn').style.display = 'none';
  document.getElementById('remotetap-disconnect-btn').style.display = '';
  document.getElementById('remotetap-stats').style.display = '';
  document.getElementById('remotetap-usage').style.display = '';
  const statusEl = document.getElementById('remotetap-status');
  statusEl.textContent = 'Connected · Streaming';
  statusEl.className = 'status-connected';
}

function showRemotetapDisconnected() {
  if (rtPollTimer) {
    clearInterval(rtPollTimer);
    rtPollTimer = null;
  }
  if (rtUsagePollTimer) {
    clearInterval(rtUsagePollTimer);
    rtUsagePollTimer = null;
  }
  rtRunID = null;
  document.getElementById('remotetap-connect-btn').style.display = '';
  document.getElementById('remotetap-connect-btn').disabled = false;
  document.getElementById('remotetap-disconnect-btn').style.display = 'none';
  document.getElementById('remotetap-disconnect-btn').disabled = false;
  document.getElementById('remotetap-stats').style.display = 'none';
  document.getElementById('remotetap-usage').style.display = 'none';
  const statusEl = document.getElementById('remotetap-status');
  statusEl.textContent = 'Disconnected';
  statusEl.className = 'status-disconnected';
}

loadScenarios();
// Restore per-signal collapse states
['trace', 'log', 'metric'].forEach(function(signal) {
  try {
    if (localStorage.getItem('o11y-workbench-config-' + signal + '-collapsed') === '1') {
      var el = document.getElementById(signal + '-config-area');
      if (el) el.classList.add('collapsed');
    }
  } catch(e) {}
});
initChunkedBars();
document.addEventListener('mousemove', updateBarDrag);
document.addEventListener('mouseup', stopBarDrag);
document.addEventListener('touchmove', updateBarDrag, { passive: false });
document.addEventListener('touchend', stopBarDrag);
onSignalCheckboxChange();
updateSeverityBars();
updateMetricTypeBars();
updateToggleAllBtn();







// Load existing runs from the server so the run list
// is populated immediately on page load.
(async function loadExistingRuns() {
  try {
    const data = await api('/api/runs');
    if (data.runs) {
      data.runs.forEach(r => { runs[r.id] = mergeListRun(runs[r.id], r); });
      renderRunList();
    }
  } catch(e) { /* page may load before any runs exist */ }
  if (hasActiveRuns(runs)) startPolling();
})();

updateAllPillState('signal-type');
updateAllPillState('run-filter-signals');

// Initialize infinite indicators on page load
['trace-count', 'rate', 'duration'].forEach(updateInfiniteIndicator);
syncTraceCountDuration();
