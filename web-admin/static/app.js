// Tailscale Custom - Web Admin Frontend

let authToken = '';
let currentUser = { username: '', role: '' };
let nodesData = [];
let usersData = [];
let accountsData = [];
let refreshTimer = null;

// ==================== Auth ====================

async function doLogin() {
  const username = document.getElementById('login-username').value.trim();
  const password = document.getElementById('login-password').value;
  const errEl = document.getElementById('login-error');
  errEl.style.display = 'none';

  if (!username || !password) {
    errEl.textContent = 'Please enter username and password';
    errEl.style.display = 'block';
    return;
  }

  try {
    const resp = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password })
    });
    const data = await resp.json();
    if (!resp.ok || !data.ok) {
      errEl.textContent = data.error || 'Login failed';
      errEl.style.display = 'block';
      return;
    }
    authToken = data.token;
    currentUser = { username: data.username, role: data.role };
    sessionStorage.setItem('authToken', authToken);
    sessionStorage.setItem('authUser', JSON.stringify(currentUser));
    enterApp();
  } catch (e) {
    errEl.textContent = 'Connection error';
    errEl.style.display = 'block';
  }
}

function doLogout() {
  api('/api/auth/logout', 'POST').catch(() => {});
  authToken = '';
  currentUser = { username: '', role: '' };
  sessionStorage.removeItem('authToken');
  sessionStorage.removeItem('authUser');
  if (refreshTimer) clearInterval(refreshTimer);
  document.getElementById('main-app').style.display = 'none';
  document.getElementById('login-screen').style.display = 'flex';
  document.getElementById('login-password').value = '';
}

function enterApp() {
  document.getElementById('login-screen').style.display = 'none';
  document.getElementById('main-app').style.display = 'flex';

  // Update user display
  document.getElementById('user-display').textContent = currentUser.username;
  document.getElementById('user-avatar').textContent = currentUser.username.charAt(0).toUpperCase();
  const badge = document.getElementById('user-role-badge');
  badge.textContent = currentUser.role;
  badge.className = 'role-badge role-' + currentUser.role;

  // Show/hide tabs based on role
  const isAdmin = currentUser.role === 'admin';
  document.querySelectorAll('.admin-only').forEach(el => el.style.display = isAdmin ? '' : 'none');
  document.querySelectorAll('.user-only').forEach(el => el.style.display = isAdmin ? 'none' : '');

  // Activate first visible tab
  const firstTab = document.querySelector('.nav-item[style=""], .nav-item:not([style])');
  if (firstTab) switchTab(firstTab.dataset.tab);

  // Auto-refresh
  refreshAll();
  if (refreshTimer) clearInterval(refreshTimer);
  refreshTimer = setInterval(refreshAll, 30000);
}

// ==================== API helper ====================

async function api(url, method = 'GET', body = null) {
  const opts = {
    method,
    headers: { 'Authorization': 'Bearer ' + authToken, 'Content-Type': 'application/json' }
  };
  if (body) opts.body = JSON.stringify(body);
  const resp = await fetch(url, opts);
  if (resp.status === 401) {
    doLogout();
    throw new Error('Session expired');
  }
  return resp;
}

// ==================== Tab switching ====================

document.addEventListener('click', (e) => {
  const item = e.target.closest('.nav-item');
  if (item) switchTab(item.dataset.tab);
});

function switchTab(tab) {
  document.querySelectorAll('.nav-item').forEach(el => el.classList.toggle('active', el.dataset.tab === tab));
  document.querySelectorAll('.tab-content').forEach(el => el.style.display = el.id === 'tab-' + tab ? '' : 'none');

  // Refresh data for this tab
  switch (tab) {
    case 'dashboard': refreshDashboard(); break;
    case 'nodes': refreshNodes(); break;
    case 'accounts': refreshAccounts(); break;
    case 'users': refreshUsers(); break;
    case 'keys': refreshKeys(); break;
    case 'my-nodes': refreshMyNodes(); break;
    case 'download': refreshDownloads(); break;
  }
}

// ==================== Refresh all ====================

function refreshAll() {
  if (currentUser.role === 'admin') {
    refreshDashboard();
    refreshNodes();
  } else {
    refreshMyNodes();
  }
}

// ==================== Dashboard (admin) ====================

async function refreshDashboard() {
  try {
    const [nodesResp, usersResp] = await Promise.all([
      api('/api/admin/nodes'),
      api('/api/admin/users')
    ]);
    const nodesJson = await nodesResp.json();
    const usersJson = await usersResp.json();
    const nodes = nodesJson.nodes || [];
    const users = usersJson.users || [];

    const online = nodes.filter(n => isOnline(n)).length;
    const total = nodes.length;

    document.getElementById('stats-grid').innerHTML = `
      <div class="stat-card">
        <div class="stat-value">${total}</div>
        <div class="stat-label">Total Nodes</div>
      </div>
      <div class="stat-card stat-online">
        <div class="stat-value">${online}</div>
        <div class="stat-label">Online</div>
      </div>
      <div class="stat-card stat-offline">
        <div class="stat-value">${total - online}</div>
        <div class="stat-label">Offline</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">${users.length}</div>
        <div class="stat-label">Users</div>
      </div>
    `;
  } catch (e) { console.error('dashboard', e); }
}

// ==================== Nodes (admin) ====================

async function refreshNodes() {
  try {
    const resp = await api('/api/admin/nodes');
    const data = await resp.json();
    nodesData = data.nodes || [];
    renderNodes();
  } catch (e) { console.error('nodes', e); }
}

function renderNodes() {
  const tbody = document.querySelector('#nodes-table tbody');
  if (!nodesData.length) {
    tbody.innerHTML = '<tr><td colspan="6" class="empty">No nodes found</td></tr>';
    return;
  }
  tbody.innerHTML = nodesData.map(n => {
    const on = isOnline(n);
    const ip = (n.ipAddresses || [])[0] || '-';
    const user = n.user?.name || '-';
    const lastSeen = n.lastSeen ? timeAgo(n.lastSeen) : 'never';
    const name = n.givenName || n.name || '-';
    return `<tr>
      <td><strong>${esc(name)}</strong></td>
      <td><code>${esc(ip)}</code></td>
      <td>${esc(user)}</td>
      <td><span class="badge ${on ? 'badge-online' : 'badge-offline'}">${on ? 'Online' : 'Offline'}</span></td>
      <td>${lastSeen}</td>
      <td>
        <button class="btn btn-xs btn-danger" onclick="deleteNode('${n.id}', '${esc(name)}')">Delete</button>
      </td>
    </tr>`;
  }).join('');
}

async function deleteNode(id, name) {
  if (!confirm(`Delete node "${name}"?`)) return;
  try {
    await api('/api/admin/nodes/' + id, 'DELETE');
    toast('Node deleted');
    refreshNodes();
    refreshDashboard();
  } catch (e) { toast('Error: ' + e.message, true); }
}

// ==================== Accounts (admin) ====================

async function refreshAccounts() {
  try {
    const resp = await api('/api/admin/accounts');
    const data = await resp.json();
    accountsData = data.accounts || [];
    renderAccounts();
  } catch (e) { console.error('accounts', e); }
}

function renderAccounts() {
  const tbody = document.querySelector('#accounts-table tbody');
  if (!accountsData.length) {
    tbody.innerHTML = '<tr><td colspan="4" class="empty">No accounts</td></tr>';
    return;
  }
  tbody.innerHTML = accountsData.map(a => {
    const created = a.createdAt ? new Date(a.createdAt).toLocaleString() : '-';
    const isAdmin = a.username === 'admin';
    return `<tr>
      <td><strong>${esc(a.username)}</strong></td>
      <td><span class="role-badge role-${a.role}">${a.role}</span></td>
      <td>${created}</td>
      <td>
        <button class="btn btn-xs btn-outline" onclick="showResetPasswordModal('${esc(a.username)}')">Reset Password</button>
        ${isAdmin ? '' : `<button class="btn btn-xs btn-danger" onclick="deleteAccount('${esc(a.username)}')">Delete</button>`}
      </td>
    </tr>`;
  }).join('');
}

async function deleteAccount(username) {
  if (!confirm(`Delete account "${username}"? This will also delete the Headscale user.`)) return;
  try {
    const resp = await api('/api/admin/accounts/' + username, 'DELETE');
    if (!resp.ok) { const d = await resp.json(); toast(d.error || 'Failed', true); return; }
    toast('Account deleted');
    refreshAccounts();
    refreshUsers();
  } catch (e) { toast('Error: ' + e.message, true); }
}

function showCreateAccountModal() {
  openModal('Create Account', `
    <div class="form-group">
      <label>Username</label>
      <input type="text" id="new-acct-username" placeholder="username">
    </div>
    <div class="form-group">
      <label>Password</label>
      <input type="password" id="new-acct-password" placeholder="min 4 characters">
    </div>
    <div class="form-group">
      <label>Role</label>
      <select id="new-acct-role">
        <option value="user">User</option>
        <option value="admin">Admin</option>
      </select>
    </div>
    <button class="btn btn-primary btn-block" onclick="createAccount()">Create</button>
  `);
}

async function createAccount() {
  const username = document.getElementById('new-acct-username').value.trim();
  const password = document.getElementById('new-acct-password').value;
  const role = document.getElementById('new-acct-role').value;
  if (!username || !password) { toast('Fill all fields', true); return; }
  try {
    const resp = await api('/api/admin/accounts', 'POST', { username, password, role });
    const data = await resp.json();
    if (!resp.ok) { toast(data.error || 'Failed', true); return; }
    toast('Account created');
    closeModal();
    refreshAccounts();
  } catch (e) { toast('Error: ' + e.message, true); }
}

function showResetPasswordModal(username) {
  openModal('Reset Password: ' + username, `
    <div class="form-group">
      <label>New Password</label>
      <input type="password" id="reset-password" placeholder="min 4 characters">
    </div>
    <button class="btn btn-primary btn-block" onclick="resetPassword('${esc(username)}')">Reset</button>
  `);
}

async function resetPassword(username) {
  const password = document.getElementById('reset-password').value;
  if (!password) { toast('Enter password', true); return; }
  try {
    const resp = await api('/api/admin/accounts/' + username + '/password', 'POST', { password });
    const data = await resp.json();
    if (!resp.ok) { toast(data.error || 'Failed', true); return; }
    toast('Password reset');
    closeModal();
  } catch (e) { toast('Error: ' + e.message, true); }
}

// ==================== Headscale Users (admin) ====================

async function refreshUsers() {
  try {
    const resp = await api('/api/admin/users');
    const data = await resp.json();
    usersData = data.users || [];
    renderUsers();
  } catch (e) { console.error('users', e); }
}

function renderUsers() {
  const tbody = document.querySelector('#users-table tbody');
  if (!usersData.length) {
    tbody.innerHTML = '<tr><td colspan="3" class="empty">No users</td></tr>';
    return;
  }
  tbody.innerHTML = usersData.map(u => {
    const created = u.createdAt ? new Date(u.createdAt).toLocaleString() : '-';
    return `<tr>
      <td><strong>${esc(u.name)}</strong></td>
      <td>${created}</td>
      <td>
        <button class="btn btn-xs btn-outline" onclick="showResetUserPasswordModal('${esc(u.name)}')">Reset Password</button>
        <button class="btn btn-xs btn-danger" onclick="deleteUser('${esc(u.name)}')">Delete</button>
      </td>
    </tr>`;
  }).join('');
}

function showCreateUserModal() {
  openModal('Create Headscale User', `
    <div class="form-group">
      <label>Username</label>
      <input type="text" id="new-hs-user" placeholder="username">
    </div>
    <div class="form-group">
      <label>Password (for login account)</label>
      <input type="password" id="new-hs-password" placeholder="min 4 characters">
    </div>
    <p class="help-text">A login account will also be created so this user can access the admin panel.</p>
    <button class="btn btn-primary btn-block" onclick="createUser()">Create</button>
  `);
}

async function createUser() {
  const name = document.getElementById('new-hs-user').value.trim();
  const password = document.getElementById('new-hs-password').value;
  if (!name) { toast('Enter username', true); return; }
  if (!password || password.length < 4) { toast('Password required (min 4 characters)', true); return; }
  try {
    const resp = await api('/api/admin/users', 'POST', { name, password });
    if (!resp.ok) { const d = await resp.json(); toast(d.error || 'Failed', true); return; }
    toast('User created');
    closeModal();
    refreshUsers();
    refreshAccounts();
  } catch (e) { toast('Error: ' + e.message, true); }
}

async function deleteUser(name) {
  if (!confirm(`Delete Headscale user "${name}"? This will also delete the login account.`)) return;
  try {
    const resp = await api('/api/admin/users/' + name, 'DELETE');
    if (!resp.ok) { const d = await resp.json(); toast(d.error || 'Failed', true); return; }
    toast('User deleted');
    refreshUsers();
    refreshAccounts();
  } catch (e) { toast('Error: ' + e.message, true); }
}

function showResetUserPasswordModal(username) {
  openModal('Reset Password: ' + username, `
    <div class="form-group">
      <label>New Password</label>
      <input type="password" id="reset-user-password" placeholder="min 4 characters">
    </div>
    <button class="btn btn-primary btn-block" onclick="resetUserPassword('${esc(username)}')">Reset</button>
  `);
}

async function resetUserPassword(username) {
  const password = document.getElementById('reset-user-password').value;
  if (!password || password.length < 4) { toast('Password required (min 4 characters)', true); return; }
  try {
    const resp = await api('/api/admin/users/' + username + '/password', 'POST', { password });
    const data = await resp.json();
    if (!resp.ok) { toast(data.error || 'Failed', true); return; }
    toast('Password reset');
    closeModal();
  } catch (e) { toast('Error: ' + e.message, true); }
}

// ==================== Keys (admin) ====================

async function refreshKeys() {
  try {
    // Get keys for all users
    const usersResp = await api('/api/admin/users');
    const usersJson = await usersResp.json();
    const users = usersJson.users || [];

    let allKeys = [];
    for (const u of users) {
      try {
        const keysResp = await api('/api/admin/preauthkeys?user=' + u.name);
        const keysJson = await keysResp.json();
        const keys = keysJson.preAuthKeys || [];
        keys.forEach(k => k._user = u.name);
        allKeys = allKeys.concat(keys);
      } catch (e) { /* skip */ }
    }
    renderKeys(allKeys);
  } catch (e) { console.error('keys', e); }
}

function renderKeys(keys) {
  const tbody = document.querySelector('#keys-table tbody');
  if (!keys.length) {
    tbody.innerHTML = '<tr><td colspan="6" class="empty">No keys</td></tr>';
    return;
  }
  tbody.innerHTML = keys.map(k => {
    const exp = k.expiration ? new Date(k.expiration).toLocaleString() : '-';
    return `<tr>
      <td><code class="key-text">${esc((k.key || '').substring(0, 12))}...</code></td>
      <td>${esc(k._user || k.user || '-')}</td>
      <td>${k.reusable ? '&#x2705;' : '&#x274C;'}</td>
      <td>${k.ephemeral ? '&#x2705;' : '&#x274C;'}</td>
      <td>${k.used ? '&#x2705;' : '&#x274C;'}</td>
      <td>${exp}</td>
    </tr>`;
  }).join('');
}

function showCreateKeyModal() {
  // Need users list for dropdown
  const usersOpts = usersData.map(u => `<option value="${esc(u.name)}">${esc(u.name)}</option>`).join('');
  openModal('Create Pre-Auth Key', `
    <div class="form-group">
      <label>User</label>
      <select id="new-key-user">${usersOpts}</select>
    </div>
    <div class="form-group">
      <label><input type="checkbox" id="new-key-reusable"> Reusable</label>
    </div>
    <div class="form-group">
      <label><input type="checkbox" id="new-key-ephemeral"> Ephemeral</label>
    </div>
    <div class="form-group">
      <label>Expiration (hours)</label>
      <input type="number" id="new-key-expiry" value="24" min="1">
    </div>
    <button class="btn btn-primary btn-block" onclick="createKey()">Create</button>
  `);
}

async function createKey() {
  const user = document.getElementById('new-key-user').value;
  const reusable = document.getElementById('new-key-reusable').checked;
  const ephemeral = document.getElementById('new-key-ephemeral').checked;
  const hours = parseInt(document.getElementById('new-key-expiry').value) || 24;
  const expiration = new Date(Date.now() + hours * 3600000).toISOString();

  try {
    const resp = await api('/api/admin/preauthkeys', 'POST', { user, reusable, ephemeral, expiration });
    if (!resp.ok) { const d = await resp.json(); toast(d.error || 'Failed', true); return; }
    toast('Key created');
    closeModal();
    refreshKeys();
  } catch (e) { toast('Error: ' + e.message, true); }
}

// ==================== Register Node (admin) ====================

function showRegisterModal() {
  const usersOpts = usersData.length
    ? usersData.map(u => `<option value="${esc(u.name)}">${esc(u.name)}</option>`).join('')
    : '<option value="">No users</option>';
  openModal('Register Node', `
    <div class="form-group">
      <label>User</label>
      <select id="reg-user">${usersOpts}</select>
    </div>
    <div class="form-group">
      <label>Registration Key</label>
      <input type="text" id="reg-key" placeholder="Paste the nodekey:... or mkey:... from client">
    </div>
    <p class="help-text">The registration key is shown when the client tries to connect.</p>
    <button class="btn btn-primary btn-block" onclick="registerNode()">Register</button>
  `);
}

async function registerNode() {
  const user = document.getElementById('reg-user').value;
  let key = document.getElementById('reg-key').value.trim();
  if (!user || !key) { toast('Fill all fields', true); return; }
  key = extractKey(key);
  try {
    const resp = await api('/api/admin/register', 'POST', { user, key });
    const data = await resp.json();
    if (!resp.ok) { toast(data.message || data.error || 'Registration failed', true); return; }
    toast('Node registered!');
    closeModal();
    refreshNodes();
    refreshDashboard();
  } catch (e) { toast('Error: ' + e.message, true); }
}

function extractKey(input) {
  // Extract nodekey from URL or text
  const match = input.match(/key=([a-f0-9]+)/i);
  if (match) return match[1];
  // Remove nodekey: or mkey: prefix
  return input.replace(/^(nodekey:|mkey:)/, '');
}

// ==================== My Nodes (user) ====================

async function refreshMyNodes() {
  try {
    const resp = await api('/api/user/nodes');
    const data = await resp.json();
    const nodes = data.nodes || [];
    renderMyNodes(nodes);
  } catch (e) { console.error('my-nodes', e); }
}

function renderMyNodes(nodes) {
  const tbody = document.querySelector('#my-nodes-table tbody');
  if (!nodes.length) {
    tbody.innerHTML = '<tr><td colspan="4" class="empty">No nodes registered. Click "+ Register Node" to add one.</td></tr>';
    return;
  }
  tbody.innerHTML = nodes.map(n => {
    const on = isOnline(n);
    const ip = (n.ipAddresses || [])[0] || '-';
    const lastSeen = n.lastSeen ? timeAgo(n.lastSeen) : 'never';
    const name = n.givenName || n.name || '-';
    return `<tr>
      <td><strong>${esc(name)}</strong></td>
      <td><code>${esc(ip)}</code></td>
      <td><span class="badge ${on ? 'badge-online' : 'badge-offline'}">${on ? 'Online' : 'Offline'}</span></td>
      <td>${lastSeen}</td>
    </tr>`;
  }).join('');
}

function showUserRegisterModal() {
  openModal('Register My Node', `
    <div class="form-group">
      <label>Registration Key</label>
      <input type="text" id="user-reg-key" placeholder="Paste the nodekey:... or mkey:... from client">
    </div>
    <p class="help-text">Open Tailscale Custom client &#x2192; it will show a registration URL. Copy the key from there and paste it here.</p>
    <button class="btn btn-primary btn-block" onclick="userRegisterNode()">Register</button>
  `);
}

async function userRegisterNode() {
  let key = document.getElementById('user-reg-key').value.trim();
  if (!key) { toast('Enter registration key', true); return; }
  key = extractKey(key);
  try {
    const resp = await api('/api/user/register', 'POST', { key });
    const data = await resp.json();
    if (!resp.ok) { toast(data.message || data.error || 'Registration failed', true); return; }
    toast('Node registered!');
    closeModal();
    refreshMyNodes();
  } catch (e) { toast('Error: ' + e.message, true); }
}

// ==================== Downloads ====================

async function refreshDownloads() {
  try {
    const resp = await fetch('/download/');
    const data = await resp.json();
    const files = data.files || [];
    const container = document.getElementById('downloads-list');

    if (!files.length) {
      container.innerHTML = '<div class="empty-state"><p>No downloads available yet.</p></div>';
      return;
    }

    container.innerHTML = files.map(f => {
      const sizeStr = formatSize(f.size);
      const icon = f.name.endsWith('.msi') ? '&#x1F4BF;' : f.name.endsWith('.exe') ? '&#x2699;' : '&#x1F4C4;';
      return `<div class="download-card">
        <div class="download-icon">${icon}</div>
        <div class="download-info">
          <div class="download-name">${esc(f.name)}</div>
          <div class="download-size">${sizeStr}</div>
        </div>
        <a href="/download/${encodeURIComponent(f.name)}" class="btn btn-primary btn-sm" download>Download</a>
      </div>`;
    }).join('');
  } catch (e) { console.error('downloads', e); }
}

// ==================== Change Password ====================

function showPasswordModal() {
  openModal('Change Password', `
    <div class="form-group">
      <label>Current Password</label>
      <input type="password" id="chg-old-pw">
    </div>
    <div class="form-group">
      <label>New Password</label>
      <input type="password" id="chg-new-pw" placeholder="min 4 characters">
    </div>
    <button class="btn btn-primary btn-block" onclick="changePassword()">Change</button>
  `);
}

async function changePassword() {
  const oldPassword = document.getElementById('chg-old-pw').value;
  const newPassword = document.getElementById('chg-new-pw').value;
  if (!oldPassword || !newPassword) { toast('Fill all fields', true); return; }
  try {
    const resp = await api('/api/auth/password', 'POST', { oldPassword, newPassword });
    const data = await resp.json();
    if (!resp.ok) { toast(data.error || 'Failed', true); return; }
    toast('Password changed');
    closeModal();
  } catch (e) { toast('Error: ' + e.message, true); }
}

// ==================== Modal helpers ====================

function openModal(title, bodyHtml) {
  document.getElementById('modal-title').textContent = title;
  document.getElementById('modal-body').innerHTML = bodyHtml;
  document.getElementById('modal-overlay').style.display = 'flex';
}

function closeModal() {
  document.getElementById('modal-overlay').style.display = 'none';
}

// ==================== Utility ====================

function isOnline(node) {
  if (!node.lastSeen) return false;
  const diff = Date.now() - new Date(node.lastSeen).getTime();
  return node.online === true || diff < 300000;
}

function timeAgo(dateStr) {
  const diff = Date.now() - new Date(dateStr).getTime();
  if (diff < 60000) return 'just now';
  if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
  if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
  return Math.floor(diff / 86400000) + 'd ago';
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
  if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB';
  return (bytes / 1073741824).toFixed(1) + ' GB';
}

function esc(str) {
  const d = document.createElement('div');
  d.textContent = str || '';
  return d.innerHTML;
}

function toast(msg, isError = false) {
  const el = document.getElementById('toast');
  el.textContent = msg;
  el.className = 'toast' + (isError ? ' toast-error' : ' toast-success');
  el.style.display = 'block';
  setTimeout(() => { el.style.display = 'none'; }, 3000);
}

// Enter key for login
document.getElementById('login-password').addEventListener('keyup', (e) => { if (e.key === 'Enter') doLogin(); });
document.getElementById('login-username').addEventListener('keyup', (e) => { if (e.key === 'Enter') document.getElementById('login-password').focus(); });

// Auto-login from session
(function init() {
  const token = sessionStorage.getItem('authToken');
  const user = sessionStorage.getItem('authUser');
  if (token && user) {
    authToken = token;
    currentUser = JSON.parse(user);
    // Verify session is still valid
    fetch('/api/auth/me', { headers: { 'Authorization': 'Bearer ' + token } })
      .then(r => {
        if (r.ok) enterApp();
        else doLogout();
      })
      .catch(() => doLogout());
  }
})();
