// --- Connections ---

async function loadConnections() {
  try {
    const conns = await api('GET', '/connections');
    if (!conns || !conns.length) {
      document.getElementById('connections-list').innerHTML = '<div class="empty">No connections yet. Create one to link agents.</div>';
      return;
    }

    // Group by source
    const grouped = {};
    for (const c of conns) {
      if (!grouped[c.source]) grouped[c.source] = [];
      grouped[c.source].push(c);
    }

    let html = '<div class="connections-grid">';
    for (const [source, targets] of Object.entries(grouped).sort((a, b) => a[0].localeCompare(b[0]))) {
      html += `
        <div class="card">
          <div class="card-header">
            <h3>${esc(source)}</h3>
            <div class="card-actions">
              <span class="badge" style="background:rgba(46,196,182,0.15);color:var(--green)">${targets.length} connection${targets.length !== 1 ? 's' : ''}</span>
            </div>
          </div>
          <div class="card-allowlist">
            ${targets.map(t => `
              <span class="card-allowlist-tag" style="border-color:var(--blue);color:var(--blue);cursor:pointer;position:relative">
                ${esc(t.target)}
                <span onclick="confirmDeleteConnection('${esc(t.source)}','${esc(t.target)}')" style="margin-left:6px;color:var(--red);cursor:pointer;font-weight:bold" title="Remove">&times;</span>
              </span>
            `).join('')}
          </div>
          <div class="card-footer">
            <span>Created ${timeAgo(targets[0].createdAt)}</span>
          </div>
        </div>
      `;
    }
    html += '</div>';
    document.getElementById('connections-list').innerHTML = html;
  } catch(e) {
    if (e.message !== 'unauthorized')
      document.getElementById('connections-list').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

async function showCreateConnectionModal() {
  // Fetch agent profile names for dropdowns
  let agentNames = [];
  try {
    const profiles = await api('GET', '/agent-profiles');
    agentNames = (profiles || []).map(p => p.name).sort();
  } catch(e) {}

  const options = agentNames.map(n => `<option value="${esc(n)}">${esc(n)}</option>`).join('');

  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>New Connection</h2>
        <p style="color:var(--muted);margin-bottom:16px;font-size:13px">Connections are directed: source can talk to targets, but not the reverse unless you create both.</p>
        <div class="form-group">
          <label>Source (agent name)</label>
          <select id="cc-source"><option value="">Select agent...</option>${options}</select>
        </div>
        <div class="form-group">
          <label>Targets (agent names)</label>
          <select id="cc-targets" multiple size="${Math.min(agentNames.length, 6)}" style="min-height:80px">${options}</select>
          <span style="color:var(--muted);font-size:11px;margin-top:4px;display:block">Hold Cmd/Ctrl to select multiple</span>
        </div>
        <div id="cc-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" id="cc-submit" onclick="createConnection()">Create</button>
        </div>
      </div>
    </div>
  `;
  document.getElementById('cc-source').focus();
}

async function createConnection() {
  const source = document.getElementById('cc-source').value.trim();
  const selectEl = document.getElementById('cc-targets');
  const targets = [...selectEl.selectedOptions].map(o => o.value).filter(v => v && v !== source);
  if (!source || !targets.length) return;

  document.getElementById('cc-submit').disabled = true;
  try {
    await api('POST', '/connections', { source, targets });
    closeModal();
    loadConnections();
  } catch(e) {
    document.getElementById('cc-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
    document.getElementById('cc-submit').disabled = false;
  }
}

function confirmDeleteConnection(source, target) {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>Delete Connection</h2>
        <p style="margin-bottom:16px;color:var(--muted)">Remove connection <strong style="color:var(--text)">${esc(source)}</strong> &rarr; <strong style="color:var(--text)">${esc(target)}</strong>?</p>
        <div id="dc-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-sm btn-danger" onclick="deleteConnection('${esc(source)}','${esc(target)}')">Delete</button>
        </div>
      </div>
    </div>
  `;
}

async function deleteConnection(source, target) {
  try {
    await api('DELETE', '/connections', { source, target });
    closeModal();
    loadConnections();
  } catch(e) {
    document.getElementById('dc-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
  }
}
