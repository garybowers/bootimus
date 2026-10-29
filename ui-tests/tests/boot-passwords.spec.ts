import { expect, loginAsAdmin, openTab, test, uniqueSuffix } from './helpers';

test.use({ ignoreHTTPSErrors: true });

test('boot password fields explain their scope and preserve an unset password', async ({ page }) => {
  await loginAsAdmin(page);
  await openTab(page, 'boot-menu');
  await expect(page.locator('#theme-boot-password')).toHaveAttribute('type', 'password');
  await expect(page.locator('#theme-boot-password')).toHaveValue('');
  await expect(page.locator('#theme-boot-password-status')).toHaveText('No password set.');
  await expect(page.locator('#theme-form')).toContainText('image downloads are not protected');
  await expect(page.locator('#theme-form')).toContainText('No +, ; or spaces');
  const requestPromise = page.waitForRequest(request => request.url().endsWith('/api/theme') && request.method() === 'PUT');
  await page.locator('#theme-form button[type="submit"]').click();
  const request = await requestPromise;
  expect(request.postDataJSON()).not.toHaveProperty('boot_password');
});

test('HTTP cannot enable a boot password', async ({ page, baseURL }) => {
  test.skip(baseURL?.startsWith('https:') === true, 'This check needs the HTTP admin listener');
  await loginAsAdmin(page);
  await openTab(page, 'boot-menu');
  await page.fill('#theme-boot-password', 'disposable-password');
  const responsePromise = page.waitForResponse(response => response.url().endsWith('/api/theme') && response.request().method() === 'PUT');
  await page.locator('#theme-form button[type="submit"]').click();
  const response = await responsePromise;
  expect(response.status()).toBe(400);
  expect((await response.json()).error).toContain('HTTPS');
  await page.reload();
  await openTab(page, 'boot-menu');
  await expect(page.locator('#theme-boot-password-status')).toHaveText('No password set.');
});

test('HTTPS can set, preserve, rotate and remove the menu password', async ({ page, baseURL }) => {
  test.skip(!baseURL?.startsWith('https:'), 'This check needs the HTTPS admin listener');
  await loginAsAdmin(page);
  await openTab(page, 'boot-menu');
  for (const password of ['menu+word', 'menu;word', 'menu word']) {
    await page.fill('#theme-boot-password', password);
    await page.locator('#theme-form button[type="submit"]').click();
    await expect(page.getByText('Boot passwords must not contain +, ; or spaces (iPXE v2.0.0 compatibility)', { exact: true }).last()).toBeVisible();
    await expect(page.locator('#theme-boot-password-status')).toHaveText('No password set.');
  }
  for (const password of ['menu&=%', undefined, 'changed-menu', '']) {
    if (password === '') await page.check('#theme-remove-boot-password');
    else if (password !== undefined) await page.fill('#theme-boot-password', password);
    const responsePromise = page.waitForResponse(response => response.url().endsWith('/api/theme') && response.request().method() === 'PUT');
    await page.locator('#theme-form button[type="submit"]').click();
    const response = await responsePromise;
    expect(response.status()).toBe(200);
    const result = (await response.json()).data;
    expect(result.boot_password_required).toBe(password !== '');
    expect(result).not.toHaveProperty('boot_password');
    expect(result).not.toHaveProperty('boot_password_hash');
    if (password === undefined) expect(response.request().postDataJSON()).not.toHaveProperty('boot_password');
    await expect(page.locator('#theme-boot-password-status')).toHaveText(password === '' ? 'No password set.' : 'Password set.');
    await expect(page.locator('#theme-boot-password')).toHaveValue('');
  }
});

test('HTTPS image password changes survive reopening properties', async ({ page, baseURL }) => {
  test.skip(!baseURL?.startsWith('https:'), 'This check needs the HTTPS admin listener');
  await loginAsAdmin(page);
  await openTab(page, 'images');
  const filename = `password-test-${uniqueSuffix()}.iso`;
  await page.getByRole('button', { name: 'Upload ISO', exact: true }).click();
  await page.locator('#file-input').setInputFiles({ name: filename, mimeType: 'application/octet-stream', buffer: Buffer.alloc(4096) });
  const upload = page.waitForResponse(response => response.url().includes('/api/images/upload') && response.request().method() === 'POST');
  await page.locator('#upload-form button[type="submit"]').click();
  expect((await upload).ok()).toBeTruthy();
  await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
  await expect(page.locator('#image-properties-modal')).toContainText('No +, ; or spaces');
  for (const password of ['image+word', 'image;word', 'image word']) {
    await page.fill('#image-props-boot-password', password);
    await page.getByRole('button', { name: 'Save Properties', exact: true }).click();
    await expect(page.getByText('Boot passwords must not contain +, ; or spaces (iPXE v2.0.0 compatibility)', { exact: true }).last()).toBeVisible();
    await expect(page.locator('#image-props-boot-password-status')).toHaveText('No password set.');
  }
  await page.locator('#image-properties-modal').getByRole('button', { name: 'Cancel', exact: true }).click();
  for (const password of ['image&=%', undefined, 'changed-image', '']) {
    await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
    await expect(page.locator('#image-properties-modal')).toBeVisible();
    await expect(page.locator('#image-props-boot-password')).toHaveAttribute('type', 'password');
    await expect(page.locator('#image-props-boot-password')).toHaveValue('');
    if (password === '') await page.check('#image-props-remove-boot-password');
    else if (password !== undefined) await page.fill('#image-props-boot-password', password);
    const responsePromise = page.waitForResponse(response => response.url().includes('/api/images?filename=') && response.request().method() === 'PUT');
    await page.getByRole('button', { name: 'Save Properties', exact: true }).click();
    const response = await responsePromise;
    expect(response.status()).toBe(200);
    if (password === undefined) expect(response.request().postDataJSON()).not.toHaveProperty('boot_password');
    await expect(page.locator('#image-props-boot-password-status')).toHaveText(password === '' ? 'No password set.' : 'Password set.');
    await expect(page.locator('#image-props-boot-password')).toHaveValue('');
    await page.locator('#image-properties-modal').getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(page.locator('#image-properties-modal')).toBeHidden();
    await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
    await expect(page.locator('#image-props-boot-password-status')).toHaveText(password === '' ? 'No password set.' : 'Password set.');
    await expect(page.locator('#image-props-boot-password')).toHaveValue('');
    await page.locator('#image-properties-modal').getByRole('button', { name: 'Cancel', exact: true }).click();
  }
  await page.locator('#images-table tr[onclick]').getByText(filename, { exact: true }).click();
  await page.locator('#image-props-delete-btn').click();
  await expect(page.locator('#images-table')).not.toContainText(filename);
});
