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

// --- Profile ACL Modal ---

async function showProfileACLModal(profileName) {
  const [acl, users, roles] = await Promise.all([
    api('GET', `/agent-profiles/${encodeURIComponent(profileName)}/acl`).catch(() => []),
    api('GET', '/users').catch(() => []),
    api('GET', '/roles').catch(() => []),
  ]);
  const profileRoles = (roles || []).filter(r => r.Scope === 'profile');
  renderProfileACLModal(profileName, acl || [], users || [], profileRoles);
}

function renderProfileACLModal(profileName, acl, users, roles) {
  const userMap = Object.fromEntries((users || []).map(u => [u.id, u.username]));
  const aclRows = acl.length
    ? acl.map(entry => `
        <div class="acl-row">
          <span>${esc(userMap[entry.UserID] || 'user #' + entry.UserID)}</span>
          <span class="pill">${esc(entry.Role?.Name || 'role #' + entry.RoleID)}</span>
          <button class="btn btn-sm btn-danger" onclick="removeProfileACL('${esc(profileName)}', ${entry.UserID}, this)">Remove</button>
        </div>`)
      .join('')
    : '<p class="muted" style="font-size:13px">No access entries yet.</p>';

  const userOptions = users.map(u => `<option value="${u.id}">${esc(u.username)}</option>`).join('');
  const roleOptions = roles.map(r => `<option value="${r.ID}">${esc(r.Name)}</option>`).join('');

  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>Access — ${esc(profileName)}</h3>
        <div class="acl-list" id="acl-list">${aclRows}</div>
        <div class="acl-add-row">
          <select id="acl-user">${userOptions}</select>
          <select id="acl-role">${roleOptions}</select>
          <button class="btn btn-sm btn-primary" onclick="addProfileACL('${esc(profileName)}')">Add</button>
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Done</button>
        </div>
      </div>
    </div>`;
}

async function addProfileACL(profileName) {
  const userId = parseInt(document.getElementById('acl-user').value);
  const roleId = parseInt(document.getElementById('acl-role').value);
  const err = document.getElementById('m-error');
  try {
    await api('POST', `/agent-profiles/${encodeURIComponent(profileName)}/acl`, { user_id: userId, role_id: roleId });
    await showProfileACLModal(profileName);
  } catch(e) {
    err.textContent = e.message;
  }
}

async function removeProfileACL(profileName, userId, btn) {
  btn.disabled = true;
  const err = document.getElementById('m-error');
  try {
    await api('DELETE', `/agent-profiles/${encodeURIComponent(profileName)}/acl/${userId}`);
    await showProfileACLModal(profileName);
  } catch(e) {
    err.textContent = e.message;
    btn.disabled = false;
  }
}
