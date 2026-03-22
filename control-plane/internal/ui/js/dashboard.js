// --- Dashboard ---

let dashboardRange = '1h';

function setDashboardRange(range, btn) {
  dashboardRange = range;
  document.querySelectorAll('#dashboard-time-range button').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  loadDashboardCharts();
}

async function loadDashboard() {
  loadDashboardKPIs();
  loadDashboardCharts();
  loadDashboardEvents();
}

async function loadDashboardKPIs() {
  try {
    const [healthResult, agentsResult, registrationsResult] = await Promise.allSettled([
      fetch('/health').then(r => r.json()),
      api('GET', '/agents'),
      api('GET', '/agent-registrations'),
    ]);
    const health = healthResult.status === 'fulfilled' ? healthResult.value : {};
    const agents = agentsResult.status === 'fulfilled' ? agentsResult.value : [];
    const registrations = registrationsResult.status === 'fulfilled' ? registrationsResult.value : null;

    if (registrations && registrations.length) {
      registrationsData = registrations;
      updateRegistrationFilters(registrations);
    }

    const stale = agents ? agents.filter(a => a.status === 'stale').length : 0;
    const killed = agents ? agents.filter(a => a.status === 'kill').length : 0;
    const totalReqs = agents ? agents.reduce((s, a) => s + (a.stats.allowed || 0) + (a.stats.blocked || 0), 0) : 0;
    const totalBlocked = agents ? agents.reduce((s, a) => s + (a.stats.blocked || 0), 0) : 0;
    const blockRate = totalReqs > 0 ? Math.round((totalBlocked / totalReqs) * 100) : 0;

    document.getElementById('dashboard-kpis').innerHTML = `
      <div class="kpi-row">
        <div class="kpi-card">
          <div class="kpi-label">Registrations</div>
          <div class="kpi-value blue">${registrations ? registrations.length : 0}</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Active Agents</div>
          <div class="kpi-value green">${health.healthy || 0}</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Stale</div>
          <div class="kpi-value ${stale > 0 ? 'yellow' : ''}">${stale}</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Killed</div>
          <div class="kpi-value ${killed > 0 ? 'red' : ''}">${killed}</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Total Requests</div>
          <div class="kpi-value">${totalReqs.toLocaleString()}</div>
        </div>
        <div class="kpi-card">
          <div class="kpi-label">Block Rate</div>
          <div class="kpi-value ${blockRate > 10 ? 'red' : ''}">${blockRate}%</div>
        </div>
      </div>
    `;
  } catch(e) {
    if (e.message !== 'unauthorized' && e.message !== 'forbidden')
      document.getElementById('dashboard-kpis').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

async function loadDashboardCharts() {
  const templateName = document.getElementById('dashboard-template-filter').value;
  const bucketMap = { '1h': '1m', '6h': '5m', '24h': '30m', '168h': '3h' };
  const bucket = bucketMap[dashboardRange] || '5m';

  let path = '/metrics/series?since=' + dashboardRange + '&bucket=' + bucket;
  if (templateName) path += '&type=' + encodeURIComponent(templateName);

  try {
    const data = await api('GET', path);
    const container = document.getElementById('dashboard-charts');

    if (!data || !data.length) {
      container.innerHTML = '<div class="empty">No metrics data for this time range</div>';
      return;
    }

    container.innerHTML = `
      <div class="chart-row">
        <div class="chart-container">
          <h3>Traffic</h3>
          <div id="chart-traffic"></div>
          <div class="chart-legend">
            <span class="leg-green">Allowed</span>
            <span class="leg-red">Blocked</span>
          </div>
        </div>
        <div class="chart-container">
          <h3>Latency (avg ms)</h3>
          <div id="chart-latency"></div>
          <div class="chart-legend">
            <span class="leg-blue">Avg latency</span>
          </div>
        </div>
      </div>
    `;

    renderAreaChart('chart-traffic', data, [
      { key: 'allowed', color: '#2ec4b6', label: 'Allowed' },
      { key: 'blocked', color: '#e63946', label: 'Blocked' },
    ]);

    renderLineChart('chart-latency', data, { key: 'avgMs', color: '#457b9d' });
  } catch(e) {
    if (e.message !== 'unauthorized' && e.message !== 'forbidden')
      document.getElementById('dashboard-charts').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

async function loadDashboardEvents() {
  try {
    const events = await api('GET', '/audit?since=24h');
    if (events && events.length) {
      // Merge API events with live SSE events, dedup by timestamp
      const seen = new Set(dashboardEvents.map(e => e.type + e.ts.toISOString()));
      for (const e of events) {
        const key = e.type + new Date(e.ts).toISOString();
        if (!seen.has(key)) {
          dashboardEvents.push({ type: e.type, data: e.data || {}, ts: new Date(e.ts) });
        }
      }
      // Sort newest first, cap at 50
      dashboardEvents.sort((a, b) => b.ts - a.ts);
      if (dashboardEvents.length > 50) dashboardEvents.length = 50;
    }
  } catch(e) {}
  renderDashboardEvents();
}

function renderDashboardEvents() {
  const container = document.getElementById('dashboard-events');
  if (!container) return;

  if (!dashboardEvents.length) {
    container.innerHTML = `
      <div class="event-feed">
        <div class="event-feed-header">Live Events</div>
        <div style="padding:20px;text-align:center;color:var(--muted);font-size:13px">No events yet. Activity will appear here in real-time.</div>
      </div>
    `;
    return;
  }

  const eventMeta = {
    'agent:registered': { dot: 'var(--green)', label: 'Agent registered' },
    'agent:stale':      { dot: 'var(--yellow)', label: 'Agent stale' },
    'agent:recovered':  { dot: 'var(--green)', label: 'Agent recovered' },
    'agent:killed':     { dot: 'var(--red)', label: 'Agent killed' },
    'log:batch':        { dot: 'var(--muted)', label: 'Logs ingested' },
    'registration:created': { dot: 'var(--blue)', label: 'Registration created' },
    'registration:updated': { dot: 'var(--blue)', label: 'Registration updated' },
  };

  const rows = dashboardEvents.slice(0, 20).map(evt => {
    const meta = eventMeta[evt.type] || { dot: 'var(--muted)', label: evt.type };
    const detail = evt.data.id || evt.data.name || (evt.data.count ? evt.data.count + ' entries' : '');
    const time = evt.ts.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    return `
      <div class="event-item">
        <span class="event-dot" style="background:${meta.dot}"></span>
        <span class="event-time">${time}</span>
        <span class="event-text">${esc(meta.label)}${detail ? ' &mdash; <span style="color:var(--muted)">' + esc(detail) + '</span>' : ''}</span>
      </div>
    `;
  }).join('');

  container.innerHTML = `
    <div class="event-feed">
      <div class="event-feed-header">Live Events</div>
      ${rows}
    </div>
  `;
}

// --- SVG Charts ---

function renderAreaChart(containerId, data, series) {
  const W = 500, H = 180, pad = { top: 10, right: 10, bottom: 30, left: 45 };
  const cw = W - pad.left - pad.right;
  const ch = H - pad.top - pad.bottom;

  let maxVal = 0;
  for (const d of data) {
    for (const s of series) {
      if (d[s.key] > maxVal) maxVal = d[s.key];
    }
  }
  if (maxVal === 0) maxVal = 1;

  const xScale = (i) => pad.left + (i / (data.length - 1 || 1)) * cw;
  const yScale = (v) => pad.top + ch - (v / maxVal) * ch;

  let svg = `<svg viewBox="0 0 ${W} ${H}" class="chart-svg" preserveAspectRatio="xMidYMid meet">`;

  // Grid lines
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (ch / 4) * i;
    const val = Math.round(maxVal * (1 - i / 4));
    svg += `<line x1="${pad.left}" y1="${y}" x2="${W - pad.right}" y2="${y}" stroke="#1e2235" stroke-width="1"/>`;
    svg += `<text x="${pad.left - 6}" y="${y + 4}" fill="#6b7194" font-size="10" text-anchor="end">${val}</text>`;
  }

  // X-axis labels
  const labelCount = Math.min(data.length, 6);
  for (let i = 0; i < labelCount; i++) {
    const idx = Math.round(i * (data.length - 1) / (labelCount - 1 || 1));
    const x = xScale(idx);
    const d = new Date(data[idx].ts);
    const label = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
    svg += `<text x="${x}" y="${H - 4}" fill="#6b7194" font-size="10" text-anchor="middle">${label}</text>`;
  }

  // Areas + lines
  for (const s of series) {
    const points = data.map((d, i) => `${xScale(i)},${yScale(d[s.key])}`);
    const areaPath = `M${xScale(0)},${yScale(0)} L${points.join(' L')} L${xScale(data.length - 1)},${yScale(0)} Z`;
    svg += `<path d="${areaPath}" fill="${s.color}" fill-opacity="0.15"/>`;
    svg += `<polyline points="${points.join(' ')}" fill="none" stroke="${s.color}" stroke-width="2"/>`;
  }

  // Tooltip hover zones
  for (let i = 0; i < data.length; i++) {
    const x = xScale(i);
    const d = data[i];
    const labels = series.map(s => `${s.label}: ${d[s.key]}`).join(', ');
    const time = new Date(d.ts).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
    for (const s of series) {
      svg += `<circle cx="${x}" cy="${yScale(d[s.key])}" r="3" fill="${s.color}" opacity="0"><title>${time} - ${labels}</title></circle>`;
    }
    svg += `<rect x="${x - cw / data.length / 2}" y="${pad.top}" width="${cw / data.length}" height="${ch}" fill="transparent">
      <title>${time} - ${labels}</title>
    </rect>`;
  }

  svg += '</svg>';
  document.getElementById(containerId).innerHTML = svg;
}

function renderLineChart(containerId, data, opts) {
  const W = 500, H = 180, pad = { top: 10, right: 10, bottom: 30, left: 45 };
  const cw = W - pad.left - pad.right;
  const ch = H - pad.top - pad.bottom;

  let maxVal = 0;
  for (const d of data) {
    if (d[opts.key] > maxVal) maxVal = d[opts.key];
  }
  if (maxVal === 0) maxVal = 1;

  const xScale = (i) => pad.left + (i / (data.length - 1 || 1)) * cw;
  const yScale = (v) => pad.top + ch - (v / maxVal) * ch;

  let svg = `<svg viewBox="0 0 ${W} ${H}" class="chart-svg" preserveAspectRatio="xMidYMid meet">`;

  // Grid
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (ch / 4) * i;
    const val = Math.round(maxVal * (1 - i / 4));
    svg += `<line x1="${pad.left}" y1="${y}" x2="${W - pad.right}" y2="${y}" stroke="#1e2235" stroke-width="1"/>`;
    svg += `<text x="${pad.left - 6}" y="${y + 4}" fill="#6b7194" font-size="10" text-anchor="end">${val}</text>`;
  }

  // X-axis
  const labelCount = Math.min(data.length, 6);
  for (let i = 0; i < labelCount; i++) {
    const idx = Math.round(i * (data.length - 1) / (labelCount - 1 || 1));
    const x = xScale(idx);
    const d = new Date(data[idx].ts);
    const label = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
    svg += `<text x="${x}" y="${H - 4}" fill="#6b7194" font-size="10" text-anchor="middle">${label}</text>`;
  }

  // Area fill + line
  const points = data.map((d, i) => `${xScale(i)},${yScale(d[opts.key])}`);
  const areaPath = `M${xScale(0)},${yScale(0)} L${points.join(' L')} L${xScale(data.length - 1)},${yScale(0)} Z`;
  svg += `<path d="${areaPath}" fill="${opts.color}" fill-opacity="0.1"/>`;
  svg += `<polyline points="${points.join(' ')}" fill="none" stroke="${opts.color}" stroke-width="2"/>`;

  // Hover zones
  for (let i = 0; i < data.length; i++) {
    const x = xScale(i);
    const d = data[i];
    const time = new Date(d.ts).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
    svg += `<circle cx="${x}" cy="${yScale(d[opts.key])}" r="3" fill="${opts.color}" opacity="0"><title>${time}: ${d[opts.key]}ms</title></circle>`;
    svg += `<rect x="${x - cw / data.length / 2}" y="${pad.top}" width="${cw / data.length}" height="${ch}" fill="transparent">
      <title>${time}: ${d[opts.key]}ms</title>
    </rect>`;
  }

  svg += '</svg>';
  document.getElementById(containerId).innerHTML = svg;
}
