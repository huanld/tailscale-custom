// Headscale Web Admin - Frontend

let adminToken = '';
let nodesData = [];
let usersData = [];

// --- Auth ---

function doLogin() {
  const pw = document.getElementById('login-password').value;
  fetch('/api/auth', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password: pw })
  })
    .then(r => r.json())
    .then(data => {
      if (data.ok) {
        adminToken = pw;
        sessionStorage.setItem('token', pw);
        document.getElementById('login-screen').style.display = 'none';
        document.getElementById('app').style.display = 'block';
        refreshAll();
      } else {
        document.getElementById('login-error').textContent = 'Wrong password';
      }
    })
    .catch(() => {
      document.getElementById('login-error').textContent = 'Connection error';
    });
}

function doLogout() {
  adminToken = '';
  sessionStorage.removeItem('token');
  document.getElementById('app').style.display = 'none';
  document.getElementById('login-screen').style.display = 'flex';
  document.getElementById('login-password').value = '';
}

// Auto-login from session
(function() {
  const saved = sessionStorage.getItem('token');
  if (saved) {
    adminToken = saved;
    document.getElementById('login-screen').style.display = 'none';
    document.getElementById('app').style.display = 'block';
    refreshAll();
  }
})();

// --- API helpers ---

function api(path, opts = {}) {
  const headers = { 'X-Admin-Token': adminToken, ...opts.headers };
  if (opts.body && typeof opts.body === 'object') {
    headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(opts.body);
  }
  return fetch(path, { ...opts, headers }).then(r => {
    if (r.status === 401) { doLogout(); throw new Error('unauthorized'); }
    return r.json();
  });
}

function toast(msg, type = 'success') {
  const el = document.getElementById('toast');
  el.textContent = msg;
  el.className = 'toast show ' + type;
  setTimeout(() => el.className = 'toast', 3000);
}

// --- Tabs ---

function switchTab(name) {
  document.querySelectorAll('.tab').forEach(t => t.classList.toggle('active', t.dataset.tab === name));
  document.querySelectorAll('.tab-content').forEach(t => t.classList.toggle('active', t.id === 'tab-' + name));
}

// --- Data ---

function refreshAll() {
  refreshNodes();
  refreshUsers();
  refreshKeys();
}

// --- Nodes ---

function refreshNodes() {
  api('/api/nodes').then(data => {
    const nodes = data.nodes || [];
    nodesData = nodes;
    renderNodes(nodes);
    renderDashboard(nodes);
  }).catch(e => toast('Failed to load nodes: ' + e.message, 'error'));
}

function renderNodes(nodes) {
  const tbody = document.getElementById('nodes-table');
  if (!nodes.length) {
    tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:#64748b;padding:40px">No nodes registered</td></tr>';
    return;
  }
  tbody.innerHTML = nodes.map(n => {
    const online = isOnline(n);
    const ips = (n.ipAddresses || []).join(', ');
    const userName = n.user ? (n.user.name || n.user.Name || '-') : '-';
    const name = n.givenName || n.name || '-';
    return `<tr>
      <td>${n.id}</td>
      <td><strong>${esc(name)}</strong></td>
      <td><code style="font-size:12px">${esc(ips)}</code></td>
      <td>${esc(userName)}</td>
      <td>${online ? '<span class="badge badge-online">Online</span>' : '<span class="badge badge-offline">Offline</span>'}</td>
      <td>${timeAgo(n.lastSeen)}</td>
      <td>${fmtDate(n.createdAt)}</td>
      <td>
        <button class="btn btn-sm btn-danger" onclick="deleteNode('${n.id}','${esc(name)}')">Delete</button>
        <button class="btn btn-sm" onclick="expireNode('${n.id}')">Expire</button>
      </td>
    </tr>`;
  }).join('');
}

function renderDashboard(nodes) {
  const online = nodes.filter(isOnline).length;
  document.getElementById('stat-total').textContent = nodes.length;
  document.getElementById('stat-online').textContent = online;
  document.getElementById('stat-offline').textContent = nodes.length - online;

  // Recent 5 nodes
  const recent = [...nodes].sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt)).slice(0, 5);
  const tbody = document.getElementById('dashboard-nodes');
  tbody.innerHTML = recent.map(n => {
    const online = isOnline(n);
    const ips = (n.ipAddresses || []).join(', ');
    const userName = n.user ? (n.user.name || n.user.Name || '-') : '-';
    const name = n.givenName || n.name || '-';
    return `<tr>
      <td><strong>${esc(name)}</strong></td>
      <td><code style="font-size:12px">${esc(ips)}</code></td>
      <td>${esc(userName)}</td>
      <td>${online ? '<span class="badge badge-online">Online</span>' : '<span class="badge badge-offline">Offline</span>'}</td>
      <td>${timeAgo(n.lastSeen)}</td>
    </tr>`;
  }).join('') || '<tr><td colspan="5" style="text-align:center;color:#64748b">No nodes yet</td></tr>';
}

function deleteNode(id, name) {
  if (!confirm(`Delete node "${name}" (ID: ${id})?`)) return;
  api('/api/nodes/' + id, { method: 'DELETE' })
    .then(() => { toast('Node deleted'); refreshNodes(); })
    .catch(e => toast('Delete failed: ' + e.message, 'error'));
}

function expireNode(id) {
  api('/api/nodes/' + id + '/expire', { method: 'POST' })
    .then(() => { toast('Node expired'); refreshNodes(); })
    .catch(e => toast('Expire failed: ' + e.message, 'error'));
}

// --- Users ---

function refreshUsers() {
  api('/api/users').then(data => {
    const users = data.users || [];
    usersData = users;
    renderUsers(users);
    document.getElementById('stat-users').textContent = users.length;
    updateKeyUserSelect(users);
  }).catch(e => toast('Failed to load users: ' + e.message, 'error'));
}

function renderUsers(users) {
  const tbody = document.getElementById('users-table');
  if (!users.length) {
    tbody.innerHTML = '<tr><td colspan="4" style="text-align:center;color:#64748b;padding:40px">No users</td></tr>';
    return;
  }
  tbody.innerHTML = users.map(u => {
    const name = u.name || u.Name || '-';
    return `<tr>
      <td>${u.id}</td>
      <td><strong>${esc(name)}</strong></td>
      <td>${fmtDate(u.createdAt)}</td>
      <td><button class="btn btn-sm btn-danger" onclick="deleteUser('${esc(name)}')">Delete</button></td>
    </tr>`;
  }).join('');
}

function createUser() {
  const input = document.getElementById('new-user-name');
  const name = input.value.trim();
  if (!name) { toast('Enter a username', 'error'); return; }

  api('/api/users', { method: 'POST', body: { name: name } })
    .then(() => { toast('User created: ' + name); input.value = ''; refreshUsers(); })
    .catch(e => toast('Create failed: ' + e.message, 'error'));
}

function deleteUser(name) {
  if (!confirm(`Delete user "${name}"? All nodes of this user will also be removed.`)) return;
  api('/api/users/' + encodeURIComponent(name), { method: 'DELETE' })
    .then(() => { toast('User deleted'); refreshUsers(); refreshNodes(); })
    .catch(e => toast('Delete failed: ' + e.message, 'error'));
}

// --- Pre-auth Keys ---

function updateKeyUserSelect(users) {
  const sel = document.getElementById('key-user-select');
  sel.innerHTML = users.map(u => {
    const name = u.name || u.Name;
    return `<option value="${esc(name)}">${esc(name)}</option>`;
  }).join('');
}

function refreshKeys() {
  // Need to fetch keys per user
  if (!usersData.length) {
    api('/api/users').then(data => {
      usersData = data.users || [];
      fetchAllKeys();
    });
  } else {
    fetchAllKeys();
  }
}

function fetchAllKeys() {
  const promises = usersData.map(u => {
    const name = u.name || u.Name;
    return api('/api/preauthkeys?user=' + encodeURIComponent(name))
      .then(data => (data.preAuthKeys || []).map(k => ({ ...k, _user: name })))
      .catch(() => []);
  });

  Promise.all(promises).then(results => {
    const allKeys = results.flat().sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt));
    renderKeys(allKeys);
  });
}

function renderKeys(keys) {
  const tbody = document.getElementById('keys-table');
  if (!keys.length) {
    tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:#64748b;padding:40px">No pre-auth keys</td></tr>';
    return;
  }
  tbody.innerHTML = keys.map(k => {
    const expired = k.expiration && new Date(k.expiration) < new Date();
    const keyShort = k.key ? k.key.substring(0, 16) + '...' : '-';
    return `<tr>
      <td><span class="key-text" title="Click to copy full key" onclick="copyKey('${esc(k.key)}')">${esc(keyShort)}</span></td>
      <td>${esc(k._user || k.user || '-')}</td>
      <td>${k.reusable ? '✅' : '—'}</td>
      <td>${k.ephemeral ? '✅' : '—'}</td>
      <td>${k.used ? '✅' : '—'}</td>
      <td>${expired ? '<span class="badge badge-expired">Expired</span>' : fmtDate(k.expiration)}</td>
      <td>${fmtDate(k.createdAt)}</td>
    </tr>`;
  }).join('');
}

function createKey() {
  const user = document.getElementById('key-user-select').value;
  if (!user) { toast('Select a user first', 'error'); return; }

  const reusable = document.getElementById('key-reusable').checked;
  const ephemeral = document.getElementById('key-ephemeral').checked;
  const hours = parseInt(document.getElementById('key-expiry').value) || 24;

  const expiration = new Date(Date.now() + hours * 3600000).toISOString();

  api('/api/preauthkeys', {
    method: 'POST',
    body: { user: user, reusable, ephemeral, expiration }
  }).then(data => {
    const key = data.preAuthKey ? data.preAuthKey.key : 'created';
    toast('Key created: ' + key.substring(0, 20) + '...');
    refreshKeys();
  }).catch(e => toast('Create key failed: ' + e.message, 'error'));
}

function copyKey(key) {
  navigator.clipboard.writeText(key).then(
    () => toast('Key copied to clipboard'),
    () => toast('Copy failed', 'error')
  );
}

// --- Helpers ---

function isOnline(node) {
  return node.online === true;
}

function timeAgo(dateStr) {
  if (!dateStr) return '-';
  const diff = Date.now() - new Date(dateStr).getTime();
  if (diff < 0) return 'just now';
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return mins + 'm ago';
  const hours = Math.floor(mins / 60);
  if (hours < 24) return hours + 'h ago';
  const days = Math.floor(hours / 24);
  return days + 'd ago';
}

function fmtDate(dateStr) {
  if (!dateStr) return '-';
  const d = new Date(dateStr);
  return d.toLocaleDateString('vi-VN') + ' ' + d.toLocaleTimeString('vi-VN', { hour: '2-digit', minute: '2-digit' });
}

function esc(s) {
  if (!s) return '';
  const div = document.createElement('div');
  div.textContent = String(s);
  return div.innerHTML;
}

// Auto-refresh every 30s
setInterval(() => {
  if (adminToken) refreshNodes();
}, 30000);
