const API = '';

async function api(path, opts) {
  const res = await fetch(API + path, opts);
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || (Array.isArray(data.errors) && data.errors[0]) || res.statusText);
  return data;
}

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

// --- Signal type UI ---

function getSignalType() {
  const checks = document.querySelectorAll('#signal-type input[type="checkbox"]');
  const checked = [];
  checks.forEach(c => { if (c.checked) checked.push(c.value); });
  if (checked.length === 0) return 'traces'; // default — shouldn't happen since traces can't be unchecked alone
  if (checked.length === 1) return checked[0]; // 'traces', 'logs', or 'metrics'
  if (checked.includes('traces') && checked.includes('logs') && checked.includes('metrics')) return 'all';
  if (checked.includes('traces') && checked.includes('logs')) return 'both';
  if (checked.includes('traces') && checked.includes('metrics')) return 'traces+metrics';
  if (checked.includes('logs') && checked.includes('metrics')) return 'logs+metrics';
  return 'traces';
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
    try { localStorage.removeItem('otel-workbench-config-trace-collapsed'); } catch(e) {}
  }
  // Show/hide trace-specific fields
  document.getElementById('trace-fields-group').classList.toggle('hidden', !hasTraces);
  // Show/hide log config area — only auto-expand when this signal was just toggled on
  var logArea = document.getElementById('log-config-area');
  logArea.style.display = hasLogs ? '' : 'none';
  if (changed === 'logs' && hasLogs && logArea.classList.contains('collapsed')) {
    logArea.classList.remove('collapsed');
    try { localStorage.removeItem('otel-workbench-config-log-collapsed'); } catch(e) {}
  }
  document.getElementById('log-per-span-field').style.display = (hasTraces && hasLogs) ? 'block' : 'none';
  document.getElementById('log-count-field').style.display = hasLogs ? 'block' : 'none';
  // Show/hide metric config area — only auto-expand when this signal was just toggled on
  var metricArea = document.getElementById('metric-config-area');
  metricArea.style.display = hasMetrics ? '' : 'none';
  if (changed === 'metrics' && hasMetrics && metricArea.classList.contains('collapsed')) {
    metricArea.classList.remove('collapsed');
    try { localStorage.removeItem('otel-workbench-config-metric-collapsed'); } catch(e) {}
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
      try { localStorage.removeItem('otel-workbench-config-' + signal + '-collapsed'); } catch(e) {}
    } else {
      el.classList.add('collapsed');
      try { localStorage.setItem('otel-workbench-config-' + signal + '-collapsed', '1'); } catch(e) {}
    }
  });
  document.getElementById('toggle-all-btn').textContent = anyCollapsed ? 'Collapse All' : 'Expand All';
}

function toggleSignalConfig(signal) {
  const area = document.getElementById(signal + '-config-area');
  if (!area) return;
  const isCollapsed = area.classList.toggle('collapsed');
  const key = 'otel-workbench-config-' + signal + '-collapsed';
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

async function startRun() {
  const runBtn = document.getElementById('run-btn');
  runBtn.disabled = true;
  const statusEl = document.getElementById('run-status');
  statusEl.textContent = 'Starting...';

  const signalType = getSignalType();
  const hasTraces = document.querySelector('#signal-type input[value="traces"]').checked;
  const hasLogs = document.querySelector('#signal-type input[value="logs"]').checked;
  const hasMetrics = document.querySelector('#signal-type input[value="metrics"]').checked;

  var body = {
    seed: parseInt(document.getElementById('seed').value) || 42,
    output: document.getElementById('output').value,
    format: document.getElementById('format').value,
    insecure: document.getElementById('insecure').checked,
    endpoints: document.getElementById('endpoints').value.split(',').map(s => s.trim()).filter(s => s),
    signal_type: signalType
  };

  if (hasTraces) {
    body.trace_count = (function() { var v = parseInt(document.getElementById('trace-count').value); return isNaN(v) || v < 1 ? (v === 0 ? 0 : 1) : v; })();
    body.rate = parseInt(document.getElementById('rate').value) || 0;
    body.topology = document.getElementById('topology').value;
    body.scenario = document.getElementById('scenario').value;
    body.span_count = parseInt(document.getElementById('span-count').value) || 10;
    body.scenario_span_count = parseInt(document.getElementById('scenario-span-count').value) || 0;
    body.max_depth = parseInt(document.getElementById('max-depth').value) || 5;
    body.service_name = document.getElementById('service-name').value || 'mock-service';
  }

  if (hasMetrics) {
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

  if (hasLogs) {
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
    statusEl.textContent = 'Run queued';
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
    updateOttlRunSelector();
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

document.body.addEventListener('htmx:afterSwap', function(evt) {
  if (evt.detail.target && evt.detail.target.id === 'ottl-result-content') {
    const panel = document.getElementById('ottl-result');
    if (panel) panel.classList.remove('hidden');
  }
});




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

function startPolling() {
  const announcedCompleted = new Set();
  pollTimer = setInterval(async () => {
    try {
      const data = await api('/api/runs');
      data.runs.forEach(r => {
        const prevStatus = runs[r.id]?.status;
        runs[r.id] = r;
        // Show brief "Completed!" when a run transitions to completed
        if (r.status === 'completed' && prevStatus && prevStatus !== 'completed' && !announcedCompleted.has(r.id)) {
          announcedCompleted.add(r.id);
          const el = document.getElementById('run-status');
          el.textContent = 'Completed!';
          setTimeout(() => { if (el.textContent === 'Completed!') el.textContent = ''; }, 3000);
        }
      });
      renderRunList();
      updateOttlRunSelector();
      // Show detail if viewing one
      const detail = document.getElementById('detail-section');
      if (!detail.classList.contains('hidden')) {
        const id = detail.querySelector('#detail-id').textContent;
        const r = runs[id];
        if (r) renderDetail(r);
      }
    } catch(e) { /* ignore polling errors */ }
  }, 1000);
}

let lastDetailKey = '';

function getRunSignalType(r) {
  if (r.output === 'import') return 'traces';
  return r.config?.signal_type || 'traces';
}

function getSignalBadge(signalType) {
  if (signalType === 'both') return '<span class="signal-badge both">TL</span>';
  if (signalType === 'logs') return '<span class="signal-badge logs">L</span>';
  if (signalType === 'metrics') return '<span class="signal-badge metrics">M</span>';
  if (signalType === 'all') return '<span class="signal-badge all">A</span>';
  if (signalType === 'traces+metrics') return '<span class="signal-badge both" style="background:rgba(34,197,94,0.2);color:#4ade80">TM</span>';
  if (signalType === 'logs+metrics') return '<span class="signal-badge both" style="background:rgba(139,92,246,0.2);color:#c084fc">LM</span>';
  return '<span class="signal-badge traces">T</span>';
}

function renderRunList() {
  const list = document.getElementById('run-list');
  const search = (document.getElementById('filter-search').value || '').toLowerCase();
  let entries = Object.values(runs).sort((a, b) => new Date(b.started_at) - new Date(a.started_at));

  const checkedSignals = [];
  document.querySelectorAll('#run-filter-signals input[type="checkbox"]:checked').forEach(c => checkedSignals.push(c.value));
  if (checkedSignals.length > 0 && checkedSignals.length < 3) {
    entries = entries.filter(r =>
      (checkedSignals.includes('traces') && !!r.trace_data) ||
      (checkedSignals.includes('logs') && !!(r.log_data || r.log_count || r.log_count_completed)) ||
      (checkedSignals.includes('metrics') && !!(r.metric_data || r.metric_count || r.metric_count_completed))
    );
  }
  if (search) {
    entries = entries.filter(r => r.id.toLowerCase().includes(search) || (r.scenario || '').toLowerCase().includes(search));
  }

  if (entries.length === 0) {
    list.innerHTML = '<div class="empty"><p>No jobs match the current filter.</p></div>';
    return;
  }
  list.innerHTML = entries.map(r => {
    const signalType = getRunSignalType(r);
    const signalBadge = getSignalBadge(signalType);
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
    return `<div class="run-card" onclick="showDetail('${r.id}')">
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

function showDetail(id) {
  const r = runs[id];
  if (!r) return;
  const section = document.getElementById('detail-section');
  section.classList.remove('hidden');
  document.getElementById('detail-id').textContent = r.id;
  lastDetailKey = '';
  renderDetail(r);
  section.scrollIntoView({ behavior: 'smooth' });
}

let detailTab = 'traces';

function renderDetail(r) {
  // Build a key from the run's mutable fields. If unchanged since last
  // render, skip DOM replacement to preserve scroll position in <pre>.
  const key = r.id + '|' + r.status + '|' + (r.span_count || 0) + '|' + (r.log_count || 0) + '|' + (r.duration_ns || 0) + '|' + (r.error || '') + '|' + (typeof r.trace_data === 'string' ? r.trace_data.substring(0, 100) : '') + '|' + detailTab;
  if (key === lastDetailKey) return;
  lastDetailKey = key;

  const el = document.getElementById('detail-content');
  const signalType = getRunSignalType(r);
  const hasLogs = r.log_data || (r.log_count && r.log_count > 0);

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
  let spanCountTotal = 0, traceCount = 0;
  if (r.trace_data) {
    try {
      const td = typeof r.trace_data === 'string' ? JSON.parse(r.trace_data) : r.trace_data;
      if (td.resource_spans) {
        traceCount = td.resource_spans.length;
        td.resource_spans.forEach(rs => {
          if (rs.scope_spans) rs.scope_spans.forEach(ss => {
            if (ss.spans) spanCountTotal += ss.spans.length;
          });
        });
      }
    } catch(e) { /* use r.span_count */ }
  }

  // Count log records
  let logRecordCount = 0, logSeverities = {};
  if (r.log_data) {
    try {
      const ld = typeof r.log_data === 'string' ? JSON.parse(r.log_data) : r.log_data;
      if (ld.resource_logs) {
        ld.resource_logs.forEach(rl => {
          if (rl.scope_logs) rl.scope_logs.forEach(sl => {
            if (sl.log_records) {
              sl.log_records.forEach(lr => {
                logRecordCount++;
                var sev = (lr.severity_text || 'INFO').toLowerCase();
                logSeverities[sev] = (logSeverities[sev] || 0) + 1;
              });
            }
          });
        });
      }
    } catch(e) {}
  }

  // Count metric data points
  let metricRecordCount = 0, metricTypes = {};
  if (r.metric_data) {
    try {
      const md = typeof r.metric_data === 'string' ? JSON.parse(r.metric_data) : r.metric_data;
      if (md.resource_metrics) {
        md.resource_metrics.forEach(rm => {
          if (rm.scope_metrics) rm.scope_metrics.forEach(sm => {
            if (sm.metrics) {
              sm.metrics.forEach(m => { metricRecordCount++; });
            }
          });
        });
      }
    } catch(e) {}
  }

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
  var hasTraceData = !!r.trace_data;
  var hasLogData = logRecordCount > 0;
  var hasMetricData = metricRecordCount > 0;
  var tabHtml = '<div class="detail-tabs">';
  if (hasTraceData) {
    tabHtml += '<button class="detail-tab' + (detailTab === 'traces' ? ' active' : '') + '" onclick="switchDetailTab(\'traces\')">Traces<span class="tab-badge">' + (traceCount || r.trace_count || 0) + '</span></button>';
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

function renderLogTable(logData) {
  let logs;
  try {
    logs = typeof logData === 'string' ? JSON.parse(logData) : logData;
  } catch(e) { return '<div class="error">Failed to parse log data</div>'; }

  var html = '<table class="log-table"><thead><tr><th style="width:52px">Severity</th><th style="width:140px">Timestamp</th><th>Body</th><th style="width:80px">Trace ID</th></tr></thead><tbody>';

  if (logs.resource_logs) {
    var allRecords = [];
    logs.resource_logs.forEach(function(rl) {
      if (rl.scope_logs) rl.scope_logs.forEach(function(sl) {
        if (sl.log_records) sl.log_records.forEach(function(lr) {
          allRecords.push(lr);
        });
      });
    });

    allRecords.slice(0, 200).forEach(function(lr, idx) {
      var sev = (lr.severity_text || 'info').toLowerCase();
      var sevClass = sev === 'warn' ? 'warn' : sev === 'error' ? 'error' : sev === 'fatal' ? 'fatal' : 'info';
      var ts = lr.time_unix_nano ? new Date(lr.time_unix_nano / 1e6).toLocaleTimeString() : '-';

      var bodyStr = '';
      if (lr.body) {
        bodyStr = lr.body.stringValue || JSON.stringify(lr.body);
      }

      var traceIdStr = '';
      if (lr.trace_id) {
        traceIdStr = typeof lr.trace_id === 'string' ? lr.trace_id.substring(0, 16) : '';
      }

      html += '<tr onclick="toggleLogDetail(this, ' + idx + ')" class="log-row" data-idx="' + idx + '">';
      html += '<td><span class="severity-tag ' + sevClass + '">' + (lr.severity_text || 'INFO') + '</span></td>';
      html += '<td style="white-space:nowrap;color:#64748b;font-family:monospace;font-size:11px">' + ts + '</td>';
      html += '<td><div class="log-body">' + escapeHtml(bodyStr) + '</div></td>';
      html += '<td style="font-family:monospace;font-size:10px;color:#475569">' + escapeHtml(traceIdStr) + '...</td>';
      html += '</tr>';

      // Detail row (hidden)
      html += '<tr class="log-detail" id="log-detail-' + idx + '"><td colspan="4">';
      html += '<pre>' + escapeHtml(JSON.stringify(lr, null, 2)) + '</pre>';
      if (lr.attributes && lr.attributes.length) {
        html += '<div class="attr-list">';
        lr.attributes.forEach(function(attr) {
          var av = '';
          if (attr.value) av = attr.value.stringValue || attr.value.intValue || attr.value.doubleValue || JSON.stringify(attr.value);
          html += '<span class="attr-item"><strong>' + escapeHtml(attr.key) + ':</strong> ' + escapeHtml(String(av)) + '</span>';
        });
        html += '</div>';
      }
      html += '</td></tr>';
    });
  }

  html += '</tbody></table>';
  return html;
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

  var html = '<table class="log-table"><thead><tr><th style="width:80px">Type</th><th>Name</th><th>Value(s)</th><th style="width:140px">Timestamp</th></tr></thead><tbody>';

  if (metrics.resource_metrics) {
    var allMetrics = [];
    metrics.resource_metrics.forEach(function(rm) {
      if (rm.scope_metrics) rm.scope_metrics.forEach(function(sm) {
        if (sm.metrics) sm.metrics.forEach(function(m) {
          allMetrics.push(m);
        });
      });
    });

    allMetrics.slice(0, 200).forEach(function(m) {
      var type = 'gauge';
      var dataPoints = [];
      if (m.gauge) { type = 'gauge'; dataPoints = m.gauge.data_points || []; }
      else if (m.sum) { type = 'sum'; dataPoints = m.sum.data_points || []; }
      else if (m.histogram) { type = 'histogram'; dataPoints = m.histogram.data_points || []; }

      var typeClass = type === 'sum' ? 'warn' : type === 'histogram' ? 'fatal' : 'info';

      html += '<tr class="log-row">';
      html += '<td><span class="severity-tag ' + typeClass + '">' + type.toUpperCase() + '</span></td>';
      html += '<td><strong>' + escapeHtml(m.name || '') + '</strong>';
      if (m.description) {
        html += '<br><span style="font-size:11px;color:#64748b">' + escapeHtml(m.description) + '</span>';
      }
      html += '</td>';
      html += '<td style="font-family:monospace;font-size:11px">';

      if (type === 'histogram') {
        var hCount = 0, hSum = 0;
        dataPoints.forEach(function(dp) {
          hCount += dp.count || 0;
          hSum += dp.sum || 0;
        });
        html += 'count: ' + hCount + ', sum: ' + hSum;
        if (dataPoints.length > 0 && dataPoints[0].bucket_counts) {
          html += '<br>buckets: [' + dataPoints[0].bucket_counts.join(', ') + ']';
        }
      } else {
        dataPoints.slice(0, 3).forEach(function(dp, idx) {
          if (idx > 0) html += '<br>';
          var val = dp.as_int !== undefined ? dp.as_int : (dp.as_double !== undefined ? dp.as_double : (dp.asInt !== undefined ? dp.asInt : (dp.asDouble !== undefined ? dp.asDouble : '?')));
          html += val;
        });
        if (dataPoints.length > 3) {
          html += '<br><span style="color:#64748b">...and ' + (dataPoints.length - 3) + ' more</span>';
        }
      }

      html += '</td>';
      var ts = dataPoints.length > 0 && dataPoints[0].time_unix_nano
        ? new Date(Number(dataPoints[0].time_unix_nano) / 1e6).toLocaleTimeString()
        : '-';
      html += '<td style="white-space:nowrap;color:#64748b;font-family:monospace;font-size:11px">' + ts + '</td>';
      html += '</tr>';
    });
  }

  html += '</tbody></table>';
  return html;
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

loadScenarios();
// Restore per-signal collapse states
['trace', 'log', 'metric'].forEach(function(signal) {
  try {
    if (localStorage.getItem('otel-workbench-config-' + signal + '-collapsed') === '1') {
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

function updateOttlRunSelector() {
  const select = document.getElementById('ottl-run-select');
  if (!select) return;
  const currentVal = select.value;
  while (select.options.length > 1) select.remove(1);
  const entries = Object.values(runs).filter(r =>
    r.status === 'completed' && (r.span_count > 0 || r.log_count > 0 || r.metric_count > 0)
  );
  entries.sort((a, b) => new Date(b.started_at) - new Date(a.started_at));
  entries.forEach(r => {
    const opt = document.createElement('option');
    opt.value = r.id;
    const hasTrace = !!r.trace_data;
    const hasLog = !!(r.log_data || r.log_count || r.log_count_completed);
    const hasMetric = !!(r.metric_data || r.metric_count || r.metric_count_completed);
    const sig = hasTrace && hasLog ? 'T+L' : hasTrace && hasMetric ? 'T+M' : hasTrace ? 'T' : hasLog ? 'L' : 'M';
    const label = r.output === 'import' ? r.id + ' (imported)' : r.id + (r.scenario ? ' [' + r.scenario + ']' : '');
    opt.textContent = label + ' \u2014 ' + sig + ' ' + (r.span_count || 0) + ' spans' +
      (hasLog ? ', ' + (r.log_count || r.log_count_completed || 0) + ' logs' : '') +
      (hasMetric ? ', ' + (r.metric_count || r.metric_count_completed || 0) + ' metrics' : '');
    select.appendChild(opt);
  });
  if (currentVal) {
    for (let i = 0; i < select.options.length; i++) {
      if (select.options[i].value === currentVal) {
        select.selectedIndex = i;
        break;
      }
    }
  }
}

function updateOttlLineNumbers() {
  const textarea = document.getElementById('ottl-input');
  const numbers = document.getElementById('ottl-line-numbers');
  if (!textarea || !numbers) return;
  const lines = textarea.value.split('\n').length;
  numbers.innerHTML = Array.from({length: lines}, (_, i) => '<span>' + (i + 1) + '</span>').join('');
}

function insertSnippet(content) {
  const textarea = document.getElementById('ottl-input');
  if (!textarea) return;
  textarea.value = content;
  textarea.dispatchEvent(new Event('input'));
}

// Load existing runs from the server so the run list and OTTL selector
// are populated immediately on page load (not just after the first new run).
(async function loadExistingRuns() {
  try {
    const data = await api('/api/runs');
    if (data.runs) {
      data.runs.forEach(r => { runs[r.id] = r; });
      renderRunList();
      updateOttlRunSelector();
    }
  } catch(e) { /* page may load before any runs exist */ }
  if (!pollTimer) startPolling();
})();

document.getElementById('ottl-input').addEventListener('input', updateOttlLineNumbers);
updateOttlLineNumbers();

updateAllPillState('signal-type');
updateAllPillState('ottl-signal-checkboxes');
updateAllPillState('run-filter-signals');

// --- OTTL Prototyper: Diff & Tab Switching ---

function canComputeDiff(data) {
  if (!data) return false;
  if (data._composite) return !!(data.traces || data.logs || data.metrics);
  if (!data.original) return false;
  const d = typeof data.original === 'string' ? JSON.parse(data.original) : data.original;
  return d && (d.resource_spans || d.resource_logs);
}

function switchOttlResultTab(tab) {
  const content = document.getElementById('ottl-result-content');
  if (!content) return;

  const data = window.__ottlTransformData;
  const tabSections = content.querySelectorAll('[data-tab-content]');

  const computeSummary = d => {
    const diff = computeDiff(d.original, d.transformed);
    return renderDiffSummary(diff) + renderDiffTable(diff);
  };

  if ((tab === 'summary' || tab === 'fulldiff') && data && canComputeDiff(data)) {
    const isComposite = data._composite;
    const hasData = isComposite
      ? !!(data.traces?.original && data.traces?.transformed) ||
        !!(data.logs?.original && data.logs?.transformed) ||
        !!(data.metrics?.original && data.metrics?.transformed)
      : !!(data.original && data.transformed);

    if (hasData) {
      tabSections.forEach(el => el.style.display = 'none');
      const oldDiff = content.querySelector('[data-diff-content]');
      if (oldDiff) oldDiff.remove();
      const div = document.createElement('div');
      div.setAttribute('data-diff-content', '');

      if (tab === 'summary') {
        if (isComposite) {
          let html = '';
          for (const st of ['traces', 'logs', 'metrics']) {
            const sd = data[st];
            if (sd && sd.original && sd.transformed) {
              const label = st.charAt(0).toUpperCase() + st.slice(1);
              html += '<div style="margin-bottom:16px">';
              html += '<h3 style="margin:0 0 4px;font-size:14px;color:#e2e8f0">' + label + '</h3>';
              html += computeSummary(sd);
              html += '</div>';
            }
          }
          div.innerHTML = html;
        } else {
          div.innerHTML = computeSummary(data);
        }
      } else {
        // fulldiff tab
        if (isComposite) {
          let html = '';
          for (const st of ['traces', 'logs', 'metrics']) {
            const sd = data[st];
            if (sd && sd.original && sd.transformed) {
              const label = st.charAt(0).toUpperCase() + st.slice(1);
              html += '<h3 style="margin:12px 0 4px;font-size:14px;color:#e2e8f0">' + label + '</h3>';
              html += renderInlineDiff(sd.original, sd.transformed);
            }
          }
          div.innerHTML = html;
        } else {
          div.innerHTML = renderInlineDiff(data.original, data.transformed);
        }
      }
      content.appendChild(div);
      return;
    }
  }

  // Fall through to server-rendered tab sections
  tabSections.forEach(el => {
    el.style.display = el.getAttribute('data-tab-content') === tab ? '' : 'none';
  });
  const oldDiff = content.querySelector('[data-diff-content]');
  if (oldDiff) oldDiff.remove();
}

function computeDiff(originalData, transformedData) {
  const parse = d => (typeof d === 'string' ? JSON.parse(d) : d);
  let orig, trans;
  try { orig = parse(originalData); } catch(e) { orig = null; }
  try { trans = parse(transformedData); } catch(e) { trans = null; }
  const result = { added: 0, removed: 0, modified: 0, totalSpans: 0, spanDiffs: [], spansRemoved: 0, spansAdded: 0, signalType: null };
  if (!orig || !trans) return result;

  // Detect signal type
  if (orig.resource_spans) result.signalType = 'traces';
  else if (orig.resource_logs) result.signalType = 'logs';
  else if (orig.resource_metrics) result.signalType = 'metrics';
  else return result;

  // Extract records based on signal type
  let origRecords, transRecords;
  const getTraceRecords = td => {
    const list = [];
    if (td.resource_spans) td.resource_spans.forEach((rs, ri) => {
      if (rs.scope_spans) rs.scope_spans.forEach((ss, si) => {
        if (ss.spans) ss.spans.forEach((sp, pi) => {
          const attrs = {};
          if (sp.attributes) sp.attributes.forEach(a => { attrs[a.key] = a.value; });
          list.push({ resourceIdx: ri, scopeIdx: si, recordIdx: pi, name: sp.name || 'span-' + pi, attrs });
        });
      });
    });
    return list;
  };
  const getLogRecords = td => {
    const list = [];
    if (td.resource_logs) td.resource_logs.forEach((rs, ri) => {
      if (rs.scope_logs) rs.scope_logs.forEach((ss, si) => {
        if (ss.log_records) ss.log_records.forEach((lr, li) => {
          const attrs = {};
          if (lr.attributes) lr.attributes.forEach(a => { attrs[a.key] = a.value; });
          list.push({
            resourceIdx: ri, scopeIdx: si, recordIdx: li,
            name: lr.severity_text ? lr.severity_text + ': ' + formatAttrValue(lr.body) : formatAttrValue(lr.body),
            attrs, body: formatAttrValue(lr.body)
          });
        });
      });
    });
    return list;
  };

  if (result.signalType === 'traces') {
    origRecords = getTraceRecords(orig);
    transRecords = getTraceRecords(trans);
  } else if (result.signalType === 'logs') {
    origRecords = getLogRecords(orig);
    transRecords = getLogRecords(trans);
  } else {
    return result;
  }

  result.totalSpans = Math.max(origRecords.length, transRecords.length);

  // When record count differs (e.g. filter removed some), report
  // record-level additions/removals and skip attribute comparison.
  if (origRecords.length !== transRecords.length) {
    if (origRecords.length > transRecords.length) {
      result.spansRemoved = origRecords.length - transRecords.length;
      for (let i = transRecords.length; i < origRecords.length; i++) {
        const s = origRecords[i];
        result.spanDiffs.push({
          name: s.name, idx: i,
          added: [], removed: [], modified: [], _removed: true
        });
      }
      result.removed = result.spansRemoved;
    }
    if (transRecords.length > origRecords.length) {
      result.spansAdded = transRecords.length - origRecords.length;
      for (let i = origRecords.length; i < transRecords.length; i++) {
        const s = transRecords[i];
        result.spanDiffs.push({
          name: s.name, idx: i,
          added: [], removed: [], modified: [], _added: true
        });
      }
      result.added = result.spansAdded;
    }
    return result;
  }

  // Attribute-level diff by index (transform mode: same number of records)
  for (let i = 0; i < origRecords.length; i++) {
    const os = origRecords[i];
    const ts = transRecords[i];
    const recDiff = { name: os.name, idx: i, added: [], removed: [], modified: [] };

    // Compare body for log records
    if (os.body !== undefined && ts.body !== undefined && os.body !== ts.body) {
      recDiff.bodyOld = os.body;
      recDiff.bodyNew = ts.body;
      result.modified++;
    }

    const origKeys = Object.keys(os.attrs);
    const transKeys = Object.keys(ts.attrs);
    const allKeys = [...new Set([...origKeys, ...transKeys])];
    for (const key of allKeys) {
      const ov = key in os.attrs ? os.attrs[key] : undefined;
      const tv = key in ts.attrs ? ts.attrs[key] : undefined;
      if (ov === undefined && tv !== undefined) {
        recDiff.added.push({ key, value: tv });
        result.added++;
      } else if (ov !== undefined && tv === undefined) {
        recDiff.removed.push({ key, value: ov });
        result.removed++;
      } else if (JSON.stringify(ov) !== JSON.stringify(tv)) {
        recDiff.modified.push({ key, oldValue: ov, newValue: tv });
        result.modified++;
      }
    }
    if (recDiff.added.length > 0 || recDiff.removed.length > 0 || recDiff.modified.length > 0 || recDiff.bodyOld !== undefined) {
      result.spanDiffs.push(recDiff);
    }
  }
  return result;
}

function renderDiffSummary(diff) {
  const recordLabel = diff.signalType === 'logs' ? 'Log Records' : diff.signalType === 'traces' ? 'Spans' : 'Records';
  let html = '<div class="ottl-diff-summary">';
  html += '<div class="ottl-diff-stat ottl-diff-added"><div class="count">' + diff.added + '</div><div class="label">Added</div></div>';
  html += '<div class="ottl-diff-stat ottl-diff-removed"><div class="count">' + diff.removed + '</div><div class="label">Removed</div></div>';
  html += '<div class="ottl-diff-stat ottl-diff-modified"><div class="count">' + diff.modified + '</div><div class="label">Modified</div></div>';
  html += '<div class="ottl-diff-stat" style="min-width:80px"><div class="count">' + diff.totalSpans + '</div><div class="label">' + recordLabel + '</div></div>';
  html += '</div>';
  return html;
}

function renderDiffTable(diff) {
  if (diff.spanDiffs.length === 0) {
    return '<div style="padding:12px;color:#94a3b8;font-size:13px">No differences found between original and transformed data.</div>';
  }
  const isLogs = diff.signalType === 'logs';
  const recordLabel = isLogs ? 'log' : 'span';
  const header1 = isLogs ? 'Body / Attribute' : 'Attribute';
  let html = '<table class="ottl-diff-table"><tr><th style="width:30%">' + header1 + '</th><th style="width:35%">Original</th><th style="width:35%">Transformed</th></tr>';
  for (const sd of diff.spanDiffs) {
    if (sd._removed) {
      html += '<tr class="span-header span-removed"><td colspan="3">' + escapeHtml(sd.name) + ' (' + recordLabel + ' ' + sd.idx + ') — removed</td></tr>';
      continue;
    }
    if (sd._added) {
      html += '<tr class="span-header span-added"><td colspan="3">' + escapeHtml(sd.name) + ' (' + recordLabel + ' ' + sd.idx + ') — added</td></tr>';
      continue;
    }
    html += '<tr class="span-header"><td colspan="3">' + escapeHtml(sd.name) + ' (' + recordLabel + ' ' + sd.idx + ')</td></tr>';
    // Show body changes for log records
    if (sd.bodyOld !== undefined || sd.bodyNew !== undefined) {
      if (sd.bodyOld !== undefined && sd.bodyNew !== undefined) {
        html += '<tr class="modified"><td>body</td><td class="val-old">' + escapeHtml(sd.bodyOld) + '</td><td class="val-new">' + escapeHtml(sd.bodyNew) + '</td></tr>';
      } else if (sd.bodyOld !== undefined) {
        html += '<tr class="removed"><td>body</td><td class="val-old">' + escapeHtml(sd.bodyOld) + '</td><td></td></tr>';
      } else {
        html += '<tr class="added"><td>body</td><td></td><td class="val-new">' + escapeHtml(sd.bodyNew) + '</td></tr>';
      }
    }
    for (const item of sd.removed) {
      const v = formatAttrValue(item.value);
      html += '<tr class="removed"><td>' + escapeHtml(item.key) + '</td><td class="val-old">' + escapeHtml(v) + '</td><td></td></tr>';
    }
    for (const item of sd.added) {
      const v = formatAttrValue(item.value);
      html += '<tr class="added"><td>' + escapeHtml(item.key) + '</td><td></td><td class="val-new">' + escapeHtml(v) + '</td></tr>';
    }
    for (const item of sd.modified) {
      const ov = formatAttrValue(item.oldValue);
      const nv = formatAttrValue(item.newValue);
      html += '<tr class="modified"><td>' + escapeHtml(item.key) + '</td><td class="val-old">' + escapeHtml(ov) + '</td><td class="val-new">' + escapeHtml(nv) + '</td></tr>';
    }
  }
  html += '</table>';
  return html;
}

function formatAttrValue(v) {
  if (v === null || v === undefined) return '';
  if (typeof v === 'object') {
    for (const key of ['stringValue', 'intValue', 'doubleValue', 'boolValue', 'arrayValue', 'kvlistValue']) {
      if (key in v) {
        const inner = v[key];
        return typeof inner === 'object' ? JSON.stringify(inner) : String(inner);
      }
    }
    return JSON.stringify(v);
  }
  return String(v);
}

// --- Full inline diff (unified diff of full JSON) ---

function computeInlineDiff(originalData, transformedData) {
  const parse = d => (typeof d === 'string' ? JSON.parse(d) : d);
  let orig, trans;
  try { orig = parse(originalData); } catch(e) { orig = null; }
  try { trans = parse(transformedData); } catch(e) { trans = null; }
  if (!orig || !trans) return [];

  const origLines = JSON.stringify(orig, null, 2).split('\n');
  const transLines = JSON.stringify(trans, null, 2).split('\n');

  // Simple unified diff: walk both line arrays, matching lines forward.
  const hunks = [];
  let oi = 0, ti = 0;
  while (oi < origLines.length && ti < transLines.length) {
    if (origLines[oi] === transLines[ti]) {
      hunks.push({ type: 'same', text: origLines[oi] });
      oi++; ti++;
      continue;
    }
    let found = false;
    const lookAhead = Math.min(10, Math.max(origLines.length - oi, transLines.length - ti));
    for (let ahead = 1; ahead <= lookAhead; ahead++) {
      if (oi + ahead < origLines.length && origLines[oi + ahead] === transLines[ti]) {
        for (let r = 0; r < ahead; r++) hunks.push({ type: 'removed', text: origLines[oi + r] });
        oi += ahead;
        found = true;
        break;
      }
      if (ti + ahead < transLines.length && origLines[oi] === transLines[ti + ahead]) {
        for (let a = 0; a < ahead; a++) hunks.push({ type: 'added', text: transLines[ti + a] });
        ti += ahead;
        found = true;
        break;
      }
    }
    if (!found) {
      if (origLines[oi] !== undefined) hunks.push({ type: 'removed', text: origLines[oi] });
      if (transLines[ti] !== undefined) hunks.push({ type: 'added', text: transLines[ti] });
      oi++; ti++;
    }
  }
  while (oi < origLines.length) hunks.push({ type: 'removed', text: origLines[oi++] });
  while (ti < transLines.length) hunks.push({ type: 'added', text: transLines[ti++] });
  return hunks;
}

function renderInlineDiff(originalData, transformedData) {
  const hunks = computeInlineDiff(originalData, transformedData);
  if (hunks.length === 0) {
    return '<div style="padding:12px;color:#94a3b8;font-size:13px">No differences found.</div>';
  }
  let html = '<div class="ottl-inline-diff">';
  let origLine = 0, transLine = 0;
  for (const hunk of hunks) {
    const type = hunk.type;
    const cls = type === 'added' ? ' added' : type === 'removed' ? ' removed' : '';
    let marker = ' ';
    let lineNum = '';
    if (type === 'added') {
      marker = '+';
      transLine++;
      lineNum = '<span class="ln"></span><span class="ln" style="color:#22c55e">' + transLine + '</span>';
    } else if (type === 'removed') {
      marker = '-';
      origLine++;
      lineNum = '<span class="ln" style="color:#ef4444">' + origLine + '</span><span class="ln"></span>';
    } else {
      origLine++;
      transLine++;
      lineNum = '<span class="ln">' + origLine + '</span><span class="ln">' + transLine + '</span>';
    }
    html += '<div class="diff-line' + cls + '"><span class="marker">' + marker + '</span>' + lineNum + '<span class="text">' + escapeHtml(hunk.text) + '</span></div>';
  }
  html += '</div>';
  return html;
}
