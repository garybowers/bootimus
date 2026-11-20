import { expect, loginAsAdmin, openClientsSubtab, test, uniqueSuffix } from './helpers';

test('a group can omit option 43 and restore the default', async ({ page }) => {
  await loginAsAdmin(page);
  await openClientsSubtab(page, 'client-groups');
  await page.getByRole('button', { name: 'Add Client Group' }).click();
  const add = page.locator('#add-client-group-form');
  const name = `firmware-${uniqueSuffix()}`;
  const label = 'Omit PXE vendor option (43) from proxyDHCP offers';
  await add.locator('[name="name"]').fill(name);
  await expect(add.getByLabel(label)).not.toBeChecked();
  await add.getByLabel(label).check();
  await add.getByRole('button', { name: 'Create Group' }).click();

  const edit = page.locator('#edit-client-group-form');
  await expect(edit.getByLabel(label)).toBeChecked();
  await edit.getByLabel(label).uncheck();
  await edit.getByRole('button', { name: 'Save Group' }).click();
  await expect(edit).toBeHidden();
  await page.locator('#client-groups-table').getByText(name, { exact: true }).click();
  await expect(edit.getByLabel(label)).toBeVisible();
  await expect(edit.getByLabel(label)).not.toBeChecked();
});
