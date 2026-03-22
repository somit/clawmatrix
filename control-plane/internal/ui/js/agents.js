// --- Agents ---

async function loadAgents() {
  const templateFilter = document.getElementById('template-filter').value;
  const path = '/agents' + (templateFilter ? '?type=' + templateFilter : '');
  try {
    const agents = await api('GET', path);
    if (!agents || !agents.length) {
      document.getElementById('agents-list').innerHTML = '<div class="empty">No agents registered</div>';
      return;
    }
    document.getElementById('agents-list').innerHTML = `
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Profile</th>
            <th>Runner</th>
            <th>Status</th>
            <th>Environment</th>
            <th>Stats</th>
            <th>Uptime</th>
            <th>Heartbeat</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          ${agents.map(a => {
            const hasChatUrl = a.meta && a.meta.chatUrl;
            const hasWorkspace = a.meta && a.meta.workspaceUrl;
            const hasSessions = a.meta && a.meta.sessionsUrl;
            return `
            <tr>
              <td class="mono" style="cursor:pointer" onclick="showAgentDetail('${esc(a.id)}')">${esc(a.id)}</td>
              <td>${esc(a.name || '')}</td>
              <td>${a.meta && a.meta.runner ? `<code>${esc(a.meta.runner)}</code>` : '-'}</td>
              <td><span class="badge badge-${a.status}">${a.status}</span>${a.killReason ? ' <span style="color:var(--muted);font-size:11px">(' + esc(a.killReason) + ')</span>' : ''}</td>
              <td><div class="env-pills">${envPills(a.environment)}</div></td>
              <td class="mono">${a.stats.allowed} ok / ${a.stats.blocked} blk${a.stats.avgMs ? ' / ' + a.stats.avgMs + 'ms avg' : ''}</td>
              <td>${timeAgo(a.registeredAt)}</td>
              <td>${timeAgo(a.lastHeartbeat)}</td>
              <td style="display:flex;gap:4px">${hasWorkspace ? `<button class="btn btn-sm btn-blue" onclick="openWorkspace('${esc(a.id)}')">Workspace</button>` : ''}${hasSessions ? `<button class="btn btn-sm btn-yellow" onclick="openSessions('${esc(a.id)}')">Sessions</button>` : ''}${hasChatUrl ? `<button class="btn btn-sm btn-accent" onclick="openChat('${esc(a.id)}', '${esc(a.name)}')">Chat</button>` : ''}</td>
            </tr>
          `}).join('')}
        </tbody>
      </table>
    `;
  } catch(e) {
    if (e.message !== 'unauthorized' && e.message !== 'forbidden')
      document.getElementById('agents-list').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

async function showAgentDetail(id) {
  try {
    const a = await api('GET', '/agents/' + id);
    const panel = document.getElementById('agent-detail');
    panel.style.display = 'block';
    panel.innerHTML = `
      <div class="detail-panel">
        <div class="card-header">
          <h3>${esc(a.id)} <span class="badge badge-${a.status}">${a.status}</span></h3>
          <button class="btn btn-sm" onclick="document.getElementById('agent-detail').style.display='none'">Close</button>
        </div>
        <div class="detail-grid">
          <div class="detail-item"><label>Profile</label><div class="value">${esc(a.name || '')}</div></div>
          <div class="detail-item"><label>Status</label><div class="value">${a.status}${a.killReason ? ' (' + esc(a.killReason) + ')' : ''}</div></div>
          <div class="detail-item"><label>Allowed</label><div class="value">${a.stats.allowed}</div></div>
          <div class="detail-item"><label>Blocked</label><div class="value">${a.stats.blocked}</div></div>
          <div class="detail-item"><label>Avg Latency</label><div class="value">${a.stats.avgMs || 0}ms</div></div>
          <div class="detail-item"><label>Min / Max</label><div class="value">${a.stats.minMs || 0}ms / ${a.stats.maxMs || 0}ms</div></div>
          <div class="detail-item"><label>Registered</label><div class="value">${new Date(a.registeredAt).toLocaleString()}</div></div>
          <div class="detail-item"><label>Last Heartbeat</label><div class="value">${new Date(a.lastHeartbeat).toLocaleString()}</div></div>
        </div>
        <h4 class="section-label">Environment</h4>
        <div class="detail-json">${JSON.stringify(a.environment, null, 2)}</div>
        <h4 class="section-label">Meta</h4>
        <div class="detail-json">${JSON.stringify(a.meta, null, 2)}</div>
        <h4 class="section-label">Gateway</h4>
        <div class="detail-json">${JSON.stringify(a.gateway, null, 2)}</div>
      </div>
    `;
    panel.scrollIntoView({ behavior: 'smooth' });
  } catch(e) {}
}
