// --- Labels helpers ---

function renderLabels(labels) {
  if (!labels || typeof labels !== 'object') return '';
  const entries = Object.entries(labels);
  if (!entries.length) return '';
  return entries.map(([k, v]) => `<span class="label-pill">${esc(k)}:${esc(v)}</span>`).join('');
}

function labelsToText(labels) {
  if (!labels || typeof labels !== 'object') return '';
  return Object.entries(labels).map(([k,v]) => k + ':' + v).join('\n');
}

function parseLabelsText(text) {
  const labels = {};
  text.split('\n').map(s => s.trim()).filter(Boolean).forEach(line => {
    const idx = line.indexOf(':');
    if (idx > 0) {
      labels[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
    }
  });
  return labels;
}

// --- Registrations ---

function showRegistrationsSubtab(name, btn) {
  document.querySelectorAll('#registrations-tab .subtab').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  document.getElementById('registrations-list').style.display = name === 'active' ? '' : 'none';
  document.getElementById('registrations-archived-list').style.display = name === 'archived' ? '' : 'none';
}

function renderRegistrationCard(t) {
  const labelHtml = renderLabels(t.labels);
  return `
    <div class="card">
      <div class="card-header">
        <h3>${esc(t.name)}</h3>
        <div class="card-actions">
          <button class="btn btn-sm" onclick="showEditRegistrationModal('${esc(t.name)}')">Edit</button>
          ${t.totalRegistered > 0
            ? (t.archived
              ? `<button class="btn btn-sm btn-green" onclick="toggleArchiveRegistration('${esc(t.name)}', false)">Unarchive</button>`
              : `<button class="btn btn-sm btn-yellow" onclick="toggleArchiveRegistration('${esc(t.name)}', true)">Archive</button>`)
            : `<button class="btn btn-sm btn-red" onclick="confirmDeleteRegistration('${esc(t.name)}')">Delete</button>`
          }
        </div>
      </div>
      ${t.description ? `<div class="card-desc">${esc(t.description)}</div>` : '<div style="margin-bottom:14px"></div>'}
      ${labelHtml ? `<div class="card-labels">${labelHtml}</div>` : ''}
      <div class="card-stats">
        <div class="card-stat"><div class="label">Agents</div><div class="val">${t.agents}</div></div>
        <div class="card-stat"><div class="label">TTL</div><div class="val">${t.ttlMinutes === -1 ? '&infin;' : t.ttlMinutes + 'm'}</div></div>
        <div class="card-stat"><div class="label">Total Registered</div><div class="val">${t.totalRegistered}</div></div>
        <div class="card-stat"><div class="label">Allowlist Rules</div><div class="val">${t.egressAllowlist ? t.egressAllowlist.length : 0}</div></div>
      </div>
      ${t.egressAllowlist && t.egressAllowlist.length ? `
        <div class="card-allowlist">
          ${t.egressAllowlist.map(a => `<span class="card-allowlist-tag">${esc(a)}</span>`).join('')}
        </div>
      ` : ''}
      <div class="card-footer">
        <span>Updated ${timeAgo(t.updatedAt)}</span>
        <span class="monitor-status ${t.monitoringEnabled ? 'on' : 'off'}"><span class="dot"></span>${t.monitoringEnabled ? 'Network Monitoring Active' : 'Network Monitoring Off'}</span>
      </div>
    </div>
  `;
}

async function loadRegistrations() {
  try {
    const regs = await api('GET', '/agent-registrations');
    registrationsData = regs || [];

    const active = registrationsData.filter(r => !r.archived);
    const archived = registrationsData.filter(r => r.archived);

    document.getElementById('registrations-list').innerHTML = active.length
      ? active.map(renderRegistrationCard).join('')
      : '<div class="empty">No registrations yet. Create one to get started.</div>';

    document.getElementById('registrations-archived-list').innerHTML = archived.length
      ? archived.map(renderRegistrationCard).join('')
      : '<div class="empty">No archived registrations.</div>';

    updateRegistrationFilters(regs);
  } catch(e) {
    if (e.message !== 'unauthorized' && e.message !== 'forbidden')
      document.getElementById('registrations-list').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

function showCreateRegistrationModal() {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>New Registration</h2>
        <div class="form-group">
          <label>Name</label>
          <input id="cr-name" placeholder="e.g. ratchet" onkeydown="if(event.key==='Enter')createRegistration()">
        </div>
        <div class="form-group">
          <label>Description</label>
          <input id="cr-desc" placeholder="SRE monitoring agent">
        </div>
        <div class="form-group">
          <label>Labels (key:value, one per line)</label>
          <textarea id="cr-labels" rows="3" placeholder="env:prod&#10;team:sre"></textarea>
        </div>
        <div class="form-group">
          <label>Allowlist (one domain per line)</label>
          <textarea id="cr-allowlist" rows="4" placeholder="*.googleapis.com&#10;*.svc.cluster.local&#10;metadata.google.internal"></textarea>
        </div>
        <div class="form-group">
          <label>TTL Minutes (-1 = persistent)</label>
          <input id="cr-ttl" type="number" value="-1" min="-1">
        </div>
        <div id="cr-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" id="cr-submit" onclick="createRegistration()">Create</button>
        </div>
      </div>
    </div>
  `;
  document.getElementById('cr-name').focus();
}

async function createRegistration() {
  const name = document.getElementById('cr-name').value.trim();
  const description = document.getElementById('cr-desc').value.trim();
  const allowlist = document.getElementById('cr-allowlist').value.split('\n').map(s => s.trim()).filter(Boolean);
  const labels = parseLabelsText(document.getElementById('cr-labels').value);
  const ttlMinutes = parseInt(document.getElementById('cr-ttl').value);
  if (!name) return;

  document.getElementById('cr-submit').disabled = true;
  try {
    const result = await api('POST', '/agent-registrations', { name, description, egressAllowlist: allowlist, labels, ttlMinutes });
    document.getElementById('cr-result').innerHTML = `
      <div class="token-display">
        <div style="font-weight:600;margin-bottom:4px">Registration token:</div>
        <code>${esc(result.token)}</code>
        <div class="hint">Copy this token now. It won't be shown again. Use as AGENT_TOKEN in the sidecar env.</div>
      </div>
    `;
    document.getElementById('cr-submit').style.display = 'none';
    loadRegistrations();
  } catch(e) {
    document.getElementById('cr-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
    document.getElementById('cr-submit').disabled = false;
  }
}

function showEditRegistrationModal(name) {
  const t = registrationsData.find(t => t.name === name);
  if (!t) return;
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>Edit &mdash; ${esc(name)}</h2>
        <div class="form-group">
          <label>Name</label>
          <input id="er-name" value="${esc(t.name)}">
        </div>
        <div class="form-group">
          <label>Description</label>
          <input id="er-desc" value="${esc(t.description || '')}">
        </div>
        <div class="form-group">
          <label>Labels (key:value, one per line)</label>
          <textarea id="er-labels" rows="3">${labelsToText(t.labels)}</textarea>
        </div>
        <div class="form-group">
          <label>Allowlist (one domain per line)</label>
          <textarea id="er-allowlist" rows="6">${(t.egressAllowlist || []).join('\n')}</textarea>
        </div>
        <div class="form-group">
          <label>TTL Minutes (-1 = persistent)</label>
          <input id="er-ttl" type="number" value="${t.ttlMinutes}" min="-1">
        </div>
        <div id="er-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" id="er-submit" onclick="saveRegistration('${esc(name)}')">Save</button>
        </div>
      </div>
    </div>
  `;
  document.getElementById('er-name').focus();
}

async function saveRegistration(originalName) {
  const name = document.getElementById('er-name').value.trim();
  const description = document.getElementById('er-desc').value.trim();
  const allowlist = document.getElementById('er-allowlist').value.split('\n').map(s => s.trim()).filter(Boolean);
  const labels = parseLabelsText(document.getElementById('er-labels').value);
  const ttlMinutes = parseInt(document.getElementById('er-ttl').value);
  if (!name) return;

  document.getElementById('er-submit').disabled = true;
  try {
    await api('PUT', '/agent-registrations/' + encodeURIComponent(originalName), { name, description, egressAllowlist: allowlist, labels, ttlMinutes });
    closeModal();
    loadRegistrations();
  } catch(e) {
    document.getElementById('er-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
    document.getElementById('er-submit').disabled = false;
  }
}

function confirmDeleteRegistration(name) {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>Delete Registration</h2>
        <p style="margin-bottom:16px;color:var(--muted)">Are you sure you want to delete <strong style="color:var(--text)">${esc(name)}</strong>?</p>
        <div id="dr-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-sm btn-danger" onclick="deleteRegistration('${esc(name)}')">Delete</button>
        </div>
      </div>
    </div>
  `;
}

async function deleteRegistration(name) {
  try {
    await api('DELETE', '/agent-registrations/' + encodeURIComponent(name));
    closeModal();
    loadRegistrations();
  } catch(e) {
    document.getElementById('dr-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
  }
}

async function toggleArchiveRegistration(name, archived) {
  try {
    await api('PUT', '/agent-registrations/' + encodeURIComponent(name) + '/archive', { archived });
    await loadRegistrations();
    // Switch to the relevant subtab after action
    const targetTab = archived ? 'archived' : 'active';
    const btn = document.getElementById(`subtab-regs-${targetTab}`);
    if (btn) showRegistrationsSubtab(targetTab, btn);
  } catch(e) {
    alert(e.message);
  }
}
