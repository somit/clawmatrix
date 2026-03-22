// --- Global State ---
let token = localStorage.getItem('cp_token') || '';
let currentUser = null;
let refreshTimer = null;
let registrationsData = [];
let eventSource = null;
let dashboardEvents = []; // collected from SSE for the event feed

// --- Auth ---

async function doLogin() {
  const username = document.getElementById('login-username').value.trim();
  const password = document.getElementById('login-password').value;
  const err = document.getElementById('login-error');
  const btn = document.getElementById('login-btn');
  if (!username || !password) { err.textContent = 'Username and password required'; return; }

  btn.disabled = true;
  btn.textContent = 'Signing in...';
  err.textContent = '';

  try {
    const resp = await fetch('/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password })
    });
    const data = await resp.json();
    if (resp.status === 401) {
      err.textContent = 'Invalid credentials';
      btn.disabled = false;
      btn.textContent = 'Sign in';
      return;
    }
    if (!resp.ok) throw new Error(data.error || 'Server error');

    token = data.token;
    currentUser = { username: data.username, system_role: data.system_role };
    localStorage.setItem('cp_token', token);
    enterApp();
  } catch(e) {
    err.textContent = e.message === 'Invalid credentials' ? e.message : 'Cannot reach server';
    btn.disabled = false;
    btn.textContent = 'Sign in';
  }
}

function doLogout() {
  token = '';
  currentUser = null;
  localStorage.removeItem('cp_token');
  if (refreshTimer) clearInterval(refreshTimer);
  if (eventSource) { eventSource.close(); eventSource = null; }
  closeChat();
  closeWorkspace();
  if (typeof closeSessions === 'function') closeSessions();
  document.getElementById('app').classList.remove('visible');
  document.getElementById('login-screen').style.display = 'flex';
  document.getElementById('login-username').value = '';
  document.getElementById('login-password').value = '';
  document.getElementById('login-error').textContent = '';
  const loginBtn = document.getElementById('login-btn');
  if (loginBtn) { loginBtn.disabled = false; loginBtn.textContent = 'Sign in'; }
}

async function enterApp() {
  document.getElementById('login-screen').style.display = 'none';
  document.getElementById('app').classList.add('visible');
  // Always fetch /auth/me to get full user info
  try {
    currentUser = await api('GET', '/auth/me');
  } catch(e) { doLogout(); return; }
  renderUserInfo();
  initTZSelect();
  loadHealth();
  initTabFromHash();
  refreshTimer = setInterval(refresh, 30000);
  connectSSE();
}

function renderUserInfo() {
  const el = document.getElementById('current-user');
  if (!el || !currentUser) return;
  el.textContent = currentUser.username + (currentUser.system_role ? ' (' + currentUser.system_role + ')' : '');
}

// OIDC — handle hash fragment on page load (#oidc_token=... or #oidc_error=...)
(function() {
  const hash = window.location.hash;
  if (hash.startsWith('#oidc_token=')) {
    token = hash.slice('#oidc_token='.length);
    localStorage.setItem('cp_token', token);
    history.replaceState(null, '', '/');
  } else if (hash.startsWith('#oidc_error=')) {
    const code = hash.slice('#oidc_error='.length);
    history.replaceState(null, '', '/');
    document.getElementById('login-screen').style.display = 'flex';
    const msgs = {
      not_registered: 'Your account is not registered. Ask an admin to add you.',
      invalid_state:  'Login session expired. Please try again.',
    };
    document.getElementById('login-error').textContent = msgs[code] || 'SSO login failed (' + code + ')';
    initOIDCButton();
    return;
  }
})();

function doOIDCLogin() {
  window.location.href = '/auth/oidc/start';
}

const oidcIcons = {
  google:    '<svg width="18" height="18" viewBox="0 0 48 48"><path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.08 17.74 9.5 24 9.5z"/><path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"/><path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"/><path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.18 1.48-4.97 2.31-8.16 2.31-6.26 0-11.57-3.59-13.46-8.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"/></svg>',
  github:    '<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/></svg>',
  microsoft: '<svg width="18" height="18" viewBox="0 0 21 21"><rect x="1" y="1" width="9" height="9" fill="#F25022"/><rect x="11" y="1" width="9" height="9" fill="#7FBA00"/><rect x="1" y="11" width="9" height="9" fill="#00A4EF"/><rect x="11" y="11" width="9" height="9" fill="#FFB900"/></svg>',
  okta:      '',
  keycloak:  '',
  custom:    '',
};

function initOIDCButton() {
  fetch('/auth/oidc/config')
    .then(r => r.json())
    .then(cfg => {
      if (!cfg.enabled) return;
      document.getElementById('oidc-divider').style.display = '';
      const btn = document.getElementById('oidc-btn');
      const icon = oidcIcons[cfg.button_icon] || '';
      btn.innerHTML = (icon ? '<span class="oidc-btn-icon">' + icon + '</span>' : '') + cfg.button_label;
      btn.style.display = '';
    })
    .catch(() => {});
}

// Auto-login if token exists
if (token) {
  fetch('/auth/me', { headers: { 'Authorization': 'Bearer ' + token } })
    .then(r => { if (r.ok) return r.json(); throw new Error(); })
    .then(me => { currentUser = me; enterApp(); })
    .catch(() => doLogout());
} else {
  document.getElementById('login-screen').style.display = 'flex';
  initOIDCButton();
}

// --- SSE ---

function connectSSE() {
  if (eventSource) eventSource.close();
  eventSource = new EventSource('/events?token=' + encodeURIComponent(token));

  eventSource.addEventListener('agent:registered', (e) => {
    pushDashboardEvent('agent:registered', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'agents-tab') loadAgents();
    if (active && active.id === 'dashboard-tab') loadDashboardKPIs();
    loadHealth();
  });

  eventSource.addEventListener('agent:stale', (e) => {
    pushDashboardEvent('agent:stale', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'agents-tab') loadAgents();
    if (active && active.id === 'dashboard-tab') loadDashboardKPIs();
    loadHealth();
  });

  eventSource.addEventListener('agent:recovered', (e) => {
    pushDashboardEvent('agent:recovered', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'agents-tab') loadAgents();
    if (active && active.id === 'dashboard-tab') loadDashboardKPIs();
    loadHealth();
  });

  eventSource.addEventListener('agent:killed', (e) => {
    pushDashboardEvent('agent:killed', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'agents-tab') loadAgents();
    if (active && active.id === 'dashboard-tab') loadDashboardKPIs();
    loadHealth();
  });

  eventSource.addEventListener('log:batch', (e) => {
    pushDashboardEvent('log:batch', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'logs-tab') loadLogs();
  });

  eventSource.addEventListener('registration:created', (e) => {
    pushDashboardEvent('registration:created', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'registrations-tab') loadRegistrations();
  });

  eventSource.addEventListener('registration:updated', (e) => {
    pushDashboardEvent('registration:updated', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'registrations-tab') loadRegistrations();
  });

  eventSource.addEventListener('connection:created', (e) => {
    pushDashboardEvent('connection:created', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'connections-tab') loadConnections();
  });

  eventSource.addEventListener('connection:deleted', (e) => {
    pushDashboardEvent('connection:deleted', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'connections-tab') loadConnections();
  });

  eventSource.addEventListener('template:created', (e) => {
    pushDashboardEvent('template:created', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'templates-tab') loadTemplates();
  });

  eventSource.addEventListener('template:updated', (e) => {
    pushDashboardEvent('template:updated', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'templates-tab') loadTemplates();
  });

  eventSource.addEventListener('cron:created', (e) => {
    pushDashboardEvent('cron:created', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'crons-tab') loadCrons();
  });

  eventSource.addEventListener('cron:executed', (e) => {
    pushDashboardEvent('cron:executed', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'crons-tab') loadCrons();
  });

  eventSource.addEventListener('cron:failed', (e) => {
    pushDashboardEvent('cron:failed', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'crons-tab') loadCrons();
  });

  eventSource.addEventListener('cron:updated', (e) => {
    pushDashboardEvent('cron:updated', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'crons-tab') loadCrons();
  });

  eventSource.addEventListener('cron:deleted', (e) => {
    pushDashboardEvent('cron:deleted', e.data);
    const active = document.querySelector('section.active');
    if (active && active.id === 'crons-tab') loadCrons();
  });

  eventSource.addEventListener('health:update', () => loadHealth());

  eventSource.onerror = () => {
    // Silently reconnect — EventSource auto-reconnects
  };
}

function pushDashboardEvent(type_, rawData) {
  let data = {};
  try { data = JSON.parse(rawData); } catch(e) {}
  dashboardEvents.unshift({ type: type_, data, ts: new Date() });
  if (dashboardEvents.length > 50) dashboardEvents.length = 50;
  const active = document.querySelector('section.active');
  if (active && active.id === 'dashboard-tab') renderDashboardEvents();
}

// --- API ---

async function api(method, path, body) {
  const opts = {
    method,
    headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' }
  };
  if (body) opts.body = JSON.stringify(body);
  const resp = await fetch(path, opts);
  if (resp.status === 401) { doLogout(); throw new Error('unauthorized'); }
  if (resp.status === 403) { throw new Error('forbidden'); }
  if (resp.status === 304) return null;
  const data = await resp.json();
  if (!resp.ok) throw new Error(data.error || resp.statusText);
  return data;
}

// --- Tabs ---

function showTab(name, btn) {
  document.querySelectorAll('section').forEach(s => s.classList.remove('active'));
  document.querySelectorAll('nav button').forEach(b => b.classList.remove('active'));
  document.getElementById(name + '-tab').classList.add('active');
  if (btn) btn.classList.add('active');
  else document.querySelector(`nav button[onclick*="${name}"]`).classList.add('active');
  const detailEl = document.getElementById('agent-detail');
  if (detailEl) detailEl.style.display = 'none';
  history.replaceState(null, '', '#' + name);
  if (name === 'dashboard') loadDashboard();
  if (name === 'registrations') loadRegistrations();
  if (name === 'templates') loadTemplates();
  if (name === 'connections') loadConnections();
  if (name === 'agents') loadAgents();
  if (name === 'logs') loadLogs();
  if (name === 'crons') loadCrons();
  if (name === 'events') loadEventsTab();
  if (name === 'humans') loadHumans();
  if (name === 'roles') loadRoles();
}

function initTabFromHash() {
  const hash = location.hash.replace('#', '') || 'dashboard';

  // Close any open full-page views first
  const wsRoot = document.getElementById('workspace-root');
  if (wsRoot && wsRoot.innerHTML) {
    wsRoot.innerHTML = '';
    const app = document.getElementById('app');
    if (app) app.style.display = '';
  }

  if (hash.startsWith('workspace:')) {
    const parts = hash.split(':');
    const agentId = parts[1] || '';
    const filePath = parts.slice(2).join(':') || '';
    if (agentId) wsRestoreFromHash(agentId, filePath);
    return;
  }
  if (hash.startsWith('sessions:')) {
    const parts = hash.split(':');
    const agentId = parts[1] || '';
    const fileName = parts.slice(2).join(':') || '';
    if (agentId) sessRestoreFromHash(agentId, fileName);
    return;
  }
  if (['dashboard', 'registrations', 'templates', 'connections', 'agents', 'logs', 'crons', 'events', 'humans', 'roles'].includes(hash)) {
    showTab(hash);
  }
}

window.addEventListener('hashchange', () => initTabFromHash());

// --- Health ---

async function loadHealth() {
  try {
    const h = await fetch('/health').then(r => r.json());
    document.getElementById('health').innerHTML = `
      <span><span class="dot ok"></span>${h.healthy} healthy</span>
      <span>${h.agents} total</span>
    `;
  } catch(e) {
    document.getElementById('health').innerHTML = '<span style="color:var(--red)">offline</span>';
  }
}

// --- Shared Helpers ---

function updateRegistrationFilters(registrations) {
  ['template-filter', 'log-template-filter', 'dashboard-template-filter', 'cron-template-filter'].forEach(id => {
    const sel = document.getElementById(id);
    if (!sel) return;
    const cur = sel.value;
    sel.innerHTML = '<option value="">All registrations</option>' + registrations.map(t => `<option value="${esc(t.name)}">${esc(t.name)}</option>`).join('');
    sel.value = cur;
  });
}

function closeModal() {
  document.getElementById('modal-root').innerHTML = '';
}

function envPills(env) {
  if (!env) return '';
  const pills = [];
  if (env.runtime) pills.push(env.runtime);
  if (env.cluster) pills.push(env.cluster);
  if (env.namespace) pills.push(env.namespace);
  if (env.zone) pills.push(env.zone);
  if (env.podName) pills.push(env.podName);
  return pills.map(p => `<span class="pill">${esc(p)}</span>`).join('');
}

// --- Timezone ---

function getTZ() {
  const m = document.cookie.match(/(?:^|;\s*)ui_tz=([^;]*)/);
  return m ? decodeURIComponent(m[1]) : Intl.DateTimeFormat().resolvedOptions().timeZone;
}

function setTZ(tz) {
  document.cookie = 'ui_tz=' + encodeURIComponent(tz) + ';path=/;max-age=31536000';
}

function onTZChange(tz) {
  setTZ(tz);
  // Refresh current tab to re-render all times
  const active = document.querySelector('nav button.active');
  if (active) active.click();
}

const TZ_LIST = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Moscow',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Bangkok',
  'Asia/Singapore',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Australia/Sydney',
  'Pacific/Auckland',
];

function initTZSelect() {
  const sel = document.getElementById('tz-select');
  if (!sel) return;
  const current = getTZ();
  const list = TZ_LIST.includes(current) ? TZ_LIST : [current, ...TZ_LIST];
  sel.innerHTML = list.map(z => `<option value="${z}"${z === current ? ' selected' : ''}>${z}</option>`).join('');
}

function timeAgo(iso) {
  if (!iso) return '-';
  const d = new Date(iso);
  const s = Math.floor((Date.now() - d.getTime()) / 1000);
  if (s < 0) return formatDateTime(iso);
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.floor(s/60) + 'm ago';
  if (s < 86400) return Math.floor(s/3600) + 'h ago';
  return formatDateTime(iso);
}

function formatTime(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  return d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit', timeZone: getTZ() });
}

function formatDateTime(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  return d.toLocaleString('en-GB', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit', timeZone: getTZ() });
}

function esc(s) {
  if (!s) return '';
  const d = document.createElement('div');
  d.textContent = String(s);
  return d.innerHTML;
}

function refresh() {
  loadHealth();
  const active = document.querySelector('section.active');
  if (active && active.id === 'dashboard-tab') loadDashboard();
  if (active && active.id === 'registrations-tab') loadRegistrations();
  if (active && active.id === 'templates-tab') loadTemplates();
  if (active && active.id === 'connections-tab') loadConnections();
  if (active && active.id === 'agents-tab') loadAgents();
  if (active && active.id === 'logs-tab') loadLogs();
  if (active && active.id === 'crons-tab') loadCrons();
  if (active && active.id === 'events-tab') loadEventsTab();
  if (active && active.id === 'humans-tab') loadHumans();
  if (active && active.id === 'roles-tab') loadRoles();
}
