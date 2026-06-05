// --- Attachments / uploads ---

async function loadUploads() {
  try {
    const ups = await api('GET', '/uploads');
    renderUploads(ups);
  } catch(e) {
    if (e.message !== 'forbidden')
      document.getElementById('uploads-list').innerHTML = `<p class="error">${esc(e.message)}</p>`;
  }
}

function fmtUploadSize(n) {
  if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MB';
  if (n >= 1024) return (n / 1024).toFixed(1) + ' KB';
  return n + ' B';
}

function fmtUploadDate(v) {
  if (!v) return '<span class="muted">—</span>';
  const d = new Date(v);
  if (isNaN(d)) return '<span class="muted">—</span>';
  return esc(d.toLocaleString());
}

function renderUploads(ups) {
  const el = document.getElementById('uploads-list');
  if (!ups || !ups.length) {
    el.innerHTML = '<p class="empty">No attachments yet. Upload a file to reference it in an agent message.</p>';
    return;
  }
  el.innerHTML = `
    <table class="data-table">
      <thead><tr><th>Name</th><th>Type</th><th>Size</th><th>Reference</th><th>Uploaded</th><th>Actions</th></tr></thead>
      <tbody>
        ${ups.map(u => `
          <tr>
            <td>${u.name ? esc(u.name) : '<span class="muted">unnamed</span>'}</td>
            <td><span class="muted">${esc(u.mimeType || '')}</span></td>
            <td>${fmtUploadSize(u.size || 0)}</td>
            <td>
              <code style="font-size:11px">${esc(u.uri)}</code>
              <button class="btn btn-sm" onclick="copyUploadUri('${esc(u.uri)}')">Copy</button>
            </td>
            <td>${fmtUploadDate(u.createdAt)}</td>
            <td><button class="btn btn-sm btn-danger" onclick="deleteUpload('${esc(u.id)}', '${esc(u.name || 'unnamed')}')">Delete</button></td>
          </tr>
        `).join('')}
      </tbody>
    </table>`;
}

function copyUploadUri(uri) {
  navigator.clipboard?.writeText(uri).catch(() => {});
}

async function deleteUpload(id, name) {
  if (!confirm(`Delete attachment "${name}"? Any message referencing it will no longer resolve.`)) return;
  try {
    await api('DELETE', `/uploads/${id}`);
    loadUploads();
  } catch(e) {
    alert(e.message);
  }
}
