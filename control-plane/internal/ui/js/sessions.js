// --- Sessions Browser ---

let sessAgentId = null;
let sessActiveFile = null;
let sessChatMessages = [];
let sessChatSession = null;

function openSessions(agentId) {
  sessAgentId = agentId;
  sessActiveFile = null;
  sessChatMessages = [];
  sessChatSession = null;
  document.getElementById('app').style.display = 'none';
  const root = document.getElementById('workspace-root');
  root.innerHTML = `
    <div class="ws-page">
      <div class="ws-header">
        <div class="ws-header-left">
          <button class="btn btn-sm ws-back-btn" onclick="closeSessions()"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><path d="M12 19l-7-7 7-7"/></svg> Back</button>
          <h3><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128" width="22" height="22" fill="none" style="vertical-align:middle;margin-right:6px;border-radius:3px"><rect width="128" height="128" rx="18" fill="#0F172A"/><g stroke="#334155" stroke-width="2" opacity="0.85"><circle cx="64" cy="64" r="14"/><circle cx="64" cy="64" r="26"/><circle cx="64" cy="64" r="38"/><circle cx="64" cy="64" r="50"/><path d="M64 8V120"/><path d="M8 64H120"/><path d="M26 26L102 102"/><path d="M102 26L26 102"/></g><defs><g id="scr" stroke="#7F1D1D" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="0" cy="0" rx="9" ry="7" fill="#EF4444"/><path d="M-12 -2 C-16 -6 -16 2 -12 0"/><path d="M12 -2 C16 -6 16 2 12 0"/><path d="M-7 6 L-12 10"/><path d="M-3 7 L-6 12"/><path d="M3 7 L6 12"/><path d="M7 6 L12 10"/></g><g id="slb" stroke="#92400E" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="0" cy="0" rx="6" ry="14" fill="#F59E0B"/><path d="M0 14 L-3 20"/><path d="M0 14 L3 20"/><path d="M-10 -6 C-18 -10 -18 -2 -12 -2"/><path d="M10 -6 C18 -10 18 -2 12 -2"/><path d="M-4 2 L-10 6"/><path d="M4 2 L10 6"/></g><g id="scg" stroke="#166534" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="0" cy="0" rx="6" ry="5" fill="#22C55E"/><path d="M-8 -1 C-12 -4 -12 1 -8 0"/><path d="M8 -1 C12 -4 12 1 8 0"/><path d="M-5 4 L-8 7"/><path d="M5 4 L8 7"/></g></defs><g transform="translate(40 42) rotate(-18)"><use href="#scr"/></g><g transform="translate(92 44) rotate(20)"><use href="#slb"/></g><g transform="translate(72 92) rotate(10) scale(0.85)"><use href="#scg"/></g></svg>${esc(agentId)} <span style="color:var(--yellow);font-weight:400;font-size:13px">sessions</span></h3>
        </div>
        <div class="ws-header-right"></div>
      </div>
      <div class="ws-body">
        <div class="ws-tree-col">
          <div class="ws-tree-header"><span>Sessions</span></div>
          <div class="ws-tree" id="sess-list"><div class="loading-msg">Loading...</div></div>
        </div>
        <div class="ws-content" id="sess-content">
          <div class="panel-placeholder">Select a session to view</div>
        </div>
        <div class="ws-chat" id="sess-chat" style="display:none">
          <div class="ws-chat-header">
            <span>Continue Chat</span>
            <span style="font-size:11px;color:var(--muted)" id="sess-chat-session-label"></span>
          </div>
          <div class="ws-chat-messages" id="sess-chat-messages">
            <div style="text-align:center;color:var(--muted);font-size:12px;padding:20px">Send a message to continue this session</div>
          </div>
          <div class="ws-chat-input">
            <input id="sess-chat-input" placeholder="Continue the conversation..." onkeydown="if(event.key==='Enter'&&!event.shiftKey)sessSendChat()">
            <button id="sess-chat-send" onclick="sessSendChat()">Send</button>
          </div>
        </div>
      </div>
    </div>
  `;
  history.replaceState(null, '', '#sessions:' + agentId);
  sessLoadList();
}

function closeSessions() {
  sessAgentId = null;
  sessActiveFile = null;
  sessChatMessages = [];
  sessChatSession = null;
  document.getElementById('workspace-root').innerHTML = '';
  const app = document.getElementById('app');
  if (app) app.style.display = '';
  history.replaceState(null, '', '#agents');
}

async function sessLoadList() {
  const el = document.getElementById('sess-list');
  if (!el || !sessAgentId) return;
  try {
    const resp = await fetch('/agents/' + encodeURIComponent(sessAgentId) + '/sessions', {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}));
      el.innerHTML = `<div class="error-msg" style="padding:12px">${esc(j.error || 'Error ' + resp.status)}</div>`;
      return;
    }
    const list = await resp.json();
    if (!list.length) {
      el.innerHTML = '<div class="loading-msg">No sessions</div>';
      return;
    }
    list.sort((a, b) => b.mtime.localeCompare(a.mtime));
    el.innerHTML = list.map(s => {
      const name = s.name.replace(/\.json$/, '');
      const active = sessActiveFile === s.name ? ' ws-active' : '';
      return `<div class="ws-entry ws-entry-file${active}" onclick="sessOpen('${esc(s.name)}')"><span class="ws-entry-name">${esc(name)}</span><span class="ws-entry-size">${sessFormatSize(s.size)}</span></div>`;
    }).join('');
  } catch(e) {
    el.innerHTML = `<div class="error-msg" style="padding:12px">${esc(e.message)}</div>`;
  }
}

async function sessOpen(fileName) {
  sessActiveFile = fileName;
  sessChatSession = fileName.replace(/\.json$/, '');
  sessChatMessages = [];
  sessLoadList(); // re-render to update active highlight

  // Show chat panel and update session label
  const chat = document.getElementById('sess-chat');
  if (chat) chat.style.display = '';
  const label = document.getElementById('sess-chat-session-label');
  if (label) label.textContent = sessChatSession;
  sessRenderChat();

  const content = document.getElementById('sess-content');
  if (!content || !sessAgentId) return;
  content.innerHTML = '<div class="panel-placeholder">Loading...</div>';

  try {
    const resp = await fetch('/agents/' + encodeURIComponent(sessAgentId) + '/sessions?name=' + encodeURIComponent(fileName), {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}));
      content.innerHTML = `<div class="error-msg" style="padding:20px">${esc(j.error || 'Error ' + resp.status)}</div>`;
      return;
    }
    const session = await resp.json();
    sessRender(content, session, fileName);
    history.replaceState(null, '', '#sessions:' + sessAgentId + ':' + fileName);
  } catch(e) {
    content.innerHTML = `<div class="error-msg" style="padding:20px">${esc(e.message)}</div>`;
  }
}

function sessRender(el, session, fileName) {
  const name = fileName.replace(/\.json$/, '');
  const msgs = session.messages || [];
  const meta = [];
  if (session.key) meta.push('Key: ' + session.key);
  if (session.created) meta.push('Created: ' + new Date(session.created).toLocaleString());
  if (session.updated) meta.push('Updated: ' + new Date(session.updated).toLocaleString());
  meta.push(msgs.length + ' messages');

  let html = `<div class="ws-file-header"><span>${esc(name)}</span><span style="font-size:11px;color:var(--muted)">${esc(meta.join(' / '))}</span></div>`;
  html += '<div class="ws-session-messages">';

  if (!msgs.length) {
    html += '<div style="color:var(--muted);padding:20px;text-align:center;font-size:13px">Empty session</div>';
  } else {
    for (const m of msgs) {
      const role = m.role || 'unknown';
      const text = m.content || '';
      if (role === 'user') {
        html += `<div class="ws-sess-msg ws-sess-user"><div class="ws-sess-role">user</div><div class="ws-sess-text">${esc(text)}</div></div>`;
      } else if (role === 'assistant') {
        html += `<div class="ws-sess-msg ws-sess-assistant"><div class="ws-sess-role">assistant</div><div class="ws-sess-text">${mdToHtml(text)}</div></div>`;
      } else if (role === 'system') {
        html += `<div class="ws-sess-msg ws-sess-system"><div class="ws-sess-role">system</div><div class="ws-sess-text">${esc(text)}</div></div>`;
      } else {
        html += `<div class="ws-sess-msg ws-sess-system"><div class="ws-sess-role">${esc(role)}</div><div class="ws-sess-text">${esc(text)}</div></div>`;
      }
    }
  }
  html += '</div>';
  el.innerHTML = html;
}

function sessRestoreFromHash(agentId, fileName) {
  openSessions(agentId);
  if (fileName) sessOpen(fileName);
}

// --- Session Chat ---

async function sessSendChat() {
  const input = document.getElementById('sess-chat-input');
  const sendBtn = document.getElementById('sess-chat-send');
  const message = input.value.trim();
  if (!message || !sessAgentId || !sessChatSession) return;

  input.value = '';
  sessChatMessages.push({ role: 'user', text: message });
  sessRenderChat();
  sendBtn.disabled = true;
  input.disabled = true;

  const msgIdx = sessChatMessages.length;
  sessChatMessages.push({ role: 'agent', text: '' });
  sessRenderChat();

  try {
    const resp = await fetch('/agents/' + encodeURIComponent(sessAgentId) + '/chat', {
      method: 'POST',
      headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
      body: JSON.stringify({ message, session: sessChatSession })
    });

    if (!resp.ok) {
      let errMsg = 'Error ' + resp.status;
      try { const j = await resp.json(); errMsg = j.error || errMsg; } catch(e) {}
      sessChatMessages[msgIdx] = { role: 'error', text: errMsg };
    } else {
      const contentType = resp.headers.get('Content-Type') || '';
      if (contentType.includes('text/event-stream')) {
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop();
          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const payload = line.slice(6);
              try {
                const j = JSON.parse(payload);
                if (j.text || j.content || j.delta) sessChatMessages[msgIdx].text += (j.text || j.content || j.delta);
              } catch(e) { sessChatMessages[msgIdx].text += payload; }
              sessRenderChat();
            }
          }
        }
      } else {
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          sessChatMessages[msgIdx].text += decoder.decode(value, { stream: true });
          sessRenderChat();
        }
        const raw = sessChatMessages[msgIdx].text.trim();
        if (raw.startsWith('{')) {
          try {
            const j = JSON.parse(raw);
            const extracted = j.response || j.message || j.text || j.reply;
            sessChatMessages[msgIdx].text = extracted || '';
            if (j.thinking) sessChatMessages[msgIdx].thinking = j.thinking;
            if (j.error) sessChatMessages[msgIdx] = { role: 'error', text: j.error };
          } catch(e) {}
        }
      }
    }
  } catch(e) {
    sessChatMessages[msgIdx] = { role: 'error', text: 'Failed: ' + e.message };
  }

  sessRenderChat();
  sendBtn.disabled = false;
  input.disabled = false;
  input.focus();
}

function sessRenderChat() {
  const el = document.getElementById('sess-chat-messages');
  if (!el) return;
  if (!sessChatMessages.length) {
    el.innerHTML = '<div style="text-align:center;color:var(--muted);font-size:12px;padding:20px">Send a message to continue this session</div>';
    return;
  }
  el.innerHTML = sessChatMessages.map(m => {
    if (m.role === 'error') return `<div class="chat-msg error">${esc(m.text)}</div>`;
    if (m.role === 'agent') {
      const thinkingHtml = m.thinking ? `<details class="chat-thinking"><summary>Thinking...</summary><div class="chat-thinking-body">${esc(m.thinking)}</div></details>` : '';
      return `<div class="chat-msg agent">${thinkingHtml}${mdToHtml(m.text)}</div>`;
    }
    return `<div class="chat-msg user">${esc(m.text)}</div>`;
  }).join('');
  el.scrollTop = el.scrollHeight;
}

function sessFormatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}
