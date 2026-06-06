// --- Activity (who called which agent) ---

async function loadActivity() {
  try {
    const caller = (document.getElementById('activity-caller-filter')?.value || '').trim();
    const target = (document.getElementById('activity-target-filter')?.value || '').trim();
    const state = document.getElementById('activity-state-filter')?.value || '';
    const qs = new URLSearchParams();
    if (caller) qs.set('caller', caller);
    if (target) qs.set('target', target);
    if (state) qs.set('state', state);
    qs.set('limit', '200');
    const tasks = await api('GET', '/tasks?' + qs.toString());
    renderActivity(tasks);
  } catch(e) {
    if (e.message !== 'forbidden')
      document.getElementById('activity-list').innerHTML = `<p class="error">${esc(e.message)}</p>`;
  }
}

function activityStateBadge(state) {
  const cls = { completed: 'ok', working: 'warn', submitted: 'warn', failed: 'err', canceled: 'muted', unknown: 'muted' }[state] || 'muted';
  return `<span class="badge badge-${cls}">${esc(state || '—')}</span>`;
}

function activityCaller(t) {
  if (!t.callerName) return '<span class="muted">unknown</span>';
  const kind = (t.callerKind && t.callerKind !== 'user')
    ? ` <span class="muted">(${esc(t.callerKind)})</span>` : '';
  return esc(t.callerName) + kind;
}

function renderActivity(tasks) {
  const el = document.getElementById('activity-list');
  if (!tasks || !tasks.length) {
    el.innerHTML = '<p class="empty">No activity yet.</p>';
    return;
  }
  el.innerHTML = `
    <table class="data-table">
      <thead><tr><th>Who</th><th>Agent</th><th>State</th><th>Prompt</th><th>When</th><th></th></tr></thead>
      <tbody>
        ${tasks.map(t => `
          <tr>
            <td>${activityCaller(t)}</td>
            <td>${esc(t.target || '')}</td>
            <td>${activityStateBadge(t.state)}</td>
            <td class="muted" style="max-width:340px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(t.prompt || '')}</td>
            <td>${fmtActivityDate(t.createdAt)}</td>
            <td><button class="btn btn-sm" onclick="viewTask('${esc(t.id)}')">View</button></td>
          </tr>
        `).join('')}
      </tbody>
    </table>`;
}

function fmtActivityDate(v) {
  if (!v) return '<span class="muted">—</span>';
  const d = new Date(v);
  if (isNaN(d)) return '<span class="muted">—</span>';
  return esc(d.toLocaleString());
}

// View shows the whole conversation (thread) the task belongs to — every turn
// that shares its session, in time order, each turn with its delegation subtree.
async function viewTask(id) {
  try {
    const data = await api('GET', '/tasks/' + encodeURIComponent(id) + '/thread');
    renderThreadModal(data);
  } catch(e) {
    alert(e.message);
  }
}

// Render one node (a root turn or a nested delegation) + its children.
function traceNodeHtml(n, depth, focusId) {
  const kids = n.children || [];
  const focus = n.id === focusId ? ' trace-node-focus' : '';
  const sessionBtn = n.targetAgentId && n.session
    ? `<button class="btn btn-sm" onclick="openTaskSession('${esc(n.targetAgentId)}','${esc(n.session)}')">session</button>`
    : '';
  return `
    <div class="trace-node${focus}" style="margin-left:${depth * 20}px">
      <div class="trace-line">
        ${depth > 0 ? '<span class="muted">↳</span> ' : ''}
        <strong>${esc(n.callerName || '?')}</strong>
        <span class="muted">→</span>
        <strong>${esc(n.target || '?')}</strong>
        ${activityStateBadge(n.state)}
        ${sessionBtn}
      </div>
      ${n.prompt ? `<div class="trace-prompt muted">${esc(n.prompt)}</div>` : ''}
    </div>
    ${kids.map(k => traceNodeHtml(k, depth + 1, focusId)).join('')}`;
}

function renderThreadModal(data) {
  const turns = data.turns || [];
  const focusId = data.focusId;
  const body = turns.length
    ? turns.map((t, i) => `
        <div class="thread-turn">
          <div class="thread-turn-head"><span class="thread-turn-num">${i + 1}</span> <span class="muted">${fmtActivityDate(t.createdAt)}</span></div>
          ${traceNodeHtml(t, 0, focusId)}
        </div>`).join('')
    : '<p class="empty">No activity.</p>';
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="closeModal()">
      <div class="modal" onclick="event.stopPropagation()" style="max-width:720px">
        <h3>Conversation</h3>
        <p class="muted" style="margin:0 0 2px"><code>${esc(data.session || '')}</code></p>
        <p class="muted" style="margin:0 0 12px">${turns.length} turn${turns.length === 1 ? '' : 's'} — oldest first; each turn shows what it delegated. “session” opens that hop's conversation.</p>
        <div class="trace-tree">${body}</div>
        <div class="modal-actions"><button class="btn btn-primary" onclick="closeModal()">Close</button></div>
      </div>
    </div>`;
}

// Open a specific hop's conversation in the session viewer. The session file is
// the runtime session with ':' replaced by '-'.
function openTaskSession(agentId, session) {
  const file = (session || '').replace(/:/g, '-');
  closeModal();
  if (typeof openSessions === 'function') {
    openSessions(agentId);
    if (file && typeof sessOpen === 'function') setTimeout(() => sessOpen(file), 50);
  }
}
