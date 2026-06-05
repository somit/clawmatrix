// --- Personal Access Tokens (for CLI tools) ---

async function loadTokens() {
  try {
    const tokens = await api('GET', '/me/tokens');
    renderTokens(tokens);
  } catch(e) {
    if (e.message !== 'forbidden')
      document.getElementById('tokens-list').innerHTML = `<p class="error">${esc(e.message)}</p>`;
  }
}

function fmtTokenDate(v) {
  if (!v) return '<span class="muted">never</span>';
  const d = new Date(v);
  if (isNaN(d)) return '<span class="muted">—</span>';
  return esc(d.toLocaleString());
}

function renderTokens(tokens) {
  const el = document.getElementById('tokens-list');
  if (!tokens || !tokens.length) {
    el.innerHTML = '<p class="empty">No access tokens yet. Create one to authenticate a CLI tool or script.</p>';
    return;
  }
  el.innerHTML = `
    <table class="data-table">
      <thead><tr><th>Name</th><th>Created</th><th>Last used</th><th>Expires</th><th>Actions</th></tr></thead>
      <tbody>
        ${tokens.map(t => `
          <tr>
            <td>${t.name ? esc(t.name) : '<span class="muted">unnamed</span>'}</td>
            <td>${fmtTokenDate(t.createdAt)}</td>
            <td>${fmtTokenDate(t.lastUsedAt)}</td>
            <td>${fmtTokenDate(t.expiresAt)}</td>
            <td>
              <button class="btn btn-sm btn-danger" onclick="deleteToken(${t.id}, '${esc(t.name || 'unnamed')}')">Revoke</button>
            </td>
          </tr>
        `).join('')}
      </tbody>
    </table>`;
}

// --- Create Token Modal ---

function openCreateTokenModal() {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>New Access Token</h3>
        <div class="form-group">
          <label>Name</label>
          <input id="m-token-name" type="text" placeholder="e.g. my laptop" autofocus />
        </div>
        <div class="form-group">
          <label>Expires</label>
          <select id="m-token-expiry">
            <option value="0">Never</option>
            <option value="30">30 days</option>
            <option value="90" selected>90 days</option>
            <option value="365">1 year</option>
          </select>
        </div>
        <div class="modal-error" id="m-error"></div>
        <div class="modal-actions">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" onclick="submitCreateToken()">Create</button>
        </div>
      </div>
    </div>`;
}

async function submitCreateToken() {
  const name = document.getElementById('m-token-name').value.trim();
  const expiresInDays = parseInt(document.getElementById('m-token-expiry').value) || 0;
  const err = document.getElementById('m-error');
  try {
    const res = await api('POST', '/me/tokens', { name, expiresInDays });
    showTokenOnceModal(res.token);
  } catch(e) {
    err.textContent = e.message;
  }
}

// Show the raw token exactly once — it cannot be retrieved again.
function showTokenOnceModal(rawToken) {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeTokenModal()">
      <div class="modal" onclick="event.stopPropagation()">
        <h3>Token created</h3>
        <p class="muted" style="margin:0 0 10px">Copy this token now — it will not be shown again.</p>
        <div style="display:flex;gap:6px;align-items:center">
          <input id="m-token-value" type="text" readonly value="${esc(rawToken)}" style="flex:1;font-family:monospace;font-size:12px" onclick="this.select()" />
          <button class="btn btn-sm" onclick="copyToken()">Copy</button>
        </div>
        <div class="modal-actions">
          <button class="btn btn-primary" onclick="closeTokenModal()">Done</button>
        </div>
      </div>
    </div>`;
}

function copyToken() {
  const input = document.getElementById('m-token-value');
  input.select();
  navigator.clipboard?.writeText(input.value).catch(() => {});
}

function closeTokenModal() {
  closeModal();
  loadTokens();
}

async function deleteToken(id, name) {
  if (!confirm(`Revoke token "${name}"? Any CLI using it will stop working.`)) return;
  try {
    await api('DELETE', `/me/tokens/${id}`);
    loadTokens();
  } catch(e) {
    alert(e.message);
  }
}
