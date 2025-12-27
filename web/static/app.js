// API Base URL
const API_BASE = '/api';

// State
let clients = [];
let images = [];
let currentClient = null;

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    setupTabs();
    setupForms();
    setupUpload();
    loadStats();
    loadClients();
    loadImages();
    loadLogs();

    // Refresh every 30 seconds
    setInterval(() => {
        loadStats();
        const activeTab = document.querySelector('.tab.active').dataset.tab;
        if (activeTab === 'clients') loadClients();
        if (activeTab === 'images') loadImages();
        if (activeTab === 'logs') loadLogs();
    }, 30000);
});

// Tab Management
function setupTabs() {
    document.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

            tab.classList.add('active');
            document.getElementById(`${tab.dataset.tab}-tab`).classList.add('active');
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
                        <td>${(client.images || []).length} images</td>
                        <td>${client.boot_count || 0}</td>
                        <td>${client.last_boot ? new Date(client.last_boot).toLocaleString() : 'Never'}</td>
                        <td>
                            <button class="btn btn-primary btn-sm" onclick="editClient(${client.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteClient(${client.id}, '${client.mac_address}')">Delete</button>
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

async function editClient(id) {
    try {
        const res = await fetch(`${API_BASE}/clients?id=${id}`);
        const data = await res.json();

        if (data.success) {
            currentClient = data.data;
            const form = document.getElementById('edit-client-form');
            form.id.value = currentClient.id;
            form.mac_address.value = currentClient.mac_address;
            form.name.value = currentClient.name || '';
            form.description.value = currentClient.description || '';
            form.enabled.checked = currentClient.enabled;

            // Populate images select
            const select = document.getElementById('edit-images-select');
            select.innerHTML = images.map(img => `
                <option value="${img.id}" ${(currentClient.images || []).some(i => i.id === img.id) ? 'selected' : ''}>
                    ${img.name}
                </option>
            `).join('');

            showModal('edit-client-modal');
        }
    } catch (err) {
        alert('Failed to load client');
    }
}

async function deleteClient(id, mac) {
    if (!confirm(`Delete client ${mac}?`)) return;

    try {
        const res = await fetch(`${API_BASE}/clients?id=${id}`, { method: 'DELETE' });
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
        const res = await fetch(`${API_BASE}/images`);
        const data = await res.json();

        if (data.success) {
            images = data.data || [];
            renderImagesTable();
        }
    } catch (err) {
        document.getElementById('images-table').innerHTML = '<p class="alert alert-error">Failed to load images</p>';
    }
}

function renderImagesTable() {
    const container = document.getElementById('images-table');

    if (images.length === 0) {
        container.innerHTML = '<p style="color: #94a3b8; padding: 20px;">No images yet. Upload or scan for ISOs.</p>';
        return;
    }

    const html = `
        <table>
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Filename</th>
                    <th>Size</th>
                    <th>Status</th>
                    <th>Visibility</th>
                    <th>Boot Count</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${images.map(img => `
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
                        <td>${img.boot_count || 0}</td>
                        <td>
                            <button class="btn btn-primary btn-sm" onclick="toggleImage('${img.filename}', ${img.enabled})">
                                ${img.enabled ? 'Disable' : 'Enable'}
                            </button>
                            <button class="btn btn-primary btn-sm" onclick="togglePublic('${img.filename}', ${img.public})">
                                ${img.public ? 'Make Private' : 'Make Public'}
                            </button>
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
        const id = formData.get('id');

        const updates = {
            name: formData.get('name'),
            description: formData.get('description'),
            enabled: formData.get('enabled') === 'on'
        };

        try {
            // Update client
            const res1 = await fetch(`${API_BASE}/clients?id=${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates)
            });

            // Update image assignments
            const selectedImages = Array.from(document.getElementById('edit-images-select').selectedOptions)
                .map(opt => parseInt(opt.value));

            const res2 = await fetch(`${API_BASE}/clients/assign`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    client_id: parseInt(id),
                    image_ids: selectedImages
                })
            });

            const data1 = await res1.json();
            const data2 = await res2.json();

            if (data1.success && data2.success) {
                showAlert('Client updated successfully', 'success');
                closeModal('edit-client-modal');
                loadClients();
            } else {
                showAlert('Failed to update client', 'error');
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
    const alertDiv = document.createElement('div');
    alertDiv.className = `alert alert-${type}`;
    alertDiv.textContent = message;

    document.querySelector('.container').insertBefore(alertDiv, document.querySelector('.stats'));

    setTimeout(() => alertDiv.remove(), 5000);
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}
