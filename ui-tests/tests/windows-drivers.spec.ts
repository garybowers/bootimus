import { expect, loginAsAdmin, loginAsViewer, openTab, test, uniqueSuffix } from './helpers';
import type { Page } from '@playwright/test';

// A ZIP with an INF and a companion file; deliberately not a working driver.
const driverZIP = Buffer.from('UEsDBBQAAAAAAPdoaF1CG82FIwAAACMAAAAPAAAAbmV0d29yay9uaWMuaW5mW1ZlcnNpb25dClNpZ25hdHVyZT0iJFdpbmRvd3MgTlQkIgpQSwMEFAAAAAAA92hoXejyGO0dAAAAHQAAAA8AAABuZXR3b3JrL25pYy5zeXNVSSB0ZXN0IGZpeHR1cmUsIG5vdCBhIGRyaXZlclBLAQIUAxQAAAAAAPdoaF1CG82FIwAAACMAAAAPAAAAAAAAAAAAAACAAQAAAABuZXR3b3JrL25pYy5pbmZQSwECFAMUAAAAAAD3aGhd6PIY7R0AAAAdAAAADwAAAAAAAAAAAAAAgAFQAAAAbmV0d29yay9uaWMuc3lzUEsFBgAAAAACAAIAegAAAJoAAAAAAA==', 'base64');

async function windowsImage(page: Page): Promise<string> {
  await loginAsAdmin(page);
  await openTab(page, 'images');
  const filename = `drivers-${uniqueSuffix()}.iso`;
  await page.getByRole('button', { name: 'Upload ISO', exact: true }).click();
  await page.locator('#file-input').setInputFiles({ name: filename, mimeType: 'application/octet-stream', buffer: Buffer.alloc(4096) });
  const upload = page.waitForResponse(response => response.url().includes('/api/images/upload') && response.request().method() === 'POST');
  await page.locator('#upload-form button[type="submit"]').click();
  expect((await upload).ok()).toBeTruthy();
  await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
  await expect(page.locator('#image-props-drivers-tab')).toBeHidden();
  await page.selectOption('#image-props-distro', 'windows');
  const saved = page.waitForResponse(response => response.url().includes('/api/images?filename=') && response.request().method() === 'PUT');
  await page.getByRole('button', { name: 'Save Properties', exact: true }).click();
  expect((await saved).ok()).toBeTruthy();
  await page.locator('#image-properties-modal').getByRole('button', { name: 'Cancel', exact: true }).click();
  await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
  await page.getByRole('button', { name: 'Drivers', exact: true }).click();
  await expect(page.locator('#image-drivers-status')).toHaveText('No driver changes');
  return filename;
}

test('Windows driver packs upload, persist and require applying removals', async ({ page }) => {
  const filename = await windowsImage(page);
  await page.setViewportSize({ width: 420, height: 900 });
  await expect(page.locator('#image-drivers-reason')).toContainText('Extract this Windows image');
  await page.locator('#image-driver-files').setInputFiles({ name: 'broken.zip', mimeType: 'application/zip', buffer: Buffer.from('invalid') });
  await page.getByRole('button', { name: 'Upload packs', exact: true }).click();
  await expect(page.locator('#image-drivers-error')).toContainText('invalid ZIP');
  const name = 'network-<driver>.zip';
  await page.locator('#image-driver-files').setInputFiles({ name, mimeType: 'application/zip', buffer: driverZIP });
  await page.getByRole('button', { name: 'Upload packs', exact: true }).click();
  await expect(page.locator('#image-drivers-status')).toHaveText('Pending changes');
  await expect(page.locator('#image-drivers-list')).toContainText(name);
  await expect(page.getByRole('button', { name: 'Apply changes', exact: true })).toBeDisabled();
  expect(await page.locator('#props-drivers-content').evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBeTruthy();
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.reload();
  await openTab(page, 'images');
  await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
  await page.getByRole('button', { name: 'Drivers', exact: true }).click();
  await expect(page.locator('#image-drivers-status')).toHaveText('Pending changes');
  await page.getByRole('button', { name: `Remove ${name}`, exact: true }).click();
  await expect(page.locator('#image-drivers-list')).toContainText('No driver packs uploaded.');
  await expect(page.locator('#image-drivers-status')).toHaveText('Pending changes');
  await page.locator('#image-props-delete-btn').click();
  await expect(page.locator('#images-table')).not.toContainText(filename);
});

test('driver build progress and errors remain visible without another upload', async ({ page }) => {
  const filename = await windowsImage(page);
  let status = 'pending';
  let polls = 0;
  await page.route('**/api/drivers/status?*', async route => {
    if (status === 'applying' && ++polls >= 2) status = 'applied';
    await route.fulfill({ json: { success: true, data: { status, can_apply: status !== 'applying', smb_loading: true, error: status === 'failed' ? 'Re-extract this image: original backup missing.' : '' } } });
  });
  await page.route('**/api/drivers/rebuild?*', async route => {
    expect(route.request().method()).toBe('POST');
    status = 'applying';
    await route.fulfill({ status: 202, json: { success: true, data: { status } } });
  });
  await page.getByRole('button', { name: 'Properties', exact: true }).click();
  await page.getByRole('button', { name: 'Drivers', exact: true }).click();
  await page.getByRole('button', { name: 'Apply changes', exact: true }).click();
  await expect(page.locator('#image-drivers-status')).toContainText('Applying');
  await expect(page.getByRole('button', { name: 'Upload packs', exact: true })).toBeDisabled();
  await expect(page.locator('#image-drivers-status')).toHaveText('Applied');
  status = 'failed';
  await page.getByRole('button', { name: 'Properties', exact: true }).click();
  await page.getByRole('button', { name: 'Drivers', exact: true }).click();
  await expect(page.locator('#image-drivers-error')).toContainText('original backup missing');
  await expect(page.getByRole('button', { name: 'Apply changes', exact: true })).toBeEnabled();
  await page.locator('#image-props-delete-btn').click();
  await expect(page.locator('#images-table')).not.toContainText(filename);
});

test('read-only users can inspect driver packs without changing them', async ({ page }) => {
  const filename = await windowsImage(page);
  await page.evaluate(() => localStorage.clear());
  await loginAsViewer(page);
  await openTab(page, 'images');
  await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
  await page.getByRole('button', { name: 'Drivers', exact: true }).click();
  await expect(page.locator('#image-drivers-status')).toHaveText('No driver changes');
  await expect(page.getByRole('button', { name: 'Upload packs', exact: true })).toBeHidden();
  await expect(page.getByRole('button', { name: 'Apply changes', exact: true })).toBeHidden();
  await page.evaluate(() => localStorage.clear());
  await loginAsAdmin(page);
  await openTab(page, 'images');
  await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
  await page.locator('#image-props-delete-btn').click();
});
