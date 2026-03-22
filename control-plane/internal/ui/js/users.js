// --- Humans & Roles ---

let rolesData = [];

async function loadHumans() {
  try {
    const users = await api('GET', '/users');
    renderUsers(users);
  } catch(e) {
    if (e.message !== 'forbidden')
      document.getElementById('users-list').innerHTML = `<p class="error">${esc(e.message)}</p>`;
  }
}

async function loadRoles() {
  try {
    const roles = await api('GET', '/roles');
    rolesData = roles || [];
    renderRoles(roles);
  } catch(e) {
    rolesData = [];
  }
}

// Keep loadUsers for backward compat (modal flows call loadUsers after submit)
async function loadUsers() {
  await Promise.all([loadHumans(), loadRoles()]);
}

async function ensureRolesLoaded() {
  if (!rolesData.length) {
    rolesData = await api('GET', '/roles') || [];
  }
}

function renderUsers(users) {
  const el = document.getElementById('users-list');
  if (!users || !users.length) {
    el.innerHTML = '<p class="empty">No users found.</p>';
    return;
  }
  el.innerHTML = `
    <table class="data-table">
      <thead><tr><th>ID</th><th>Username</th><th>Email</th><th>System Role</th><th>Actions</th></tr></thead>
      <tbody>
        ${users.map(u => `
          <tr>
            <td>${esc(String(u.id))}</td>
            <td>${esc(u.username)}</td>
            <td>${u.email ? `<span class="muted">${esc(u.email)}</span>` : '<span class="muted">—</span>'}</td>
            <td>${u.system_role ? `<span class="pill">${esc(u.system_role)}</span>` : '<span class="muted">—</span>'}</td>
            <td>
              <button class="btn btn-sm" onclick="openEditUserModal(${u.id}, '${esc(u.username)}', '${esc(u.email||'')}', '${esc(u.system_role||'')}')">Edit</button>
              <button class="btn btn-sm btn-danger" onclick="deleteUser(${u.id}, '${esc(u.username)}')">Delete</button>
            </td>
          </tr>
        `).join('')}
      </tbody>
    </table>`;
}

function renderRoles(roles) {
  const el = document.getElementById('roles-list');
  if (!roles || !roles.length) {
    el.innerHTML = '<p class="empty">No roles found.</p>';
    return;
  }
  el.innerHTML = `
    <table class="data-table">
      <thead><tr><th>Name</th><th>Scope</th><th>Permissions</th><th>Actions</th></tr></thead>
      <tbody>
        ${roles.map(r => `
          <tr>
            <td>${esc(r.Name)}${r.SystemDefault ? ' <span class="pill">built-in</span>' : ''}</td>
            <td><span class="pill">${esc(r.Scope)}</span></td>
            <td class="perms-cell">${(r.Permissions || []).map(p => `<span class="pill pill-sm">${esc(p.Permission)}</span>`).join(' ')}</td>
            <td>
              <button class="btn btn-sm" onclick="openEditRoleModal(${r.ID})">Edit</button>
              ${!r.SystemDefault ? `<button class="btn btn-sm btn-danger" onclick="deleteRole(${r.ID}, '${esc(r.Name)}')">Delete</button>` : ''}
            </td>
          </tr>
        `).join('')}
      </tbody>
    </table>`;
}

// --- Create User Modal ---

async function openCreateUserModal() {
  await ensureRolesLoaded();
  const systemRoles = rolesData.filter(r => r.Scope === 'system');
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>New User</h3>
        <div class="form-group">
          <label>Username</label>
          <input id="m-username" type="text" placeholder="username" autofocus />
        </div>
        <div class="form-group">
          <label>Password</label>
          <input id="m-password" type="password" placeholder="password" />
        </div>
        <div class="form-group">
          <label>Email <span class="muted">(optional)</span></label>
          <input id="m-email" type="email" placeholder="user@example.com" />
        </div>
        <div class="form-group">
          <label>System Role</label>
          <select id="m-role">
            <option value="">None (member only)</option>
            ${systemRoles.map(r => `<option value="${r.ID}">${esc(r.Name)}</option>`).join('')}
          </select>
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" onclick="submitCreateUser()">Create</button>
        </div>
      </div>
    </div>`;
}

async function submitCreateUser() {
  const username = document.getElementById('m-username').value.trim();
  const password = document.getElementById('m-password').value;
  const email = document.getElementById('m-email').value.trim();
  const roleVal = document.getElementById('m-role').value;
  const err = document.getElementById('m-error');
  if (!username || !password) { err.textContent = 'Username and password required'; return; }
  try {
    await api('POST', '/users', {
      username,
      password,
      email: email || null,
      system_role_id: roleVal ? parseInt(roleVal) : null
    });
    closeModal();
    loadUsers();
  } catch(e) {
    err.textContent = e.message;
  }
}

// --- Edit User Modal ---

async function openEditUserModal(id, username, email, currentRole) {
  await ensureRolesLoaded();
  const systemRoles = rolesData.filter(r => r.Scope === 'system');
  let identities = [];
  try { identities = await api('GET', `/users/${id}/identities`); } catch(_) {}

  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>Edit User: ${esc(username)}</h3>
        <div class="form-group">
          <label>Email</label>
          <input id="m-email" type="email" placeholder="user@example.com" value="${esc(email)}" />
        </div>
        <div class="form-group">
          <label>New Password <span class="muted">(leave blank to keep)</span></label>
          <input id="m-password" type="password" placeholder="new password" />
        </div>
        <div class="form-group">
          <label>System Role</label>
          <select id="m-role">
            <option value="">None (member only)</option>
            ${systemRoles.map(r => `<option value="${r.ID}" ${r.Name === currentRole ? 'selected' : ''}>${esc(r.Name)}</option>`).join('')}
          </select>
        </div>
        <div class="form-group">
          <label>Linked Identities</label>
          <div id="m-identities" class="identities-list">
            ${renderIdentitiesList(id, identities)}
          </div>
          <div class="identities-add" style="display:flex;gap:6px;margin-top:8px">
            <input id="m-ident-provider" type="text" placeholder="provider" style="width:90px;flex:none" />
            <input id="m-ident-extid" type="text" placeholder="external ID" style="flex:1;min-width:0" />
            <button class="btn btn-sm" onclick="addIdentity(${id})">+ Link</button>
          </div>
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" onclick="submitEditUser(${id})">Save</button>
        </div>
      </div>
    </div>`;
}

function renderIdentitiesList(userId, identities) {
  if (!identities || !identities.length) return '<span class="muted" style="font-size:13px">No linked identities</span>';
  return identities.map(i => `
    <div class="identity-row" style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
      <span class="pill pill-sm" style="min-width:48px;text-align:center">${esc(i.provider)}</span>
      <code style="font-size:11px;color:var(--muted);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(i.external_id)}">${esc(i.external_id)}</code>
      <button class="btn btn-sm" style="color:var(--red);border-color:var(--red);padding:2px 8px;font-size:12px" onclick="removeIdentity(${userId}, '${esc(i.provider)}')">×</button>
    </div>
  `).join('');
}

async function addIdentity(userId) {
  const provider = document.getElementById('m-ident-provider').value.trim();
  const externalId = document.getElementById('m-ident-extid').value.trim();
  const err = document.getElementById('m-error');
  if (!provider || !externalId) { err.textContent = 'Provider and external ID required'; return; }
  try {
    await api('POST', `/users/${userId}/identities`, { provider, external_id: externalId });
    const identities = await api('GET', `/users/${userId}/identities`);
    document.getElementById('m-identities').innerHTML = renderIdentitiesList(userId, identities);
    document.getElementById('m-ident-provider').value = '';
    document.getElementById('m-ident-extid').value = '';
    err.textContent = '';
  } catch(e) {
    err.textContent = e.message;
  }
}

async function removeIdentity(userId, provider) {
  try {
    await api('DELETE', `/users/${userId}/identities/${encodeURIComponent(provider)}`);
    const identities = await api('GET', `/users/${userId}/identities`);
    document.getElementById('m-identities').innerHTML = renderIdentitiesList(userId, identities);
  } catch(e) {
    document.getElementById('m-error').textContent = e.message;
  }
}

async function submitEditUser(id) {
  const email = document.getElementById('m-email').value.trim();
  const password = document.getElementById('m-password').value;
  const roleVal = document.getElementById('m-role').value;
  const err = document.getElementById('m-error');
  const body = {};
  if (email !== undefined) body.email = email || '';  // empty string → null on server
  if (password) body.password = password;
  if (roleVal !== undefined) body.system_role_id = roleVal ? parseInt(roleVal) : null;
  try {
    await api('PUT', `/users/${id}`, body);
    closeModal();
    loadUsers();
  } catch(e) {
    err.textContent = e.message;
  }
}

async function deleteUser(id, username) {
  if (!confirm(`Delete user "${username}"?`)) return;
  try {
    await api('DELETE', `/users/${id}`);
    loadUsers();
  } catch(e) {
    alert(e.message);
  }
}

// --- Create Role Modal ---

function openCreateRoleModal() {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>New Role</h3>
        <div class="form-group">
          <label>Name</label>
          <input id="m-name" type="text" placeholder="role name" autofocus />
        </div>
        <div class="form-group">
          <label>Description</label>
          <input id="m-desc" type="text" placeholder="description" />
        </div>
        <div class="form-group">
          <label>Scope</label>
          <select id="m-scope">
            <option value="profile">profile</option>
            <option value="system">system</option>
          </select>
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" onclick="submitCreateRole()">Create</button>
        </div>
      </div>
    </div>`;
}

async function submitCreateRole() {
  const name = document.getElementById('m-name').value.trim();
  const description = document.getElementById('m-desc').value.trim();
  const scope = document.getElementById('m-scope').value;
  const err = document.getElementById('m-error');
  if (!name) { err.textContent = 'Name required'; return; }
  try {
    await api('POST', '/roles', { name, description, scope });
    closeModal();
    loadUsers();
  } catch(e) {
    err.textContent = e.message;
  }
}

async function deleteRole(id, name) {
  if (!confirm(`Delete role "${name}"?`)) return;
  try {
    await api('DELETE', `/roles/${id}`);
    loadUsers();
  } catch(e) {
    alert(e.message);
  }
}

// --- Edit Role Modal ---

const systemPerms = [
  'can_manage_users','can_manage_roles','can_manage_registrations',
  'can_manage_profiles','can_manage_connections','can_manage_crons',
  'can_view_logs','can_view_audit','can_view_metrics',
];
const profilePerms = [
  'can_view_agents','can_chat_with_agents','can_configure_agents',
  'can_view_crons','can_trigger_crons','can_add_crons','can_edit_crons','can_delete_crons',
];

async function openEditRoleModal(id) {
  const role = await api('GET', `/roles/${id}`);
  const currentPerms = new Set((role.Permissions || []).map(p => p.Permission));
  const perms = role.Scope === 'system' ? systemPerms : profilePerms;
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>Edit Role: ${esc(role.Name)}</h3>
        ${!role.SystemDefault ? `
        <div class="form-group">
          <label>Name</label>
          <input id="m-name" type="text" value="${esc(role.Name)}" />
        </div>
        <div class="form-group">
          <label>Description</label>
          <input id="m-desc" type="text" value="${esc(role.Description || '')}" />
        </div>
        ` : ''}
        <div class="form-group">
          <label>Permissions</label>
          <div class="perm-checklist">
            ${perms.map(p => `
              <label class="perm-check">
                <input type="checkbox" value="${p}" ${currentPerms.has(p) ? 'checked' : ''} />
                ${p}
              </label>
            `).join('')}
          </div>
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" onclick="submitEditRole(${id}, ${role.SystemDefault})">Save</button>
        </div>
      </div>
    </div>`;
}

async function submitEditRole(id, isSystemDefault) {
  const err = document.getElementById('m-error');
  const checked = [...document.querySelectorAll('.perm-checklist input:checked')].map(i => i.value);
  const unchecked = [...document.querySelectorAll('.perm-checklist input:not(:checked)')].map(i => i.value);
  try {
    if (!isSystemDefault) {
      const name = document.getElementById('m-name')?.value.trim();
      const description = document.getElementById('m-desc')?.value.trim();
      if (name) await api('PUT', `/roles/${id}`, { name, description });
    }
    await Promise.all([
      ...checked.map(p => api('POST', `/roles/${id}/permissions`, { permission: p })),
      ...unchecked.map(p => api('DELETE', `/roles/${id}/permissions/${p}`)),
    ]);
    closeModal();
    loadUsers();
  } catch(e) {
    err.textContent = e.message;
  }
}
