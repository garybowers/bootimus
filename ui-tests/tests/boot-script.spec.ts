import { expect, loginAsAdmin, openTab, test, uniqueSuffix } from './helpers';

test('a custom boot script persists and can be cleared', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'images');
  const filename = `scripted-${uniqueSuffix()}.iso`;
  await page.getByRole('button', { name: 'Upload ISO', exact: true }).click();
  await page.locator('#file-input').setInputFiles({ name: filename, mimeType: 'application/octet-stream', buffer: Buffer.alloc(4096) });
  const upload = page.waitForResponse(response => response.url().includes('/api/images/upload') && response.request().method() === 'POST');
  await page.locator('#upload-form button[type="submit"]').click();
  expect((await upload).ok()).toBeTruthy();
  const row = page.locator('#images-table tr[onclick]').getByText(filename, { exact: true });
  await row.click();
  const script = page.getByLabel('Custom Boot Script', { exact: true });
  await expect(script).toHaveValue('');
  await expect(page.locator('#image-properties-modal')).toContainText('Replaces the generated boot commands for this image');

  const commands = 'kernel {{BASE_URL}}/wimboot\ninitrd {{BASE_URL}}/boot/{{CACHE_DIR}}/iso/sources/boot.wim boot.wim\nboot || goto failed';
  for (const value of [commands, '']) {
    await script.fill(value);
    const saved = page.waitForResponse(response => response.url().includes('/api/images?filename=') && response.request().method() === 'PUT');
    await page.getByRole('button', { name: 'Save Properties', exact: true }).click();
    const response = await saved;
    expect(response.ok()).toBeTruthy();
    expect(response.request().postDataJSON().boot_script).toBe(value);
    await page.locator('#image-properties-modal').getByRole('button', { name: 'Cancel', exact: true }).click();
    await row.click();
    await expect(script).toHaveValue(value);
  }
  await page.locator('#image-props-delete-btn').click();
  await expect(page.locator('#images-table')).not.toContainText(filename);
});
