// --- Crons Tab ---

let cronsData = [];

function describeCron(expr) {
  try {
    return cronstrue.toString(expr, { use24HourTimeFormat: false });
  } catch (e) {
    return '';
  }
}

async function loadCrons() {
  try {
    if (!registrationsData.length) {
      try {
        const regs = await api('GET', '/agent-registrations');
        if (regs && regs.length) { registrationsData = regs; updateRegistrationFilters(regs); }
      } catch(e) {}
    }
    const templateFilter = document.getElementById('cron-template-filter')?.value || '';
    const url = '/crons' + (templateFilter ? '?type=' + encodeURIComponent(templateFilter) : '');
    const crons = await api('GET', url);
    cronsData = crons || [];

    if (!crons || !crons.length) {
      document.getElementById('crons-list').innerHTML = '<div class="empty">No cron jobs yet. Create one to get started.</div>';
      return;
    }

    let html = `
      <div class="log-stats">
        <div class="log-stat"><div class="label">Total</div><div class="val">${crons.length}</div></div>
        <div class="log-stat"><div class="label">Enabled</div><div class="val green">${crons.filter(c => c.enabled).length}</div></div>
        <div class="log-stat"><div class="label">Disabled</div><div class="val">${crons.filter(c => !c.enabled).length}</div></div>
        <div class="log-stat"><div class="label">One-time</div><div class="val">${crons.filter(c => c.runAt).length}</div></div>
        <div class="log-stat"><div class="label">Failed</div><div class="val red">${crons.filter(c => c.lastStatus === 'error' || c.lastStatus === 'no_agent').length}</div></div>
      </div>
    `;

    html += '<div class="box">';

    // Table header
    html += `
      <div class="cron-header">
        <span class="cron-col-name">Name</span>
        <span class="cron-col-template">Registration</span>
        <span class="cron-col-agent">Agent</span>
        <span class="cron-col-schedule">Schedule</span>
        <span class="cron-col-tz">TZ</span>
        <span class="cron-col-next">Next Run</span>
        <span class="cron-col-last">Last Run</span>
        <span class="cron-col-status">Status</span>
        <span class="cron-col-created">Created</span>
        <span class="cron-col-actions">Actions</span>
      </div>
    `;

    html += crons.map(c => {
      const statusDot = c.lastStatus === 'ok' ? 'var(--green)'
        : (c.lastStatus === 'error' || c.lastStatus === 'no_agent') ? 'var(--red)'
        : 'var(--muted)';
      const statusText = c.lastStatus || 'never';
      const enabledClass = c.enabled ? '' : 'cron-disabled';
      const scheduleDisplay = c.runAt
        ? `<span style="color:var(--muted);font-size:11px">once</span> ${formatTime(c.runAt)}`
        : `<code>${esc(c.schedule)}</code>${describeCron(c.schedule) ? `<div class="hint">${esc(describeCron(c.schedule))}</div>` : ''}`;

      return `
        <div class="cron-row ${enabledClass}" onclick="showCronExecutions(${c.id})" style="cursor:pointer">
          <span class="cron-col-name">
            <strong>${esc(c.name)}</strong>
            ${c.description ? `<div class="hint">${esc(c.description)}</div>` : ''}
          </span>
          <span class="cron-col-template">${esc(c.registrationName || '—')}</span>
          <span class="cron-col-agent">${esc(c.agentName || '—')}</span>
          <span class="cron-col-schedule">${scheduleDisplay}</span>
          <span class="cron-col-tz">${esc(c.timezone)}</span>
          <span class="cron-col-next">${c.nextRunAt ? timeAgo(c.nextRunAt).replace(' ago', '').replace('just now', 'now') : '-'}</span>
          <span class="cron-col-last">${c.lastRunAt ? timeAgo(c.lastRunAt) : 'never'}</span>
          <span class="cron-col-status">
            <span class="event-dot" style="background:${statusDot}"></span>
            ${esc(statusText)}
          </span>
          <span class="cron-col-created">${c.createdAt ? timeAgo(c.createdAt) : '-'}</span>
          <span class="cron-col-actions" onclick="event.stopPropagation()">
            <button class="btn btn-sm" onclick="toggleCron(${c.id}, ${!c.enabled})">${c.enabled ? 'Disable' : 'Enable'}</button>
            <button class="btn btn-sm" onclick="triggerCron(${c.id})" title="Run Now">Run</button>
            <button class="btn btn-sm" onclick="showEditCronModal(${c.id})">Edit</button>
            <button class="btn btn-sm btn-red" onclick="confirmDeleteCron(${c.id}, '${esc(c.name)}')">Del</button>
          </span>
        </div>
      `;
    }).join('');

    html += '</div>';
    html += '<div id="cron-executions"></div>';

    document.getElementById('crons-list').innerHTML = html;
  } catch(e) {
    if (e.message !== 'unauthorized' && e.message !== 'forbidden')
      document.getElementById('crons-list').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}

async function showCronExecutions(cronId) {
  try {
    const execs = await api('GET', `/crons/${cronId}/executions`);
    const cron = cronsData.find(c => c.id === cronId);

    let html = `
      <div style="margin-top:16px;padding:16px;background:var(--surface);border:1px solid var(--border);border-radius:8px">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
          <h3 style="margin:0">Executions &mdash; ${esc(cron?.name || '#' + cronId)}</h3>
          <button class="btn btn-sm" onclick="document.getElementById('cron-executions').innerHTML=''">Close</button>
        </div>
        ${cron?.message ? `<div style="margin-bottom:12px;padding:8px 12px;background:var(--bg);border-radius:4px;font-size:13px;color:var(--muted)"><strong>Message:</strong> ${esc(cron.message)}${cron.session ? '<br><strong>Session:</strong> ' + esc(cron.session) : ''}${cron.agentName ? '<br><strong>Agent:</strong> ' + esc(cron.agentName) : ''}</div>` : ''}
    `;

    if (!execs || !execs.length) {
      html += '<div class="empty">No executions yet</div>';
    } else {
      html += `<div style="border:1px solid var(--border);border-radius:4px;overflow:hidden">`;
      html += execs.map(e => {
        const statusColor = e.status === 'ok' ? 'var(--green)' : 'var(--red)';
        return `
          <div class="event-row">
            <span class="event-dot" style="background:${statusColor}"></span>
            <span class="event-row-time">${formatTime(e.ts)}</span>
            <span class="event-row-type">${esc(e.status)}</span>
            <span class="event-row-detail">${e.agentId ? esc(e.agentId) : '-'} &middot; ${e.durationMs}ms${e.error ? ' &middot; <span style="color:var(--red)">' + esc(e.error) + '</span>' : ''}</span>
          </div>
        `;
      }).join('');
      html += '</div>';
    }

    html += '</div>';
    document.getElementById('cron-executions').innerHTML = html;
  } catch(e) {
    console.error('Failed to load executions:', e);
  }
}

function toggleCronType(prefix) {
  const isOnce = document.getElementById(prefix + '-type-once').checked;
  document.getElementById(prefix + '-schedule-row').style.display = isOnce ? 'none' : '';
  document.getElementById(prefix + '-runat-row').style.display = isOnce ? '' : 'none';
}

function showCreateCronModal() {
  const registrationOptions = '<option value="">— any —</option>' + registrationsData.map(t =>
    `<option value="${esc(t.name)}">${esc(t.name)}</option>`
  ).join('');

  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>New Cron Job</h2>
        <div class="form-group">
          <label>Name</label>
          <input id="cc-name" placeholder="e.g. hourly-health-check" onkeydown="if(event.key==='Enter')createCron()">
        </div>
        <div class="form-group">
          <label>Description <span style="color:var(--muted);font-weight:normal">(optional)</span></label>
          <input id="cc-description" placeholder="Short 1-2 line summary of what this cron does">
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>Registration <span style="color:var(--muted);font-weight:normal">(optional)</span></label>
            <select id="cc-template">${registrationOptions}</select>
            <div class="hint">Target a specific registration, or any healthy agent with the profile</div>
          </div>
          <div class="form-group">
            <label>Agent Profile</label>
            <input id="cc-agentid" placeholder="e.g. ceo" required>
            <div class="hint">Agent profile name to send the message to</div>
          </div>
        </div>
        <div class="form-group">
          <label>Session <span style="color:var(--muted);font-weight:normal">(optional)</span></label>
          <input id="cc-session" placeholder="auto: cron:<id>:<name>">
          <div class="hint">Chat session for continuity across runs</div>
        </div>
        <div class="form-group">
          <label>Type</label>
          <div style="display:flex;gap:16px">
            <label style="font-weight:normal"><input type="radio" name="cc-type" id="cc-type-recurring" checked onchange="toggleCronType('cc')"> Recurring</label>
            <label style="font-weight:normal"><input type="radio" name="cc-type" id="cc-type-once" onchange="toggleCronType('cc')"> One-time</label>
          </div>
        </div>
        <div class="form-row" id="cc-schedule-row">
          <div class="form-group">
            <label>Schedule (cron expression)</label>
            <input id="cc-schedule" placeholder="*/5 * * * *" oninput="document.getElementById('cc-schedule-desc').textContent=describeCron(this.value)">
            <div class="hint" id="cc-schedule-desc"></div>
          </div>
          <div class="form-group">
            <label>Timezone</label>
            <input id="cc-tz" value="UTC" placeholder="UTC">
          </div>
        </div>
        <div class="form-row" id="cc-runat-row" style="display:none">
          <div class="form-group">
            <label>Run At</label>
            <input type="datetime-local" id="cc-runat">
            <div class="hint">One-time execution; auto-disables after firing</div>
          </div>
          <div class="form-group">
            <label>Timezone</label>
            <input id="cc-runat-tz" value="Asia/Kolkata" placeholder="Asia/Kolkata">
          </div>
        </div>
        <div class="form-group">
          <label>Message</label>
          <textarea id="cc-message" rows="3" placeholder="Message sent to agent when cron fires..."></textarea>
        </div>
        <div id="cc-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" id="cc-submit" onclick="createCron()">Create</button>
        </div>
      </div>
    </div>
  `;
  document.getElementById('cc-name').focus();
}

function localDatetimeToRFC3339(datetimeLocal, timezone) {
  // datetime-local gives "2026-03-01T10:00" — we need to convert to RFC3339
  // We'll use a simple offset lookup for common timezones
  const offsets = {
    'Asia/Kolkata': '+05:30', 'Asia/Calcutta': '+05:30',
    'UTC': '+00:00', 'GMT': '+00:00',
    'US/Eastern': '-05:00', 'US/Pacific': '-08:00',
    'America/New_York': '-05:00', 'America/Los_Angeles': '-08:00',
    'Europe/London': '+00:00', 'Europe/Berlin': '+01:00',
    'Asia/Tokyo': '+09:00', 'Asia/Shanghai': '+08:00',
    'Asia/Dubai': '+04:00', 'Asia/Singapore': '+08:00',
  };
  const offset = offsets[timezone] || '+00:00';
  return datetimeLocal + ':00' + offset;
}

async function createCron() {
  const name = document.getElementById('cc-name').value.trim();
  const registrationName = document.getElementById('cc-template').value;
  const agentId = document.getElementById('cc-agentid').value.trim();
  const session = document.getElementById('cc-session').value.trim();
  const message = document.getElementById('cc-message').value.trim();
  const isOnce = document.getElementById('cc-type-once').checked;

  if (!name || !agentId || !message) return;

  const description = document.getElementById('cc-description').value.trim();
  const body = { name, agentId, message };
  if (registrationName) body.registrationName = registrationName;
  if (description) body.description = description;
  if (session) body.session = session;

  if (isOnce) {
    const runat = document.getElementById('cc-runat').value;
    if (!runat) return;
    const tz = document.getElementById('cc-runat-tz').value.trim() || 'UTC';
    body.runAt = localDatetimeToRFC3339(runat, tz);
    body.timezone = tz;
  } else {
    const schedule = document.getElementById('cc-schedule').value.trim();
    if (!schedule) return;
    body.schedule = schedule;
    body.timezone = document.getElementById('cc-tz').value.trim() || 'UTC';
  }

  document.getElementById('cc-submit').disabled = true;
  try {
    await api('POST', '/crons', body);
    closeModal();
    loadCrons();
  } catch(e) {
    document.getElementById('cc-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
    document.getElementById('cc-submit').disabled = false;
  }
}

function showEditCronModal(cronId) {
  const c = cronsData.find(x => x.id === cronId);
  if (!c) return;

  const registrationOptions = '<option value="">— any —</option>' + registrationsData.map(t =>
    `<option value="${esc(t.name)}" ${t.name === c.registrationName ? 'selected' : ''}>${esc(t.name)}</option>`
  ).join('');

  const isOnce = !!c.runAt;
  let runatLocal = '';
  if (c.runAt) {
    runatLocal = c.runAt.slice(0, 16);
  }

  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>Edit Cron &mdash; ${esc(c.name)}</h2>
        <div class="form-group">
          <label>Name</label>
          <input id="ec-name" value="${esc(c.name)}">
        </div>
        <div class="form-group">
          <label>Description <span style="color:var(--muted);font-weight:normal">(optional)</span></label>
          <input id="ec-description" value="${esc(c.description || '')}">
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>Registration <span style="color:var(--muted);font-weight:normal">(optional)</span></label>
            <select id="ec-template">${registrationOptions}</select>
          </div>
          <div class="form-group">
            <label>Agent Profile</label>
            <input id="ec-agentid" value="${esc(c.agentName || '')}" placeholder="e.g. ceo" required>
          </div>
        </div>
        <div class="form-group">
          <label>Session <span style="color:var(--muted);font-weight:normal">(optional)</span></label>
          <input id="ec-session" value="${esc(c.session || '')}" placeholder="auto: cron:<id>:<name>">
        </div>
        <div class="form-group">
          <label>Type</label>
          <div style="display:flex;gap:16px">
            <label style="font-weight:normal"><input type="radio" name="ec-type" id="ec-type-recurring" ${!isOnce ? 'checked' : ''} onchange="toggleCronType('ec')"> Recurring</label>
            <label style="font-weight:normal"><input type="radio" name="ec-type" id="ec-type-once" ${isOnce ? 'checked' : ''} onchange="toggleCronType('ec')"> One-time</label>
          </div>
        </div>
        <div class="form-row" id="ec-schedule-row" style="${isOnce ? 'display:none' : ''}">
          <div class="form-group">
            <label>Schedule (cron expression)</label>
            <input id="ec-schedule" value="${esc(c.schedule)}" oninput="document.getElementById('ec-schedule-desc').textContent=describeCron(this.value)">
            <div class="hint" id="ec-schedule-desc">${esc(describeCron(c.schedule))}</div>
          </div>
          <div class="form-group">
            <label>Timezone</label>
            <input id="ec-tz" value="${esc(c.timezone)}">
          </div>
        </div>
        <div class="form-row" id="ec-runat-row" style="${isOnce ? '' : 'display:none'}">
          <div class="form-group">
            <label>Run At</label>
            <input type="datetime-local" id="ec-runat" value="${runatLocal}">
            <div class="hint">One-time execution; auto-disables after firing</div>
          </div>
          <div class="form-group">
            <label>Timezone</label>
            <input id="ec-runat-tz" value="${esc(c.timezone)}" placeholder="Asia/Kolkata">
          </div>
        </div>
        <div class="form-group">
          <label>Message</label>
          <textarea id="ec-message" rows="3">${esc(c.message)}</textarea>
        </div>
        <div class="form-group">
          <label><input type="checkbox" id="ec-enabled" ${c.enabled ? 'checked' : ''}> Enabled</label>
        </div>
        <div id="ec-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-primary" id="ec-submit" onclick="saveCron(${cronId})">Save</button>
        </div>
      </div>
    </div>
  `;
}

async function saveCron(cronId) {
  const name = document.getElementById('ec-name').value.trim();
  const description = document.getElementById('ec-description').value.trim();
  const agentId = document.getElementById('ec-agentid').value.trim();
  const registrationName = document.getElementById('ec-template').value;
  const session = document.getElementById('ec-session').value.trim();
  const message = document.getElementById('ec-message').value.trim();
  const enabled = document.getElementById('ec-enabled').checked;
  const isOnce = document.getElementById('ec-type-once').checked;

  if (!agentId) return;

  const body = { name, description, agentId, registrationName, session, message, enabled };

  if (isOnce) {
    const runat = document.getElementById('ec-runat').value;
    if (!runat) return;
    const tz = document.getElementById('ec-runat-tz').value.trim() || 'UTC';
    body.runAt = localDatetimeToRFC3339(runat, tz);
    body.schedule = '';
    body.timezone = tz;
  } else {
    body.schedule = document.getElementById('ec-schedule').value.trim();
    body.timezone = document.getElementById('ec-tz').value.trim();
    body.runAt = ''; // clear runAt
  }

  document.getElementById('ec-submit').disabled = true;
  try {
    await api('PUT', `/crons/${cronId}`, body);
    closeModal();
    loadCrons();
  } catch(e) {
    document.getElementById('ec-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
    document.getElementById('ec-submit').disabled = false;
  }
}

async function toggleCron(cronId, enabled) {
  try {
    await api('PUT', `/crons/${cronId}`, { enabled });
    loadCrons();
  } catch(e) {
    alert(e.message);
  }
}

async function triggerCron(cronId) {
  try {
    await api('POST', `/crons/${cronId}/trigger`);
    // Brief delay then refresh to show new execution
    setTimeout(() => loadCrons(), 1000);
  } catch(e) {
    alert(e.message);
  }
}

function confirmDeleteCron(cronId, name) {
  document.getElementById('modal-root').innerHTML = `
    <div class="modal-overlay" onclick="if(event.target===this)closeModal()">
      <div class="modal">
        <h2>Delete Cron</h2>
        <p style="margin-bottom:16px;color:var(--muted)">Delete cron job <strong style="color:var(--text)">${esc(name)}</strong>? This will stop all future executions.</p>
        <div id="dc-result"></div>
        <div class="modal-footer">
          <button class="btn" onclick="closeModal()">Cancel</button>
          <button class="btn btn-sm btn-danger" onclick="deleteCron(${cronId})">Delete</button>
        </div>
      </div>
    </div>
  `;
}

async function deleteCron(cronId) {
  try {
    await api('DELETE', `/crons/${cronId}`);
    closeModal();
    loadCrons();
  } catch(e) {
    document.getElementById('dc-result').innerHTML = `<div class="error-msg">${esc(e.message)}</div>`;
  }
}
