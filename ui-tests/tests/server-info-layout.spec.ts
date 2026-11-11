import { expect, loginAsAdmin, test } from './helpers';

for (const width of [360, 768, 1280]) {
  test(`long TLS details stay inside the server information card at ${width}px`, async ({ page }) => {
    const details = `Enabled (custom certificate): /certificates/${'certificate'.repeat(40)}.pem; SHA-256 ${'AB:'.repeat(31)}AB`;
    await page.setViewportSize({ width, height: 900 });
    await page.route('**/api/server-info', async route => {
      const response = await route.fetch();
      const result = await response.json();
      result.data.configuration.admin_tls = details;
      await route.fulfill({ response, json: result });
    });
    await loginAsAdmin(page);
    const row = page.locator('#server-info .info-item').filter({ has: page.getByText('Admin Tls', { exact: true }) });
    await expect(row.locator('.info-value')).toHaveText(details);
    const geometry = await row.evaluate(element => {
      const label = element.querySelector('.info-label')!.getBoundingClientRect();
      const value = element.querySelector('.info-value')!;
      const bounds = value.getBoundingClientRect();
      const section = element.closest('.info-section')!.getBoundingClientRect();
      const grid = element.closest('.info-grid')!;
      return {
        overlap: label.right - bounds.left,
        overflow: value.scrollWidth - value.clientWidth,
        cardOverflow: bounds.right - section.right,
        gridOverflow: grid.scrollWidth - grid.clientWidth,
      };
    });
    expect(geometry.overlap).toBeLessThanOrEqual(0);
    expect(geometry.overflow).toBeLessThanOrEqual(1);
    expect(geometry.cardOverflow).toBeLessThanOrEqual(0);
    expect(geometry.gridOverflow).toBeLessThanOrEqual(1);
  });
}
