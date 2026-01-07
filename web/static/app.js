// API Base URL
const API_BASE = '/api';

// State
let clients = [];
let images = [];
let currentClient = null;
let imageSortColumn = 'name';
let imageSortDirection = 'asc';
let extractionProgress = {}; // Track extraction progress by filename

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

async function loadCurrentUser() {
    try {
        // We'll need to get the current user info from the auth context
        // For now, we can extract it from Basic Auth header or create an endpoint
        const username = getCurrentUsername();
        document.getElementById('current-username').textContent = username;

        // Check if user is admin by trying to access admin-only endpoint
        try {
            const res = await fetch(`${API_BASE}/users`);
            if (res.ok) {
                document.getElementById('current-user-role').textContent = 'Administrator';
            } else {
                document.getElementById('current-user-role').textContent = 'User';
            }
        } catch (err) {
            document.getElementById('current-user-role').textContent = 'User';
        }
    } catch (err) {
        console.error('Failed to load user info:', err);
    }
}

function getCurrentUsername() {
    // Try to get username from localStorage if we stored it during login
    const stored = localStorage.getItem('bootimus_username');
    if (stored) return stored;

    // Otherwise return a default
    return 'admin';
}

function logout() {
    // Clear any stored credentials
    localStorage.removeItem('bootimus_username');

    // Show notification
    showNotification('Logging out...', 'info');

    // Redirect to logout endpoint which will clear Basic Auth
    // Since we use Basic Auth, we need to send invalid credentials to force re-auth
    setTimeout(() => {
        window.location.href = '/logout';
    }, 500);
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

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    setupTabs();
    setupForms();
    setupUpload();
    loadCurrentUser();
    loadStats();
    loadServerInfo();
    loadClients();
    loadImages();
    loadPublicFiles();
    loadLogs();
    loadUsers();

    // Load active sessions
    loadActiveSessions();

    // Refresh every 30 seconds
    setInterval(() => {
        loadStats();
        loadActiveSessions();
        const activeTab = document.querySelector('.tab.active').dataset.tab;
        if (activeTab === 'clients') loadClients();
        if (activeTab === 'images') loadImages();
        if (activeTab === 'public-files') loadPublicFiles();
        if (activeTab === 'logs') loadLogs();
        if (activeTab === 'users') loadUsers();
    }, 30000);

    // Refresh server info more frequently for live stats (every 5 seconds)
    setInterval(() => {
        const activeTab = document.querySelector('.tab.active').dataset.tab;
        if (activeTab === 'server') loadServerInfo();
    }, 5000);

    // Refresh active sessions more frequently (every 3 seconds)
    setInterval(loadActiveSessions, 3000);
});

// Tab Management
function setupTabs() {
    document.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

            tab.classList.add('active');
            document.getElementById(`${tab.dataset.tab}-tab`).classList.add('active');

            if (tab.dataset.tab === 'groups') {
                loadGroups();
            }
        });
    });
}

// Stats
async function loadStats() {
    try {
        const res = await fetch(`${API_BASE}/stats`);
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
        const res = await fetch(`${API_BASE}/active-sessions`);
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

    content.innerHTML = html || '<p style="color: #94a3b8;">No active sessions</p>';
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

// Server Info
async function loadServerInfo() {
    try {
        const res = await fetch(`${API_BASE}/server-info`);
        const data = await res.json();

        if (data.success) {
            renderServerInfo(data.data);
        }
    } catch (err) {
        document.getElementById('server-info').innerHTML = '<p class="alert alert-error">Failed to load server info</p>';
    }
}

function renderServerInfo(info) {
    const container = document.getElementById('server-info');

    const sysStats = info.system_stats || {};

    const html = `
        ${info.version || sysStats.host ? `
            <div class="info-section" style="margin-bottom: 20px;">
                <h3>System Information</h3>
                ${info.version ? `
                <div class="info-item">
                    <span class="info-label">Bootimus Version</span>
                    <span class="info-value"><code>${info.version}</code></span>
                </div>
                ` : ''}
                ${info.configuration && info.configuration.runtime_mode ? `
                <div class="info-item">
                    <span class="info-label">Runtime Mode</span>
                    <span class="info-value"><span class="badge ${info.configuration.runtime_mode === 'Docker' ? 'badge-info' : 'badge-success'}">${info.configuration.runtime_mode}</span></span>
                </div>
                ` : ''}
                ${sysStats.host ? `
                    ${sysStats.host.platform ? `
                    <div class="info-item">
                        <span class="info-label">Operating System</span>
                        <span class="info-value">${sysStats.host.platform} ${sysStats.host.platform_version || ''}</span>
                    </div>
                    ` : sysStats.host.os ? `
                    <div class="info-item">
                        <span class="info-label">Operating System</span>
                        <span class="info-value">${sysStats.host.os}</span>
                    </div>
                    ` : ''}
                    ${sysStats.host.architecture ? `
                    <div class="info-item">
                        <span class="info-label">Architecture</span>
                        <span class="info-value">${sysStats.host.architecture}</span>
                    </div>
                    ` : ''}
                ` : ''}
                ${sysStats.uptime ? `
                <div class="info-item">
                    <span class="info-label">Server Uptime</span>
                    <span class="info-value">${sysStats.uptime}</span>
                </div>
                ` : ''}
            </div>
        ` : ''}

        ${sysStats.cpu || sysStats.memory || (sysStats.disk && sysStats.disk.length) ? `
            <div style="margin-bottom: 30px;">
                <h3 style="margin-bottom: 15px;">System Resources</h3>
                <div class="stats-grid">
                    ${sysStats.cpu ? `
                    <div class="stat-card">
                        <div class="stat-card-header">
                            <div class="stat-card-title">CPU Usage</div>
                            <div class="stat-card-value" style="color: ${sysStats.cpu.usage_percent > 80 ? '#ef4444' : sysStats.cpu.usage_percent > 60 ? '#f59e0b' : '#10b981'}">
                                ${sysStats.cpu.usage_percent.toFixed(1)}%
                            </div>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: ${sysStats.cpu.usage_percent}%; background: ${sysStats.cpu.usage_percent > 80 ? '#ef4444' : sysStats.cpu.usage_percent > 60 ? '#f59e0b' : '#10b981'}"></div>
                        </div>
                        <div class="stat-card-info">${sysStats.cpu.cores} CPU cores available</div>
                    </div>
                    ` : ''}

                    ${sysStats.memory ? `
                    <div class="stat-card">
                        <div class="stat-card-header">
                            <div class="stat-card-title">Memory Usage</div>
                            <div class="stat-card-value" style="color: ${sysStats.memory.used_percent > 80 ? '#ef4444' : sysStats.memory.used_percent > 60 ? '#f59e0b' : '#10b981'}">
                                ${sysStats.memory.used_percent.toFixed(1)}%
                            </div>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: ${sysStats.memory.used_percent}%; background: ${sysStats.memory.used_percent > 80 ? '#ef4444' : sysStats.memory.used_percent > 60 ? '#f59e0b' : '#10b981'}"></div>
                        </div>
                        <div class="stat-card-info">
                            ${formatBytes(sysStats.memory.used)} / ${formatBytes(sysStats.memory.total)} (${formatBytes(sysStats.memory.free)} free)
                        </div>
                    </div>
                    ` : ''}

                    ${(sysStats.disk || []).map(disk => `
                    <div class="stat-card">
                        <div class="stat-card-header">
                            <div class="stat-card-title">Disk: ${disk.path}</div>
                            <div class="stat-card-value" style="color: ${disk.used_percent > 80 ? '#ef4444' : disk.used_percent > 60 ? '#f59e0b' : '#10b981'}">
                                ${disk.used_percent.toFixed(1)}%
                            </div>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-fill" style="width: ${disk.used_percent}%; background: ${disk.used_percent > 80 ? '#ef4444' : disk.used_percent > 60 ? '#f59e0b' : '#10b981'}"></div>
                        </div>
                        <div class="stat-card-info">
                            ${formatBytes(disk.used)} / ${formatBytes(disk.total)} (${formatBytes(disk.free)} free)
                        </div>
                    </div>
                    `).join('')}
                </div>
            </div>
        ` : ''}

        <div class="info-grid">
            <div class="info-section">
                <h3>Configuration</h3>
                ${Object.entries(info.configuration || {}).filter(([key]) => key !== 'runtime_mode').map(([key, value]) => `
                    <div class="info-item">
                        <span class="info-label">${key.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase())}</span>
                        <span class="info-value">${value || '<em>not set</em>'}</span>
                    </div>
                `).join('')}
            </div>
            <div class="info-section">
                <h3>Environment Variables</h3>
                ${Object.entries(info.environment || {}).filter(([k, v]) => v).map(([key, value]) => `
                    <div class="info-item">
                        <span class="info-label">${key}</span>
                        <span class="info-value">${value || '<em>not set</em>'}</span>
                    </div>
                `).join('')}
                ${Object.entries(info.environment || {}).every(([k, v]) => !v) ? '<p style="color: #94a3b8; padding: 10px;">No environment variables set</p>' : ''}
            </div>
        </div>
    `;

    container.innerHTML = html;
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

// Clients
async function loadClients() {
    try {
        const res = await fetch(`${API_BASE}/clients`);
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

    if (clients.length === 0) {
        container.innerHTML = '<p style="color: #94a3b8; padding: 20px;">No clients yet. Add one to get started.</p>';
        return;
    }

    const html = `
        <table>
            <thead>
                <tr>
                    <th>MAC Address</th>
                    <th>Name</th>
                    <th>Status</th>
                    <th>Assigned Images</th>
                    <th>Boot Count</th>
                    <th>Last Boot</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${clients.map(client => `
                    <tr>
                        <td><code>${client.mac_address}</code></td>
                        <td>${client.name || '-'}</td>
                        <td>
                            <span class="badge ${client.enabled ? 'badge-success' : 'badge-danger'}">
                                ${client.enabled ? 'Enabled' : 'Disabled'}
                            </span>
                        </td>
                        <td>
                            ${(client.images || []).length > 0 ?
                                `<span title="${(client.images || []).map(i => i.name).join(', ')}">${(client.images || []).length} images</span>` :
                                '<span style="color: #94a3b8;">No images</span>'
                            }
                        </td>
                        <td>${client.boot_count || 0}</td>
                        <td>${client.last_boot ? new Date(client.last_boot).toLocaleString() : 'Never'}</td>
                        <td>
                            <button class="btn btn-primary btn-sm" onclick="editClient('${client.mac_address}')">Edit & Assign Images</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteClient('${client.mac_address}')">Delete</button>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;

    container.innerHTML = html;
}

function showAddClientModal() {
    document.getElementById('add-client-form').reset();
    showModal('add-client-modal');
}

async function editClient(mac) {
    try {
        const res = await fetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`);
        const data = await res.json();

        console.log('Edit client API response:', data);

        if (data.success) {
            currentClient = data.data;
            console.log('Current client data:', currentClient);

            const form = document.getElementById('edit-client-form');

            // Set form values
            form.querySelector('[name="mac_address"]').value = currentClient.mac_address || mac || '';
            form.querySelector('[name="name"]').value = currentClient.name || '';
            form.querySelector('[name="description"]').value = currentClient.description || '';
            form.querySelector('[name="enabled"]').checked = currentClient.enabled || false;

            console.log('Form values after setting:', {
                mac: form.querySelector('[name="mac_address"]').value,
                name: form.querySelector('[name="name"]').value,
                description: form.querySelector('[name="description"]').value,
                enabled: form.querySelector('[name="enabled"]').checked
            });

            // Populate images select
            const select = document.getElementById('edit-images-select');
            console.log('Current client images:', currentClient.images);
            console.log('Available images:', images);

            select.innerHTML = images.map(img => {
                const isSelected = (currentClient.images || []).some(i => i.filename === img.filename);
                console.log(`Image ${img.name}: selected=${isSelected}`);
                return `<option value="${img.filename}" ${isSelected ? 'selected' : ''}>${img.name}</option>`;
            }).join('');

            showModal('edit-client-modal');
        } else {
            showAlert(data.error || 'Failed to load client', 'error');
        }
    } catch (err) {
        console.error('Error in editClient:', err);
        showAlert('Failed to load client', 'error');
    }
}

async function deleteClient(mac) {
    if (!confirm(`Delete client ${mac}?`)) return;

    try {
        const res = await fetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`, { method: 'DELETE' });
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

// Images
async function loadImages() {
    try {
        const [imagesRes, filesRes] = await Promise.all([
            fetch(`${API_BASE}/images`),
            fetch(`${API_BASE}/files`)
        ]);

        const imagesData = await imagesRes.json();
        const filesData = await filesRes.json();

        if (imagesData.success) {
            images = imagesData.data || [];

            // Associate files with images
            if (filesData.success) {
                const allFiles = filesData.data || [];
                images.forEach(img => {
                    img.files = allFiles.filter(f => !f.public && f.image_id === img.id);
                });
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
            default:
                return 0;
        }

        if (aVal < bVal) return imageSortDirection === 'asc' ? -1 : 1;
        if (aVal > bVal) return imageSortDirection === 'asc' ? 1 : -1;
        return 0;
    });

    return sorted;
}

function renderImagesTable() {
    const container = document.getElementById('images-table');

    if (images.length === 0) {
        container.innerHTML = '<p style="color: #94a3b8; padding: 20px;">No images yet. Upload or scan for ISOs.</p>';
        return;
    }

    const sortIcon = (column) => {
        if (imageSortColumn !== column) return '↕';
        return imageSortDirection === 'asc' ? '↑' : '↓';
    };

    const sortedImages = getSortedImages();

    const html = `
        <table>
            <thead>
                <tr>
                    <th onclick="sortImages('name')" style="cursor: pointer;">Name ${sortIcon('name')}</th>
                    <th onclick="sortImages('filename')" style="cursor: pointer;">Filename ${sortIcon('filename')}</th>
                    <th onclick="sortImages('size')" style="cursor: pointer;">Size ${sortIcon('size')}</th>
                    <th onclick="sortImages('status')" style="cursor: pointer;">Status ${sortIcon('status')}</th>
                    <th onclick="sortImages('visibility')" style="cursor: pointer;">Visibility ${sortIcon('visibility')}</th>
                    <th onclick="sortImages('boot_method')" style="cursor: pointer;">Boot Method ${sortIcon('boot_method')}</th>
                    <th onclick="sortImages('distro')" style="cursor: pointer;">Distro ${sortIcon('distro')}</th>
                    <th onclick="sortImages('boot_count')" style="cursor: pointer;">Boot Count ${sortIcon('boot_count')}</th>
                    <th>Operations</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${sortedImages.map(img => `
                    <tr>
                        <td>${img.name}</td>
                        <td><code>${img.filename}</code></td>
                        <td>${formatBytes(img.size)}</td>
                        <td>
                            <span class="badge ${img.enabled ? 'badge-success' : 'badge-danger'}">
                                ${img.enabled ? 'Enabled' : 'Disabled'}
                            </span>
                        </td>
                        <td>
                            <span class="badge ${img.public ? 'badge-success' : 'badge-info'}">
                                ${img.public ? 'Public' : 'Private'}
                            </span>
                        </td>
                        <td>
                            ${img.boot_method === 'kernel' ?
                                '<span class="badge badge-success">Kernel/Initrd</span>' :
                                img.boot_method === 'memdisk' ?
                                '<span class="badge badge-warning">Memdisk (ThinOS)</span>' :
                                '<span class="badge badge-info">SAN Boot</span>'
                            }
                            ${!img.sanboot_compatible && img.sanboot_hint && img.boot_method === 'sanboot' && !img.extracted ?
                                '<br><span style="color: #ff9800; font-size: 0.85em; margin-top: 4px; display: block;">⚠ '+img.sanboot_hint+'</span>' :
                                ''
                            }
                            ${img.extracted && img.boot_method === 'sanboot' ?
                                '<br><button class="btn btn-sm" style="margin-top: 4px;" onclick="setBootMethod(\''+img.filename+'\', \'kernel\')">Switch to Kernel</button>' :
                                ''
                            }
                            ${img.boot_method === 'kernel' && img.sanboot_compatible ?
                                '<br><button class="btn btn-sm" style="margin-top: 4px;" onclick="setBootMethod(\''+img.filename+'\', \'sanboot\')">Switch to SAN</button>' :
                                ''
                            }
                            ${img.boot_method === 'kernel' && !img.sanboot_compatible ?
                                '<br><button class="btn btn-sm" style="margin-top: 4px; opacity: 0.5;" disabled title="'+img.sanboot_hint+'">SAN Not Supported</button>' :
                                ''
                            }
                        </td>
                        <td>
                            ${img.extracted ?
                                (img.distro ? '<span class="badge badge-info">'+img.distro+'</span>' : '<span class="badge badge-success">✓ Extracted</span>') :
                                (img.extraction_error ? '<span class="badge badge-danger" title="'+img.extraction_error+'">Error</span>' : '')
                            }
                        </td>
                        <td>${img.boot_count || 0}</td>
                        <td class="operations-cell">
                            ${extractionProgress[img.filename] ? `
                                <div class="progress-container">
                                    <div class="progress-bar">
                                        <div class="progress-fill" style="width: ${extractionProgress[img.filename].progress}%"></div>
                                    </div>
                                    <div class="progress-text">${extractionProgress[img.filename].status}</div>
                                </div>
                            ` : (img.netboot_required ?
                                (img.netboot_available ?
                                    '<span style="color: #4caf50;">✓ Netboot Ready</span>' :
                                    '<span style="color: #ff9800;">⚠ Netboot Required</span>') :
                                (img.extracted ? '<span style="color: #4caf50;">✓ Ready</span>' : '<span style="color: #999;">Not extracted</span>')
                            )}
                        </td>
                        <td>
                            ${!img.extracted && !extractionProgress[img.filename] && !img.netboot_required ?
                                '<button class="btn btn-success btn-sm" onclick="extractImage(\''+img.filename+'\', \''+img.name+'\')">Extract</button>' :
                                ''
                            }
                            ${img.netboot_required && !img.netboot_available ?
                                '<button class="btn btn-warning btn-sm" onclick="downloadNetboot(\''+img.filename+'\', \''+img.name+'\')">⬇ Netboot</button>' :
                                ''
                            }
                            ${extractionProgress[img.filename] ?
                                '<button class="btn btn-sm" disabled style="opacity: 0.5;">Extracting...</button>' :
                                ''
                            }
                            <button class="btn btn-info btn-sm" onclick="showImagePropertiesModal('${img.filename}')">⚙️ Properties</button>
                            <button class="btn btn-sm" onclick="showAutoInstallModal('${img.filename}', '${img.name}')">Auto-Install</button>
                            <button class="btn btn-sm" onclick="showImageFilesModal(${img.id}, '${escapeHtml(img.name)}')">📁 Files</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteImage('${img.filename}', '${img.name}')">Delete</button>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;

    container.innerHTML = html;
}

async function toggleImage(filename, currentState) {
    try {
        const res = await fetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}`, {
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
        const res = await fetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}`, {
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
        const res = await fetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}&delete_file=true`, { method: 'DELETE' });
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
        const res = await fetch(`${API_BASE}/scan`, { method: 'POST' });
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

async function extractImage(filename, name) {
    if (!confirm(`Extract kernel and initrd from ${name}?\n\nThis will mount the ISO and extract boot files for direct kernel booting.`)) return;

    try {
        // Set initial progress
        extractionProgress[filename] = { progress: 0, status: 'Starting extraction...' };
        renderImagesTable();

        // Simulate progress updates (since we don't have real progress from backend)
        const progressInterval = setInterval(() => {
            if (extractionProgress[filename] && extractionProgress[filename].progress < 90) {
                extractionProgress[filename].progress += 10;
                if (extractionProgress[filename].progress < 30) {
                    extractionProgress[filename].status = 'Mounting ISO...';
                } else if (extractionProgress[filename].progress < 60) {
                    extractionProgress[filename].status = 'Detecting distribution...';
                } else {
                    extractionProgress[filename].status = 'Extracting boot files...';
                }
                renderImagesTable();
            }
        }, 500);

        const res = await fetch(`${API_BASE}/images/extract?filename=${encodeURIComponent(filename)}`, { method: 'POST' });
        const data = await res.json();

        clearInterval(progressInterval);

        if (data.success) {
            extractionProgress[filename] = { progress: 100, status: 'Complete!' };
            renderImagesTable();
            setTimeout(() => {
                delete extractionProgress[filename];
                loadImages();
                showAlert(data.message || 'Extraction successful', 'success');
            }, 1000);
        } else {
            delete extractionProgress[filename];
            renderImagesTable();
            showAlert(data.error || 'Extraction failed', 'error');
        }
    } catch (err) {
        delete extractionProgress[filename];
        renderImagesTable();
        showAlert('Failed to extract image', 'error');
    }
}

async function downloadNetboot(filename, name) {
    if (!confirm(`Download netboot files for ${name}?\n\nThis will download and extract the proper network boot files required for Debian/Ubuntu network installation.`)) return;

    try {
        // Set initial progress
        extractionProgress[filename] = { progress: 0, status: 'Downloading netboot...' };
        renderImagesTable();

        // Simulate progress updates
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
                renderImagesTable();
            }
        }, 500);

        const res = await fetch(`${API_BASE}/images/netboot/download?filename=${encodeURIComponent(filename)}`, { method: 'POST' });
        const data = await res.json();

        clearInterval(progressInterval);

        if (data.success) {
            extractionProgress[filename] = { progress: 100, status: 'Complete!' };
            renderImagesTable();
            setTimeout(() => {
                delete extractionProgress[filename];
                loadImages();
                showAlert(data.message || 'Netboot files downloaded successfully', 'success');
            }, 1000);
        } else {
            delete extractionProgress[filename];
            renderImagesTable();
            showAlert(data.error || 'Netboot download failed', 'error');
        }
    } catch (err) {
        delete extractionProgress[filename];
        renderImagesTable();
        showAlert('Failed to download netboot files', 'error');
    }
}

async function setBootMethod(filename, method) {
    try {
        const res = await fetch(`${API_BASE}/images/boot-method`, {
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
        'kernel': 'memdisk',
        'memdisk': 'sanboot'
    };
    const nextMethod = cycle[currentMethod] || 'sanboot';
    await setBootMethod(filename, nextMethod);
}

// Boot Logs
async function loadLogs() {
    try {
        const res = await fetch(`${API_BASE}/logs?limit=50`);
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
        container.innerHTML = '<p style="color: #94a3b8; padding: 20px;">No boot logs yet.</p>';
        return;
    }

    const html = `
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
            const res = await fetch(`${API_BASE}/clients`, {
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

        const updates = {
            name: formData.get('name'),
            description: formData.get('description'),
            enabled: formData.get('enabled') === 'on'
        };

        try {
            // Update client
            const res1 = await fetch(`${API_BASE}/clients?mac=${encodeURIComponent(mac)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates)
            });

            // Update image assignments
            const selectedFilenames = Array.from(document.getElementById('edit-images-select').selectedOptions)
                .map(opt => opt.value);

            const res2 = await fetch(`${API_BASE}/assign-images`, {
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
                <div style="background: #0f172a; height: 20px; border-radius: 10px; overflow: hidden;">
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
    fetch('/api/logs/buffer')
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
        liveLogsDiv.innerHTML = '<div style="color: #94a3b8;">No logs available. Logs will appear here as the server runs.</div>';
        return;
    }

    logs.forEach(log => {
        const logEntry = document.createElement('div');
        logEntry.style.color = '#e2e8f0';
        logEntry.style.marginBottom = '4px';
        logEntry.style.fontFamily = 'monospace';
        logEntry.style.fontSize = '13px';
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
    document.getElementById('live-logs').innerHTML = '<div style="color: #94a3b8;">Click "Refresh" to load logs...</div>';
}

// ==================== User Management ====================

function loadUsers() {
    fetch('/api/users')
        .then(response => response.json())
        .then(data => {
            if (data.success) {
                renderUsersTable(data.data);
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

function renderUsersTable(users) {
    if (!users || users.length === 0) {
        document.getElementById('users-table').innerHTML =
            '<p style="color: #94a3b8;">No users found</p>';
        return;
    }

    let html = `
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
        const role = user.is_admin ? '<span class="badge badge-admin">Admin</span>' : '<span class="badge badge-user">User</span>';
        const status = user.enabled ? '<span class="badge badge-enabled">Enabled</span>' : '<span class="badge badge-disabled">Disabled</span>';
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
                    <button class="btn-small" onclick='editUser(${JSON.stringify(user)})'>Edit</button>
                    <button class="btn-small" onclick='showResetPasswordModal(${JSON.stringify(user)})'>Reset Password</button>
                    ${user.username !== 'admin' ? `<button class="btn-small btn-danger" onclick="deleteUser('${user.username}')">Delete</button>` : ''}
                </td>
            </tr>
        `;
    });

    html += '</tbody></table>';
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

    fetch(`/api/users?username=${encodeURIComponent(username)}`, {
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

    fetch('/api/users', {
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

    const userData = {
        username: formData.get('username'),
        is_admin: formData.get('is_admin') === 'on',
        enabled: formData.get('enabled') === 'on'
    };

    fetch('/api/users', {
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

    fetch('/api/users/reset-password', {
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
        const res = await fetch(`${API_BASE}/groups`, {
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
        const res = await fetch(`${API_BASE}/groups/update`, {
            method: 'POST',
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

    fetch('/api/images/download', {
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
    fetch('/api/downloads/progress?filename=' + encodeURIComponent(filename))
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
        const res = await fetch(`${API_BASE}/images/autoinstall?filename=${encodeURIComponent(filename)}`);
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
        const res = await fetch(`${API_BASE}/images/autoinstall?filename=${encodeURIComponent(filename)}`, {
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
        const res = await fetch('/api/files');
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
        container.innerHTML = '<p style="color: #94a3b8; padding: 20px; text-align: center;">No public files found. Upload your first public file to get started.</p>';
        return;
    }

    const table = document.createElement('table');
    table.className = 'data-table';
    table.innerHTML = `
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
                    <td><code style="color: #38bdf8;">${escapeHtml(file.filename)}</code></td>
                    <td>${escapeHtml(file.description || '-')}</td>
                    <td><span class="badge">${escapeHtml(file.content_type || 'unknown')}</span></td>
                    <td>${formatBytes(file.size)}</td>
                    <td>${file.download_count || 0}</td>
                    <td>
                        <button class="btn btn-small" onclick="copyFileDownloadURL('${escapeHtml(file.filename)}')">📋 Copy URL</button>
                        <button class="btn btn-small" onclick="showEditFileModal(${file.id})">Edit</button>
                        <button class="btn btn-danger btn-small" onclick="deleteFile(${file.id}, '${escapeHtml(file.filename)}')">Delete</button>
                    </td>
                </tr>
            `).join('')}
        </tbody>
    `;

    container.innerHTML = '';
    container.appendChild(table);
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
        const res = await fetch('/api/files/upload', {
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
        container.innerHTML = '<p style="color: #94a3b8; padding: 20px; text-align: center;">No files for this image yet.</p>';
        return;
    }

    const table = document.createElement('table');
    table.className = 'data-table';
    table.innerHTML = `
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
                    <td><code style="color: #38bdf8;">${escapeHtml(file.filename)}</code></td>
                    <td>${escapeHtml(file.description || '-')}</td>
                    <td><span class="badge">${escapeHtml(file.content_type || 'unknown')}</span></td>
                    <td>${formatBytes(file.size)}</td>
                    <td>${file.download_count || 0}</td>
                    <td>
                        <button class="btn btn-small" onclick="copyFileDownloadURL('${escapeHtml(file.filename)}')">📋 Copy URL</button>
                        <button class="btn btn-small" onclick="showEditFileModal(${file.id})">Edit</button>
                        <button class="btn btn-danger btn-small" onclick="deleteFile(${file.id}, '${escapeHtml(file.filename)}')">Delete</button>
                    </td>
                </tr>
            `).join('')}
        </tbody>
    `;

    container.innerHTML = '';
    container.appendChild(table);
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
        const res = await fetch('/api/files/upload', {
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
        const res = await fetch('/api/files');
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
        const res = await fetch(`/api/files/update?id=${fileId}`, {
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
        const res = await fetch(`/api/files/delete?id=${fileId}`, {
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
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

let groups = [];

async function loadGroups() {
    try {
        const res = await fetch(`${API_BASE}/groups`);
        const data = await res.json();

        if (data.success && data.data) {
            groups = data.data;
            renderGroupsTable();
        } else {
            document.getElementById('groups-table').innerHTML = '<p style="color: #94a3b8; padding: 20px;">No groups found.</p>';
        }
    } catch (err) {
        document.getElementById('groups-table').innerHTML = '<p style="color: #ef4444; padding: 20px;">Failed to load groups</p>';
        console.error(err);
    }
}

function renderGroupsTable() {
    const container = document.getElementById('groups-table');

    if (!groups || groups.length === 0) {
        container.innerHTML = '<p style="color: #94a3b8; padding: 20px;">No groups found. Click "+ Add Group" to create one.</p>';
        return;
    }

    const sortedGroups = [...groups].sort((a, b) => {
        if (a.order !== b.order) return a.order - b.order;
        return a.name.localeCompare(b.name);
    });

    let html = `
        <table>
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Description</th>
                    <th>Parent</th>
                    <th>Order</th>
                    <th>Status</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
    `;

    for (const group of sortedGroups) {
        const parentName = group.parent_id ? (groups.find(g => g.id === group.parent_id)?.name || 'Unknown') : '-';
        const status = group.enabled ? '<span class="badge badge-success">Enabled</span>' : '<span class="badge badge-danger">Disabled</span>';

        html += `
            <tr>
                <td><strong>${escapeHtml(group.name)}</strong></td>
                <td>${escapeHtml(group.description || '-')}</td>
                <td>${escapeHtml(parentName)}</td>
                <td>${group.order}</td>
                <td>${status}</td>
                <td>
                    <button class="btn btn-sm btn-info" onclick="showEditGroupModal(${group.id})">Edit</button>
                    <button class="btn btn-sm btn-danger" onclick="deleteGroup(${group.id}, '${escapeHtml(group.name).replace(/'/g, "\\'")}')">Delete</button>
                </td>
            </tr>
        `;
    }

    html += `
            </tbody>
        </table>
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

async function deleteGroup(groupId, groupName) {
    if (!confirm(`Delete group "${groupName}"? This will unassign all images from this group.`)) return;

    try {
        const res = await fetch(`${API_BASE}/groups/delete`, {
            method: 'POST',
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
    document.querySelectorAll('#image-properties-modal .tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.props-tab-content').forEach(c => c.style.display = 'none');

    const clickedTab = document.querySelector(`#image-properties-modal .tab[data-tab="${tabName}"]`);
    if (clickedTab) clickedTab.classList.add('active');

    if (tabName === 'props-general') {
        document.getElementById('props-general-content').style.display = 'block';
    } else if (tabName === 'props-files') {
        document.getElementById('props-files-content').style.display = 'block';
        const filename = document.getElementById('image-props-filename').value;
        if (filename) {
            loadImageFileBrowser(filename);
        }
    }
}

async function showImagePropertiesModal(filename) {
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
    document.getElementById('image-props-boot-params').value = img.boot_params || '';
    document.getElementById('image-props-enabled').checked = img.enabled;
    document.getElementById('image-props-public').checked = img.public;

    const groupSelect = document.getElementById('image-props-group');
    groupSelect.innerHTML = '<option value="">Unassigned</option>';
    for (const group of groups) {
        const selected = img.group_id === group.id ? 'selected' : '';
        groupSelect.innerHTML += `<option value="${group.id}" ${selected}>${escapeHtml(group.name)}</option>`;
    }

    switchPropsTab('props-general');
    openModal('image-properties-modal');
}

async function loadImageFileBrowser(filename) {
    const container = document.getElementById('image-file-browser');
    container.innerHTML = '<div class="loading"><div class="spinner"></div>Loading files...</div>';

    try {
        const res = await fetch(`${API_BASE}/images/files?filename=${encodeURIComponent(filename)}`);
        const data = await res.json();

        if (data.success && data.data && data.data.files) {
            renderFileBrowser(data.data.files, filename);
        } else {
            container.innerHTML = '<p style="color: #94a3b8; padding: 20px;">No files found for this image.</p>';
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
        <div style="background: #0f172a; padding: 15px; border-radius: 6px; margin-bottom: 15px;">
            <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px;">
                <div style="font-family: monospace; color: #38bdf8;">📁 /isos/${escapeHtml(filename)}</div>
                <button class="btn btn-danger btn-sm" onclick="deleteImageFile('${escapeHtml(filename)}', '${escapeHtml(baseDir)}', '${escapeHtml(filename)}', false, true)" style="padding: 4px 10px; font-size: 12px;">Delete ISO</button>
            </div>
            <div style="display: flex; align-items: center; justify-content: space-between;">
                <div style="font-family: monospace; color: ${hasFiles ? '#38bdf8' : '#64748b'};">📁 /boot/${escapeHtml(baseDir)}/ ${hasFiles ? '' : '<span style="color: #94a3b8; font-size: 11px;">(not extracted)</span>'}</div>
                ${hasFiles ? '<button class="btn btn-danger btn-sm" onclick="deleteImageFile(\'' + escapeHtml(filename) + '\', \'' + escapeHtml(baseDir) + '\', \'\', true, false)" style="padding: 4px 10px; font-size: 12px;">Delete Boot Folder</button>' : ''}
            </div>
        </div>
    `;

    if (hasFiles) {
        const tree = buildFileTree(files);
        html += `
            <div style="max-height: 500px; overflow-y: auto; background: #0f172a; border-radius: 6px; padding: 10px;">
                ${renderFileTreeNode(tree, filename, baseDir, '')}
            </div>
        `;
    } else {
        html += `
            <div style="background: #0f172a; padding: 20px; border-radius: 6px; text-align: center; color: #94a3b8;">
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
                <div style="padding: 6px; cursor: pointer; font-family: monospace; font-size: 13px; color: #e2e8f0; border-bottom: 1px solid #1e293b;" onclick="toggleTreeNode('${id}')">
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
            <div style="margin-left: ${indent}; padding: 6px; border-bottom: 1px solid #1e293b; font-family: monospace; font-size: 13px; color: #e2e8f0;">
                📄 ${escapeHtml(file.name)} <span style="color: #94a3b8; font-size: 11px;">(${formatBytes(file.size)})</span>
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
        const res = await fetch(`${API_BASE}/images/files/delete`, {
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

async function saveImageProperties() {
    const filename = document.getElementById('image-props-filename').value;
    const displayName = document.getElementById('image-props-display-name').value;
    const description = document.getElementById('image-props-description').value;
    const groupId = document.getElementById('image-props-group').value;
    const order = parseInt(document.getElementById('image-props-order').value) || 0;
    const bootMethod = document.getElementById('image-props-boot-method').value;
    const bootParams = document.getElementById('image-props-boot-params').value;
    const enabled = document.getElementById('image-props-enabled').checked;
    const isPublic = document.getElementById('image-props-public').checked;

    const updates = {
        name: displayName,
        description: description,
        group_id: groupId ? parseInt(groupId) : null,
        order: order,
        boot_method: bootMethod,
        boot_params: bootParams,
        enabled: enabled,
        public: isPublic
    };

    try {
        const res = await fetch(`${API_BASE}/images?filename=${encodeURIComponent(filename)}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(updates)
        });

        const data = await res.json();

        if (data.success) {
            showNotification('Image properties updated successfully', 'success');
            closeModal('image-properties-modal');
            loadImages();
            loadStats();
        } else {
            showNotification('Failed to update image: ' + (data.error || 'Unknown error'), 'error');
        }
    } catch (err) {
        showNotification('Failed to update image properties', 'error');
        console.error(err);
    }
}
