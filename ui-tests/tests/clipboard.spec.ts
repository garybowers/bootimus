import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import { expect, test } from './helpers';

const script = resolve(__dirname, '../../web/static/app.js');
const markup = readFileSync(resolve(__dirname, '../../web/static/index.html'), 'utf8')
  .replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, '')
  .replace(/<link\b[^>]*>/g, '');
const curl = "curl -H 'Authorization: Bearer $BOOTIMUS_TOKEN' http://bootimus.test/api/images";
const menu = '#!ipxe\nmenu Bootimus\nitem local Boot from local disk\n';
const token = 'test-token-+&%="\u00e4';
const codes = 'first-recovery-code\nsecond-recovery-code';
const fileURL = 'http://bootimus.test:8080/files/example.iso';
const buttons = [
  { id: 'download', handler: "copyFileDownloadURL('example.iso')", text: fileURL, message: 'Download URL copied to clipboard' },
  { id: 'file', handler: 'copyFileURL()', text: fileURL, message: 'URL copied to clipboard' },
  { id: 'curl', handler: 'copyAPIRefCurl(this)', text: curl },
  { id: 'menu', handler: 'copyMenuPreview()', text: menu, message: 'Boot menu script copied' },
  { id: 'token', handler: 'copyAPITokenSecret()', text: token, message: 'Token copied' },
  { id: 'codes', handler: 'copyTOTPRecoveryCodes()', text: codes, message: 'Recovery codes copied' },
];

async function loadCopyButtons(page: Page, origin = 'http://bootimus.test') {
  // A routed .test origin is genuinely insecure, unlike HTTP localhost. Load
  // the real app after DOMContentLoaded to isolate copying from server polling.
  await page.route(`${origin}/`, route => route.fulfill({ contentType: 'text/html', body: markup }));
  await page.goto(`${origin}/`);
  await page.addScriptTag({ path: script });
  await page.evaluate(({ buttons, curl, menu, token, codes, fileURL }) => {
    for (const child of Array.from(document.body.children)) (child as HTMLElement).hidden = true;
    const fixture = document.createElement('div');
    fixture.innerHTML = buttons.map(button =>
      `<div class="api-curl"><code></code><button id="copy-${button.id}" onclick="${button.handler}">${button.id}</button></div>`
    ).join('') + '<textarea id="paste-target" aria-label="Paste here"></textarea>';
    document.body.appendChild(fixture);
    fixture.querySelector('#copy-curl')!.parentElement!.querySelector('code')!.textContent = curl;
    document.getElementById('edit-file-url')!.textContent = fileURL;
    document.getElementById('api-token-secret-value')!.textContent = token;
    document.getElementById('totp-recovery-codes')!.textContent = codes;
    // menuPreviewData is a top-level lexical binding, not a window property.
    const setup = document.createElement('script');
    setup.textContent = `menuPreviewData = ${JSON.stringify({ script: menu })};`;
    document.body.appendChild(setup);
  }, { buttons, curl, menu, token, codes, fileURL });
}

async function expectPasted(page: Page, text: string) {
  const target = page.locator('#paste-target');
  await target.fill('');
  await target.press('ControlOrMeta+V');
  await expect(target).toHaveValue(text);
}

for (const button of buttons) {
  test(`${button.id} copies over plain HTTP`, async ({ page }) => {
    await loadCopyButtons(page);
    expect(await page.evaluate(() => window.isSecureContext)).toBe(false);
    expect(await page.evaluate(() => typeof navigator.clipboard)).toBe('undefined');
    const textareas = await page.locator('textarea').count();
    const copy = page.locator(`#copy-${button.id}`);
    await copy.click();
    if (button.message) await expect(page.getByText(button.message, { exact: true })).toBeVisible();
    else await expect(copy).toHaveClass('copied');
    await expect(copy).toBeFocused();
    await expect(page.locator('textarea')).toHaveCount(textareas);
    await expectPasted(page, button.text);
  });
}

test('HTTPS uses the Clipboard API without the fallback', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: 'https://bootimus.test' });
  await loadCopyButtons(page, 'https://bootimus.test');
  expect(await page.evaluate(() => window.isSecureContext)).toBe(true);
  await page.evaluate(() => {
    document.execCommand = () => { throw new Error('Unexpected fallback'); };
  });
  await page.locator('#copy-token').click();
  await expect(page.getByText('Token copied', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(token);
});

test('a rejected Clipboard API write falls back to copying', async ({ page }) => {
  await loadCopyButtons(page, 'https://bootimus.test');
  await page.evaluate(() => {
    navigator.clipboard.writeText = async () => { throw new DOMException('Denied', 'NotAllowedError'); };
  });
  await page.locator('#copy-token').click();
  await expect(page.getByText('Token copied', { exact: true })).toBeVisible();
  await expectPasted(page, token);
});

for (const failure of ['false', 'throw']) {
  test(`failed fallback (${failure}) reports errors and restores focus`, async ({ page }) => {
    await loadCopyButtons(page);
    await page.evaluate(failure => {
      document.execCommand = () => {
        if (failure === 'throw') throw new Error('Copy blocked');
        return false;
      };
    }, failure);
    const textareas = await page.locator('textarea').count();
    for (const button of buttons) {
      const copy = page.locator(`#copy-${button.id}`);
      await copy.click();
      const error = page.getByText(/Failed to copy|Copy failed/).last();
      await expect(error).toBeVisible();
      await expect(copy).toBeFocused();
      await expect(page.locator('textarea')).toHaveCount(textareas);
      await expect(copy).not.toHaveClass('copied');
      await expect(page.getByText(/copied/i)).toHaveCount(0);
      await error.evaluate(element => element.remove());
    }
  });
}

test('fallback restores the existing text selection', async ({ page }) => {
  await loadCopyButtons(page);
  await page.locator('#copy-curl').focus();
  await page.evaluate(() => {
    const range = document.createRange();
    range.selectNodeContents(document.querySelector('#copy-curl')!.parentElement!.querySelector('code')!);
    window.getSelection()!.removeAllRanges();
    window.getSelection()!.addRange(range);
  });
  await page.locator('#copy-curl').press('Enter');
  await expect(page.locator('#copy-curl')).toHaveClass('copied');
  expect(await page.evaluate(() => window.getSelection()!.toString())).toBe(curl);
  await expect(page.locator('#copy-curl')).toBeFocused();
});
