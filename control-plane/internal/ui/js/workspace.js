// --- Workspace Browser ---

let wsAgentId = null;
let wsMode = 'view'; // 'view' or 'edit'
let wsRawContent = ''; // raw file text for toggling
let wsCurrentFile = ''; // full path of currently open file
let wsTreeCache = {}; // path -> entries (cached dir listings)
let wsExpanded = {};  // path -> true (which dirs are expanded)
let wsChatMessages = []; // {role, text}
let wsChatOpen = true;
let wsLockedFiles = new Set(); // locked file paths

function openWorkspace(agentId) {
  wsAgentId = agentId;
  wsMode = 'view';
  wsRawContent = '';
  wsCurrentFile = '';
  wsTreeCache = {};
  wsExpanded = {};
  document.getElementById('app').style.display = 'none';
  const root = document.getElementById('workspace-root');
  wsChatMessages = [];
  wsChatOpen = true;
  wsLockedFiles = new Set();
  root.innerHTML = `
    <div class="ws-page">
      <div class="ws-header">
        <div class="ws-header-left">
          <button class="btn btn-sm ws-back-btn" onclick="closeWorkspace()"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><path d="M12 19l-7-7 7-7"/></svg> Back</button>
          <h3><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128" width="22" height="22" fill="none" style="vertical-align:middle;margin-right:6px;border-radius:3px"><rect width="128" height="128" rx="18" fill="#0F172A"/><g stroke="#334155" stroke-width="2" opacity="0.85"><circle cx="64" cy="64" r="14"/><circle cx="64" cy="64" r="26"/><circle cx="64" cy="64" r="38"/><circle cx="64" cy="64" r="50"/><path d="M64 8V120"/><path d="M8 64H120"/><path d="M26 26L102 102"/><path d="M102 26L26 102"/></g><defs><g id="wcr" stroke="#7F1D1D" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="0" cy="0" rx="9" ry="7" fill="#EF4444"/><path d="M-12 -2 C-16 -6 -16 2 -12 0"/><path d="M12 -2 C16 -6 16 2 12 0"/><path d="M-7 6 L-12 10"/><path d="M-3 7 L-6 12"/><path d="M3 7 L6 12"/><path d="M7 6 L12 10"/></g><g id="wlb" stroke="#92400E" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="0" cy="0" rx="6" ry="14" fill="#F59E0B"/><path d="M0 14 L-3 20"/><path d="M0 14 L3 20"/><path d="M-10 -6 C-18 -10 -18 -2 -12 -2"/><path d="M10 -6 C18 -10 18 -2 12 -2"/><path d="M-4 2 L-10 6"/><path d="M4 2 L10 6"/></g><g id="wcg" stroke="#166534" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="0" cy="0" rx="6" ry="5" fill="#22C55E"/><path d="M-8 -1 C-12 -4 -12 1 -8 0"/><path d="M8 -1 C12 -4 12 1 8 0"/><path d="M-5 4 L-8 7"/><path d="M5 4 L8 7"/></g></defs><g transform="translate(40 42) rotate(-18)"><use href="#wcr"/></g><g transform="translate(92 44) rotate(20)"><use href="#wlb"/></g><g transform="translate(72 92) rotate(10) scale(0.85)"><use href="#wcg"/></g></svg>${esc(agentId)}</h3>
          <div class="ws-breadcrumb" id="ws-breadcrumb"></div>
        </div>
        <div class="ws-header-right">
          <button class="btn btn-sm" id="ws-chat-toggle" onclick="wsToggleChat()" style="color:var(--energon);border-color:var(--energon)"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg> Chat</button>
        </div>
      </div>
      <div class="ws-body">
        <div class="ws-tree-col">
          <div class="ws-tree-header">
            <span>Files</span>
            <button class="btn btn-sm" onclick="wsRefreshTree()" style="padding:2px 8px;font-size:11px" title="Refresh"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></button>
          </div>
          <div class="ws-tree" id="ws-tree"></div>
        </div>
        <div class="ws-content" id="ws-content">
          <div class="panel-placeholder">Select a file to view</div>
        </div>
        <div class="ws-chat" id="ws-chat">
          <div class="ws-chat-header">
            <span>Agent Chat</span>
            <span style="font-size:11px;color:var(--muted)">session: autobot-manager-workspace-editor-chat</span>
          </div>
          <div class="ws-chat-messages" id="ws-chat-messages">
            <div style="text-align:center;color:var(--muted);font-size:12px;padding:20px">Ask the agent to edit workspace files</div>
          </div>
          <div class="ws-chat-input">
            <input id="ws-chat-input" placeholder="e.g. Update SOUL.md to add..." onkeydown="if(event.key==='Enter'&&!event.shiftKey)wsSendChat()">
            <button id="ws-chat-send" onclick="wsSendChat()">Send</button>
          </div>
        </div>
      </div>
    </div>
  `;
  renderBreadcrumb();
  history.replaceState(null, '', '#workspace:' + agentId);
  // Load locks and tree in parallel
  Promise.all([
    loadDir(''),
    wsLoadLocks()
  ]).then(() => {
    wsExpanded[''] = true;
    renderTree();
  });
}

function closeWorkspace() {
  wsAgentId = null;
  wsRawContent = '';
  wsCurrentFile = '';
  wsTreeCache = {};
  wsExpanded = {};
  document.getElementById('workspace-root').innerHTML = '';
  const app = document.getElementById('app');
  if (app) app.style.display = '';
  history.replaceState(null, '', '#agents');
}

// --- Breadcrumb ---

function renderBreadcrumb() {
  const el = document.getElementById('ws-breadcrumb');
  if (!el) return;
  if (!wsCurrentFile) {
    el.innerHTML = '<span class="ws-crumb">root</span>';
    return;
  }
  const parts = wsCurrentFile.split('/');
  let html = `<span class="ws-crumb" onclick="wsFileClick('')">root</span>`;
  for (let i = 0; i < parts.length; i++) {
    const path = parts.slice(0, i + 1).join('/');
    html += ` <span class="ws-crumb-sep">/</span> <span class="ws-crumb">${esc(parts[i])}</span>`;
  }
  el.innerHTML = html;
}

// --- Tree ---

async function loadDir(dirPath) {
  if (wsTreeCache[dirPath]) return;
  const qp = dirPath ? '?path=' + encodeURIComponent(dirPath) : '';
  const resp = await fetch('/agents/' + encodeURIComponent(wsAgentId) + '/workspace' + qp, {
    headers: { 'Authorization': 'Bearer ' + token }
  });
  if (!resp.ok) return;
  let entries = await resp.json();
  // Hide sessions dir from workspace tree (has its own viewer)
  if (dirPath === '') entries = entries.filter(e => e.name !== 'sessions');
  entries.sort((a, b) => {
    if (a.type !== b.type) return a.type === 'dir' ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
  wsTreeCache[dirPath] = entries;
}

function renderTree() {
  const el = document.getElementById('ws-tree');
  if (!el) return;
  const html = renderDir('', 0);
  el.innerHTML = html || '<div class="loading-msg">Empty</div>';
}

function renderDir(dirPath, depth) {
  const entries = wsTreeCache[dirPath];
  if (!entries) return '';
  return entries.map(e => {
    const fullPath = dirPath ? dirPath + '/' + e.name : e.name;
    const indent = depth * 16;
    const isActive = wsCurrentFile === fullPath;
    if (e.type === 'dir') {
      const expanded = wsExpanded[fullPath];
      const arrow = expanded ? '&#9660;' : '&#9654;';
      let children = '';
      if (expanded && wsTreeCache[fullPath]) {
        children = renderDir(fullPath, depth + 1);
      }
      return `<div class="ws-entry ${expanded ? 'ws-expanded' : ''}" style="padding-left:${12 + indent}px" onclick="wsToggleDir('${esc(fullPath)}')"><span class="ws-arrow">${arrow}</span><span class="ws-entry-name">${esc(e.name)}</span></div>${children}`;
    } else {
      const isLocked = wsLockedFiles.has(fullPath);
      const lockIcon = `<span class="ws-lock ${isLocked ? 'locked' : ''}" onclick="wsToggleLock(event,'${esc(fullPath)}')" title="${isLocked ? 'Unlock' : 'Lock'}">${isLocked ? '&#128274;' : '&#128275;'}</span>`;
      const size = `<span class="ws-entry-size">${formatSize(e.size)}</span>`;
      return `<div class="ws-entry ws-entry-file ${isActive ? 'ws-active' : ''}" style="padding-left:${12 + indent + 16}px" onclick="wsFileClick('${esc(fullPath)}')"><span class="ws-entry-name">${esc(e.name)}</span>${lockIcon}${size}</div>`;
    }
  }).join('');
}

async function wsToggleDir(dirPath) {
  if (wsExpanded[dirPath]) {
    delete wsExpanded[dirPath];
  } else {
    await loadDir(dirPath);
    wsExpanded[dirPath] = true;
  }
  renderTree();
}

function wsFileClick(fullPath) {
  if (!fullPath) return;
  wsCurrentFile = fullPath;
  renderBreadcrumb();
  renderTree();
  wsUpdateHash();
  loadFile(fullPath);
}

function wsUpdateHash() {
  const h = 'workspace:' + wsAgentId + (wsCurrentFile ? ':' + wsCurrentFile : '');
  history.replaceState(null, '', '#' + h);
}

// --- File Viewer ---

function isMarkdown(name) {
  return /\.(md|markdown)$/i.test(name);
}

function renderFileContent() {
  const el = document.getElementById('ws-content');
  if (!el || !wsCurrentFile) return;

  const fileName = wsCurrentFile.split('/').pop();
  const md = isMarkdown(fileName);
  const modeToggle = `
    <div class="ws-mode-toggle">
      <button class="${wsMode === 'view' ? 'active' : ''}" onclick="wsSetMode('view')">${md ? 'View' : 'Source'}</button>
      <button class="${wsMode === 'edit' ? 'active' : ''}" onclick="wsSetMode('edit')">Edit</button>
    </div>`;

  let body;
  if (wsMode === 'edit') {
    body = `<textarea class="ws-editor" id="ws-editor" spellcheck="false">${esc(wsRawContent)}</textarea>`;
  } else if (md) {
    body = `<div class="ws-file-rendered">${mdToHtml(wsRawContent)}</div>`;
  } else {
    body = `<pre class="ws-file-content">${esc(wsRawContent)}</pre>`;
  }

  el.innerHTML = `<div class="ws-file-header"><span>${esc(fileName)}</span>${modeToggle}</div>${body}`;

  if (wsMode === 'edit') {
    const ta = document.getElementById('ws-editor');
    if (ta) ta.addEventListener('keydown', wsEditorKeydown);
  }
}

function wsSetMode(mode) {
  if (wsMode === 'edit') {
    const ta = document.getElementById('ws-editor');
    if (ta) wsRawContent = ta.value;
  }
  wsMode = mode;
  renderFileContent();
}

async function loadFile(fullPath) {
  const el = document.getElementById('ws-content');
  if (!el || !wsAgentId) return;
  el.innerHTML = '<div class="panel-placeholder">Loading...</div>';
  try {
    const resp = await fetch('/agents/' + encodeURIComponent(wsAgentId) + '/workspace?path=' + encodeURIComponent(fullPath), {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!resp.ok) {
      const j = await resp.json().catch(() => ({}));
      el.innerHTML = `<div style="color:var(--red);padding:20px;font-size:12px">${esc(j.error || 'Error ' + resp.status)}</div>`;
      return;
    }
    wsRawContent = await resp.text();
    const fileName = fullPath.split('/').pop();
    wsMode = isMarkdown(fileName) ? 'view' : 'edit';
    renderFileContent();
  } catch(e) {
    el.innerHTML = `<div style="color:var(--red);padding:20px;font-size:12px">${esc(e.message)}</div>`;
  }
}

function wsEditorKeydown(e) {
  if (e.key === 'Tab') {
    e.preventDefault();
    const ta = e.target;
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    ta.value = ta.value.substring(0, start) + '  ' + ta.value.substring(end);
    ta.selectionStart = ta.selectionEnd = start + 2;
  }
}

// --- Workspace Locks ---

async function wsLoadLocks() {
  if (!wsAgentId) return;
  try {
    const resp = await fetch('/agents/' + encodeURIComponent(wsAgentId) + '/workspace/locks', {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (resp.ok) {
      const files = await resp.json();
      wsLockedFiles = new Set(files || []);
    }
  } catch(e) { /* ignore — locks not available */ }
}

async function wsToggleLock(event, fullPath) {
  event.stopPropagation();
  if (!wsAgentId) return;
  const isLocked = wsLockedFiles.has(fullPath);
  try {
    const resp = await fetch('/agents/' + encodeURIComponent(wsAgentId) + '/workspace/locks', {
      method: 'PUT',
      headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: fullPath, locked: !isLocked })
    });
    if (resp.ok) {
      const files = await resp.json();
      wsLockedFiles = new Set(files || []);
      renderTree();
    }
  } catch(e) { /* ignore */ }
}

// --- Workspace Chat ---

function wsToggleChat() {
  wsChatOpen = !wsChatOpen;
  const el = document.getElementById('ws-chat');
  if (el) el.style.display = wsChatOpen ? '' : 'none';
  const btn = document.getElementById('ws-chat-toggle');
  if (btn) btn.style.opacity = wsChatOpen ? '1' : '0.5';
}

async function wsSendChat() {
  const input = document.getElementById('ws-chat-input');
  const sendBtn = document.getElementById('ws-chat-send');
  const message = input.value.trim();
  if (!message || !wsAgentId) return;

  input.value = '';
  wsChatMessages.push({ role: 'user', text: message });
  wsRenderChat();

  sendBtn.disabled = true;
  input.disabled = true;

  const msgIdx = wsChatMessages.length;
  wsChatMessages.push({ role: 'agent', text: '' });
  wsRenderChat();

  // Build prompt — only add file context hint if a file is open
  let prompt = message;
  if (wsCurrentFile) {
    prompt += `\n<context>Currently viewing: ${wsCurrentFile}</context>`;
  }

  try {
    const resp = await fetch('/agents/' + encodeURIComponent(wsAgentId) + '/chat', {
      method: 'POST',
      headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: prompt, session: 'autobot-manager-workspace-editor-chat' })
    });

    if (!resp.ok) {
      let errMsg = 'Error ' + resp.status;
      try { const j = await resp.json(); errMsg = j.error || errMsg; } catch(e) {}
      wsChatMessages[msgIdx] = { role: 'error', text: errMsg };
      wsRenderChat();
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
                if (j.text || j.content || j.delta) {
                  wsChatMessages[msgIdx].text += (j.text || j.content || j.delta);
                }
              } catch(e) {
                wsChatMessages[msgIdx].text += payload;
              }
              wsRenderChat();
            }
          }
        }
        if (buffer.startsWith('data: ')) {
          wsChatMessages[msgIdx].text += buffer.slice(6);
          wsRenderChat();
        }
      } else {
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          wsChatMessages[msgIdx].text += decoder.decode(value, { stream: true });
          wsRenderChat();
        }
        const raw = wsChatMessages[msgIdx].text.trim();
        if (raw.startsWith('{')) {
          try {
            const j = JSON.parse(raw);
            const extracted = j.response || j.message || j.text || j.reply;
            if (extracted) wsChatMessages[msgIdx].text = extracted;
          } catch(e) {}
        }
        wsRenderChat();
      }

      // Refresh tree after agent response (edits may have happened)
      wsRefreshTree();
    }
  } catch(e) {
    wsChatMessages[msgIdx] = { role: 'error', text: 'Failed: ' + e.message };
    wsRenderChat();
  }

  sendBtn.disabled = false;
  input.disabled = false;
  input.focus();
}

function wsRefreshTree() {
  // Invalidate all cached dirs, reload expanded ones, refresh current file
  const expandedDirs = Object.keys(wsTreeCache).filter(d => d === '' || wsExpanded[d]);
  wsTreeCache = {};
  Promise.all(expandedDirs.map(d => loadDir(d))).then(() => {
    renderTree();
    if (wsCurrentFile) loadFile(wsCurrentFile);
  });
}

function wsRenderChat() {
  const el = document.getElementById('ws-chat-messages');
  if (!el) return;
  el.innerHTML = wsChatMessages.map(m => {
    if (m.role === 'error') return `<div class="chat-msg error">${esc(m.text)}</div>`;
    if (m.role === 'agent') return `<div class="chat-msg agent">${mdToHtml(m.text)}</div>`;
    return `<div class="chat-msg user">${esc(m.text)}</div>`;
  }).join('');
  el.scrollTop = el.scrollHeight;
}

// --- Restore from URL hash ---

async function wsRestoreFromHash(agentId, filePath) {
  openWorkspace(agentId);
  if (!filePath) return;
  // Expand all parent dirs then open the file
  const parts = filePath.split('/');
  for (let i = 0; i < parts.length - 1; i++) {
    const dirPath = parts.slice(0, i + 1).join('/');
    await loadDir(dirPath);
    wsExpanded[dirPath] = true;
  }
  renderTree();
  wsFileClick(filePath);
}

// --- Helpers ---

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}
