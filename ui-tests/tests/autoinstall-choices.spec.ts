import { expect, loginAsAdmin, loginAsViewer, openTab, randomMAC, test, uniqueSuffix } from './helpers';
import type { Page } from '@playwright/test';

async function api(page: Page, method: string, url: string, body?: unknown) {
  return page.evaluate(async ({ method, url, body }) => {
    const response = await (window as any).authFetch('/api/' + url, {
      method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body),
    });
    const data = await response.json();
    if (!data.success) throw new Error(data.error);
    return data.data;
  }, { method, url, body });
}

test('Windows answer-file choices persist and clear in image properties', async ({ page }) => {
  await loginAsAdmin(page);
  const suffix = uniqueSuffix();
  const files = [`desktop-${suffix}.xml`, `kiosk-${suffix}.xml`];
  for (const filename of files) await api(page, 'POST', 'autoinstall-files/save', { distro: 'windows', filename, content: '<unattend/>' });
  const filename = `answers-${suffix}.iso`;
  try {
    await openTab(page, 'images');
    await page.getByRole('button', { name: 'Upload ISO', exact: true }).click();
    await page.locator('#file-input').setInputFiles({ name: filename, mimeType: 'application/octet-stream', buffer: Buffer.alloc(4096) });
    const upload = page.waitForResponse(r => r.url().includes('/api/images/upload') && r.request().method() === 'POST');
    await page.locator('#upload-form button[type="submit"]').click();
    expect((await upload).ok()).toBeTruthy();
    await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
    await page.locator('#image-properties-modal').getByRole('button', { name: 'Auto-Install', exact: true }).click();
    await expect(page.locator('#image-props-answer-files')).toBeHidden();
    await page.getByRole('button', { name: 'Properties', exact: true }).click();
    await page.selectOption('#image-props-distro', 'windows');
    await page.locator('#image-properties-modal').getByRole('button', { name: 'Auto-Install', exact: true }).click();
    await expect(page.locator('#image-props-answer-files')).toBeVisible();
    for (const name of files) await page.getByRole('checkbox', { name: `windows/${name}`, exact: true }).check();
    await page.setViewportSize({ width: 420, height: 900 });
    expect(await page.locator('#image-props-answer-files').evaluate(e => e.scrollWidth <= e.clientWidth + 1)).toBeTruthy();
    const save = page.waitForResponse(r => r.url().includes('/api/images?filename=') && r.request().method() === 'PUT');
    await page.getByRole('button', { name: 'Save Properties', exact: true }).click();
    expect((await save).ok()).toBeTruthy();
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.reload();
    await openTab(page, 'images');
    await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
    await page.locator('#image-properties-modal').getByRole('button', { name: 'Auto-Install', exact: true }).click();
    for (const name of files) await expect(page.getByRole('checkbox', { name: `windows/${name}`, exact: true })).toBeChecked();
    await page.setViewportSize({ width: 420, height: 900 });
    await page.screenshot({ path: test.info().outputPath('answer-file-choices-mobile.png') });
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.evaluate(() => localStorage.clear());
    await loginAsViewer(page);
    await openTab(page, 'images');
    await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
    await expect(page.locator('#image-properties-modal [data-props-tab=autoinstall]')).toBeHidden();
    await page.evaluate(() => localStorage.clear());
    await loginAsAdmin(page);
    await openTab(page, 'images');
    await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
    await page.locator('#image-properties-modal').getByRole('button', { name: 'Auto-Install', exact: true }).click();
    for (const name of files) await page.getByRole('checkbox', { name: `windows/${name}`, exact: true }).uncheck();
    const clear = page.waitForResponse(r => r.url().includes('/api/images?filename=') && r.request().method() === 'PUT');
    await page.getByRole('button', { name: 'Save Properties', exact: true }).click();
    expect((await clear).ok()).toBeTruthy();
    expect((await api(page, 'GET', `images?filename=${filename}`)).auto_install_choices || []).toEqual([]);
  } finally {
    await api(page, 'DELETE', `images?filename=${filename}`);
    for (const name of files) await api(page, 'DELETE', `autoinstall-files/delete?distro=windows&filename=${name}`);
  }
});

test('client and group answer-file lists can be edited independently', async ({ page }) => {
  await loginAsAdmin(page);
  const suffix = uniqueSuffix();
  const filename = `lab-${suffix}.xml`;
  const mac = randomMAC();
  await api(page, 'POST', 'autoinstall-files/save', { distro: 'windows', filename, content: '<unattend/>' });
  await api(page, 'POST', 'clients', { mac_address: mac, name: `answers-${suffix}` });
  const group = await api(page, 'POST', 'client-groups', { name: `answers-${suffix}` });
  try {
    await page.evaluate(id => (window as any).showEditClientGroupModal(id), group.id);
    await page.locator('#cg-autoinstall-choices').getByRole('checkbox', { name: `windows/${filename}`, exact: true }).check();
    const save = page.waitForResponse(r => r.url().includes('/api/client-groups/update') && r.request().method() === 'PUT');
    await page.locator('#edit-client-group-form button[type="submit"]').click();
    expect((await save).ok()).toBeTruthy();
    expect((await api(page, 'GET', `client-groups/get?id=${group.id}`)).auto_install_choices).toEqual([`windows/${filename}`]);
    await page.evaluate(mac => (window as any).editClient(mac), mac);
    await page.locator('#edit-client-autoinstall-choices').getByRole('checkbox', { name: `windows/${filename}`, exact: true }).check();
    const clientSave = page.waitForResponse(r => r.url().includes('/api/clients?') && r.request().method() === 'PUT');
    await page.locator('#edit-client-form button[type="submit"]').click();
    expect((await clientSave).ok()).toBeTruthy();
    expect((await api(page, 'GET', `clients?mac=${mac}`)).auto_install_choices).toEqual([`windows/${filename}`]);
  } finally {
    await api(page, 'DELETE', `clients?mac=${mac}`);
    await api(page, 'DELETE', `client-groups/delete?id=${group.id}`);
    await api(page, 'DELETE', `autoinstall-files/delete?distro=windows&filename=${filename}`);
  }
});
