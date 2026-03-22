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

// Auto-login if token exists
if (token) {
  fetch('/auth/me', { headers: { 'Authorization': 'Bearer ' + token } })
    .then(r => { if (r.ok) return r.json(); throw new Error(); })
    .then(me => { currentUser = me; enterApp(); })
    .catch(() => doLogout());
} else {
  document.getElementById('login-screen').style.display = 'flex';
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
