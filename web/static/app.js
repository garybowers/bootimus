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
        const backends = (data.success && data.data) || [];
        const passwordBackends = backends.filter(b => b.type !== 'redirect');
        const redirectBackends = backends.filter(b => b.type === 'redirect');
        if (passwordBackends.length > 1) {
            const select = document.getElementById('login-auth-method');
            select.innerHTML = passwordBackends.map(b =>
                `<option value="${escapeHtml(b.id)}">${escapeHtml(b.name)}</option>`
            ).join('');
            document.getElementById('login-auth-selector').style.display = 'block';
        } else {
            document.getElementById('login-auth-selector').style.display = 'none';
        }
        renderSSOButtons(redirectBackends);
    } catch (e) {
        document.getElementById('login-auth-selector').style.display = 'none';
        renderSSOButtons([]);
    }

    if (pendingLoginError) {
        const errorDiv = document.getElementById('login-error');
        errorDiv.textContent = pendingLoginError;
        errorDiv.style.display = 'block';
        pendingLoginError = null;
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

function renderSSOButtons(backends) {
    const container = document.getElementById('login-sso-buttons');
    container.innerHTML = backends.map(b =>
        `<a class="login-sso-button" href="${escapeHtml(b.url)}">${escapeHtml(t('login.signin_with', { name: b.name }))}</a>`
    ).join('');
    document.getElementById('login-sso').style.display = backends.length ? 'block' : 'none';
}

let pendingLoginError = null;

function decodeTokenClaims(token) {
    try {
        const payload = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
        return JSON.parse(atob(payload.padEnd(payload.length + (4 - payload.length % 4) % 4, '=')));
    } catch {
        return {};
    }
}

function consumeOIDCRedirect() {
    if (!location.hash.startsWith('#oidc_')) return;
    const params = new URLSearchParams(location.hash.slice(1));
    history.replaceState(null, '', location.pathname + location.search);
    const token = params.get('oidc_token');
    if (token) {
        const claims = decodeTokenClaims(token);
        setToken(token);
        localStorage.setItem('bootimus_username', claims.username || '');
        localStorage.setItem('bootimus_is_admin', claims.is_admin === true);
    }
    pendingLoginError = params.get('oidc_error');
}

function showApp() {
    applyRole();
    document.getElementById('login-screen').style.display = 'none';
    document.getElementById('main-header').style.display = '';
    document.getElementById('main-app').style.display = '';
    if (macFromHash()) handleClientDetailHash();
}

async function handleLogin(e) {
    e.preventDefault();
    const username = document.getElementById('login-username').value;
    const password = document.getElementById('login-password').value;
    const authMethod = document.getElementById('login-auth-method').value || 'local';
    const totpGroup = document.getElementById('login-totp-group');
    const codeInput = document.getElementById('login-totp-code');
    const code = totpGroup.style.display !== 'none' ? codeInput.value.trim() : '';
    const errorDiv = document.getElementById('login-error');

    try {
        const res = await fetch(`${API_BASE}/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password, auth_method: authMethod, code })
        });
        const data = await res.json();

        if (data.success) {
            setToken(data.data.token);
            localStorage.setItem('bootimus_username', data.data.username);
            localStorage.setItem('bootimus_is_admin', data.data.is_admin);
            totpGroup.style.display = 'none';
            codeInput.value = '';
            showApp();
            initApp();
        } else if (data.totp_required) {
            totpGroup.style.display = 'block';
            errorDiv.textContent = data.message || 'Enter your two-factor code';
            errorDiv.style.display = 'block';
            codeInput.focus();
        } else {
            errorDiv.textContent = data.error || 'Login failed';
            errorDiv.style.display = 'block';
            if (totpGroup.style.display !== 'none') codeInput.select();
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
    document.getElementById('login-totp-group').style.display = 'none';
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
let clientGroupedView = (() => {
    try { return localStorage.getItem('client-grouped-view') === '1'; } catch (_) { return false; }
})();
const collapsedClientGroups = (() => {
    try {
        const raw = localStorage.getItem('client-collapsed-groups');
        return new Set(raw ? JSON.parse(raw) : []);
    } catch (_) { return new Set(); }
})();
let extractionProgress = {}; // Track extraction progress by filename
const pendingUploads = new Map();

// Theme
function toggleTheme() {
    const html = document.documentElement;
    const next = html.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    html.setAttribute('data-theme', next);
    localStorage.setItem('bootimus_theme', next);
    document.querySelectorAll('.theme-switch').forEach(btn => {
        btn.setAttribute('aria-checked', next === 'dark' ? 'true' : 'false');
    });
}

function loadSavedTheme() {
    const saved = localStorage.getItem('bootimus_theme') || 'light';
    document.documentElement.setAttribute('data-theme', saved);
    document.querySelectorAll('.theme-switch').forEach(btn => {
        btn.setAttribute('aria-checked', saved === 'dark' ? 'true' : 'false');
    });
}

// Utility Functions
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function openModal(modalId) {
    const modal = document.getElementById(modalId);
    applyReadOnlyToModal(modal);
    modal.classList.add('active');
}

function closeModal(modalId) {
    document.getElementById(modalId).classList.remove('active');
    if (modalId === 'get-images-modal' && typeof clearGetISOPolling === 'function') clearGetISOPolling();
    if (modalId === 'console-modal') disconnectClientConsole();
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
    document.getElementById('current-username').textContent = username;
    const admin = localStorage.getItem('bootimus_is_admin') === 'true';
    document.getElementById('current-user-role').textContent = admin ? (isReplica() ? 'Administrator (replica)' : 'Administrator') : 'Read-only';
}

function isReadOnly() {
    return localStorage.getItem('bootimus_is_admin') !== 'true' || isReplica();
}

function isReplica() {
    return !!replicaInfo();
}

function replicaInfo() {
    try {
        const raw = localStorage.getItem('bootimus_replica');
        return raw ? JSON.parse(raw) : null;
    } catch (_) {
        return null;
    }
}

function renderReplicaBanner() {
    const banner = document.getElementById('replica-banner');
    const info = replicaInfo();
    document.body.classList.toggle('replica', !!info);
    if (!banner || !info) return;
    const last = info.last_sync ? new Date(info.last_sync).toLocaleString() : 'never';
    let text = `Replica of ${escapeHtml(info.primary_url || '')}. Last successful sync: ${escapeHtml(last)}. Make changes on the primary.`;
    if (info.last_error) text += ` Last sync error: ${escapeHtml(info.last_error)}`;
    banner.innerHTML = text;
}

function applyRole() {
    const readOnly = isReadOnly();
    document.body.classList.toggle('read-only', readOnly);
    renderReplicaBanner();
    loadCurrentUser();
    const active = document.querySelector('.sidebar-nav .nav-item.active');
    if (readOnly && active && active.classList.contains('admin-only')) {
        document.querySelector('.sidebar-nav .nav-item[data-tab="server"]').click();
    }
}

async function refreshRole() {
    try {
        const res = await authFetch(`${API_BASE}/me`);
        const data = await res.json();
        if (data.success) {
            localStorage.setItem('bootimus_username', data.data.username);
            localStorage.setItem('bootimus_is_admin', data.data.is_admin);
            if (data.data.replica) {
                localStorage.setItem('bootimus_replica', JSON.stringify(data.data.replica));
            } else {
                localStorage.removeItem('bootimus_replica');
            }
            applyRole();
        }
    } catch (err) {
        console.error('Failed to refresh role:', err);
    }
}

function applyReadOnlyToModal(modal) {
    const readOnly = isReadOnly();
    modal.querySelectorAll('form input, form select, form textarea').forEach(el => {
        if (readOnly && !el.disabled) {
            el.disabled = true;
            el.dataset.readOnlyDisabled = '1';
        } else if (!readOnly && el.dataset.readOnlyDisabled) {
            el.disabled = false;
            delete el.dataset.readOnlyDisabled;
        }
    });
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

    // Set background colour based on type
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

let appInitialised = false;

function initApp() {
    if (appInitialised) return;
    appInitialised = true;

    refreshRole();
    loadStats();
    loadServerInfo();
    loadReplicaStatus();
    loadProfileCache();
    loadClients();
    loadImages();
    loadPublicFiles();
    loadLogs();
    if (!isReadOnly()) { loadUsers(); loadAPITokens(); }
    loadTOTPStatus();
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
        if (activeTab === 'users' && !isReadOnly()) loadUsers();
    }, 30000);

    // Refresh server info more frequently for live stats (every 5 seconds)
    setInterval(() => {
        const activeTab = (document.querySelector('.nav-item.active') || document.querySelector('.tab.active')).dataset.tab;
        if (activeTab === 'server') { loadServerInfo(); loadReplicaStatus(); }
    }, 5000);

    // Refresh active sessions more frequently (every 3 seconds)
    setInterval(loadActiveSessions, 3000);
}

// Initialise
document.addEventListener('DOMContentLoaded', () => {
    loadSavedTheme();
    setupTabs();
    setupForms();
    setupUpload();
    document.getElementById('login-form').addEventListener('submit', handleLogin);
    window.addEventListener('popstate', handleClientDetailHash);
    consumeOIDCRedirect();
    checkAuth();
});

// Tab Management
function setupTabs() {
    // Setup sidebar nav items
    document.querySelectorAll('.sidebar-nav .nav-item').forEach(item => {
        item.addEventListener('click', () => {
            if (clientDetailMac) {
                clientDetailMac = null;
                clientDetailClient = null;
                if (location.hash.startsWith('#clients/')) history.replaceState(null, '', location.pathname + location.search);
            }
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
            if (item.dataset.tab === 'clients') { loadClientGroups(); loadDeploymentProfiles(true); loadEnrolmentRules(true); }
            if (item.dataset.tab === 'tools') loadTools();
            if (item.dataset.tab === 'bootloaders') loadBootloaders();
            if (item.dataset.tab === 'profiles') loadProfiles();
            if (item.dataset.tab === 'autoinstall') loadAutoInstallFiles();
            if (item.dataset.tab === 'clusters') loadClustersTab();
            if (item.dataset.tab === 'boot-menu') { loadTheme(); populateMenuPreviewMACs(); }
            if (item.dataset.tab === 'settings') { loadUSBImages(); loadWebhookConfig(); loadRestoreStatus(); loadHousekeeping(); }
            if (item.dataset.tab === 'api-reference') showAPIReference();
            if (item.dataset.tab === 'audit') loadAuditLog();
            if (item.dataset.tab === 'pxe-debug') loadPXEDebug();
            if (item.dataset.tab === 'disk-images') loadDiskImages();
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
            const updates = document.getElementById('stat-updates');
            if (updates) updates.textContent = data.data.updates_available ?? 0;
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

let consoleSocket = null;
let consoleTerm = null;
let consoleFit = null;

function consoleSetStatus(text, colour) {
    const el = document.getElementById('console-status');
    if (!el) return;
    el.textContent = text;
    el.style.color = colour || 'var(--text-secondary)';
}

function openClientConsole(mac) {
    if (!mac) {
        const form = document.getElementById('edit-client-form');
        mac = form ? form.querySelector('[name="mac_address"]').value : '';
    }
    if (!mac) return;
    disconnectClientConsole();
    document.getElementById('console-title').textContent = mac;
    showModal('console-modal');

    const host = document.getElementById('console-terminal');
    host.innerHTML = '';
    consoleTerm = new Terminal({
        cursorBlink: true,
        fontSize: 13,
        fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
        theme: { background: '#000000' },
    });
    consoleFit = new FitAddon.FitAddon();
    consoleTerm.loadAddon(consoleFit);
    consoleTerm.open(host);
    consoleFit.fit();

    const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${scheme}://${window.location.host}${API_BASE}/clients/console?mac=${encodeURIComponent(mac)}`;
    const token = getToken();
    const socket = token ? new WebSocket(url, ['bearer', token]) : new WebSocket(url);
    socket.binaryType = 'arraybuffer';
    consoleSocket = socket;
    consoleSetStatus('Connecting…');

    const encoder = new TextEncoder();
    socket.onopen = () => {
        consoleSetStatus('Opening console…');
        socket.send(JSON.stringify({ type: 'resize', cols: consoleTerm.cols, rows: consoleTerm.rows }));
    };
    socket.onmessage = (ev) => {
        if (!consoleTerm) return;
        if (typeof ev.data === 'string') {
            let msg = null;
            try { msg = JSON.parse(ev.data); } catch (_) { }
            if (!msg || !msg.type) {
                consoleTerm.write(ev.data);
                return;
            }
            const failed = msg.type === 'error' || msg.type === 'closed';
            consoleSetStatus(msg.message || msg.type, failed ? 'var(--danger)' : 'var(--teal, green)');
            if (failed) consoleTerm.writeln(`\r\n[${msg.message || msg.type}]`);
            return;
        }
        consoleTerm.write(new Uint8Array(ev.data));
    };
    socket.onclose = () => {
        if (consoleSocket === socket) {
            consoleSocket = null;
            const status = document.getElementById('console-status');
            if (status && !status.textContent.startsWith('Console closed') && !status.textContent.startsWith('Intel') && status.style.color !== 'var(--danger)') {
                consoleSetStatus('Disconnected');
            }
        }
    };
    socket.onerror = () => consoleSetStatus('Connection error', 'var(--danger)');
    consoleTerm.onData((data) => {
        if (socket.readyState === WebSocket.OPEN) socket.send(encoder.encode(data));
    });
    consoleTerm.focus();
}

function disconnectClientConsole() {
    if (consoleSocket) {
        const socket = consoleSocket;
        consoleSocket = null;
        socket.close();
        consoleSetStatus('Disconnected');
    }
    if (consoleTerm) {
        consoleTerm.dispose();
        consoleTerm = null;
        consoleFit = null;
    }
}

window.addEventListener('resize', () => {
    if (consoleFit) consoleFit.fit();
});

function updateBMCPortPlaceholder(select) {
    const port = select.form.querySelector('[name="ipmi_port"]');
    if (!port) return;
    port.placeholder = select.value === 'amt' ? '16992 (16993 with TLS)' : '443';
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
            label.textContent = 'Power Action';
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
        document.getElementById('webhook-on-admin-action').checked = !!c.on_admin_action;
        document.getElementById('webhook-on-disk-imaging').checked = c.on_disk_imaging !== false;
        document.getElementById('webhook-on-install').checked = !!c.on_install;
        document.getElementById('webhook-on-auto-enrol').checked = !!c.on_auto_enrol;
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
        on_admin_action: document.getElementById('webhook-on-admin-action').checked,
        on_disk_imaging: document.getElementById('webhook-on-disk-imaging').checked,
        on_install: document.getElementById('webhook-on-install').checked,
        on_auto_enrol: document.getElementById('webhook-on-auto-enrol').checked,
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

const HOUSEKEEPING_TASK_LABELS = {
    boot_logs: 'Boot logs',
    inventory_history: 'Inventory history',
    discovered_clients: 'Discovered clients',
    audit_logs: 'Audit log',
    catalogue_check: 'Catalogue check',
};

function renderHousekeeping(view) {
    const settings = view.settings || {};
    document.getElementById('housekeeping-boot-log-days').value = settings.boot_log_days ?? 0;
    document.getElementById('housekeeping-inventory-days').value = settings.inventory_history_days ?? 0;
    document.getElementById('housekeeping-discovered-days').value = settings.discovered_client_days ?? 0;
    document.getElementById('housekeeping-audit-days').value = settings.audit_log_days ?? 0;
    document.getElementById('housekeeping-catalogue-check').checked = !settings.catalogue_check_off;

    const lastRun = document.getElementById('housekeeping-last-run');
    if (settings.last_run) {
        const status = settings.last_status || 'ok';
        const badge = status === 'ok' ? 'badge-success' : (status === 'partial' ? 'badge-warning' : 'badge-danger');
        lastRun.innerHTML = `Last run ${new Date(settings.last_run).toLocaleString()} <span class="badge ${badge}">${escapeHtml(status)}</span>`;
    } else {
        lastRun.textContent = 'Not run yet';
    }

    const reportEl = document.getElementById('housekeeping-report');
    const report = view.last_report;
    if (report && Array.isArray(report.tasks) && report.tasks.length) {
        let html = '<div class="table-scroll"><table><thead><tr><th>Task</th><th>Result</th></tr></thead><tbody>';
        for (const t of report.tasks) {
            const label = HOUSEKEEPING_TASK_LABELS[t.name] || t.name;
            let result;
            if (t.error) result = `<span style="color: var(--danger);">${escapeHtml(t.error)}</span>`;
            else if (t.skipped) result = '<span style="color: var(--text-muted);">Disabled</span>';
            else if (t.name === 'catalogue_check') result = `${t.affected} update${t.affected === 1 ? '' : 's'} available`;
            else result = `${t.affected} removed`;
            html += `<tr><td>${escapeHtml(label)}</td><td>${result}</td></tr>`;
        }
        html += '</tbody></table></div>';
        reportEl.innerHTML = html;
    } else {
        reportEl.innerHTML = '';
    }

    const usageEl = document.getElementById('housekeeping-usage');
    if (view.usage && Array.isArray(view.usage.entries)) {
        let html = `<div class="table-scroll"><table><thead><tr><th>Entry</th><th>Files</th><th>Size</th></tr></thead><tbody>`;
        for (const e of view.usage.entries) {
            html += `<tr><td><code>${escapeHtml(e.name)}${e.dir ? '/' : ''}</code></td><td>${e.files}</td><td>${formatBytes(e.bytes)}</td></tr>`;
        }
        html += `<tr><td><strong>Total</strong></td><td></td><td><strong>${formatBytes(view.usage.total_bytes)}</strong></td></tr>`;
        html += '</tbody></table></div>';
        html += `<div style="font-size: 12px; color: var(--text-muted); margin-top: 6px;">${escapeHtml(view.usage.data_dir)} · measured ${new Date(view.usage.measured_at).toLocaleTimeString()}</div>`;
        usageEl.innerHTML = html;
    } else {
        usageEl.textContent = view.usage_error ? 'Could not measure data directory: ' + view.usage_error : 'No usage information available.';
    }
}

async function checkCatalogueNow() {
    const btn = document.getElementById('catalogue-check-btn');
    const out = document.getElementById('catalogue-check-result');
    btn.disabled = true;
    out.textContent = 'Checking the catalogue…';
    try {
        const res = await authFetch(`${API_BASE}/catalogue/check`, { method: 'POST' });
        const data = await res.json();
        if (!data.success) {
            out.innerHTML = `<span style="color: var(--danger);">${escapeHtml(data.error || 'Check failed')}</span>`;
            return;
        }
        const r = data.data;
        const parts = [];
        for (const i of (r.images || [])) parts.push(`${escapeHtml(i.name)} → ${escapeHtml(i.version || i.filename)}`);
        for (const t of (r.tools || [])) parts.push(`${escapeHtml(t.display_name || t.name)} (tool)`);
        const source = r.remote ? 'online catalogue' : 'built-in catalogue';
        out.innerHTML = parts.length
            ? `${parts.length} update${parts.length === 1 ? '' : 's'} available (${source}): ${parts.join(', ')}`
            : `Everything is current against the ${source}.`;
        loadImages();
        loadTools();
        loadStats();
    } catch (err) {
        out.innerHTML = `<span style="color: var(--danger);">Check failed: ${escapeHtml(err.message)}</span>`;
    } finally {
        btn.disabled = false;
    }
}

async function loadHousekeeping() {
    if (!document.getElementById('housekeeping-form')) return;
    try {
        const res = await authFetch(`${API_BASE}/housekeeping`);
        const data = await res.json();
        if (data.success && data.data) renderHousekeeping(data.data);
    } catch (err) {
        console.error('Failed to load housekeeping settings:', err);
    }
}

async function saveHousekeeping(e) {
    if (e) e.preventDefault();
    const body = {
        boot_log_days: parseInt(document.getElementById('housekeeping-boot-log-days').value, 10) || 0,
        inventory_history_days: parseInt(document.getElementById('housekeeping-inventory-days').value, 10) || 0,
        discovered_client_days: parseInt(document.getElementById('housekeeping-discovered-days').value, 10) || 0,
        audit_log_days: parseInt(document.getElementById('housekeeping-audit-days').value, 10) || 0,
        catalogue_check_off: !document.getElementById('housekeeping-catalogue-check').checked,
    };
    try {
        const res = await authFetch(`${API_BASE}/housekeeping`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (data.success) {
            showAlert('Housekeeping settings saved', 'success');
            loadHousekeeping();
        } else {
            showAlert(data.error || 'Save failed', 'error');
        }
    } catch (err) {
        showAlert('Save failed: ' + err.message, 'error');
    }
}

async function runHousekeeping() {
    const btn = document.getElementById('housekeeping-run-btn');
    btn.disabled = true;
    try {
        const res = await authFetch(`${API_BASE}/housekeeping/run`, { method: 'POST' });
        const data = await res.json();
        if (data.data) renderHousekeeping(data.data);
        showAlert(data.message || data.error || 'Housekeeping finished', data.success ? 'success' : 'error');
    } catch (err) {
        showAlert('Housekeeping failed: ' + err.message, 'error');
    } finally {
        btn.disabled = false;
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

function setRestorePendingNotice(pending) {
    const notice = document.getElementById('restore-pending-notice');
    if (notice) notice.style.display = pending ? 'flex' : 'none';
}

function setRestoreFailedNotice(failure) {
    const notice = document.getElementById('restore-failed-notice');
    const text = document.getElementById('restore-failed-text');
    if (!notice || !text) return;
    if (!failure) {
        notice.style.display = 'none';
        return;
    }
    const when = failure.failed_at ? new Date(failure.failed_at).toLocaleString() : '';
    text.textContent = `The staged restore could not be applied${when ? ' on ' + when : ''}: ${failure.error || 'unknown error'}. Bootimus started with the current database; the archive was moved to .restore-failed in the data directory.`;
    notice.style.display = 'flex';
}

async function loadRestoreStatus() {
    try {
        const res = await authFetch(`${API_BASE}/backup/import`);
        const data = await res.json();
        setRestorePendingNotice(!!(data.success && data.data && data.data.pending));
        setRestoreFailedNotice(data.success && data.data ? data.data.last_failure : null);
    } catch (err) {
        console.error('Failed to load restore status:', err);
    }
}

async function uploadRestore() {
    const input = document.getElementById('restore-file');
    const status = document.getElementById('restore-status');
    const btn = document.getElementById('restore-upload-btn');
    const file = input && input.files && input.files[0];
    if (!file) {
        showAlert('Choose a backup file first', 'error');
        return;
    }
    if (!confirm(`Restore "${file.name}"?\n\nThe current database, bootloader configuration and settings will be replaced the next time Bootimus restarts. ISOs are not affected.`)) return;
    const form = new FormData();
    form.append('backup', file);
    btn.disabled = true;
    status.textContent = 'Uploading…';
    try {
        const res = await authFetch(`${API_BASE}/backup/import`, { method: 'POST', body: form });
        const data = await res.json();
        if (data.success) {
            status.textContent = '';
            input.value = '';
            setRestorePendingNotice(true);
            showAlert(data.message || 'Restore staged. Restart Bootimus to apply it.', 'success');
        } else {
            status.textContent = '';
            showAlert(data.error || 'Restore upload failed', 'error');
        }
    } catch (err) {
        status.textContent = '';
        showAlert('Restore upload failed: ' + err.message, 'error');
    } finally {
        btn.disabled = false;
    }
}

async function cancelRestore() {
    if (!confirm('Discard the staged or failed restore?')) return;
    try {
        const res = await authFetch(`${API_BASE}/backup/import`, { method: 'DELETE' });
        const data = await res.json();
        if (data.success) {
            setRestorePendingNotice(false);
            setRestoreFailedNotice(null);
            showAlert(data.message || 'Staged restore cancelled', 'success');
        } else {
            showAlert(data.error || 'Failed to cancel restore', 'error');
        }
    } catch (err) {
        showAlert('Failed to cancel restore: ' + err.message, 'error');
    }
}

function downloadISOFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    if (!filename) return;
    const url = `http://${window.location.hostname}:${cachedHTTPPort}/isos/${encodeURIComponent(filename)}`;
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
    if (info.configuration && info.configuration.windows_smb) {
        const smb = info.configuration.windows_smb;
        const split = smb.search(/ \(|: /);
        const state = split >= 0 ? smb.slice(0, split) : smb;
        const detail = split >= 0 ? smb.slice(split).replace(/^[ :(]+|\)$/g, '') : '';
        const badge = smb.startsWith('Enabled') ? 'badge-success' : (smb.startsWith('Disabled') ? 'badge-disabled' : 'badge-warning');
        statusCards += `<div class="rs-metric"><span class="rs-label">${t('server.field.windows_smb')}</span><span class="rs-value"><span class="badge ${badge}">${escapeHtml(state)}</span>${detail ? ' <span style="color: var(--text-secondary); font-size: 12px;">' + escapeHtml(detail) + '</span>' : ''}</span></div>`;
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
            if (clientDetailMac) refreshClientDetail();
        }
    } catch (err) {
        document.getElementById('clients-table').innerHTML = '<p class="alert alert-error">Failed to load clients</p>';
    }
}

const selectedClientMacs = new Set();

function toggleClientSelection(mac, checked) {
    if (checked) selectedClientMacs.add(mac);
    else selectedClientMacs.delete(mac);
    updateClientsBulkUI();
}

function clearClientSelection() {
    selectedClientMacs.clear();
    renderClientsTable();
}

function toggleClientsSelectAll(checked) {
    selectedClientMacs.clear();
    if (checked) {
        for (const c of (clients || [])) selectedClientMacs.add(c.mac_address);
    }
    renderClientsTable();
}

function updateClientsBulkUI() {
    const wrap = document.getElementById('clients-bulk-actions');
    const count = document.getElementById('clients-bulk-count');
    const n = selectedClientMacs.size;
    if (wrap) wrap.style.display = n > 0 ? 'flex' : 'none';
    if (count) count.textContent = n + ' selected';
    const selectAll = document.getElementById('clients-select-all');
    if (selectAll) {
        const total = (clients || []).length;
        if (n === 0) { selectAll.checked = false; selectAll.indeterminate = false; }
        else if (n >= total) { selectAll.checked = true; selectAll.indeterminate = false; }
        else { selectAll.checked = false; selectAll.indeterminate = true; }
    }
}

async function bulkSetClientsEnabled(enabled) {
    const macs = Array.from(selectedClientMacs);
    if (!macs.length) return;
    let success = 0, fail = 0;
    for (const mac of macs) {
        try {
            const res = await authFetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ enabled }),
            });
            const data = await res.json();
            if (data.success) success++; else fail++;
        } catch (e) { fail++; }
    }
    const verb = enabled ? 'Enabled' : 'Disabled';
    showAlert(`${verb} ${success}${fail ? ', ' + fail + ' failed' : ''}`, fail ? 'error' : 'success');
    selectedClientMacs.clear();
    await loadClients();
    loadStats();
}

async function bulkWakeClients() {
    const macs = Array.from(selectedClientMacs);
    if (!macs.length) return;
    let success = 0, fail = 0;
    for (const mac of macs) {
        try {
            const res = await authFetch(`${API_BASE}/clients/wake?mac=${encodeURIComponent(mac)}`, { method: 'POST' });
            const data = await res.json();
            if (data.success) success++; else fail++;
        } catch (e) { fail++; }
    }
    showAlert(`Wake sent to ${success}${fail ? ', ' + fail + ' failed' : ''}`, fail ? 'error' : 'success');
}

async function bulkDeleteClients() {
    const macs = Array.from(selectedClientMacs);
    if (!macs.length) return;
    if (!confirm(`Delete ${macs.length} client${macs.length === 1 ? '' : 's'}?`)) return;
    let success = 0, fail = 0;
    for (const mac of macs) {
        try {
            const res = await authFetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`, { method: 'DELETE' });
            const data = await res.json();
            if (data.success) success++; else fail++;
        } catch (e) { fail++; }
    }
    showAlert(`Deleted ${success}${fail ? ', ' + fail + ' failed' : ''}`, fail ? 'error' : 'success');
    selectedClientMacs.clear();
    await loadClients();
    loadStats();
}

async function openBulkAssignClientGroupModal() {
    if (!selectedClientMacs.size) return;
    const select = document.getElementById('bulk-assign-client-group-select');
    select.innerHTML = '<option value="">(none)</option>';
    try {
        const res = await authFetch(`${API_BASE}/client-groups`);
        const data = await res.json();
        if (data.success && data.data) {
            for (const g of data.data) {
                select.innerHTML += `<option value="${g.id}">${escapeHtml(g.name)}</option>`;
            }
        }
    } catch (e) {}
    const n = selectedClientMacs.size;
    document.getElementById('bulk-assign-client-group-count').textContent = n + ' client' + (n === 1 ? '' : 's');
    openModal('bulk-assign-client-group-modal');
}

async function confirmBulkAssignClientGroup() {
    const macs = Array.from(selectedClientMacs);
    if (!macs.length) {
        closeModal('bulk-assign-client-group-modal');
        return;
    }
    const raw = document.getElementById('bulk-assign-client-group-select').value;
    const body = JSON.stringify({ client_group_id: raw === '' ? null : parseInt(raw, 10) });
    let success = 0, fail = 0;
    for (const mac of macs) {
        try {
            const res = await authFetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body,
            });
            const data = await res.json();
            if (data.success) success++; else fail++;
        } catch (e) { fail++; }
    }
    showAlert(`Assigned ${success}${fail ? ', ' + fail + ' failed' : ''}`, fail ? 'error' : 'success');
    selectedClientMacs.clear();
    closeModal('bulk-assign-client-group-modal');
    await loadClients();
}

function toggleClientGrouping() {
    clientGroupedView = !clientGroupedView;
    try { localStorage.setItem('client-grouped-view', clientGroupedView ? '1' : '0'); } catch (_) {}
    syncClientGroupingButton();
    renderClientsTable();
}

function syncClientGroupingButton() {
    const btn = document.getElementById('clients-group-toggle');
    if (!btn) return;
    btn.setAttribute('aria-pressed', clientGroupedView ? 'true' : 'false');
}

function toggleClientGroup(key) {
    if (collapsedClientGroups.has(key)) collapsedClientGroups.delete(key);
    else collapsedClientGroups.add(key);
    try { localStorage.setItem('client-collapsed-groups', JSON.stringify([...collapsedClientGroups])); } catch (_) {}
    renderClientsTable();
}

function toggleClientGroupFromEvent(el) {
    if (!el || !el.dataset) return;
    try { toggleClientGroup(JSON.parse(el.dataset.groupKey)); } catch (_) {}
}

function clientGroupName(client) {
    if (client.client_group && client.client_group.name) return client.client_group.name;
    if (client.client_group_id != null) {
        const g = (clientGroups || []).find(g => g.id === client.client_group_id);
        if (g) return g.name;
    }
    return null;
}

function clientRowHTML(client, includeGroupCell) {
    const groupName = clientGroupName(client);
    const groupCell = includeGroupCell ? `
                        <td>
                            ${groupName ?
                                '<span class="badge badge-info">' + escapeHtml(groupName) + '</span>' :
                                '<span style="color: var(--text-secondary);">-</span>'
                            }
                        </td>` : '';
    return `
                    <tr class="row-clickable" onclick="showClientDetail('${client.mac_address}')">
                        <td class="col-check" onclick="event.stopPropagation()"><input type="checkbox" ${selectedClientMacs.has(client.mac_address) ? 'checked' : ''} onchange="toggleClientSelection('${client.mac_address}', this.checked)"></td>
                        <td><code>${client.mac_address}</code></td>
                        <td>${client.name || '-'}${client.enrollment_state ? ' ' + enrollmentStateBadge(client.enrollment_state) : ''}${deploymentProfileBadge(client)}</td>
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
                        </td>${groupCell}
                        <td>
                            ${(client.images || []).length > 0 ?
                                `<span title="${(client.images || []).map(i => i.name).join(', ')}">${(client.images || []).length} images</span>` :
                                '<span style="color: var(--text-secondary);">No images</span>'
                            }
                        </td>
                        <td>${client.boot_count || 0}</td>
                        <td>
                            ${client.last_boot ? new Date(client.last_boot).toLocaleString() : 'Never'}
                            ${client.next_boot_image ? '<br><span class="badge badge-info" title="' + escapeHtml(client.next_boot_image) + '">Next: ' + escapeHtml(nextBootLabel(client.next_boot_image)) + '</span>' : ''}
                        </td>
                        <td>${installStateBadge(client)}</td>
                        <td onclick="event.stopPropagation()">
                            ${!client.static ? '<button class="btn btn-success btn-sm" onclick="promoteClient(\'' + client.mac_address + '\')">Make Static</button>' : ''}
                            <button class="btn btn-success btn-sm" onclick="wakeClient('${client.mac_address}')">Wake</button>
                            <button class="btn btn-primary btn-sm" onclick="showNextBoot('${client.mac_address}')">Next Boot</button>
                            ${client.ipmi_host ? '<button class="btn btn-sm" onclick="openClientConsole(\'' + client.mac_address + '\')">Console</button>' : ''}
                            <button class="btn btn-sm" onclick="previewBootMenu('${client.mac_address}')">Preview Menu</button>
                            <button class="btn btn-sm" onclick="showCaptureDiskModal('${client.mac_address}')">Capture Disk</button>
                            <button class="btn btn-sm" onclick="showDeployDiskModal({ mac: '${client.mac_address}' })">Deploy Image</button>
                        </td>
                    </tr>`;
}

function renderClientsTable() {
    const container = document.getElementById('clients-table');

    const filteredClients = clients.filter(c => rowMatchesFilter('clients', [c.mac_address, c.name, c.description, c.bootloader_set, clientGroupName(c)]));

    if (clients.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No clients yet. Add one to get started.</p>';
        return;
    }

    syncClientGroupingButton();

    const filterActive = !!(tableFilters['clients'] && tableFilters['clients'].length);

    let bodyHtml;
    let theadHtml;
    let colspan;

    if (!clientGroupedView) {
        colspan = 12;
        theadHtml = `
                <tr>
                    <th class="col-check"><input type="checkbox" id="clients-select-all" onchange="toggleClientsSelectAll(this.checked)" title="Select all"></th>
                    <th>MAC Address</th>
                    <th>Name</th>
                    <th>Type</th>
                    <th class="col-dot" title="Enabled / Disabled">On</th>
                    <th>Bootloader</th>
                    <th>Group</th>
                    <th>Assigned Images</th>
                    <th>Boot Count</th>
                    <th>Last Boot</th>
                    <th>Install</th>
                    <th>Quick Actions</th>
                </tr>`;
        bodyHtml = filteredClients.map(c => clientRowHTML(c, true)).join('');
    } else {
        colspan = 11;
        theadHtml = `
                <tr>
                    <th class="col-check"><input type="checkbox" id="clients-select-all" onchange="toggleClientsSelectAll(this.checked)" title="Select all"></th>
                    <th>MAC Address</th>
                    <th>Name</th>
                    <th>Type</th>
                    <th class="col-dot" title="Enabled / Disabled">On</th>
                    <th>Bootloader</th>
                    <th>Assigned Images</th>
                    <th>Boot Count</th>
                    <th>Last Boot</th>
                    <th>Install</th>
                    <th>Quick Actions</th>
                </tr>`;

        const buckets = new Map();
        const ungrouped = [];
        for (const c of filteredClients) {
            const gid = c.client_group_id;
            if (gid != null) {
                if (!buckets.has(gid)) buckets.set(gid, []);
                buckets.get(gid).push(c);
            } else {
                ungrouped.push(c);
            }
        }

        let out = '';
        const sortedGroups = (clientGroups || []).slice().sort((a, b) => (a.name || '').localeCompare(b.name || ''));
        for (const group of sortedGroups) {
            const members = buckets.get(group.id) || [];
            if (members.length === 0) continue;
            const collapsed = !filterActive && collapsedClientGroups.has('id:' + group.id);
            const dataKey = _attrEscape(JSON.stringify('id:' + group.id));
            out += `
                    <tr class="tr-group" data-group-key="${dataKey}" onclick="toggleClientGroupFromEvent(this)">
                        <td colspan="${colspan}" style="background: var(--bg-tertiary); cursor: pointer; user-select: none;">
                            <span style="display: inline-block; width: 14px; color: var(--text-secondary);">${collapsed ? '▶' : '▼'}</span>
                            <strong style="color: var(--text-primary);">${escapeHtml(group.name)}</strong>
                            <span style="color: var(--text-muted); font-weight: 400; margin-left: 8px; font-size: 12px;">${members.length} ${members.length === 1 ? 'client' : 'clients'}</span>
                        </td>
                    </tr>`;
            if (!collapsed) {
                for (const c of members) out += clientRowHTML(c, false);
            }
        }

        if (ungrouped.length > 0) {
            const collapsed = !filterActive && collapsedClientGroups.has('id:ungrouped');
            const dataKey = _attrEscape(JSON.stringify('id:ungrouped'));
            out += `
                    <tr class="tr-group" data-group-key="${dataKey}" onclick="toggleClientGroupFromEvent(this)">
                        <td colspan="${colspan}" style="background: var(--bg-tertiary); cursor: pointer; user-select: none;">
                            <span style="display: inline-block; width: 14px; color: var(--text-secondary);">${collapsed ? '▶' : '▼'}</span>
                            <strong style="color: var(--text-primary);">Ungrouped</strong>
                            <span style="color: var(--text-muted); font-weight: 400; margin-left: 8px; font-size: 12px;">${ungrouped.length} ${ungrouped.length === 1 ? 'client' : 'clients'}</span>
                        </td>
                    </tr>`;
            if (!collapsed) {
                for (const c of ungrouped) out += clientRowHTML(c, false);
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
    updateClientsBulkUI();
}

function showAddClientModal() {
    document.getElementById('add-client-form').reset();
    showModal('add-client-modal');
}

let clientDetailMac = null;
let clientDetailClient = null;
let clientDetailTimelineBefore = '';
let clientDetailTimelineItems = [];
let clientDetailInventory = null;
let clientDetailInventoryHistory = null;

const TIMELINE_KIND_LABELS = { boot: 'Boot', install: 'Install', disk: 'Disk', dhcp: 'DHCP', audit: 'Change' };

function clientDetailHash(mac) {
    return '#clients/' + encodeURIComponent(mac);
}

function macFromHash() {
    const h = location.hash || '';
    if (!h.startsWith('#clients/')) return '';
    try { return decodeURIComponent(h.slice('#clients/'.length)).toLowerCase(); } catch (_) { return ''; }
}

function activateClientDetailPanel() {
    document.querySelectorAll('.sidebar-nav .nav-item').forEach(n => n.classList.toggle('active', n.dataset.tab === 'clients'));
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
    document.getElementById('client-detail-tab').classList.add('active');
}

async function showClientDetail(mac, options) {
    if (!mac) return;
    mac = mac.toLowerCase();
    const opts = options || {};
    if (!opts.fromHash && location.hash !== clientDetailHash(mac)) {
        history.pushState({ clientDetail: mac }, '', clientDetailHash(mac));
    }
    clientDetailMac = mac;
    clientDetailTimelineBefore = '';
    clientDetailTimelineItems = [];
    clientDetailInventory = null;
    clientDetailInventoryHistory = null;
    activateClientDetailPanel();
    document.getElementById('client-detail-header').innerHTML = '<p style="color: var(--text-secondary); margin: 0;">Loading…</p>';
    document.getElementById('client-detail-actions').innerHTML = '';
    document.getElementById('client-detail-hardware-history').style.display = 'none';
    document.getElementById('client-detail-hw-toggle').textContent = 'Show history';
    await refreshClientDetail();
}

async function refreshClientDetail() {
    const mac = clientDetailMac;
    if (!mac) return;
    try {
        const res = await authFetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`);
        const data = await res.json();
        if (!data.success || !data.data) {
            document.getElementById('client-detail-header').innerHTML = `<p class="alert alert-error">${escapeHtml(data.error || 'Client not found')}</p>`;
            return;
        }
        if (clientDetailMac !== mac) return;
        clientDetailClient = data.data;
        if (!sanTargetsLoaded) await loadSANTargets(true);
        renderClientDetailHeader(clientDetailClient);
        renderClientDetailActions(clientDetailClient);
        renderClientDetailOverview(clientDetailClient);
        renderClientDetailProvisioning(clientDetailClient);
        await Promise.all([
            loadClientDetailProvisioning(mac, clientDetailClient),
            loadClientDetailHardware(mac),
            loadClientInstallHistory(mac, 'client-detail-installs'),
            loadClientTimeline(mac, true),
        ]);
    } catch (err) {
        document.getElementById('client-detail-header').innerHTML = '<p class="alert alert-error">Failed to load client</p>';
    }
}

function closeClientDetail() {
    clientDetailMac = null;
    clientDetailClient = null;
    if (location.hash.startsWith('#clients/')) history.pushState(null, '', location.pathname + location.search);
    const nav = document.querySelector('.sidebar-nav .nav-item[data-tab="clients"]');
    if (nav) nav.click();
}

function handleClientDetailHash() {
    const mac = macFromHash();
    if (mac) {
        if (mac !== clientDetailMac) showClientDetail(mac, { fromHash: true });
        return;
    }
    if (clientDetailMac) {
        clientDetailMac = null;
        clientDetailClient = null;
        const nav = document.querySelector('.sidebar-nav .nav-item[data-tab="clients"]');
        if (nav) nav.click();
    }
}

function clientDetailBadges(client) {
    const badges = [];
    badges.push(`<span class="badge ${client.static ? 'badge-success' : 'badge-info'}">${client.static ? 'Static' : 'Discovered'}</span>`);
    badges.push(`<span class="badge ${client.enabled ? 'badge-success' : 'badge-danger'}">${client.enabled ? 'Enabled' : 'Disabled'}</span>`);
    if (client.enrollment_state) badges.push(enrollmentStateBadge(client.enrollment_state));
    const install = INSTALL_STATE_LABELS[client.install_state];
    if (install) badges.push(`<span class="badge ${install[1]}" title="${escapeHtml(client.install_image || '')}">${install[0]}${client.install_updated_at ? ' · ' + relativeTime(client.install_updated_at) : ''}</span>`);
    if (client.next_boot_image) badges.push(`<span class="badge badge-warning" title="${escapeHtml(client.next_boot_image)}">Next boot: ${escapeHtml(nextBootLabel(client.next_boot_image))}</span>`);
    const group = clientGroupName(client);
    if (group) badges.push(`<span class="badge badge-info">Group: ${escapeHtml(group)}</span>`);
    return badges.join('');
}

function renderClientDetailHeader(client) {
    const header = document.getElementById('client-detail-header');
    header.innerHTML = `
        <div class="client-detail-title">
            <h1>${escapeHtml(client.name || 'Unnamed client')}</h1>
            <code>${escapeHtml(client.mac_address)}</code>
        </div>
        <div class="client-detail-badges">${clientDetailBadges(client)}</div>
        ${client.description ? `<p style="color: var(--text-secondary); margin: 8px 0 0; font-size: 13px;">${escapeHtml(client.description)}</p>` : ''}`;
}

function renderClientDetailActions(client) {
    const mac = jsAttrEscape(client.mac_address);
    const buttons = [];
    buttons.push(`<button class="btn btn-primary btn-sm" type="button" onclick="editClient('${mac}')">Edit</button>`);
    if (!client.static) buttons.push(`<button class="btn btn-success btn-sm" type="button" onclick="promoteClient('${mac}')">Make Static</button>`);
    buttons.push(`<button class="btn btn-success btn-sm" type="button" onclick="wakeClient('${mac}')">Wake</button>`);
    buttons.push(`<button class="btn btn-sm" type="button" onclick="showNextBoot('${mac}')">Next Boot</button>`);
    if (client.ipmi_host) {
        buttons.push(`<button class="btn btn-sm" type="button" onclick="clientDetailPower('On')">Power On</button>`);
        buttons.push(`<button class="btn btn-sm" type="button" onclick="clientDetailPower('GracefulShutdown')">Graceful Off</button>`);
        buttons.push(`<button class="btn btn-danger btn-sm" type="button" onclick="clientDetailPower('ForceOff')">Force Off</button>`);
        buttons.push(`<button class="btn btn-sm" type="button" onclick="clientDetailPower('ForceRestart')">Restart</button>`);
        buttons.push(`<button class="btn btn-sm read-ok" type="button" onclick="clientDetailPowerStatus()">Power State</button>`);
        buttons.push(`<button class="btn btn-sm" type="button" onclick="openClientConsole('${mac}')">Open Console</button>`);
    }
    buttons.push(`<button class="btn btn-sm read-ok" type="button" onclick="previewBootMenu('${mac}')">Preview Menu</button>`);
    buttons.push(`<button class="btn btn-sm" type="button" onclick="showCaptureDiskModal('${mac}')">Capture Disk</button>`);
    buttons.push(`<button class="btn btn-sm" type="button" onclick="showDeployDiskModal({ mac: '${mac}' })">Deploy Image</button>`);
    buttons.push(`<button class="btn btn-danger btn-sm" type="button" onclick="deleteClient('${mac}')">Delete</button>`);
    document.getElementById('client-detail-actions').innerHTML = buttons.join('');
    if (client.ipmi_host) clientDetailPowerStatus(true);
    else document.getElementById('client-detail-power-result').textContent = '';
}

function kvRows(rows) {
    return rows.map(([label, value]) => `<div class="kv-label">${escapeHtml(label)}</div><div class="kv-value">${value}</div>`).join('');
}

function kvText(value, fallback) {
    if (value === undefined || value === null || value === '') return `<span style="color: var(--text-muted);">${escapeHtml(fallback || '-')}</span>`;
    return escapeHtml(String(value));
}

function renderClientDetailOverview(client) {
    const san = client.san_target_id ? (sanTargets.find(t => t.id === client.san_target_id) || {}).name || `#${client.san_target_id}` : '';
    const rows = [
        ['Last boot', client.last_boot ? `${new Date(client.last_boot).toLocaleString()} <span style="color: var(--text-muted); font-weight: 400;">(${relativeTime(client.last_boot)})</span>` : kvText('', 'Never')],
        ['Boot count', kvText(client.boot_count || 0)],
        ['Bootloader set', kvText(client.bootloader_set, 'Default')],
        ['Allowed images', kvText((client.allowed_images || client.images || []).length)],
        ['Public images', kvText(client.show_public_images !== false ? 'Shown' : 'Hidden')],
        ['SAN target', san ? `${escapeHtml(san)}${client.boot_from_san ? ' <span class="badge badge-info">default</span>' : ''}` : kvText('', 'None')],
        ['Auto-install file', kvText(client.auto_install_file, 'None')],
        ['BMC', client.ipmi_host ? `${escapeHtml(client.bmc_protocol || 'redfish')} · ${escapeHtml(client.ipmi_host)}${client.ipmi_port ? ':' + client.ipmi_port : ''}` : kvText('', 'Not configured')],
        ['proxyDHCP option 43', kvText(client.proxy_dhcp_no_vendor_option ? 'Omitted' : 'Sent')],
        ['Created', kvText(client.created_at ? new Date(client.created_at).toLocaleString() : '')],
        ['Updated', kvText(client.updated_at ? new Date(client.updated_at).toLocaleString() : '')],
    ];
    document.getElementById('client-detail-overview').innerHTML = kvRows(rows);
}

function renderClientDetailProvisioning(client) {
    const rows = [
        ['Client group', kvText(clientGroupName(client), 'None')],
        ['Next boot', client.next_boot_image ? escapeHtml(nextBootLabel(client.next_boot_image)) : kvText('', 'None pending')],
        ['Install state', client.install_state ? `${(INSTALL_STATE_LABELS[client.install_state] || [client.install_state])[0]}${client.install_image ? ' · ' + escapeHtml(client.install_image) : ''}` : kvText('', 'No unattended install recorded')],
    ];
    if (client.cluster_id || client.cluster_role) {
        rows.push(['Kubernetes', `${escapeHtml(client.cluster && client.cluster.name ? client.cluster.name : '#' + client.cluster_id)}${client.cluster_role ? ' · ' + escapeHtml(client.cluster_role) : ''}`]);
    }
    document.getElementById('client-detail-provisioning').innerHTML = kvRows(rows) + '<div id="client-detail-provisioning-extra" style="grid-column: 1 / -1;"></div>';
}

const EFFECTIVE_SOURCE_LABELS = {
    client: 'client',
    profile: 'profile',
    'group-profile': 'group profile',
    group: 'group',
    default: 'default',
};

function openDeploymentProfilesTab() {
    clientDetailMac = null;
    clientDetailClient = null;
    if (location.hash.startsWith('#clients/')) history.pushState(null, '', location.pathname + location.search);
    const nav = document.querySelector('.sidebar-nav .nav-item[data-tab="clients"]');
    if (nav) nav.click();
    switchClientsSubtab('deployment-profiles');
}

function effectiveSourceBadge(sources, key) {
    const source = (sources || {})[key] || 'default';
    return `<span class="badge badge-disabled" title="Where this value comes from">${escapeHtml(EFFECTIVE_SOURCE_LABELS[source] || source)}</span>`;
}

async function loadClientDetailProvisioning(mac, client) {
    const container = document.getElementById('client-detail-provisioning-extra');
    if (!container) return;
    container.innerHTML = '<p style="color: var(--text-secondary); margin: 8px 0 0; font-size: 12px;">Loading…</p>';
    try {
        const [effRes] = await Promise.all([
            authFetch(`${API_BASE}/clients/effective?mac=${encodeURIComponent(mac)}`),
            enrolmentRulesLoaded ? Promise.resolve() : loadEnrolmentRules(true),
        ]);
        const effData = await effRes.json();
        if (clientDetailMac !== mac) return;
        if (!effData.success) {
            container.innerHTML = `<p class="alert alert-error" style="margin: 8px 0 0;">${escapeHtml(effData.error || 'Failed to resolve effective settings')}</p>`;
            return;
        }
        renderClientDetailProvisioningExtra(client, effData.data);
    } catch (err) {
        container.innerHTML = '<p class="alert alert-error" style="margin: 8px 0 0;">Failed to resolve effective settings</p>';
    }
}

function renderClientDetailProvisioningExtra(client, eff) {
    const container = document.getElementById('client-detail-provisioning-extra');
    const profileLink = name => `<a href="#" onclick="openDeploymentProfilesTab(); return false;">${escapeHtml(name)}</a>`;
    let profile;
    if (eff.profile_name) {
        profile = `${profileLink(eff.profile_name)} <span style="color: var(--text-muted); font-weight: 400;">(assigned to this client)</span>`;
    } else if (eff.group_profile_name) {
        profile = `${profileLink(eff.group_profile_name)} <span style="color: var(--text-muted); font-weight: 400;">(inherited from group ${escapeHtml(eff.group_name || '')})</span>`;
    } else {
        profile = kvText('', 'None');
    }

    let enrolment;
    const canReevaluate = !client.static && !client.enrolled_by_rule_id && !client.profile_id && !client.client_group_id;
    if (client.enrolled_by_rule_id) {
        const rule = (enrolmentRules || []).find(r => r.id === client.enrolled_by_rule_id);
        enrolment = `${escapeHtml(rule ? rule.name : 'rule #' + client.enrolled_by_rule_id)}${client.enrolled_at ? ` <span style="color: var(--text-muted); font-weight: 400;">${new Date(client.enrolled_at).toLocaleString()}</span>` : ''}`;
    } else if (client.static) {
        enrolment = kvText('', 'Manually added');
    } else {
        enrolment = `<span style="color: var(--text-muted);">Not enrolled</span>${canReevaluate ? ` <button class="btn btn-sm admin-only" type="button" onclick="reevaluateEnrolmentRules()" style="margin-left: 8px;">Re-evaluate rules</button>` : ''}`;
    }

    const sources = eff.sources || {};
    const settings = [
        ['Image', eff.image_filename ? escapeHtml(eff.image_name || eff.image_filename) : kvText('', 'None'), 'image_filename'],
        ['Auto-install file', kvText(eff.auto_install_file, 'None'), 'auto_install_file'],
        ['SAN target', eff.san_target_id ? `${escapeHtml(eff.san_target_name || '#' + eff.san_target_id)}${eff.boot_from_san ? ' <span class="badge badge-info">default</span>' : ''}` : kvText('', 'None'), 'san_target_id'],
        ['Bootloader set', kvText(eff.bootloader_set, 'Default'), 'bootloader_set'],
        ['Show public images', kvText(eff.show_public_images ? 'Yes' : 'No'), 'show_public_images'],
        ['First-boot action', kvText(eff.first_boot_action === 'install' ? 'Install image' : (eff.first_boot_action === 'deploy' ? 'Deploy disk image' : ''), 'None'), 'first_boot_action'],
        ['Disk image', eff.disk_image_id ? escapeHtml(eff.disk_image_name || '#' + eff.disk_image_id) : kvText('', 'None'), 'first_boot_action'],
    ];

    container.innerHTML = `
        <div class="kv-grid" style="padding: 8px 0 0;">
            ${kvRows([['Deployment profile', profile], ['Enrolled by', enrolment]])}
        </div>
        <h3 style="font-size: 13px; margin: 12px 0 6px; color: var(--text-secondary); font-weight: 500;">Effective settings</h3>
        <div class="table-scroll"><table>
            <thead><tr><th>Setting</th><th>Value</th><th>Source</th></tr></thead>
            <tbody>${settings.map(([label, value, key]) => `<tr><td>${escapeHtml(label)}</td><td>${value}</td><td>${effectiveSourceBadge(sources, key)}</td></tr>`).join('')}</tbody>
        </table></div>`;
}

async function reevaluateEnrolmentRules() {
    const mac = clientDetailMac;
    try {
        const res = await authFetch(`${API_BASE}/enrolment-rules/apply`, { method: 'POST' });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to run enrolment rules', 'error');
            return;
        }
        const hit = ((data.data && data.data.enrolled) || []).find(e => e.mac_address === mac);
        showAlert(hit ? `Enrolled by rule ${hit.rule}` : (data.message || 'No rule matched this client'), hit ? 'success' : 'info');
        if (clientDetailMac === mac) await refreshClientDetail();
    } catch (err) {
        showAlert('Failed to run enrolment rules: ' + err.message, 'error');
    }
}

function renderClientDetailActivity(counts) {
    const rows = [
        ['Boots recorded', kvText(counts.boots)],
        ['Unattended installs', kvText(counts.installs)],
        ['Disk imaging jobs', kvText(counts.disk)],
        ['Inventory submissions', kvText(counts.inventory)],
    ];
    document.getElementById('client-detail-activity').innerHTML = kvRows(rows);
}

const INVENTORY_FIELDS = [
    ['Manufacturer', 'manufacturer'],
    ['Product', 'product'],
    ['Serial', 'serial'],
    ['UUID', 'uuid'],
    ['Asset tag', 'asset'],
    ['CPU', 'cpu'],
    ['Memory', 'memory'],
    ['Platform', 'platform'],
    ['Architecture', 'buildarch'],
    ['NIC', 'nic_chip'],
    ['IP address', 'ip_address'],
];

function inventoryValue(inv, key) {
    const v = inv ? inv[key] : '';
    if (!v) return '';
    return key === 'memory' ? formatBytes(v) : String(v);
}

async function loadClientDetailHardware(mac) {
    const container = document.getElementById('client-detail-hardware');
    container.innerHTML = '<p style="color: var(--text-secondary); margin: 0;">Loading…</p>';
    try {
        const [invRes, histRes] = await Promise.all([
            authFetch(`${API_BASE}/clients/inventory?mac=${encodeURIComponent(mac)}`),
            authFetch(`${API_BASE}/clients/inventory/history?mac=${encodeURIComponent(mac)}&limit=50`),
        ]);
        const invData = await invRes.json();
        const histData = await histRes.json();
        if (clientDetailMac !== mac) return;
        clientDetailInventory = invData.success ? invData.data : null;
        clientDetailInventoryHistory = histData.success ? (histData.data || []) : [];
        renderClientDetailHardware();
        renderClientDetailInventoryHistory();
        updateClientDetailActivity();
    } catch (err) {
        container.innerHTML = '<p style="color: var(--danger); margin: 0;">Failed to load hardware inventory.</p>';
    }
}

function renderClientDetailHardware() {
    const container = document.getElementById('client-detail-hardware');
    const inv = clientDetailInventory;
    const rows = INVENTORY_FIELDS.map(([label, key]) => [label, inventoryValue(inv, key)]).filter(([, v]) => v);
    if (!inv || rows.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); margin: 0;">No hardware inventory yet. It is collected automatically when the client PXE boots.</p>';
        return;
    }
    rows.push(['Last seen', inv.created_at ? new Date(inv.created_at).toLocaleString() : '']);
    container.innerHTML = `<div class="kv-grid" style="padding: 0;">${kvRows(rows.map(([l, v]) => [l, escapeHtml(v)]))}</div>`;
}

function toggleClientDetailInventoryHistory() {
    const panel = document.getElementById('client-detail-hardware-history');
    const btn = document.getElementById('client-detail-hw-toggle');
    const show = panel.style.display === 'none';
    panel.style.display = show ? '' : 'none';
    btn.textContent = show ? 'Hide history' : 'Show history';
}

function renderClientDetailInventoryHistory() {
    const panel = document.getElementById('client-detail-hardware-history');
    const history = clientDetailInventoryHistory || [];
    if (history.length === 0) {
        panel.innerHTML = '<p style="color: var(--text-secondary); margin: 0;">No inventory history.</p>';
        return;
    }
    const cols = INVENTORY_FIELDS.filter(([, key]) => history.some(h => h[key]));
    const body = history.map((h, i) => {
        const older = history[i + 1];
        return `<tr>
            <td style="white-space: nowrap;">${new Date(h.created_at).toLocaleString()}</td>
            ${cols.map(([, key]) => {
                const v = inventoryValue(h, key);
                const changed = older && inventoryValue(older, key) !== v;
                return `<td>${changed ? `<span class="hw-changed" title="Changed since the previous submission">${escapeHtml(v || '-')}</span>` : escapeHtml(v || '-')}</td>`;
            }).join('')}
        </tr>`;
    }).join('');
    panel.innerHTML = `<p style="color: var(--text-secondary); margin: 0 0 8px; font-size: 12px;">Newest first. Highlighted values differ from the previous submission.</p>
        <div class="table-scroll"><table>
            <thead><tr><th>Submitted</th>${cols.map(([label]) => `<th>${escapeHtml(label)}</th>`).join('')}</tr></thead>
            <tbody>${body}</tbody>
        </table></div>`;
}

function updateClientDetailActivity() {
    const items = clientDetailTimelineItems;
    const ids = new Set();
    const counts = { boots: 0, installs: 0, disk: 0, inventory: (clientDetailInventoryHistory || []).length };
    for (const it of items) {
        if (it.source_id && ids.has(it.source_id)) continue;
        if (it.source_id) ids.add(it.source_id);
        if (it.kind === 'boot') counts.boots++;
        if (it.kind === 'install') counts.installs++;
        if (it.kind === 'disk') counts.disk++;
    }
    if (clientDetailClient && clientDetailClient.boot_count > counts.boots) counts.boots = clientDetailClient.boot_count;
    renderClientDetailActivity(counts);
}

async function loadClientTimeline(mac, reset) {
    const container = document.getElementById('client-detail-timeline');
    const more = document.getElementById('client-detail-timeline-more');
    const status = document.getElementById('client-detail-timeline-status');
    if (reset) {
        clientDetailTimelineBefore = '';
        clientDetailTimelineItems = [];
        container.innerHTML = '<p style="color: var(--text-secondary); margin: 0;">Loading…</p>';
    }
    try {
        const cursor = clientDetailTimelineBefore ? `&before=${encodeURIComponent(clientDetailTimelineBefore)}` : '';
        const res = await authFetch(`${API_BASE}/clients/timeline?mac=${encodeURIComponent(mac)}&limit=50${cursor}`);
        const data = await res.json();
        if (clientDetailMac !== mac) return;
        if (!data.success) {
            container.innerHTML = `<p class="alert alert-error">${escapeHtml(data.error || 'Failed to load timeline')}</p>`;
            return;
        }
        clientDetailTimelineItems = clientDetailTimelineItems.concat(data.data.items || []);
        clientDetailTimelineBefore = data.data.next_before || '';
        more.style.display = data.data.has_more ? '' : 'none';
        status.textContent = `${clientDetailTimelineItems.length} event${clientDetailTimelineItems.length === 1 ? '' : 's'}`;
        renderClientTimeline();
        updateClientDetailActivity();
    } catch (err) {
        container.innerHTML = '<p class="alert alert-error">Failed to load timeline</p>';
    }
}

function loadMoreClientTimeline() {
    if (clientDetailMac) loadClientTimeline(clientDetailMac, false);
}

function renderClientTimeline() {
    const container = document.getElementById('client-detail-timeline');
    if (clientDetailTimelineItems.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); margin: 0;">Nothing recorded for this client yet.</p>';
        return;
    }
    container.innerHTML = clientDetailTimelineItems.map(it => `
        <div class="timeline-item">
            <span class="timeline-dot ${escapeHtml(it.status || 'info')}"></span>
            <span class="timeline-time">${new Date(it.time).toLocaleString()}</span>
            <div>
                <span class="timeline-kind">${escapeHtml(TIMELINE_KIND_LABELS[it.kind] || it.kind)}</span><strong>${escapeHtml(it.title || '')}</strong>
                ${it.detail ? `<div class="timeline-detail">${escapeHtml(it.detail)}</div>` : ''}
            </div>
        </div>`).join('');
}

async function clientDetailPower(action) {
    const mac = clientDetailMac;
    const result = document.getElementById('client-detail-power-result');
    if (!mac) return;
    result.textContent = `Sending ${action}…`;
    result.style.color = 'var(--text-secondary)';
    try {
        const res = await authFetch(`${API_BASE}/clients/power?mac=${encodeURIComponent(mac)}&action=${encodeURIComponent(action)}`, { method: 'POST' });
        const data = await res.json();
        result.textContent = (data.success ? '✓ ' : '✗ ') + (data.message || data.error || '');
        result.style.color = data.success ? 'var(--teal, green)' : 'var(--danger)';
        setTimeout(() => clientDetailPowerStatus(true), 3000);
    } catch (err) {
        result.textContent = '✗ ' + err.message;
        result.style.color = 'var(--danger)';
    }
}

async function clientDetailPowerStatus(quiet) {
    const mac = clientDetailMac;
    const result = document.getElementById('client-detail-power-result');
    if (!mac) return;
    if (!quiet) {
        result.textContent = 'Querying…';
        result.style.color = 'var(--text-secondary)';
    }
    try {
        const res = await authFetch(`${API_BASE}/clients/power/status?mac=${encodeURIComponent(mac)}`);
        const data = await res.json();
        if (clientDetailMac !== mac) return;
        if (data.success && data.data) {
            result.textContent = 'Power: ' + (data.data.state || 'unknown');
            result.style.color = 'var(--text-primary)';
        } else if (!quiet) {
            result.textContent = '✗ ' + (data.error || 'unknown');
            result.style.color = 'var(--danger)';
        }
    } catch (err) {
        if (!quiet) {
            result.textContent = '✗ ' + err.message;
            result.style.color = 'var(--danger)';
        }
    }
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
            form.querySelector('[name="proxy_dhcp_no_vendor_option"]').checked = !!currentClient.proxy_dhcp_no_vendor_option;

            // BMC / Redfish
            form.querySelector('[name="bmc_protocol"]').value = currentClient.bmc_protocol || '';
            updateBMCPortPlaceholder(form.querySelector('[name="bmc_protocol"]'));
            form.querySelector('[name="ipmi_host"]').value = currentClient.ipmi_host || '';
            form.querySelector('[name="ipmi_port"]').value = currentClient.ipmi_port || '';
            form.querySelector('[name="ipmi_username"]').value = currentClient.ipmi_username || '';
            form.querySelector('[name="ipmi_password"]').value = currentClient.ipmi_password || '';
            form.querySelector('[name="ipmi_insecure"]').checked = !!currentClient.ipmi_insecure;
            await populateSANTargetSelect('edit-client-san-select', currentClient.san_target_id, '(inherit from group, or none)');
            form.querySelector('[name="boot_from_san"]').checked = !!currentClient.boot_from_san;
            await populateDeploymentProfileSelect('edit-client-profile-select', currentClient.profile_id, '(inherit from group, or none)');
            loadClientEffective(currentClient.mac_address || mac);
            const powerResult = document.getElementById('power-client-result');
            if (powerResult) powerResult.textContent = '';
            const consoleBtn = document.getElementById('open-console-btn');
            if (consoleBtn) consoleBtn.style.display = currentClient.ipmi_host ? '' : 'none';
            loadClientInstallHistory(currentClient.mac_address || mac);

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
            renderVariableRows('edit-client-variables', currentClient.variables);

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
        const res = await authFetch(`${API_BASE}/logs?mac=${encodeURIComponent(mac)}&limit=50`);
        const data = await res.json();
        if (!data.success || !data.data || data.data.length === 0) {
            container.innerHTML = '<p style="color: var(--text-secondary); margin: 0;">No boot history for this client yet.</p>';
            return;
        }
        const rows = data.data.map(entry => {
            const when = entry.created_at ? new Date(entry.created_at).toLocaleString() : '-';
            const img = entry.image_name || '(unknown)';
            const ok = entry.success
                ? '<span class="status-dot on" title="Success"></span>'
                : '<span class="status-dot off" title="Failed"></span>';
            const err = entry.error_msg ? `<div style="color: var(--danger); font-size: 12px;">${escapeHtml(entry.error_msg)}</div>` : '';
            const ip = entry.ip_address ? `<span style="color: var(--text-muted); font-size: 12px;">${escapeHtml(entry.ip_address)}</span>` : '';
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
            if (clientDetailMac === mac.toLowerCase()) closeClientDetail();
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
    await loadSANTargets(true);
    select.innerHTML = images.map(img =>
        `<option value="${img.filename}">${img.name}</option>`
    ).join('') + sanTargets.map(t =>
        `<option value="san:${t.id}">Boot from SAN: ${escapeHtml(t.name)}</option>`
    ).join('');

    const currentDiv = document.getElementById('next-boot-current');
    if (client && client.next_boot_image) {
        document.getElementById('next-boot-current-image').textContent = nextBootLabel(client.next_boot_image);
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
    if (img.boot_method === 'nfs' && !img.extracted) {
        return { reason: 'NFS boot needs extraction' };
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

const selectedImageFilenames = new Set();

function toggleImageSelection(filename, checked) {
    if (checked) selectedImageFilenames.add(filename);
    else selectedImageFilenames.delete(filename);
    updateImagesBulkUI();
}

function clearImageSelection() {
    selectedImageFilenames.clear();
    renderImagesTable();
}

function toggleImagesSelectAll(checked) {
    selectedImageFilenames.clear();
    if (checked) {
        for (const img of (images || [])) selectedImageFilenames.add(img.filename);
    }
    renderImagesTable();
}

function updateImagesBulkUI() {
    const wrap = document.getElementById('images-bulk-actions');
    const count = document.getElementById('images-bulk-count');
    const n = selectedImageFilenames.size;
    if (wrap) wrap.style.display = n > 0 ? 'flex' : 'none';
    if (count) count.textContent = n + ' selected';
    const selectAll = document.getElementById('images-select-all');
    if (selectAll) {
        const total = (images || []).length;
        if (n === 0) { selectAll.checked = false; selectAll.indeterminate = false; }
        else if (n >= total) { selectAll.checked = true; selectAll.indeterminate = false; }
        else { selectAll.checked = false; selectAll.indeterminate = true; }
    }
}

async function bulkSetImagesEnabled(enabled) {
    const filenames = Array.from(selectedImageFilenames);
    if (!filenames.length) return;
    let success = 0, fail = 0;
    for (const filename of filenames) {
        try {
            const res = await authFetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ enabled }),
            });
            const data = await res.json();
            if (data.success) success++; else fail++;
        } catch (e) { fail++; }
    }
    const verb = enabled ? 'Enabled' : 'Disabled';
    showAlert(`${verb} ${success}${fail ? ', ' + fail + ' failed' : ''}`, fail ? 'error' : 'success');
    selectedImageFilenames.clear();
    await loadImages();
    loadStats();
}

async function openBulkAssignGroupModal() {
    if (!selectedImageFilenames.size) return;
    if (!groups || groups.length === 0) {
        await loadGroups();
    }
    const select = document.getElementById('bulk-assign-group-select');
    select.innerHTML = '<option value="">Unassigned</option>';
    for (const group of (groups || [])) {
        select.innerHTML += `<option value="${group.id}">${escapeHtml(group.name)}</option>`;
    }
    const n = selectedImageFilenames.size;
    document.getElementById('bulk-assign-group-count').textContent = n + ' image' + (n === 1 ? '' : 's');
    openModal('bulk-assign-group-modal');
}

async function confirmBulkAssignGroup() {
    const filenames = Array.from(selectedImageFilenames);
    if (!filenames.length) {
        closeModal('bulk-assign-group-modal');
        return;
    }
    const raw = document.getElementById('bulk-assign-group-select').value;
    const body = JSON.stringify({ group_id: raw === '' ? null : parseInt(raw, 10) });
    let success = 0, fail = 0;
    for (const filename of filenames) {
        try {
            const res = await authFetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body,
            });
            const data = await res.json();
            if (data.success) success++; else fail++;
        } catch (e) { fail++; }
    }
    showAlert(`Assigned ${success}${fail ? ', ' + fail + ' failed' : ''}`, fail ? 'error' : 'success');
    selectedImageFilenames.clear();
    closeModal('bulk-assign-group-modal');
    await loadImages();
}

async function bulkDeleteImages() {
    const filenames = Array.from(selectedImageFilenames);
    if (!filenames.length) return;
    if (!confirm(`Delete ${filenames.length} image${filenames.length === 1 ? '' : 's'}?\n\nWARNING: This will permanently delete the ISO files from disk.`)) return;
    let success = 0, fail = 0;
    for (const filename of filenames) {
        try {
            const res = await authFetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}&delete_file=true`, { method: 'DELETE' });
            const data = await res.json();
            if (data.success) success++; else fail++;
        } catch (e) { fail++; }
    }
    showAlert(`Deleted ${success}${fail ? ', ' + fail + ' failed' : ''}`, fail ? 'error' : 'success');
    selectedImageFilenames.clear();
    await loadImages();
    loadStats();
}

const DISTRO_LOGO_FALLBACK = 'data:image/svg+xml;utf8,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" opacity="0.4"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3"/></svg>');

function distroLogoSrc(distro) {
    const id = (distro || '').toLowerCase().replace(/[^a-z0-9_-]/g, '');
    return id ? `/distros/${id}.svg` : DISTRO_LOGO_FALLBACK;
}

function distroLogoHTML(distro, size) {
    const sz = size || 24;
    const src = distroLogoSrc(distro);
    const alt = distro ? escapeHtml(distro) : '';
    return `<img src="${src}" width="${sz}" height="${sz}" class="distro-logo" alt="${alt}" onerror="this.onerror=null; this.src='${DISTRO_LOGO_FALLBACK}'">`;
}

function uploadRowHTML(u, includeGroupCell) {
    const groupCell = includeGroupCell ? '<td></td>' : '';
    const fnSafe = escapeHtml(u.filename);
    const errCls = u.error ? ' row-upload-error' : '';
    const statusText = u.error ? ('✗ ' + u.error) : (u.status || (u.progress.toFixed(1) + '%'));
    const dismissBtn = u.error
        ? `<button class="btn btn-sm" onclick="dismissPendingUpload('${escapeHtml(u.filename)}')" title="Dismiss">×</button>`
        : '';
    return `
                    <tr class="row-uploading${errCls}" data-upload="${fnSafe}">
                        <td class="col-check"></td>
                        <td class="col-logo">${distroLogoHTML('')}</td>
                        <td>${escapeHtml(u.name)} <span class="badge badge-info">Uploading</span></td>
                        <td><code>${fnSafe}</code></td>
                        <td>${formatBytes(u.size)}</td>
                        <td><span style="color: var(--text-secondary);">-</span></td>${groupCell}
                        <td class="col-dot"></td>
                        <td class="col-dot"></td>
                        <td><span style="color: var(--text-secondary);">-</span></td>
                        <td class="operations-cell">
                            <div class="progress-container" style="display: flex; align-items: center; gap: 8px;">
                                <div class="progress-bar" style="flex: 1; height: 6px; background: var(--bg-tertiary); border-radius: 3px; overflow: hidden;">
                                    <div class="progress-fill" style="width: ${u.progress}%; height: 100%; background: ${u.error ? 'var(--danger)' : 'var(--success)'}; transition: width 0.2s;"></div>
                                </div>
                                <span class="progress-text" style="font-size: 11px; color: var(--text-secondary); white-space: nowrap;">${escapeHtml(statusText)}</span>
                                ${dismissBtn}
                            </div>
                        </td>
                    </tr>`;
}

function updateUploadRowDOM(filename) {
    const u = pendingUploads.get(filename);
    if (!u) return;
    const row = document.querySelector(`tr[data-upload="${CSS.escape(filename)}"]`);
    if (!row) {
        renderImagesTable();
        return;
    }
    const fill = row.querySelector('.progress-fill');
    const text = row.querySelector('.progress-text');
    if (fill) {
        fill.style.width = u.progress + '%';
        fill.style.background = u.error ? 'var(--danger)' : 'var(--success)';
    }
    if (text) text.textContent = u.error ? ('✗ ' + u.error) : (u.status || (u.progress.toFixed(1) + '%'));
    if (u.error) row.classList.add('row-upload-error');
}

function dismissPendingUpload(filename) {
    pendingUploads.delete(filename);
    renderImagesTable();
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
    const checked = selectedImageFilenames.has(img.filename) ? 'checked' : '';
    return `
                    <tr class="${rowClass}"${rowTitle} onclick="showImagePropertiesModal('${img.filename}')">
                        <td class="col-check" onclick="event.stopPropagation()"><input type="checkbox" ${checked} onchange="toggleImageSelection('${img.filename}', this.checked)"></td>
                        <td class="col-logo">${distroLogoHTML(img.distro)}</td>
                        <td${namePadStyle}>${img.name}</td>
                        <td><code>${img.filename}</code></td>
                        <td>${formatBytes(img.size)}</td>
                        <td>
                            ${img.extracted ?
                                (img.distro ? '<span class="badge badge-info">'+(img.distro === 'winpe' ? 'WinPE' : img.distro)+'</span>' : '<span class="badge badge-success">✓ Extracted</span>') :
                                (img.extraction_error ? '<span class="badge badge-danger" title="'+img.extraction_error+'">Error</span>' : '')
                            }
                            ${img.smb_install_enabled ? ' <span class="badge badge-warning" title="boot.wim patched to auto-mount SMB share and launch setup.exe">SMB</span>' : ''}
                            ${img.update_available ? ' <span class="badge badge-warning" title="' + escapeHtml(img.update_filename || img.update_url || '') + '">Update available: ' + escapeHtml(img.update_version || 'newer release') + '</span>' : ''}
                            ${img.checksum_verified ? ' <span class="badge badge-success" title="SHA-256 ' + escapeHtml(img.checksum || '') + '">✓ Verified</span>' : ''}
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
                                img.boot_method === 'uki' ?
                                '<span class="badge badge-success">UKI</span>' :
                                img.boot_method === 'nbd' ?
                                '<span class="badge badge-warning">NBD</span>' :
                                img.boot_method === 'nfs' ?
                                '<span class="badge badge-warning">NFS</span>' :
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

    if (images.length === 0 && pendingUploads.size === 0) {
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
        colspan = 11;
        theadHtml = `
                <tr>
                    <th class="col-check"><input type="checkbox" id="images-select-all" onchange="toggleImagesSelectAll(this.checked)" title="Select all"></th>
                    <th class="col-logo"></th>
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
        colspan = 10;
        theadHtml = `
                <tr>
                    <th class="col-check"><input type="checkbox" id="images-select-all" onchange="toggleImagesSelectAll(this.checked)" title="Select all"></th>
                    <th class="col-logo"></th>
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

    let uploadHtml = '';
    if (pendingUploads.size > 0) {
        const includeGroupCell = !imageGroupedView;
        for (const u of pendingUploads.values()) {
            uploadHtml += uploadRowHTML(u, includeGroupCell);
        }
    }

    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead>${theadHtml}</thead>
            <tbody>${uploadHtml}${bodyHtml}</tbody>
        </table>
        </div>
    `;
    updateImagesBulkUI();
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

    const actionBtns = ['image-props-extract-btn', 'image-props-patch-smb-btn', 'image-props-unpatch-smb-btn', 'image-props-netboot-btn', 'image-props-download-btn', 'image-props-delete-btn'];

    if (p) {
        container.style.display = '';
        fill.style.width = p.progress + '%';
        text.textContent = p.status;
        if (percent) percent.textContent = Math.round(p.progress) + '%';
        actionBtns.forEach(id => { const b = document.getElementById(id); if (b) b.disabled = true; });
    } else {
        container.style.display = 'none';
        actionBtns.forEach(id => { const b = document.getElementById(id); if (b) b.disabled = false; });
        const unpatchBtn = document.getElementById('image-props-unpatch-smb-btn');
        if (unpatchBtn && _imagePropsState) unpatchBtn.disabled = !_imagePropsState.img.smb_unpatch_available;
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
        'nbd': 'nfs',
        'nfs': 'sanboot'
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
            proxy_dhcp_no_vendor_option: formData.get('proxy_dhcp_no_vendor_option') === 'on',
            client_group_id: groupIdRaw ? parseInt(groupIdRaw, 10) : null,
            bmc_protocol: formData.get('bmc_protocol') || '',
            ipmi_host: formData.get('ipmi_host') || '',
            ipmi_port: ipmiPortRaw ? parseInt(ipmiPortRaw, 10) : 0,
            ipmi_username: formData.get('ipmi_username') || '',
            ipmi_password: formData.get('ipmi_password') || '',
            ipmi_insecure: formData.get('ipmi_insecure') === 'on',
            auto_install_file: formData.get('auto_install_file') || '',
            san_target_id: formData.get('san_target_id') ? parseInt(formData.get('san_target_id'), 10) : null,
            boot_from_san: formData.get('boot_from_san') === 'on',
            profile_id: formData.get('profile_id') ? parseInt(formData.get('profile_id'), 10) : null,
        };
        try {
            updates.variables = collectVariables('edit-client-variables');
        } catch (err) {
            showAlert(err.message, 'error');
            return;
        }
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
    const hf = document.getElementById('housekeeping-form');
    if (hf) hf.addEventListener('submit', saveHousekeeping);
}

let apiReferenceSpec = null;

async function loadAPIReferenceSpec() {
    if (apiReferenceSpec) return apiReferenceSpec;
    const res = await fetch(`${API_BASE}/openapi.json`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    apiReferenceSpec = await res.json();
    return apiReferenceSpec;
}

function apiReferenceSections(spec) {
    const byTag = new Map();
    for (const tag of (spec.tags || [])) byTag.set(tag.name, { category: tag.name, description: tag.description || '', endpoints: [] });
    for (const [path, ops] of Object.entries(spec.paths || {})) {
        for (const [method, op] of Object.entries(ops)) {
            const tag = (op.tags && op.tags[0]) || 'Other';
            if (!byTag.has(tag)) byTag.set(tag, { category: tag, description: '', endpoints: [] });
            const params = op.parameters || [];
            const required = params.filter(p => p.in === 'query' && p.required).map(p => `${p.name}={${p.name}}`);
            const optional = params.filter(p => p.in === 'query' && !p.required);
            const body = op.requestBody || null;
            const contentType = body ? Object.keys(body.content || {})[0] || '' : '';
            byTag.get(tag).endpoints.push({
                method: method.toUpperCase(),
                path: required.length ? `${path}?${required.join('&')}` : path,
                desc: op.summary || '',
                optional,
                body: body ? body.description || '' : '',
                multipart: contentType === 'multipart/form-data',
                publicAccess: !!op['x-bootimus-public'],
                readOnly: !!op['x-bootimus-readonly'],
                order: op['x-bootimus-order'] || 0,
            });
        }
    }
    const sections = [...byTag.values()].filter(s => s.endpoints.length);
    for (const s of sections) s.endpoints.sort((a, b) => a.order - b.order);
    return sections;
}

let apiRefImage = '';
let apiRefMac = '';
let apiRefServer = '';

async function showAPIReference() {
    const container = document.getElementById('api-reference-content');
    try {
        await loadAPIReferenceSpec();
    } catch (err) {
        if (container) container.innerHTML = `<p class="alert alert-error">Failed to load the OpenAPI document: ${escapeHtml(err.message)}</p>`;
        return;
    }
    if (!images || images.length === 0) {
        try { await loadImages(); } catch (_) {}
    }
    if (!clients || clients.length === 0) {
        try { await loadClients(); } catch (_) {}
    }
    const serverInput = document.getElementById('api-ref-server');
    if (serverInput && !serverInput.value) {
        serverInput.value = window.location.origin;
        apiRefServer = serverInput.value;
    } else if (serverInput) {
        apiRefServer = serverInput.value;
    }
    populateAPIRefDropdowns();
    renderAPIReference();
}

function onAPIRefServerChange() {
    apiRefServer = document.getElementById('api-ref-server').value;
    const filter = document.getElementById('api-ref-filter');
    renderAPIReference(filter ? filter.value : '');
}

function populateAPIRefDropdowns() {
    const imgSel = document.getElementById('api-ref-image');
    const macSel = document.getElementById('api-ref-mac');
    if (imgSel) {
        let html = '<option value="">(none)</option>';
        for (const img of (images || [])) {
            const sel = img.filename === apiRefImage ? ' selected' : '';
            html += `<option value="${escapeHtml(img.filename)}"${sel}>${escapeHtml(img.name || img.filename)}</option>`;
        }
        imgSel.innerHTML = html;
    }
    if (macSel) {
        let html = '<option value="">(none)</option>';
        for (const c of (clients || [])) {
            const sel = c.mac_address === apiRefMac ? ' selected' : '';
            const label = c.name ? `${c.name} (${c.mac_address})` : c.mac_address;
            html += `<option value="${escapeHtml(c.mac_address)}"${sel}>${escapeHtml(label)}</option>`;
        }
        macSel.innerHTML = html;
    }
}

function onAPIRefSelectChange() {
    apiRefImage = document.getElementById('api-ref-image').value;
    apiRefMac = document.getElementById('api-ref-mac').value;
    const filter = document.getElementById('api-ref-filter');
    renderAPIReference(filter ? filter.value : '');
}

function fillAPIRefPath(path) {
    const fnEnc = encodeURIComponent(apiRefImage || '{filename}');
    const macEnc = encodeURIComponent(apiRefMac || '{mac}');
    return path
        .replace(/\{mac\}/g, macEnc)
        .replace(/\{fn\}/g, fnEnc)
        .replace(/\{filename\}/g, fnEnc);
}

function buildAPIRefCurl(method, path, isPublic, multipart) {
    const base = (apiRefServer || window.location.origin).replace(/\/+$/, '');
    const url = base + fillAPIRefPath(path);
    const parts = [`curl -X ${method} '${url}'`];
    if (!isPublic) parts.push(`-H 'Authorization: Bearer $BOOTIMUS_TOKEN'`);
    if (multipart) {
        parts.push(`-F 'file=@/path/to/file'`);
    } else if (method === 'POST' || method === 'PUT') {
        parts.push(`-H 'Content-Type: application/json' -d '{}'`);
    }
    return parts.join(' ');
}

async function copyAPIRefCurl(btn) {
    const wrap = btn.closest('.api-curl');
    if (!wrap) return;
    const code = wrap.querySelector('code').textContent;
    try {
        await navigator.clipboard.writeText(code);
        btn.classList.add('copied');
        const orig = btn.innerHTML;
        btn.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>';
        setTimeout(() => { btn.classList.remove('copied'); btn.innerHTML = orig; }, 1200);
    } catch (e) {
        showAlert('Failed to copy', 'error');
    }
}

function renderAPIReference(filter) {
    const container = document.getElementById('api-reference-content');
    if (!container) return;
    const q = (filter || '').trim().toLowerCase();
    const copyIcon = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
    let html = '';
    if (!apiReferenceSpec) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">Loading the OpenAPI document…</p>';
        return;
    }
    for (const section of apiReferenceSections(apiReferenceSpec)) {
        const matching = section.endpoints.filter(e =>
            !q || e.path.toLowerCase().includes(q) || e.desc.toLowerCase().includes(q) || e.body.toLowerCase().includes(q) || section.category.toLowerCase().includes(q) || e.method.toLowerCase().includes(q)
        );
        if (matching.length === 0) continue;
        html += `<div class="api-section"><h3>${escapeHtml(section.category)}</h3>`;
        for (const e of matching) {
            const filledPath = fillAPIRefPath(e.path);
            const noAuthBadge = e.publicAccess ? ' <span style="color: #ffb000; font-family: ui-monospace, monospace; font-size: 10px; font-weight: 700; letter-spacing: 0.5px;">NO AUTH</span>' : '';
            const readOnlyBadge = !e.publicAccess && e.readOnly ? ' <span style="color: #3498db; font-family: ui-monospace, monospace; font-size: 10px; font-weight: 700; letter-spacing: 0.5px;" title="Available to read-only users">READ-ONLY OK</span>' : '';
            const curl = buildAPIRefCurl(e.method, e.path, !!e.publicAccess, !!e.multipart);
            let detail = escapeHtml(e.desc);
            if (e.body) detail += ` ${e.multipart ? 'Multipart' : 'Body'}: <code>${escapeHtml(e.body)}</code>`;
            if (e.optional.length) detail += ` Optional: ${e.optional.map(p => `<code>${escapeHtml(p.name)}</code>${p.description ? ' (' + escapeHtml(p.description) + ')' : ''}`).join(', ')}.`;
            html += `<div class="api-row">
                <div class="api-row-head">
                    <span class="api-method ${e.method.toLowerCase()}">${e.method}</span>
                    <code class="api-path">${escapeHtml(filledPath)}${noAuthBadge}${readOnlyBadge}</code>
                    <span class="api-detail">${detail}</span>
                </div>
                <div class="api-curl">
                    <code>${escapeHtml(curl)}</code>
                    <button type="button" onclick="copyAPIRefCurl(this)" title="Copy curl command">${copyIcon}</button>
                </div>
            </div>`;
        }
        html += '</div>';
    }
    if (!html) html = '<p style="color: var(--text-secondary); padding: 20px;">No endpoints match the filter.</p>';
    container.innerHTML = html;
}

// Theme
async function loadTheme() {
    try {
        const res = await authFetch(`${API_BASE}/theme`);
        const data = await res.json();
        if (data.success) {
            document.getElementById('theme-title').value = data.data.title || '';
            document.getElementById('theme-timeout').value = data.data.menu_timeout != null ? data.data.menu_timeout : 30;
            document.getElementById('theme-default-item').value = data.data.default_menu_item || '';
            document.getElementById('theme-boot-password').value = '';
            document.getElementById('theme-remove-boot-password').checked = false;
            document.getElementById('theme-boot-password-status').textContent = data.data.boot_password_required ? 'Password set.' : 'No password set.';
        }
    } catch (err) {
        console.error('Failed to load theme:', err);
    }
}

function validateBootPassword(password) {
    if (/[+; ]/.test(password)) {
        showAlert('Boot passwords must not contain +, ; or spaces (iPXE v2.0.0 compatibility)', 'error');
        return false;
    }
    return true;
}

async function saveTheme(e) {
    e.preventDefault();
    const theme = {
        title: document.getElementById('theme-title').value,
        menu_timeout: parseInt(document.getElementById('theme-timeout').value) || 0,
        default_menu_item: document.getElementById('theme-default-item').value,
    };
    const bootPassword = document.getElementById('theme-boot-password').value;
    if (document.getElementById('theme-remove-boot-password').checked) theme.boot_password = '';
    else if (bootPassword) {
        if (!validateBootPassword(bootPassword)) return;
        theme.boot_password = bootPassword;
    }
    try {
        const res = await authFetch(`${API_BASE}/theme`, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(theme),
        });
        const data = await res.json();
        if (data.success) {
            showAlert('Theme saved successfully', 'success');
            await loadTheme();
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
            if (tool.update_available) html += `<span class="badge badge-warning" title="${escapeHtml(tool.update_url || '')}">Update available</span>`;
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
                if (tool.update_available && tool.update_url) {
                    html += `<button class="btn btn-primary" onclick="downloadNewerTool('${tool.name}', '${escapeHtml(tool.update_url)}')">Download newer</button>`;
                }
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

        for (const tool of toolsList) {
            if (!tool.downloaded) resumeToolProgress(tool.name);
        }
    } catch (err) {
        document.getElementById('tools-list').innerHTML = `<p class="alert alert-error">Failed to load tools: ${err.message}</p>`;
    }
}

const toolProgressIntervals = new Map();

async function downloadNewerTool(name, url) {
    try {
        const res = await authFetch(`${API_BASE}/tools/url?name=${encodeURIComponent(name)}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url }),
        });
        const data = await res.json();
        if (!data.success) {
            showNotification(data.error || 'Could not switch to the newer URL', 'error');
            return;
        }
        await loadTools();
        await downloadTool(name);
    } catch (err) {
        showNotification('Could not start the update: ' + err.message, 'error');
    }
}

async function downloadTool(name) {
    try {
        const res = await authFetch(`${API_BASE}/tools/download?name=${encodeURIComponent(name)}`, { method: 'POST' });
        const data = await res.json();
        if (res.status === 409) {
            showNotification(data.error || 'Download already in progress', 'info');
            showToolProgress(name);
            return;
        }
        if (!data.success) {
            showNotification(data.error || 'Download failed', 'error');
            return;
        }
        showToolProgress(name);
    } catch (err) {
        showNotification('Download failed: ' + err.message, 'error');
    }
}

function showToolProgress(name) {
    const btn = document.getElementById(`tool-dl-btn-${name}`);
    const wrap = document.getElementById(`tool-progress-wrap-${name}`);
    if (btn) btn.style.display = 'none';
    if (wrap) wrap.style.display = 'block';
    startToolProgressPolling(name);
}

async function resumeToolProgress(name) {
    try {
        const r = await authFetch(`${API_BASE}/tools/progress?name=${encodeURIComponent(name)}`);
        const d = await r.json();
        if (!d.success || !d.data) return;
        const s = d.data.status;
        if (s === 'starting' || s === 'downloading' || s === 'extracting') {
            showToolProgress(name);
        }
    } catch (e) { /* ignore */ }
}

function startToolProgressPolling(name) {
    const existing = toolProgressIntervals.get(name);
    if (existing) clearInterval(existing);

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
                toolProgressIntervals.delete(name);
                showNotification('Download complete', 'success');
                loadTools();
            } else if (p.status === 'error') {
                clearInterval(poll);
                toolProgressIntervals.delete(name);
                showNotification('Download failed: ' + (p.error || 'unknown error'), 'error');
                loadTools();
            }
        } catch (e) { /* ignore poll errors */ }
    }, 1000);
    toolProgressIntervals.set(name, poll);
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
    if (!isToolDownloadURL(data.download_url)) {
        showNotification(t('tools.notify.invalid_download_url'), 'error');
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

function isToolDownloadURL(value) {
    return /^(https?:\/\/|\{\{HTTP_URL\}\})/i.test(value);
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
            const isBuiltIn = set.built_in === true || set.name === 'built-in';
            const escapedName = escapeHtml(set.name);
            const files = set.files || [];

            html += `<div style="background: var(--bg-tertiary); padding: 20px 24px; border-radius: var(--radius); margin-bottom: 14px; border: 2px solid ${isActive ? 'var(--success)' : 'transparent'};">`;

            // Header row
            html += `<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px;">`;
            html += `<div style="display: flex; align-items: center; gap: 12px;">`;
            html += `<input type="radio" name="active-bootloader-set" value="${escapedName}" ${isActive ? 'checked' : ''} ${isReadOnly() ? 'disabled' : ''} onchange="selectBootloaderSet('${escapedName}')" style="width: auto; accent-color: var(--success); transform: scale(1.3);">`;
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

    document.getElementById('upload-form').addEventListener('submit', (e) => {
        e.preventDefault();

        const formData = new FormData(e.target);
        const file = formData.get('file');

        if (!file || file.size === 0) {
            showAlert('Please select a file', 'error');
            return;
        }

        const filename = file.name;
        if (pendingUploads.has(filename)) {
            showAlert('That file is already being uploaded', 'error');
            return;
        }
        pendingUploads.set(filename, {
            filename,
            name: filename.replace(/\.(iso|efi)$/i, ''),
            size: file.size,
            progress: 0,
            status: 'Starting…',
            error: null,
        });

        closeModal('upload-modal');
        e.target.reset();
        fileNameDisplay.textContent = '';
        renderImagesTable();

        const xhr = new XMLHttpRequest();
        xhr.upload.addEventListener('progress', (event) => {
            if (!event.lengthComputable) return;
            const op = pendingUploads.get(filename);
            if (!op) return;
            op.progress = (event.loaded / event.total) * 100;
            op.status = `${op.progress.toFixed(1)}% · ${formatBytes(event.loaded)}/${formatBytes(event.total)}`;
            updateUploadRowDOM(filename);
        });
        xhr.addEventListener('load', () => {
            if (xhr.status >= 200 && xhr.status < 300) {
                let data;
                try { data = JSON.parse(xhr.responseText); } catch (_) { data = { success: false, error: 'Invalid server response' }; }
                if (data.success) {
                    pendingUploads.delete(filename);
                    showAlert(`Uploaded: ${filename}`, 'success');
                    loadImages();
                    loadStats();
                } else {
                    const op = pendingUploads.get(filename);
                    if (op) {
                        op.error = data.error || 'Upload failed';
                        updateUploadRowDOM(filename);
                    }
                    showAlert(data.error || 'Upload failed', 'error');
                }
            } else {
                const op = pendingUploads.get(filename);
                if (op) {
                    op.error = `HTTP ${xhr.status}`;
                    updateUploadRowDOM(filename);
                }
                showAlert(`Upload failed: HTTP ${xhr.status}`, 'error');
            }
        });
        xhr.addEventListener('error', () => {
            const op = pendingUploads.get(filename);
            if (op) {
                op.error = 'Network error';
                updateUploadRowDOM(filename);
            }
            showAlert('Upload failed: network error', 'error');
        });
        xhr.addEventListener('abort', () => {
            const op = pendingUploads.get(filename);
            if (op) {
                op.error = 'Cancelled';
                updateUploadRowDOM(filename);
            }
        });

        xhr.open('POST', `${API_BASE}/images/upload`);
        const token = getToken();
        if (token) xhr.setRequestHeader('Authorization', 'Bearer ' + token);
        xhr.send(formData);
    });
}

function showUploadModal() {
    document.getElementById('upload-form').reset();
    document.getElementById('file-name').textContent = '';
    showModal('upload-modal');
}

// Utilities
function showModal(id) {
    const modal = document.getElementById(id);
    applyReadOnlyToModal(modal);
    modal.classList.add('active');
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

// ==================== PXE Debug ====================

let pxeDebugEvents = [];
let pxeDebugEnabled = false;
let pxeDebugInterval = null;

async function loadPXEDebug() {
    try {
        const res = await authFetch(`${API_BASE}/proxydhcp/events`);
        const data = await res.json();
        if (!data.success) return;
        pxeDebugEvents = data.events || [];
        pxeDebugEnabled = !!data.enabled;
        const banner = document.getElementById('pxe-debug-disabled');
        if (banner) banner.style.display = pxeDebugEnabled ? 'none' : 'block';
        const summary = document.getElementById('pxe-debug-summary');
        if (summary) {
            const bf = data.bootfiles || {};
            summary.innerHTML = pxeDebugEnabled
                ? `Advertising next-server <code>${escapeHtml(data.server_ip || '')}</code> with bootfiles BIOS <code>${escapeHtml(bf.bios || '')}</code>, UEFI <code>${escapeHtml(bf.uefi || '')}</code>, ARM64 <code>${escapeHtml(bf.arm64 || '')}</code>. Showing the last ${pxeDebugEvents.length} requests.`
                : '';
        }
        if (!clients || clients.length === 0) {
            try { await loadClients(); } catch (_) {}
        }
        renderPXEDebug();
    } catch (err) {
        console.error('Failed to load PXE debug events:', err);
    }
}

function pxeDebugClientName(mac) {
    const match = (clients || []).find(c => c.mac_address && c.mac_address.toLowerCase() === (mac || '').toLowerCase());
    return match && match.name ? match.name : '';
}

function renderPXEDebug() {
    const container = document.getElementById('pxe-debug-table');
    if (!container) return;
    const filterInput = document.getElementById('pxe-debug-filter');
    const q = filterInput ? filterInput.value.trim().toLowerCase() : '';
    const rows = pxeDebugEvents
        .map(ev => ({ ev, name: pxeDebugClientName(ev.mac) }))
        .filter(({ ev, name }) => !q || [ev.mac, name, ev.decision, ev.vendor_class, ev.source, ev.relay, ev.bootfile].some(v => (v || '').toLowerCase().includes(q)))
        .reverse();
    if (rows.length === 0) {
        container.innerHTML = `<p style="color: var(--text-secondary); padding: 20px;">${pxeDebugEvents.length === 0 ? 'No PXE requests recorded yet.' : 'No requests match the filter.'}</p>`;
        return;
    }
    const colours = { replied: 'var(--success)', ignored: 'var(--text-secondary)', failed: 'var(--danger)' };
    let html = '<div class="table-scroll"><table><thead><tr><th>Time</th><th>MAC</th><th>Client</th><th>Source</th><th>Relay</th><th>Type</th><th>Arch</th><th>Vendor class</th><th>Decision</th><th>Bootfile</th></tr></thead><tbody>';
    for (const { ev, name } of rows) {
        const flags = [];
        if (ev.raspberry_pi) flags.push('<span class="badge badge-info" title="Raspberry Pi firmware stage">Pi</span>');
        if (ev.vendor_option_omitted) flags.push('<span class="badge badge-warning" title="PXE vendor option 43 omitted">no opt 43</span>');
        if (ev.client_guid) flags.push(`<span style="color: var(--text-muted); font-size: 11px;" title="Client machine identifier">${escapeHtml(ev.client_guid)}</span>`);
        html += `<tr>
            <td style="white-space: nowrap;">${new Date(ev.time).toLocaleTimeString()}</td>
            <td><code>${escapeHtml(ev.mac || '')}</code></td>
            <td>${name ? escapeHtml(name) : '<span style="color: var(--text-secondary);">unknown</span>'}</td>
            <td><code>${escapeHtml(ev.source || '')}</code><br><span style="color: var(--text-muted); font-size: 11px;">port ${ev.port}</span></td>
            <td>${ev.relay ? '<code>' + escapeHtml(ev.relay) + '</code>' : '<span style="color: var(--text-secondary);">—</span>'}</td>
            <td>${escapeHtml(ev.message_type || '')}</td>
            <td title="${escapeHtml(ev.arch_name || '')}">${ev.arch}<br><span style="color: var(--text-muted); font-size: 11px;">${escapeHtml(ev.arch_name || '')}</span></td>
            <td><code style="font-size: 11px;">${escapeHtml(ev.vendor_class || '')}</code></td>
            <td style="color: ${colours[ev.outcome] || 'inherit'}; max-width: 420px;">${escapeHtml(ev.decision || '')}${flags.length ? '<br>' + flags.join(' ') : ''}</td>
            <td>${ev.bootfile ? '<code>' + escapeHtml(ev.bootfile) + '</code>' : ''}</td>
        </tr>`;
    }
    html += '</tbody></table></div>';
    container.innerHTML = html;
}

function startPXEDebugLive() {
    loadPXEDebug();
    if (!pxeDebugInterval) {
        pxeDebugInterval = setInterval(loadPXEDebug, 2000);
    }
    const status = document.getElementById('pxe-debug-status');
    status.textContent = 'Live (refreshing every 2s)';
    status.style.color = 'var(--success)';
    document.getElementById('pxe-debug-live-btn').style.display = 'none';
    document.getElementById('pxe-debug-stop-btn').style.display = 'inline-block';
}

function stopPXEDebugLive() {
    if (pxeDebugInterval) {
        clearInterval(pxeDebugInterval);
        pxeDebugInterval = null;
    }
    const status = document.getElementById('pxe-debug-status');
    status.textContent = 'Stopped';
    status.style.color = 'var(--text-secondary)';
    document.getElementById('pxe-debug-live-btn').style.display = 'inline-block';
    document.getElementById('pxe-debug-stop-btn').style.display = 'none';
}

async function clearPXEDebug() {
    try {
        const res = await authFetch(`${API_BASE}/proxydhcp/events/clear`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            pxeDebugEvents = [];
            renderPXEDebug();
        } else {
            showAlert(data.error || 'Failed to clear PXE request history', 'error');
        }
    } catch (err) {
        showAlert('Failed to clear PXE request history: ' + err.message, 'error');
    }
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
                    <th>Source</th>
                    <th>Status</th>
                    <th>2FA</th>
                    <th>Last Login</th>
                    <th>Created</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
    `;

    users.forEach(user => {
        const twoFactor = user.totp_enabled ? '<span class="badge badge-success">On</span>' : '<span style="color: var(--text-secondary);">Off</span>';
        const role = user.is_admin ? '<span class="badge badge-info">Admin</span>' : '<span class="badge badge-success">Read-only</span>';
        const source = { local: 'Local', ldap: 'LDAP', oidc: 'OIDC' }[user.source || 'local'] || user.source;
        const isLocal = !user.source || user.source === 'local';
        const status = user.enabled ? '<span class="badge badge-success">Enabled</span>' : '<span class="badge badge-danger">Disabled</span>';
        const lastLogin = user.last_login ? new Date(user.last_login).toLocaleString() : 'Never';
        const created = new Date(user.created_at).toLocaleString();

        html += `
            <tr>
                <td><strong>${escapeHtml(user.username)}</strong></td>
                <td>${role}</td>
                <td>${escapeHtml(source)}</td>
                <td>${status}</td>
                <td>${twoFactor}</td>
                <td>${lastLogin}</td>
                <td>${created}</td>
                <td>
                    <button class="btn btn-info btn-sm" onclick='editUser(${JSON.stringify(user)})'>Edit</button>
                    ${isLocal ? `<button class="btn btn-sm" onclick='showResetPasswordModal(${JSON.stringify(user)})'>Reset Password</button>` : ''}
                    ${isLocal && user.totp_enabled ? `<button class="btn btn-sm" onclick="resetUserTOTP(${user.id}, '${escapeHtml(user.username).replace(/'/g, "\\'")}')">Reset 2FA</button>` : ''}
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

    const note = document.getElementById('edit-user-external-note');
    if (user.source && user.source !== 'local') {
        note.textContent = `This user signs in via ${user.source.toUpperCase()}. Administrator rights are reset from their groups at each sign-in.`;
        note.style.display = 'block';
    } else {
        note.style.display = 'none';
    }

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

let isoCatalog = null;

async function showGetImagesModal() {
    if (!isoCatalog) {
        try {
            const res = await authFetch(`${API_BASE}/iso-catalog`);
            const data = await res.json();
            if (data.success) {
                isoCatalog = data.data;
            } else {
                showAlert(data.error || 'Failed to load ISO catalogue', 'error');
                return;
            }
        } catch (e) {
            showAlert('Failed to load ISO catalogue', 'error');
            return;
        }
    }
    const filter = document.getElementById('get-iso-filter');
    if (filter) filter.value = '';
    renderGetImagesList('');
    openModal('get-images-modal');
}

function renderGetImagesList(filter) {
    const container = document.getElementById('get-images-list');
    if (!container) return;
    const q = (filter || '').trim().toLowerCase();
    let html = '';
    for (const distro of (isoCatalog.distros || [])) {
        const matching = (distro.releases || []).filter(r =>
            !q || distro.name.toLowerCase().includes(q) || r.label.toLowerCase().includes(q) || distro.id.toLowerCase().includes(q)
        );
        if (matching.length === 0) continue;
        html += `<div class="get-iso-distro"><h3>${escapeHtml(distro.name)}</h3>`;
        for (let ri = 0; ri < matching.length; ri++) {
            const r = matching[ri];
            const realIdx = distro.releases.indexOf(r);
            const rowKey = `${distro.id}-${realIdx}`;
            const defaultBase = (distro.mirrors[0] && distro.mirrors[0].base) || '';
            const defaultURL = defaultBase.replace(/\/+$/, '') + r.path;
            const mirrorOpts = (distro.mirrors || []).map(m =>
                `<option value="${escapeHtml(m.base)}">${escapeHtml(m.region)}</option>`
            ).join('');
            html += `<div class="get-iso-row">
                <div class="get-iso-label" title="${escapeHtml(r.label)}">${escapeHtml(r.label)}</div>
                <select class="get-iso-mirror-sel" data-row="${escapeHtml(rowKey)}" data-path="${escapeHtml(r.path)}" onchange="updateGetISORowURL(this)">${mirrorOpts}</select>
                <input type="text" id="get-iso-url-${escapeHtml(rowKey)}" class="get-iso-url" value="${escapeHtml(defaultURL)}">
                <div id="get-iso-actions-${escapeHtml(rowKey)}" class="get-iso-actions">
                    <button type="button" class="btn btn-sm" onclick="downloadFromGetISO('${escapeHtml(rowKey)}', '${escapeHtml(distro.name)}', '${escapeHtml(r.label)}', '${escapeHtml(distro.id)}', '${escapeHtml(r.sha256 || '')}', '${escapeHtml(r.checksum_path || '')}')">
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                        Download
                    </button>
                </div>
            </div>`;
        }
        html += '</div>';
    }
    if (!html) html = '<p style="color: var(--text-secondary); padding: 20px;">No matches.</p>';
    container.innerHTML = html;
}

function updateGetISORowURL(select) {
    const rowKey = select.dataset.row;
    const path = select.dataset.path;
    const base = select.value.replace(/\/+$/, '');
    const input = document.getElementById('get-iso-url-' + rowKey);
    if (input) input.value = base + path;
}

const getISOActiveDownloads = new Map();

function jsAttrEscape(s) {
    return String(s).replace(/\\/g, '\\\\').replace(/'/g, "\\'");
}

async function downloadFromGetISO(rowKey, distroName, releaseLabel, catalogueId, sha256, checksumPath) {
    const input = document.getElementById('get-iso-url-' + rowKey);
    if (!input) return;
    const url = input.value.trim();
    if (!url) {
        showAlert('URL is required', 'error');
        return;
    }
    renderGetISOProgress(rowKey, 'Starting…');
    try {
        const body = { url, description: `${distroName} ${releaseLabel}` };
        if (!catalogueId) catalogueId = (rowKey || '').replace(/-\d+$/, '');
        body.catalogue_id = catalogueId;
        body.catalogue_release = releaseLabel;
        if (sha256) body.sha256 = sha256;
        if (checksumPath) body.checksum_url = checksumPath;
        const res = await authFetch(`${API_BASE}/images/download`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (res.status === 409 && data.data && data.data.filename) {
            showNotification('Download already in progress — resuming progress display', 'info');
            startGetISOProgressPolling(rowKey, data.data.filename, distroName, releaseLabel);
        } else if (data.success && data.data && data.data.filename) {
            startGetISOProgressPolling(rowKey, data.data.filename, distroName, releaseLabel);
        } else {
            renderGetISOError(rowKey, data.error || 'Failed to start download', distroName, releaseLabel);
        }
    } catch (e) {
        renderGetISOError(rowKey, e.message || 'Network error', distroName, releaseLabel);
    }
}

function renderGetISOProgress(rowKey, statusText) {
    const el = document.getElementById('get-iso-actions-' + rowKey);
    if (!el) return;
    el.innerHTML = `
        <div class="get-iso-progress">
            <div class="get-iso-bar"><div id="get-iso-bar-fill-${escapeHtml(rowKey)}" class="get-iso-bar-fill" style="width: 0%;"></div></div>
            <span id="get-iso-status-${escapeHtml(rowKey)}" class="get-iso-status-text">${escapeHtml(statusText)}</span>
        </div>`;
}

function startGetISOProgressPolling(rowKey, filename, distroName, releaseLabel) {
    const existing = getISOActiveDownloads.get(rowKey);
    if (existing) clearInterval(existing);

    const stop = () => {
        const id = getISOActiveDownloads.get(rowKey);
        if (id) clearInterval(id);
        getISOActiveDownloads.delete(rowKey);
    };
    const tick = async () => {
        try {
            const res = await authFetch(`${API_BASE}/downloads/progress?filename=${encodeURIComponent(filename)}`);
            if (res.status === 404) {
                stop();
                renderGetISOError(rowKey, 'Download record disappeared (likely failed before tracking started)', distroName, releaseLabel);
                return;
            }
            const data = await res.json();
            if (!data.success) {
                stop();
                renderGetISOError(rowKey, data.error || 'Progress unavailable', distroName, releaseLabel);
                return;
            }
            if (data.data) {
                const p = data.data;
                const pct = (p.percentage || 0).toFixed(1);
                const fill = document.getElementById('get-iso-bar-fill-' + rowKey);
                const status = document.getElementById('get-iso-status-' + rowKey);
                if (fill) fill.style.width = pct + '%';
                if (status) status.textContent = `${pct}% · ${p.speed || ''}${p.verification === 'pending' ? ' · verifying SHA-256' : ''}`;

                if (p.status === 'completed') {
                    stop();
                    renderGetISOComplete(rowKey, distroName, releaseLabel, p.verification === 'verified');
                    loadImages();
                } else if (p.status === 'error') {
                    stop();
                    renderGetISOError(rowKey, p.error || 'Download failed', distroName, releaseLabel);
                }
            }
        } catch (e) {
            stop();
            renderGetISOError(rowKey, 'Lost connection to server', distroName, releaseLabel);
        }
    };
    tick();
    const intervalId = setInterval(tick, 1000);
    getISOActiveDownloads.set(rowKey, intervalId);
}

function renderGetISOError(rowKey, errorMsg, distroName, releaseLabel) {
    const el = document.getElementById('get-iso-actions-' + rowKey);
    if (!el) return;
    const short = errorMsg.length > 50 ? errorMsg.slice(0, 50) + '…' : errorMsg;
    el.innerHTML = `
        <div class="get-iso-progress">
            <span class="get-iso-status-text" style="color: var(--danger);" title="${escapeHtml(errorMsg)}">✗ ${escapeHtml(short)}</span>
            <button type="button" class="btn btn-sm" onclick="downloadFromGetISO('${jsAttrEscape(rowKey)}', '${jsAttrEscape(distroName)}', '${jsAttrEscape(releaseLabel)}')">Retry</button>
        </div>`;
}

function renderGetISOComplete(rowKey, distroName, releaseLabel, verified) {
    const el = document.getElementById('get-iso-actions-' + rowKey);
    if (!el) return;
    el.innerHTML = `
        <div class="get-iso-progress">
            <span class="get-iso-status-text" style="color: var(--success);" title="${verified ? 'SHA-256 matched the published checksum' : 'No published checksum was found, so the file was not verified'}">✓ Downloaded${verified ? ' and verified' : ' (unverified)'}</span>
            <button type="button" class="btn btn-sm" onclick="downloadFromGetISO('${jsAttrEscape(rowKey)}', '${jsAttrEscape(distroName)}', '${jsAttrEscape(releaseLabel)}')">Again</button>
        </div>`;
}

function clearGetISOPolling() {
    for (const id of getISOActiveDownloads.values()) clearInterval(id);
    getISOActiveDownloads.clear();
}

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
    .then(response => response.json().then(data => ({ status: response.status, data })))
    .then(({ status, data }) => {
        if (status === 409 && data.data && data.data.filename) {
            showNotification('Download already in progress — resuming progress display', 'info');
            const filename = data.data.filename;
            downloadProgressInterval = setInterval(() => {
                checkDownloadProgress(filename);
            }, 1000);
        } else if (data.success) {
            showNotification('Download started: ' + data.data.filename, 'success');

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
        case 'openbsd':
        case 'netbsd':
            return '';
        case 'windows':
        case 'windows7':
        case 'winpe':
            return '';
        default:
            return 'iso-url={{BASE_URL}}/isos/{{FILENAME}} ip=dhcp';
    }
}

function renderImageIntegrity(img) {
    const status = document.getElementById('image-props-checksum-status');
    const input = document.getElementById('image-props-checksum-input');
    const update = document.getElementById('image-props-update');
    const updateText = document.getElementById('image-props-update-text');
    if (!status) return;
    input.value = '';
    if (img.checksum_verified && img.checksum) {
        status.innerHTML = `<span class="badge badge-success">✓ Verified</span> <code title="${escapeHtml(img.checksum)}">sha256 ${escapeHtml(img.checksum.slice(0, 16))}…</code>${img.checksum_checked_at ? ' · ' + new Date(img.checksum_checked_at).toLocaleString() : ''}`;
        input.placeholder = 'Re-verify against the recorded checksum, or paste another';
    } else if (img.checksum) {
        status.innerHTML = `<span class="badge badge-warning">Unverified</span> <code title="${escapeHtml(img.checksum)}">sha256 ${escapeHtml(img.checksum.slice(0, 16))}…</code>`;
        input.placeholder = 'SHA-256 or URL of a SHA256SUMS file to verify against';
    } else {
        status.innerHTML = '<span class="badge badge-warning">Unverified</span> No checksum recorded';
        input.placeholder = 'SHA-256 or URL of a SHA256SUMS file to verify against';
    }
    if (img.update_available) {
        update.style.display = 'flex';
        updateText.textContent = `A newer release is in the catalogue: ${img.update_version || ''} (${img.update_filename || img.update_url}).`;
    } else {
        update.style.display = 'none';
    }
}

async function verifyImageFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    const value = document.getElementById('image-props-checksum-input').value.trim();
    const body = {};
    if (/^https?:\/\//i.test(value)) body.checksum_url = value;
    else if (value) body.sha256 = value;
    const status = document.getElementById('image-props-checksum-status');
    const btn = document.getElementById('image-props-verify-btn');
    try {
        const res = await authFetch(`${API_BASE}/images/verify?filename=${encodeURIComponent(filename)}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Verification could not start', 'error');
            return;
        }
        btn.disabled = true;
        const poll = setInterval(async () => {
            try {
                const pr = await authFetch(`${API_BASE}/images/verify?filename=${encodeURIComponent(filename)}`);
                const pd = await pr.json();
                if (!pd.success) { clearInterval(poll); btn.disabled = false; return; }
                const p = pd.data;
                if (p.status === 'running') {
                    status.textContent = `Hashing… ${(p.percent || 0).toFixed(0)}%`;
                    return;
                }
                clearInterval(poll);
                btn.disabled = false;
                if (p.status === 'completed' && p.verified) {
                    showAlert('Checksum verified', 'success');
                } else {
                    showAlert(p.error || 'Checksum mismatch', 'error');
                }
                await loadImages();
                const img = images.find(i => i.filename === filename);
                if (img && document.getElementById('image-props-filename').value === filename) renderImageIntegrity(img);
            } catch (_) {
                clearInterval(poll);
                btn.disabled = false;
            }
        }, 1000);
    } catch (err) {
        showAlert('Verification could not start: ' + err.message, 'error');
    }
}

async function downloadNewerImage() {
    const filename = document.getElementById('image-props-filename').value;
    const img = images.find(i => i.filename === filename);
    if (!img || !img.update_url) return;
    try {
        const res = await authFetch(`${API_BASE}/images/download`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: img.update_url, description: img.description || '', catalogue_id: img.catalogue_id || '', catalogue_release: img.catalogue_release || '' }),
        });
        const data = await res.json();
        if (data.success || res.status === 409) {
            showAlert(`Downloading ${data.data && data.data.filename ? data.data.filename : 'newer release'} – progress is shown in the downloads list`, 'success');
            closeModal('image-properties-modal');
            if (typeof loadDownloads === 'function') loadDownloads();
        } else {
            showAlert(data.error || 'Download failed to start', 'error');
        }
    } catch (err) {
        showAlert('Download failed to start: ' + err.message, 'error');
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
    const logoEl = document.getElementById('image-props-logo');
    logoEl.src = distroLogoSrc(img.distro);
    logoEl.alt = img.distro || '';
    logoEl.onerror = () => { logoEl.onerror = null; logoEl.src = DISTRO_LOGO_FALLBACK; };
    document.getElementById('image-props-display-name').value = img.name || '';
    document.getElementById('image-props-description').value = img.description || '';
    document.getElementById('image-props-order').value = img.order || 0;
    const bootMethodSelect = document.getElementById('image-props-boot-method');
    const isUKI = img.boot_method === 'uki';
    bootMethodSelect.querySelector('option[value="uki"]').hidden = !isUKI;
    bootMethodSelect.disabled = isUKI;
    bootMethodSelect.value = img.boot_method || 'sanboot';

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
    document.getElementById('image-props-boot-password').value = '';
    document.getElementById('image-props-remove-boot-password').checked = false;
    document.getElementById('image-props-boot-password-status').textContent = img.boot_password_required ? 'Password set.' : 'No password set.';
    renderImageIntegrity(img);

    await populateBootFileOverrides(img);
    populateWIMSelector(img);

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
    if (isUKI) {
        extractBtn.style.display = 'none';
        netbootBtn.style.display = 'none';
    } else if (extractionProgress[filename]) {
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

    const unpatchSmbBtn = document.getElementById('image-props-unpatch-smb-btn');
    const unpatchVisible = img.extracted && img.distro === 'windows' && img.smb_install_enabled;
    unpatchSmbBtn.style.display = unpatchVisible ? 'inline-block' : 'none';
    unpatchSmbBtn.textContent = t('props.action.unpatch_smb');
    unpatchSmbBtn.disabled = !img.smb_unpatch_available;
    unpatchSmbBtn.style.opacity = img.smb_unpatch_available ? '' : '0.5';
    unpatchSmbBtn.title = img.smb_unpatch_available ? t('props.action.unpatch_smb_tip') : t('props.action.unpatch_smb_unavailable');

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

async function populateBootFileOverrides(img) {
    const row = document.getElementById('image-props-bootfiles-row');
    const kernelSelect = document.getElementById('image-props-kernel-override');
    const initrdSelect = document.getElementById('image-props-initrd-override');
    const autoOption = `<option value="">${t('props.field.bootfile_auto')}</option>`;
    kernelSelect.innerHTML = autoOption;
    initrdSelect.innerHTML = autoOption;

    if (!img.extracted || isWindowsFamily(img.distro)) {
        row.style.display = 'none';
        return;
    }

    try {
        const res = await authFetch(`${API_BASE}/images/boot-candidates?filename=${encodeURIComponent(img.filename)}`);
        const data = await res.json();
        if (!data.success || !data.data) throw new Error(data.error || 'failed');

        const addOptions = (select, candidates, current) => {
            for (const c of candidates || []) {
                const selected = current === c.path ? 'selected' : '';
                select.innerHTML += `<option value="${escapeHtml(c.path)}" ${selected}>${escapeHtml(c.path)} (${formatBytes(c.size)})</option>`;
            }
        };
        addOptions(kernelSelect, data.data.kernels, data.data.kernel_override);
        addOptions(initrdSelect, data.data.initrds, data.data.initrd_override);

        const hasCandidates = (data.data.kernels || []).length > 0 || (data.data.initrds || []).length > 0;
        row.style.display = hasCandidates ? '' : 'none';
    } catch (e) {
        row.style.display = 'none';
    }
}

function updateImagePropsWarnings() {
    if (!_imagePropsState) return;
    const img = _imagePropsState.img;
    const fname = img.filename || '';
    const looksWindows = isWindowsFamily(img.distro) || filenameLooksWindows(fname);

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

function isWindowsFamily(distro) {
    return distro === 'windows' || distro === 'windows7' || distro === 'winpe';
}

function populateWIMSelector(img) {
    const row = document.getElementById('image-props-wim-row');
    const select = document.getElementById('image-props-wim-path');
    const index = document.getElementById('image-props-wim-index');
    if (!row || !select || !index) return;
    const wims = img.wim_files || [];
    if (!img.extracted || !isWindowsFamily(img.distro) || wims.length === 0) {
        row.style.display = 'none';
        return;
    }
    select.innerHTML = wims.map(w =>
        `<option value="${escapeHtml(w.path)}" ${w.path === img.wim_path ? 'selected' : ''}>${escapeHtml(w.path)} (${formatBytes(w.size || 0)})</option>`
    ).join('');
    if (img.wim_path && !wims.some(w => w.path === img.wim_path)) {
        select.innerHTML += `<option value="${escapeHtml(img.wim_path)}" selected>${escapeHtml(img.wim_path)}</option>`;
    }
    index.value = img.wim_index || 1;
    row.dataset.path = img.wim_path || '';
    row.dataset.index = String(img.wim_index || 0);
    row.style.display = '';
}

async function saveWIMSelection(filename) {
    const row = document.getElementById('image-props-wim-row');
    if (!row || row.style.display === 'none') return true;
    const path = document.getElementById('image-props-wim-path').value;
    const index = parseInt(document.getElementById('image-props-wim-index').value, 10) || 0;
    const storedIndex = parseInt(row.dataset.index, 10) || 0;
    if (path === row.dataset.path && (index === storedIndex || (index <= 1 && storedIndex <= 1))) return true;
    const res = await authFetch(`${API_BASE}/images/winpe?filename=${encodeURIComponent(filename)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ wim_path: path, wim_index: index <= 1 ? 0 : index })
    });
    const data = await res.json();
    if (!data.success) {
        showNotification('Failed to update boot WIM: ' + (data.error || 'Unknown error'), 'error');
        return false;
    }
    return true;
}

function applyBootParamsWindowsLock(distro) {
    const input = document.getElementById('image-props-boot-params');
    const redetect = document.getElementById('image-props-redetect-btn');
    const isWindows = isWindowsFamily(distro);
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

async function unpatchSmbFromProperties() {
    const filename = document.getElementById('image-props-filename').value;
    const btn = document.getElementById('image-props-unpatch-smb-btn');
    btn.disabled = true;
    btn.textContent = t('props.action.unpatching');
    try {
        const res = await authFetch(`${API_BASE}/images/unpatch-smb?filename=${encodeURIComponent(filename)}`, { method: 'POST' });
        const data = await res.json();
        if (!data.success) throw new Error(data.error || t('props.notify.unpatch_failed'));
        showNotification(t('props.notify.unpatch_success'), 'success');
        await loadImages();
        refreshImagePropsIfOpenFor(filename);
    } catch (err) {
        showNotification(t('props.notify.unpatch_failed') + ': ' + err.message, 'error');
    } finally {
        btn.disabled = false;
        btn.textContent = t('props.action.unpatch_smb');
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

    const baseDir = filename.replace(/\.[^/.]+$/, '');
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
    const bootParamsInput = document.getElementById('image-props-boot-params');
    const bootParams = bootParamsInput.disabled ? '' : bootParamsInput.value;
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

    const bootPassword = document.getElementById('image-props-boot-password').value;
    if (document.getElementById('image-props-remove-boot-password').checked) updates.boot_password = '';
    else if (bootPassword) {
        if (!validateBootPassword(bootPassword)) return false;
        updates.boot_password = bootPassword;
    }

    const bootFilesRow = document.getElementById('image-props-bootfiles-row');
    if (bootFilesRow && bootFilesRow.style.display !== 'none') {
        updates.kernel_override = document.getElementById('image-props-kernel-override').value;
        updates.initrd_override = document.getElementById('image-props-initrd-override').value;
    }

    try {
        if (!await saveWIMSelection(filename)) return false;

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
        'proxy_dhcp_no_vendor_option',
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
        c.proxy_dhcp_no_vendor_option ? 'true' : 'false',
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
    document.getElementById('san-targets-subtab').style.display = name === 'san-targets' ? '' : 'none';
    document.getElementById('deployment-profiles-subtab').style.display = name === 'deployment-profiles' ? '' : 'none';
    document.getElementById('enrolment-rules-subtab').style.display = name === 'enrolment-rules' ? '' : 'none';
    if (name === 'client-groups') loadClientGroups();
    if (name === 'san-targets') loadSANTargets();
    if (name === 'deployment-profiles') loadDeploymentProfiles();
    if (name === 'enrolment-rules') loadEnrolmentRules();
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
                bmc_protocol: fd.get('bmc_protocol') || '',
                ipmi_port: parseInt(fd.get('ipmi_port') || '0', 10),
                ipmi_username: fd.get('ipmi_username') || '',
                ipmi_password: fd.get('ipmi_password') || '',
                ipmi_insecure: fd.get('ipmi_insecure') === 'on',
                auto_install_file: fd.get('auto_install_file') || '',
                san_target_id: fd.get('san_target_id') ? parseInt(fd.get('san_target_id'), 10) : null,
                boot_from_san: fd.get('boot_from_san') === 'on',
                profile_id: fd.get('profile_id') ? parseInt(fd.get('profile_id'), 10) : null,
            };
            try {
                body.variables = collectVariables('cg-variables');
            } catch (err) {
                showAlert(err.message, 'error');
                return;
            }
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

const VARIABLE_NAME_PATTERN = /^[A-Za-z0-9_]+$/;

function renderVariableRows(containerId, vars) {
    const container = document.getElementById(containerId);
    if (!container) return;
    container.innerHTML = '';
    for (const name of Object.keys(vars || {}).sort()) {
        addVariableRow(containerId, name, vars[name]);
    }
}

function addVariableRow(containerId, name = '', value = '') {
    const container = document.getElementById(containerId);
    if (!container) return;
    const row = document.createElement('div');
    row.className = 'kv-row';

    const nameInput = document.createElement('input');
    nameInput.type = 'text';
    nameInput.className = 'kv-name';
    nameInput.placeholder = t('variables.name');
    nameInput.autocomplete = 'off';
    nameInput.spellcheck = false;
    nameInput.value = name;

    const valueInput = document.createElement('input');
    valueInput.type = 'text';
    valueInput.className = 'kv-value';
    valueInput.placeholder = t('variables.value');
    valueInput.autocomplete = 'off';
    valueInput.value = value;

    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'btn btn-sm btn-icon';
    remove.title = t('variables.remove');
    remove.setAttribute('aria-label', t('variables.remove'));
    remove.textContent = '×';
    remove.onclick = () => row.remove();

    row.append(nameInput, valueInput, remove);
    container.appendChild(row);
    if (!name) nameInput.focus();
}

function collectVariables(containerId) {
    const vars = {};
    for (const row of document.querySelectorAll(`#${containerId} .kv-row`)) {
        const name = row.querySelector('.kv-name').value.trim();
        if (!name) continue;
        if (!VARIABLE_NAME_PATTERN.test(name)) {
            throw new Error(t('variables.invalid_name', { name }));
        }
        vars[name] = row.querySelector('.kv-value').value;
    }
    return vars;
}

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
        form.elements.bmc_protocol.value = g.bmc_protocol || 'redfish';
        updateBMCPortPlaceholder(form.elements.bmc_protocol);
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
        renderVariableRows('cg-variables', g.variables);
        await populateSANTargetSelect('cg-san-select', g.san_target_id, '(none)');
        form.elements.boot_from_san.checked = !!g.boot_from_san;
        await populateDeploymentProfileSelect('cg-profile-select', g.profile_id, '(none)');

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
        showAlert(data.message || data.error || (data.success ? 'Wake started' : 'Wake failed'), data.success ? 'success' : 'error');
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

async function confirmAutoInstallUpload(force = false) {
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
    if (force) fd.append('force', 'true');
    try {
        const token = localStorage.getItem('bootimus_token');
        const res = await fetch(`${API_BASE}/autoinstall-files/upload`, {
            method: 'POST',
            headers: token ? { 'Authorization': 'Bearer ' + token } : {},
            body: fd,
        });
        const data = await res.json();
        if (!data.success && data.data && data.data.validation_failed) {
            if (confirm(t('autoinstall.upload_anyway_confirm', { error: data.error }))) {
                return confirmAutoInstallUpload(true);
            }
            return;
        }
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
    const previewBtn = document.getElementById('autoinstall-editor-preview-btn');
    hideAutoInstallEditorError();
    if (file) {
        title.textContent = `Edit ${file.distro}/${file.filename}`;
        distroSel.value = file.distro;
        distroSel.disabled = true;
        filenameInput.value = file.filename;
        filenameInput.disabled = true;
        delBtn.style.display = 'inline-block';
        dlBtn.style.display = 'inline-block';
        previewBtn.style.display = 'inline-block';
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
        previewBtn.style.display = 'none';
    }

    openModal('autoinstall-editor-modal');
}

function hideAutoInstallEditorError() {
    const box = document.getElementById('autoinstall-editor-error');
    if (box) box.style.display = 'none';
}

function showAutoInstallEditorError(message) {
    const box = document.getElementById('autoinstall-editor-error');
    if (!box) return;
    document.getElementById('autoinstall-editor-error-text').textContent = t('autoinstall.validation_failed', { error: message });
    box.style.display = 'flex';
}

async function saveAutoInstallFile(force = false) {
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
            body: JSON.stringify({ distro, filename, content, force: !!force }),
        });
        const data = await res.json();
        if (!data.success && data.data && data.data.validation_failed) {
            showAutoInstallEditorError(data.error);
            return;
        }
        if (!data.success) throw new Error(data.error || 'Save failed');
        hideAutoInstallEditorError();
        showNotification('Saved', 'success');
        closeModal('autoinstall-editor-modal');
        loadAutoInstallFiles();
    } catch (err) {
        showNotification('Save failed: ' + err.message, 'error');
    }
}

let autoInstallPreviewFile = null;

async function showAutoInstallPreview() {
    const distro = document.getElementById('autoinstall-editor-distro').value;
    const filename = document.getElementById('autoinstall-editor-filename').value;
    if (!distro || !filename) return;
    autoInstallPreviewFile = { distro, filename };
    document.getElementById('autoinstall-preview-title').textContent = t('autoinstall.preview_title', { file: `${distro}/${filename}` });

    if (!clients || clients.length === 0) await loadClients();
    if (!images || images.length === 0) await loadImages();

    const clientSel = document.getElementById('autoinstall-preview-client');
    clientSel.innerHTML = `<option value="">${escapeHtml(t('autoinstall.preview_no_client'))}</option>`;
    const sortedClients = [...clients].sort((a, b) => (a.name || a.mac_address).localeCompare(b.name || b.mac_address));
    for (const c of sortedClients) {
        const label = c.name ? `${c.name} (${c.mac_address})` : c.mac_address;
        clientSel.innerHTML += `<option value="${escapeHtml(c.mac_address)}">${escapeHtml(label)}</option>`;
    }

    const imageSel = document.getElementById('autoinstall-preview-image');
    imageSel.innerHTML = `<option value="">${escapeHtml(t('autoinstall.preview_no_image'))}</option>`;
    const matching = images.filter(img => img.distro === distro);
    const others = images.filter(img => img.distro !== distro);
    for (const img of [...matching, ...others]) {
        imageSel.innerHTML += `<option value="${escapeHtml(img.filename)}">${escapeHtml(img.name || img.filename)}</option>`;
    }
    if (matching.length > 0) imageSel.value = matching[0].filename;

    openModal('autoinstall-preview-modal');
    renderAutoInstallPreview();
}

async function renderAutoInstallPreview() {
    if (!autoInstallPreviewFile) return;
    const mac = document.getElementById('autoinstall-preview-client').value;
    const image = document.getElementById('autoinstall-preview-image').value;
    const output = document.getElementById('autoinstall-preview-output');
    const unresolvedBox = document.getElementById('autoinstall-preview-unresolved');
    const resolvedBox = document.getElementById('autoinstall-preview-resolved');
    output.textContent = t('common.loading');
    unresolvedBox.style.display = 'none';
    resolvedBox.style.display = 'none';

    let url = `${API_BASE}/autoinstall-files/render?distro=${encodeURIComponent(autoInstallPreviewFile.distro)}&filename=${encodeURIComponent(autoInstallPreviewFile.filename)}`;
    if (mac) url += `&mac=${encodeURIComponent(mac)}`;
    if (image) url += `&image=${encodeURIComponent(image)}`;
    try {
        const res = await authFetch(url);
        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Render failed');
        output.textContent = data.data.content || '';
        const unresolved = data.data.unresolved || [];
        if (unresolved.length > 0) {
            const list = unresolved.map(name => `{{${name}}}`).join(', ');
            unresolvedBox.innerHTML = `<strong>${escapeHtml(t('autoinstall.preview_unresolved', { list }))}</strong><br><span style="font-size: 12px;">${escapeHtml(t('autoinstall.preview_unresolved_hint'))}</span>`;
            unresolvedBox.style.display = 'block';
        } else {
            resolvedBox.style.display = 'block';
        }
    } catch (err) {
        output.textContent = '';
        showNotification(t('autoinstall.preview_failed', { error: err.message }), 'error');
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

let clusters = [];
let clusterNodes = [];
let nodeImages = [];
let kubernetesProviders = [];
let clusterConfigMode = 'edit';

const INSTALL_STATE_LABELS = {
    booted: ['Booted', 'badge-info'],
    installing: ['Installing', 'badge-warning'],
    completed: ['Installed', 'badge-success'],
    failed: ['Failed', 'badge-danger'],
};

function relativeTime(iso) {
    if (!iso) return '';
    const diff = Date.now() - new Date(iso).getTime();
    const mins = Math.round(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins} min ago`;
    const hours = Math.round(mins / 60);
    if (hours < 48) return `${hours} h ago`;
    return `${Math.round(hours / 24)} d ago`;
}

function installStateBadge(client) {
    const entry = INSTALL_STATE_LABELS[client.install_state];
    if (!entry) return '<span style="color: var(--text-muted);">-</span>';
    const title = `${escapeHtml(client.install_image || '')} ${client.install_updated_at ? new Date(client.install_updated_at).toLocaleString() : ''}`.trim();
    return `<span class="badge ${entry[1]}" title="${title}">${entry[0]}</span><br><span style="font-size: 11px; color: var(--text-muted);">${relativeTime(client.install_updated_at)}</span>`;
}

async function loadClientInstallHistory(mac, containerId) {
    const container = document.getElementById(containerId || 'client-install-history');
    if (!container) return;
    container.innerHTML = '<span style="color: var(--text-muted);">Loading…</span>';
    try {
        const res = await authFetch(`${API_BASE}/clients/installs?mac=${encodeURIComponent(mac)}&limit=20`);
        const data = await res.json();
        if (!data.success) {
            container.innerHTML = `<span style="color: var(--danger);">${escapeHtml(data.error || 'Failed to load install history')}</span>`;
            return;
        }
        const recs = data.data || [];
        if (recs.length === 0) {
            container.innerHTML = '<span style="color: var(--text-muted);">No unattended installs recorded for this client.</span>';
            return;
        }
        container.innerHTML = `<div class="table-scroll"><table>
            <thead><tr><th>Booted</th><th>Image</th><th>State</th><th>Finished</th><th>Message</th></tr></thead>
            <tbody>${recs.map(r => {
                const entry = INSTALL_STATE_LABELS[r.state] || [r.state, 'badge-info'];
                return `<tr>
                    <td style="white-space: nowrap;">${new Date(r.booted_at).toLocaleString()}</td>
                    <td>${escapeHtml(r.image_name || r.image_filename || '')}</td>
                    <td><span class="badge ${entry[1]}">${escapeHtml(entry[0])}</span></td>
                    <td style="white-space: nowrap;">${r.finished_at ? new Date(r.finished_at).toLocaleString() : '-'}</td>
                    <td>${escapeHtml(r.message || '')}</td>
                </tr>`;
            }).join('')}</tbody></table></div>`;
    } catch (err) {
        container.innerHTML = '<span style="color: var(--danger);">Failed to load install history</span>';
    }
}

function enrollmentStateBadge(state) {
    const classes = {
        pending: 'badge-disabled',
        approved: 'badge-info',
        installing: 'badge-warning',
        installed: 'badge-success',
        rejected: 'badge-danger',
    };
    return `<span class="badge ${classes[state] || 'badge-info'}">${escapeHtml(state)}</span>`;
}

function loadClustersTab() {
    loadKubernetesProviders();
    loadNodeImages();
    loadClusters();
    loadClusterNodes();
}

function providerLabel(name) {
    const p = kubernetesProviders.find(p => p.name === name);
    if (p && p.label) return p.label;
    return name ? name.charAt(0).toUpperCase() + name.slice(1) : '-';
}

async function loadKubernetesProviders() {
    try {
        const res = await authFetch(`${API_BASE}/kubernetes/providers`);
        const data = await res.json();
        if (data.success) kubernetesProviders = data.data || [];
    } catch (err) {}
}

async function loadNodeImages() {
    try {
        const res = await authFetch(`${API_BASE}/node-images`);
        const data = await res.json();
        if (data.success) {
            nodeImages = data.data || [];
            renderNodeImagesTable();
        } else {
            showAlert(data.error || 'Failed to load node images', 'error');
        }
    } catch (err) {
        document.getElementById('node-images-table').innerHTML = '<p class="alert alert-error">Failed to load node images</p>';
    }
}

function renderNodeImagesTable() {
    const container = document.getElementById('node-images-table');
    if (!container) return;
    if (nodeImages.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No node images yet. Add one to get started.</p>';
        return;
    }
    const rows = nodeImages.map(img => `
                    <tr>
                        <td><strong>${escapeHtml(img.name || '-')}</strong></td>
                        <td>${escapeHtml(providerLabel(img.provider))}</td>
                        <td>${img.version ? escapeHtml(img.version) : '<span style="color: var(--text-secondary);">-</span>'}</td>
                        <td>${img.image_ref ? '<code title="' + escapeHtml(img.image_ref) + '">' + escapeHtml(img.image_ref.length > 24 ? img.image_ref.slice(0, 24) + '…' : img.image_ref) + '</code>' : '<span style="color: var(--text-secondary);">-</span>'}</td>
                        <td>${img.downloaded ?
                            '<span class="badge badge-success">Downloaded ✓</span>' :
                            '<button class="btn btn-sm" id="node-image-dl-btn-' + img.id + '" onclick="downloadNodeImage(' + img.id + ')">Download</button>'
                        }</td>
                        <td><button class="btn btn-danger btn-sm" onclick="deleteNodeImage(${img.id})">Delete</button></td>
                    </tr>`).join('');
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Provider</th>
                    <th>Version</th>
                    <th>Image Ref</th>
                    <th>Downloaded</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>${rows}</tbody>
        </table>
        </div>
    `;
}

async function showAddNodeImageModal() {
    document.getElementById('node-image-form').reset();
    if (kubernetesProviders.length === 0) await loadKubernetesProviders();
    const select = document.getElementById('node-image-provider');
    select.innerHTML = kubernetesProviders.map(p => `<option value="${escapeHtml(p.name)}">${escapeHtml(p.label)}</option>`).join('');
    onNodeImageProviderChange();
    showModal('node-image-modal');
}

function onNodeImageProviderChange() {
    const name = document.getElementById('node-image-provider').value;
    const p = kubernetesProviders.find(p => p.name === name);
    const fields = document.getElementById('node-image-provider-fields');
    if (!p) {
        fields.style.display = 'none';
        return;
    }
    fields.style.display = '';
    document.getElementById('node-image-version-label').textContent = p.version_label || 'Version';
    const versionInput = document.getElementById('node-image-version');
    versionInput.placeholder = p.version_default || '';
    document.getElementById('node-image-ref-label').textContent = p.ref_label || 'Image Reference';
    const refInput = document.getElementById('node-image-ref');
    refInput.placeholder = p.ref_default || '';
    const refHelp = document.getElementById('node-image-ref-help');
    refHelp.textContent = p.ref_help || '';
    refHelp.style.display = p.ref_help ? '' : 'none';
}

async function saveNodeImage(e) {
    e.preventDefault();
    const body = JSON.stringify({
        name: document.getElementById('node-image-name').value.trim(),
        provider: document.getElementById('node-image-provider').value,
        version: document.getElementById('node-image-version').value.trim(),
        image_ref: document.getElementById('node-image-ref').value.trim(),
    });
    try {
        const res = await authFetch(`${API_BASE}/node-images`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body,
        });
        const data = await res.json();
        if (data.success) {
            showAlert(data.message || 'Node image added', 'success');
            closeModal('node-image-modal');
            loadNodeImages();
        } else {
            showAlert(data.error || 'Failed to add node image', 'error');
        }
    } catch (err) {
        showAlert('Failed to add node image', 'error');
    }
}

async function downloadNodeImage(id) {
    const btn = document.getElementById('node-image-dl-btn-' + id);
    if (btn) {
        btn.disabled = true;
        btn.innerHTML = '<span class="spinner" style="display: inline-block; width: 12px; height: 12px; border-width: 2px; margin: 0 6px -2px 0;"></span>Downloading...';
    }
    try {
        const res = await authFetch(`${API_BASE}/node-images/download?id=${encodeURIComponent(id)}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showAlert(data.message || 'Node image downloaded', 'success');
            loadNodeImages();
        } else {
            showAlert(data.error || 'Failed to download node image', 'error');
            if (btn) { btn.disabled = false; btn.textContent = 'Download'; }
        }
    } catch (err) {
        showAlert('Failed to download node image', 'error');
        if (btn) { btn.disabled = false; btn.textContent = 'Download'; }
    }
}

async function deleteNodeImage(id) {
    const img = nodeImages.find(i => i.id === id);
    const name = img ? img.name : id;
    if (!confirm(`Delete node image "${name}"?`)) return;
    try {
        const res = await authFetch(`${API_BASE}/node-images/delete?id=${encodeURIComponent(id)}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showAlert(data.message || 'Node image deleted', 'success');
            loadNodeImages();
        } else {
            showAlert(data.error || 'Failed to delete node image', 'error');
        }
    } catch (err) {
        showAlert('Failed to delete node image', 'error');
    }
}

async function loadClusters() {
    try {
        const res = await authFetch(`${API_BASE}/clusters`);
        const data = await res.json();
        if (data.success) {
            clusters = data.data || [];
            renderClustersTable();
            populateClusterNodesSelect();
        } else {
            showAlert(data.error || 'Failed to load clusters', 'error');
        }
    } catch (err) {
        document.getElementById('clusters-table').innerHTML = '<p class="alert alert-error">Failed to load clusters</p>';
    }
}

function clusterNodeImageLabel(c) {
    if (!c.node_image_name) return '<span style="color: var(--text-secondary);">-</span>';
    const version = c.node_image_version ? ' ' + c.node_image_version : '';
    return escapeHtml(c.node_image_name + version);
}

function clusterConfigBadges(c) {
    const badge = (ok, label, title) => ok ?
        `<span class="badge badge-success" title="${title} uploaded">${label} ✓</span>` :
        `<span class="badge badge-disabled" title="${title} not uploaded">${label}</span>`;
    return badge(c.has_control_plane_config, 'CP', 'Control plane config') + ' ' +
        badge(c.has_worker_config, 'Worker', 'Worker config');
}

function renderClustersTable() {
    const container = document.getElementById('clusters-table');
    if (!container) return;
    if (clusters.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No clusters yet. Add one to get started.</p>';
        return;
    }
    const rows = clusters.map(c => `
                    <tr>
                        <td><strong>${escapeHtml(c.name)}</strong>${c.description ? '<br><span style="color: var(--text-secondary); font-size: 12px;">' + escapeHtml(c.description) + '</span>' : ''}</td>
                        <td>${clusterNodeImageLabel(c)}</td>
                        <td>${clusterConfigBadges(c)}</td>
                        <td>${c.node_count || 0}</td>
                        <td>
                            <button class="btn btn-sm" onclick="showEditClusterModal(${c.id})">Edit</button>
                            <button class="btn btn-sm" onclick="showClusterConfigsModal(${c.id})">Configure</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteCluster(${c.id})">Delete</button>
                        </td>
                    </tr>`).join('');
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Node Image</th>
                    <th>Configs</th>
                    <th>Nodes</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>${rows}</tbody>
        </table>
        </div>
    `;
}

function populateNodeImageSelect(selectId, selectedId) {
    const select = document.getElementById(selectId);
    if (!select) return;
    let html = '<option value="">(none)</option>';
    html += nodeImages.map(img => {
        const label = img.name + (img.version ? ' (' + providerLabel(img.provider) + ' ' + img.version + ')' : ' (' + providerLabel(img.provider) + ')');
        const selected = selectedId != null && img.id === selectedId ? ' selected' : '';
        return `<option value="${img.id}"${selected}>${escapeHtml(label)}</option>`;
    }).join('');
    select.innerHTML = html;
}

function populateClusterNodesSelect() {
    const select = document.getElementById('cluster-nodes-select');
    if (!select) return;
    const current = select.value;
    select.innerHTML = '<option value="">(select a cluster)</option>' +
        clusters.map(c => `<option value="${c.id}">${escapeHtml(c.name)}</option>`).join('');
    if (current && clusters.some(c => String(c.id) === current)) {
        select.value = current;
    }
}

async function showAddClusterModal() {
    document.getElementById('cluster-form').reset();
    document.getElementById('cluster-id').value = '';
    document.getElementById('cluster-modal-title').textContent = 'Add Cluster';
    document.getElementById('cluster-save-btn').textContent = 'Create Cluster';
    if (nodeImages.length === 0) await loadNodeImages();
    populateNodeImageSelect('cluster-node-image', null);
    showModal('cluster-modal');
}

async function showEditClusterModal(id) {
    const cluster = clusters.find(c => c.id === id);
    if (!cluster) return;
    document.getElementById('cluster-form').reset();
    document.getElementById('cluster-id').value = cluster.id;
    document.getElementById('cluster-modal-title').textContent = 'Edit Cluster';
    document.getElementById('cluster-save-btn').textContent = 'Update Cluster';
    document.getElementById('cluster-name').value = cluster.name || '';
    document.getElementById('cluster-description').value = cluster.description || '';
    document.getElementById('cluster-kernel-params').value = cluster.kernel_params || '';
    document.getElementById('cluster-default-install-disk').value = cluster.default_install_disk || '';
    if (nodeImages.length === 0) await loadNodeImages();
    populateNodeImageSelect('cluster-node-image', cluster.node_image_id);
    showModal('cluster-modal');
}

async function saveCluster(e) {
    e.preventDefault();
    const id = document.getElementById('cluster-id').value;
    const nodeImageRaw = document.getElementById('cluster-node-image').value;
    const body = JSON.stringify({
        name: document.getElementById('cluster-name').value.trim(),
        description: document.getElementById('cluster-description').value.trim(),
        node_image_id: nodeImageRaw === '' ? null : parseInt(nodeImageRaw, 10),
        default_install_disk: document.getElementById('cluster-default-install-disk').value.trim(),
        kernel_params: document.getElementById('cluster-kernel-params').value.trim(),
    });
    const url = id ? `${API_BASE}/clusters/update?id=${encodeURIComponent(id)}` : `${API_BASE}/clusters`;
    try {
        const res = await authFetch(url, {
            method: id ? 'PUT' : 'POST',
            headers: { 'Content-Type': 'application/json' },
            body,
        });
        const data = await res.json();
        if (data.success) {
            showAlert(id ? 'Cluster updated' : 'Cluster created', 'success');
            closeModal('cluster-modal');
            loadClusters();
        } else {
            showAlert(data.error || 'Failed to save cluster', 'error');
        }
    } catch (err) {
        showAlert('Failed to save cluster', 'error');
    }
}

async function deleteCluster(id) {
    const cluster = clusters.find(c => c.id === id);
    const name = cluster ? cluster.name : id;
    if (!confirm(`Delete cluster "${name}"? Its nodes will be detached and become unmanaged.`)) return;
    try {
        const res = await authFetch(`${API_BASE}/clusters/delete?id=${encodeURIComponent(id)}`, { method: 'DELETE' });
        const data = await res.json();
        if (data.success) {
            showAlert(data.message || 'Cluster deleted', 'success');
            await loadClusters();
            loadClusterNodes();
            loadClients();
        } else {
            showAlert(data.error || 'Failed to delete cluster', 'error');
        }
    } catch (err) {
        showAlert('Failed to delete cluster', 'error');
    }
}

async function showClusterConfigsModal(id) {
    const cluster = clusters.find(c => c.id === id);
    if (!cluster) return;
    document.getElementById('cluster-configs-form').reset();
    document.getElementById('cluster-configs-id').value = id;
    document.getElementById('cluster-configs-name').textContent = cluster.name;
    document.getElementById('cluster-configs-provider').value = cluster.node_image_provider || '';
    const noImage = !cluster.node_image_provider;
    document.getElementById('cluster-configs-no-image').style.display = noImage ? '' : 'none';
    document.querySelectorAll('#cluster-configs-modal .configs-template-btn').forEach(b => { b.disabled = noImage; });
    setClusterConfigMode('edit');
    setClusterConfigStatus(cluster);
    showModal('cluster-configs-modal');
    try {
        const res = await authFetch(`${API_BASE}/clusters/config?id=${encodeURIComponent(id)}`);
        const data = await res.json();
        if (data.success && data.data) setClusterConfigStatus(data.data);
    } catch (err) {}
}

function setClusterConfigMode(mode) {
    clusterConfigMode = mode;
    document.getElementById('configs-mode-edit-btn').classList.toggle('active', mode === 'edit');
    document.getElementById('configs-mode-upload-btn').classList.toggle('active', mode === 'upload');
    document.querySelectorAll('#cluster-configs-modal .configs-edit-controls').forEach(el => {
        el.style.display = mode === 'edit' ? '' : 'none';
    });
    document.querySelectorAll('#cluster-configs-modal .configs-upload-controls').forEach(el => {
        el.style.display = mode === 'upload' ? '' : 'none';
    });
}

function setClusterConfigStatus(flags) {
    const status = (elId, ok) => {
        document.getElementById(elId).innerHTML = ok ?
            '<span class="badge badge-success">Saved ✓</span>' :
            '<span class="badge badge-disabled">Not set</span>';
    };
    status('cluster-configs-cp-status', flags.has_control_plane_config);
    status('cluster-configs-worker-status', flags.has_worker_config);
}

function readClusterConfigFile(input, textareaId) {
    const file = input.files && input.files[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
        document.getElementById(textareaId).value = reader.result;
    };
    reader.readAsText(file);
}

async function loadConfigTemplate(role, textareaId) {
    const provider = document.getElementById('cluster-configs-provider').value;
    if (!provider) {
        showAlert('Assign a node image to the cluster first', 'error');
        return;
    }
    try {
        const res = await authFetch(`${API_BASE}/clusters/config/template?provider=${encodeURIComponent(provider)}&role=${encodeURIComponent(role)}`);
        const data = await res.json();
        if (data.success && data.data) {
            document.getElementById(textareaId).value = data.data.template || '';
        } else {
            showAlert(data.error || 'Failed to load template', 'error');
        }
    } catch (err) {
        showAlert('Failed to load template', 'error');
    }
}

async function saveClusterConfigs(e) {
    e.preventDefault();
    const id = document.getElementById('cluster-configs-id').value;
    const body = {
        control_plane_config: document.getElementById('cluster-config-cp').value,
        worker_config: document.getElementById('cluster-config-worker').value,
        source: clusterConfigMode,
    };
    if (!body.control_plane_config && !body.worker_config) {
        showAlert('Provide at least one config document', 'error');
        return;
    }
    try {
        const res = await authFetch(`${API_BASE}/clusters/config?id=${encodeURIComponent(id)}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (data.success) {
            showAlert(data.message || 'Configs saved', 'success');
            closeModal('cluster-configs-modal');
            loadClusters();
        } else {
            showAlert(data.error || 'Failed to save configs', 'error');
        }
    } catch (err) {
        showAlert('Failed to save configs', 'error');
    }
}

async function loadClusterNodes() {
    const select = document.getElementById('cluster-nodes-select');
    const container = document.getElementById('cluster-nodes-table');
    if (!select || !container) return;
    const id = select.value;
    if (!id) {
        clusterNodes = [];
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">Select a cluster to view its nodes.</p>';
        return;
    }
    try {
        const res = await authFetch(`${API_BASE}/clusters/nodes?id=${encodeURIComponent(id)}`);
        const data = await res.json();
        if (data.success) {
            clusterNodes = data.data || [];
            renderClusterNodesTable();
        } else {
            showAlert(data.error || 'Failed to load cluster nodes', 'error');
        }
    } catch (err) {
        container.innerHTML = '<p class="alert alert-error">Failed to load cluster nodes</p>';
    }
}

function renderClusterNodesTable() {
    const container = document.getElementById('cluster-nodes-table');
    if (!container) return;
    if (clusterNodes.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No nodes in this cluster yet. Use Add Node to assign one.</p>';
        return;
    }
    const rows = clusterNodes.map(n => `
                    <tr>
                        <td>${n.name ? escapeHtml(n.name) + '<br>' : ''}<code>${n.mac_address}</code></td>
                        <td>${n.cluster_role === 'controlplane' ? 'Control Plane' : n.cluster_role === 'worker' ? 'Worker' : '-'}</td>
                        <td>${n.enrollment_state ? enrollmentStateBadge(n.enrollment_state) : '-'}</td>
                        <td>${n.install_disk ? '<code>' + escapeHtml(n.install_disk) + '</code>' : '<span style="color: var(--text-secondary);">Default</span>'}</td>
                        <td>${n.approved_at ? new Date(n.approved_at).toLocaleString() : '-'}</td>
                        <td>
                            <button class="btn btn-sm" onclick="reinstallNode('${n.mac_address}')">Reinstall</button>
                            ${n.enrollment_state === 'installing' ? '<button class="btn btn-primary btn-sm" onclick="markNodeInstalled(\'' + n.mac_address + '\')">Mark Installed</button>' : ''}
                            <button class="btn btn-danger btn-sm" onclick="unassignNode('${n.mac_address}')">Remove from cluster</button>
                        </td>
                    </tr>`).join('');
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Node</th>
                    <th>Role</th>
                    <th>State</th>
                    <th>Install Disk</th>
                    <th>Approved At</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>${rows}</tbody>
        </table>
        </div>
    `;
}

function showAddNodeModal() {
    const clusterId = document.getElementById('cluster-nodes-select').value;
    if (!clusterId) {
        showAlert('Select a cluster first', 'error');
        return;
    }
    const cluster = clusters.find(c => String(c.id) === clusterId);
    document.getElementById('add-node-form').reset();
    document.getElementById('add-node-cluster-name').textContent = cluster ? cluster.name : '';
    document.getElementById('add-node-install-disk').placeholder =
        cluster && cluster.default_install_disk ? cluster.default_install_disk : '/dev/sda';
    showModal('add-node-modal');
}

async function assignNode(e) {
    e.preventDefault();
    const clusterId = parseInt(document.getElementById('cluster-nodes-select').value, 10);
    if (!clusterId) {
        showAlert('Select a cluster first', 'error');
        return;
    }
    try {
        const res = await authFetch(`${API_BASE}/clusters/nodes/assign`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                mac: document.getElementById('add-node-mac').value.trim(),
                cluster_id: clusterId,
                role: document.querySelector('input[name="add-node-role"]:checked').value,
                install_disk: document.getElementById('add-node-install-disk').value.trim(),
                config_patch: document.getElementById('add-node-config-patch').value,
                reboot: document.getElementById('add-node-reboot').checked,
            }),
        });
        const data = await res.json();
        if (data.success) {
            showAlert(data.message || 'Node added to cluster', 'success');
            closeModal('add-node-modal');
            await loadClusters();
            loadClusterNodes();
            loadClients();
        } else {
            showAlert(data.error || 'Failed to add node', 'error');
        }
    } catch (err) {
        showAlert('Failed to add node', 'error');
    }
}

async function clusterNodeAction(path, mac, fallbackMessage) {
    try {
        const res = await authFetch(`${API_BASE}/clusters/nodes/${path}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mac }),
        });
        const data = await res.json();
        if (data.success) {
            showAlert(data.message || fallbackMessage, 'success');
            await loadClusters();
            loadClusterNodes();
            loadClients();
        } else {
            showAlert(data.error || 'Action failed', 'error');
        }
    } catch (err) {
        showAlert('Action failed', 'error');
    }
}

function reinstallNode(mac) {
    if (!confirm(`Reinstall ${mac}? The node will be wiped and reprovisioned on next PXE boot.`)) return;
    clusterNodeAction('reinstall', mac, 'Node will be reprovisioned on next PXE boot');
}

function unassignNode(mac) {
    if (!confirm(`Remove ${mac} from its cluster? The node will become unmanaged.`)) return;
    clusterNodeAction('unassign', mac, 'Node removed from cluster');
}

function markNodeInstalled(mac) {
    clusterNodeAction('mark-installed', mac, 'Node marked installed');
}

const AUDIT_PAGE_SIZE = 100;
let auditOffset = 0;
let auditEntries = [];

function auditFilterQuery() {
    const params = new URLSearchParams();
    const user = document.getElementById('audit-filter-user').value.trim();
    const path = document.getElementById('audit-filter-path').value.trim();
    const since = document.getElementById('audit-filter-since').value;
    if (user) params.set('username', user);
    if (path) params.set('path', path);
    if (since) params.set('since', new Date(since).toISOString());
    params.set('limit', AUDIT_PAGE_SIZE);
    params.set('offset', auditOffset);
    return params.toString();
}

async function loadAuditLog() {
    auditOffset = 0;
    auditEntries = [];
    await fetchAuditPage();
}

async function loadMoreAuditLog() {
    auditOffset = auditEntries.length;
    await fetchAuditPage();
}

async function fetchAuditPage() {
    const status = document.getElementById('audit-status');
    const more = document.getElementById('audit-load-more');
    try {
        const res = await authFetch(`${API_BASE}/audit?${auditFilterQuery()}`);
        const data = await res.json();
        if (!data.success) {
            document.getElementById('audit-table').innerHTML = `<p class="alert alert-error">${escapeHtml(data.error || 'Failed to load audit log')}</p>`;
            return;
        }
        auditEntries = auditEntries.concat(data.data.entries || []);
        renderAuditTable();
        status.textContent = `Showing ${auditEntries.length} of ${data.data.total}`;
        more.style.display = auditEntries.length < data.data.total ? '' : 'none';
    } catch (err) {
        document.getElementById('audit-table').innerHTML = '<p class="alert alert-error">Failed to load audit log</p>';
    }
}

function clearAuditFilters() {
    document.getElementById('audit-filter-user').value = '';
    document.getElementById('audit-filter-path').value = '';
    document.getElementById('audit-filter-since').value = '';
    loadAuditLog();
}

function auditStatusBadge(status) {
    const cls = status >= 500 ? 'badge-danger' : status >= 400 ? 'badge-warning' : 'badge-success';
    return `<span class="badge ${cls}">${status}</span>`;
}

function renderAuditTable() {
    const container = document.getElementById('audit-table');
    container.classList.remove('loading');
    if (auditEntries.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No audit entries match.</p>';
        return;
    }
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead>
                <tr>
                    <th>Time</th>
                    <th>User</th>
                    <th>Action</th>
                    <th>Method</th>
                    <th>Path</th>
                    <th>Status</th>
                    <th>IP</th>
                </tr>
            </thead>
            <tbody>
                ${auditEntries.map(e => `
                    <tr>
                        <td style="white-space: nowrap;">${new Date(e.created_at).toLocaleString()}</td>
                        <td>${escapeHtml(e.username)}</td>
                        <td>${escapeHtml(e.summary || '')}</td>
                        <td><span class="api-method ${e.method.toLowerCase()}">${e.method}</span></td>
                        <td><code>${escapeHtml(e.path)}${e.query ? '?' + escapeHtml(e.query) : ''}</code></td>
                        <td>${auditStatusBadge(e.status)}</td>
                        <td>${escapeHtml(e.remote_ip || '-')}</td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
        </div>`;
}

let menuPreviewData = null;

function populateMenuPreviewMACs() {
    const list = document.getElementById('menu-preview-mac-list');
    if (!list) return;
    list.innerHTML = (clients || []).map(c =>
        `<option value="${escapeHtml(c.mac_address)}">${escapeHtml(c.name || '')}</option>`
    ).join('');
}

async function previewBootMenu(mac) {
    const query = mac && mac.trim() ? `?mac=${encodeURIComponent(mac.trim())}` : '';
    try {
        const res = await authFetch(`${API_BASE}/clients/menu-preview${query}`);
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to render boot menu preview', 'error');
            return;
        }
        menuPreviewData = data.data;
        const parts = [`MAC: ${escapeHtml(menuPreviewData.mac)}`];
        parts.push(menuPreviewData.known_client
            ? `Client: ${escapeHtml(menuPreviewData.client_name || '(unnamed)')}`
            : 'Client: not registered (anonymous menu)');
        parts.push(menuPreviewData.kind === 'install' ? 'Kind: cluster install boot' : 'Kind: boot menu');
        if (menuPreviewData.next_boot_image) {
            parts.push(`Next boot: ${escapeHtml(menuPreviewData.next_boot_image)}`);
        }
        document.getElementById('menu-preview-summary').innerHTML = parts.join(' &nbsp;·&nbsp; ');
        document.getElementById('menu-preview-script').textContent = menuPreviewData.script || '';
        showModal('menu-preview-modal');
    } catch (err) {
        showAlert('Failed to render boot menu preview: ' + err.message, 'error');
    }
}

async function copyMenuPreview() {
    if (!menuPreviewData) return;
    try {
        await navigator.clipboard.writeText(menuPreviewData.script || '');
        showAlert('Boot menu script copied', 'success');
    } catch (err) {
        showAlert('Copy failed: ' + err.message, 'error');
    }
}

function downloadMenuPreview() {
    if (!menuPreviewData) return;
    const blob = new Blob([menuPreviewData.script || ''], { type: 'text/plain' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `menu-${(menuPreviewData.mac || 'unknown').replace(/:/g, '')}.ipxe`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(a.href);
}

let sanTargets = [];
let sanTargetsLoaded = false;

function nextBootLabel(value) {
    if (!value) return '';
    if (value.startsWith('san:')) {
        const t = sanTargets.find(x => String(x.id) === value.slice(4));
        return 'SAN: ' + (t ? t.name : value.slice(4));
    }
    const img = images.find(i => i.filename === value);
    return img ? img.name : value;
}

async function loadSANTargets(quiet) {
    try {
        const res = await authFetch(`${API_BASE}/san-targets`);
        const data = await res.json();
        if (!data.success) {
            if (!quiet) document.getElementById('san-targets-table').innerHTML = `<p class="alert alert-error">${escapeHtml(data.error || 'Failed to load SAN targets')}</p>`;
            return;
        }
        sanTargets = data.data || [];
        sanTargetsLoaded = true;
        if (!quiet) renderSANTargetsTable();
    } catch (err) {
        console.error('Failed to load SAN targets:', err);
    }
}

async function populateSANTargetSelect(selectId, selectedId, emptyLabel) {
    if (!sanTargetsLoaded) await loadSANTargets(true);
    const select = document.getElementById(selectId);
    if (!select) return;
    select.innerHTML = `<option value="">${escapeHtml(emptyLabel)}</option>` + sanTargets.map(t =>
        `<option value="${t.id}" ${selectedId === t.id ? 'selected' : ''}>${escapeHtml(t.name)} — ${escapeHtml(t.uri)}</option>`
    ).join('');
}

function renderSANTargetsTable() {
    const container = document.getElementById('san-targets-table');
    container.classList.remove('loading');
    if (sanTargets.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No SAN targets yet. Click "Add SAN Target" to define an iSCSI, AoE or HTTP disk.</p>';
        return;
    }
    const rows = sanTargets
        .filter(t => rowMatchesFilter('san-targets', [t.name, t.description, t.uri, t.initiator_iqn]))
        .map(t => `
        <tr>
            <td><strong>${escapeHtml(t.name)}</strong>${t.description ? '<br><span style="color: var(--text-secondary); font-size: 12px;">' + escapeHtml(t.description) + '</span>' : ''}</td>
            <td><code>${escapeHtml(t.uri)}</code></td>
            <td>${t.initiator_iqn ? '<code>' + escapeHtml(t.initiator_iqn) + '</code>' : '<span style="color: var(--text-secondary);">iPXE default</span>'}</td>
            <td>${t.chap_username ? '<span class="badge badge-info">CHAP</span>' : '<span style="color: var(--text-secondary);">None</span>'}${t.reverse_chap_username ? ' <span class="badge badge-info">Mutual</span>' : ''}</td>
            <td>${t.client_count || 0} client${t.client_count === 1 ? '' : 's'}, ${t.group_count || 0} group${t.group_count === 1 ? '' : 's'}</td>
            <td>
                <button type="button" class="btn btn-sm read-ok" onclick="previewSANTarget(${t.id})">Preview Menu</button>
                <button type="button" class="btn btn-sm admin-only" onclick="showSANTargetModal(${t.id})">Edit</button>
                <button type="button" class="btn btn-sm btn-danger admin-only" onclick="deleteSANTarget(${t.id})">Delete</button>
            </td>
        </tr>`).join('');
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead><tr><th>Name</th><th>URI</th><th>Initiator IQN</th><th>Auth</th><th>Used by</th><th>Actions</th></tr></thead>
            <tbody>${rows}</tbody>
        </table>
        </div>`;
}

function showSANTargetModal(id) {
    const form = document.getElementById('san-target-form');
    form.reset();
    const target = id ? sanTargets.find(t => t.id === id) : null;
    form.elements.id.value = target ? target.id : '';
    document.getElementById('san-target-modal-title').textContent = target ? 'Edit SAN Target' : 'Add SAN Target';
    if (target) {
        for (const key of ['name', 'description', 'uri', 'initiator_iqn', 'chap_username', 'chap_password', 'reverse_chap_username', 'reverse_chap_password']) {
            form.elements[key].value = target[key] || '';
        }
    }
    showModal('san-target-modal');
}

async function saveSANTarget(e) {
    e.preventDefault();
    const form = e.target;
    const fd = new FormData(form);
    const id = fd.get('id');
    const body = {};
    for (const key of ['name', 'description', 'uri', 'initiator_iqn', 'chap_username', 'chap_password', 'reverse_chap_username', 'reverse_chap_password']) {
        body[key] = fd.get(key) || '';
    }
    try {
        const res = await authFetch(`${API_BASE}/san-targets${id ? '?id=' + id : ''}`, {
            method: id ? 'PUT' : 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to save SAN target', 'error');
            return;
        }
        closeModal('san-target-modal');
        showAlert(data.message || 'SAN target saved', 'success');
        sanTargetsLoaded = false;
        await loadSANTargets();
    } catch (err) {
        showAlert('Failed to save SAN target: ' + err.message, 'error');
    }
}

async function deleteSANTarget(id) {
    const target = sanTargets.find(t => t.id === id);
    if (!target) return;
    const users = (target.client_count || 0) + (target.group_count || 0);
    if (!confirm(`Delete SAN target "${target.name}"?` + (users ? ` It will be removed from ${users} client(s) and group(s).` : ''))) return;
    try {
        const res = await authFetch(`${API_BASE}/san-targets?id=${id}`, { method: 'DELETE' });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to delete SAN target', 'error');
            return;
        }
        showAlert('SAN target deleted', 'success');
        sanTargetsLoaded = false;
        await loadSANTargets();
        if (clients.length) loadClients();
    } catch (err) {
        showAlert('Failed to delete SAN target: ' + err.message, 'error');
    }
}

async function previewSANTarget(id) {
    if (!clients || clients.length === 0) {
        try { await loadClients(); } catch (_) {}
    }
    const direct = clients.find(c => c.san_target_id === id);
    let mac = direct ? direct.mac_address : '';
    if (!mac) {
        const groupIds = clientGroups.filter(g => g.san_target_id === id).map(g => g.id);
        const viaGroup = clients.find(c => !c.san_target_id && groupIds.includes(c.client_group_id));
        if (viaGroup) mac = viaGroup.mac_address;
    }
    if (!mac) {
        showAlert('Assign this SAN target to a client or client group first, then preview that client\'s menu.', 'info');
        return;
    }
    previewBootMenu(mac);
}

document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('san-target-form');
    if (form) form.addEventListener('submit', saveSANTarget);
});
let lastLoadedAPITokens = [];

async function loadAPITokens() {
    const container = document.getElementById('api-tokens-table');
    if (!container) return;
    try {
        const res = await authFetch(`${API_BASE}/tokens`);
        const data = await res.json();
        if (!data.success) {
            container.innerHTML = `<p class="alert alert-error">${escapeHtml(data.error || 'Failed to load tokens')}</p>`;
            return;
        }
        lastLoadedAPITokens = data.data || [];
        renderAPITokensTable();
    } catch (err) {
        container.innerHTML = '<p class="alert alert-error">Failed to load tokens</p>';
    }
}

function renderAPITokensTable() {
    const container = document.getElementById('api-tokens-table');
    if (!container) return;
    if (lastLoadedAPITokens.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary);">No API tokens yet.</p>';
        return;
    }
    const now = Date.now();
    const rows = lastLoadedAPITokens.map(tok => {
        const expired = tok.expires_at && new Date(tok.expires_at).getTime() <= now;
        let status = '<span class="badge badge-success">Active</span>';
        if (tok.revoked) status = '<span class="badge badge-danger">Revoked</span>';
        else if (expired) status = '<span class="badge badge-warning">Expired</span>';
        const scope = tok.scope === 'admin' ? '<span class="badge badge-info">Admin</span>' : '<span class="badge badge-success">Read-only</span>';
        return `<tr>
            <td><strong>${escapeHtml(tok.name)}</strong></td>
            <td><code>${escapeHtml(tok.prefix)}…</code></td>
            <td>${scope}</td>
            <td>${escapeHtml(tok.created_by || '-')}</td>
            <td>${new Date(tok.created_at).toLocaleString()}</td>
            <td>${tok.last_used_at ? new Date(tok.last_used_at).toLocaleString() : 'Never'}</td>
            <td>${tok.expires_at ? new Date(tok.expires_at).toLocaleDateString() : 'Never'}</td>
            <td>${status}</td>
            <td>${tok.revoked ? '' : `<button class="btn btn-danger btn-sm" onclick="revokeAPIToken(${tok.id}, '${escapeHtml(tok.name).replace(/'/g, "\\'")}')">Revoke</button>`}</td>
        </tr>`;
    }).join('');
    container.innerHTML = `<div class="table-scroll"><table>
        <thead><tr><th>Name</th><th>Prefix</th><th>Scope</th><th>Created by</th><th>Created</th><th>Last used</th><th>Expires</th><th>Status</th><th>Actions</th></tr></thead>
        <tbody>${rows}</tbody></table></div>`;
}

async function createAPIToken(e) {
    if (e) e.preventDefault();
    const body = {
        name: document.getElementById('api-token-name').value.trim(),
        scope: document.getElementById('api-token-scope').value,
        expires_in_days: parseInt(document.getElementById('api-token-expires').value, 10) || 0,
    };
    try {
        const res = await authFetch(`${API_BASE}/tokens`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to create token', 'error');
            return;
        }
        document.getElementById('api-token-secret-value').textContent = data.data.secret;
        document.getElementById('api-token-secret').style.display = '';
        document.getElementById('api-token-name').value = '';
        showAlert(data.message || 'Token created', 'success');
        loadAPITokens();
    } catch (err) {
        showAlert('Failed to create token: ' + err.message, 'error');
    }
}

async function copyAPITokenSecret() {
    const secret = document.getElementById('api-token-secret-value').textContent;
    try {
        await navigator.clipboard.writeText(secret);
        showAlert('Token copied', 'success');
    } catch (err) {
        showAlert('Copy failed: ' + err.message, 'error');
    }
}

async function revokeAPIToken(id, name) {
    if (!confirm(`Revoke the token "${name}"? Anything using it will stop working immediately.`)) return;
    try {
        const res = await authFetch(`${API_BASE}/tokens?id=${id}`, { method: 'DELETE' });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to revoke token', 'error');
            return;
        }
        showAlert('Token revoked', 'success');
        loadAPITokens();
    } catch (err) {
        showAlert('Failed to revoke token: ' + err.message, 'error');
    }
}

async function loadTOTPStatus() {
    const card = document.getElementById('totp-card');
    const status = document.getElementById('totp-status');
    if (!card || !status) return;
    try {
        const res = await authFetch(`${API_BASE}/me`);
        const data = await res.json();
        if (!data.success) return;
        const me = data.data;
        if (me.principal === 'token' || (me.source && me.source !== 'local')) {
            card.style.display = 'none';
            return;
        }
        card.style.display = '';
        if (me.totp_enabled) {
            status.innerHTML = `<span class="badge badge-success">Enabled</span> Two-factor authentication is on for <strong>${escapeHtml(me.username)}</strong>.
                <button type="button" class="btn btn-sm" style="margin-left: 10px;" onclick="disableTOTP()">Disable</button>`;
        } else {
            status.innerHTML = `<span class="badge badge-warning">Off</span> Two-factor authentication is not enabled for <strong>${escapeHtml(me.username)}</strong>.
                <button type="button" class="btn btn-sm btn-primary" style="margin-left: 10px;" onclick="startTOTPSetup()">Set up</button>`;
        }
    } catch (err) {
        status.textContent = 'Could not load two-factor status';
    }
}

async function startTOTPSetup() {
    try {
        const res = await authFetch(`${API_BASE}/me/totp/setup`, { method: 'POST' });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Could not start two-factor setup', 'error');
            return;
        }
        document.getElementById('totp-qr').src = data.data.qr_code;
        document.getElementById('totp-secret').textContent = data.data.secret;
        document.getElementById('totp-confirm-code').value = '';
        document.getElementById('totp-setup').style.display = '';
        document.getElementById('totp-recovery').style.display = 'none';
        document.getElementById('totp-confirm-code').focus();
    } catch (err) {
        showAlert('Could not start two-factor setup: ' + err.message, 'error');
    }
}

function cancelTOTPSetup() {
    document.getElementById('totp-setup').style.display = 'none';
}

async function enableTOTP() {
    const code = document.getElementById('totp-confirm-code').value.trim();
    if (!code) {
        showAlert('Enter the code shown in your authenticator app', 'error');
        return;
    }
    try {
        const res = await authFetch(`${API_BASE}/me/totp/enable`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ code }),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Could not enable two-factor authentication', 'error');
            return;
        }
        document.getElementById('totp-setup').style.display = 'none';
        document.getElementById('totp-recovery-codes').textContent = (data.data.recovery_codes || []).join('\n');
        document.getElementById('totp-recovery').style.display = '';
        showAlert('Two-factor authentication enabled', 'success');
        loadTOTPStatus();
        if (!isReadOnly()) loadUsers();
    } catch (err) {
        showAlert('Could not enable two-factor authentication: ' + err.message, 'error');
    }
}

async function copyTOTPRecoveryCodes() {
    try {
        await navigator.clipboard.writeText(document.getElementById('totp-recovery-codes').textContent);
        showAlert('Recovery codes copied', 'success');
    } catch (err) {
        showAlert('Copy failed: ' + err.message, 'error');
    }
}

async function disableTOTP() {
    const password = prompt('Enter your password to turn off two-factor authentication:');
    if (password === null) return;
    try {
        const res = await authFetch(`${API_BASE}/me/totp/disable`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password }),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Could not disable two-factor authentication', 'error');
            return;
        }
        showAlert('Two-factor authentication disabled', 'success');
        loadTOTPStatus();
        if (!isReadOnly()) loadUsers();
    } catch (err) {
        showAlert('Could not disable two-factor authentication: ' + err.message, 'error');
    }
}

async function resetUserTOTP(id, username) {
    if (!confirm(`Remove two-factor authentication from "${username}"? They will be able to sign in with just their password until they set it up again.`)) return;
    try {
        const res = await authFetch(`${API_BASE}/users/totp/reset?id=${id}`, { method: 'POST' });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Could not reset two-factor authentication', 'error');
            return;
        }
        showAlert(data.message || 'Two-factor authentication removed', 'success');
        loadUsers();
        loadTOTPStatus();
    } catch (err) {
        showAlert('Could not reset two-factor authentication: ' + err.message, 'error');
    }
}

let diskImages = [];
let diskJobs = [];
let diskImagingStatus = null;

async function loadDiskImages() {
    try {
        const [imgRes, jobRes] = await Promise.all([
            authFetch(`${API_BASE}/disk-images`),
            authFetch(`${API_BASE}/disk-images/jobs?limit=100`),
        ]);
        const imgData = await imgRes.json();
        const jobData = await jobRes.json();
        if (!imgData.success) {
            document.getElementById('disk-images-table').innerHTML = `<p class="alert alert-error">${escapeHtml(imgData.error || 'Failed to load disk images')}</p>`;
            return;
        }
        diskImagingStatus = imgData.data.status || null;
        diskImages = imgData.data.images || [];
        diskJobs = jobData.success ? (jobData.data || []) : [];
        renderDiskImagingProblems();
        renderDiskImages();
        renderDiskJobs();
    } catch (err) {
        document.getElementById('disk-images-table').innerHTML = '<p class="alert alert-error">Failed to load disk images</p>';
    }
}

function renderDiskImagingProblems() {
    const banner = document.getElementById('disk-images-problems');
    if (!banner) return;
    const problems = (diskImagingStatus && diskImagingStatus.problems) || [];
    if (problems.length === 0) {
        banner.style.display = 'none';
        return;
    }
    banner.innerHTML = '<strong>Disk imaging is not ready.</strong> ' + problems.map(escapeHtml).join(' ') +
        ' Enable the Clonezilla tool under <strong>Tools</strong> and start Bootimus with <code>--nfs-enabled</code>.';
    banner.style.display = 'block';
}

function diskImageStateBadge(state) {
    const cls = state === 'ready' ? 'badge-success' : (state === 'capturing' ? 'badge-info' : 'badge-danger');
    return `<span class="badge ${cls}">${escapeHtml(state)}</span>`;
}

function diskJobStateBadge(state) {
    const cls = { completed: 'badge-success', running: 'badge-info', pending: 'badge-warning', failed: 'badge-danger', cancelled: 'badge-info' }[state] || 'badge-info';
    return `<span class="badge ${cls}">${escapeHtml(state)}</span>`;
}

function renderDiskImages() {
    const container = document.getElementById('disk-images-table');
    if (!container) return;
    const filterInput = document.getElementById('disk-images-filter');
    const q = filterInput ? filterInput.value.trim().toLowerCase() : '';
    const rows = diskImages.filter(img => !q || [img.name, img.description, img.source_mac, img.source_client_name, img.state].some(v => (v || '').toLowerCase().includes(q)));
    if (rows.length === 0) {
        container.innerHTML = `<p style="color: var(--text-secondary); padding: 20px;">${diskImages.length === 0 ? 'No disk images yet. Use Capture Disk to create one from a client.' : 'No images match the filter.'}</p>`;
        return;
    }
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead><tr>
                <th>Name</th><th>Description</th><th>Source</th><th>Disk</th><th>Size</th><th>State</th><th>Created</th><th>Actions</th>
            </tr></thead>
            <tbody>
                ${rows.map(img => `
                    <tr>
                        <td><strong>${escapeHtml(img.name)}</strong></td>
                        <td>${escapeHtml(img.description || '-')}</td>
                        <td>${img.source_mac ? `<code>${escapeHtml(img.source_mac)}</code>${img.source_client_name ? ' ' + escapeHtml(img.source_client_name) : ''}` : '-'}</td>
                        <td>${escapeHtml(img.disk || '-')}</td>
                        <td>${img.size_bytes ? formatBytes(img.size_bytes) : '-'}</td>
                        <td>${diskImageStateBadge(img.state)}</td>
                        <td style="white-space: nowrap;">${new Date(img.created_at).toLocaleString()}</td>
                        <td style="white-space: nowrap;">
                            ${img.state === 'ready' ? `<button class="btn btn-sm btn-primary" onclick="showDeployDiskModal({ imageId: ${img.id} })">Deploy</button>` : ''}
                            <button class="btn btn-sm admin-only" onclick="editDiskImageDescription(${img.id})">Edit</button>
                            <button class="btn btn-sm btn-danger admin-only" onclick="deleteDiskImage(${img.id}, '${jsAttrEscape(img.name)}')">Delete</button>
                        </td>
                    </tr>`).join('')}
            </tbody>
        </table>
        </div>`;
}

function diskJobDuration(job) {
    if (!job.started_at) return '-';
    const end = job.finished_at ? new Date(job.finished_at) : new Date();
    const secs = Math.max(0, Math.round((end - new Date(job.started_at)) / 1000));
    if (secs < 60) return `${secs}s`;
    return `${Math.floor(secs / 60)}m ${secs % 60}s`;
}

function renderDiskJobs() {
    const container = document.getElementById('disk-jobs-table');
    if (!container) return;
    if (diskJobs.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No capture or deploy jobs yet.</p>';
        return;
    }
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead><tr>
                <th>Queued</th><th>Client</th><th>Job</th><th>Image</th><th>Disk</th><th>State</th><th>Duration</th><th>Message</th><th></th>
            </tr></thead>
            <tbody>
                ${diskJobs.map(job => `
                    <tr>
                        <td style="white-space: nowrap;">${new Date(job.created_at).toLocaleString()}</td>
                        <td><code>${escapeHtml(job.client_mac)}</code>${job.client_name ? ' ' + escapeHtml(job.client_name) : ''}</td>
                        <td>${escapeHtml(job.kind)}</td>
                        <td>${job.disk_image ? escapeHtml(job.disk_image.name) : (job.disk_image_id ? '#' + job.disk_image_id : '<span style="color: var(--text-secondary);">Deleted image</span>')}</td>
                        <td>${escapeHtml(job.disk || '-')}</td>
                        <td>${diskJobStateBadge(job.state)}</td>
                        <td>${diskJobDuration(job)}</td>
                        <td style="font-size: 12px; color: var(--text-secondary);">${escapeHtml(job.message || '')}</td>
                        <td>${(job.state === 'pending' || job.state === 'running') ? `<button class="btn btn-sm btn-danger admin-only" onclick="cancelDiskJob(${job.id})">Cancel</button>` : ''}</td>
                    </tr>`).join('')}
            </tbody>
        </table>
        </div>`;
}

function clientOptionLabel(c) {
    return `${c.mac_address}${c.name ? ' (' + c.name + ')' : ''}`;
}

async function ensureClientsLoaded() {
    if (!clients || clients.length === 0) {
        try { await loadClients(); } catch (_) {}
    }
}

async function showCaptureDiskModal(mac) {
    await ensureClientsLoaded();
    const select = document.getElementById('capture-disk-mac');
    select.innerHTML = (clients || []).map(c => `<option value="${escapeHtml(c.mac_address)}"${c.mac_address === mac ? ' selected' : ''}>${escapeHtml(clientOptionLabel(c))}</option>`).join('');
    document.getElementById('capture-disk-name').value = '';
    document.getElementById('capture-disk-description').value = '';
    document.getElementById('capture-disk-device').value = 'sda';
    document.getElementById('capture-disk-wake').checked = false;
    showModal('capture-disk-modal');
}

async function submitCaptureDisk(e) {
    if (e) e.preventDefault();
    const body = {
        mac_address: document.getElementById('capture-disk-mac').value,
        name: document.getElementById('capture-disk-name').value.trim(),
        description: document.getElementById('capture-disk-description').value.trim(),
        disk: document.getElementById('capture-disk-device').value.trim(),
        wake: document.getElementById('capture-disk-wake').checked,
    };
    try {
        const res = await authFetch(`${API_BASE}/disk-images/capture`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to queue capture', 'error');
            return;
        }
        showAlert(data.message || 'Capture queued', 'success');
        closeModal('capture-disk-modal');
        loadDiskImages();
    } catch (err) {
        showAlert('Failed to queue capture: ' + err.message, 'error');
    }
}

async function showDeployDiskModal(opts) {
    opts = opts || {};
    await ensureClientsLoaded();
    if (diskImages.length === 0) {
        try { await loadDiskImages(); } catch (_) {}
    }
    const ready = diskImages.filter(img => img.state === 'ready');
    if (ready.length === 0) {
        showAlert('No disk images are ready to deploy. Capture one first.', 'error');
        return;
    }
    const imageSelect = document.getElementById('deploy-disk-image');
    imageSelect.innerHTML = ready.map(img => `<option value="${img.id}"${img.id === opts.imageId ? ' selected' : ''}>${escapeHtml(img.name)}${img.size_bytes ? ' (' + formatBytes(img.size_bytes) + ')' : ''}</option>`).join('');
    const targetSelect = document.getElementById('deploy-disk-target');
    const groups = (typeof clientGroups !== 'undefined' && clientGroups) ? clientGroups : [];
    let html = '<optgroup label="Clients">' + (clients || []).map(c => `<option value="mac:${escapeHtml(c.mac_address)}"${c.mac_address === opts.mac ? ' selected' : ''}>${escapeHtml(clientOptionLabel(c))}</option>`).join('') + '</optgroup>';
    if (groups.length) {
        html += '<optgroup label="Client groups (all enabled members)">' + groups.map(g => `<option value="group:${g.id}"${g.id === opts.groupId ? ' selected' : ''}>${escapeHtml(g.name)} (${g.member_count} members)</option>`).join('') + '</optgroup>';
    }
    targetSelect.innerHTML = html;
    document.getElementById('deploy-disk-device').value = 'sda';
    document.getElementById('deploy-disk-wake').checked = false;
    showModal('deploy-disk-modal');
}

async function submitDeployDisk(e) {
    if (e) e.preventDefault();
    const target = document.getElementById('deploy-disk-target').value;
    const body = {
        image_id: parseInt(document.getElementById('deploy-disk-image').value, 10),
        disk: document.getElementById('deploy-disk-device').value.trim(),
        wake: document.getElementById('deploy-disk-wake').checked,
    };
    if (target.startsWith('group:')) {
        body.client_group_id = parseInt(target.slice(6), 10);
    } else {
        body.mac_address = target.slice(4);
    }
    const imageName = document.getElementById('deploy-disk-image').selectedOptions[0].textContent;
    if (!confirm(`Deploy ${imageName} and overwrite disk ${body.disk || 'sda'} on the selected target on its next PXE boot?`)) return;
    try {
        const res = await authFetch(`${API_BASE}/disk-images/deploy`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to queue deploy', 'error');
            return;
        }
        showAlert(data.message + (data.error ? ' (' + data.error + ')' : ''), data.error ? 'warning' : 'success');
        closeModal('deploy-disk-modal');
        loadDiskImages();
    } catch (err) {
        showAlert('Failed to queue deploy: ' + err.message, 'error');
    }
}

function bulkDeployDiskImageClientGroup() {
    const id = parseInt(document.getElementById('edit-client-group-form').elements.id.value, 10);
    if (!id) return;
    showDeployDiskModal({ groupId: id });
}

async function cancelDiskJob(id) {
    if (!confirm('Cancel this job? A capture in progress will be discarded.')) return;
    try {
        const res = await authFetch(`${API_BASE}/disk-images/jobs?id=${id}`, { method: 'DELETE' });
        const data = await res.json();
        showAlert(data.message || data.error || 'Done', data.success ? 'success' : 'error');
        loadDiskImages();
    } catch (err) {
        showAlert('Failed to cancel job', 'error');
    }
}

async function deleteDiskImage(id, name) {
    if (!confirm(`Delete disk image ${name} and all of its files? This cannot be undone.`)) return;
    try {
        const res = await authFetch(`${API_BASE}/disk-images?id=${id}`, { method: 'DELETE' });
        const data = await res.json();
        showAlert(data.message || data.error || 'Done', data.success ? 'success' : 'error');
        loadDiskImages();
    } catch (err) {
        showAlert('Failed to delete disk image', 'error');
    }
}

async function editDiskImageDescription(id) {
    const img = diskImages.find(i => i.id === id);
    if (!img) return;
    const description = prompt(`Description for ${img.name}`, img.description || '');
    if (description === null) return;
    try {
        const res = await authFetch(`${API_BASE}/disk-images?id=${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ description }),
        });
        const data = await res.json();
        showAlert(data.message || data.error || 'Done', data.success ? 'success' : 'error');
        loadDiskImages();
    } catch (err) {
        showAlert('Failed to update disk image', 'error');
    }
}

const REPLICA_KIND_LABELS = {
    san_targets: 'SAN targets',
    image_groups: 'Image groups',
    images: 'Images',
    client_groups: 'Client groups',
    clients: 'Clients',
    theme: 'Boot menu theme',
    profiles: 'Custom distro profiles',
    tools: 'Boot tools',
    autoinstall_files: 'Auto-install files',
    custom_files: 'Custom files',
    bootloader_set: 'Active bootloader set',
};

async function loadReplicaStatus() {
    const card = document.getElementById('replica-card');
    if (!card) return;
    try {
        const res = await authFetch(`${API_BASE}/replica/status`);
        const data = await res.json();
        if (!data.success || !data.data.enabled) {
            card.style.display = 'none';
            return;
        }
        card.style.display = '';
        renderReplicaStatus(data.data.status);
    } catch (err) {
        card.style.display = 'none';
    }
}

function renderReplicaStatus(status) {
    const line = document.getElementById('replica-status-line');
    const kinds = document.getElementById('replica-kinds');
    const syncBtn = document.getElementById('replica-sync-btn');
    if (syncBtn) syncBtn.style.display = localStorage.getItem('bootimus_is_admin') === 'true' ? '' : 'none';
    const lastRun = status.last_run ? new Date(status.last_run).toLocaleString() : 'never';
    const lastOk = status.last_success ? new Date(status.last_success).toLocaleString() : 'never';
    let text = `Primary ${status.primary_url} · every ${status.interval_seconds}s · ISOs ${status.sync_isos ? 'copied locally' : 'served from the primary'} · last run ${lastRun} · last success ${lastOk}`;
    if (status.running) text += ' · sync in progress';
    line.textContent = text;
    if (status.last_error) {
        line.innerHTML += ` <span style="color: var(--danger);">${escapeHtml(status.last_error)}</span>`;
    }
    const rows = (status.kinds || []).map(k => {
        let state;
        if (k.error) state = `<span class="badge badge-danger" title="${escapeHtml(k.error)}">Failed</span>`;
        else if (k.skipped) state = `<span class="badge badge-info">Skipped</span>`;
        else state = `<span class="badge badge-success">OK</span>`;
        return `<tr>
            <td>${escapeHtml(REPLICA_KIND_LABELS[k.name] || k.name)}</td>
            <td>${state}</td>
            <td>${k.skipped ? '-' : k.count}</td>
            <td>${k.skipped ? '-' : k.changed}</td>
            <td>${k.skipped ? '-' : k.removed}</td>
            <td style="color: var(--text-secondary); font-size: 12px;">${escapeHtml(k.error || k.note || '')}</td>
        </tr>`;
    }).join('');
    kinds.innerHTML = rows
        ? `<div class="table-scroll"><table><thead><tr><th>Kind</th><th>State</th><th>On primary</th><th>Changed</th><th>Removed</th><th>Notes</th></tr></thead><tbody>${rows}</tbody></table></div>`
        : '<p style="color: var(--text-secondary);">No sync has run yet.</p>';
}

async function runReplicaSync() {
    const btn = document.getElementById('replica-sync-btn');
    const line = document.getElementById('replica-status-line');
    if (btn) btn.disabled = true;
    if (line) line.textContent = 'Syncing from the primary…';
    try {
        const res = await authFetch(`${API_BASE}/replica/sync`, { method: 'POST' });
        const data = await res.json();
        if (data.data) renderReplicaStatus(data.data);
        showAlert(data.success ? 'Sync finished' : (data.error || 'Sync failed'), data.success ? 'success' : 'error');
        refreshRole();
    } catch (err) {
        showAlert('Sync failed: ' + err.message, 'error');
    } finally {
        if (btn) btn.disabled = false;
    }
}

let deploymentProfiles = [];
let deploymentProfilesLoaded = false;

function deploymentProfileName(id) {
    const p = deploymentProfiles.find(x => x.id === id);
    return p ? p.name : ('#' + id);
}

function deploymentProfileBadge(client) {
    let html = '';
    if (client.profile_id) html += ` <span class="badge badge-info" title="Deployment profile">${escapeHtml(deploymentProfileName(client.profile_id))}</span>`;
    if (client.enrolled_by_rule_id) {
        const rule = enrolmentRules.find(r => r.id === client.enrolled_by_rule_id);
        html += ` <span class="badge badge-warning" title="Enrolled by rule${rule ? ': ' + escapeHtml(rule.name) : ''}">auto-enrolled</span>`;
    }
    return html;
}

async function loadDeploymentProfiles(quiet) {
    try {
        const res = await authFetch(`${API_BASE}/deployment-profiles`);
        const data = await res.json();
        if (!data.success) {
            if (!quiet) document.getElementById('deployment-profiles-table').innerHTML = `<p class="alert alert-error">${escapeHtml(data.error || 'Failed to load deployment profiles')}</p>`;
            return;
        }
        deploymentProfiles = data.data || [];
        deploymentProfilesLoaded = true;
        if (!quiet) renderDeploymentProfilesTable();
    } catch (err) {
        console.error('Failed to load deployment profiles:', err);
    }
}

async function populateDeploymentProfileSelect(selectId, selectedId, emptyLabel) {
    if (!deploymentProfilesLoaded) await loadDeploymentProfiles(true);
    const select = document.getElementById(selectId);
    if (!select) return;
    select.innerHTML = `<option value="">${escapeHtml(emptyLabel)}</option>` + deploymentProfiles.map(p =>
        `<option value="${p.id}" ${selectedId === p.id ? 'selected' : ''}>${escapeHtml(p.name)}${p.description ? ' — ' + escapeHtml(p.description) : ''}</option>`
    ).join('');
}

function firstBootActionLabel(action) {
    if (action === 'install') return 'Install image';
    if (action === 'deploy') return 'Deploy disk image';
    return 'None';
}

function renderDeploymentProfilesTable() {
    const container = document.getElementById('deployment-profiles-table');
    container.classList.remove('loading');
    if (deploymentProfiles.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No deployment profiles yet. Click "Add Deployment Profile" to bundle an image, auto-install file and boot settings.</p>';
        return;
    }
    const imageName = f => { const img = (images || []).find(i => i.filename === f); return img ? img.name : f; };
    const sanName = id => { const t = sanTargets.find(x => x.id === id); return t ? t.name : ('#' + id); };
    const rows = deploymentProfiles
        .filter(p => rowMatchesFilter('deployment-profiles', [p.name, p.description, p.image_filename, p.auto_install_file, p.bootloader_set]))
        .map(p => `
        <tr>
            <td><strong>${escapeHtml(p.name)}</strong>${p.description ? '<br><span style="color: var(--text-secondary); font-size: 12px;">' + escapeHtml(p.description) + '</span>' : ''}</td>
            <td>${p.image_filename ? escapeHtml(imageName(p.image_filename)) : '<span style="color: var(--text-secondary);">—</span>'}</td>
            <td>${p.auto_install_file ? '<code>' + escapeHtml(p.auto_install_file) + '</code>' : '<span style="color: var(--text-secondary);">—</span>'}</td>
            <td>${p.san_target_id ? escapeHtml(sanName(p.san_target_id)) + (p.boot_from_san ? ' <span class="badge badge-info">default</span>' : '') : '<span style="color: var(--text-secondary);">—</span>'}</td>
            <td>${p.bootloader_set ? escapeHtml(p.bootloader_set) : '<span style="color: var(--text-secondary);">Default</span>'}</td>
            <td>${escapeHtml(firstBootActionLabel(p.first_boot_action))}</td>
            <td>${p.client_count || 0} client${p.client_count === 1 ? '' : 's'}, ${p.group_count || 0} group${p.group_count === 1 ? '' : 's'}</td>
            <td>
                <button type="button" class="btn btn-sm admin-only" onclick="showDeploymentProfileModal(${p.id})">Edit</button>
                <button type="button" class="btn btn-sm btn-danger admin-only" onclick="deleteDeploymentProfile(${p.id})">Delete</button>
            </td>
        </tr>`).join('');
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead><tr><th>Name</th><th>Image</th><th>Auto-install</th><th>SAN</th><th>Bootloader set</th><th>First boot</th><th>Used by</th><th>Actions</th></tr></thead>
            <tbody>${rows}</tbody>
        </table>
        </div>`;
}

async function populateDeploymentProfileModalSelects(profile) {
    if (!images || images.length === 0) { try { await loadImages(); } catch (_) {} }
    const imageSel = document.getElementById('dp-image-select');
    imageSel.innerHTML = '<option value="">(none)</option>' + (images || []).map(img =>
        `<option value="${escapeHtml(img.filename)}" ${profile && profile.image_filename === img.filename ? 'selected' : ''}>${escapeHtml(img.name)}${img.enabled ? '' : ' (disabled)'}</option>`
    ).join('');

    await populateAutoInstallFileDropdown('dp-autoinstall-select', profile ? profile.auto_install_file : '');
    const aiFirst = document.getElementById('dp-autoinstall-select').options[0];
    if (aiFirst) aiFirst.textContent = '(none)';

    await populateSANTargetSelect('dp-san-select', profile ? profile.san_target_id : null, '(none)');

    const blSel = document.getElementById('dp-bootloader-select');
    blSel.innerHTML = '<option value="">Default (global or group setting)</option>';
    try {
        const blRes = await authFetch(`${API_BASE}/bootloaders`);
        const blData = await blRes.json();
        if (blData.success && blData.data && blData.data.sets) {
            for (const set of blData.data.sets) {
                blSel.innerHTML += `<option value="${escapeHtml(set.name)}" ${profile && profile.bootloader_set === set.name ? 'selected' : ''}>${escapeHtml(set.name)}</option>`;
            }
        }
    } catch (_) {}

    const diSel = document.getElementById('dp-disk-image-select');
    diSel.innerHTML = '<option value="">(none)</option>';
    try {
        const res = await authFetch(`${API_BASE}/disk-images`);
        const data = await res.json();
        if (data.success) {
            for (const di of (data.data.images || [])) {
                diSel.innerHTML += `<option value="${di.id}" ${profile && profile.disk_image_id === di.id ? 'selected' : ''}>${escapeHtml(di.name)}${di.state !== 'ready' ? ' (' + escapeHtml(di.state) + ')' : ''}</option>`;
            }
        }
    } catch (_) {}
}

async function showDeploymentProfileModal(id) {
    const form = document.getElementById('deployment-profile-form');
    form.reset();
    const profile = id ? deploymentProfiles.find(p => p.id === id) : null;
    form.elements.id.value = profile ? profile.id : '';
    document.getElementById('deployment-profile-modal-title').textContent = profile ? 'Edit Deployment Profile' : 'Add Deployment Profile';
    if (profile) {
        form.elements.name.value = profile.name || '';
        form.elements.description.value = profile.description || '';
        form.elements.boot_from_san.checked = !!profile.boot_from_san;
        const action = form.querySelector(`[name="first_boot_action"][value="${profile.first_boot_action || ''}"]`);
        if (action) action.checked = true;
        form.elements.show_public_images.value = profile.show_public_images === undefined || profile.show_public_images === null ? '' : String(profile.show_public_images);
    }
    await populateDeploymentProfileModalSelects(profile);
    showModal('deployment-profile-modal');
}

async function saveDeploymentProfile(e) {
    e.preventDefault();
    const form = e.target;
    const fd = new FormData(form);
    const id = fd.get('id');
    const showPublic = fd.get('show_public_images');
    const body = {
        name: fd.get('name') || '',
        description: fd.get('description') || '',
        image_filename: fd.get('image_filename') || '',
        auto_install_file: fd.get('auto_install_file') || '',
        san_target_id: fd.get('san_target_id') ? parseInt(fd.get('san_target_id'), 10) : null,
        boot_from_san: fd.get('boot_from_san') === 'on',
        bootloader_set: fd.get('bootloader_set') || '',
        disk_image_id: fd.get('disk_image_id') ? parseInt(fd.get('disk_image_id'), 10) : null,
        first_boot_action: fd.get('first_boot_action') || '',
        show_public_images: showPublic === '' ? null : showPublic === 'true',
    };
    try {
        const res = await authFetch(`${API_BASE}/deployment-profiles${id ? '?id=' + id : ''}`, {
            method: id ? 'PUT' : 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to save deployment profile', 'error');
            return;
        }
        closeModal('deployment-profile-modal');
        showAlert(data.message || 'Deployment profile saved', 'success');
        deploymentProfilesLoaded = false;
        await loadDeploymentProfiles();
    } catch (err) {
        showAlert('Failed to save deployment profile: ' + err.message, 'error');
    }
}

async function deleteDeploymentProfile(id) {
    const profile = deploymentProfiles.find(p => p.id === id);
    if (!profile) return;
    const users = (profile.client_count || 0) + (profile.group_count || 0);
    if (!confirm(`Delete deployment profile "${profile.name}"?` + (users ? ` It will be removed from ${users} client(s) and group(s).` : ''))) return;
    try {
        const res = await authFetch(`${API_BASE}/deployment-profiles?id=${id}`, { method: 'DELETE' });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to delete deployment profile', 'error');
            return;
        }
        showAlert('Deployment profile deleted', 'success');
        deploymentProfilesLoaded = false;
        await loadDeploymentProfiles();
        if (clients.length) loadClients();
    } catch (err) {
        showAlert('Failed to delete deployment profile: ' + err.message, 'error');
    }
}

function effectiveSourceBadge(source) {
    const labels = { client: 'client', profile: 'profile', 'group-profile': 'group profile', group: 'group', default: 'default' };
    const cls = source === 'client' ? 'badge-success' : (source === 'default' ? 'badge-info' : 'badge-warning');
    return `<span class="badge ${cls}" style="margin-left: 6px;">${escapeHtml(labels[source] || source)}</span>`;
}

async function loadClientEffective(mac) {
    const box = document.getElementById('edit-client-effective');
    if (!box) return;
    box.textContent = 'Loading…';
    try {
        const res = await authFetch(`${API_BASE}/clients/effective?mac=${encodeURIComponent(mac)}`);
        const data = await res.json();
        if (!data.success) {
            box.textContent = data.error || 'Could not resolve effective settings';
            return;
        }
        const e = data.data;
        const src = e.sources || {};
        const row = (label, value, source) => `<div style="display: flex; justify-content: space-between; gap: 12px; padding: 2px 0;"><span>${label}</span><span>${value}${source ? effectiveSourceBadge(source) : ''}</span></div>`;
        const none = '<span style="color: var(--text-muted);">none</span>';
        let html = '';
        html += row('Profile', e.profile_name ? escapeHtml(e.profile_name) : (e.group_profile_name ? escapeHtml(e.group_profile_name) + ' <span style="color: var(--text-muted);">(via group)</span>' : none));
        html += row('Image', e.image_name ? escapeHtml(e.image_name) : none, e.image_filename ? src.image_filename : '');
        html += row('Auto-install file', e.auto_install_file ? '<code>' + escapeHtml(e.auto_install_file) + '</code>' : none, e.auto_install_file ? src.auto_install_file : '');
        html += row('SAN target', e.san_target_name ? escapeHtml(e.san_target_name) + (e.boot_from_san ? ' (default)' : '') : none, e.san_target_id ? src.san_target_id : '');
        html += row('Bootloader set', e.bootloader_set ? escapeHtml(e.bootloader_set) : '<span style="color: var(--text-muted);">global</span>', e.bootloader_set ? src.bootloader_set : '');
        html += row('Public images', e.show_public_images ? 'shown' : 'hidden', src.show_public_images);
        html += row('First boot', escapeHtml(firstBootActionLabel(e.first_boot_action)) + (e.disk_image_name ? ': ' + escapeHtml(e.disk_image_name) : ''), e.first_boot_action ? src.first_boot_action : '');
        box.innerHTML = html;
    } catch (err) {
        box.textContent = 'Could not resolve effective settings';
    }
}

document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('deployment-profile-form');
    if (form) form.addEventListener('submit', saveDeploymentProfile);
});

let enrolmentRules = [];
let enrolmentRulesLoaded = false;
let enrolmentPreviewTimer = null;

async function loadEnrolmentRules(quiet) {
    try {
        const res = await authFetch(`${API_BASE}/enrolment-rules`);
        const data = await res.json();
        if (!data.success) {
            if (!quiet) document.getElementById('enrolment-rules-table').innerHTML = `<p class="alert alert-error">${escapeHtml(data.error || 'Failed to load enrolment rules')}</p>`;
            return;
        }
        enrolmentRules = data.data || [];
        enrolmentRulesLoaded = true;
        if (!quiet) renderEnrolmentRulesTable();
    } catch (err) {
        console.error('Failed to load enrolment rules:', err);
    }
}

function enrolmentConditions(r) {
    const parts = [];
    if (r.mac_prefix) parts.push(`MAC ${escapeHtml(r.mac_prefix)}*`);
    if (r.manufacturer_contains) parts.push(`manufacturer ~ ${escapeHtml(r.manufacturer_contains)}`);
    if (r.product_contains) parts.push(`product ~ ${escapeHtml(r.product_contains)}`);
    if (r.serial_pattern) parts.push(`serial =~ <code>${escapeHtml(r.serial_pattern)}</code>`);
    if (r.uuid_prefix) parts.push(`UUID ${escapeHtml(r.uuid_prefix)}*`);
    if (r.firmware) parts.push(`firmware ${escapeHtml(r.firmware)}`);
    return parts.join('<br>');
}

function enrolmentActions(r) {
    const parts = [];
    if (r.client_group_id) { const g = clientGroups.find(x => x.id === r.client_group_id); parts.push('group ' + escapeHtml(g ? g.name : '#' + r.client_group_id)); }
    if (r.profile_id) parts.push('profile ' + escapeHtml(deploymentProfileName(r.profile_id)));
    if (r.name_pattern) parts.push('name <code>' + escapeHtml(r.name_pattern) + '</code>');
    if (r.make_static) parts.push('make static');
    return parts.join('<br>');
}

function renderEnrolmentRulesTable() {
    const container = document.getElementById('enrolment-rules-table');
    container.classList.remove('loading');
    if (enrolmentRules.length === 0) {
        container.innerHTML = '<p style="color: var(--text-secondary); padding: 20px;">No enrolment rules yet. Click "Add Rule" to place newly discovered clients automatically.</p>';
        return;
    }
    const rows = enrolmentRules.map((r, i) => `
        <tr style="${r.enabled ? '' : 'opacity: 0.6;'}">
            <td style="white-space: nowrap;">
                ${r.priority}
                <button type="button" class="btn btn-sm admin-only" title="Move up" ${i === 0 ? 'disabled' : ''} onclick="moveEnrolmentRule(${r.id}, -1)">▲</button>
                <button type="button" class="btn btn-sm admin-only" title="Move down" ${i === enrolmentRules.length - 1 ? 'disabled' : ''} onclick="moveEnrolmentRule(${r.id}, 1)">▼</button>
            </td>
            <td><strong>${escapeHtml(r.name)}</strong>${r.enabled ? '' : ' <span class="badge badge-warning">disabled</span>'}</td>
            <td style="font-size: 12px;">${enrolmentConditions(r)}</td>
            <td style="font-size: 12px;">${enrolmentActions(r)}</td>
            <td>${r.match_count || 0}${r.last_matched_at ? '<br><span style="color: var(--text-secondary); font-size: 11px;">' + new Date(r.last_matched_at).toLocaleString() + '</span>' : ''}</td>
            <td>
                <button type="button" class="btn btn-sm admin-only" onclick="toggleEnrolmentRule(${r.id})">${r.enabled ? 'Disable' : 'Enable'}</button>
                <button type="button" class="btn btn-sm admin-only" onclick="showEnrolmentRuleModal(${r.id})">Edit</button>
                <button type="button" class="btn btn-sm btn-danger admin-only" onclick="deleteEnrolmentRule(${r.id})">Delete</button>
            </td>
        </tr>`).join('');
    container.innerHTML = `
        <div class="table-scroll">
        <table>
            <thead><tr><th>Priority</th><th>Name</th><th>Conditions</th><th>Applies</th><th>Matches</th><th>Actions</th></tr></thead>
            <tbody>${rows}</tbody>
        </table>
        </div>`;
}

function enrolmentRuleBody(form) {
    const fd = new FormData(form);
    return {
        name: fd.get('name') || '',
        priority: parseInt(fd.get('priority') || '100', 10),
        enabled: fd.get('enabled') === 'on',
        mac_prefix: fd.get('mac_prefix') || '',
        manufacturer_contains: fd.get('manufacturer_contains') || '',
        product_contains: fd.get('product_contains') || '',
        serial_pattern: fd.get('serial_pattern') || '',
        uuid_prefix: fd.get('uuid_prefix') || '',
        firmware: fd.get('firmware') || '',
        client_group_id: fd.get('client_group_id') ? parseInt(fd.get('client_group_id'), 10) : null,
        profile_id: fd.get('profile_id') ? parseInt(fd.get('profile_id'), 10) : null,
        make_static: fd.get('make_static') === 'on',
        name_pattern: fd.get('name_pattern') || '',
    };
}

async function saveEnrolmentRuleQuiet(rule) {
    const res = await authFetch(`${API_BASE}/enrolment-rules?id=${rule.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(rule),
    });
    return res.json();
}

async function showEnrolmentRuleModal(id) {
    const form = document.getElementById('enrolment-rule-form');
    form.reset();
    const rule = id ? enrolmentRules.find(r => r.id === id) : null;
    form.elements.id.value = rule ? rule.id : '';
    document.getElementById('enrolment-rule-modal-title').textContent = rule ? 'Edit Enrolment Rule' : 'Add Enrolment Rule';
    if (!clientGroups || clientGroups.length === 0) { try { await loadClientGroups(); } catch (_) {} }
    const gSel = document.getElementById('er-group-select');
    gSel.innerHTML = '<option value="">(leave unchanged)</option>' + (clientGroups || []).map(g => `<option value="${g.id}" ${rule && rule.client_group_id === g.id ? 'selected' : ''}>${escapeHtml(g.name)}</option>`).join('');
    await populateDeploymentProfileSelect('er-profile-select', rule ? rule.profile_id : null, '(leave unchanged)');
    if (rule) {
        for (const key of ['name', 'priority', 'mac_prefix', 'manufacturer_contains', 'product_contains', 'serial_pattern', 'uuid_prefix', 'firmware', 'name_pattern']) {
            form.elements[key].value = rule[key] == null ? '' : rule[key];
        }
        form.elements.enabled.checked = !!rule.enabled;
        form.elements.make_static.checked = !!rule.make_static;
    } else {
        form.elements.priority.value = enrolmentRules.length ? Math.max(...enrolmentRules.map(r => r.priority)) + 10 : 100;
    }
    document.getElementById('enrolment-rule-preview').textContent = '';
    scheduleEnrolmentPreview();
    showModal('enrolment-rule-modal');
}

function scheduleEnrolmentPreview() {
    clearTimeout(enrolmentPreviewTimer);
    enrolmentPreviewTimer = setTimeout(previewEnrolmentRule, 400);
}

async function previewEnrolmentRule() {
    const form = document.getElementById('enrolment-rule-form');
    const box = document.getElementById('enrolment-rule-preview');
    const body = enrolmentRuleBody(form);
    body.name = body.name || 'preview';
    const hasCondition = body.mac_prefix || body.manufacturer_contains || body.product_contains || body.serial_pattern || body.uuid_prefix || body.firmware;
    if (!hasCondition) {
        box.textContent = 'Fill in at least one condition to see which existing clients would match.';
        return;
    }
    if (!body.client_group_id && !body.profile_id && !body.make_static && !body.name_pattern) body.make_static = true;
    try {
        const res = await authFetch(`${API_BASE}/enrolment-rules/test`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) {
            box.textContent = data.error || 'Preview unavailable';
            return;
        }
        const matches = data.data.matches || [];
        const eligible = matches.filter(m => m.eligible).length;
        if (matches.length === 0) {
            box.textContent = 'Matches no existing client.';
            return;
        }
        const sample = matches.slice(0, 5).map(m => `<code>${escapeHtml(m.mac_address)}</code>${m.name ? ' ' + escapeHtml(m.name) : (m.would_name ? ' → ' + escapeHtml(m.would_name) : '')}${m.eligible ? '' : ' <span style="color: var(--text-muted);">(already managed)</span>'}`).join(', ');
        box.innerHTML = `Matches ${matches.length} existing client${matches.length === 1 ? '' : 's'}, ${eligible} eligible for enrolment: ${sample}${matches.length > 5 ? ', …' : ''}`;
    } catch (err) {
        box.textContent = 'Preview unavailable';
    }
}

async function saveEnrolmentRule(e) {
    e.preventDefault();
    const form = e.target;
    const id = form.elements.id.value;
    const body = enrolmentRuleBody(form);
    try {
        const res = await authFetch(`${API_BASE}/enrolment-rules${id ? '?id=' + id : ''}`, {
            method: id ? 'PUT' : 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to save enrolment rule', 'error');
            return;
        }
        closeModal('enrolment-rule-modal');
        showAlert(data.message || 'Enrolment rule saved', 'success');
        await loadEnrolmentRules();
    } catch (err) {
        showAlert('Failed to save enrolment rule: ' + err.message, 'error');
    }
}

async function toggleEnrolmentRule(id) {
    const rule = enrolmentRules.find(r => r.id === id);
    if (!rule) return;
    const data = await saveEnrolmentRuleQuiet({ ...rule, enabled: !rule.enabled });
    if (!data.success) {
        showAlert(data.error || 'Failed to update enrolment rule', 'error');
        return;
    }
    await loadEnrolmentRules();
}

async function moveEnrolmentRule(id, direction) {
    const index = enrolmentRules.findIndex(r => r.id === id);
    const other = enrolmentRules[index + direction];
    if (index < 0 || !other) return;
    const rule = enrolmentRules[index];
    let a = other.priority, b = rule.priority;
    if (a === b) { a = direction < 0 ? b : b + 1; b = direction < 0 ? b + 1 : b; }
    const first = await saveEnrolmentRuleQuiet({ ...rule, priority: a });
    const second = await saveEnrolmentRuleQuiet({ ...other, priority: b });
    if (!first.success || !second.success) showAlert((first.error || second.error) || 'Failed to reorder rules', 'error');
    await loadEnrolmentRules();
}

async function deleteEnrolmentRule(id) {
    const rule = enrolmentRules.find(r => r.id === id);
    if (!rule) return;
    if (!confirm(`Delete enrolment rule "${rule.name}"? Clients it already enrolled keep their group and profile.`)) return;
    try {
        const res = await authFetch(`${API_BASE}/enrolment-rules?id=${id}`, { method: 'DELETE' });
        const data = await res.json();
        if (!data.success) {
            showAlert(data.error || 'Failed to delete enrolment rule', 'error');
            return;
        }
        showAlert('Enrolment rule deleted', 'success');
        await loadEnrolmentRules();
    } catch (err) {
        showAlert('Failed to delete enrolment rule: ' + err.message, 'error');
    }
}

async function applyEnrolmentRules() {
    if (!confirm('Run every enabled rule over discovered clients that have no group, profile or rule yet?')) return;
    const status = document.getElementById('enrolment-rules-status');
    status.textContent = 'Applying…';
    try {
        const res = await authFetch(`${API_BASE}/enrolment-rules/apply`, { method: 'POST' });
        const data = await res.json();
        if (!data.success) {
            status.textContent = '';
            showAlert(data.error || 'Failed to apply enrolment rules', 'error');
            return;
        }
        status.textContent = `${data.data.enrolled.length} of ${data.data.considered} unenrolled client(s) placed`;
        showAlert(data.message || 'Enrolment rules applied', 'success');
        await loadEnrolmentRules();
        if (clients.length) loadClients();
    } catch (err) {
        status.textContent = '';
        showAlert('Failed to apply enrolment rules: ' + err.message, 'error');
    }
}

document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('enrolment-rule-form');
    if (!form) return;
    form.addEventListener('submit', saveEnrolmentRule);
    form.addEventListener('input', scheduleEnrolmentPreview);
    form.addEventListener('change', scheduleEnrolmentPreview);
});
