// API Base URL
const API_BASE = '/api';

// Auth
function getToken() {
    return localStorage.getItem('bootimus_token');
}

function setToken(token) {
    localStorage.setItem('bootimus_token', token);
}

function clearToken() {
    localStorage.removeItem('bootimus_token');
    localStorage.removeItem('bootimus_username');
    localStorage.removeItem('bootimus_is_admin');
}

async function authFetch(url, options = {}) {
    const token = getToken();
    if (token) {
        options.headers = options.headers || {};
        options.headers['Authorization'] = 'Bearer ' + token;
    }
    const res = await fetch(url, options);
    if (res.status === 401) {
        clearToken();
        showLoginScreen();
        throw new Error('Authentication required');
    }
    return res;
}

async function showLoginScreen() {
    document.getElementById('login-screen').style.display = 'flex';
    document.getElementById('main-header').style.display = 'none';
    document.getElementById('main-app').style.display = 'none';
    document.getElementById('login-error').style.display = 'none';

    // Load available auth backends
    try {
        const res = await fetch(`${API_BASE}/auth-info`);
        const data = await res.json();
        if (data.success && data.data && data.data.length > 1) {
            const select = document.getElementById('login-auth-method');
            select.innerHTML = data.data.map(b =>
                `<option value="${b.id}">${b.name}</option>`
            ).join('');
            document.getElementById('login-auth-selector').style.display = 'block';
        } else {
            document.getElementById('login-auth-selector').style.display = 'none';
        }
    } catch (e) {
        document.getElementById('login-auth-selector').style.display = 'none';
    }

    // Only auto-focus the username field if the user isn't already
    // interacting with the login form. Background polling can hit 401s and
    // re-run showLoginScreen mid-typing; without this guard, focus would
    // get stolen away from the password field on every poll.
    const active = document.activeElement;
    if (!active || !active.closest || !active.closest('#login-form')) {
        document.getElementById('login-username').focus();
    }
}

function showApp() {
    document.getElementById('login-screen').style.display = 'none';
    document.getElementById('main-header').style.display = '';
    document.getElementById('main-app').style.display = '';
}

async function handleLogin(e) {
    e.preventDefault();
    const username = document.getElementById('login-username').value;
    const password = document.getElementById('login-password').value;
    const authMethod = document.getElementById('login-auth-method').value || 'local';
    const errorDiv = document.getElementById('login-error');

    try {
        const res = await fetch(`${API_BASE}/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password, auth_method: authMethod })
        });
        const data = await res.json();

        if (data.success) {
            setToken(data.data.token);
            localStorage.setItem('bootimus_username', data.data.username);
            localStorage.setItem('bootimus_is_admin', data.data.is_admin);
            showApp();
            initApp();
        } else {
            errorDiv.textContent = data.error || 'Login failed';
            errorDiv.style.display = 'block';
        }
    } catch (err) {
        errorDiv.textContent = 'Connection error';
        errorDiv.style.display = 'block';
    }
}

function logout() {
    clearToken();
    showLoginScreen();
    document.getElementById('login-form').reset();
}

async function checkAuth() {
    const token = getToken();
    if (!token) {
        showLoginScreen();
        return;
    }

    try {
        const res = await authFetch(`${API_BASE}/stats`);
        if (res.ok) {
            showApp();
            initApp();
        } else {
            showLoginScreen();
        }
    } catch {
        showLoginScreen();
    }
}

// State
let clients = [];
let images = [];
let currentClient = null;
let imageSortColumn = 'name';
let imageSortDirection = 'asc';
let imageGroupedView = (() => {
    try { return localStorage.getItem('image-grouped-view') === '1'; } catch (_) { return false; }
})();
const collapsedImageGroups = (() => {
    try {
        const raw = localStorage.getItem('image-collapsed-groups');
        return new Set(raw ? JSON.parse(raw) : []);
    } catch (_) { return new Set(); }
})();
let extractionProgress = {}; // Track extraction progress by filename

// Theme
function toggleTheme() {
    const html = document.documentElement;
    const next = html.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    html.setAttribute('data-theme', next);
    localStorage.setItem('bootimus_theme', next);
    const btn = document.getElementById('theme-toggle-btn');
    if (btn) btn.setAttribute('aria-checked', next === 'dark' ? 'true' : 'false');
}

function loadSavedTheme() {
    const saved = localStorage.getItem('bootimus_theme') || 'light';
    document.documentElement.setAttribute('data-theme', saved);
    const btn = document.getElementById('theme-toggle-btn');
    if (btn) btn.setAttribute('aria-checked', saved === 'dark' ? 'true' : 'false');
}

// Utility Functions
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function openModal(modalId) {
    document.getElementById(modalId).classList.add('active');
}

function closeModal(modalId) {
    document.getElementById(modalId).classList.remove('active');
}

function injectModalCloseButtons() {
    const svg = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>';
    document.querySelectorAll('.modal > .modal-content > .modal-header').forEach(header => {
        if (header.querySelector('.modal-close')) return;
        const modal = header.closest('.modal');
        if (!modal || !modal.id) return;
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn btn-icon modal-close';
        btn.setAttribute('aria-label', 'Close');
        btn.title = 'Close';
        btn.innerHTML = svg;
        btn.addEventListener('click', () => closeModal(modal.id));
        header.appendChild(btn);
    });
    // Backdrop click closes the modal. e.target === modal means the click
    // was on the dimmed area outside .modal-content, not bubbled up from
    // inside the panel.
    document.querySelectorAll('.modal').forEach(modal => {
        if (!modal.id || modal.dataset.backdropWired) return;
        modal.dataset.backdropWired = '1';
        modal.addEventListener('click', e => {
            if (e.target === modal) closeModal(modal.id);
        });
    });
}
document.addEventListener('DOMContentLoaded', injectModalCloseButtons);

function toggleUserProfile() {
    const dropdown = document.getElementById('user-profile-dropdown');
    dropdown.classList.toggle('show');
}

// Close dropdown when clicking outside
document.addEventListener('click', (e) => {
    const dropdown = document.getElementById('user-profile-dropdown');
    const button = document.querySelector('.user-profile-button');

    if (dropdown && button && !dropdown.contains(e.target) && !button.contains(e.target)) {
        dropdown.classList.remove('show');
    }
});

function loadCurrentUser() {
    const username = localStorage.getItem('bootimus_username') || 'admin';
    const isAdmin = localStorage.getItem('bootimus_is_admin') === 'true';
    document.getElementById('current-username').textContent = username;
    document.getElementById('current-user-role').textContent = isAdmin ? 'Administrator' : 'User';
}

function showNotification(message, type = 'info') {
    // Create notification element
    const notification = document.createElement('div');
    notification.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 15px 20px;
        border-radius: 8px;
        color: white;
        font-weight: 500;
        z-index: 10000;
        max-width: 400px;
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
        animation: slideIn 0.3s ease-out;
    `;

    // Set background color based on type
    if (type === 'success') {
        notification.style.background = 'linear-gradient(135deg, #10b981, #059669)';
    } else if (type === 'error') {
        notification.style.background = 'linear-gradient(135deg, #ef4444, #dc2626)';
    } else {
        notification.style.background = 'linear-gradient(135deg, #3b82f6, #2563eb)';
    }

    notification.textContent = message;

    // Add animation styles if not already present
    if (!document.getElementById('notification-styles')) {
        const style = document.createElement('style');
        style.id = 'notification-styles';
        style.textContent = `
            @keyframes slideIn {
                from {
                    transform: translateX(400px);
                    opacity: 0;
                }
                to {
                    transform: translateX(0);
                    opacity: 1;
                }
            }
            @keyframes slideOut {
                from {
                    transform: translateX(0);
                    opacity: 1;
                }
                to {
                    transform: translateX(400px);
                    opacity: 0;
                }
            }
        `;
        document.head.appendChild(style);
    }

    document.body.appendChild(notification);

    // Auto-remove after 4 seconds
    setTimeout(() => {
        notification.style.animation = 'slideOut 0.3s ease-out';
        setTimeout(() => notification.remove(), 300);
    }, 4000);
}

let appInitialized = false;

function initApp() {
    if (appInitialized) return;
    appInitialized = true;

    loadCurrentUser();
    loadStats();
    loadServerInfo();
    loadProfileCache();
    loadClients();
    loadImages();
    loadPublicFiles();
    loadLogs();
    loadUsers();
    loadActiveSessions();

    // Refresh every 30 seconds
    setInterval(() => {
        loadStats();
        loadActiveSessions();
        const activeTab = (document.querySelector('.nav-item.active') || document.querySelector('.tab.active')).dataset.tab;
        if (activeTab === 'clients') loadClients();
        if (activeTab === 'images') loadImages();
        if (activeTab === 'public-files') loadPublicFiles();
        if (activeTab === 'logs') loadLogs();
        if (activeTab === 'users') loadUsers();
    }, 30000);

    // Refresh server info more frequently for live stats (every 5 seconds)
    setInterval(() => {
        const activeTab = (document.querySelector('.nav-item.active') || document.querySelector('.tab.active')).dataset.tab;
        if (activeTab === 'server') loadServerInfo();
    }, 5000);

    // Refresh active sessions more frequently (every 3 seconds)
    setInterval(loadActiveSessions, 3000);
}

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    loadSavedTheme();
    setupTabs();
    setupForms();
    setupUpload();
    document.getElementById('login-form').addEventListener('submit', handleLogin);
    checkAuth();
});

// Tab Management
function setupTabs() {
    // Setup sidebar nav items
    document.querySelectorAll('.sidebar-nav .nav-item').forEach(item => {
        item.addEventListener('click', () => {
            // Update sidebar active state
            document.querySelectorAll('.sidebar-nav .nav-item').forEach(n => n.classList.remove('active'));
            item.classList.add('active');

            // Update tab content
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
            document.getElementById(`${item.dataset.tab}-tab`).classList.add('active');

            // Also update hidden tabs for compat
            document.querySelectorAll('.tabs .tab').forEach(t => t.classList.remove('active'));
            const matchingTab = document.querySelector(`.tabs .tab[data-tab="${item.dataset.tab}"]`);
            if (matchingTab) matchingTab.classList.add('active');

            if (item.dataset.tab === 'images') loadGroups();
            if (item.dataset.tab === 'clients') loadClientGroups();
            if (item.dataset.tab === 'tools') loadTools();
            if (item.dataset.tab === 'bootloaders') loadBootloaders();
            if (item.dataset.tab === 'profiles') loadProfiles();
            if (item.dataset.tab === 'autoinstall') loadAutoInstallFiles();
            if (item.dataset.tab === 'settings') { loadTheme(); loadUSBImages(); loadWebhookConfig(); }
        });
    });

    // Keep old tab click handlers for modal tabs
    document.querySelectorAll('#image-properties-modal .tabs .tab').forEach(tab => {
        // These are handled by switchPropsTab, no action needed
    });
}

// Stats
async function loadStats() {
    try {
        const res = await authFetch(`${API_BASE}/stats`);
        const data = await res.json();

        if (data.success) {
            document.getElementById('stat-clients').textContent = data.data.total_clients;
            document.getElementById('stat-active-clients').textContent = data.data.active_clients;
            document.getElementById('stat-images').textContent = data.data.total_images;
            document.getElementById('stat-enabled-images').textContent = data.data.enabled_images;
            document.getElementById('stat-boots').textContent = data.data.total_boots;
        }
    } catch (err) {
        console.error('Failed to load stats:', err);
    }
}

// Active Sessions
async function loadActiveSessions() {
    try {
        const res = await authFetch(`${API_BASE}/active-sessions`);
        const sessions = await res.json();

        const panel = document.getElementById('active-sessions-panel');
        const content = document.getElementById('active-sessions-content');

        if (sessions && sessions.length > 0) {
            panel.style.display = 'block';
            renderActiveSessions(sessions);
        } else {
            panel.style.display = 'none';
        }
    } catch (err) {
        console.error('Failed to load active sessions:', err);
    }
}

function renderActiveSessions(sessions) {
    const content = document.getElementById('active-sessions-content');

    const html = sessions.map(session => {
        const progress = session.total_bytes > 0
            ? Math.round((session.bytes_read / session.total_bytes) * 100)
            : 0;

        const elapsed = Math.round((Date.now() - new Date(session.started_at).getTime()) / 1000);
        const speed = elapsed > 0 ? (session.bytes_read / elapsed / 1024 / 1024).toFixed(2) : 0;

        return `
            <div class="session-item">
                <div class="session-header">
                    <div>
                        <div class="session-ip">${session.ip}</div>
                        <div class="session-filename">${session.filename}</div>
                    </div>
                    <div class="session-activity">${session.activity}</div>
                </div>
                <div class="progress-bar">
                    <div class="progress-fill" style="width: ${progress}%"></div>
                </div>
                <div class="progress-text">
                    ${formatBytes(session.bytes_read)} / ${formatBytes(session.total_bytes)}
                    (${progress}%) - ${speed} MB/s - ${elapsed}s elapsed
                </div>
            </div>
        `;
    }).join('');

    content.innerHTML = html || '<p style="color: var(--text-secondary);">No active sessions</p>';
}

function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Server Info
async function loadServerInfo() {
    try {
        const res = await authFetch(`${API_BASE}/server-info`);
        const data = await res.json();

        if (data.success) {
            if (data.data && data.data.configuration && data.data.configuration.http_port) {
                cachedHTTPPort = parseInt(data.data.configuration.http_port, 10) || cachedHTTPPort;
            }
            if (data.data && data.data.configuration) {
                const smb = data.data.configuration.windows_smb || '';
                cachedWindowsSMBActive = smb.startsWith('Enabled');
                const patcher = data.data.configuration.windows_smb_patcher || '';
                cachedWindowsSMBPatcherAvailable = patcher.startsWith('Available');
            }
            renderServerInfo(data.data);
        }
    } catch (err) {
        document.getElementById('server-info').innerHTML = '<p class="alert alert-error">Failed to load server info</p>';
    }
}

// Cached from /api/server-info so the admin UI can build links to the
// public HTTP port (where ISOs are served) regardless of what port the
// admin panel itself is on.
let cachedHTTPPort = 8080;

// Cached from /api/server-info so the image properties modal knows whether
// to offer the "Patch SMB" button.
let cachedWindowsSMBActive = false;
let cachedWindowsSMBPatcherAvailable = true;

async function powerClient(action) {
    const form = document.getElementById('edit-client-form');
    const mac = form.querySelector('[name="mac_address"]').value;
    const result = document.getElementById('power-client-result');
    if (!mac) return;
    result.textContent = `Sending ${action}…`;
    result.style.color = 'var(--text-secondary)';
    try {
        const res = await authFetch(`${API_BASE}/clients/power?mac=${encodeURIComponent(mac)}&action=${encodeURIComponent(action)}`, { method: 'POST' });
        const data = await res.json();
        result.textContent = (data.success ? '✓ ' : '✗ ') + (data.message || data.error || '');
        result.style.color = data.success ? 'var(--teal, green)' : 'var(--danger)';
    } catch (err) {
        result.textContent = '✗ ' + err.message;
        result.style.color = 'var(--danger)';
    }
}

async function powerStatusClient() {
    const form = document.getElementById('edit-client-form');
    const mac = form.querySelector('[name="mac_address"]').value;
    const result = document.getElementById('power-client-result');
    if (!mac) return;
    result.textContent = 'Querying…';
    result.style.color = 'var(--text-secondary)';
    try {
        const res = await authFetch(`${API_BASE}/clients/power/status?mac=${encodeURIComponent(mac)}`);
        const data = await res.json();
        if (data.success && data.data) {
            result.textContent = 'Power: ' + (data.data.state || 'unknown');
            result.style.color = 'var(--text-primary)';
        } else {
            result.textContent = '✗ ' + (data.error || 'unknown');
            result.style.color = 'var(--danger)';
        }
    } catch (err) {
        result.textContent = '✗ ' + err.message;
        result.style.color = 'var(--danger)';
    }
}

function applySchedulePreset() {
    const preset = document.getElementById('cg-sched-preset').value;
    if (preset) document.getElementById('cg-sched-cron').value = preset;
}

function updateScheduleActionHint() {
    const action = document.getElementById('cg-sched-action').value;
    const wrap = document.getElementById('cg-sched-param-wrap');
    const label = document.getElementById('cg-sched-param-label');
    const hint = document.getElementById('cg-sched-param-hint');
    const input = document.getElementById('cg-sched-param');
    switch (action) {
        case 'power':
            wrap.style.display = '';
            label.textContent = 'Redfish Action';
            input.placeholder = 'On';
            hint.textContent = 'On / ForceOff / ForceRestart / GracefulShutdown / GracefulRestart';
            break;
        case 'next-boot':
            wrap.style.display = '';
            label.textContent = 'Image Filename';
            input.placeholder = 'ubuntu-24.04.iso';
            hint.textContent = 'Filename of the image to set as the next boot';
            break;
        default:
            wrap.style.display = 'none';
            input.value = '';
    }
}

async function loadGroupSchedules(groupId) {
    const container = document.getElementById('cg-schedules-list');
    if (!container) return;
    container.innerHTML = '<span style="color: var(--text-secondary); font-size: 12px;">Loading…</span>';
    try {
        const res = await authFetch(`${API_BASE}/scheduled-tasks?group_id=${groupId}`);
        const data = await res.json();
        if (!data.success) { container.innerHTML = ''; return; }
        const tasks = data.data || [];
        if (tasks.length === 0) {
            container.innerHTML = '<div style="color: var(--text-secondary); font-size: 12px;">No schedules yet.</div>';
            return;
        }
        container.innerHTML = tasks.map(t => {
            const last = t.last_run ? new Date(t.last_run).toLocaleString() : 'never';
            const status = t.last_status || '—';
            const param = t.action_param ? ` <code>${escapeHtml(t.action_param)}</code>` : '';
            return `
                <div style="padding: 8px; margin-bottom: 6px; background: var(--bg-secondary); border-radius: var(--radius-sm); display: flex; gap: 10px; align-items: center; flex-wrap: wrap;">
                    <span class="status-dot ${t.enabled ? 'on' : 'off'}" title="${t.enabled ? 'Enabled' : 'Disabled'}"></span>
                    <div style="flex: 1; min-width: 200px;">
                        <div><strong>${escapeHtml(t.name)}</strong> — ${escapeHtml(t.action_type)}${param}</div>
                        <div style="color: var(--text-muted); font-size: 11px;"><code>${escapeHtml(t.cron_expr)}</code> · last: ${last} (${escapeHtml(status)})</div>
                    </div>
                    <button type="button" class="btn btn-sm" onclick="runScheduleNow(${t.id})">Run Now</button>
                    <button type="button" class="btn btn-sm" onclick="toggleSchedule(${t.id}, ${!t.enabled})">${t.enabled ? 'Disable' : 'Enable'}</button>
                    <button type="button" class="btn btn-sm btn-danger" onclick="deleteSchedule(${t.id})">Delete</button>
                </div>
            `;
        }).join('');
    } catch (err) {
        container.innerHTML = '<span style="color: var(--danger);">Load failed</span>';
    }
}

async function saveNewSchedule() {
    const form = document.getElementById('edit-client-group-form');
    const groupId = parseInt(form.elements.id.value, 10);
    if (!groupId) return;
    const body = {
        name: document.getElementById('cg-sched-name').value.trim(),
        cron_expr: document.getElementById('cg-sched-cron').value.trim(),
        action_type: document.getElementById('cg-sched-action').value,
        action_param: document.getElementById('cg-sched-param').value.trim(),
        client_group_id: groupId,
        enabled: true,
    };
    if (!body.name || !body.cron_expr) {
        showAlert('Name and cron expression are required', 'error');
        return;
    }
    try {
        const res = await authFetch(`${API_BASE}/scheduled-tasks`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) { showAlert(data.error || 'Failed', 'error'); return; }
        showAlert('Schedule created', 'success');
        document.getElementById('cg-sched-name').value = '';
        document.getElementById('cg-sched-cron').value = '';
        document.getElementById('cg-sched-param').value = '';
        await loadGroupSchedules(groupId);
    } catch (err) {
        showAlert('Create failed: ' + err.message, 'error');
    }
}

async function runScheduleNow(id) {
    try {
        const res = await authFetch(`${API_BASE}/scheduled-tasks/run?id=${id}`, { method: 'POST' });
        const data = await res.json();
        showAlert(data.message || (data.success ? 'Dispatched' : 'Failed'), data.success ? 'success' : 'error');
        const groupId = parseInt(document.getElementById('edit-client-group-form').elements.id.value, 10);
        setTimeout(() => loadGroupSchedules(groupId), 800);
    } catch (err) { showAlert('Run failed: ' + err.message, 'error'); }
}

async function toggleSchedule(id, enabled) {
    try {
        const res = await authFetch(`${API_BASE}/scheduled-tasks/update?id=${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ enabled }),
        });
        const data = await res.json();
        if (!data.success) { showAlert(data.error || 'Update failed', 'error'); return; }
        const groupId = parseInt(document.getElementById('edit-client-group-form').elements.id.value, 10);
        await loadGroupSchedules(groupId);
    } catch (err) { showAlert('Toggle failed: ' + err.message, 'error'); }
}

async function deleteSchedule(id) {
    if (!confirm('Delete this schedule?')) return;
    try {
        const res = await authFetch(`${API_BASE}/scheduled-tasks/delete?id=${id}`, { method: 'DELETE' });
        const data = await res.json();
        if (!data.success) { showAlert(data.error || 'Delete failed', 'error'); return; }
        const groupId = parseInt(document.getElementById('edit-client-group-form').elements.id.value, 10);
        await loadGroupSchedules(groupId);
    } catch (err) { showAlert('Delete failed: ' + err.message, 'error'); }
}

async function bulkPowerClientGroup(action) {
    const id = parseInt(document.getElementById('edit-client-group-form').elements.id.value, 10);
    if (!id) return;
    try {
        const res = await authFetch(`${API_BASE}/client-groups/power?id=${id}&action=${encodeURIComponent(action)}`, { method: 'POST' });
        const data = await res.json();
        showAlert(data.message || (data.success ? 'OK' : 'Failed'), data.success ? 'success' : 'error');
    } catch (err) { showAlert('Bulk power failed: ' + err.message, 'error'); }
}

async function loadWebhookConfig() {
    try {
        const res = await authFetch(`${API_BASE}/webhook`);
        const data = await res.json();
        if (!data.success || !data.data) return;
        const c = data.data;
        document.getElementById('webhook-enabled').checked = !!c.enabled;
        document.getElementById('webhook-url').value = c.url || '';
        document.getElementById('webhook-on-boot-started').checked = !!c.on_boot_started;
        document.getElementById('webhook-on-client-discovered').checked = !!c.on_client_discovered;
        document.getElementById('webhook-on-inventory-updated').checked = !!c.on_inventory_updated;
    } catch (err) {
        console.error('Failed to load webhook config:', err);
    }
}

async function saveWebhookConfig(e) {
    if (e) e.preventDefault();
    const body = {
        enabled: document.getElementById('webhook-enabled').checked,
        url: document.getElementById('webhook-url').value.trim(),
        on_boot_started: document.getElementById('webhook-on-boot-started').checked,
        on_client_discovered: document.getElementById('webhook-on-client-discovered').checked,
        on_inventory_updated: document.getElementById('webhook-on-inventory-updated').checked,
    };
    try {
        const res = await authFetch(`${API_BASE}/webhook`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (data.success) showAlert('Webhook saved', 'success');
        else showAlert(data.error || 'Save failed', 'error');
    } catch (err) {
        showAlert('Save failed: ' + err.message, 'error');
    }
}

async function testWebhook() {
    const result = document.getElementById('webhook-test-result');
    result.textContent = 'Sending…';
    result.style.color = 'var(--text-secondary)';
    try {
        const res = await authFetch(`${API_BASE}/webhook/test`, { method: 'POST' });
        const data = await res.json();
        result.textContent = (data.success ? '✓ ' : '✗ ') + (data.message || data.error || '');
        result.style.color = data.success ? 'var(--teal, green)' : 'var(--danger)';
    } catch (err) {
        result.textContent = '✗ ' + err.message;
        result.style.color = 'var(--danger)';
    }
}

async function downloadBackup() {
    try {
        const res = await authFetch(`${API_BASE}/backup/export`);
        if (!res.ok) {
            showAlert('Backup download failed', 'error');
            return;
        }
        const blob = await res.blob();
        const cd = res.headers.get('Content-Disposition') || '';
        let filename = 'bootimus-backup.tar.gz';
        const m = cd.match(/filename="?([^";]+)"?/);
        if (m) filename = m[1];
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(a.href);
    } catch (err) {
        showAlert('Backup download failed: ' + err.message, 'error');
    }
}

function downloadISOFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    if (!filename) return;
    const url = `${window.location.protocol}//${window.location.hostname}:${cachedHTTPPort}/isos/${encodeURIComponent(filename)}`;
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.target = '_blank';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
}

function renderServerInfo(info) {
    const container = document.getElementById('server-info');
    const sysStats = info.system_stats || {};

    function resColor(pct) {
        if (pct > 80) return 'var(--danger)';
        if (pct > 60) return 'var(--warning)';
        return 'var(--teal)';
    }

    function ringGauge(pct, color) {
        const size = 128, r = 54, stroke = 10;
        const C = 2 * Math.PI * r;
        const offset = C - (Math.max(0, Math.min(100, pct)) / 100) * C;
        return `
            <svg viewBox="0 0 ${size} ${size}">
                <circle class="res-gauge-track" cx="${size/2}" cy="${size/2}" r="${r}"
                        fill="none" stroke-width="${stroke}"/>
                <circle class="res-gauge-fill" cx="${size/2}" cy="${size/2}" r="${r}"
                        fill="none" stroke="${color}" stroke-width="${stroke}"
                        stroke-dasharray="${C.toFixed(2)}" stroke-dashoffset="${offset.toFixed(2)}"
                        stroke-linecap="round"
                        transform="rotate(-90 ${size/2} ${size/2})"
                        style="color: ${color}"/>
            </svg>
        `;
    }

    function resCard(label, pct, detail) {
        const color = resColor(pct);
        return `
        <div class="res-card">
            <span class="res-label">${label}</span>
            <div class="res-gauge">
                ${ringGauge(pct, color)}
                <div class="res-gauge-text" style="color: ${color}">${pct.toFixed(1)}%</div>
            </div>
            <span class="res-detail">${detail}</span>
        </div>`;
    }

    // Update version in sidebar and about modal
    if (info.version) {
        document.getElementById('sidebar-version').textContent = 'v' + info.version;
        document.getElementById('about-version').textContent = 'Version ' + info.version;
    }

    // Build running status grid cells
    let statusCards = '';
    if (info.version) {
        statusCards += `<div class="rs-metric"><span class="rs-label">${t('server.field.version')}</span><span class="rs-value">${info.version}</span></div>`;
    }
    if (sysStats.uptime) {
        statusCards += `<div class="rs-metric"><span class="rs-label">${t('server.field.uptime')}</span><span class="rs-value" style="color: var(--accent)">${sysStats.uptime}</span></div>`;
    }
    if (info.configuration && info.configuration.runtime_mode) {
        statusCards += `<div class="rs-metric"><span class="rs-label">${t('server.field.runtime_mode')}</span><span class="rs-value"><span class="badge ${info.configuration.runtime_mode === 'Docker' ? 'badge-info' : 'badge-success'}">${info.configuration.runtime_mode}</span></span></div>`;
    }
    if (sysStats.host) {
        const os = sysStats.host.platform ? `${sysStats.host.platform} ${sysStats.host.platform_version || ''}`.trim() : (sysStats.host.os || '');
        if (os) statusCards += `<div class="rs-metric"><span class="rs-label">${t('server.field.os')}</span><span class="rs-value">${os}</span></div>`;
        if (sysStats.host.architecture) statusCards += `<div class="rs-metric"><span class="rs-label">${t('server.field.arch')}</span><span class="rs-value">${sysStats.host.architecture}</span></div>`;
    }

    // Build resource cards: ring gauge per metric
    let resourceCards = '';
    if (sysStats.cpu) {
        resourceCards += resCard(
            t('server.metric.cpu'),
            sysStats.cpu.usage_percent,
            t('server.metric.cores_available', { n: sysStats.cpu.cores })
        );
    }
    if (sysStats.memory) {
        resourceCards += resCard(
            t('server.metric.memory'),
            sysStats.memory.used_percent,
            t('server.metric.memory_detail', {
                used: formatBytes(sysStats.memory.used),
                total: formatBytes(sysStats.memory.total),
            })
        );
    }
    (sysStats.disk || []).forEach(disk => {
        resourceCards += resCard(
            `${t('server.metric.disk')} ${disk.path}`,
            disk.used_percent,
            t('server.metric.disk_detail', {
                free: formatBytes(disk.free),
                total: formatBytes(disk.total),
            })
        );
    });

    // Translate the config row label using the dictionary if a key exists,
    // otherwise fall back to the humanised snake_case version.
    function configLabel(key) {
        const dictKey = 'server.config.' + key;
        const translated = t(dictKey);
        if (translated !== dictKey) return translated;
        return key.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
    }

    // Build configuration key-value pairs
    const configItems = Object.entries(info.configuration || {}).filter(([key]) => key !== 'runtime_mode').map(([key, value]) => `
        <div class="info-item">
            <span class="info-label">${configLabel(key)}</span>
            <span class="info-value">${value || '<em style="color:var(--text-muted)">-</em>'}</span>
        </div>
    `).join('');

    // Build environment variable items
    const envEntries = Object.entries(info.environment || {}).filter(([, v]) => v);
    const envItems = envEntries.length > 0
        ? envEntries.map(([key, value]) => `<div class="info-item"><span class="info-label">${key}</span><span class="info-value">${value}</span></div>`).join('')
        : `<p style="color: var(--text-muted); padding: 16px 0; font-size: 13px;">${t('server.env.empty')}</p>`;

    container.innerHTML = `
        <div class="si-section">
            <h3 class="si-heading">${t('server.section.running_status')}</h3>
            <div class="rs-grid">${statusCards}</div>
        </div>

        ${resourceCards ? `
        <div class="si-section">
            <h3 class="si-heading si-heading-teal">${t('server.section.system_resources')}</h3>
            <div class="res-grid">${resourceCards}</div>
        </div>
        ` : ''}

        <div class="si-section">
            <div class="info-grid">
                <div class="info-section">
                    <h3>${t('server.section.configuration')}</h3>
                    ${configItems}
                </div>
                <div class="info-section">
                    <h3>${t('server.section.environment')}</h3>
                    ${envItems}
                </div>
            </div>
        </div>
    `;
}

function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Clients
let clientsAutoRefreshInterval = null;

function toggleClientsAutoRefresh() {
    const btn = document.getElementById('clients-autorefresh-btn');
    if (clientsAutoRefreshInterval) {
        clearInterval(clientsAutoRefreshInterval);
        clientsAutoRefreshInterval = null;
        if (btn) btn.setAttribute('aria-pressed', 'false');
    } else {
        loadClients();
        clientsAutoRefreshInterval = setInterval(loadClients, 5000);
        if (btn) btn.setAttribute('aria-pressed', 'true');
    }
}

async function loadClients() {
    try {
        const res = await authFetch(`${API_BASE}/clients`);
        const data = await res.json();

        if (data.success) {
            clients = data.data || [];
            renderClientsTable();
        }
    } catch (err) {
        document.getElementById('clients-table').innerHTML = '<p class="alert alert-error">Failed to load clients</p>';
    }
}

function renderClientsTable() {
    const container = document.getElementById('clients-table');

    const filteredClients = clients.filter(c => rowMatchesFilter('clients', [c.mac_address, c.name, c.description, c.bootloader_set]));

    if (clients.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No clients yet. Add one to get started.</p>';
        return;
    }

    const html = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>MAC Address</th>
                    <th>Name</th>
                    <th>Type</th>
                    <th class="col-dot" title="Enabled / Disabled">On</th>
                    <th>Bootloader</th>
                    <th>Assigned Images</th>
                    <th>Boot Count</th>
                    <th>Last Boot</th>
                    <th>Quick Actions</th>
                </tr>
            </thead>
            <tbody>
                ${filteredClients.map(client => `
                    <tr class="row-clickable" onclick="editClient('${client.mac_address}')">
                        <td><code>${client.mac_address}</code></td>
                        <td>${client.name || '-'}</td>
                        <td>
                            <span class="badge ${client.static ? 'badge-success' : 'badge-info'}">
                                ${client.static ? 'Static' : 'Discovered'}
                            </span>
                        </td>
                        <td class="col-dot">
                            <span class="status-dot ${client.enabled ? 'on' : 'off'}" title="${client.enabled ? 'Enabled' : 'Disabled'}"></span>
                        </td>
                        <td>
                            ${client.bootloader_set ?
                                '<span class="badge badge-info">' + escapeHtml(client.bootloader_set) + '</span>' :
                                '<span style="color: var(--text-secondary);">Default</span>'
                            }
                        </td>
                        <td>
                            ${(client.images || []).length > 0 ?
                                `<span title="${(client.images || []).map(i => i.name).join(', ')}">${(client.images || []).length} images</span>` :
                                '<span style="color: var(--text-secondary);">No images</span>'
                            }
                        </td>
                        <td>${client.boot_count || 0}</td>
                        <td>
                            ${client.last_boot ? new Date(client.last_boot).toLocaleString() : 'Never'}
                            ${client.next_boot_image ? '<br><span class="badge badge-info" title="' + escapeHtml(client.next_boot_image) + '">Next: ' + escapeHtml(client.next_boot_image) + '</span>' : ''}
                        </td>
                        <td onclick="event.stopPropagation()">
                            ${!client.static ? '<button class="btn btn-success btn-sm" onclick="promoteClient(\'' + client.mac_address + '\')">Make Static</button>' : ''}
                            <button class="btn btn-success btn-sm" onclick="wakeClient('${client.mac_address}')">Wake</button>
                            <button class="btn btn-primary btn-sm" onclick="showNextBoot('${client.mac_address}')">Next Boot</button>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
        </div>
    `;

    container.innerHTML = html;
}

function showAddClientModal() {
    document.getElementById('add-client-form').reset();
    showModal('add-client-modal');
}

async function editClient(mac) {
    try {
        const res = await authFetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`);
        const data = await res.json();

        console.log('GetClient response:', JSON.stringify(data));
        if (data.success) {
            currentClient = data.data;
            console.log('Client data:', JSON.stringify(currentClient));

            const form = document.getElementById('edit-client-form');

            // Set form values
            form.querySelector('[name="mac_address"]').value = currentClient.mac_address || mac || '';
            form.querySelector('[name="name"]').value = currentClient.name || '';
            form.querySelector('[name="description"]').value = currentClient.description || '';
            form.querySelector('[name="enabled"]').checked = currentClient.enabled || false;
            form.querySelector('[name="show_public_images"]').checked = currentClient.show_public_images !== false;

            // BMC / Redfish
            form.querySelector('[name="ipmi_host"]').value = currentClient.ipmi_host || '';
            form.querySelector('[name="ipmi_port"]').value = currentClient.ipmi_port || '';
            form.querySelector('[name="ipmi_username"]').value = currentClient.ipmi_username || '';
            form.querySelector('[name="ipmi_password"]').value = currentClient.ipmi_password || '';
            form.querySelector('[name="ipmi_insecure"]').checked = !!currentClient.ipmi_insecure;
            const powerResult = document.getElementById('power-client-result');
            if (powerResult) powerResult.textContent = '';

            // Populate bootloader set dropdown
            try {
                const blRes = await authFetch(`${API_BASE}/bootloaders`);
                const blData = await blRes.json();
                const blSelect = document.getElementById('edit-bootloader-set-select');
                blSelect.innerHTML = '<option value="">Default (global setting)</option>';
                if (blData.success && blData.data && blData.data.sets) {
                    for (const set of blData.data.sets) {
                        const selected = currentClient.bootloader_set === set.name ? 'selected' : '';
                        blSelect.innerHTML += `<option value="${escapeHtml(set.name)}" ${selected}>${escapeHtml(set.name)}</option>`;
                    }
                }
            } catch (err) {
                console.error('Failed to load bootloader sets:', err);
            }

            // Populate client group dropdown
            try {
                const cgRes = await authFetch(`${API_BASE}/client-groups`);
                const cgData = await cgRes.json();
                const cgSelect = document.getElementById('edit-client-group-select');
                cgSelect.innerHTML = '<option value="">(none)</option>';
                if (cgData.success && cgData.data) {
                    for (const g of cgData.data) {
                        const selected = currentClient.client_group_id === g.id ? 'selected' : '';
                        cgSelect.innerHTML += `<option value="${g.id}" ${selected}>${escapeHtml(g.name)}</option>`;
                    }
                }
            } catch (err) {
                console.error('Failed to load client groups:', err);
            }

            await populateAutoInstallFileDropdown('edit-client-autoinstall-select', currentClient.auto_install_file);

            // Populate images select using allowed_images (persisted filename list)
            const select = document.getElementById('edit-images-select');
            const allowedImages = currentClient.allowed_images || [];

            select.innerHTML = images.map(img => {
                const isSelected = allowedImages.includes(img.filename);
                return `<option value="${img.filename}" ${isSelected ? 'selected' : ''}>${img.name}</option>`;
            }).join('');

            showModal('edit-client-modal');
            loadClientInventory(currentClient.mac_address);
            loadClientBootHistory(currentClient.mac_address);
        } else {
            showAlert(data.error || 'Failed to load client', 'error');
        }
    } catch (err) {
        console.error('Error in editClient:', err);
        showAlert('Failed to load client', 'error');
    }
}

async function loadClientBootHistory(mac) {
    const container = document.getElementById('client-boot-history-list');
    if (!container) return;
    container.innerHTML = '<p style="color: var(--text-secondary); margin: 0;">Loading…</p>';
    try {
        const res = await authFetch(`${API_BASE}/boot-logs?mac=${encodeURIComponent(mac)}&limit=50`);
        const data = await res.json();
        if (!data.success || !data.data || data.data.length === 0) {
            container.innerHTML = '<p style="color: var(--text-secondary); margin: 0;">No boot history for this client yet.</p>';
            return;
        }
        const rows = data.data.map(entry => {
            const when = entry.CreatedAt ? new Date(entry.CreatedAt).toLocaleString() : '-';
            const img = entry.ImageName || '(unknown)';
            const ok = entry.Success
                ? '<span class="status-dot on" title="Success"></span>'
                : '<span class="status-dot off" title="Failed"></span>';
            const err = entry.ErrorMsg ? `<div style="color: var(--danger); font-size: 12px;">${escapeHtml(entry.ErrorMsg)}</div>` : '';
            const ip = entry.IPAddress ? `<span style="color: var(--text-muted); font-size: 12px;">${escapeHtml(entry.IPAddress)}</span>` : '';
            return `
                <div style="padding: 8px 0; border-bottom: 1px solid var(--border); display: flex; gap: 10px; align-items: center;">
                    <div style="width: 14px;">${ok}</div>
                    <div style="flex: 1;">
                        <div><strong>${escapeHtml(img)}</strong></div>
                        <div style="color: var(--text-secondary); font-size: 12px;">${when} &middot; ${ip}</div>
                        ${err}
                    </div>
                </div>
            `;
        }).join('');
        container.innerHTML = `<div style="max-height: 300px; overflow-y: auto;">${rows}</div>`;
    } catch (err) {
        container.innerHTML = '<p style="color: var(--danger); margin: 0;">Failed to load boot history.</p>';
    }
}

async function loadClientInventory(mac) {
    const container = document.getElementById('client-hardware-info');
    const details = document.getElementById('client-hw-details');

    try {
        const res = await authFetch(`${API_BASE}/clients/inventory?mac=${encodeURIComponent(mac)}`);
        const data = await res.json();

        if (!data.success || !data.data) {
            container.style.display = 'none';
            return;
        }

        const inv = data.data;
        container.style.display = 'block';

        const fields = [
            ['Manufacturer', inv.manufacturer],
            ['Product', inv.product],
            ['Serial', inv.serial],
            ['UUID', inv.uuid],
            ['CPU', inv.cpu],
            ['Memory', inv.memory ? formatBytes(inv.memory) : ''],
            ['Platform', inv.platform],
            ['Architecture', inv.buildarch],
            ['NIC', inv.nic_chip],
            ['IP Address', inv.ip_address],
            ['Last Seen', inv.created_at ? new Date(inv.created_at).toLocaleString() : ''],
        ].filter(([, v]) => v);

        if (fields.length === 0) {
            container.style.display = 'none';
            return;
        }

        details.innerHTML = `<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 6px 16px; font-size: 13px;">
            ${fields.map(([label, value]) => `
                <div style="color: var(--text-secondary);">${label}</div>
                <div style="color: var(--text-primary); font-weight: 500;">${escapeHtml(String(value))}</div>
            `).join('')}
        </div>`;
    } catch (err) {
        container.style.display = 'none';
    }
}

async function showInventoryHistory() {
    if (!currentClient) return;

    try {
        const res = await authFetch(`${API_BASE}/clients/inventory/history?mac=${encodeURIComponent(currentClient.mac_address)}&limit=50`);
        const data = await res.json();
        const container = document.getElementById('inventory-history-table');

        if (!data.success || !data.data || data.data.length === 0) {
            container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No inventory history.</p>';
            openModal('inventory-history-modal');
            return;
        }

        const html = `
            <div class="table-scroll" style="max-height: 400px;">
            <table>
                <thead>
                    <tr>
                        <th>Time</th>
                        <th>IP Address</th>
                        <th>Manufacturer</th>
                        <th>Product</th>
                        <th>Serial</th>
                        <th>CPU</th>
                        <th>Memory</th>
                        <th>Platform</th>
                    </tr>
                </thead>
                <tbody>
                    ${data.data.map(inv => `
                        <tr>
                            <td>${new Date(inv.created_at).toLocaleString()}</td>
                            <td>${escapeHtml(inv.ip_address || '-')}</td>
                            <td>${escapeHtml(inv.manufacturer || '-')}</td>
                            <td>${escapeHtml(inv.product || '-')}</td>
                            <td>${escapeHtml(inv.serial || '-')}</td>
                            <td>${escapeHtml(inv.cpu || '-')}</td>
                            <td>${inv.memory ? formatBytes(inv.memory) : '-'}</td>
                            <td>${escapeHtml(inv.platform || '-')}</td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
            </div>
        `;

        container.innerHTML = html;
        openModal('inventory-history-modal');
    } catch (err) {
        showNotification('Failed to load inventory history', 'error');
    }
}

function deleteFromEditClient() {
    const form = document.getElementById('edit-client-form');
    const mac = form.querySelector('[name="mac_address"]').value;
    if (!mac) return;
    closeModal('edit-client-modal');
    deleteClient(mac);
}

async function deleteClient(mac) {
    if (!confirm(`Delete client ${mac}?`)) return;

    try {
        const res = await authFetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`, { method: 'DELETE' });
        const data = await res.json();

        if (data.success) {
            showAlert('Client deleted successfully', 'success');
            loadClients();
            loadStats();
        } else {
            showAlert(data.error || 'Failed to delete client', 'error');
        }
    } catch (err) {
        showAlert('Failed to delete client', 'error');
    }
}

async function wakeClient(mac) {
    try {
        const res = await authFetch(`${API_BASE}/clients/wake?mac=${encodeURIComponent(mac)}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showNotification('Wake-on-LAN packet sent to ' + mac, 'success');
        } else {
            showNotification(data.error || 'Failed to send WOL packet', 'error');
        }
    } catch (err) {
        showNotification('Failed to send WOL packet', 'error');
    }
}

async function promoteClient(mac) {
    try {
        const res = await authFetch(`${API_BASE}/clients/promote?mac=${encodeURIComponent(mac)}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showNotification('Client promoted to static', 'success');
            loadClients();
        } else {
            showNotification(data.error || 'Failed to promote client', 'error');
        }
    } catch (err) {
        showNotification('Failed to promote client', 'error');
    }
}

async function showNextBoot(mac) {
    const client = clients.find(c => c.mac_address === mac);
    document.getElementById('next-boot-mac').value = mac + (client && client.name ? ' (' + client.name + ')' : '');
    document.getElementById('next-boot-mac').dataset.mac = mac;

    const select = document.getElementById('next-boot-image-select');
    select.innerHTML = images.map(img =>
        `<option value="${img.filename}">${img.name}</option>`
    ).join('');

    const currentDiv = document.getElementById('next-boot-current');
    if (client && client.next_boot_image) {
        const img = images.find(i => i.filename === client.next_boot_image);
        document.getElementById('next-boot-current-image').textContent = img ? img.name : client.next_boot_image;
        currentDiv.style.display = 'block';
    } else {
        currentDiv.style.display = 'none';
    }

    showModal('next-boot-modal');
}

async function saveNextBoot() {
    const mac = document.getElementById('next-boot-mac').dataset.mac;
    const imageFilename = document.getElementById('next-boot-image-select').value;
    try {
        const res = await authFetch(`${API_BASE}/clients/next-boot`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac_address: mac, image_filename: imageFilename })
        });
        const data = await res.json();
        if (data.success) {
            showNotification('Next boot action set', 'success');
            closeModal('next-boot-modal');
            loadClients();
        } else {
            showNotification(data.error || 'Failed to set next boot', 'error');
        }
    } catch (err) {
        showNotification('Failed to set next boot', 'error');
    }
}

async function saveNextBootAndWake() {
    const mac = document.getElementById('next-boot-mac').dataset.mac;
    const imageFilename = document.getElementById('next-boot-image-select').value;
    try {
        const res = await authFetch(`${API_BASE}/clients/next-boot`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac_address: mac, image_filename: imageFilename })
        });
        const data = await res.json();
        if (data.success) {
            // Now send WOL
            const wolRes = await authFetch(`${API_BASE}/clients/wake?mac=${encodeURIComponent(mac)}`, { method: 'POST' });
            const wolData = await wolRes.json();
            if (wolData.success) {
                showNotification('Next boot set & WOL packet sent', 'success');
            } else {
                showNotification('Next boot set but WOL failed: ' + (wolData.error || ''), 'warning');
            }
            closeModal('next-boot-modal');
            loadClients();
        } else {
            showNotification(data.error || 'Failed to set next boot', 'error');
        }
    } catch (err) {
        showNotification('Failed to set next boot', 'error');
    }
}

async function clearNextBoot() {
    const mac = document.getElementById('next-boot-mac').dataset.mac;
    try {
        const res = await authFetch(`${API_BASE}/clients/next-boot`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac_address: mac, image_filename: '' })
        });
        const data = await res.json();
        if (data.success) {
            showNotification('Next boot action cleared', 'success');
            closeModal('next-boot-modal');
            loadClients();
        } else {
            showNotification(data.error || 'Failed to clear next boot', 'error');
        }
    } catch (err) {
        showNotification('Failed to clear next boot', 'error');
    }
}

// Images
async function loadImages() {
    try {
        const [imagesRes, filesRes, groupsRes] = await Promise.all([
            authFetch(`${API_BASE}/images`),
            authFetch(`${API_BASE}/files`),
            authFetch(`${API_BASE}/groups`),
        ]);

        const imagesData = await imagesRes.json();
        const filesData = await filesRes.json();
        const groupsData = await groupsRes.json();

        if (imagesData.success) {
            images = imagesData.data || [];

            // Associate files with images
            if (filesData.success) {
                const allFiles = filesData.data || [];
                images.forEach(img => {
                    img.files = allFiles.filter(f => !f.public && f.image_id === img.id);
                });
            }

            // Cache groups so the tree view can build the parent hierarchy.
            if (groupsData && groupsData.success) {
                groups = groupsData.data || [];
            }

            renderImagesTable();
        }
    } catch (err) {
        document.getElementById('images-table').innerHTML = '<p class="alert alert-error">Failed to load images</p>';
    }
}

function sortImages(column) {
    if (imageSortColumn === column) {
        imageSortDirection = imageSortDirection === 'asc' ? 'desc' : 'asc';
    } else {
        imageSortColumn = column;
        imageSortDirection = 'asc';
    }
    renderImagesTable();
}

function getSortedImages() {
    const sorted = [...images].sort((a, b) => {
        let aVal, bVal;

        switch (imageSortColumn) {
            case 'name':
                aVal = (a.name || '').toLowerCase();
                bVal = (b.name || '').toLowerCase();
                break;
            case 'filename':
                aVal = (a.filename || '').toLowerCase();
                bVal = (b.filename || '').toLowerCase();
                break;
            case 'size':
                aVal = a.size || 0;
                bVal = b.size || 0;
                break;
            case 'status':
                aVal = a.enabled ? 1 : 0;
                bVal = b.enabled ? 1 : 0;
                break;
            case 'visibility':
                aVal = a.public ? 1 : 0;
                bVal = b.public ? 1 : 0;
                break;
            case 'boot_method':
                aVal = a.boot_method || '';
                bVal = b.boot_method || '';
                break;
            case 'distro':
                aVal = (a.distro || '').toLowerCase();
                bVal = (b.distro || '').toLowerCase();
                break;
            case 'boot_count':
                aVal = a.boot_count || 0;
                bVal = b.boot_count || 0;
                break;
            case 'group':
                aVal = (a.group && a.group.name ? a.group.name : '').toLowerCase();
                bVal = (b.group && b.group.name ? b.group.name : '').toLowerCase();
                break;
            default:
                return 0;
        }

        if (aVal < bVal) return imageSortDirection === 'asc' ? -1 : 1;
        if (aVal > bVal) return imageSortDirection === 'asc' ? 1 : -1;
        return 0;
    });

    return sorted;
}

const UNGROUPED_KEY = '__ungrouped__';

function toggleImageGrouping() {
    imageGroupedView = !imageGroupedView;
    try { localStorage.setItem('image-grouped-view', imageGroupedView ? '1' : '0'); } catch (_) {}
    syncImageGroupingButton();
    renderImagesTable();
}

function syncImageGroupingButton() {
    const btn = document.getElementById('images-group-toggle');
    if (!btn) return;
    btn.setAttribute('aria-pressed', imageGroupedView ? 'true' : 'false');
}

function toggleImageGroup(key) {
    if (collapsedImageGroups.has(key)) collapsedImageGroups.delete(key);
    else collapsedImageGroups.add(key);
    try { localStorage.setItem('image-collapsed-groups', JSON.stringify([...collapsedImageGroups])); } catch (_) {}
    renderImagesTable();
}

function toggleImageGroupFromEvent(el) {
    if (!el || !el.dataset) return;
    try { toggleImageGroup(JSON.parse(el.dataset.groupKey)); } catch (_) {}
}

function _attrEscape(s) {
    return String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
}

// Build a parent/child tree from a flat list of groups using `group.parent.id`.
// Groups with a missing or unresolved parent become roots. Returns {roots, byId}.
function _buildGroupTree(allGroups) {
    const byId = new Map();
    for (const g of allGroups) {
        byId.set(g.id, { id: g.id, name: g.name, order: g.order || 0, parentId: g.parent && g.parent.id, children: [] });
    }
    const roots = [];
    for (const node of byId.values()) {
        if (node.parentId != null && byId.has(node.parentId) && node.parentId !== node.id) {
            byId.get(node.parentId).children.push(node);
        } else {
            roots.push(node);
        }
    }
    const sortFn = (a, b) => (a.order - b.order) || a.name.localeCompare(b.name);
    roots.sort(sortFn);
    for (const node of byId.values()) node.children.sort(sortFn);
    return { roots, byId };
}

// Count images directly in this group + all descendant groups.
function _countTreeImages(node, ownByGroup) {
    let n = (ownByGroup.get(node.id) || []).length;
    for (const child of node.children) n += _countTreeImages(child, ownByGroup);
    return n;
}

// Heuristics for guiding the user before extraction has run. The backend
// only knows the distro after extraction, so we pattern-match the filename
// to flag images that almost certainly won't boot via sanboot.
const _filenameNeedsExtraction = [
    'ubuntu', 'xubuntu', 'kubuntu', 'lubuntu', 'edubuntu', 'ubuntumate',
    'ubuntustudio', 'budgie', 'noble', 'jammy',
    'popos', 'pop-os', 'pop_os',
    'mint', 'linuxmint',
    'elementary', 'zorin',
    'windows', 'win10', 'win11', 'win7',
    'server2019', 'server2022', 'server2025'
];
const _filenameWindows = [
    'windows', 'win10', 'win11', 'win7',
    'server2019', 'server2022', 'server2025'
];
function filenameLikelyNeedsExtraction(filename) {
    const f = (filename || '').toLowerCase();
    return _filenameNeedsExtraction.some(p => f.includes(p));
}
function filenameLooksWindows(filename) {
    const f = (filename || '').toLowerCase();
    return _filenameWindows.some(p => f.includes(p));
}

// Profile cache, keyed by profile_id. Populated by loadProfileCache() at
// startup so the images table can compare each image's boot_method against
// the distro's preferred boot_method without a per-row fetch.
let _profileCache = {};
async function loadProfileCache() {
    try {
        const res = await authFetch(`${API_BASE}/profiles`);
        const data = await res.json();
        if (!data.success) return;
        const next = {};
        for (const p of (data.data || [])) {
            next[p.profile_id] = p;
        }
        _profileCache = next;
    } catch (e) {
        // Non-fatal: row health check just falls back to "ok".
    }
}
function getPreferredBootMethod(distro) {
    if (!distro) return '';
    const p = _profileCache[distro];
    return (p && p.boot_method) || '';
}

// computeImageHealth returns 'ok' or an object {reason} when the image's
// current state will not boot reliably with its current boot method.
function computeImageHealth(img) {
    const preferred = getPreferredBootMethod(img.distro);

    if (img.netboot_required && !img.netboot_available) {
        return { reason: 'Netboot files required' };
    }
    if ((img.boot_method === 'kernel' || img.boot_method === 'wimboot') && !img.extracted) {
        return { reason: 'Kernel boot needs extraction' };
    }
    if (img.boot_method === 'nbd' && !img.extracted) {
        return { reason: 'NBD boot needs extraction' };
    }
    // Distro is known and the boot method doesn't match what the profile
    // recommends — same-distro mismatch is the strongest signal of "won't
    // boot well", e.g. Ubuntu on sanboot.
    if (preferred && img.boot_method && preferred !== img.boot_method) {
        // Treat wimboot and kernel interchangeably for this check; both are
        // "extracted, served directly" in the iPXE menu.
        const p = preferred === 'wimboot' ? 'kernel' : preferred;
        const m = img.boot_method === 'wimboot' ? 'kernel' : img.boot_method;
        if (p !== m) {
            return { reason: `${img.distro} prefers ${preferred} boot, currently ${img.boot_method}` };
        }
    }
    return 'ok';
}

function imageRowHTML(img, includeGroupCell, depth = 0) {
    const groupCell = includeGroupCell ? `
                        <td>
                            ${img.group && img.group.name ?
                                '<span class="badge badge-info">' + escapeHtml(img.group.name) + '</span>' :
                                '<span style="color: var(--text-secondary);">-</span>'
                            }
                        </td>` : '';
    const namePadStyle = depth > 0 ? ` style="padding-left: ${10 + depth * 20}px;"` : '';
    const health = computeImageHealth(img);
    const rowClass = health === 'ok' ? 'row-clickable' : 'row-clickable row-warning';
    const rowTitle = health === 'ok' ? '' : ` title="${escapeHtml(health.reason)}"`;
    return `
                    <tr class="${rowClass}"${rowTitle} onclick="showImagePropertiesModal('${img.filename}')">
                        <td${namePadStyle}>${img.name}</td>
                        <td><code>${img.filename}</code></td>
                        <td>${formatBytes(img.size)}</td>
                        <td>
                            ${img.extracted ?
                                (img.distro ? '<span class="badge badge-info">'+img.distro+'</span>' : '<span class="badge badge-success">✓ Extracted</span>') :
                                (img.extraction_error ? '<span class="badge badge-danger" title="'+img.extraction_error+'">Error</span>' : '')
                            }
                            ${img.smb_install_enabled ? ' <span class="badge badge-warning" title="boot.wim patched to auto-mount SMB share and launch setup.exe">SMB</span>' : ''}
                        </td>${groupCell}
                        <td class="col-dot">
                            <span class="status-dot ${img.enabled ? 'on' : 'off'}" title="${img.enabled ? 'Enabled' : 'Disabled'}"></span>
                        </td>
                        <td class="col-dot">
                            <span class="status-dot ${img.public ? 'on' : 'off'}" title="${img.public ? 'Public' : 'Private'}"></span>
                        </td>
                        <td style="white-space: nowrap;">
                            ${img.boot_method === 'kernel' ?
                                '<span class="badge badge-success">Kernel</span>' :
                                img.boot_method === 'nbd' ?
                                '<span class="badge badge-warning">NBD</span>' :
                                '<span class="badge badge-info">SAN</span>'
                            }
                            ${!img.sanboot_compatible && img.sanboot_hint && img.boot_method === 'sanboot' && !img.extracted ?
                                ' <span title="'+escapeHtml(img.sanboot_hint)+'" style="color: #ff9800; cursor: help;">⚠</span>' :
                                ''
                            }
                            ${img.extracted && img.boot_method === 'sanboot' ?
                                ' <button class="btn btn-sm" onclick="setBootMethod(\''+img.filename+'\', \'kernel\')">→ Kernel</button>' :
                                ''
                            }
                        </td>
                        <td class="operations-cell">
                            ${extractionProgress[img.filename] ? `
                                <div class="progress-container">
                                    <div class="progress-bar">
                                        <div class="progress-fill" style="width: ${extractionProgress[img.filename].progress}%"></div>
                                    </div>
                                    <div class="progress-text">${extractionProgress[img.filename].status}</div>
                                </div>
                            ` : (health !== 'ok'
                                ? '<span style="color: var(--warning-hover); font-weight:500;">⚠ '+escapeHtml(health.reason)+'</span>'
                                : '<span style="color: #4caf50;">✓ Ready</span>'
                            )}
                        </td>
                    </tr>`;
}

function renderImagesTable() {
    const container = document.getElementById('images-table');

    if (images.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No images yet. Upload or scan for ISOs.</p>';
        return;
    }

    syncImageGroupingButton();

    const sortIcon = (column) => {
        if (imageSortColumn !== column) return '↕';
        return imageSortDirection === 'asc' ? '↑' : '↓';
    };

    const sortedImages = getSortedImages().filter(img => rowMatchesFilter('images', [
        img.name, img.filename, img.distro, img.group && img.group.name,
    ]));

    const filterActive = !!(tableFilters['images'] && tableFilters['images'].length);

    let bodyHtml;
    let theadHtml;
    let colspan;

    if (!imageGroupedView) {
        colspan = 9;
        theadHtml = `
                <tr>
                    <th onclick="sortImages('name')" style="cursor: pointer;">Name ${sortIcon('name')}</th>
                    <th onclick="sortImages('filename')" style="cursor: pointer;">Filename ${sortIcon('filename')}</th>
                    <th onclick="sortImages('size')" style="cursor: pointer;">Size ${sortIcon('size')}</th>
                    <th onclick="sortImages('distro')" style="cursor: pointer;">Distro ${sortIcon('distro')}</th>
                    <th onclick="sortImages('group')" style="cursor: pointer;">Group ${sortIcon('group')}</th>
                    <th onclick="sortImages('status')" style="cursor: pointer;" class="col-dot" title="Enabled / Disabled">On ${sortIcon('status')}</th>
                    <th onclick="sortImages('visibility')" style="cursor: pointer;" class="col-dot" title="Public / Private">Pub ${sortIcon('visibility')}</th>
                    <th onclick="sortImages('boot_method')" style="cursor: pointer;">Boot Method ${sortIcon('boot_method')}</th>
                    <th>Operations</th>
                </tr>`;
        bodyHtml = sortedImages.map(img => imageRowHTML(img, true)).join('');
    } else {
        colspan = 8;
        theadHtml = `
                <tr>
                    <th onclick="sortImages('name')" style="cursor: pointer;">Name ${sortIcon('name')}</th>
                    <th onclick="sortImages('filename')" style="cursor: pointer;">Filename ${sortIcon('filename')}</th>
                    <th onclick="sortImages('size')" style="cursor: pointer;">Size ${sortIcon('size')}</th>
                    <th onclick="sortImages('distro')" style="cursor: pointer;">Distro ${sortIcon('distro')}</th>
                    <th onclick="sortImages('status')" style="cursor: pointer;" class="col-dot" title="Enabled / Disabled">On ${sortIcon('status')}</th>
                    <th onclick="sortImages('visibility')" style="cursor: pointer;" class="col-dot" title="Public / Private">Pub ${sortIcon('visibility')}</th>
                    <th onclick="sortImages('boot_method')" style="cursor: pointer;">Boot Method ${sortIcon('boot_method')}</th>
                    <th>Operations</th>
                </tr>`;

        const tree = _buildGroupTree(groups || []);
        const ownByGroup = new Map();
        const ungroupedImgs = [];
        for (const img of sortedImages) {
            const gid = img.group && img.group.id;
            if (gid != null && tree.byId.has(gid)) {
                if (!ownByGroup.has(gid)) ownByGroup.set(gid, []);
                ownByGroup.get(gid).push(img);
            } else {
                ungroupedImgs.push(img);
            }
        }

        const renderNode = (node, depth) => {
            const total = _countTreeImages(node, ownByGroup);
            if (total === 0) return ''; // hide empty branches (esp. when filtered)
            const own = ownByGroup.get(node.id) || [];
            const collapsed = !filterActive && collapsedImageGroups.has('id:' + node.id);
            const indent = depth * 18;
            const dataKey = _attrEscape(JSON.stringify('id:' + node.id));
            let html = `
                    <tr class="tr-group" data-group-key="${dataKey}" onclick="toggleImageGroupFromEvent(this)">
                        <td colspan="${colspan}" style="background: var(--bg-tertiary); cursor: pointer; user-select: none; padding-left: ${10 + indent}px;">
                            <span style="display: inline-block; width: 14px; color: var(--text-secondary);">${collapsed ? '▶' : '▼'}</span>
                            <strong style="color: var(--text-primary);">${escapeHtml(node.name)}</strong>
                            <span style="color: var(--text-muted); font-weight: 400; margin-left: 8px; font-size: 12px;">${total} ${total === 1 ? 'image' : 'images'}</span>
                        </td>
                    </tr>`;
            if (!collapsed) {
                for (const img of own) html += imageRowHTML(img, false, depth + 1);
                for (const child of node.children) html += renderNode(child, depth + 1);
            }
            return html;
        };

        let out = '';
        for (const root of tree.roots) out += renderNode(root, 0);

        if (ungroupedImgs.length > 0) {
            const collapsed = !filterActive && collapsedImageGroups.has('id:ungrouped');
            const dataKey = _attrEscape(JSON.stringify('id:ungrouped'));
            out += `
                    <tr class="tr-group" data-group-key="${dataKey}" onclick="toggleImageGroupFromEvent(this)">
                        <td colspan="${colspan}" style="background: var(--bg-tertiary); cursor: pointer; user-select: none;">
                            <span style="display: inline-block; width: 14px; color: var(--text-secondary);">${collapsed ? '▶' : '▼'}</span>
                            <strong style="color: var(--text-primary);">Ungrouped</strong>
                            <span style="color: var(--text-muted); font-weight: 400; margin-left: 8px; font-size: 12px;">${ungroupedImgs.length} ${ungroupedImgs.length === 1 ? 'image' : 'images'}</span>
                        </td>
                    </tr>`;
            if (!collapsed) {
                for (const img of ungroupedImgs) out += imageRowHTML(img, false, 1);
            }
        }
        bodyHtml = out;
    }

    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead>${theadHtml}</thead>
            <tbody>${bodyHtml}</tbody>
        </table>
        </div>
    `;
}

async function toggleImage(filename, currentState) {
    try {
        const res = await authFetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ enabled: !currentState })
        });

        const data = await res.json();
        if (data.success) {
            loadImages();
            loadStats();
        }
    } catch (err) {
        showAlert('Failed to update image', 'error');
    }
}

async function togglePublic(filename, currentState) {
    try {
        const res = await authFetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ public: !currentState })
        });

        const data = await res.json();
        if (data.success) {
            loadImages();
        }
    } catch (err) {
        showAlert('Failed to update image', 'error');
    }
}

async function deleteImage(filename, name) {
    if (!confirm(`Delete image ${name}?\n\nWARNING: This will permanently delete the ISO file from disk and remove it from the database.`)) return;

    try {
        const res = await authFetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}&delete_file=true`, { method: 'DELETE' });
        const data = await res.json();

        if (data.success) {
            showAlert('Image deleted successfully', 'success');
            loadImages();
            loadStats();
        } else {
            showAlert(data.error || 'Failed to delete image', 'error');
        }
    } catch (err) {
        showAlert('Failed to delete image', 'error');
    }
}

async function scanImages() {
    try {
        const res = await authFetch(`${API_BASE}/scan`, { method: 'POST' });
        const data = await res.json();

        if (data.success) {
            showAlert(data.message, 'success');
            loadImages();
            loadStats();
        } else {
            showAlert(data.error || 'Scan failed', 'error');
        }
    } catch (err) {
        showAlert('Failed to scan images', 'error');
    }
}

async function redetectFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    try {
        const res = await authFetch(`${API_BASE}/images/redetect?filename=${encodeURIComponent(filename)}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            const img = data.data;
            document.getElementById('image-props-boot-params').value = img.boot_params || '';
            document.getElementById('image-props-boot-params').placeholder = getDefaultBootParams(img) || 'Optional kernel parameters';
            document.getElementById('image-props-boot-method').value = img.boot_method || 'sanboot';
            showNotification(data.message || 'Re-detection complete', 'success');
            loadImages();
        } else {
            showNotification(data.error || 'Re-detection failed', 'error');
        }
    } catch (err) {
        showNotification('Re-detection failed', 'error');
    }
}

async function redetectImage(filename) {
    try {
        const res = await authFetch(`${API_BASE}/images/redetect?filename=${encodeURIComponent(filename)}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showNotification(data.message || 'Re-detection complete', 'success');
            loadImages();
        } else {
            showNotification(data.error || 'Re-detection failed', 'error');
        }
    } catch (err) {
        showNotification('Re-detection failed', 'error');
    }
}

function updateImagePropsProgress(filename) {
    const modal = document.getElementById('image-properties-modal');
    if (!modal || !modal.classList.contains('active')) return;
    const modalFilename = document.getElementById('image-props-filename').value;
    if (modalFilename !== filename) return;

    const container = document.getElementById('image-props-progress');
    const fill = document.getElementById('image-props-progress-fill');
    const text = document.getElementById('image-props-progress-text');
    const percent = document.getElementById('image-props-progress-percent');
    const p = extractionProgress[filename];

    const actionBtns = ['image-props-extract-btn', 'image-props-patch-smb-btn', 'image-props-netboot-btn', 'image-props-download-btn', 'image-props-delete-btn'];

    if (p) {
        container.style.display = '';
        fill.style.width = p.progress + '%';
        text.textContent = p.status;
        if (percent) percent.textContent = Math.round(p.progress) + '%';
        actionBtns.forEach(id => { const b = document.getElementById(id); if (b) b.disabled = true; });
    } else {
        container.style.display = 'none';
        actionBtns.forEach(id => { const b = document.getElementById(id); if (b) b.disabled = false; });
    }
}

function syncImagesProgress(filename) {
    renderImagesTable();
    updateImagePropsProgress(filename);
}

function closeImagePropsIfOpenFor(filename) {
    const modal = document.getElementById('image-properties-modal');
    if (!modal || !modal.classList.contains('active')) return;
    if (document.getElementById('image-props-filename').value !== filename) return;
    closeModal('image-properties-modal');
}

// Re-load the properties modal in place after an action that changed the
// image's state, keeping the user on whichever tab they were viewing.
function refreshImagePropsIfOpenFor(filename) {
    const modal = document.getElementById('image-properties-modal');
    if (!modal || !modal.classList.contains('active')) return;
    if (document.getElementById('image-props-filename').value !== filename) return;
    showImagePropertiesModal(filename, { preserveTab: true });
}

async function extractImage(filename, name) {
    if (!confirm(`Extract kernel and initrd from ${name}?\n\nThis will mount the ISO and extract boot files for direct kernel booting.`)) return;

    extractionProgress[filename] = { progress: 0, status: 'Starting extraction...' };
    syncImagesProgress(filename);

    const poll = setInterval(async () => {
        try {
            const r = await authFetch(`${API_BASE}/images/extract-progress?filename=${encodeURIComponent(filename)}`);
            const d = await r.json();
            if (!d.success || !d.data) return;
            const p = d.data;
            if (p.status === 'running' || p.status === 'done') {
                extractionProgress[filename] = {
                    progress: Math.max(1, Math.round(p.percent || 0)),
                    status: p.stage || t('props.action.extracting')
                };
                syncImagesProgress(filename);
            }
        } catch (e) { /* ignore poll errors */ }
    }, 500);

    try {
        const res = await authFetch(`${API_BASE}/images/extract?filename=${encodeURIComponent(filename)}`, { method: 'POST' });
        const data = await res.json();
        clearInterval(poll);

        if (data.success) {
            extractionProgress[filename] = { progress: 100, status: 'Complete!' };
            syncImagesProgress(filename);
            setTimeout(async () => {
                delete extractionProgress[filename];
                await loadImages();
                refreshImagePropsIfOpenFor(filename);
                showAlert(data.message || 'Extraction successful', 'success');
            }, 800);
        } else {
            delete extractionProgress[filename];
            syncImagesProgress(filename);
            showAlert(data.error || 'Extraction failed', 'error');
        }
    } catch (err) {
        clearInterval(poll);
        delete extractionProgress[filename];
        syncImagesProgress(filename);
        showAlert('Failed to extract image', 'error');
    }
}

async function downloadNetboot(filename, name) {
    if (!confirm(`Download netboot files for ${name}?\n\nThis will download and extract the proper network boot files required for Debian/Ubuntu network installation.`)) return;

    try {
        extractionProgress[filename] = { progress: 0, status: 'Downloading netboot...' };
        syncImagesProgress(filename);

        const progressInterval = setInterval(() => {
            if (extractionProgress[filename] && extractionProgress[filename].progress < 90) {
                extractionProgress[filename].progress += 10;
                if (extractionProgress[filename].progress < 30) {
                    extractionProgress[filename].status = 'Downloading tarball...';
                } else if (extractionProgress[filename].progress < 60) {
                    extractionProgress[filename].status = 'Extracting files...';
                } else {
                    extractionProgress[filename].status = 'Installing netboot files...';
                }
                syncImagesProgress(filename);
            }
        }, 500);

        const res = await authFetch(`${API_BASE}/images/netboot/download?filename=${encodeURIComponent(filename)}`, { method: 'POST' });
        const data = await res.json();

        clearInterval(progressInterval);

        if (data.success) {
            extractionProgress[filename] = { progress: 100, status: 'Complete!' };
            syncImagesProgress(filename);
            setTimeout(async () => {
                delete extractionProgress[filename];
                await loadImages();
                refreshImagePropsIfOpenFor(filename);
                showAlert(data.message || 'Netboot files downloaded successfully', 'success');
            }, 1000);
        } else {
            delete extractionProgress[filename];
            syncImagesProgress(filename);
            showAlert(data.error || 'Netboot download failed', 'error');
        }
    } catch (err) {
        delete extractionProgress[filename];
        syncImagesProgress(filename);
        showAlert('Failed to download netboot files', 'error');
    }
}

async function setBootMethod(filename, method) {
    try {
        const res = await authFetch(`${API_BASE}/images/boot-method`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                filename: filename,
                boot_method: method
            })
        });

        const data = await res.json();

        if (data.success) {
            showAlert(`Boot method set to ${method}`, 'success');
            loadImages();
        } else {
            showAlert(data.error || 'Failed to set boot method', 'error');
        }
    } catch (err) {
        showAlert('Failed to set boot method', 'error');
    }
}

async function cycleBootMethod(filename, currentMethod) {
    const cycle = {
        'sanboot': 'kernel',
        'kernel': 'nbd',
        'nbd': 'sanboot'
    };
    const nextMethod = cycle[currentMethod] || 'sanboot';
    await setBootMethod(filename, nextMethod);
}

// Boot Logs
async function loadLogs() {
    try {
        const res = await authFetch(`${API_BASE}/logs?limit=50`);
        const data = await res.json();

        if (data.success) {
            renderLogsTable(data.data || []);
        }
    } catch (err) {
        document.getElementById('logs-table').innerHTML = '<p class="alert alert-error">Failed to load logs</p>';
    }
}

function renderLogsTable(logs) {
    const container = document.getElementById('logs-table');

    if (logs.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No boot logs yet.</p>';
        return;
    }

    const html = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Time</th>
                    <th>MAC Address</th>
                    <th>Image</th>
                    <th>IP Address</th>
                    <th>Status</th>
                    <th>Error</th>
                </tr>
            </thead>
            <tbody>
                ${logs.map(log => `
                    <tr>
                        <td>${new Date(log.created_at).toLocaleString()}</td>
                        <td><code>${log.mac_address}</code></td>
                        <td>${log.image_name}</td>
                        <td>${log.ip_address || '-'}</td>
                        <td>
                            <span class="badge ${log.success ? 'badge-success' : 'badge-danger'}">
                                ${log.success ? 'Success' : 'Failed'}
                            </span>
                        </td>
                        <td>${log.error_msg || '-'}</td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
        </div>
    `;

    container.innerHTML = html;
}

// Forms
function setupForms() {
    document.getElementById('add-client-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(e.target);

        const client = {
            mac_address: formData.get('mac_address'),
            name: formData.get('name'),
            description: formData.get('description'),
            enabled: formData.get('enabled') === 'on'
        };

        try {
            const res = await authFetch(`${API_BASE}/clients`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(client)
            });

            const data = await res.json();
            if (data.success) {
                showAlert('Client created successfully', 'success');
                closeModal('add-client-modal');
                loadClients();
                loadStats();
            } else {
                showAlert(data.error || 'Failed to create client', 'error');
            }
        } catch (err) {
            showAlert('Failed to create client', 'error');
        }
    });

    document.getElementById('edit-client-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(e.target);
        const mac = formData.get('mac_address');

        const groupIdRaw = formData.get('client_group_id');
        const ipmiPortRaw = formData.get('ipmi_port');
        const updates = {
            name: formData.get('name'),
            description: formData.get('description'),
            enabled: formData.get('enabled') === 'on',
            show_public_images: formData.get('show_public_images') === 'on',
            bootloader_set: formData.get('bootloader_set') || '',
            client_group_id: groupIdRaw ? parseInt(groupIdRaw, 10) : null,
            ipmi_host: formData.get('ipmi_host') || '',
            ipmi_port: ipmiPortRaw ? parseInt(ipmiPortRaw, 10) : 0,
            ipmi_username: formData.get('ipmi_username') || '',
            ipmi_password: formData.get('ipmi_password') || '',
            ipmi_insecure: formData.get('ipmi_insecure') === 'on',
            auto_install_file: formData.get('auto_install_file') || '',
        };
        console.log('Updating client:', mac, updates);

        try {
            // Update client
            const res1 = await authFetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates)
            });

            // Update image assignments
            const selectedFilenames = Array.from(document.getElementById('edit-images-select').selectedOptions)
                .map(opt => opt.value);

            const res2 = await authFetch(`${API_BASE}/assign-images`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    mac_address: mac,
                    image_filenames: selectedFilenames
                })
            });

            const data1 = await res1.json();
            const data2 = await res2.json();

            if (data1.success && data2.success) {
                showAlert('Client updated successfully', 'success');
                closeModal('edit-client-modal');
                loadClients();
            } else {
                showAlert(data1.error || data2.error || 'Failed to update client', 'error');
            }
        } catch (err) {
            showAlert('Failed to update client', 'error');
        }
    });

    document.getElementById('theme-form').addEventListener('submit', saveTheme);
    document.getElementById('add-custom-tool-form').addEventListener('submit', createCustomTool);
    document.getElementById('add-profile-form').addEventListener('submit', createProfile);
    const wf = document.getElementById('webhook-form');
    if (wf) wf.addEventListener('submit', saveWebhookConfig);
}

// Theme
async function loadTheme() {
    try {
        const res = await authFetch(`${API_BASE}/theme`);
        const data = await res.json();
        if (data.success) {
            document.getElementById('theme-title').value = data.data.title || '';
            document.getElementById('theme-timeout').value = data.data.menu_timeout != null ? data.data.menu_timeout : 30;
        }
    } catch (err) {
        console.error('Failed to load theme:', err);
    }
}

async function saveTheme(e) {
    e.preventDefault();
    const theme = {
        title: document.getElementById('theme-title').value,
        menu_timeout: parseInt(document.getElementById('theme-timeout').value) || 0,
    };
    try {
        const res = await authFetch(`${API_BASE}/theme`, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(theme),
        });
        const data = await res.json();
        if (data.success) {
            showAlert('Theme saved successfully', 'success');
        } else {
            showAlert(data.error || 'Failed to save theme', 'error');
        }
    } catch (err) {
        showAlert('Failed to save theme', 'error');
    }
}

// Tools
// Distro Profiles
async function loadProfiles() {
    try {
        const res = await authFetch(`${API_BASE}/profiles`);
        const data = await res.json();
        const container = document.getElementById('profiles-list');

        if (!data.success) {
            container.innerHTML = `<p class="alert alert-error">${data.error || 'Failed to load profiles'}</p>`;
            return;
        }

        const profilesList = data.data || [];
        if (profilesList.length === 0) {
            container.innerHTML = '<p style="color: var(--text-secondary);">No profiles loaded.</p>';
            return;
        }

        let html = '<div class="table-scroll"><table><thead><tr><th>Name</th><th>Family</th><th>Filename Patterns</th><th>Boot Params</th><th>Type</th><th>Version</th><th>Actions</th></tr></thead><tbody>';

        for (const p of profilesList) {
            const patterns = (p.filename_patterns || []).join(', ');
            const params = p.default_boot_params || '<span style="color:var(--text-muted);">none</span>';
            const typeBadge = p.custom ?
                '<span class="badge badge-warning">Custom</span>' :
                '<span class="badge badge-info">Built-in</span>';

            html += `<tr>
                <td><strong>${escapeHtml(p.display_name)}</strong><br><code style="font-size:11px;color:var(--text-muted);">${escapeHtml(p.profile_id)}</code></td>
                <td>${escapeHtml(p.family || '-')}</td>
                <td style="font-size:12px;max-width:200px;overflow:hidden;text-overflow:ellipsis;" title="${escapeHtml(patterns)}">${escapeHtml(patterns)}</td>
                <td style="font-size:12px;max-width:250px;overflow:hidden;text-overflow:ellipsis;" title="${escapeHtml(p.default_boot_params || '')}">${params}</td>
                <td>${typeBadge}</td>
                <td style="font-size:12px;">${escapeHtml(p.version || '-')}</td>
                <td>${p.custom ? '<button class="btn btn-danger btn-sm" onclick="deleteProfile(\'' + escapeHtml(p.profile_id) + '\')">Delete</button>' : ''}</td>
            </tr>`;
        }

        html += '</tbody></table></div>';
        container.innerHTML = html;
    } catch (err) {
        document.getElementById('profiles-list').innerHTML = `<p class="alert alert-error">Failed to load profiles</p>`;
    }
}

async function createProfile(e) {
    e.preventDefault();
    const form = e.target;
    const splitTrim = (v) => v ? v.split(',').map(s => s.trim()).filter(s => s) : [];

    const data = {
        profile_id: form.profile_id.value.trim().toLowerCase().replace(/[^a-z0-9-]/g, '-'),
        display_name: form.display_name.value.trim(),
        family: form.family.value.trim(),
        filename_patterns: splitTrim(form.filename_patterns.value),
        kernel_paths: splitTrim(form.kernel_paths.value),
        initrd_paths: splitTrim(form.initrd_paths.value),
        squashfs_paths: splitTrim(form.squashfs_paths.value),
        default_boot_params: form.default_boot_params.value.trim(),
        boot_params_with_squashfs: form.boot_params_with_squashfs.value.trim(),
        auto_install_type: form.auto_install_type.value
    };

    try {
        const res = await authFetch(`${API_BASE}/profiles/save`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        });
        const result = await res.json();
        if (result.success) {
            showNotification('Profile created', 'success');
            closeModal('add-profile-modal');
            form.reset();
            loadProfiles();
        } else {
            showNotification(result.error || 'Failed to create profile', 'error');
        }
    } catch (err) {
        showNotification('Failed to create profile', 'error');
    }
}

async function deleteProfile(profileID) {
    if (!confirm(`Delete custom profile "${profileID}"?`)) return;
    try {
        const res = await authFetch(`${API_BASE}/profiles/delete?id=${encodeURIComponent(profileID)}`, { method: 'DELETE' });
        const data = await res.json();
        if (data.success) {
            showNotification('Profile deleted', 'success');
            loadProfiles();
        } else {
            showNotification(data.error || 'Failed to delete', 'error');
        }
    } catch (err) {
        showNotification('Failed to delete profile', 'error');
    }
}

async function updateProfilesFromRemote() {
    try {
        const res = await authFetch(`${API_BASE}/profiles/update`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showNotification(data.message || 'Profiles updated', 'success');
            loadProfiles();
        } else {
            showNotification(data.error || 'Update failed', 'error');
        }
    } catch (err) {
        showNotification('Update failed: ' + err.message, 'error');
    }
}

async function updateToolsFromRemote() {
    try {
        const res = await authFetch(`${API_BASE}/tools/update`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showNotification(data.message || 'Tools updated', 'success');
            loadTools();
        } else {
            showNotification(data.error || 'Update failed', 'error');
        }
    } catch (err) {
        showNotification('Update failed: ' + err.message, 'error');
    }
}

async function loadTools() {
    try {
        const res = await authFetch(`${API_BASE}/tools`);
        const data = await res.json();
        const container = document.getElementById('tools-list');

        if (!data.success) {
            container.innerHTML = `<p class="alert alert-error">${data.error || 'Failed to load tools'}</p>`;
            return;
        }

        const toolsList = data.data || [];
        if (toolsList.length === 0) {
            container.innerHTML = '<p style="color: var(--text-secondary);">No tools available.</p>';
            return;
        }

        let html = '';
        for (const tool of toolsList) {
            html += `<div style="background: var(--bg-tertiary); padding: 22px 24px; border-radius: var(--radius); margin-bottom: 14px;">`;

            // Header row
            html += `<div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; margin-bottom: 12px;">`;

            // Left: info
            html += `<div>`;
            html += `<div style="display: flex; align-items: center; gap: 10px; margin-bottom: 4px;">`;
            html += `<strong style="font-size: 16px;">${escapeHtml(tool.display_name)}</strong>`;
            html += `<span style="color: var(--text-muted); font-size: 13px;">v${escapeHtml(tool.version)}</span>`;
            if (tool.custom) html += `<span class="badge badge-warning">Custom</span>`;
            if (tool.enabled) html += `<span class="badge badge-success">Enabled</span>`;
            if (tool.downloaded) html += `<span class="badge badge-info">Downloaded</span>`;
            html += `</div>`;
            html += `<p style="color: var(--text-secondary); font-size: 14px; margin: 0;">${escapeHtml(tool.description)}</p>`;
            html += `</div>`;

            // Right: actions
            html += `<div style="display: flex; gap: 8px; align-items: center;">`;
            if (!tool.downloaded) {
                html += `<div id="tool-progress-container-${tool.name}">`;
                html += `<button class="btn btn-primary" id="tool-dl-btn-${tool.name}" onclick="downloadTool('${tool.name}')">Download</button>`;
                html += `<div id="tool-progress-wrap-${tool.name}" style="display:none; min-width: 250px;">`;
                html += `<div style="background: var(--border); border-radius: 4px; height: 8px; overflow: hidden; margin-bottom: 4px;"><div id="tool-progress-${tool.name}" style="height: 100%; width: 0%; background: var(--success); border-radius: 4px; transition: width 0.3s;"></div></div>`;
                html += `<span id="tool-progress-text-${tool.name}" style="font-size: 13px; color: var(--text-secondary);">Starting...</span>`;
                html += `</div></div>`;
            } else {
                if (tool.enabled) {
                    html += `<button class="btn btn-warning" onclick="toggleTool('${tool.name}', false)">Disable</button>`;
                } else {
                    html += `<button class="btn btn-success" onclick="toggleTool('${tool.name}', true)">Enable</button>`;
                }
                html += `<button class="btn btn-danger" onclick="deleteTool('${tool.name}')">Delete Files</button>`;
            }
            if (tool.custom) {
                html += `<button class="btn btn-danger" onclick="deleteCustomTool('${tool.name}')">Remove</button>`;
            }
            html += `</div></div>`;

            // Download URL row
            const urlId = `tool-url-${tool.name}`;
            const defaultUrl = tool.download_url || '';
            html += `<div style="display: flex; gap: 8px; align-items: center; margin-top: 8px;">`;
            html += `<input type="text" id="${urlId}" value="${escapeHtml(defaultUrl)}" placeholder="Download URL" style="flex: 1; font-size: 13px; padding: 8px 12px; font-family: monospace;">`;
            html += `<button class="btn btn-sm" onclick="updateToolURL('${tool.name}', '${urlId}')">Save URL</button>`;
            html += `</div>`;

            html += `</div>`;
        }

        container.innerHTML = html;
    } catch (err) {
        document.getElementById('tools-list').innerHTML = `<p class="alert alert-error">Failed to load tools: ${err.message}</p>`;
    }
}

async function downloadTool(name) {
    try {
        const res = await authFetch(`${API_BASE}/tools/download?name=${encodeURIComponent(name)}`, { method: 'POST' });
        const data = await res.json();
        if (!data.success) {
            showNotification(data.error || 'Download failed', 'error');
            return;
        }

        // Show progress bar, hide button
        const btn = document.getElementById(`tool-dl-btn-${name}`);
        const wrap = document.getElementById(`tool-progress-wrap-${name}`);
        if (btn) btn.style.display = 'none';
        if (wrap) wrap.style.display = 'block';

        // Poll progress
        const poll = setInterval(async () => {
            try {
                const r = await authFetch(`${API_BASE}/tools/progress?name=${encodeURIComponent(name)}`);
                const d = await r.json();
                if (!d.success) return;

                const p = d.data;
                const bar = document.getElementById(`tool-progress-${name}`);
                const text = document.getElementById(`tool-progress-text-${name}`);
                if (!bar || !text) return;

                if (p.status === 'downloading') {
                    bar.style.width = p.percent.toFixed(0) + '%';
                    const dlMB = (p.downloaded / 1048576).toFixed(1);
                    const totalMB = p.total > 0 ? (p.total / 1048576).toFixed(1) : '?';
                    text.textContent = `Downloading... ${dlMB} MB / ${totalMB} MB (${p.percent.toFixed(0)}%)`;
                } else if (p.status === 'extracting') {
                    bar.style.width = '100%';
                    text.textContent = 'Extracting...';
                } else if (p.status === 'done') {
                    clearInterval(poll);
                    showNotification('Download complete', 'success');
                    loadTools();
                } else if (p.status === 'error') {
                    clearInterval(poll);
                    showNotification('Download failed: ' + (p.error || 'unknown error'), 'error');
                    loadTools();
                }
            } catch (e) { /* ignore poll errors */ }
        }, 1000);
    } catch (err) {
        showNotification('Download failed: ' + err.message, 'error');
    }
}

async function toggleTool(name, enabled) {
    try {
        const res = await authFetch(`${API_BASE}/tools/toggle`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, enabled })
        });
        const data = await res.json();
        if (data.success) {
            showNotification(data.message, 'success');
            loadTools();
        } else {
            showNotification(data.error || 'Failed', 'error');
        }
    } catch (err) {
        showNotification('Failed: ' + err.message, 'error');
    }
}

async function deleteTool(name) {
    if (!confirm(`Delete downloaded files for ${name}? You can re-download later.`)) return;
    try {
        const res = await authFetch(`${API_BASE}/tools/delete?name=${encodeURIComponent(name)}`, { method: 'DELETE' });
        const data = await res.json();
        if (data.success) {
            showNotification('Tool files deleted', 'success');
            loadTools();
        } else {
            showNotification(data.error || 'Delete failed', 'error');
        }
    } catch (err) {
        showNotification('Failed: ' + err.message, 'error');
    }
}

async function createCustomTool(e) {
    e.preventDefault();
    const form = e.target;
    const data = {
        name: form.name.value.trim().toLowerCase().replace(/[^a-z0-9-]/g, '-'),
        display_name: form.display_name.value.trim(),
        description: form.description.value.trim(),
        download_url: form.download_url.value.trim(),
        boot_method: form.boot_method.value,
        archive_type: form.archive_type.value,
        kernel_path: form.kernel_path.value.trim(),
        initrd_path: form.initrd_path.value.trim(),
        boot_params: form.boot_params.value.trim(),
        version: form.version.value.trim()
    };

    if (!data.name || !data.display_name || !data.download_url) {
        showNotification('Name, display name, and download URL are required', 'error');
        return;
    }

    try {
        const res = await authFetch(`${API_BASE}/tools/custom`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        });
        const result = await res.json();
        if (result.success) {
            showNotification('Custom tool created', 'success');
            closeModal('add-custom-tool-modal');
            form.reset();
            loadTools();
        } else {
            showNotification(result.error || 'Failed to create tool', 'error');
        }
    } catch (err) {
        showNotification('Failed to create tool', 'error');
    }
}

async function deleteCustomTool(name) {
    if (!confirm(`Delete custom tool "${name}"? This removes the tool and all its files.`)) return;
    try {
        const res = await authFetch(`${API_BASE}/tools/custom/delete?name=${encodeURIComponent(name)}`, { method: 'DELETE' });
        const data = await res.json();
        if (data.success) {
            showNotification('Custom tool removed', 'success');
            loadTools();
        } else {
            showNotification(data.error || 'Failed to delete tool', 'error');
        }
    } catch (err) {
        showNotification('Failed to delete tool', 'error');
    }
}

async function updateToolURL(name, inputId) {
    const url = document.getElementById(inputId).value.trim();
    if (!url) {
        showNotification('URL cannot be empty', 'error');
        return;
    }
    try {
        const res = await authFetch(`${API_BASE}/tools/url`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, url })
        });
        const data = await res.json();
        if (data.success) {
            showNotification('Download URL updated', 'success');
        } else {
            showNotification(data.error || 'Failed', 'error');
        }
    } catch (err) {
        showNotification('Failed: ' + err.message, 'error');
    }
}

// Bootloaders
async function loadBootloaders() {
    try {
        const res = await authFetch(`${API_BASE}/bootloaders`);
        const data = await res.json();
        const container = document.getElementById('bootloaders-table');

        if (!data.success) {
            container.innerHTML = `<p class="alert alert-error">${data.error || 'Failed to load bootloaders'}</p>`;
            return;
        }

        const sets = (data.data && data.data.sets) || [];
        const activeSet = (data.data && data.data.active) || 'built-in';

        if (sets.length === 0) {
            container.innerHTML = '<p style="color: var(--text-secondary);">No bootloaders found.</p>';
            return;
        }

        let html = '';

        for (const set of sets) {
            const isActive = set.name === activeSet;
            const isBuiltIn = set.name === 'built-in';
            const escapedName = escapeHtml(set.name);
            const files = set.files || [];

            html += `<div style="background: var(--bg-tertiary); padding: 20px 24px; border-radius: var(--radius); margin-bottom: 14px; border: 2px solid ${isActive ? 'var(--success)' : 'transparent'};">`;

            // Header row
            html += `<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px;">`;
            html += `<div style="display: flex; align-items: center; gap: 12px;">`;
            html += `<input type="radio" name="active-bootloader-set" value="${escapedName}" ${isActive ? 'checked' : ''} onchange="selectBootloaderSet('${escapedName}')" style="width: auto; accent-color: var(--success); transform: scale(1.3);">`;
            html += `<div>`;
            html += `<strong style="font-size: 16px;">${escapedName}</strong>`;
            if (isActive) html += ` <span class="badge badge-success">Active</span>`;
            if (isBuiltIn) html += ` <span class="badge badge-info">Bundled</span>`;
            html += `<div style="color: var(--text-secondary); font-size: 13px; margin-top: 2px;">${files.length} file${files.length !== 1 ? 's' : ''}</div>`;
            html += `</div></div>`;

            // Actions
            html += `<div style="display: flex; gap: 6px;">`;
            if (!isBuiltIn) {
                html += `<button class="btn btn-sm btn-primary" onclick="showUploadBootloaderFilesModal('${escapedName}')">Upload Files</button>`;
                html += `<button class="btn btn-sm btn-danger" onclick="deleteBootloaderSet('${escapedName}')">Delete Set</button>`;
            }
            html += `</div>`;
            html += `</div>`;

            // File list
            if (files.length > 0) {
                html += `<div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 8px;">`;
                for (const file of files) {
                    html += `<div style="background: var(--bg-secondary); padding: 10px 14px; border-radius: var(--radius-sm); display: flex; justify-content: space-between; align-items: center; font-size: 14px;">`;
                    html += `<span>${escapeHtml(file.name)}</span>`;
                    html += `<div style="display: flex; align-items: center; gap: 10px;">`;
                    html += `<span style="color: var(--text-muted); font-size: 13px;">${formatBytes(file.size)}</span>`;
                    if (!isBuiltIn) {
                        html += `<button class="btn btn-danger btn-sm" style="padding: 2px 8px; font-size: 11px;" onclick="deleteBootloaderFile('${escapedName}', '${escapeHtml(file.name)}')">✕</button>`;
                    }
                    html += `</div>`;
                    html += `</div>`;
                }
                html += `</div>`;
            } else if (!isBuiltIn) {
                html += `<p style="color: var(--text-muted); font-size: 13px; margin-top: 4px;">No files yet. Upload bootloader files or place them in <code>data/bootloaders/${escapedName}/</code>.</p>`;
            }

            html += `</div>`;
        }

        container.innerHTML = html;
    } catch (err) {
        document.getElementById('bootloaders-table').innerHTML = `<p class="alert alert-error">Failed to load bootloaders: ${err.message}</p>`;
    }
}

async function selectBootloaderSet(setName) {
    try {
        const res = await authFetch(`${API_BASE}/bootloaders/select`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ set: setName })
        });
        const data = await res.json();
        if (data.success) {
            showNotification(`Active bootloader set: ${setName}`, 'success');
            loadBootloaders();
        } else {
            showNotification(data.error || 'Failed', 'error');
        }
    } catch (err) {
        showNotification('Failed: ' + err.message, 'error');
    }
}

function showCreateBootloaderSetModal() {
    document.getElementById('bootloader-set-name').value = '';
    openModal('create-bootloader-set-modal');
}

async function createBootloaderSet(event) {
    event.preventDefault();
    const nameInput = document.getElementById('bootloader-set-name');
    const setName = nameInput.value.trim();
    if (!setName) return;

    try {
        const res = await authFetch(`${API_BASE}/bootloaders/create`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: setName })
        });
        const data = await res.json();
        if (data.success) {
            showNotification(`Set "${setName}" created`, 'success');
            closeModal('create-bootloader-set-modal');
            loadBootloaders();
        } else {
            showNotification(data.error || 'Failed to create set', 'error');
        }
    } catch (err) {
        showNotification('Failed: ' + err.message, 'error');
    }
}

function showUploadBootloaderFilesModal(setName) {
    document.getElementById('upload-bl-set-name').textContent = setName;
    document.getElementById('upload-bl-set-value').value = setName;
    document.getElementById('bootloader-files-upload').value = '';
    openModal('upload-bootloader-files-modal');
}

async function uploadBootloaderFiles(event) {
    event.preventDefault();
    const fileInput = document.getElementById('bootloader-files-upload');
    const setName = document.getElementById('upload-bl-set-value').value;
    if (!fileInput.files.length || !setName) return;

    const formData = new FormData();
    formData.append('set', setName);
    for (const file of fileInput.files) {
        formData.append('files', file);
    }

    try {
        const res = await authFetch(`${API_BASE}/bootloaders/upload`, { method: 'POST', body: formData });
        const data = await res.json();
        if (data.success) {
            showNotification(data.message || 'Files uploaded', 'success');
            closeModal('upload-bootloader-files-modal');
            loadBootloaders();
        } else {
            showNotification(data.error || 'Upload failed', 'error');
        }
    } catch (err) {
        showNotification('Upload failed: ' + err.message, 'error');
    }
}

async function deleteBootloaderSet(setName) {
    if (!confirm(`Delete the entire "${setName}" bootloader set and all its files?`)) return;

    try {
        const res = await authFetch(`${API_BASE}/bootloaders/delete?set=${encodeURIComponent(setName)}`, { method: 'DELETE' });
        const data = await res.json();
        if (data.success) {
            showNotification(`Set "${setName}" deleted`, 'success');
            loadBootloaders();
        } else {
            showNotification(data.error || 'Delete failed', 'error');
        }
    } catch (err) {
        showNotification('Failed: ' + err.message, 'error');
    }
}

async function deleteBootloaderFile(setName, fileName) {
    if (!confirm(`Delete "${fileName}" from set "${setName}"?`)) return;

    try {
        const res = await authFetch(`${API_BASE}/bootloaders/delete?set=${encodeURIComponent(setName)}&name=${encodeURIComponent(fileName)}`, { method: 'DELETE' });
        const data = await res.json();
        if (data.success) {
            showNotification(`Deleted ${fileName}`, 'success');
            loadBootloaders();
        } else {
            showNotification(data.error || 'Delete failed', 'error');
        }
    } catch (err) {
        showNotification('Failed: ' + err.message, 'error');
    }
}

// USB Images
// Pulls the file via authFetch (Bearer token) and triggers a download.
// Plain <a href> can't carry the auth header so direct navigation 401s.
async function downloadUSBImage(name) {
    try {
        const res = await authFetch(`${API_BASE}/usb/download?name=${encodeURIComponent(name)}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = name;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
    } catch (err) {
        showNotification('Failed to download ' + name + ': ' + err.message, 'error');
    }
}

async function loadUSBImages() {
    try {
        const res = await authFetch(`${API_BASE}/usb`);
        const data = await res.json();
        const container = document.getElementById('usb-images-content');

        if (!data.success || !data.data || data.data.length === 0) {
            container.innerHTML = '<p style="color: var(--text-secondary);">No USB boot images available.</p>';
            return;
        }

        let html = '<div class="table-scroll"><table><thead><tr><th>Image</th><th>Size</th><th>Type</th><th>Action</th></tr></thead><tbody>';
        for (const img of data.data) {
            const size = formatBytes(img.size);
            const isSecureBoot = img.name.includes('secureboot');
            const type = isSecureBoot ? 'UEFI Secure Boot' : 'BIOS / UEFI';
            html += `<tr>
                <td>${escapeHtml(img.name)}</td>
                <td>${size}</td>
                <td>${type}</td>
                <td><button type="button" class="btn btn-sm btn-primary" onclick="downloadUSBImage('${escapeHtml(img.name).replace(/'/g, "\\'")}')">Download</button></td>
            </tr>`;
        }
        html += '</tbody></table></div>';
        html += `<div style="margin-top: 15px; padding: 15px; background: var(--bg-secondary); border-radius: 8px; color: var(--text-secondary); font-size: 13px;">
            <strong style="color: var(--accent);">Writing to USB:</strong><br>
            <code style="color: var(--text-primary);">sudo dd if=bootimus.usb of=/dev/sdX bs=4M status=progress</code><br><br>
            Replace <code>/dev/sdX</code> with your USB device. The USB boots iPXE which uses DHCP to find bootimus automatically.
        </div>`;
        container.innerHTML = html;
    } catch (err) {
        document.getElementById('usb-images-content').innerHTML = '<p style="color: #ef4444;">Failed to load USB images</p>';
    }
}

// Upload
function setupUpload() {
    const area = document.getElementById('upload-area');
    const input = document.getElementById('file-input');
    const fileNameDisplay = document.getElementById('file-name');

    area.addEventListener('click', () => input.click());

    input.addEventListener('change', (e) => {
        if (e.target.files.length > 0) {
            fileNameDisplay.textContent = `Selected: ${e.target.files[0].name} (${formatBytes(e.target.files[0].size)})`;
        }
    });

    area.addEventListener('dragover', (e) => {
        e.preventDefault();
        area.classList.add('dragging');
    });

    area.addEventListener('dragleave', () => {
        area.classList.remove('dragging');
    });

    area.addEventListener('drop', (e) => {
        e.preventDefault();
        area.classList.remove('dragging');

        if (e.dataTransfer.files.length > 0) {
            input.files = e.dataTransfer.files;
            fileNameDisplay.textContent = `Selected: ${e.dataTransfer.files[0].name} (${formatBytes(e.dataTransfer.files[0].size)})`;
        }
    });

    document.getElementById('upload-form').addEventListener('submit', async (e) => {
        e.preventDefault();

        const formData = new FormData(e.target);
        const file = formData.get('file');

        if (!file || file.size === 0) {
            showAlert('Please select a file', 'error');
            return;
        }

        // Show progress indicator
        const submitBtn = e.target.querySelector('button[type="submit"]');
        const originalBtnText = submitBtn.textContent;
        submitBtn.disabled = true;
        submitBtn.textContent = 'Uploading...';

        // Add progress message
        const progressMsg = document.createElement('div');
        progressMsg.className = 'alert alert-info';
        progressMsg.id = 'upload-progress';
        progressMsg.innerHTML = `
            <div>Uploading ${file.name} (${formatBytes(file.size)})</div>
            <div style="margin-top: 10px;">
                <div style="background: var(--bg-primary); height: 20px; border-radius: 10px; overflow: hidden;">
                    <div id="progress-bar" style="background: #38bdf8; height: 100%; width: 0%; transition: width 0.3s;"></div>
                </div>
                <div id="progress-text" style="margin-top: 5px; text-align: center;">Starting upload...</div>
            </div>
        `;
        e.target.insertBefore(progressMsg, submitBtn);

        try {
            const xhr = new XMLHttpRequest();

            // Track upload progress
            xhr.upload.addEventListener('progress', (event) => {
                if (event.lengthComputable) {
                    const percentComplete = (event.loaded / event.total) * 100;
                    const progressBar = document.getElementById('progress-bar');
                    const progressText = document.getElementById('progress-text');
                    if (progressBar && progressText) {
                        progressBar.style.width = percentComplete + '%';
                        progressText.textContent = `${Math.round(percentComplete)}% - ${formatBytes(event.loaded)} / ${formatBytes(event.total)}`;
                    }
                }
            });

            // Handle completion
            const uploadPromise = new Promise((resolve, reject) => {
                xhr.addEventListener('load', () => {
                    if (xhr.status >= 200 && xhr.status < 300) {
                        resolve(JSON.parse(xhr.responseText));
                    } else {
                        reject(new Error(`Upload failed with status ${xhr.status}`));
                    }
                });
                xhr.addEventListener('error', () => reject(new Error('Upload failed')));
                xhr.addEventListener('abort', () => reject(new Error('Upload cancelled')));
            });

            xhr.open('POST', `${API_BASE}/images/upload`);
            const token = getToken();
            if (token) xhr.setRequestHeader('Authorization', 'Bearer ' + token);
            xhr.send(formData);

            const data = await uploadPromise;

            if (data.success) {
                showAlert('Image uploaded successfully', 'success');
                closeModal('upload-modal');
                loadImages();
                loadStats();
                e.target.reset();
                fileNameDisplay.textContent = '';
            } else {
                showAlert(data.error || 'Upload failed', 'error');
            }
        } catch (err) {
            showAlert('Failed to upload image: ' + err.message, 'error');
        } finally {
            // Clean up progress UI
            const progress = document.getElementById('upload-progress');
            if (progress) {
                progress.remove();
            }
            submitBtn.disabled = false;
            submitBtn.textContent = originalBtnText;
        }
    });
}

function showUploadModal() {
    document.getElementById('upload-form').reset();
    document.getElementById('file-name').textContent = '';
    showModal('upload-modal');
}

// Utilities
function showModal(id) {
    document.getElementById(id).classList.add('active');
}

function closeModal(id) {
    document.getElementById(id).classList.remove('active');
}

function showAlert(message, type) {
    // Create notification container if it doesn't exist
    let container = document.getElementById('notification-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'notification-container';
        container.className = 'notification-container';
        document.body.appendChild(container);
    }

    const alertDiv = document.createElement('div');
    alertDiv.className = `notification notification-${type}`;
    alertDiv.textContent = message;

    // Add to container
    container.appendChild(alertDiv);

    // Trigger animation
    setTimeout(() => alertDiv.classList.add('show'), 10);

    // Auto-remove after 5 seconds
    setTimeout(() => {
        alertDiv.classList.remove('show');
        setTimeout(() => alertDiv.remove(), 300);
    }, 5000);

    // Click to dismiss
    alertDiv.addEventListener('click', () => {
        alertDiv.classList.remove('show');
        setTimeout(() => alertDiv.remove(), 300);
    });
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

// Server Logs Viewer
let logsRefreshInterval = null;
let autoScrollEnabled = true;

function loadServerLogs() {
    authFetch('/api/logs/buffer')
        .then(response => response.json())
        .then(data => {
            if (data.success && data.logs) {
                displayLogs(data.logs);
            }
        })
        .catch(error => {
            console.error('Failed to load logs:', error);
        });
}

function displayLogs(logs) {
    const liveLogsDiv = document.getElementById('live-logs');
    const wasScrolledToBottom = liveLogsDiv.scrollHeight - liveLogsDiv.clientHeight <= liveLogsDiv.scrollTop + 1;

    liveLogsDiv.innerHTML = '';

    if (logs.length === 0) {
        liveLogsDiv.innerHTML = '<div style="color: var(--text-secondary);">No logs available. Logs will appear here as the server runs.</div>';
        return;
    }

    logs.forEach(log => {
        const logEntry = document.createElement('div');
        logEntry.style.marginBottom = '2px';
        logEntry.style.wordBreak = 'break-all';
        // Colour-code by content
        const lower = log.toLowerCase();
        if (lower.includes('error') || lower.includes('failed')) {
            logEntry.style.color = '#ef4444';
        } else if (lower.includes('warn')) {
            logEntry.style.color = '#f59e0b';
        } else if (lower.includes('success') || lower.includes('ready')) {
            logEntry.style.color = '#10b981';
        } else {
            logEntry.style.color = '#d0d0d0';
        }
        logEntry.textContent = log;
        liveLogsDiv.appendChild(logEntry);
    });

    // Auto-scroll to bottom if user was already at bottom or auto-scroll is enabled
    if (autoScrollEnabled || wasScrolledToBottom) {
        liveLogsDiv.scrollTop = liveLogsDiv.scrollHeight;
    }
}

function connectLiveLogs() {
    // Immediately load logs
    loadServerLogs();

    // Start auto-refresh every 2 seconds
    if (!logsRefreshInterval) {
        logsRefreshInterval = setInterval(loadServerLogs, 2000);
    }

    // Update UI
    const statusSpan = document.getElementById('log-status');
    const connectBtn = document.getElementById('connect-logs-btn');
    const disconnectBtn = document.getElementById('disconnect-logs-btn');

    statusSpan.textContent = 'Auto-refreshing (every 2s)';
    statusSpan.style.color = '#10b981';
    connectBtn.style.display = 'none';
    disconnectBtn.style.display = 'inline-block';
}

function disconnectLiveLogs() {
    if (logsRefreshInterval) {
        clearInterval(logsRefreshInterval);
        logsRefreshInterval = null;
    }

    const statusSpan = document.getElementById('log-status');
    const connectBtn = document.getElementById('connect-logs-btn');
    const disconnectBtn = document.getElementById('disconnect-logs-btn');

    statusSpan.textContent = 'Stopped';
    statusSpan.style.color = '#94a3b8';
    connectBtn.style.display = 'inline-block';
    disconnectBtn.style.display = 'none';
}

function clearLiveLogs() {
    document.getElementById('live-logs').innerHTML = '<div style="color: var(--text-secondary);">Click "Refresh" to load logs...</div>';
}

// ==================== User Management ====================

function loadUsers() {
    authFetch('/api/users')
        .then(response => response.json())
        .then(data => {
            if (data.success) {
                lastLoadedUsers = data.data || [];
                renderUsersTable();
            } else {
                document.getElementById('users-table').innerHTML =
                    `<div class="error">Error loading users: ${data.error}</div>`;
            }
        })
        .catch(error => {
            document.getElementById('users-table').innerHTML =
                `<div class="error">Error loading users: ${error.message}</div>`;
        });
}

let lastLoadedUsers = [];
function renderUsersTable(users) {
    if (users === undefined) users = lastLoadedUsers;
    if (!users || users.length === 0) {
        document.getElementById('users-table').innerHTML =
            '<p style="color: var(--text-secondary);">No users found</p>';
        return;
    }
    users = users.filter(u => rowMatchesFilter('users', [u.username, u.email, u.description]));

    let html = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Username</th>
                    <th>Role</th>
                    <th>Status</th>
                    <th>Last Login</th>
                    <th>Created</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
    `;

    users.forEach(user => {
        const role = user.is_admin ? '<span class="badge badge-info">Admin</span>' : '<span class="badge badge-success">User</span>';
        const status = user.enabled ? '<span class="badge badge-success">Enabled</span>' : '<span class="badge badge-danger">Disabled</span>';
        const lastLogin = user.last_login ? new Date(user.last_login).toLocaleString() : 'Never';
        const created = new Date(user.created_at).toLocaleString();

        html += `
            <tr>
                <td><strong>${escapeHtml(user.username)}</strong></td>
                <td>${role}</td>
                <td>${status}</td>
                <td>${lastLogin}</td>
                <td>${created}</td>
                <td>
                    <button class="btn btn-info btn-sm" onclick='editUser(${JSON.stringify(user)})'>Edit</button>
                    <button class="btn btn-sm" onclick='showResetPasswordModal(${JSON.stringify(user)})'>Reset Password</button>
                    ${user.username !== 'admin' ? `<button class="btn btn-danger btn-sm" onclick="deleteUser('${user.username}')">Delete</button>` : ''}
                </td>
            </tr>
        `;
    });

    html += '</tbody></table></div>';
    document.getElementById('users-table').innerHTML = html;
}

function showAddUserModal() {
    document.getElementById('add-user-form').reset();
    openModal('add-user-modal');
}

function editUser(user) {
    const form = document.getElementById('edit-user-form');
    form.elements['id'].value = user.id;
    form.elements['username'].value = user.username;
    form.elements['is_admin'].checked = user.is_admin;
    form.elements['enabled'].checked = user.enabled;

    // Lock the admin/enabled toggles if this is the only active admin —
    // demoting or disabling them would lock everyone out of the system.
    const otherActiveAdmins = (lastLoadedUsers || []).filter(u =>
        u.username !== user.username && u.is_admin && u.enabled
    ).length;
    const lockOut = user.is_admin && user.enabled && otherActiveAdmins === 0;
    const lockTitle = lockOut ? 'This is the only active admin — at least one must remain.' : '';
    form.elements['is_admin'].disabled = lockOut;
    form.elements['is_admin'].title = lockTitle;
    form.elements['enabled'].disabled = lockOut;
    form.elements['enabled'].title = lockTitle;

    openModal('edit-user-modal');
}

function showResetPasswordModal(user) {
    const form = document.getElementById('reset-password-form');
    form.elements['username'].value = user.username;
    form.elements['username_display'].value = user.username;
    form.elements['password'].value = '';
    openModal('reset-password-modal');
}

function deleteUser(username) {
    if (!confirm(`Are you sure you want to delete user "${username}"?`)) {
        return;
    }

    authFetch(`/api/users?username=${encodeURIComponent(username)}`, {
        method: 'DELETE'
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showNotification('User deleted successfully', 'success');
            loadUsers();
        } else {
            showNotification(data.error || 'Failed to delete user', 'error');
        }
    })
    .catch(error => {
        showNotification('Error: ' + error.message, 'error');
    });
}

// Form submission handlers
document.getElementById('add-user-form').addEventListener('submit', function(e) {
    e.preventDefault();
    const formData = new FormData(e.target);

    const userData = {
        username: formData.get('username'),
        password: formData.get('password'),
        is_admin: formData.get('is_admin') === 'on',
        enabled: formData.get('enabled') === 'on'
    };

    authFetch('/api/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(userData)
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showNotification('User created successfully', 'success');
            closeModal('add-user-modal');
            loadUsers();
        } else {
            showNotification(data.error || 'Failed to create user', 'error');
        }
    })
    .catch(error => {
        showNotification('Error: ' + error.message, 'error');
    });
});

document.getElementById('edit-user-form').addEventListener('submit', function(e) {
    e.preventDefault();
    const formData = new FormData(e.target);

    const username = formData.get('username');
    const userData = {
        is_admin: formData.get('is_admin') === 'on',
        enabled: formData.get('enabled') === 'on'
    };

    authFetch(`/api/users?username=${encodeURIComponent(username)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(userData)
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showNotification('User updated successfully', 'success');
            closeModal('edit-user-modal');
            loadUsers();
        } else {
            showNotification(data.error || 'Failed to update user', 'error');
        }
    })
    .catch(error => {
        showNotification('Error: ' + error.message, 'error');
    });
});

document.getElementById('reset-password-form').addEventListener('submit', function(e) {
    e.preventDefault();
    const formData = new FormData(e.target);

    const resetData = {
        username: formData.get('username'),
        new_password: formData.get('password')
    };

    authFetch('/api/users/reset-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(resetData)
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showNotification('Password reset successfully', 'success');
            closeModal('reset-password-modal');
        } else {
            showNotification(data.error || 'Failed to reset password', 'error');
        }
    })
    .catch(error => {
        showNotification('Error: ' + error.message, 'error');
    });
});

document.getElementById('add-group-form').addEventListener('submit', async function(e) {
    e.preventDefault();
    const formData = new FormData(e.target);

    const groupData = {
        name: formData.get('name'),
        description: formData.get('description'),
        parent_id: formData.get('parent_id') ? parseInt(formData.get('parent_id')) : null,
        order: parseInt(formData.get('order')) || 0,
        enabled: formData.get('enabled') === 'on'
    };

    try {
        const res = await authFetch(`${API_BASE}/groups`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(groupData)
        });

        const data = await res.json();

        if (data.success) {
            showNotification('Group created successfully', 'success');
            closeModal('add-group-modal');
            loadGroups();
        } else {
            showNotification(data.error || 'Failed to create group', 'error');
        }
    } catch (error) {
        showNotification('Error: ' + error.message, 'error');
    }
});

document.getElementById('edit-group-form').addEventListener('submit', async function(e) {
    e.preventDefault();
    const formData = new FormData(e.target);

    const groupData = {
        id: parseInt(formData.get('id')),
        name: formData.get('name'),
        description: formData.get('description'),
        parent_id: formData.get('parent_id') ? parseInt(formData.get('parent_id')) : null,
        order: parseInt(formData.get('order')) || 0,
        enabled: formData.get('enabled') === 'on'
    };

    try {
        const res = await authFetch(`${API_BASE}/groups/update?id=${groupData.id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(groupData)
        });

        const data = await res.json();

        if (data.success) {
            showNotification('Group updated successfully', 'success');
            closeModal('edit-group-modal');
            loadGroups();
            loadImages();
        } else {
            showNotification(data.error || 'Failed to update group', 'error');
        }
    } catch (error) {
        showNotification('Error: ' + error.message, 'error');
    }
});

// ==================== ISO Download Management ====================

let downloadProgressInterval = null;

function showDownloadModal() {
    document.getElementById('download-form').reset();
    document.getElementById('download-progress-container').style.display = 'none';
    document.getElementById('download-submit-btn').disabled = false;
    if (downloadProgressInterval) {
        clearInterval(downloadProgressInterval);
        downloadProgressInterval = null;
    }
    openModal('download-modal');
}

document.getElementById('download-form').addEventListener('submit', function(e) {
    e.preventDefault();
    const formData = new FormData(e.target);

    const downloadData = {
        url: formData.get('url'),
        description: formData.get('description')
    };

    // Disable submit button
    document.getElementById('download-submit-btn').disabled = true;
    document.getElementById('download-progress-container').style.display = 'block';

    authFetch('/api/images/download', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(downloadData)
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showNotification('Download started: ' + data.data.filename, 'success');

            // Start polling for progress
            const filename = data.data.filename;
            downloadProgressInterval = setInterval(() => {
                checkDownloadProgress(filename);
            }, 1000);
        } else {
            showNotification(data.error || 'Failed to start download', 'error');
            document.getElementById('download-submit-btn').disabled = false;
            document.getElementById('download-progress-container').style.display = 'none';
        }
    })
    .catch(error => {
        showNotification('Error: ' + error.message, 'error');
        document.getElementById('download-submit-btn').disabled = false;
        document.getElementById('download-progress-container').style.display = 'none';
    });
});

function checkDownloadProgress(filename) {
    authFetch('/api/downloads/progress?filename=' + encodeURIComponent(filename))
        .then(response => response.json())
        .then(data => {
            if (data.success && data.data) {
                const progress = data.data;
                const progressBar = document.getElementById('download-progress-bar');
                const progressText = document.getElementById('download-progress-text');

                progressBar.style.width = progress.percentage.toFixed(1) + '%';
                progressText.textContent = progress.percentage.toFixed(1) + '% - ' + (progress.speed || '0 B/s');

                if (progress.status === 'completed') {
                    clearInterval(downloadProgressInterval);
                    downloadProgressInterval = null;
                    showNotification('Download completed: ' + filename, 'success');
                    closeModal('download-modal');
                    loadImages(); // Refresh images list
                } else if (progress.status === 'error') {
                    clearInterval(downloadProgressInterval);
                    downloadProgressInterval = null;
                    showNotification('Download failed: ' + (progress.error || 'Unknown error'), 'error');
                    document.getElementById('download-submit-btn').disabled = false;
                }
            }
        })
        .catch(error => {
            console.error('Failed to check download progress:', error);
        });
}

// Auto-Install Script Management
async function showAutoInstallModal(filename, name) {
    document.getElementById('autoinstall-image-filename').value = filename;
    document.getElementById('autoinstall-image-name').textContent = name;

    // Load current auto-install configuration
    try {
        const res = await authFetch(`${API_BASE}/images/autoinstall?filename=${encodeURIComponent(filename)}`);
        const data = await res.json();

        if (data.success && data.data) {
            document.getElementById('autoinstall-enabled').checked = data.data.auto_install_enabled || false;
            document.getElementById('autoinstall-script-type').value = data.data.auto_install_script_type || 'preseed';
            document.getElementById('autoinstall-script').value = data.data.auto_install_script || '';
        } else {
            // Default values for new configuration
            document.getElementById('autoinstall-enabled').checked = false;
            document.getElementById('autoinstall-script-type').value = 'preseed';
            document.getElementById('autoinstall-script').value = '';
        }
    } catch (err) {
        console.error('Failed to load auto-install config:', err);
        document.getElementById('autoinstall-enabled').checked = false;
        document.getElementById('autoinstall-script-type').value = 'preseed';
        document.getElementById('autoinstall-script').value = '';
    }

    openModal('autoinstall-modal');
}

async function saveAutoInstallScript() {
    const filename = document.getElementById('autoinstall-image-filename').value;
    const enabled = document.getElementById('autoinstall-enabled').checked;
    const scriptType = document.getElementById('autoinstall-script-type').value;
    const script = document.getElementById('autoinstall-script').value;

    try {
        const res = await authFetch(`${API_BASE}/images/autoinstall?filename=${encodeURIComponent(filename)}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                auto_install_enabled: enabled,
                auto_install_script_type: scriptType,
                auto_install_script: script
            })
        });

        const data = await res.json();
        if (data.success) {
            showNotification('Auto-install configuration saved', 'success');
            closeModal('autoinstall-modal');
            loadImages(); // Refresh images list
        } else {
            showNotification('Failed to save auto-install configuration: ' + data.error, 'error');
        }
    } catch (err) {
        showNotification('Failed to save auto-install configuration', 'error');
        console.error(err);
    }
}

// ============================================================================
// Custom File Management
// ============================================================================

let allFiles = [];
let currentFileFilter = 'all';

// ==================== PUBLIC FILES ====================

async function loadPublicFiles() {
    const container = document.getElementById('public-files-table');
    container.innerHTML = '<div class="spinner"></div><p>Loading files...</p>';

    try {
        const res = await authFetch('/api/files');
        const data = await res.json();

        if (data.success) {
            const publicFiles = (data.data || []).filter(f => f.public);
            renderPublicFilesTable(publicFiles);
        } else {
            container.innerHTML = `<p class="error">Failed to load files: ${data.error}</p>`;
        }
    } catch (err) {
        container.innerHTML = '<p class="error">Failed to load files</p>';
        console.error(err);
    }
}

function renderPublicFilesTable(files) {
    const container = document.getElementById('public-files-table');

    if (files.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px; text-align: center;">No public files found. Upload your first public file to get started.</p>';
        return;
    }

    const html = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Filename</th>
                    <th>Description</th>
                    <th>Type</th>
                    <th>Size</th>
                    <th>Downloads</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${files.map(file => `
                    <tr>
                        <td><code>${escapeHtml(file.filename)}</code></td>
                        <td>${escapeHtml(file.description || '-')}</td>
                        <td><span class="badge badge-info">${escapeHtml(file.content_type || 'unknown')}</span></td>
                        <td>${formatBytes(file.size)}</td>
                        <td>${file.download_count || 0}</td>
                        <td>
                            <button class="btn btn-sm" onclick="copyFileDownloadURL('${escapeHtml(file.filename)}')">📋 Copy URL</button>
                            <button class="btn btn-info btn-sm" onclick="showEditFileModal(${file.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteFile(${file.id}, '${escapeHtml(file.filename)}')">Delete</button>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
        </div>
    `;

    container.innerHTML = html;
}

function showUploadPublicFileModal() {
    document.getElementById('upload-public-file-form').reset();
    showModal('upload-public-file-modal');
}

async function uploadPublicFile(event) {
    event.preventDefault();

    const fileInput = document.getElementById('public-file-upload');
    const description = document.getElementById('public-file-description').value;

    if (!fileInput.files || fileInput.files.length === 0) {
        showNotification('Please select a file', 'error');
        return;
    }

    const file = fileInput.files[0];

    if (file.size > 100 * 1024 * 1024) {
        showNotification('File size exceeds 100MB limit', 'error');
        return;
    }

    const formData = new FormData();
    formData.append('file', file);
    formData.append('description', description);
    formData.append('public', 'true');

    try {
        const res = await authFetch('/api/files/upload', {
            method: 'POST',
            body: formData
        });

        const data = await res.json();

        if (data.success) {
            showNotification('File uploaded successfully', 'success');
            closeModal('upload-public-file-modal');
            loadPublicFiles();
        } else {
            showNotification('Failed to upload file: ' + data.error, 'error');
        }
    } catch (err) {
        showNotification('Failed to upload file', 'error');
        console.error(err);
    }
}

// ==================== IMAGE-SPECIFIC FILES ====================

function showImageFilesModal(imageId, imageName) {
    const image = images.find(img => img.id === imageId);
    if (!image) return;

    document.getElementById('image-files-image-name').textContent = imageName;
    document.getElementById('image-files-image-id').value = imageId;

    const imageFiles = image.files || [];
    renderImageFilesTable(imageFiles, imageId, imageName);

    showModal('image-files-modal');
}

function renderImageFilesTable(files, imageId, imageName) {
    const container = document.getElementById('image-files-table');

    if (files.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px; text-align: center;">No files for this image yet.</p>';
        return;
    }

    const html = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Filename</th>
                    <th>Description</th>
                    <th>Type</th>
                    <th>Size</th>
                    <th>Downloads</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${files.map(file => `
                    <tr>
                        <td><code>${escapeHtml(file.filename)}</code></td>
                        <td>${escapeHtml(file.description || '-')}</td>
                        <td><span class="badge badge-info">${escapeHtml(file.content_type || 'unknown')}</span></td>
                        <td>${formatBytes(file.size)}</td>
                        <td>${file.download_count || 0}</td>
                        <td>
                            <button class="btn btn-sm" onclick="copyFileDownloadURL('${escapeHtml(file.filename)}')">📋 Copy URL</button>
                            <button class="btn btn-info btn-sm" onclick="showEditFileModal(${file.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteFile(${file.id}, '${escapeHtml(file.filename)}')">Delete</button>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
        </div>
    `;

    container.innerHTML = html;
}

async function uploadImageFile(event) {
    event.preventDefault();

    const fileInput = document.getElementById('image-file-upload');
    const description = document.getElementById('image-file-description').value;
    const destinationPath = document.getElementById('image-file-destination').value;
    const autoInstall = document.getElementById('image-file-autoinstall').checked;
    const imageId = document.getElementById('image-files-image-id').value;

    if (!fileInput.files || fileInput.files.length === 0) {
        showNotification('Please select a file', 'error');
        return;
    }

    const file = fileInput.files[0];

    if (file.size > 100 * 1024 * 1024) {
        showNotification('File size exceeds 100MB limit', 'error');
        return;
    }

    const formData = new FormData();
    formData.append('file', file);
    formData.append('description', description);
    formData.append('destinationPath', destinationPath);
    formData.append('autoInstall', autoInstall);
    formData.append('public', 'false');
    formData.append('imageId', imageId);

    try {
        const res = await authFetch('/api/files/upload', {
            method: 'POST',
            body: formData
        });

        const data = await res.json();

        if (data.success) {
            showNotification('File uploaded successfully', 'success');

            // Reset form
            document.getElementById('upload-image-file-form').reset();
            // Re-check the autoinstall checkbox (reset unchecks it)
            document.getElementById('image-file-autoinstall').checked = true;

            // Reload images data and refresh the modal
            await loadImages();

            // Refresh the files table in the modal
            const imageName = document.getElementById('image-files-image-name').textContent;
            const image = images.find(img => img.id === parseInt(imageId));
            if (image) {
                renderImageFilesTable(image.files || [], imageId, imageName);
            }
        } else {
            showNotification('Failed to upload file: ' + data.error, 'error');
        }
    } catch (err) {
        showNotification('Failed to upload file', 'error');
        console.error(err);
    }
}

// ==================== COMMON FILE OPERATIONS ====================

async function showEditFileModal(fileId) {
    try {
        const res = await authFetch('/api/files');
        const data = await res.json();

        if (!data.success) {
            showNotification('Failed to load file details', 'error');
            return;
        }

        const file = (data.data || []).find(f => f.id === fileId);
        if (!file) {
            showNotification('File not found', 'error');
            return;
        }

        document.getElementById('edit-file-id').value = file.id;
        document.getElementById('edit-file-name').value = file.filename;
        document.getElementById('edit-file-description').value = file.description || '';
        document.getElementById('edit-file-type').value = file.public ? 'Public' : 'Image-Specific';
        document.getElementById('edit-file-size').value = formatBytes(file.size);

        const serverAddr = window.location.hostname;
        const port = 8080;
        const url = `http://${serverAddr}:${port}/files/${file.filename}`;
        document.getElementById('edit-file-url').textContent = url;

        showModal('edit-file-modal');
    } catch (err) {
        showNotification('Failed to load file details', 'error');
        console.error(err);
    }
}

async function updateFile(event) {
    event.preventDefault();

    const fileId = document.getElementById('edit-file-id').value;
    const description = document.getElementById('edit-file-description').value;

    try {
        const res = await authFetch(`/api/files/update?id=${fileId}`, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ description })
        });

        const data = await res.json();

        if (data.success) {
            showNotification('File updated successfully', 'success');
            closeModal('edit-file-modal');
            loadPublicFiles();
            loadImages();
        } else {
            showNotification('Failed to update file: ' + data.error, 'error');
        }
    } catch (err) {
        showNotification('Failed to update file', 'error');
        console.error(err);
    }
}

async function deleteFile(fileId, filename) {
    if (!confirm(`Are you sure you want to delete "${filename}"?\n\nThis will permanently delete the file from the server.`)) {
        return;
    }

    try {
        const res = await authFetch(`/api/files/delete?id=${fileId}`, {
            method: 'DELETE'
        });

        const data = await res.json();

        if (data.success) {
            showNotification('File deleted successfully', 'success');
            loadPublicFiles();
            loadImages();
        } else {
            showNotification('Failed to delete file: ' + data.error, 'error');
        }
    } catch (err) {
        showNotification('Failed to delete file', 'error');
        console.error(err);
    }
}

function copyFileDownloadURL(filename) {
    const serverAddr = window.location.hostname;
    const port = 8080;
    const url = `http://${serverAddr}:${port}/files/${filename}`;

    navigator.clipboard.writeText(url).then(() => {
        showNotification('Download URL copied to clipboard', 'success');
    }).catch(() => {
        showNotification('Failed to copy URL', 'error');
    });
}

function copyFileURL() {
    const url = document.getElementById('edit-file-url').textContent;
    navigator.clipboard.writeText(url).then(() => {
        showNotification('URL copied to clipboard', 'success');
    }).catch(() => {
        showNotification('Failed to copy URL', 'error');
    });
}

function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

let groups = [];

async function loadGroups() {
    try {
        const res = await authFetch(`${API_BASE}/groups`);
        const data = await res.json();

        if (data.success && data.data) {
            groups = data.data;
            renderGroupsTable();
        } else {
            document.getElementById('groups-table').innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No groups found.</p>';
        }
    } catch (err) {
        document.getElementById('groups-table').innerHTML = '<p style="color: #ef4444; padding: 20px;">Failed to load groups</p>';
        console.error(err);
    }
}

function renderGroupsTable() {
    const container = document.getElementById('groups-table');

    if (!groups || groups.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No groups found. Click "+ Add Group" to create one.</p>';
        return;
    }

    const sortedGroups = [...groups]
        .filter(g => rowMatchesFilter('image-groups', [g.name, g.description, g.parent && g.parent.name]))
        .sort((a, b) => {
            if (a.order !== b.order) return a.order - b.order;
            return a.name.localeCompare(b.name);
        });

    let html = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Description</th>
                    <th>Parent</th>
                    <th>Order</th>
                    <th class="col-dot" title="Enabled / Disabled">On</th>
                </tr>
            </thead>
            <tbody>
    `;

    for (const group of sortedGroups) {
        const parentName = group.parent_id ? (groups.find(g => g.id === group.parent_id)?.name || 'Unknown') : '-';

        html += `
            <tr class="row-clickable" onclick="showEditGroupModal(${group.id})">
                <td><strong>${escapeHtml(group.name)}</strong></td>
                <td>${escapeHtml(group.description || '-')}</td>
                <td>${escapeHtml(parentName)}</td>
                <td>${group.order}</td>
                <td class="col-dot">
                    <span class="status-dot ${group.enabled ? 'on' : 'off'}" title="${group.enabled ? 'Enabled' : 'Disabled'}"></span>
                </td>
            </tr>
        `;
    }

    html += `
            </tbody>
        </table>
        </div>
    `;

    container.innerHTML = html;
}

function showAddGroupModal() {
    const form = document.getElementById('add-group-form');
    form.reset();

    const parentSelect = document.getElementById('add-group-parent-select');
    parentSelect.innerHTML = '<option value="">None (Root Level)</option>';

    for (const group of groups) {
        parentSelect.innerHTML += `<option value="${group.id}">${escapeHtml(group.name)}</option>`;
    }

    openModal('add-group-modal');
}

function showEditGroupModal(groupId) {
    const group = groups.find(g => g.id === groupId);
    if (!group) return;

    const form = document.getElementById('edit-group-form');
    form.elements.id.value = group.id;
    form.elements.name.value = group.name;
    form.elements.description.value = group.description || '';
    form.elements.order.value = group.order;
    form.elements.enabled.checked = group.enabled;

    const parentSelect = document.getElementById('edit-group-parent-select');
    parentSelect.innerHTML = '<option value="">None (Root Level)</option>';

    for (const g of groups) {
        if (g.id !== groupId) {
            const selected = group.parent_id === g.id ? 'selected' : '';
            parentSelect.innerHTML += `<option value="${g.id}" ${selected}>${escapeHtml(g.name)}</option>`;
        }
    }

    openModal('edit-group-modal');
}

function deleteFromEditGroup() {
    const form = document.getElementById('edit-group-form');
    const groupId = parseInt(form.elements.id.value, 10);
    const groupName = form.elements.name.value;
    if (!groupId) return;
    closeModal('edit-group-modal');
    deleteGroup(groupId, groupName);
}

async function deleteGroup(groupId, groupName) {
    if (!confirm(`Delete group "${groupName}"? This will unassign all images from this group.`)) return;

    try {
        const res = await authFetch(`${API_BASE}/groups/delete?id=${groupId}`, {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id: groupId })
        });

        const data = await res.json();

        if (data.success) {
            showNotification('Group deleted successfully', 'success');
            loadGroups();
            loadImages();
        } else {
            showNotification('Failed to delete group: ' + (data.error || 'Unknown error'), 'error');
        }
    } catch (err) {
        showNotification('Failed to delete group', 'error');
        console.error(err);
    }
}

function switchPropsTab(tabName) {
    const short = tabName.startsWith('props-') ? tabName.slice('props-'.length) : tabName;
    document.querySelectorAll('#image-properties-modal .subtab').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.propsTab === short);
    });
    document.querySelectorAll('#image-properties-modal .props-tab-content').forEach(el => {
        el.style.display = 'none';
    });
    const active = document.getElementById(`props-${short}-content`);
    if (active) active.style.display = '';

    if (short === 'files') {
        const filename = document.getElementById('image-props-filename').value;
        if (filename) loadPropsImageFiles();
    }
}

function getDefaultBootParams(img) {
    if (img.boot_method !== 'kernel' || !img.extracted) return '';
    switch (img.distro) {
        case 'arch':
            return 'archiso_http_srv={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ ip=dhcp';
        case 'nixos':
            return 'ip=dhcp';
        case 'fedora':
        case 'centos':
            return 'root=live:{{BASE_URL}}/isos/{{FILENAME}} rd.live.image inst.repo={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ inst.stage2={{BASE_URL}}/boot/{{CACHE_DIR}}/iso/ rd.neednet=1 ip=dhcp';
        case 'debian':
            return img.squashfs_path ? 'initrd=initrd priority=critical fetch={{SQUASHFS}}' : 'initrd=initrd priority=critical';
        case 'ubuntu':
            if (img.squashfs_path) return 'initrd=initrd ip=dhcp fetch={{SQUASHFS}}';
            return 'initrd=initrd ip=dhcp url={{BASE_URL}}/isos/{{FILENAME}}';
        case 'freebsd':
            return 'vfs.root.mountfrom=cd9660:/dev/md0 kernelname=/boot/kernel/kernel';
        default:
            return 'iso-url={{BASE_URL}}/isos/{{FILENAME}} ip=dhcp';
    }
}

async function showImagePropertiesModal(filename, opts) {
    opts = opts || {};
    const img = images.find(i => i.filename === filename);
    if (!img) return;

    if (!groups || groups.length === 0) {
        await loadGroups();
    }

    document.getElementById('image-props-name').textContent = img.name;
    document.getElementById('image-props-filename').value = img.filename;
    document.getElementById('image-props-display-name').value = img.name || '';
    document.getElementById('image-props-description').value = img.description || '';
    document.getElementById('image-props-order').value = img.order || 0;
    document.getElementById('image-props-boot-method').value = img.boot_method || 'sanboot';

    // Populate distro profile dropdown
    const distroSelect = document.getElementById('image-props-distro');
    distroSelect.innerHTML = '<option value="">Auto-detect</option>';
    try {
        const profRes = await authFetch(`${API_BASE}/profiles`);
        const profData = await profRes.json();
        if (profData.success && profData.data) {
            for (const p of profData.data) {
                const selected = img.distro === p.profile_id ? 'selected' : '';
                distroSelect.innerHTML += `<option value="${p.profile_id}" ${selected}>${p.display_name}</option>`;
            }
        }
    } catch (e) {}

    document.getElementById('image-props-boot-params').value = img.boot_params || getDefaultBootParams(img) || '';
    document.getElementById('image-props-redetect-btn').style.display = img.extracted ? '' : 'none';
    document.getElementById('image-props-enabled').checked = img.enabled;
    document.getElementById('image-props-public').checked = img.public;

    applyBootParamsWindowsLock(distroSelect.value);
    distroSelect.onchange = () => applyBootParamsWindowsLock(distroSelect.value);

    // Auto-install fields
    document.getElementById('image-props-autoinstall-enabled').checked = img.auto_install_enabled || false;
    document.getElementById('image-props-autoinstall-type').value = img.auto_install_script_type || 'preseed';
    document.getElementById('image-props-autoinstall-script').value = img.auto_install_script || '';
    document.getElementById('image-props-autoinstall-url').textContent = img.filename;
    await populateImageAutoInstallFileDropdown(img);

    const groupSelect = document.getElementById('image-props-group');
    groupSelect.innerHTML = '<option value="">Unassigned</option>';
    for (const group of groups) {
        const selected = img.group_id === group.id ? 'selected' : '';
        groupSelect.innerHTML += `<option value="${group.id}" ${selected}>${escapeHtml(group.name)}</option>`;
    }

    // Toggle action buttons based on image state
    const extractBtn = document.getElementById('image-props-extract-btn');
    const netbootBtn = document.getElementById('image-props-netboot-btn');
    if (extractionProgress[filename]) {
        extractBtn.style.display = 'inline-block';
        extractBtn.disabled = true;
        extractBtn.style.opacity = '0.5';
        extractBtn.textContent = t('props.action.extracting');
        netbootBtn.style.display = 'none';
    } else if (img.netboot_required && !img.netboot_available) {
        extractBtn.style.display = 'none';
        netbootBtn.style.display = 'inline-block';
    } else if (!img.netboot_required) {
        netbootBtn.style.display = 'none';
        extractBtn.style.display = 'inline-block';
        extractBtn.disabled = false;
        extractBtn.style.opacity = '';
        extractBtn.textContent = img.extracted ? t('props.action.re_extract') : t('props.action.extract');
    } else {
        extractBtn.style.display = 'none';
        netbootBtn.style.display = 'none';
    }

    const patchSmbBtn = document.getElementById('image-props-patch-smb-btn');
    const smbEligible = cachedWindowsSMBActive && img.extracted && img.distro === 'windows';
    patchSmbBtn.style.display = smbEligible ? 'inline-block' : 'none';
    patchSmbBtn.textContent = img.smb_install_enabled ? t('props.action.re_patch_smb') : t('props.action.patch_smb');

    // Stash state used by the live warnings so onChange handlers can re-evaluate.
    _imagePropsState = {
        img: img,
        initialAutoInstallFile: img.auto_install_file || '',
    };
    const aiSel = document.getElementById('image-props-autoinstall-file');
    if (aiSel) aiSel.onchange = updateImagePropsWarnings;
    updateImagePropsWarnings();

    if (!opts.preserveTab) {
        openModal('image-properties-modal');
        switchPropsTab('general');
    }
    updateImagePropsProgress(filename);
}

let _imagePropsState = null;

function updateImagePropsWarnings() {
    if (!_imagePropsState) return;
    const img = _imagePropsState.img;
    const fname = img.filename || '';
    const looksWindows = img.distro === 'windows' || filenameLooksWindows(fname);

    // 1. Not extracted but probably needs to be.
    const needsExtract = !img.extracted && filenameLikelyNeedsExtraction(fname);
    document.getElementById('image-props-warn-extract').style.display = needsExtract ? '' : 'none';

    // 1b. Debian/Ubuntu DVD ISOs that ship an installer kernel/initrd
    // separate from the live system — bootimus needs a netboot bundle
    // pulled from the mirror before this image can boot.
    const needsNetboot = img.netboot_required && !img.netboot_available;
    document.getElementById('image-props-warn-netboot').style.display = needsNetboot ? '' : 'none';

    // 2. Windows image but the server SMB share isn't enabled.
    const smbServerOff = looksWindows && !cachedWindowsSMBActive;
    document.getElementById('image-props-warn-smb-server').style.display = smbServerOff ? '' : 'none';

    // 3. Windows image, SMB enabled, but wimlib-imagex isn't installed —
    // patching can never succeed until the host has the tool.
    const wimlibMissing = looksWindows && cachedWindowsSMBActive && !cachedWindowsSMBPatcherAvailable;
    document.getElementById('image-props-warn-wimlib').style.display = wimlibMissing ? '' : 'none';

    // 4. Re-patch needed: image is currently patched AND either the backend
    // already detected drift, or the user has changed the auto-install file
    // in this modal session.
    const aiSel = document.getElementById('image-props-autoinstall-file');
    const currentAI = aiSel ? aiSel.value : (img.auto_install_file || '');
    const localChange = currentAI !== _imagePropsState.initialAutoInstallFile;
    const needsRepatch = img.smb_install_enabled && (img.smb_needs_repatch || localChange);
    document.getElementById('image-props-warn-repatch').style.display = needsRepatch ? '' : 'none';
}

function navigateToSettings() {
    closeModal('image-properties-modal');
    const item = document.querySelector('.sidebar-nav .nav-item[data-tab="settings"]');
    if (item) item.click();
}

async function saveAndRepatchFromProperties() {
    await saveImageProperties({ skipClose: true });
    await patchSmbFromProperties();
}

function applyBootParamsWindowsLock(distro) {
    const input = document.getElementById('image-props-boot-params');
    const redetect = document.getElementById('image-props-redetect-btn');
    const isWindows = distro === 'windows';
    input.disabled = isWindows;
    input.style.opacity = isWindows ? '0.5' : '';
    input.style.cursor = isWindows ? 'not-allowed' : '';
    input.title = isWindows ? 'Windows boots via wimboot — kernel parameters are not used' : '';
    if (isWindows) redetect.style.display = 'none';
}

async function patchSmbFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    const btn = document.getElementById('image-props-patch-smb-btn');
    btn.disabled = true;
    btn.textContent = t('props.action.patching');
    try {
        const res = await authFetch(`${API_BASE}/images/patch-smb?filename=${encodeURIComponent(filename)}`, { method: 'POST' });
        const data = await res.json();
        if (!data.success) throw new Error(data.error || t('props.notify.patch_failed'));
        showNotification(t('props.notify.patch_success'), 'success');
        await loadImages();
        refreshImagePropsIfOpenFor(filename);
    } catch (err) {
        showNotification(t('props.notify.patch_failed') + ': ' + err.message, 'error');
    } finally {
        btn.disabled = false;
        btn.textContent = t('props.action.patch_smb');
    }
}

function extractFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    const name = document.getElementById('image-props-display-name').value;
    extractImage(filename, name);
}

function deleteFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    const name = document.getElementById('image-props-display-name').value;
    closeModal('image-properties-modal');
    deleteImage(filename, name);
}

function downloadNetbootFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    const name = document.getElementById('image-props-display-name').value;
    downloadNetboot(filename, name);
}

async function loadImageFileBrowser(filename) {
    const container = document.getElementById('image-file-browser');
    container.innerHTML = '<div class="loading"><div class="spinner"></div>Loading files...</div>';

    try {
        const res = await authFetch(`${API_BASE}/images/files?filename=${encodeURIComponent(filename)}`);
        const data = await res.json();

        if (data.success && data.data && data.data.files) {
            renderFileBrowser(data.data.files, filename);
        } else {
            container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No files found for this image.</p>';
        }
    } catch (err) {
        container.innerHTML = '<p style="color: #ef4444; padding: 20px;">Failed to load file browser</p>';
        console.error(err);
    }
}

function renderFileBrowser(files, filename) {
    const container = document.getElementById('image-file-browser');

    const baseDir = filename.replace('.iso', '');
    const hasFiles = files && files.length > 0;

    let html = `
        <div style="background: var(--bg-primary); padding: 15px; border-radius: 6px; margin-bottom: 15px;">
            <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px;">
                <div style="font-family: monospace; color: var(--accent);">📁 /isos/${escapeHtml(filename)}</div>
                <button class="btn btn-danger btn-sm" onclick="deleteImageFile('${escapeHtml(filename)}', '${escapeHtml(baseDir)}', '${escapeHtml(filename)}', false, true)" style="padding: 4px 10px; font-size: 12px;">Delete ISO</button>
            </div>
            <div style="display: flex; align-items: center; justify-content: space-between;">
                <div style="font-family: monospace; color: ${hasFiles ? '#38bdf8' : '#64748b'};">📁 /boot/${escapeHtml(baseDir)}/ ${hasFiles ? '' : '<span style="color: var(--text-secondary); font-size: 11px;">(not extracted)</span>'}</div>
                ${hasFiles ? '<button class="btn btn-danger btn-sm" onclick="deleteImageFile(\'' + escapeHtml(filename) + '\', \'' + escapeHtml(baseDir) + '\', \'\', true, false)" style="padding: 4px 10px; font-size: 12px;">Delete Boot Folder</button>' : ''}
            </div>
        </div>
    `;

    if (hasFiles) {
        const tree = buildFileTree(files);
        html += `
            <div style="max-height: 500px; overflow-y: auto; background: var(--bg-primary); border-radius: 6px; padding: 10px;">
                ${renderFileTreeNode(tree, filename, baseDir, '')}
            </div>
        `;
    } else {
        html += `
            <div style="background: var(--bg-primary); padding: 20px; border-radius: 6px; text-align: center; color: var(--text-secondary);">
                No extracted files found. Extract the kernel first to browse files.
            </div>
        `;
    }

    container.innerHTML = html;
}

function buildFileTree(files) {
    const root = { name: '', children: {}, files: [] };

    for (const file of files) {
        const parts = file.path.split('/');
        let current = root;

        for (let i = 0; i < parts.length; i++) {
            const part = parts[i];
            const isLast = i === parts.length - 1;

            if (isLast && !file.is_dir) {
                current.files.push({ name: part, size: file.size, path: file.path });
            } else {
                if (!current.children[part]) {
                    current.children[part] = { name: part, children: {}, files: [], path: parts.slice(0, i + 1).join('/') };
                }
                current = current.children[part];
            }
        }
    }

    return root;
}

function renderFileTreeNode(node, filename, baseDir, indent) {
    let html = '';

    const dirs = Object.keys(node.children).sort();
    for (const dirName of dirs) {
        const child = node.children[dirName];
        const id = 'tree-' + Math.random().toString(36).substr(2, 9);

        html += `
            <div style="margin-left: ${indent};">
                <div style="padding: 6px; cursor: pointer; font-family: monospace; font-size: 13px; color: var(--text-primary); border-bottom: 1px solid #1e293b;" onclick="toggleTreeNode('${id}')">
                    <span id="${id}-icon">▶</span> 📁 ${escapeHtml(dirName)}
                </div>
                <div id="${id}" style="display: none;">
                    ${renderFileTreeNode(child, filename, baseDir, '20px')}
                </div>
            </div>
        `;
    }

    const sortedFiles = node.files.sort((a, b) => a.name.localeCompare(b.name));
    for (const file of sortedFiles) {
        html += `
            <div style="margin-left: ${indent}; padding: 6px; border-bottom: 1px solid #1e293b; font-family: monospace; font-size: 13px; color: var(--text-primary);">
                📄 ${escapeHtml(file.name)} <span style="color: var(--text-secondary); font-size: 11px;">(${formatBytes(file.size)})</span>
            </div>
        `;
    }

    return html;
}

function toggleTreeNode(id) {
    const node = document.getElementById(id);
    const icon = document.getElementById(id + '-icon');

    if (node.style.display === 'none') {
        node.style.display = 'block';
        icon.textContent = '▼';
    } else {
        node.style.display = 'none';
        icon.textContent = '▶';
    }
}

async function deleteImageFile(filename, baseDir, path, isDir, isIso) {
    let confirmMsg = '';
    let deleteType = '';

    if (isIso) {
        confirmMsg = `Delete ISO file "${filename}"? This will remove the ISO file but keep the extracted boot folder.`;
        deleteType = 'ISO file';
    } else {
        confirmMsg = `Delete boot folder "${baseDir}"? This will remove all extracted files but keep the ISO.`;
        deleteType = 'boot folder';
    }

    if (!confirm(confirmMsg)) return;

    try {
        const res = await authFetch(`${API_BASE}/images/files/delete`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                filename: filename,
                base_dir: baseDir,
                path: path,
                is_dir: isDir,
                is_iso: isIso
            })
        });

        const data = await res.json();

        if (data.success) {
            showNotification(`${deleteType} deleted successfully`, 'success');
            loadImageFileBrowser(filename);

            // Reload images list to reflect changes in boot method
            if (!isIso) {
                loadImages();
            }
        } else {
            showNotification(`Failed to delete ${deleteType}: ` + (data.error || 'Unknown error'), 'error');
        }
    } catch (err) {
        showNotification(`Failed to delete ${deleteType}`, 'error');
        console.error(err);
    }
}

async function saveImageProperties(opts) {
    opts = opts || {};
    const filename = document.getElementById('image-props-filename').value;
    const displayName = document.getElementById('image-props-display-name').value;
    const description = document.getElementById('image-props-description').value;
    const groupId = document.getElementById('image-props-group').value;
    const order = parseInt(document.getElementById('image-props-order').value) || 0;
    const bootMethod = document.getElementById('image-props-boot-method').value;
    const distro = document.getElementById('image-props-distro').value;
    const bootParams = document.getElementById('image-props-boot-params').value;
    const enabled = document.getElementById('image-props-enabled').checked;
    const isPublic = document.getElementById('image-props-public').checked;

    // Auto-install: presence of a selected file = enabled. No separate
    // checkbox / script type / inline script from the image panel — those
    // are managed in the Auto-Install section.
    const autoInstallFile = document.getElementById('image-props-autoinstall-file').value;
    const autoInstallEnabled = autoInstallFile !== '';
    const autoInstallType = '';
    const autoInstallScript = '';

    const updates = {
        name: displayName,
        description: description,
        group_id: groupId ? parseInt(groupId) : null,
        order: order,
        boot_method: bootMethod,
        distro: distro,
        boot_params: bootParams,
        enabled: enabled,
        public: isPublic,
        auto_install_file: autoInstallFile,
    };

    try {
        // Update general image properties
        const res = await authFetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(updates)
        });

        const data = await res.json();

        if (!data.success) {
            showNotification('Failed to update image: ' + (data.error || 'Unknown error'), 'error');
            return false;
        }

        showNotification('Image properties updated', 'success');
        await loadImages();
        loadStats();
        if (!opts.skipClose) refreshImagePropsIfOpenFor(filename);
        return true;
    } catch (err) {
        showNotification('Failed to update image properties', 'error');
        console.error(err);
        return false;
    }
}

async function loadPropsImageFiles() {
    const filename = document.getElementById('image-props-filename').value;
    if (!filename) return;

    const listContainer = document.getElementById('image-props-files-list');
    listContainer.innerHTML = '<div class="loading"><div class="spinner"></div>Loading files...</div>';

    try {
        // Find image in global images array
        const image = images.find(img => img.filename === filename);
        if (!image) {
            throw new Error('Image not found');
        }

        // Fetch filesystem file list (returns {path, is_dir, size})
        const res = await authFetch(`${API_BASE}/images/files?filename=${encodeURIComponent(filename)}`);
        if (!res.ok) throw new Error('Failed to load files');

        const data = await res.json();
        const allFiles = (data.success && data.data && data.data.files) ? data.data.files : [];

        // Separate by type
        const autoinstallFiles = allFiles.filter(f => f.path.startsWith('autoinstall/'));
        // Everything else that's not in autoinstall is considered extracted ISO contents
        const isoContents = allFiles.filter(f => !f.path.startsWith('autoinstall/'));

        const sectionHeader = (label, action) => `
            <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 0 0 8px; padding-bottom: 4px; border-bottom: 1px solid var(--border);">
                <h4 style="margin: 0; font-size: 13px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.04em;">${label}</h4>
                ${action || ''}
            </div>`;
        const emptyRow = text => `<div style="color: var(--text-secondary); font-size: 12px; padding: 6px 8px;">${text}</div>`;

        let html = '';

        html += '<div style="margin-bottom: 20px;">';
        html += sectionHeader('Uploaded');
        if (autoinstallFiles.length > 0) {
            const tree = buildFSTree(autoinstallFiles, 'autoinstall/');
            html += renderFSTree(tree, filename, 0, true);
        } else {
            html += emptyRow('No uploaded files — use the form above to upload.');
        }
        html += '</div>';

        html += '<div>';
        const resetAction = (image.extracted && isoContents.length > 0)
            ? `<button class="btn btn-sm" onclick="deleteExtractedContents()" title="Delete all extracted boot files and reset to sanboot mode. The autoinstall folder is preserved." style="color: var(--danger); border-color: var(--danger);">Reset Extraction</button>`
            : '';
        html += sectionHeader('Extracted', resetAction);
        if (isoContents.length > 0) {
            const tree = buildFSTree(isoContents, '');
            html += renderFSTree(tree, filename, 0, false);
        } else if (image.extracted) {
            html += emptyRow('Extracted but no files found.');
        } else {
            html += emptyRow('Not extracted — use the Extract button below to enable kernel boot.');
        }
        html += '</div>';

        listContainer.innerHTML = html;

    } catch (err) {
        listContainer.innerHTML = `
            <div style="text-align: center; padding: 20px; color: #ef4444;">
                <p style="margin: 0; font-size: 13px;">Failed to load files</p>
                <p style="margin: 4px 0 0 0; font-size: 12px;">${escapeHtml(err.message)}</p>
            </div>
        `;
        console.error(err);
    }
}

function buildFSTree(files, stripPrefix) {
    const root = { name: '/', type: 'folder', children: {}, path: '/' };

    files.forEach(file => {
        let pathToUse = file.path;
        if (stripPrefix && pathToUse.startsWith(stripPrefix)) {
            pathToUse = pathToUse.substring(stripPrefix.length);
        }

        const parts = pathToUse.split('/').filter(p => p);
        let current = root;

        parts.forEach((part, idx) => {
            if (!current.children[part]) {
                const isLastPart = idx === parts.length - 1;
                current.children[part] = {
                    name: part,
                    type: file.is_dir && isLastPart ? 'folder' : (isLastPart ? 'file' : 'folder'),
                    children: {},
                    fullPath: file.path,
                    size: file.size || 0
                };
            }
            current = current.children[part];
        });
    });

    return root;
}

function renderFSTree(node, filename, level = 0, showDelete = false) {
    const entries = Object.values(node.children).sort((a, b) => {
        if (a.type !== b.type) return a.type === 'folder' ? -1 : 1;
        return a.name.localeCompare(b.name);
    });

    if (entries.length === 0 && level === 0) {
        return '<div style="color: var(--text-secondary); padding: 10px; font-size: 13px;">Empty directory</div>';
    }

    const baseDir = filename.replace(/\.[^/.]+$/, '');

    let html = '';
    entries.forEach(entry => {
        const indent = level * 16;
        const hasChildren = entry.type === 'folder' && Object.keys(entry.children).length > 0;

        if (entry.type === 'folder') {
            const folderId = 'folder-' + Math.random().toString(36).substr(2, 9);
            html += `
                <div style="margin-left: ${indent}px;">
                    <div onclick="toggleFolder('${folderId}')" style="cursor: pointer; padding: 4px 8px; margin: 2px 0; border-radius: 4px; display: flex; align-items: center; gap: 8px; font-size: 13px; user-select: none; color: var(--text-secondary);" onmouseover="this.style.background='#1e293b'" onmouseout="this.style.background='transparent'">
                        <span id="${folderId}-icon" style="font-family: monospace; width: 12px; display: inline-block;">▶</span>
                        <span style="color: var(--accent);">📁 ${escapeHtml(entry.name)}</span>
                    </div>
                    <div id="${folderId}" style="display: none;">
                        ${hasChildren ? renderFSTree(entry, filename, level + 1, showDelete) : ''}
                    </div>
                </div>
            `;
        } else {
            const downloadUrl = `/boot/${encodeURIComponent(baseDir)}/${encodeURIComponent(entry.fullPath)}`;
            const sizeStr = entry.size > 0 ? formatBytes(entry.size) : '';

            html += `
                <div style="margin-left: ${indent}px; padding: 6px 8px; margin: 2px 0; border-radius: 4px; display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 13px; background: var(--bg-secondary);">
                    <div style="display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0;">
                        <span style="color: #cbd5e1;">📄</span>
                        <span style="color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${escapeHtml(entry.name)}</span>
                        ${sizeStr ? `<span style="color: var(--text-secondary); font-size: 11px;">${sizeStr}</span>` : ''}
                    </div>
                    <div style="display: flex; gap: 6px; flex-shrink: 0;">
                        <a href="${downloadUrl}" class="btn btn-primary btn-sm" download style="padding: 3px 10px; font-size: 11px; white-space: nowrap; text-decoration: none;">Download</a>
                        ${showDelete ? `<button class="btn btn-danger btn-sm" onclick="deleteFileByPath('${escapeHtml(entry.fullPath)}')" style="padding: 3px 10px; font-size: 11px;">Delete</button>` : ''}
                    </div>
                </div>
            `;
        }
    });

    return html;
}

function toggleFolder(folderId) {
    const folder = document.getElementById(folderId);
    const icon = document.getElementById(folderId + '-icon');

    if (folder.style.display === 'none') {
        folder.style.display = 'block';
        icon.textContent = '▼';
    } else {
        folder.style.display = 'none';
        icon.textContent = '▶';
    }
}

async function deleteFileByPath(filePath) {
    if (!confirm(`Are you sure you want to delete ${filePath}?`)) {
        return;
    }

    const filename = document.getElementById('image-props-filename').value;
    const baseDir = filename.replace(/\.[^/.]+$/, '');

    try {
        const res = await authFetch(`${API_BASE}/images/files/delete`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                filename: filename,
                base_dir: baseDir,
                path: filePath,
                is_dir: false,
                is_iso: false
            })
        });

        const data = await res.json();

        if (!data.success) {
            throw new Error(data.error || 'Delete failed');
        }

        showNotification('File deleted successfully', 'success');
        await loadPropsImageFiles();

    } catch (err) {
        showNotification(`Failed to delete file: ${err.message}`, 'error');
        console.error(err);
    }
}

async function deleteExtractedContents() {
    if (!confirm('Are you sure you want to delete all extracted boot files? This will reset the image to sanboot mode. The autoinstall folder will be preserved.')) {
        return;
    }

    const filename = document.getElementById('image-props-filename').value;
    const baseDir = filename.replace(/\.[^/.]+$/, '');

    try {
        const res = await authFetch(`${API_BASE}/images/files/delete`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                filename: filename,
                base_dir: baseDir,
                path: '',
                is_dir: true,
                is_iso: false
            })
        });

        const data = await res.json();

        if (!data.success) {
            throw new Error(data.error || 'Delete failed');
        }

        showNotification('Extracted contents deleted successfully', 'success');

        // Reload image list and files
        await loadImages();
        await loadPropsImageFiles();

    } catch (err) {
        showNotification(`Failed to delete extracted contents: ${err.message}`, 'error');
        console.error(err);
    }
}

async function uploadPropsImageFile() {
    const filename = document.getElementById('image-props-filename').value;
    const fileInput = document.getElementById('props-file-input');

    if (!fileInput.files || fileInput.files.length === 0) {
        showNotification('Please select a file', 'error');
        return;
    }

    try {
        // Find image in global images array
        const image = images.find(img => img.filename === filename);
        if (!image) {
            throw new Error('Image not found');
        }

        // Upload file - all files go to autoinstall folder
        const formData = new FormData();
        formData.append('file', fileInput.files[0]);
        formData.append('imageId', image.id);
        formData.append('autoInstall', 'true');
        formData.append('public', 'false');

        const uploadRes = await authFetch(`${API_BASE}/files/upload`, {
            method: 'POST',
            body: formData
        });

        const data = await uploadRes.json();

        if (!data.success) {
            throw new Error(data.error || 'Upload failed');
        }

        showNotification('File uploaded successfully', 'success');

        // Reset form
        fileInput.value = '';

        // Reload file list
        await loadPropsImageFiles();

    } catch (err) {
        showNotification(`Failed to upload file: ${err.message}`, 'error');
        console.error(err);
    }
}

async function deletePropsImageFile(imageId, fileId) {
    if (!confirm('Are you sure you want to delete this file?')) {
        return;
    }

    try {
        const res = await authFetch(`${API_BASE}/images/${imageId}/files/${fileId}`, {
            method: 'DELETE'
        });

        if (!res.ok) {
            const errorData = await res.json();
            throw new Error(errorData.error || 'Delete failed');
        }

        showNotification('File deleted successfully', 'success');
        await loadPropsImageFiles();

    } catch (err) {
        showNotification(`Failed to delete file: ${err.message}`, 'error');
        console.error(err);
    }
}

// ============================================================
// Table filter — simple client-side substring search
// ============================================================

const tableFilters = {};

function applyTableFilter(key, value, renderFn) {
    tableFilters[key] = (value || '').toLowerCase();
    if (typeof renderFn === 'function') renderFn();
}

function rowMatchesFilter(key, values) {
    const q = tableFilters[key];
    if (!q) return true;
    for (const v of values) {
        if (v && String(v).toLowerCase().includes(q)) return true;
    }
    return false;
}

// ============================================================
// CSV Import / Export — Clients
// ============================================================

function csvEscape(v) {
    if (v === null || v === undefined) return '';
    const s = String(v);
    if (/[",\n\r]/.test(s)) {
        return '"' + s.replace(/"/g, '""') + '"';
    }
    return s;
}

function exportClientsCSV() {
    if (!clients || clients.length === 0) {
        showAlert('No clients to export', 'error');
        return;
    }
    const groupsById = {};
    (clientGroups || []).forEach(g => { groupsById[g.id] = g.name; });

    const header = [
        'mac_address', 'name', 'description', 'enabled', 'show_public_images',
        'static', 'bootloader_set', 'client_group', 'allowed_images', 'next_boot_image',
    ];
    const rows = clients.map(c => [
        c.mac_address,
        c.name || '',
        c.description || '',
        c.enabled ? 'true' : 'false',
        c.show_public_images !== false ? 'true' : 'false',
        c.static ? 'true' : 'false',
        c.bootloader_set || '',
        c.client_group_id ? (groupsById[c.client_group_id] || '') : '',
        (c.allowed_images || []).join('|'),
        c.next_boot_image || '',
    ]);
    const csv = [header, ...rows].map(r => r.map(csvEscape).join(',')).join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
    const a = document.createElement('a');
    const ts = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-');
    a.href = URL.createObjectURL(blob);
    a.download = `bootimus-clients-${ts}.csv`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(a.href);
}

function showImportClientsModal() {
    document.getElementById('import-clients-form').reset();
    document.getElementById('import-clients-result').style.display = 'none';
    showModal('import-clients-modal');
}

async function importClientsCSV() {
    const fileInput = document.getElementById('import-clients-file');
    const resultDiv = document.getElementById('import-clients-result');
    if (!fileInput.files || fileInput.files.length === 0) return;
    const fd = new FormData();
    fd.append('file', fileInput.files[0]);
    resultDiv.style.display = 'block';
    resultDiv.textContent = 'Uploading…';
    try {
        const res = await authFetch(`${API_BASE}/clients/import`, { method: 'POST', body: fd });
        const data = await res.json();
        if (!data.success) {
            resultDiv.textContent = data.error || 'Import failed';
            resultDiv.style.color = 'var(--danger)';
            return;
        }
        const d = data.data || {};
        const errs = (d.errors || []).length;
        resultDiv.innerHTML = `<strong>${d.created || 0}</strong> created, <strong>${d.updated || 0}</strong> updated, <strong>${d.skipped || 0}</strong> skipped${errs ? `, <strong style="color:var(--danger)">${errs}</strong> errors` : ''}.` +
            (errs ? `<div style="margin-top:8px;">${(d.errors || []).slice(0, 10).map(e => `<div>${escapeHtml(e)}</div>`).join('')}${errs > 10 ? `<div>…and ${errs - 10} more</div>` : ''}</div>` : '');
        resultDiv.style.color = '';
        await loadClients();
    } catch (err) {
        resultDiv.textContent = 'Import failed: ' + err.message;
        resultDiv.style.color = 'var(--danger)';
    }
}

// ============================================================
// Client Groups
// ============================================================

let clientGroups = [];

function switchClientsSubtab(name) {
    document.querySelectorAll('#clients-tab .subtab').forEach(b => b.classList.toggle('active', b.dataset.subtab === name));
    document.getElementById('clients-subtab').style.display = name === 'clients' ? '' : 'none';
    document.getElementById('client-groups-subtab').style.display = name === 'client-groups' ? '' : 'none';
    if (name === 'client-groups') loadClientGroups();
}

function switchImagesSubtab(name) {
    document.querySelectorAll('#images-tab .subtab').forEach(b => b.classList.toggle('active', b.dataset.subtab === name));
    document.getElementById('images-subtab').style.display = name === 'images' ? '' : 'none';
    document.getElementById('image-groups-subtab').style.display = name === 'image-groups' ? '' : 'none';
    if (name === 'image-groups') loadGroups();
}

async function loadClientGroups() {
    try {
        const res = await authFetch(`${API_BASE}/client-groups`);
        const data = await res.json();
        if (!data.success) {
            document.getElementById('client-groups-table').innerHTML = '<p class="alert alert-error">Failed to load client groups</p>';
            return;
        }
        clientGroups = data.data || [];
        renderClientGroupsTable();
    } catch (err) {
        console.error('Failed to load client groups:', err);
    }
}

function renderClientGroupsTable() {
    const container = document.getElementById('client-groups-table');
    if (clientGroups.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No client groups yet. Click "+ Add Client Group" to create one.</p>';
        return;
    }
    const rows = clientGroups
        .filter(g => rowMatchesFilter('client-groups', [g.name, g.description, g.wol_broadcast_addr]))
        .map(g => `
        <tr class="row-clickable" onclick="showEditClientGroupModal(${g.id})">
            <td><strong>${escapeHtml(g.name)}</strong></td>
            <td>${escapeHtml(g.description || '-')}</td>
            <td>${g.member_count}</td>
            <td class="col-dot"><span class="status-dot ${g.enabled ? 'on' : 'off'}" title="${g.enabled ? 'Enabled' : 'Disabled'}"></span></td>
            <td>${g.stagger_delay_millis || 0} ms</td>
            <td>${escapeHtml(g.wol_broadcast_addr || '(default)')}</td>
        </tr>
    `).join('');
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead><tr>
                <th>Name</th><th>Description</th><th>Members</th>
                <th class="col-dot" title="Enabled / Disabled">On</th>
                <th>Stagger</th><th>WOL Broadcast</th>
            </tr></thead>
            <tbody>${rows}</tbody>
        </table>
        </div>
    `;
}

function showAddClientGroupModal() {
    document.getElementById('add-client-group-form').reset();
    showModal('add-client-group-modal');
}

document.addEventListener('DOMContentLoaded', () => {
    const addForm = document.getElementById('add-client-group-form');
    if (addForm) {
        addForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const fd = new FormData(e.target);
            const body = {
                name: fd.get('name'),
                description: fd.get('description') || '',
                enabled: fd.get('enabled') === 'on',
            };
            try {
                const res = await authFetch(`${API_BASE}/client-groups`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
                const data = await res.json();
                if (data.success) {
                    closeModal('add-client-group-modal');
                    showAlert('Client group created', 'success');
                    await loadClientGroups();
                    if (data.data && data.data.id) showEditClientGroupModal(data.data.id);
                } else {
                    showAlert(data.error || 'Failed to create', 'error');
                }
            } catch (err) {
                showAlert('Failed to create client group', 'error');
            }
        });
    }

    const editForm = document.getElementById('edit-client-group-form');
    if (editForm) {
        editForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const fd = new FormData(e.target);
            const id = parseInt(fd.get('id'), 10);
            const allowed = Array.from(document.getElementById('cg-allowed-images').selectedOptions).map(o => o.value);
            const members = Array.from(document.getElementById('cg-members').selectedOptions).map(o => o.value);
            const body = {
                name: fd.get('name'),
                description: fd.get('description') || '',
                enabled: fd.get('enabled') === 'on',
                wol_broadcast_addr: fd.get('wol_broadcast_addr') || '',
                stagger_delay_millis: parseInt(fd.get('stagger_delay_millis') || '0', 10),
                bootloader_set: fd.get('bootloader_set') || '',
                allowed_images: allowed,
                ipmi_port: parseInt(fd.get('ipmi_port') || '0', 10),
                ipmi_username: fd.get('ipmi_username') || '',
                ipmi_password: fd.get('ipmi_password') || '',
                ipmi_insecure: fd.get('ipmi_insecure') === 'on',
                auto_install_file: fd.get('auto_install_file') || '',
            };
            try {
                const res = await authFetch(`${API_BASE}/client-groups/update?id=${id}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
                const data = await res.json();
                if (!data.success) { showAlert(data.error || 'Failed to save', 'error'); return; }

                const current = await fetchGroupMembers(id);
                const currentSet = new Set(current);
                const targetSet = new Set(members);
                const toAdd = members.filter(m => !currentSet.has(m));
                const toRemove = current.filter(m => !targetSet.has(m));
                for (const mac of toAdd) {
                    await authFetch(`${API_BASE}/client-groups/membership`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ mac_address: mac, group_id: id }),
                    });
                }
                for (const mac of toRemove) {
                    await authFetch(`${API_BASE}/client-groups/membership`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ mac_address: mac, group_id: null }),
                    });
                }

                showAlert('Client group saved', 'success');
                closeModal('edit-client-group-modal');
                await loadClientGroups();
            } catch (err) {
                console.error(err);
                showAlert('Failed to save client group', 'error');
            }
        });
    }
});

async function fetchGroupMembers(id) {
    try {
        const res = await authFetch(`${API_BASE}/client-groups/get?id=${id}`);
        const data = await res.json();
        if (data.success && data.data && data.data.clients) {
            return data.data.clients.map(c => c.mac_address);
        }
    } catch (err) { console.error(err); }
    return [];
}

async function showEditClientGroupModal(id) {
    try {
        const res = await authFetch(`${API_BASE}/client-groups/get?id=${id}`);
        const data = await res.json();
        if (!data.success || !data.data) { showAlert('Failed to load group', 'error'); return; }
        const g = data.data;

        const form = document.getElementById('edit-client-group-form');
        form.elements.id.value = g.id;
        form.elements.name.value = g.name;
        form.elements.description.value = g.description || '';
        form.elements.enabled.checked = !!g.enabled;
        form.elements.wol_broadcast_addr.value = g.wol_broadcast_addr || '';
        form.elements.stagger_delay_millis.value = g.stagger_delay_millis || 0;
        form.elements.ipmi_port.value = g.ipmi_port || '';
        form.elements.ipmi_username.value = g.ipmi_username || '';
        form.elements.ipmi_password.value = g.ipmi_password || '';
        form.elements.ipmi_insecure.checked = !!g.ipmi_insecure;
        document.getElementById('cg-props-name').textContent = g.name;

        try {
            const blRes = await authFetch(`${API_BASE}/bootloaders`);
            const blData = await blRes.json();
            const sel = document.getElementById('cg-bootloader-select');
            sel.innerHTML = '<option value="">Default (global setting)</option>';
            if (blData.success && blData.data && blData.data.sets) {
                for (const set of blData.data.sets) {
                    const isSel = g.bootloader_set === set.name ? 'selected' : '';
                    sel.innerHTML += `<option value="${escapeHtml(set.name)}" ${isSel}>${escapeHtml(set.name)}</option>`;
                }
            }
        } catch (err) {}

        await populateAutoInstallFileDropdown('cg-autoinstall-select', g.auto_install_file);

        if (!images || images.length === 0) await loadImages();
        const allowedSel = document.getElementById('cg-allowed-images');
        const groupAllowed = new Set(g.allowed_images || []);
        allowedSel.innerHTML = images.map(img => `<option value="${img.filename}" ${groupAllowed.has(img.filename) ? 'selected' : ''}>${escapeHtml(img.name)}</option>`).join('');

        if (!clients || clients.length === 0) await loadClients();
        const membersSel = document.getElementById('cg-members');
        const memberMACs = new Set((g.clients || []).map(c => c.mac_address));
        membersSel.innerHTML = clients.map(c => {
            const label = `${c.mac_address}${c.name ? ' — ' + escapeHtml(c.name) : ''}`;
            return `<option value="${c.mac_address}" ${memberMACs.has(c.mac_address) ? 'selected' : ''}>${label}</option>`;
        }).join('');

        const bulkImgSel = document.getElementById('cg-bulk-nextboot-image');
        bulkImgSel.innerHTML = '<option value="">(clear)</option>' + images.map(img => `<option value="${img.filename}">${escapeHtml(img.name)}</option>`).join('');

        await loadGroupSchedules(g.id);
        updateScheduleActionHint();

        openModal('edit-client-group-modal');
    } catch (err) {
        console.error(err);
        showAlert('Failed to load client group', 'error');
    }
}

function deleteFromEditClientGroup() {
    const form = document.getElementById('edit-client-group-form');
    const id = parseInt(form.elements.id.value, 10);
    const name = form.elements.name.value;
    if (!id) return;
    if (!confirm(`Delete client group "${name}"? Members will be detached (not deleted).`)) return;
    (async () => {
        try {
            const res = await authFetch(`${API_BASE}/client-groups/delete?id=${id}`, { method: 'DELETE' });
            const data = await res.json();
            if (data.success) {
                closeModal('edit-client-group-modal');
                showAlert('Client group deleted', 'success');
                await loadClientGroups();
            } else {
                showAlert(data.error || 'Failed to delete', 'error');
            }
        } catch (err) {
            showAlert('Failed to delete client group', 'error');
        }
    })();
}

async function bulkWakeClientGroup() {
    const id = parseInt(document.getElementById('edit-client-group-form').elements.id.value, 10);
    if (!id) return;
    try {
        const res = await authFetch(`${API_BASE}/client-groups/wake?id=${id}`, { method: 'POST' });
        const data = await res.json();
        showAlert(data.message || (data.success ? 'Wake sent' : 'Wake failed'), data.success ? 'success' : 'error');
    } catch (err) { showAlert('Failed to send bulk wake', 'error'); }
}

async function bulkSetNextBootClientGroup() {
    const id = parseInt(document.getElementById('edit-client-group-form').elements.id.value, 10);
    const image = document.getElementById('cg-bulk-nextboot-image').value;
    if (!id) return;
    if (!image) { showAlert('Pick an image first', 'error'); return; }
    try {
        const res = await authFetch(`${API_BASE}/client-groups/next-boot?id=${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ image_filename: image }),
        });
        const data = await res.json();
        showAlert(data.message || (data.success ? 'Next boot set' : 'Failed'), data.success ? 'success' : 'error');
    } catch (err) { showAlert('Failed to set bulk next boot', 'error'); }
}

async function bulkClearNextBootClientGroup() {
    const id = parseInt(document.getElementById('edit-client-group-form').elements.id.value, 10);
    if (!id) return;
    try {
        const res = await authFetch(`${API_BASE}/client-groups/next-boot?id=${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ image_filename: '' }),
        });
        const data = await res.json();
        showAlert(data.message || (data.success ? 'Cleared' : 'Failed'), data.success ? 'success' : 'error');
    } catch (err) { showAlert('Failed to clear bulk next boot', 'error'); }
}

let cachedAutoInstallFiles = [];

async function loadAutoInstallFiles() {
    const container = document.getElementById('autoinstall-table');
    if (container) {
        container.classList.add('loading');
        container.innerHTML = '<div class="spinner"></div> Loading files...';
    }
    try {
        const res = await authFetch(`${API_BASE}/autoinstall-files`);
        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Load failed');
        cachedAutoInstallFiles = data.data || [];
        renderAutoInstallFilesTable();
    } catch (err) {
        if (container) container.innerHTML = `<p class="alert alert-error">Failed to load auto-install files: ${escapeHtml(err.message)}</p>`;
    }
}

function renderAutoInstallFilesTable() {
    const container = document.getElementById('autoinstall-table');
    if (!container) return;
    container.classList.remove('loading');

    const files = (cachedAutoInstallFiles || []).filter(f => rowMatchesFilter('autoinstall', [
        f.filename, f.distro, f.type,
    ]));

    if (!files.length) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No auto-install files yet. Click <strong>New File</strong> or <strong>Upload File</strong> to add one.</p>';
        return;
    }

    const byDistro = {};
    for (const f of files) (byDistro[f.distro] = byDistro[f.distro] || []).push(f);
    const distros = Object.keys(byDistro).sort();

    const colspan = 4;
    let body = '';
    for (const d of distros) {
        const group = byDistro[d];
        body += `<tr class="tr-group"><td colspan="${colspan}" style="background: var(--bg-tertiary); user-select: none;"><strong style="color: var(--text-primary);">${escapeHtml(d)}</strong><span style="color: var(--text-muted); font-weight: 400; margin-left: 8px; font-size: 12px;">${group.length} ${group.length === 1 ? 'file' : 'files'}</span></td></tr>`;
        for (const f of group) {
            const distroAttr = escapeHtml(f.distro);
            const filenameAttr = escapeHtml(f.filename);
            body += `<tr class="row-clickable" onclick="showAutoInstallFileEditor({distro:'${distroAttr}',filename:'${filenameAttr}'})">`;
            body += `<td><code>${filenameAttr}</code></td>`;
            body += `<td><span class="badge badge-info">${escapeHtml(f.type)}</span></td>`;
            body += `<td>${formatBytes(f.size)}</td>`;
            body += `<td onclick="event.stopPropagation()"><button class="btn btn-sm" onclick="downloadAutoInstallFile('${distroAttr}','${filenameAttr}')" title="Download">⬇ Download</button></td>`;
            body += `</tr>`;
        }
    }

    container.innerHTML = `
        <div class="table-scroll">
            <table>
                <thead>
                    <tr>
                        <th>Filename</th>
                        <th>Type</th>
                        <th>Size</th>
                        <th>Operations</th>
                    </tr>
                </thead>
                <tbody>${body}</tbody>
            </table>
        </div>`;
}

function downloadAutoInstallFile(distro, filename) {
    const url = `${API_BASE}/autoinstall-files/download?distro=${encodeURIComponent(distro)}&filename=${encodeURIComponent(filename)}`;
    const token = localStorage.getItem('bootimus_token');
    fetch(url, { headers: token ? { 'Authorization': 'Bearer ' + token } : {} })
        .then(r => r.blob())
        .then(blob => {
            const a = document.createElement('a');
            a.href = URL.createObjectURL(blob);
            a.download = filename;
            a.click();
            URL.revokeObjectURL(a.href);
        })
        .catch(err => showNotification('Download failed: ' + err.message, 'error'));
}

function downloadAutoInstallFileFromEditor() {
    const distro = document.getElementById('autoinstall-editor-distro').value;
    const filename = document.getElementById('autoinstall-editor-filename').value;
    if (distro && filename) downloadAutoInstallFile(distro, filename);
}

let pendingUploadFile = null;

function handleAutoInstallUpload(input) {
    if (!input.files || !input.files[0]) return;
    pendingUploadFile = input.files[0];
    input.value = '';
    document.getElementById('autoinstall-upload-filename').textContent = pendingUploadFile.name + ' (' + formatBytes(pendingUploadFile.size) + ')';
    document.getElementById('autoinstall-upload-rename').value = '';
    const distroSel = document.getElementById('autoinstall-upload-distro');
    distroSel.innerHTML = '<option value="">Select a distro…</option>';
    authFetch(`${API_BASE}/profiles`).then(r => r.json()).then(data => {
        if (data.success && data.data) {
            for (const p of data.data) {
                distroSel.innerHTML += `<option value="${escapeHtml(p.profile_id)}">${escapeHtml(p.display_name)}</option>`;
            }
        }
    }).catch(() => {});
    openModal('autoinstall-upload-modal');
}

async function confirmAutoInstallUpload() {
    const distro = document.getElementById('autoinstall-upload-distro').value;
    const rename = document.getElementById('autoinstall-upload-rename').value.trim();
    if (!distro) {
        showNotification('Please select a distro', 'error');
        return;
    }
    if (!pendingUploadFile) {
        showNotification('No file selected', 'error');
        return;
    }
    const fd = new FormData();
    fd.append('distro', distro);
    fd.append('file', pendingUploadFile);
    if (rename) fd.append('filename', rename);
    try {
        const token = localStorage.getItem('bootimus_token');
        const res = await fetch(`${API_BASE}/autoinstall-files/upload`, {
            method: 'POST',
            headers: token ? { 'Authorization': 'Bearer ' + token } : {},
            body: fd,
        });
        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Upload failed');
        showNotification('Uploaded', 'success');
        closeModal('autoinstall-upload-modal');
        pendingUploadFile = null;
        loadAutoInstallFiles();
    } catch (err) {
        showNotification('Upload failed: ' + err.message, 'error');
    }
}

async function populateAutoInstallFileDropdown(selectId, currentValue, distroFilter) {
    const sel = document.getElementById(selectId);
    if (!sel) return;
    sel.innerHTML = '<option value="">(inherit from group or image)</option>';
    try {
        const res = await authFetch(`${API_BASE}/autoinstall-files`);
        const data = await res.json();
        if (!data.success) return;
        let files = data.data || [];
        if (distroFilter) files = files.filter(f => f.distro === distroFilter);
        const byDistro = {};
        for (const f of files) (byDistro[f.distro] = byDistro[f.distro] || []).push(f);
        for (const d of Object.keys(byDistro).sort()) {
            const og = document.createElement('optgroup');
            og.label = d;
            for (const f of byDistro[d]) {
                const opt = document.createElement('option');
                opt.value = f.path;
                opt.textContent = f.filename;
                if (currentValue === f.path) opt.selected = true;
                og.appendChild(opt);
            }
            sel.appendChild(og);
        }
    } catch (e) {}
}

async function populateImageAutoInstallFileDropdown(img) {
    const sel = document.getElementById('image-props-autoinstall-file');
    if (!sel) return;
    sel.innerHTML = '<option value="">(None — use inline script below)</option>';
    try {
        const res = await authFetch(`${API_BASE}/autoinstall-files`);
        const data = await res.json();
        if (!data.success) return;
        const all = data.data || [];
        const matching = img.distro ? all.filter(f => f.distro === img.distro) : all;
        for (const f of matching) {
            const sel_attr = (img.auto_install_file === f.path) ? 'selected' : '';
            sel.innerHTML += `<option value="${escapeHtml(f.path)}" ${sel_attr}>${escapeHtml(f.filename)}</option>`;
        }
    } catch (e) {}
}

async function showAutoInstallFileEditor(file) {
    const distroSel = document.getElementById('autoinstall-editor-distro');
    distroSel.innerHTML = '<option value="">Select a distro…</option>';
    try {
        const res = await authFetch(`${API_BASE}/profiles`);
        const data = await res.json();
        if (data.success && data.data) {
            for (const p of data.data) {
                distroSel.innerHTML += `<option value="${escapeHtml(p.profile_id)}">${escapeHtml(p.display_name)}</option>`;
            }
        }
    } catch (e) {}

    const title = document.getElementById('autoinstall-editor-title');
    const filenameInput = document.getElementById('autoinstall-editor-filename');
    const contentArea = document.getElementById('autoinstall-editor-content');
    const delBtn = document.getElementById('autoinstall-editor-delete-btn');

    const dlBtn = document.getElementById('autoinstall-editor-download-btn');
    if (file) {
        title.textContent = `Edit ${file.distro}/${file.filename}`;
        distroSel.value = file.distro;
        distroSel.disabled = true;
        filenameInput.value = file.filename;
        filenameInput.disabled = true;
        delBtn.style.display = 'inline-block';
        dlBtn.style.display = 'inline-block';
        contentArea.value = 'Loading…';
        try {
            const res = await authFetch(`${API_BASE}/autoinstall-files/get?distro=${encodeURIComponent(file.distro)}&filename=${encodeURIComponent(file.filename)}`);
            const data = await res.json();
            if (!data.success) throw new Error(data.error || 'Load failed');
            contentArea.value = data.data.content || '';
        } catch (err) {
            contentArea.value = '';
            showNotification('Failed to load file: ' + err.message, 'error');
        }
    } else {
        title.textContent = 'New Auto-Install File';
        distroSel.value = '';
        distroSel.disabled = false;
        filenameInput.value = '';
        filenameInput.disabled = false;
        contentArea.value = '';
        delBtn.style.display = 'none';
        dlBtn.style.display = 'none';
    }

    openModal('autoinstall-editor-modal');
}

async function saveAutoInstallFile() {
    const distro = document.getElementById('autoinstall-editor-distro').value.trim();
    const filename = document.getElementById('autoinstall-editor-filename').value.trim();
    const content = document.getElementById('autoinstall-editor-content').value;
    if (!distro || !filename) {
        showNotification('Distro and filename are required', 'error');
        return;
    }
    try {
        const res = await authFetch(`${API_BASE}/autoinstall-files/save`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ distro, filename, content }),
        });
        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Save failed');
        showNotification('Saved', 'success');
        closeModal('autoinstall-editor-modal');
        loadAutoInstallFiles();
    } catch (err) {
        showNotification('Save failed: ' + err.message, 'error');
    }
}

async function deleteAutoInstallFileFromEditor() {
    const distro = document.getElementById('autoinstall-editor-distro').value;
    const filename = document.getElementById('autoinstall-editor-filename').value;
    if (!confirm(`Delete ${distro}/${filename}?`)) return;
    try {
        const res = await authFetch(`${API_BASE}/autoinstall-files/delete?distro=${encodeURIComponent(distro)}&filename=${encodeURIComponent(filename)}`, { method: 'DELETE' });
        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Delete failed');
        showNotification('Deleted', 'success');
        closeModal('autoinstall-editor-modal');
        loadAutoInstallFiles();
    } catch (err) {
        showNotification('Delete failed: ' + err.message, 'error');
    }
}
