import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, test } from './helpers';

const markup = readFileSync(resolve(__dirname, '../../web/static/index.html'), 'utf8');
const styles = readFileSync(resolve(__dirname, '../../web/static/styles.css'), 'utf8');
const controls = [
  { id: 'theme-remove-boot-password', label: 'Remove the boot menu password' },
  { id: 'image-props-remove-boot-password', label: 'Remove the image password' },
];

for (const width of [360, 1280]) {
  for (const control of controls) {
    test(`${control.label} stays beside its checkbox at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 });
      await page.setContent(`<style>${styles}</style>`);
      await page.evaluate(({ markup, id }) => {
        const source = new DOMParser().parseFromString(markup, 'text/html');
        const field = source.getElementById(id)!.closest('.form-group')!;
        document.body.appendChild(document.importNode(field, true));
      }, { markup, id: control.id });

      const checkbox = page.getByRole('checkbox', { name: control.label, exact: true });
      await expect(checkbox).toBeVisible();
      const geometry = await checkbox.evaluate(element => {
        const bounds = element.getBoundingClientRect();
        const range = document.createRange();
        range.selectNodeContents(element.closest('label')!.lastChild!);
        const text = range.getBoundingClientRect();
        return {
          width: bounds.width,
          gap: text.left - bounds.right,
          overlap: Math.min(bounds.bottom, text.bottom) - Math.max(bounds.top, text.top),
        };
      });
      expect(geometry.width).toBeLessThan(24);
      expect(geometry.gap).toBeGreaterThanOrEqual(0);
      expect(geometry.gap).toBeLessThanOrEqual(16);
      expect(geometry.overlap).toBeGreaterThan(0);

      await page.getByText(control.label, { exact: true }).click();
      await expect(checkbox).toBeChecked();
      await page.getByText(control.label, { exact: true }).click();
      await expect(checkbox).not.toBeChecked();
    });
  }
}
