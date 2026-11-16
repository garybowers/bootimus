import { expect, loginAsAdmin, openTab, test, uniqueSuffix } from './helpers';
import type { Page } from '@playwright/test';

async function api(page: Page, method: string, path: string, body?: unknown) {
  return page.evaluate(async ({ method, path, body }) => {
    const response = await (window as any).authFetch('/api/' + path, {
      method, headers: { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const result = await response.json();
    if (!result.success) throw new Error(result.error);
    return result.data;
  }, { method, path, body });
}

for (const scenario of ['same folder', 'different folder', 'missing file', 'cleared assignment', 'file list unavailable', 'save rejected']) {
  test(`Save & Re-patch preserves the answer file: ${scenario}`, async ({ page }) => {
    await loginAsAdmin(page);
    const suffix = uniqueSuffix();
    const filename = `repatch-${suffix}.iso`;
    const answer = `answer-${suffix}.xml`;
    const distro = scenario === 'different folder' ? 'windows7' : 'windows';
    const path = `${distro}/${answer}`;
    await api(page, 'POST', 'autoinstall-files/save', {
      distro, filename: answer, content: '<unattend><ComputerName>{{HOSTNAME}}</ComputerName></unattend>',
    });
    await openTab(page, 'images');
    await page.getByRole('button', { name: 'Upload ISO', exact: true }).click();
    await page.locator('#file-input').setInputFiles({ name: filename, mimeType: 'application/octet-stream', buffer: Buffer.alloc(4096) });
    const uploaded = page.waitForResponse(r => r.url().includes('/api/images/upload') && r.request().method() === 'POST');
    await page.locator('#upload-form button[type="submit"]').click();
    expect((await uploaded).ok()).toBeTruthy();
    await api(page, 'PUT', `images?filename=${filename}`, { distro: 'windows', boot_method: 'kernel', auto_install_file: path });
    if (scenario === 'missing file') await api(page, 'DELETE', `autoinstall-files/delete?distro=${distro}&filename=${answer}`);
    let patches = 0;
    await page.route('**/api/images/patch-smb?*', async route => {
      patches++;
      await route.fulfill({ json: { success: true } });
    });
    if (scenario === 'file list unavailable') {
      let lists = 0;
      await page.route('**/api/autoinstall-files', async route => {
        if (lists++ === 0) await route.fulfill({ status: 503, json: { success: false, error: 'Library unavailable' } });
        else await route.continue();
      });
    }
    if (scenario === 'save rejected') {
      await page.route('**/api/images?filename=*', async route => {
        if (route.request().method() === 'PUT') {
          await route.fulfill({ status: 400, json: { success: false, error: 'Save rejected' } });
        } else await route.continue();
      });
    }
    try {
      await page.evaluate(async ({ filename, clear }) => {
        await (window as any).loadImages();
        await (window as any).showImagePropertiesModal(filename);
        if (clear) (document.getElementById('image-props-autoinstall-file') as HTMLSelectElement).value = '';
        await (window as any).saveAndRepatchFromProperties();
      }, { filename, clear: scenario === 'cleared assignment' });
      const image = await api(page, 'GET', `images?filename=${filename}`);
      expect(image.auto_install_file || '').toBe(scenario === 'cleared assignment' ? '' : path);
      expect(image.auto_install_enabled).toBe(scenario !== 'cleared assignment');
      expect(patches).toBe(['file list unavailable', 'save rejected'].includes(scenario) ? 0 : 1);
    } finally {
      await page.unrouteAll({ behavior: 'wait' });
      await api(page, 'DELETE', `images?filename=${filename}`);
      await api(page, 'DELETE', `autoinstall-files/delete?distro=${distro}&filename=${answer}`);
    }
  });
}
