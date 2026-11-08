import { expect, loginAsAdmin, openTab, test } from './helpers';

test('locked-menu local boot is opt-in and persists independently', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'boot-menu');
  const toggle = page.getByRole('checkbox', { name: 'Default to local disk on the locked menu' });
  const timeout = page.getByLabel('Locked menu timeout (seconds)');
  await expect(toggle).not.toBeChecked();
  await expect(timeout).toBeDisabled();
  for (const value of ['12', '0']) {
    await toggle.check();
    await timeout.fill(value);
    const response = page.waitForResponse(result => result.url().endsWith('/api/theme') && result.request().method() === 'PUT');
    await page.locator('#theme-form button[type="submit"]').click();
    expect((await response).ok()).toBeTruthy();
    await page.reload();
    await openTab(page, 'boot-menu');
    await expect(toggle).toBeChecked();
    await expect(timeout).toHaveValue(value);
    await expect(page.locator('#theme-timeout')).toHaveValue('30');
  }
  await toggle.uncheck();
  const response = page.waitForResponse(result => result.url().endsWith('/api/theme') && result.request().method() === 'PUT');
  await page.locator('#theme-form button[type="submit"]').click();
  expect((await response).ok()).toBeTruthy();
  await expect(timeout).toBeDisabled();
});
