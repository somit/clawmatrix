// --- Agent Templates ---

let agentTemplatesData = [];

async function loadTemplates() {
  try {
    const templates = await api('GET', '/agent-profiles');
    let html = '<div style="margin-bottom:16px;padding:12px 16px;background:var(--surface);border:1px solid var(--border);border-radius:8px;color:var(--muted);font-size:13px">Agent Profiles define infrastructure provisioning blueprints for agents. This feature will be used for UI-based agent provisioning in a future release.</div>';
    if (!templates || !templates.length) {
      html += '<div class="empty">No profiles yet. Create one to define how agents are provisioned.</div>';
      document.getElementById('templates-list').innerHTML = html;
      return;
    }
    agentTemplatesData = templates;
    html += templates.map(t => `
      <div class="card">
        <div class="card-header">
          <h3>${esc(t.name)}</h3>
          <div class="card-actions">
            <button class="btn btn-sm" onclick="showProfileACLModal('${esc(t.name)}')">Access</button>
            <button class="btn btn-sm" onclick="showEditAgentTemplateModal('${esc(t.name)}')">Edit</button>
            ${t.agents > 0
              ? ''
              : `<button class="btn btn-sm btn-red" onclick="confirmDeleteAgentTemplate('${esc(t.name)}')">Delete</button>`
            }
          </div>
        </div>
        ${t.description ? `<div class="card-desc">${esc(t.description)}</div>` : '<div style="margin-bottom:14px"></div>'}
        <div class="card-stats">
          <div class="card-stat">
            <div class="label">Registration</div>
            <div class="val">${esc(t.registrationName) || '&mdash;'}</div>
          </div>
          <div class="card-stat">
            <div class="label">Image</div>
            <div class="val">${esc(t.image) || '&mdash;'}</div>
          </div>
          <div class="card-stat">
            <div class="label">Max Count</div>
            <div class="val">${t.maxCount || '&infin;'}</div>
          </div>
          <div class="card-stat">
            <div class="label">TTL</div>
            <div class="val">${t.ttlMinutes === -1 ? 'Persistent' : t.ttlMinutes + 'm'}</div>
          </div>
          <div class="card-stat">
            <div class="label">Agents</div>
            <div class="val">${t.agents}</div>
          </div>
        </div>
        <div class="card-footer">
          <span>Updated ${timeAgo(t.updatedAt)}</span>
        </div>
      </div>
    `).join('');
    document.getElementById('templates-list').innerHTML = html;
  } catch(e) {
    if (e.message !== 'unauthorized' && e.message !== 'forbidden')
      document.getElementById('templates-list').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

function showCreateTemplateModal() {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>New Agent Profile</h2>
        <div class="form-group">
          <label>Name</label>
          <input id="at-name" placeholder="e.g. sre-agent" onkeydown="if(event.key==='Enter')createAgentTemplate()">
        </div>
        <div class="form-group">
          <label>Description</label>
          <input id="at-desc" placeholder="SRE monitoring agent profile">
        </div>
        <div class="form-group">
          <label>Registration Name</label>
          <input id="at-reg" placeholder="e.g. ratchet (must exist)">
        </div>
        <div class="form-group">
          <label>Image</label>
          <input id="at-image" placeholder="e.g. ghcr.io/org/agent:latest">
        </div>
        <div class="form-group">
          <label>Max Count (0 = unlimited)</label>
          <input id="at-max" type="number" value="0" min="0">
        </div>
        <div class="form-group">
          <label>TTL Minutes (-1 = persistent)</label>
          <input id="at-ttl" type="number" value="-1" min="-1">
        </div>
        <div id="at-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" id="at-submit" onclick="createAgentTemplate()">Create</button>
        </div>
      </div>
    </div>
  `;
  document.getElementById('at-name').focus();
}

async function createAgentTemplate() {
  const name = document.getElementById('at-name').value.trim();
  const description = document.getElementById('at-desc').value.trim();
  const registrationName = document.getElementById('at-reg').value.trim();
  const image = document.getElementById('at-image').value.trim();
  const maxCount = parseInt(document.getElementById('at-max').value) || 0;
  const ttlMinutes = parseInt(document.getElementById('at-ttl').value);
  if (!name) return;

  document.getElementById('at-submit').disabled = true;
  try {
    await api('POST', '/agent-profiles', { name, description, registrationName, image, maxCount, ttlMinutes: isNaN(ttlMinutes) ? -1 : ttlMinutes });
    closeModal();
    loadTemplates();
  } catch(e) {
    document.getElementById('at-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
    document.getElementById('at-submit').disabled = false;
  }
}

function showEditAgentTemplateModal(name) {
  const t = agentTemplatesData.find(t => t.name === name);
  if (!t) return;
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>Edit &mdash; ${esc(name)}</h2>
        <div class="form-group">
          <label>Name</label>
          <input id="eat-name" value="${esc(t.name)}">
        </div>
        <div class="form-group">
          <label>Description</label>
          <input id="eat-desc" value="${esc(t.description || '')}">
        </div>
        <div class="form-group">
          <label>Registration Name</label>
          <input id="eat-reg" value="${esc(t.registrationName || '')}">
        </div>
        <div class="form-group">
          <label>Image</label>
          <input id="eat-image" value="${esc(t.image || '')}">
        </div>
        <div class="form-group">
          <label>Max Count (0 = unlimited)</label>
          <input id="eat-max" type="number" value="${t.maxCount}" min="0">
        </div>
        <div class="form-group">
          <label>TTL Minutes (-1 = persistent)</label>
          <input id="eat-ttl" type="number" value="${t.ttlMinutes}" min="-1">
        </div>
        <div id="eat-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" id="eat-submit" onclick="saveAgentTemplate('${esc(name)}')">Save</button>
        </div>
      </div>
    </div>
  `;
  document.getElementById('eat-name').focus();
}

async function saveAgentTemplate(originalName) {
  const name = document.getElementById('eat-name').value.trim();
  const description = document.getElementById('eat-desc').value.trim();
  const registrationName = document.getElementById('eat-reg').value.trim();
  const image = document.getElementById('eat-image').value.trim();
  const maxCount = parseInt(document.getElementById('eat-max').value) || 0;
  const ttlMinutes = parseInt(document.getElementById('eat-ttl').value);
  if (!name) return;

  document.getElementById('eat-submit').disabled = true;
  try {
    await api('PUT', '/agent-profiles/' + encodeURIComponent(originalName), { name, description, registrationName, image, maxCount, ttlMinutes: isNaN(ttlMinutes) ? -1 : ttlMinutes });
    closeModal();
    loadTemplates();
  } catch(e) {
    document.getElementById('eat-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
    document.getElementById('eat-submit').disabled = false;
  }
}

function confirmDeleteAgentTemplate(name) {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>Delete Profile</h2>
        <p style="margin-bottom:16px;color:var(--muted)">Are you sure you want to delete <strong style="color:var(--text)">${esc(name)}</strong>?</p>
        <div id="dat-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-sm btn-danger" onclick="deleteAgentTemplate('${esc(name)}')">Delete</button>
        </div>
      </div>
    </div>
  `;
}

async function deleteAgentTemplate(name) {
  try {
    await api('DELETE', '/agent-profiles/' + encodeURIComponent(name));
    closeModal();
    loadTemplates();
  } catch(e) {
    document.getElementById('dat-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
  }
}

// --- Per-principal access summary (read-only) ---
// Shows every resource a human or team can reach, in one place.

function renderAccessSummary(title, subtitle, entries) {
  const rows = entries.length
    ? `<table class="data-table" style="margin-top:4px">
         <thead><tr><th>Resource</th><th>Role</th><th>Granted via</th></tr></thead>
         <tbody>
           ${entries.map(e => {
             const kind = e.resource_type === 'agent' ? 'agent' : 'profile';
             const src = e.source === 'direct'
               ? '<span class="pill">direct</span>'
               : `<span class="pill pill-team">${esc(e.source.replace(/^team:/, '🧑‍🤝‍🧑 '))}</span>`;
             return `<tr>
               <td><span class="muted">${kind}</span> ${esc(e.resource_id)}</td>
               <td><span class="pill">${esc(e.role_name)}</span></td>
               <td>${src}</td>
             </tr>`;
           }).join('')}
         </tbody>
       </table>`
    : '<p class="muted" style="font-size:13px">No access granted yet.</p>';

  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal modal-wide" onclick="event.stopPropagation()">
        <h3>${esc(title)}</h3>
        ${subtitle ? `<p class="muted" style="font-size:12px;margin:0 0 8px">${esc(subtitle)}</p>` : ''}
        ${rows}
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Done</button>
        </div>
      </div>
    </div>`;
}

async function showHumanAccessModal(userId, username) {
  const entries = await api('GET', `/users/${userId}/access`).catch(() => []);
  renderAccessSummary(`Access — ${username}`, 'Everything this person can reach, directly or through their teams.', entries || []);
}

async function showTeamAccessModal(groupId, name) {
  const entries = await api('GET', `/groups/${groupId}/access`).catch(() => []);
  renderAccessSummary(`Access — team ${name}`, 'Every member of this team inherits the grants below.', entries || []);
}

// --- Access (ACL) Modal ---
// Generic over the resource: a profile (/agent-profiles/{name}/acl) or a single
// agent (/agents/{id}/acl). Grantees can be a user or a team (group).

function aclBasePath(kind, id) {
  return kind === 'agent'
    ? `/agents/${encodeURIComponent(id)}/acl`
    : `/agent-profiles/${encodeURIComponent(id)}/acl`;
}

async function showProfileACLModal(profileName) {
  return showACLModal('profile', profileName, profileName);
}

async function showAgentACLModal(agentId) {
  return showACLModal('agent', agentId, agentId);
}

async function showACLModal(kind, id, label) {
  const [acl, users, groups, roles] = await Promise.all([
    api('GET', aclBasePath(kind, id)).catch(() => []),
    api('GET', '/users').catch(() => []),
    api('GET', '/groups').catch(() => []),
    api('GET', '/roles').catch(() => []),
  ]);
  const profileRoles = (roles || []).filter(r => r.Scope === 'profile');
  renderACLModal(kind, id, label, acl || [], users || [], groups || [], profileRoles);
}

function renderACLModal(kind, id, label, acl, users, groups, roles) {
  const aclRows = acl.length
    ? acl.map(entry => {
        const isGroup = entry.principal_type === 'group';
        const name = entry.principal_label || `${entry.principal_type} #${entry.principal_id}`;
        const tag = isGroup ? '<span class="pill pill-team">team</span> ' : '';
        return `
        <div class="acl-row">
          <span>${tag}${esc(name)}</span>
          <span class="pill">${esc(entry.role_name || 'role #' + entry.role_id)}</span>
          <button class="btn btn-sm btn-danger" onclick="removeACL('${kind}','${esc(id)}','${esc(entry.principal_type)}',${entry.principal_id}, this)">Remove</button>
        </div>`;
      }).join('')
    : '<p class="muted" style="font-size:13px">No access entries yet.</p>';

  const userOptions = users.map(u => `<option value="user:${u.id}">${esc(u.username)}</option>`).join('');
  const groupOptions = groups.map(g => `<option value="group:${g.id}">${esc(g.name)} (team)</option>`).join('');
  const granteeOptions = [
    userOptions ? `<optgroup label="Users">${userOptions}</optgroup>` : '',
    groupOptions ? `<optgroup label="Teams">${groupOptions}</optgroup>` : '',
  ].join('');
  const roleOptions = roles.map(r => `<option value="${r.ID}">${esc(r.Name)}</option>`).join('');

  const title = kind === 'agent' ? `Access — agent ${esc(label)}` : `Access — ${esc(label)}`;
  const hint = kind === 'agent'
    ? '<p class="muted" style="font-size:12px">Grants here apply to this agent only, in addition to any access inherited from its profile.</p>'
    : '';

  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>${title}</h3>
        ${hint}
        <div class="acl-list" id="acl-list">${aclRows}</div>
        <div class="acl-add-row">
          <select id="acl-grantee">${granteeOptions}</select>
          <select id="acl-role">${roleOptions}</select>
          <button class="btn btn-sm btn-primary" onclick="addACL('${kind}','${esc(id)}')">Add</button>
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Done</button>
        </div>
      </div>
    </div>`;
}

async function addACL(kind, id) {
  const grantee = document.getElementById('acl-grantee').value; // "user:3" | "group:1"
  const roleId = parseInt(document.getElementById('acl-role').value);
  const err = document.getElementById('m-error');
  if (!grantee) { err.textContent = 'select a user or team'; return; }
  const [ptype, pid] = grantee.split(':');
  try {
    await api('POST', aclBasePath(kind, id), {
      principal_type: ptype, principal_id: parseInt(pid), role_id: roleId,
    });
    await showACLModal(kind, id, id);
  } catch(e) {
    err.textContent = e.message;
  }
}

async function removeACL(kind, id, ptype, pid, btn) {
  btn.disabled = true;
  const err = document.getElementById('m-error');
  try {
    await api('DELETE', `${aclBasePath(kind, id)}/${encodeURIComponent(ptype)}/${pid}`);
    await showACLModal(kind, id, id);
  } catch(e) {
    err.textContent = e.message;
    btn.disabled = false;
  }
}
