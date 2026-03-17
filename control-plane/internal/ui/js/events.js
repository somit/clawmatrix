// --- Events Tab ---

let eventsRange = '1h';

function setEventsRange(range, btn) {
  eventsRange = range;
  document.querySelectorAll('#events-time-range button').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  loadEventsTab();
}

async function loadEventsTab() {
  const typeFilter = document.getElementById('event-type-filter').value;

  try {
    const events = await api('GET', '/audit?since=' + eventsRange);

    const filtered = typeFilter ? events.filter(e => e.type === typeFilter) : events;

    const eventMeta = {
      'agent:registered': { dot: 'var(--green)', label: 'Agent Registered' },
      'agent:stale':      { dot: 'var(--yellow)', label: 'Agent Stale' },
      'agent:recovered':  { dot: 'var(--green)', label: 'Agent Recovered' },
      'agent:killed':     { dot: 'var(--red)', label: 'Agent Killed' },
      'log:batch':        { dot: 'var(--muted)', label: 'Logs Ingested' },
      'registration:created': { dot: 'var(--blue)', label: 'Registration Created' },
      'registration:updated': { dot: 'var(--blue)', label: 'Registration Updated' },
      'connection:created':   { dot: 'var(--green)', label: 'Connection Created' },
      'connection:deleted':   { dot: 'var(--red)', label: 'Connection Deleted' },
      'template:created':     { dot: 'var(--purple)', label: 'Profile Created' },
      'template:updated':     { dot: 'var(--purple)', label: 'Profile Updated' },
    };

    // Stats
    const counts = {};
    for (const e of events) {
      counts[e.type] = (counts[e.type] || 0) + 1;
    }

    let html = `
      <div class="log-stats">
        <div class="log-stat"><div class="label">Total</div><div class="val">${events.length}</div></div>
        <div class="log-stat"><div class="label">Agent Events</div><div class="val green">${(counts['agent:registered']||0)+(counts['agent:recovered']||0)+(counts['agent:stale']||0)+(counts['agent:killed']||0)}</div></div>
        <div class="log-stat"><div class="label">Registration Events</div><div class="val blue">${(counts['registration:created']||0)+(counts['registration:updated']||0)}</div></div>
        <div class="log-stat"><div class="label">Log Batches</div><div class="val">${counts['log:batch']||0}</div></div>
      </div>
    `;

    if (!filtered.length) {
      html += '<div class="empty">No events in this time range</div>';
    } else {
      html += '<div class="box">';
      html += filtered.map(e => {
        const meta = eventMeta[e.type] || { dot: 'var(--muted)', label: e.type };
        const data = e.data || {};
        const detail = data.id || data.name || (data.count ? data.count + ' entries' : '');
        const reason = data.reason ? ' (' + esc(data.reason) + ')' : '';
        const time = formatTime(e.ts);
        const date = new Date(e.ts).toLocaleDateString('en-GB', { day: '2-digit', month: 'short' });
        return `
          <div class="event-row">
            <span class="event-dot" style="background:${meta.dot}"></span>
            <span class="event-row-time">${date} ${time}</span>
            <span class="event-row-type">${esc(meta.label)}</span>
            <span class="event-row-detail">${esc(detail)}${reason}</span>
          </div>
        `;
      }).join('');
      html += '</div>';
    }

    document.getElementById('events-list').innerHTML = html;
  } catch(e) {
    if (e.message !== 'unauthorized')
      document.getElementById('events-list').innerHTML = `<div class="empty error-msg">${esc(e.message)}</div>`;
  }
}
