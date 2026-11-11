let imageDriversTimer = null;
let imageDriversRequest = 0;
let imageDriversBusy = false;

function visibleDriverImage() {
    const modal = document.getElementById('image-properties-modal');
    const tab = document.getElementById('image-props-drivers-tab');
    return modal.classList.contains('active') && tab.classList.contains('active') ? _imagePropsState?.img : null;
}

async function driverAPI(path, options) {
    const response = await authFetch(`${API_BASE}/drivers${path}`, options);
    const result = await response.json();
    if (!response.ok || !result.success) throw new Error(result.error || t('drivers.request_failed'));
    return result.data;
}

async function loadImageDrivers() {
    clearTimeout(imageDriversTimer);
    const image = visibleDriverImage();
    if (!image) return;
    const request = ++imageDriversRequest;
    try {
        const [packs, state] = await Promise.all([
            driverAPI(`?imageId=${image.id}`),
            driverAPI(`/status?imageId=${image.id}`),
        ]);
        if (request !== imageDriversRequest || visibleDriverImage()?.id !== image.id) return;
        renderImageDrivers(packs || [], state);
        if (state.status === 'applying') imageDriversTimer = setTimeout(loadImageDrivers, 1000);
    } catch (error) {
        if (request !== imageDriversRequest || visibleDriverImage()?.id !== image.id) return;
        document.getElementById('image-drivers-error').textContent = error.message;
        document.getElementById('image-drivers-apply').disabled = true;
    }
}

function renderImageDrivers(packs, state) {
    const busy = imageDriversBusy || state.status === 'applying';
    const statusKeys = { idle: 'drivers.idle', pending: 'drivers.pending', applying: 'drivers.applying', applied: 'drivers.applied', failed: 'drivers.failed' };
    document.getElementById('image-drivers-status').textContent = t(statusKeys[state.status] || 'drivers.idle');
    document.getElementById('image-drivers-error').textContent = state.error || '';
    document.getElementById('image-drivers-reason').textContent = state.reason || '';
    document.getElementById('image-drivers-loading-hint').textContent = state.smb_loading ? t('drivers.smb_preserved') : t('drivers.smb_hint');
    document.getElementById('image-driver-files').disabled = busy || isReadOnly();
    document.getElementById('image-drivers-upload').disabled = busy || isReadOnly();
    document.getElementById('image-drivers-apply').disabled = busy || isReadOnly() || !state.can_apply || !['pending', 'failed'].includes(state.status);
    const list = document.getElementById('image-drivers-list');
    list.replaceChildren();
    if (!packs.length) {
        const empty = document.createElement('p');
        empty.textContent = t('drivers.empty');
        list.append(empty);
    }
    for (const pack of packs) {
        const row = document.createElement('div');
        row.className = 'driver-pack';
        const details = document.createElement('div');
        details.className = 'driver-pack-details';
        const name = document.createElement('span');
        name.textContent = pack.original_name || pack.filename;
        const size = document.createElement('small');
        size.textContent = formatBytes(pack.size);
        details.append(name, size);
        row.append(details);
        if (!isReadOnly()) {
            const remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'btn btn-sm';
            remove.textContent = t('drivers.remove');
            remove.disabled = busy;
            remove.setAttribute('aria-label', t('drivers.remove_named', { name: name.textContent }));
            remove.onclick = () => removeImageDriver(pack);
            row.append(remove);
        }
        list.append(row);
    }
}

async function changeImageDrivers(action) {
    const image = visibleDriverImage();
    if (!image || imageDriversBusy || isReadOnly()) return;
    imageDriversBusy = true;
    clearTimeout(imageDriversTimer);
    ++imageDriversRequest;
    document.querySelectorAll('#props-drivers-content button, #props-drivers-content input').forEach(control => { control.disabled = true; });
    document.getElementById('image-drivers-error').textContent = '';
    let failure = '';
    try {
        await action(image);
    } catch (error) {
        failure = error.message;
    } finally {
        imageDriversBusy = false;
        if (visibleDriverImage()?.id === image.id) {
            await loadImageDrivers();
            if (failure) document.getElementById('image-drivers-error').textContent = failure;
        }
    }
}

async function uploadImageDrivers() {
    const input = document.getElementById('image-driver-files');
    const files = Array.from(input.files || []);
    if (!files.length) {
        document.getElementById('image-drivers-error').textContent = t('drivers.select_zip');
        return;
    }
    await changeImageDrivers(async image => {
        for (const file of files) {
            document.getElementById('image-drivers-status').textContent = t('drivers.uploading', { name: file.name });
            const body = new FormData();
            body.append('imageId', image.id);
            body.append('file', file);
            await driverAPI('/upload', { method: 'POST', body });
        }
        input.value = '';
    });
}

async function removeImageDriver(pack) {
    if (!confirm(t('drivers.remove_confirm', { name: pack.original_name || pack.filename }))) return;
    await changeImageDrivers(() => driverAPI(`/delete?id=${pack.id}`, { method: 'DELETE' }));
}

async function applyImageDrivers() {
    await changeImageDrivers(image => driverAPI(`/rebuild?imageId=${image.id}`, { method: 'POST' }));
}
