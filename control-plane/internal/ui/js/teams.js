// --- Teams (human groups) ---

async function loadTeams() {
  try {
    const teams = await api('GET', '/groups');
    renderTeams(teams || []);
  } catch(e) {
    if (e.message !== 'unauthorized' && e.message !== 'forbidden')
      document.getElementById('teams-list').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

function renderTeams(teams) {
  const el = document.getElementById('teams-list');
  if (!teams.length) {
    el.innerHTML = '<p class="empty">No teams yet. Create one to grant several people access at once.</p>';
    return;
  }
  el.innerHTML = `
    <table class="data-table">
      <thead><tr><th>Name</th><th>Description</th><th>Members</th><th>Actions</th></tr></thead>
      <tbody>
        ${teams.map(t => `
          <tr>
            <td>${esc(t.name)}</td>
            <td>${t.description ? esc(t.description) : '<span class="muted">—</span>'}</td>
            <td>${t.memberCount}</td>
            <td>
              <button class="btn btn-sm" onclick="showTeamAccessModal(${t.id}, '${esc(t.name)}')">Access</button>
              <button class="btn btn-sm" onclick="showTeamMembersModal(${t.id}, '${esc(t.name)}')">Members</button>
              <button class="btn btn-sm" onclick="openEditTeamModal(${t.id}, '${esc(t.name)}', '${esc(t.description||'')}')">Edit</button>
              <button class="btn btn-sm btn-danger" onclick="deleteTeam(${t.id}, '${esc(t.name)}')">Delete</button>
            </td>
          </tr>
        `).join('')}
      </tbody>
    </table>`;
}

// --- Create / edit team ---

function openCreateTeamModal() {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>New Team</h3>
        <div class="form-group">
          <label>Name</label>
          <input id="m-team-name" type="text" placeholder="e.g. Sales Team" autofocus />
        </div>
        <div class="form-group">
          <label>Description <span class="muted">(optional)</span></label>
          <input id="m-team-desc" type="text" placeholder="what this team is for" />
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" onclick="submitCreateTeam()">Create</button>
        </div>
      </div>
    </div>`;
}

async function submitCreateTeam() {
  const name = document.getElementById('m-team-name').value.trim();
  const description = document.getElementById('m-team-desc').value.trim();
  const err = document.getElementById('m-error');
  if (!name) { err.textContent = 'Name required'; return; }
  try {
    await api('POST', '/groups', { name, description });
    closeModal();
    loadTeams();
  } catch(e) { err.textContent = e.message; }
}

function openEditTeamModal(id, name, description) {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>Edit Team</h3>
        <div class="form-group">
          <label>Name</label>
          <input id="m-team-name" type="text" value="${esc(name)}" />
        </div>
        <div class="form-group">
          <label>Description</label>
          <input id="m-team-desc" type="text" value="${esc(description)}" />
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" onclick="submitEditTeam(${id})">Save</button>
        </div>
      </div>
    </div>`;
}

async function submitEditTeam(id) {
  const name = document.getElementById('m-team-name').value.trim();
  const description = document.getElementById('m-team-desc').value.trim();
  const err = document.getElementById('m-error');
  if (!name) { err.textContent = 'Name required'; return; }
  try {
    await api('PUT', `/groups/${id}`, { name, description });
    closeModal();
    loadTeams();
  } catch(e) { err.textContent = e.message; }
}

async function deleteTeam(id, name) {
  if (!confirm(`Delete team "${name}"? Its members lose any access granted through it.`)) return;
  try {
    await api('DELETE', `/groups/${id}`);
    loadTeams();
  } catch(e) { alert(e.message); }
}

// --- Members ---

async function showTeamMembersModal(id, name) {
  const [members, users] = await Promise.all([
    api('GET', `/groups/${id}/members`).catch(() => []),
    api('GET', '/users').catch(() => []),
  ]);
  renderTeamMembersModal(id, name, members || [], users || []);
}

function renderTeamMembersModal(id, name, members, users) {
  const memberIds = new Set(members.map(m => m.id));
  const rows = members.length
    ? members.map(m => `
        <div class="acl-row">
          <span>${esc(m.username)}${m.email ? ` <span class="muted">${esc(m.email)}</span>` : ''}</span>
          <button class="btn btn-sm btn-danger" onclick="removeTeamMember(${id}, ${m.id}, '${esc(name)}', this)">Remove</button>
        </div>`).join('')
    : '<p class="muted" style="font-size:13px">No members yet.</p>';

  const addable = users.filter(u => !memberIds.has(u.id));
  const userOptions = addable.map(u => `<option value="${u.id}">${esc(u.username)}</option>`).join('');

  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>Members — ${esc(name)}</h3>
        <div class="acl-list">${rows}</div>
        ${addable.length ? `
        <div class="acl-add-row">
          <select id="member-user">${userOptions}</select>
          <button class="btn btn-sm btn-primary" onclick="addTeamMember(${id}, '${esc(name)}')">Add</button>
        </div>` : '<p class="muted" style="font-size:12px">Everyone is already a member.</p>'}
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Done</button>
        </div>
      </div>
    </div>`;
}

async function addTeamMember(id, name) {
  const userId = parseInt(document.getElementById('member-user').value);
  const err = document.getElementById('m-error');
  if (!userId) { err.textContent = 'select a user'; return; }
  try {
    await api('POST', `/groups/${id}/members`, { user_id: userId });
    await showTeamMembersModal(id, name);
  } catch(e) { err.textContent = e.message; }
}

async function removeTeamMember(id, userId, name, btn) {
  btn.disabled = true;
  const err = document.getElementById('m-error');
  try {
    await api('DELETE', `/groups/${id}/members/${userId}`);
    await showTeamMembersModal(id, name);
  } catch(e) { err.textContent = e.message; btn.disabled = false; }
}
