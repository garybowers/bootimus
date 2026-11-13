import { expect, loginAsAdmin, openTab, randomMAC, test } from './helpers';

test('native TFTP file can be created, edited and cleared', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'clients');
  const mac = randomMAC();
  const file = `${mac.replaceAll(':', '')}/kernel7.img`;
  await page.getByRole('button', { name: 'Add Client' }).click();
  await page.locator('#add-client-form [name="mac_address"]').fill(mac);
  await page.locator('#add-client-modal').getByLabel('Native TFTP boot file (optional)').fill(file);
  await page.locator('#add-client-form button[type="submit"]').click();
  await expect(page.locator('#add-client-modal')).toBeHidden();
  await page.locator('#clients-table tr.row-clickable', { hasText: mac }).first().click();
  await expect(page.locator('#client-detail-overview')).toContainText(file);
  for (const next of [file.replace('kernel7.img', 'custom.bin'), '']) {
    await page.locator('#client-detail-actions').getByRole('button', { name: 'Edit', exact: true }).click();
    const field = page.locator('#edit-client-modal').getByLabel('Native TFTP boot file (optional)');
    await expect(field).toHaveValue(next ? file : file.replace('kernel7.img', 'custom.bin'));
    await field.fill(next);
    await page.locator('#edit-client-form button[type="submit"]').click();
    await expect(page.locator('#edit-client-modal')).toBeHidden();
    await expect(page.locator('#client-detail-overview')).toContainText(next || 'Disabled');
  }
});
