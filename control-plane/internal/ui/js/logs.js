// --- Logs ---

let logsTimer = null;
let logsSince = '24h';
let logStatsSince = '24h';
let activeLogsSubtab = 'domains';

function showLogsSubtab(name, btn) {
  activeLogsSubtab = name;
  document.querySelectorAll('.subtab').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  document.getElementById('logs-panel-domains').style.display = name === 'domains' ? '' : 'none';
  document.getElementById('logs-panel-requests').style.display = name === 'requests' ? '' : 'none';
  if (name === 'domains') loadLogStats(document.getElementById('log-template-filter').value);
  if (name === 'requests') loadLogEntriesFromUI();
}

function loadLogEntriesFromUI() {
  const type = document.getElementById('log-template-filter').value;
  const action = document.getElementById('log-action-filter').value;
  const domain = document.getElementById('log-domain-filter').value.trim();
  loadLogEntries(type, action, domain);
}

async function loadLogs() {
  // Ensure registrations are loaded for filter dropdown
  if (!registrationsData.length) {
    try {
      const regs = await api('GET', '/agent-registrations');
      if (regs && regs.length) {
        registrationsData = regs;
        updateRegistrationFilters(regs);
      }
    } catch(e) {}
  }

  const type = document.getElementById('log-template-filter').value;
  const action = document.getElementById('log-action-filter').value;
  const domain = document.getElementById('log-domain-filter').value.trim();

  if (activeLogsSubtab === 'domains') {
    await loadLogStats(type);
  } else {
    await loadLogEntries(type, action, domain);
  }

  if (!logsTimer) {
    logsTimer = setInterval(() => {
      const active = document.querySelector('section.active');
      if (active && active.id === 'logs-tab') loadLogs();
      else { clearInterval(logsTimer); logsTimer = null; }
    }, 10000);
  }
}

async function loadLogStats(type) {
  try {
    let path = '/logs/stats?since=' + logStatsSince;
    if (type) path += '&type=' + encodeURIComponent(type);
    const stats = await api('GET', path);

    const total = stats.reduce((s, d) => s + d.total, 0);
    const allowed = stats.reduce((s, d) => s + d.allowed, 0);
    const blocked = stats.reduce((s, d) => s + d.blocked, 0);
    const maxCount = stats.length ? stats[0].total : 1;

    let html = `
      <div class="log-stats">
        <div class="log-stat"><div class="label">Domains</div><div class="val">${stats.length}</div></div>
        <div class="log-stat"><div class="label">Total Requests</div><div class="val">${total}</div></div>
        <div class="log-stat"><div class="label">Allowed</div><div class="val green">${allowed}</div></div>
        <div class="log-stat"><div class="label">Blocked</div><div class="val red">${blocked}</div></div>
      </div>
    `;

    if (!stats.length) {
      html += `<div class="empty" style="margin-bottom:16px">No domain activity in the last ${logStatsSince}</div>`;
    } else {
      html += `<div class="box" style="margin-bottom:16px">
        <div class="domain-stats-header">
          <span class="ds-domain">Domain</span>
          <span class="ds-reg">Registration</span>
          <span class="ds-total">Total</span>
          <span class="ds-counts">Allow / Block</span>
          <span class="ds-last">Last Seen</span>
        </div>`;
      html += stats.map(d => {
        const isBlocked = d.blocked > d.allowed;
        return `<div class="domain-stat-row" onclick="filterLogsByDomain('${esc(d.domain)}')">
          <span class="ds-domain ${isBlocked ? 'blocked' : 'allowed'}">${esc(d.domain)}</span>
          <span class="ds-reg">${esc(d.registration)}</span>
          <span class="ds-total">${d.total}</span>
          <span class="ds-counts"><span class="green">${d.allowed}</span> / <span class="red">${d.blocked}</span></span>
          <span class="ds-last">${formatTime(d.lastSeen)}</span>
        </div>`;
      }).join('');
      html += '</div>';
    }

    document.getElementById('logs-domain-stats').innerHTML = html;
  } catch(e) {
    if (e.message !== 'unauthorized')
      document.getElementById('logs-domain-stats').innerHTML = '';
  }
}

async function loadLogEntries(type, action, domain) {
  try {
    let path = '/logs?since=' + logsSince;
    if (type) path += '&type=' + encodeURIComponent(type);
    if (action) path += '&action=' + encodeURIComponent(action);
    if (domain) path += '&domain=' + encodeURIComponent(domain);

    const logs = await api('GET', path);

    const el = document.getElementById('logs-entries');
    let html = '';
    if (!logs.length) {
      html = `<div class="empty">No requests in the last ${logsSince}</div>`;
    } else {
      // Reverse to show chronological (oldest first), newest at bottom
      const sorted = logs.slice().reverse();
      html = '<div class="box log-entries-box">';
      html += sorted.map(l => `
        <div class="log-row">
          <span class="log-ts">${formatTime(l.ts)}</span>
          <span class="log-action ${l.action}">${l.action}</span>
          <span class="log-method">${esc(l.method)}</span>
          <span class="log-domain">${esc(l.domain)}${l.path && l.path !== '/' ? '<span style="color:var(--muted)">' + esc(l.path) + '</span>' : ''}</span>
          <span class="log-agent">${esc(l.registration)}</span>
          <span class="log-latency">${l.latencyMs > 0 ? l.latencyMs + 'ms' : '-'}</span>
        </div>
      `).join('');
      html += '</div>';
    }
    el.innerHTML = html;
    // Scroll to bottom to show newest entries
    const box = el.querySelector('.log-entries-box');
    if (box) box.scrollTop = box.scrollHeight;
  } catch(e) {
    if (e.message !== 'unauthorized')
      document.getElementById('logs-entries').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

function filterLogsByDomain(domain) {
  document.getElementById('log-domain-filter').value = domain;
  showLogsSubtab('requests', document.getElementById('subtab-requests'));
}

function setLogsRange(since, btn) {
  logsSince = since;
  document.querySelectorAll('#logs-time-range button').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  loadLogs();
}

function setStatsRange(since, btn) {
  logStatsSince = since;
  document.querySelectorAll('#stats-time-range button').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  loadLogStats(document.getElementById('log-template-filter').value);
}
