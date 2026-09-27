function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

// --- Table rendering builder (consolidated) ---

function buildTable(data, cols) {
  if (!data || !Array.isArray(data) || data.length === 0) {
    return '<table class="log-table"><thead><tr>' + cols.map(c => '<th style="width:' + (c.width || '150px') + '">' + c.header + '</th>').join('') + '</tr></thead><tbody><tr class="empty-row"><td colspan="' + cols.length + '">No data</td></tr></tbody></table>';
  }
  
  var html = '<table class="log-table"><thead><tr>';
  for (var c of cols) {
    html += '<th style="width:' + (c.width || '150px') + '">' + c.header + '</th>';
  }
  html += '</tr></thead><tbody>';
  
  var rows = data.slice(0, 200);
  for (var i = 0; i < rows.length; i++) {
    html += '<tr class="log-row">';
    for (var c of cols) {
      var val = c.getValue ? c.getValue(rows[i]) : rows[i][c.key];
      html += '<td>' + (c.formatValue ? c.formatValue(val) : escapeHtml(String(val))) + '</td>';
    }
    html += '</tr>';
  }
  
  html += '</tbody></table>';
  return html;
}

function renderLogTable(logData) {
  var logs = typeof logData === 'string' ? JSON.parse(logData) : logData;
  var allRecords = [];
  if (logs.resourceLogs) {
    logs.resourceLogs.forEach(rl => {
      if (rl.scopeLogs) rl.scopeLogs.forEach(sl => {
        if (sl.logRecords) sl.logRecords.forEach(lr => {
          allRecords.push(lr);
        });
      });
    });
  }
  
  var cols = [
    {
      header: 'Severity',
      width: '120px',
      getValue: function(lr) { return lr.severityText || lr.severityNumber || ''; },
      formatValue: function(v) {
        if (!v) return '';
        var s = String(v).toLowerCase();
        var colors = {
          'trace': '#94a3b8', 'debug': '#64748b',
          'info': '#3b82f6', 'warn': '#eab308',
          'error': '#ef4444', 'fatal': '#dc2626'
        };
        var color = colors[s] || '#94a3b8';
        return '<span style="color:' + color + ';font-weight:600">' + escapeHtml(v) + '</span>';
      }
    },
    {
      header: 'Timestamp',
      width: '160px',
      getValue: function(lr) { return lr.timestamp || ''; },
      formatValue: function(v) { return escapeHtml(v); }
    },
    {
      header: 'Body',
      width: '400px',
      getValue: function(lr) { return lr.body || ''; },
      formatValue: function(v) { return escapeHtml(String(v)); }
    },
    {
      header: 'Trace ID',
      width: '180px',
      getValue: function(lr) { return lr.traceId || ''; },
      formatValue: function(v) { return '<code style="font-size:12px">' + escapeHtml(v) + '</code>'; }
    }
  ];
  
  var html = buildTable(allRecords, cols);
  if (allRecords.length > 0) {
    html += '<script>function toggleLogDetail(row, idx) {' +
      'var tbody = row.parentNode;' +
      'var detailRow = tbody.childNodes[idx * 2 + 1];' +
      'if (detailRow && detailRow.classList.contains("log-detail-row")) {' +
      'detailRow.style.display = detailRow.style.display === "none" ? "" : "none";' +
      '}' +
      '}</script>';
  }
  return html;
}

function renderMetricTable(metricData) {
  var metrics = typeof metricData === 'string' ? JSON.parse(metricData) : metricData;
  var allMetrics = [];
  if (metrics.resourceMetrics) {
    metrics.resourceMetrics.forEach(rm => {
      if (rm.scopeMetrics) rm.scopeMetrics.forEach(sm => {
        if (sm.metrics) sm.metrics.forEach(m => {
          allMetrics.push(m);
        });
      });
    });
  }
  
  var cols = [
    {
      header: 'Type',
      width: '100px',
      getValue: function(m) { return m.type || ''; },
      formatValue: function(v) { return '<span style="font-weight:600">' + escapeHtml(v) + '</span>'; }
    },
    {
      header: 'Name',
      width: '200px',
      getValue: function(m) { return m.name || ''; },
      formatValue: function(v) { return '<code style="font-size:12px">' + escapeHtml(v) + '</code>'; }
    },
    {
      header: 'Value(s)',
      width: '300px',
      getValue: function(m) {
        var type = m.type || '';
        if (m.dataPoints && m.dataPoints.length > 0) {
          if (type === 'histogram') {
            var dp = m.dataPoints[0];
            var parts = [];
            if (dp.count !== undefined) parts.push('count: ' + dp.count);
            if (dp.sum !== undefined) parts.push('sum: ' + dp.sum);
            if (dp.buckets) parts.push('buckets: [' + dp.buckets.length + ']');
            return parts.join(', ');
          }
          var vals = [];
          for (var dp of m.dataPoints) {
            if (dp.value !== undefined) vals.push(dp.value);
            else if (dp.asDouble !== undefined) vals.push(dp.asDouble);
            else if (dp.asInt !== undefined) vals.push(dp.asInt);
            else if (dp.asBool !== undefined) vals.push(dp.asBool);
            else if (dp.asStr !== undefined) vals.push(dp.asStr);
          }
          if (vals.length === 0) return '-';
          return vals.slice(0, 3).join(', ') + (vals.length > 3 ? '...' : '');
        }
        return '-';
      },
      formatValue: function(v) { return '<code style="font-size:12px">' + escapeHtml(String(v)) + '</code>'; }
    },
    {
      header: 'Timestamp',
      width: '160px',
      getValue: function(m) { return m.dataPoints && m.dataPoints[0] ? (m.dataPoints[0].time || m.dataPoints[0].timestamp || '') : ''; },
      formatValue: function(v) { return escapeHtml(v); }
    }
  ];
  
  return buildTable(allMetrics, cols);
}

// --- Remotetap (Live Collector Data) ---
