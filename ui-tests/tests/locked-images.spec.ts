import { expect, loginAsAdmin, openTab, test, uniqueSuffix } from './helpers';

test('an image exception is opt-in and preserves its own password', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'images');
  const filename = `kiosk-${uniqueSuffix()}.iso`;
  await page.getByRole('button', { name: 'Upload ISO', exact: true }).click();
  await page.locator('#file-input').setInputFiles({ name: filename, mimeType: 'application/octet-stream', buffer: Buffer.alloc(4096) });
  const upload = page.waitForResponse(response => response.url().includes('/api/images/upload') && response.request().method() === 'POST');
  await page.locator('#upload-form button[type="submit"]').click();
  expect((await upload).ok()).toBeTruthy();
  const row = page.locator('#images-table tr[onclick]').getByText(filename, { exact: true });
  await row.click();
  const toggle = page.getByRole('checkbox', { name: 'Show on locked boot menu', exact: true });
  await expect(toggle).not.toBeChecked();
  await expect(page.locator('#image-properties-modal')).toContainText('Its own image password and client/profile visibility still apply');
  await page.fill('#image-props-boot-password', 'kiosk-test');
  for (const exempt of [true, false]) {
    await toggle.setChecked(exempt);
    const saved = page.waitForResponse(response => response.url().includes('/api/images?filename=') && response.request().method() === 'PUT');
    await page.getByRole('button', { name: 'Save Properties', exact: true }).click();
    const response = await saved;
    expect(response.ok()).toBeTruthy();
    expect(response.request().postDataJSON().boot_menu_exempt).toBe(exempt);
    await expect(page.locator('#image-props-boot-password-status')).toHaveText('Password set.');
    await page.locator('#image-properties-modal').getByRole('button', { name: 'Cancel', exact: true }).click();
    await row.click();
    await expect(toggle).toBeChecked({ checked: exempt });
    await expect(page.locator('#image-props-boot-password-status')).toHaveText('Password set.');
  }
  await page.locator('#image-props-delete-btn').click();
  await expect(page.locator('#images-table')).not.toContainText(filename);
});
