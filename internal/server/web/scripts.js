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
  const workload = el.value === 'workload';
  if (el.value) {
    topo.disabled = true;
    topo.style.opacity = '0.5';
    topo.style.cursor = 'not-allowed';
  } else {
    topo.disabled = false;
    topo.style.opacity = '';
    topo.style.cursor = '';
  }
  // workload is flat evidence grammar — span tree knobs do not apply
  ['span-count', 'scenario-span-count', 'max-depth'].forEach((id) => {
    const field = document.getElementById(id);
    if (!field) return;
    field.disabled = workload;
    field.title = workload ? 'Ignored for scenario=workload (flat evidence grammar)' : '';
  });
}

async function loadScenarios(preserve = false) {
  try {
    const data = await api('/api/scenarios');
    scenarios = data.scenarios;
    const sel = document.getElementById('scenario');
    const selected = sel.value;
    while (sel.options.length > 1) sel.remove(1);
    scenarios.forEach(s => {
      const opt = document.createElement('option');
      opt.value = s;
      opt.textContent = s === 'workload' ? 'workload (flat evidence)' : s;
      sel.appendChild(opt);
    });
    sel.value = preserve ? selected : (scenarios.includes('microservice') ? 'microservice' : '');
    handleScenarioChange(sel);
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

function buildAttrRow(key, val) {
  const tr = document.createElement('tr');
  const tdKey = document.createElement('td');
  const keyInput = document.createElement('input');
  keyInput.type = 'text';
  keyInput.className = 'attr-key';
  keyInput.placeholder = 'key';
  keyInput.value = key || '';
  tdKey.appendChild(keyInput);
  const tdVal = document.createElement('td');
  const valInput = document.createElement('input');
  valInput.type = 'text';
  valInput.className = 'attr-val';
  valInput.placeholder = 'value';
  valInput.value = val || '';
  tdVal.appendChild(valInput);
  const tdBtn = document.createElement('td');
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'btn-danger';
  btn.style.cssText = 'font-size:14px;padding:2px 6px';
  btn.textContent = '\u00D7';
  btn.addEventListener('click', () => tr.remove());
  tdBtn.appendChild(btn);
  tr.append(tdKey, tdVal, tdBtn);
  return tr;
}

function addLogAttr(key, val) {
  document.getElementById('attr-body').appendChild(buildAttrRow(key, val));
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
  document.getElementById('metric-attr-body').appendChild(buildAttrRow(key, val));
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
let pollStartedAt = 0;
let idleReconcileTimer = null;

const ACTIVE_RUN_STATUSES = new Set(['pending', 'running', 'paused', 'streaming', 'stopping']);
let listRequest = 0;
let listLoading = false;
let detailRequest = 0;
let selectedRunID = null;
let detailLoading = false;

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
    if (incoming.has_trace_data && incoming.trace_data == null && prev.trace_data != null) incoming.trace_data = prev.trace_data;
    if (incoming.has_log_data && incoming.log_data == null && prev.log_data != null) incoming.log_data = prev.log_data;
    if (incoming.has_metric_data && incoming.metric_data == null && prev.metric_data != null) incoming.metric_data = prev.metric_data;
  }
  return incoming;
}

function runNeedsPayloadFetch(r) {
  return (!!r.has_trace_data && !r.trace_data) ||
    (!!r.has_log_data && !r.log_data) ||
    (!!r.has_metric_data && !r.metric_data);
}

async function fetchRunDetail(id, generation) {
  const full = await api('/api/runs/' + encodeURIComponent(id));
  if (selectedRunID !== id || generation !== detailRequest) return null;
  const prev = runs[id];
  runs[id] = prev ? { ...prev, ...full } : full;
  return runs[id];
}

function numericInput(id, fallback, parse = Number) {
  const value = document.getElementById(id).value.trim();
  if (!value) return fallback;
  const parsed = parse(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

async function startRun() {
  const runBtn = document.getElementById('run-btn');
  runBtn.disabled = true;
  const statusEl = document.getElementById('run-status');
  statusEl.textContent = 'Starting...';

  const signals = getSignals();

  var body = {
    seed: numericInput('seed', 42, Number),
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
      value_max: numericInput('metric-value-max', 100),
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
    status.replaceChildren();
    const okSpan = document.createElement('span');
    okSpan.style.color = '#22c55e';
    okSpan.textContent = 'Imported ' + (run.span_count || 0) + ' spans';
    status.appendChild(okSpan);
    setTimeout(() => closeImportModal(), 800);
  } catch(e) {
    status.replaceChildren();
    const errSpan = document.createElement('span');
    errSpan.style.color = '#ef4444';
    errSpan.textContent = 'Error: ' + e.message;
    status.appendChild(errSpan);
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
    status.replaceChildren();
    const regSpan = document.createElement('span');
    regSpan.style.color = '#22c55e';
    regSpan.textContent = 'Registered: ' + (data.registered || []).join(', ');
    status.appendChild(regSpan);
    setTimeout(() => closeScenarioModal(), 800);
    await loadScenarios(true);
  } catch(e) {
    status.replaceChildren();
    const errSpan = document.createElement('span');
    errSpan.style.color = '#ef4444';
    errSpan.textContent = 'Error: ' + e.message;
    status.appendChild(errSpan);
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

async function stopTapRun(id) {
  try {
    await api('/api/remotetap/stop/' + encodeURIComponent(id), { method: 'POST' });
    await restoreRemotetapStatus();
    refreshRuns();
  } catch (e) {
    if (selectedRunID === id) {
      document.getElementById('detail-content').textContent = 'Failed to stop live capture: ' + e.message;
      lastDetailKey = '';
    }
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
  const icon = document.createElement('img');
  icon.src = '/assets/favicon.svg';
  icon.alt = '';
  icon.className = 'toast-icon';
  icon.width = 28;
  icon.height = 28;
  const text = document.createElement('span');
  text.className = 'toast-text';
  text.appendChild(document.createTextNode('Olly the otter finished '));
  const strong = document.createElement('strong');
  strong.textContent = run.label || 'run #' + run.id.slice(0, 8);
  text.appendChild(strong);
  toast.append(icon, text);
  document.body.appendChild(toast);

  setTimeout(() => {
    toast.classList.add('out');
    setTimeout(() => toast.remove(), 300);
  }, 3000);
}

const announcedCompleted = new Set();
async function refreshRuns() {
  if (listLoading) return;
  listLoading = true;
  const generation = ++listRequest;
  const requestStartedAt = Date.now();
  try {
    const data = await api('/api/runs');
    if (generation !== listRequest) return;
    const next = {};
    for (const r of (data.runs || [])) {
      const prev = runs[r.id];
      next[r.id] = mergeListRun(prev, r);
      if (r.status === 'completed' && prev && prev.status !== 'completed' && !announcedCompleted.has(r.id)) {
        announcedCompleted.add(r.id);
        showOtterToast(r);
      }
    }
    // A job created while this list request was in flight is not in the
    // response yet; keep it instead of dropping it (and its polling).
    for (const [id, prev] of Object.entries(runs)) {
      if (next[id]) continue;
      const startedAt = Date.parse(prev.started_at || '');
      if (!Number.isNaN(startedAt) && startedAt >= requestStartedAt - 1000) next[id] = prev;
    }
    runs = next;
    renderRunList();
    if (selectedRunID && !detailLoading) {
      const id = selectedRunID;
      const r = runs[id];
      if (!r) {
        ++detailRequest;
        document.getElementById('detail-content').textContent = 'This job is no longer available.';
      } else if (runNeedsPayloadFetch(r)) {
        const request = ++detailRequest;
        try {
          const full = await fetchRunDetail(id, request);
          if (full) renderDetail(full);
        } catch (e) {
          if (selectedRunID === id && request === detailRequest) {
            document.getElementById('detail-content').textContent = 'Failed to refresh run detail: ' + e.message;
            lastDetailKey = '';
          }
        }
      } else {
        renderDetail(r);
      }
    }
  } catch(e) {
    console.error('Polling error:', e);
  } finally {
    listLoading = false;
    if (hasActiveRuns(runs)) {
      if (!pollTimer) startPolling();
    } else if (Date.now() - pollStartedAt < 1500) {
      // Keep the first polling window open so progress is observable for fast
      // runs; idle backoff starts after that short grace period.
    } else {
      stopPolling();
      scheduleIdleReconcile();
    }
  }
}

function startPolling() {
  if (idleReconcileTimer) {
    clearTimeout(idleReconcileTimer);
    idleReconcileTimer = null;
  }
  if (pollTimer) return;
  pollStartedAt = Date.now();
  refreshRuns();
  pollTimer = setInterval(refreshRuns, 1000);
}

// Idle runs still discover externally created jobs and pruning without polling
// the list every second; reconciliation resumes fast polling when work appears.
function scheduleIdleReconcile() {
  if (idleReconcileTimer) return;
  idleReconcileTimer = setTimeout(() => {
    idleReconcileTimer = null;
    void refreshRuns();
  }, 15000);
}

let lastDetailKey = '';

function getRunSignals(r) {
  if (r.source === 'import' || r.output === 'import') {
    return { traces: true, logs: false, metrics: false };
  }
  if (r.source === 'remotetap') {
    return { traces: true, logs: true, metrics: true };
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
var _runListDelegated = false;

function appendTrustedHtml(parent, html) {
  const tpl = document.createElement('template');
  tpl.innerHTML = html.trim();
  const node = tpl.content.firstChild;
  if (node) parent.appendChild(node);
}

function buildRunCard(r) {
  const card = document.createElement('div');
  card.className = 'run-card' +
    (r.status === 'running' ? ' running' : '') +
    (r.status === 'completed' ? ' completed' : '');
  card.dataset.runId = r.id;
  card.tabIndex = 0;
  card.setAttribute('role', 'button');
  card.setAttribute('aria-label', 'Open job ' + r.id);

  appendTrustedHtml(card, getSignalBadge(getRunSignals(r)));

  const statusDot = document.createElement('div');
  statusDot.className = 'status ' + statusClass(r.status);
  card.appendChild(statusDot);

  const scenario = r.scenario || r.config?.Scenario || '';
  const isImported = r.source === 'import' || r.output === 'import';
  const metaParts = [formatTime(r.started_at), r.status];
  if (isImported) {
    metaParts.push('imported');
  } else if (r.source === 'remotetap') {
    metaParts.push('live collector');
  } else {
    if (r.config?.SpanCount) metaParts.push(r.config.SpanCount + ' span budget');
    var tc = r.config?.TraceCount;
    if (tc !== undefined) metaParts.push((tc === 0 ? '\u221e' : tc) + ' traces');
  }

  const info = document.createElement('div');
  info.className = 'info';
  const idDiv = document.createElement('div');
  idDiv.className = 'id';
  idDiv.textContent = r.id + (scenario ? ' \u00B7 ' + scenario : '');
  const metaDiv = document.createElement('div');
  metaDiv.className = 'meta';
  metaDiv.textContent = metaParts.join(' \u00B7 ');
  info.append(idDiv, metaDiv);
  card.appendChild(info);

  const stats = document.createElement('div');
  stats.className = 'stats';
  const spanStat = document.createElement('span');
  spanStat.textContent = (r.span_count || 0) + ' spans';
  stats.appendChild(spanStat);
  if (r.log_count || r.log_count_completed) {
    const s = document.createElement('span');
    s.textContent = (r.log_count || r.log_count_completed) + ' logs';
    stats.appendChild(s);
  }
  if (r.metric_count || r.metric_count_completed) {
    const s = document.createElement('span');
    s.textContent = (r.metric_count || r.metric_count_completed) + ' metrics';
    stats.appendChild(s);
  }
  const durSpan = document.createElement('span');
  durSpan.innerHTML = formatDuration(r.duration_ns);
  stats.appendChild(durSpan);
  card.appendChild(stats);

  if (r.status === 'running') {
    const btn = document.createElement('button');
    btn.className = 'btn-pause';
    btn.dataset.runAction = 'pause';
    btn.textContent = 'Pause';
    card.appendChild(btn);
  } else if (r.status === 'paused') {
    const btn = document.createElement('button');
    btn.className = 'btn-pause';
    btn.dataset.runAction = 'resume';
    btn.textContent = 'Resume';
    card.appendChild(btn);
  }
  if (r.status === 'running' || r.status === 'pending' || r.status === 'paused') {
    const btn = document.createElement('button');
    btn.className = 'btn-danger';
    btn.dataset.runAction = 'stop';
    btn.textContent = 'Stop';
    card.appendChild(btn);
  }
  return card;
}

function renderRunList() {
  const list = document.getElementById('run-list');
  const search = (document.getElementById('filter-search').value || '').toLowerCase();
  let entries = Object.values(runs).sort((a, b) => new Date(b.started_at) - new Date(a.started_at));

  const checkedSignals = [];
  document.querySelectorAll('#run-filter-signals input[type="checkbox"]:checked').forEach(c => checkedSignals.push(c.value));
  if (checkedSignals.length > 0 && checkedSignals.length < 3) {
    entries = entries.filter(r => {
      const signals = getRunSignals(r);
      return (checkedSignals.includes('traces') && (signals.traces || r.has_trace_data || r.trace_data)) ||
        (checkedSignals.includes('logs') && (signals.logs || r.has_log_data || r.log_data)) ||
        (checkedSignals.includes('metrics') && (signals.metrics || r.has_metric_data || r.metric_data));
    });
  }
  if (search) {
    entries = entries.filter(r => r.id.toLowerCase().includes(search) || (r.scenario || '').toLowerCase().includes(search));
  }

  if (_emptyStateHTML === null) {
    _emptyStateHTML = list.innerHTML;
  }
  const allRuns = Object.values(runs);
  document.getElementById('pause-all-btn').style.display = allRuns.some(r => r.status === 'running') ? '' : 'none';
  document.getElementById('resume-all-btn').style.display = allRuns.some(r => r.status === 'paused') ? '' : 'none';
  document.getElementById('stop-all-btn').style.display = allRuns.some(r => ['running', 'paused', 'pending'].includes(r.status)) ? '' : 'none';

  if (entries.length === 0) {
    if (Object.keys(runs).length === 0) {
      list.innerHTML = _emptyStateHTML;
      return;
    }
    list.innerHTML = '<div class="empty"><p>No jobs match the current filter.</p></div>';
    return;
  }
  const focused = document.activeElement;
  const focusedCard = focused && focused.closest('.run-card');
  const focusedID = focusedCard && focusedCard.dataset.runId;
  const focusedAction = focused && focused.dataset.runAction;
  list.replaceChildren();
  entries.forEach((r) => list.appendChild(buildRunCard(r)));
  if (focusedID) {
    const card = Array.from(list.children).find(node => node.dataset.runId === focusedID);
    if (card) {
      const target = focusedAction ? Array.from(card.querySelectorAll('[data-run-action]')).find(node => node.dataset.runAction === focusedAction) : card;
      (target || card).focus({ preventScroll: true });
    }
  }
  if (!_runListDelegated) {
    _runListDelegated = true;
    list.addEventListener('click', (e) => {
      const card = e.target.closest('.run-card');
      if (!card) return;
      const id = card.dataset.runId;
      const actionBtn = e.target.closest('[data-run-action]');
      if (actionBtn) {
        e.stopPropagation();
        const action = actionBtn.dataset.runAction;
        if (action === 'pause') pauseRun(id);
        else if (action === 'resume') resumeRun(id);
        else if (action === 'stop') stopRun(id);
        return;
      }
      if (!e.target.closest('button')) showDetail(id);
    });
    list.addEventListener('keydown', (e) => {
      if ((e.key === 'Enter' || e.key === ' ') && e.target.classList.contains('run-card')) {
        e.preventDefault();
        showDetail(e.target.dataset.runId);
      }
    });
  }
}

async function showDetail(id) {
  const section = document.getElementById('detail-section');
  selectedRunID = id;
  const request = ++detailRequest;
  detailLoading = true;
  section.classList.remove('hidden');
  document.getElementById('detail-id').textContent = id;
  document.getElementById('detail-content').textContent = 'Loading...';
  lastDetailKey = '';
  section.scrollIntoView({ behavior: 'smooth' });
  try {
    const r = await fetchRunDetail(id, request);
    if (selectedRunID === id && request === detailRequest) detailLoading = false;
    if (r) renderDetail(r);
  } catch (e) {
    if (selectedRunID === id && request === detailRequest) detailLoading = false;
    if (selectedRunID === id && request === detailRequest) {
      document.getElementById('detail-content').textContent = 'Failed to load run detail: ' + e.message;
    }
  }
}

let detailTab = 'traces';

function formatCaptureDropped(dropped) {
  if (dropped == null || typeof dropped !== 'object' || Array.isArray(dropped)) return '';
  var parts = [];
  if (dropped.traces) parts.push(dropped.traces + ' spans');
  if (dropped.logs) parts.push(dropped.logs + ' log records');
  if (dropped.metrics) parts.push(dropped.metrics + ' data points');
  return parts.join(', ');
}

function captureTruncationMessage(r) {
  var dropped = formatCaptureDropped(r.capture_dropped) || '0 items';
  var limit = r.capture_limit_bytes || 0;
  return 'Capture truncated: ' + dropped + ' not retained (per-signal limit ' + limit + ' bytes). Summaries below show retained capture only.';
}

function appendCaptureWarning(el, r) {
  if (!r.capture_truncated) return;
  const warning = document.createElement('div');
  warning.className = 'error';
  warning.textContent = captureTruncationMessage(r);
  el.appendChild(warning);
}

function renderDetail(r) {
  // Build a key from the run's mutable fields. If unchanged since last
  // render, skip DOM replacement to preserve scroll position in <pre>.
  const key = JSON.stringify(r) + '|' + detailTab;
  if (key === lastDetailKey) return;
  lastDetailKey = key;

  const el = document.getElementById('detail-content');
  const signals = getRunSignals(r);
  const hasLogs = r.has_log_data || r.log_data || (r.log_count && r.log_count > 0);

  if (['pending', 'streaming', 'stopping'].includes(r.status)) {
    const label = {pending: 'Pending', streaming: 'Streaming', stopping: 'Stopping'}[r.status];
    el.textContent = label + ' — ' + (r.span_count || 0) + ' spans, ' +
      (r.log_count_completed || r.log_count || 0) + ' logs, ' +
      (r.metric_count_completed || r.metric_count || 0) + ' metrics';
    if (r.capture_truncated) {
      el.textContent += ' — ' + captureTruncationMessage(r);
    }
    if (r.status === 'pending' || (r.status === 'streaming' && r.source === 'remotetap')) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'btn btn-secondary';
      button.dataset.detailAction = r.status === 'streaming' ? 'stop-tap' : 'stop';
      button.dataset.runId = r.id;
      button.textContent = 'Stop';
      el.appendChild(button);
    }
    return;
  }
  // Running state
  if (r.status === 'running') {
    const done = r.trace_count_completed || 0;
    const total = r.trace_count;
    const tracesLabel = total === 0 ? `${done} / &infin;` : `${done} / ${total}`;
    var progressHtml = signals.traces ? '<div class="stat"><div class="value">' + tracesLabel + '</div><div class="label">Traces</div></div>' : '';
    if (hasLogs || signals.logs) {
      var logDone = r.log_count_completed || 0;
      var logTotal = r.log_count || 0;
      progressHtml += '<div class="stat"><div class="value">' + logDone + ' / ' + (logTotal || '?') + '</div><div class="label">Logs</div></div>';
    }
    if (signals.metrics) {
      var metricDone = r.metric_count_completed || 0;
      progressHtml += '<div class="stat"><div class="value">' + metricDone + ' / ' + (r.metric_count || '?') + '</div><div class="label">Metrics</div></div>';
    }
    el.innerHTML = '<div class="stat-row"><div class="stat"><div class="value"><span class="status-running" style="display:inline-block;width:12px;height:12px;border-radius:50%;margin-right:8px;vertical-align:middle"></span>Running</div><div class="label">Status</div></div>' + progressHtml + '</div><div style="margin-top:12px"><button type="button" class="btn btn-secondary" data-detail-action="pause" data-run-id="' + escapeAttr(r.id) + '">Pause</button><button type="button" class="btn btn-secondary" data-detail-action="stop" data-run-id="' + escapeAttr(r.id) + '" style="margin-left:8px">Stop</button></div>';
    appendCaptureWarning(el, r);
    return;
  }

  // Paused state
  if (r.status === 'paused') {
    const done = r.trace_count_completed || 0;
    const total = r.trace_count;
    const tracesLabel = total === 0 ? `${done} / &infin;` : `${done} / ${total}`;
    var pausedHtml = signals.traces ? '<div class="stat"><div class="value">' + tracesLabel + '</div><div class="label">Traces</div></div>' : '';
    if (hasLogs || signals.logs) {
      pausedHtml += '<div class="stat"><div class="value">' + (r.log_count_completed || 0) + ' / ' + (r.log_count || '?') + '</div><div class="label">Logs</div></div>';
    }
    if (signals.metrics) {
      pausedHtml += '<div class="stat"><div class="value">' + (r.metric_count_completed || 0) + ' / ' + (r.metric_count || '?') + '</div><div class="label">Metrics</div></div>';
    }
    el.innerHTML = '<div class="stat-row"><div class="stat"><div class="value"><span class="status-paused" style="display:inline-block;width:12px;height:12px;border-radius:50%;margin-right:8px;vertical-align:middle"></span>Paused</div><div class="label">Status</div></div>' + pausedHtml + '</div><div style="margin-top:12px"><button type="button" class="btn btn-secondary" data-detail-action="resume" data-run-id="' + escapeAttr(r.id) + '">Resume</button><button type="button" class="btn btn-secondary" data-detail-action="stop" data-run-id="' + escapeAttr(r.id) + '" style="margin-left:8px">Stop</button></div>';
    appendCaptureWarning(el, r);
    return;
  }

  // Failed state
  if (r.status === 'failed') {
    el.innerHTML = '<div class="stat-row"><div class="stat"><div class="value" style="color:#ef4444">Failed</div><div class="label">Status</div></div><div class="stat"><div class="value">' + formatDuration(r.duration_ns) + '</div><div class="label">Duration</div></div></div><div class="error">' + escapeHtml(r.error || 'Unknown error') + '</div>';
    appendCaptureWarning(el, r);
    return;
  }


  // ===== Completed status =====
  // Counts come pre-computed from the Go backend; do not re-derive client-side.
  var spanCountTotal = r.span_count || 0;
  var logRecordCount = r.log_record_count || 0;
  var logSeverities = r.log_severity_counts || {};
  var metricRecordCount = r.metric_series_count || 0;

  const isImported = r.source === 'import' || r.output === 'import';
  var summaryHtml = '<div class="stat-row">';
  summaryHtml += '<div class="stat"><div class="value" style="color:#22c55e">' + (r.status === 'stopped' ? 'Stopped' : 'Completed') + '</div><div class="label">Status</div></div>';
  summaryHtml += '<div class="stat"><div class="value">' + (spanCountTotal || r.span_count || 0) + '</div><div class="label">Spans (retained)</div></div>';
  if (r.source === 'remotetap' || r.source === 'import' || r.output === 'import') {
    summaryHtml += '<div class="stat"><div class="value">' + (r.trace_count || 0) + '</div><div class="label">Traces (retained)</div></div>';
  }
  if (logRecordCount > 0) {
    summaryHtml += '<div class="stat"><div class="value">' + logRecordCount + '</div><div class="label">Log records (retained)</div></div>';
  }
  if (metricRecordCount > 0) {
    summaryHtml += '<div class="stat"><div class="value">' + metricRecordCount + '</div><div class="label">Data points (retained)</div></div>';
  }
  summaryHtml += '<div class="stat"><div class="value">' + formatDuration(r.duration_ns) + '</div><div class="label">Duration</div></div>';
  summaryHtml += '<div class="stat"><div class="value">' + (isImported ? '-' : (r.config?.Seed ?? '-')) + '</div><div class="label">Seed</div></div>';
  summaryHtml += '</div>';
  if (r.generator_stats) {
    var gs = r.generator_stats;
    summaryHtml += '<div class="stat-row" style="margin-top:8px">';
    summaryHtml += '<div class="stat"><div class="value">' + (gs.acked_requests ?? 0) + '/' + (gs.attempted_requests ?? 0) + '</div><div class="label">Req acked/attempted</div></div>';
    summaryHtml += '<div class="stat"><div class="value">' + (gs.acked_items ?? 0) + '/' + (gs.attempted_items ?? 0) + '</div><div class="label">Items acked/attempted</div></div>';
    summaryHtml += '<div class="stat"><div class="value">' + (gs.failed_requests ?? 0) + '</div><div class="label">Failed requests</div></div>';
    summaryHtml += '</div>';
  }

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
  const availableTabs = [hasTraceData && 'traces', hasLogData && 'logs', hasMetricData && 'metrics'].filter(Boolean);
  if (!availableTabs.includes(detailTab)) detailTab = availableTabs[0] || 'traces';
  if (r.capture_truncated) {
    summaryHtml += '<div class="error">' + escapeHtml(captureTruncationMessage(r)) + '</div>';
  }
  var tabHtml = '<div class="detail-tabs">';
  if (hasTraceData) {
    tabHtml += '<button type="button" class="detail-tab' + (detailTab === 'traces' ? ' active' : '') + '" data-tab="traces">Spans<span class="tab-badge">' + (spanCountTotal || r.span_count || 0) + '</span></button>';
  }
  if (hasLogData) {
    tabHtml += '<button type="button" class="detail-tab' + (detailTab === 'logs' ? ' active' : '') + '" data-tab="logs">Logs<span class="tab-badge">' + logRecordCount + '</span></button>';
  }
  if (hasMetricData) {
    tabHtml += '<button type="button" class="detail-tab' + (detailTab === 'metrics' ? ' active' : '') + '" data-tab="metrics">Metrics<span class="tab-badge">' + metricRecordCount + '</span></button>';
  }
  tabHtml += '</div>';

  el.innerHTML = summaryHtml + tabHtml;
  if (availableTabs.length) {
    const download = document.createElement('a');
    const format = r.format === 'protobuf' ? 'protobuf' : 'json';
    download.href = '/api/runs/' + encodeURIComponent(r.id) + '/download?signal=' + encodeURIComponent(detailTab) + '&format=' + format;
    download.textContent = 'Download ' + detailTab + ' (' + format.toUpperCase() + ')';
    download.className = 'btn btn-secondary';
    download.setAttribute('download', '');
    el.appendChild(download);
  }

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

      bodyHtml += '<tr class="log-row" data-log-idx="' + idx + '">';
      bodyHtml += '<td><span class="severity-tag ' + sevClass + '">' + escapeHtml(lr.severityText || 'INFO') + '</span></td>';
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

// HTML text escaping does not cover quotes, so attribute values need their own
// helper before they are interpolated into markup.
function escapeAttr(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

let _detailContentDelegated = false;

function wireDetailContentDelegation() {
  if (_detailContentDelegated) return;
  _detailContentDelegated = true;
  const el = document.getElementById('detail-content');
  if (!el) return;
  el.addEventListener('click', (e) => {
    const tab = e.target.closest('.detail-tab[data-tab]');
    if (tab) {
      switchDetailTab(tab.dataset.tab);
      return;
    }
    const actionBtn = e.target.closest('[data-detail-action]');
    if (actionBtn) {
      const action = actionBtn.dataset.detailAction;
      const id = actionBtn.dataset.runId;
      if (action === 'pause') pauseRun(id);
      else if (action === 'resume') resumeRun(id);
      else if (action === 'stop') stopRun(id);
      else if (action === 'stop-tap') stopTapRun(id);
      return;
    }
    const logRow = e.target.closest('tr.log-row[data-log-idx]');
    if (logRow) toggleLogDetail(logRow, parseInt(logRow.dataset.logIdx, 10));
  });
}

function wireInfiniteField(inputId, overlayId, activateOnFocus) {
  const input = document.getElementById(inputId);
  const overlay = document.getElementById(overlayId);
  if (!input) return;
  const sync = () => {
    updateInfiniteIndicator(inputId);
    if (inputId === 'trace-count' || inputId === 'duration') {
      syncTraceCountDuration(inputId);
    }
  };
  input.addEventListener('input', sync);
  input.addEventListener('blur', sync);
  if (overlay) {
    overlay.addEventListener('click', () => focusInfiniteInput(inputId, activateOnFocus));
  }
}

function wireBarDragTracks(gridId, prefix) {
  const grid = document.getElementById(gridId);
  if (!grid) return;
  grid.querySelectorAll('.sev-bar-track').forEach((track) => {
    const input = track.parentElement && track.parentElement.querySelector('input[type="number"]');
    if (!input || !input.id) return;
    const id = input.id.replace(prefix === 'sev' ? 'sev-' : 'metric-', '');
    track.addEventListener('mousedown', (e) => startBarDrag(e, prefix, id));
    track.addEventListener('touchstart', (e) => startBarDrag(e, prefix, id));
    input.addEventListener('input', () => {
      if (prefix === 'sev') onSeverityInput(id);
      else onMetricTypeInput(id);
    });
  });
}

function wireStaticDashboardUI() {
  const partyEq = document.querySelector('.signal-eq');
  if (partyEq) partyEq.addEventListener('click', togglePartyMode);

  const selectAllBtn = document.querySelector('#signal-type .pill-all');
  if (selectAllBtn) selectAllBtn.addEventListener('click', selectAllSignals);
  document.querySelectorAll('#signal-type input[type="checkbox"]').forEach((cb) => {
    cb.addEventListener('click', (e) => onSignalPillClick(e, cb.value));
  });
  const toggleAllBtn = document.getElementById('toggle-all-btn');
  if (toggleAllBtn) toggleAllBtn.addEventListener('click', toggleAllSignalConfigs);

  ['trace', 'log', 'metric'].forEach((signal) => {
    const area = document.getElementById(signal + '-config-area');
    const header = area && area.querySelector('.signal-config-header');
    if (header) header.addEventListener('click', () => toggleSignalConfig(signal));
  });

  wireInfiniteField('trace-count', 'trace-count-inf', 1);
  wireInfiniteField('rate', 'rate-inf');
  wireInfiniteField('duration', 'duration-inf', 1);

  const scenarioSel = document.getElementById('scenario');
  if (scenarioSel) scenarioSel.addEventListener('change', () => handleScenarioChange(scenarioSel));

  wireBarDragTracks('severity-grid', 'sev');
  wireBarDragTracks('metric-type-grid', 'metric');

  const addLogBtn = document.getElementById('add-log-attr-btn');
  if (addLogBtn) addLogBtn.addEventListener('click', () => addLogAttr());
  const addMetricBtn = document.getElementById('add-metric-attr-btn');
  if (addMetricBtn) addMetricBtn.addEventListener('click', () => addMetricAttr('', ''));

  const runBtn = document.getElementById('run-btn');
  if (runBtn) runBtn.addEventListener('click', startRun);
  const importOpenBtn = document.getElementById('open-import-modal-btn');
  if (importOpenBtn) importOpenBtn.addEventListener('click', openImportModal);
  const scenarioOpenBtn = document.getElementById('open-scenario-modal-btn');
  if (scenarioOpenBtn) scenarioOpenBtn.addEventListener('click', openScenarioModal);

  const pauseAllBtn = document.getElementById('pause-all-btn');
  if (pauseAllBtn) pauseAllBtn.addEventListener('click', pauseAllRuns);
  const resumeAllBtn = document.getElementById('resume-all-btn');
  if (resumeAllBtn) resumeAllBtn.addEventListener('click', resumeAllRuns);
  const stopAllBtn = document.getElementById('stop-all-btn');
  if (stopAllBtn) stopAllBtn.addEventListener('click', stopAllRuns);

  const resetFilterBtn = document.querySelector('#run-filter-signals .pill-all');
  if (resetFilterBtn) resetFilterBtn.addEventListener('click', resetSignalFilter);
  document.querySelectorAll('#run-filter-signals input[type="checkbox"]').forEach((cb) => {
    cb.addEventListener('click', (e) => onFilterSignalPillClick(e, cb.value));
  });
  const filterSearch = document.getElementById('filter-search');
  if (filterSearch) filterSearch.addEventListener('input', renderRunList);

  const importOverlay = document.getElementById('import-modal-overlay');
  if (importOverlay) {
    importOverlay.addEventListener('click', (e) => {
      if (e.target === importOverlay) closeModal('import-modal-overlay');
    });
    importOverlay.querySelectorAll('.modal-close, .import-modal-cancel').forEach((btn) => {
      btn.addEventListener('click', () => closeModal('import-modal-overlay'));
    });
  }
  const importBtn = document.getElementById('import-btn');
  if (importBtn) importBtn.addEventListener('click', importTraces);

  const scenarioOverlay = document.getElementById('scenario-modal-overlay');
  if (scenarioOverlay) {
    scenarioOverlay.addEventListener('click', (e) => {
      if (e.target === scenarioOverlay) closeModal('scenario-modal-overlay');
    });
    scenarioOverlay.querySelectorAll('.modal-close, .scenario-modal-cancel').forEach((btn) => {
      btn.addEventListener('click', () => closeModal('scenario-modal-overlay'));
    });
  }
  const scenarioUploadBtn = document.getElementById('scenario-upload-btn');
  if (scenarioUploadBtn) scenarioUploadBtn.addEventListener('click', uploadScenarios);

  const rtConnect = document.getElementById('remotetap-connect-btn');
  if (rtConnect) rtConnect.addEventListener('click', connectRemotetap);
  const rtDisconnect = document.getElementById('remotetap-disconnect-btn');
  if (rtDisconnect) rtDisconnect.addEventListener('click', disconnectRemotetap);
  const rtFiltersHeader = document.querySelector('.remotetap-filters-header');
  if (rtFiltersHeader) rtFiltersHeader.addEventListener('click', toggleRemotetapFilters);

  wireDetailContentDelegation();
}

function toggleRemotetapFilters() {
  document.getElementById('remotetap-filters').classList.toggle('collapsed');
}

// --- Remotetap (Live Collector Data) ---

let rtPollTimer = null;
let rtUsagePollTimer = null;
let rtRunID = null;
let rtBusy = false;
let rtGeneration = 0;

async function connectRemotetap() {
  rtBusy = true;
  ++rtGeneration;
  const endpoint = document.getElementById('remotetap-endpoint').value;
  const connectBtn = document.getElementById('remotetap-connect-btn');
  const statusEl = document.getElementById('remotetap-status');

  const servicesInput = document.getElementById('rt-filters-services');
  const spanKindsSelect = document.getElementById('rt-filters-spankinds');
  const minLogSeveritySelect = document.getElementById('rt-filters-minlogseverity');
  const samplingTypeSelect = document.getElementById('rt-filters-samplingtype');

  const filters = {
    services: servicesInput ? servicesInput.value.split(',').map(s => s.trim()).filter(s => s) : [],
    spanKinds: spanKindsSelect ? Array.from(spanKindsSelect.selectedOptions).map(o => o.value) : [],
    resourceAttrs: {},
    minLogSeverity: minLogSeveritySelect ? minLogSeveritySelect.value : '',
    samplingType: samplingTypeSelect ? samplingTypeSelect.value : 'none',
    samplingRate: numericInput('rt-filters-samplingrate', 0),
    maxSpans: numericInput('rt-filters-maxspans', 10000),
    maxBytes: numericInput('rt-filters-maxbytes', 10485760),
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
      refreshRuns();
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
  rtBusy = false;
}

async function disconnectRemotetap() {
  rtBusy = true;
  ++rtGeneration;
  const disconnectBtn = document.getElementById('remotetap-disconnect-btn');
  disconnectBtn.disabled = true;
  try {
    await api('/api/remotetap/disconnect', { method: 'POST' });
    showRemotetapDisconnected();
    refreshRuns();
  } catch (err) {
    const statusEl = document.getElementById('remotetap-status');
    statusEl.textContent = 'Disconnect failed: ' + err.message;
    statusEl.className = 'status-error';
  }
  disconnectBtn.disabled = false;
  rtBusy = false;
}

async function restoreRemotetapStatus() {
  if (rtBusy) return;
  const generation = rtGeneration;
  try {
    const status = await api('/api/remotetap/status');
    if (generation !== rtGeneration) return;
    if (!status.connected) {
      if (rtRunID) refreshRuns();
      showRemotetapDisconnected();
      pollRemotetapStatus();
      return;
    }
    const changed = rtRunID !== status.run_id;
    rtRunID = status.run_id;
    showRemotetapConnected();
    document.getElementById('rt-span-count').textContent = status.span_count || 0;
    document.getElementById('rt-log-count').textContent = status.log_count || 0;
    document.getElementById('rt-metric-count').textContent = status.metric_count || 0;
    if (changed) {
      refreshRuns();
      pollRemotetapUsage();
    }
    pollRemotetapStatus();
  } catch (err) {
    if (generation !== rtGeneration) return;
    const statusEl = document.getElementById('remotetap-status');
    statusEl.textContent = 'Status unavailable: ' + err.message;
    statusEl.className = 'status-error';
    pollRemotetapStatus();
  }
}

function pollRemotetapStatus() {
  if (!rtPollTimer) rtPollTimer = setInterval(restoreRemotetapStatus, 2000);
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

wireStaticDashboardUI();
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







startPolling();
scheduleIdleReconcile();
restoreRemotetapStatus();

updateAllPillState('signal-type');
updateAllPillState('run-filter-signals');

// Initialize infinite indicators on page load
['trace-count', 'rate', 'duration'].forEach(updateInfiniteIndicator);
syncTraceCountDuration();
