import { expect, test, type Page } from '@playwright/test';
import type { ReadingPosition } from '../src/readingState';
import { silentWave, twoPagePDF } from './fixtures';

// API responses are mocked; the production bundle, PDF worker, local resources,
// canvas rendering, DOM safety, downloads and focus behavior are real Chromium.
const policy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; font-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'";

async function openFixture(page: Page, title: string, filename: string, body: Buffer | string, kind?: string, mediaType = 'application/octet-stream') {
  const posts: string[] = [];
  page.on('request', request => { if (request.method() === 'POST') posts.push(request.url()); });
  await page.addInitScript(() => {
    // Exercise the browser/Wails anchor-download fallback, not a native OS picker.
    Object.defineProperty(window, 'showSaveFilePicker', { value: undefined });
  });
  await page.route('http://127.0.0.1:4173/', async route => {
    const response = await route.fetch();
    await route.fulfill({ response, headers: { ...response.headers(), 'Content-Security-Policy': policy } });
  });
  await page.route('**/auth/me', route => route.fulfill({ json: {
    authenticated: true, user: { username: 'reader', admin: false, permissions: { upload: false, edit: false, delete: false } }
  } }));
  await page.route('**/data/search?*', route => route.fulfill({ json: {
    results: [{
      Uuid: '6d496688-7027-44ba-8ee5-74a27dac573b', Title: title, OriginalFilename: filename,
      Author: 'Viewer tests', PublishDate: '2026-01-01', LastUpdated: '2026-01-01',
      Path: 'stored-original', FileType: '.pdf', DocType: 'Book', DeweyDecimal: '000'
    }], has_next: false, count: 1
  } }));
  let readingState = { source_revision: 'test-source', version: 0, position: null as ReadingPosition | null };
  await page.route('**/file/reading-state/*', async route => {
    if (route.request().method() === 'PUT') {
      const update = route.request().postDataJSON();
      if (update.expected_version !== readingState.version) {
        await route.fulfill({ status: 409, json: { error: 'reading position changed in another window' } });
        return;
      }
      readingState = { source_revision: 'test-source', version: readingState.version + 1, position: update.position };
    }
    await route.fulfill({ json: readingState });
  });
  await page.route('**/file/preview/*', route => kind
    ? route.fulfill({ body, headers: { 'Content-Type': mediaType, 'X-Scriptorium-Preview-Kind': kind, 'X-Content-Type-Options': 'nosniff' } })
    : route.fulfill({ status: 415, json: { error: 'This format has no safe built-in preview. Download the original instead.' } }));
  await page.route('**/file/download/*', route => route.fulfill({ body, contentType: mediaType }));
  await page.goto('/');
  await page.getByText(title, { exact: true }).click();
  await page.getByRole('button', { name: 'Open', exact: true }).click();
  return posts;
}

async function expectPageInk(page: Page) {
  await expect.poll(() => page.locator('canvas').evaluate(canvas => {
    const context = (canvas as HTMLCanvasElement).getContext('2d');
    if (!context) return 0;
    const { width, height } = canvas as HTMLCanvasElement;
    const pixels = context.getImageData(0, 0, width, height).data;
    let ink = 0;
    for (let i = 0; i < pixels.length; i += 4) if (pixels[i + 3] > 0 && pixels[i] < 160 && pixels[i + 1] < 160 && pixels[i + 2] < 160) ink++;
    return ink;
  })).toBeGreaterThan(100);
}

test('renders and navigates a real PDF with local fonts while leaving scripts and links inert', async ({ page }) => {
  const dialogs: string[] = [];
  const external: string[] = [];
  const resources: string[] = [];
  page.on('dialog', dialog => { dialogs.push(dialog.message()); void dialog.dismiss(); });
  page.on('request', request => {
    if (request.url().includes('example.invalid')) external.push(request.url());
    if (request.url().includes('/pdfjs/')) resources.push(request.url());
  });
  const posts = await openFixture(page, 'PDF smoke fixture', 'original.pdf', twoPagePDF(), 'pdf', 'application/pdf');
  await expectPageInk(page);
  await expect(page.getByRole('button', { name: 'Show page text' })).toBeEnabled();
  await page.getByRole('button', { name: 'Show page text' }).click();
  await expect(page.getByLabel('Selectable page text')).toContainText('FirstViewerPhrase');
  await page.getByRole('button', { name: 'Next page' }).click();
  await expectPageInk(page);
  await expect(page.getByRole('img', { name: 'PDF page 2', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Show page text' }).click();
  await expect(page.getByLabel('Selectable page text')).toContainText('SecondViewerPhrase');
  await page.getByRole('button', { name: 'Zoom in' }).click();
  await expectPageInk(page);
  await expect(page.getByText('125%', { exact: true })).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
  expect(resources.some(url => url.includes('/standard_fonts/'))).toBe(true);
  expect(dialogs).toEqual([]);
  expect(external).toEqual([]);
  expect(posts).toEqual([]);
  expect(page.workers().length).toBeGreaterThan(0);
  await expect(page.getByRole('button', { name: 'Show page text' })).toBeEnabled();
  await page.getByRole('img', { name: 'PDF page 2', exact: true }).evaluate(viewport => {
    viewport.scrollTop = .4 * (viewport.scrollHeight - viewport.clientHeight);
    viewport.dispatchEvent(new Event('scroll'));
  });
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: 'Open', exact: true })).toBeFocused();
  await expect.poll(() => page.workers().length).toBe(0);
  await page.getByRole('button', { name: 'Open', exact: true }).click();
  await expect(page.getByRole('img', { name: 'PDF page 2', exact: true })).toBeVisible();
  await expectPageInk(page);
  await expect(page.getByText('125%', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Show page text' })).toBeEnabled();
  await expect.poll(() => page.getByRole('img', { name: 'PDF page 2', exact: true }).evaluate(viewport =>
    viewport.scrollTop / (viewport.scrollHeight - viewport.clientHeight)
  )).toBeCloseTo(.4, 1);
});

test('shows uploaded markup as inert source rather than executing it', async ({ page }) => {
  const source = '<script>window.__previewExecuted = true</script><svg onload="window.__previewExecuted = true"></svg>';
  await openFixture(page, 'Markup smoke fixture', 'source.svg', source, 'text', 'text/plain; charset=utf-8');
  await expect(page.getByRole('textbox', { name: 'Document text' })).toHaveValue(source);
  expect(await page.evaluate(() => (window as Window & { __previewExecuted?: boolean }).__previewExecuted)).toBeUndefined();
  await expect(page.getByRole('dialog').locator('script, svg, iframe, object, embed')).toHaveCount(0);
});

test('reads beyond the old text prefix limit and restores the final section', async ({ page }) => {
  const source = 'Full document text '.repeat(65000) + 'EntireDocumentEnding';
  await openFixture(page, 'Complete text fixture', 'original.txt', source, 'text', 'text/plain');
  const section = page.getByRole('spinbutton', { name: 'Section' });
  const last = await section.getAttribute('max');
  await section.fill(last!);
  await page.getByRole('button', { name: 'Go', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Document text' })).toHaveValue(/EntireDocumentEnding$/);
  await page.getByRole('button', { name: 'Close viewer' }).click();
  await page.getByRole('button', { name: 'Open', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Document text' })).toHaveValue(/EntireDocumentEnding$/);
  await expect(page.getByRole('spinbutton', { name: 'Section' })).toHaveValue(last!);
});

test('decodes a raster image through the native image element', async ({ page }) => {
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aH1cAAAAASUVORK5CYII=', 'base64');
  await openFixture(page, 'Image smoke fixture', 'original.png', png, 'image', 'image/png');
  await expect.poll(() => page.getByRole('img', { name: 'Image smoke fixture' }).evaluate(image => (image as HTMLImageElement).naturalWidth)).toBe(1);
});

test('loads audio metadata using native browser controls', async ({ page }) => {
  await openFixture(page, 'Audio smoke fixture', 'original.wav', silentWave(), 'audio', 'audio/wave');
  await expect.poll(() => page.locator('audio').evaluate(audio => audio.duration)).toBe(1);
  await expect(page.locator('audio')).toHaveAttribute('controls', '');
  await page.locator('audio').evaluate(audio => { audio.currentTime = .5; audio.dispatchEvent(new Event('timeupdate')); });
  await page.getByRole('button', { name: 'Close viewer' }).click();
  await page.getByRole('button', { name: 'Open', exact: true }).click();
  await expect.poll(() => page.locator('audio').evaluate(audio => audio.currentTime)).toBeCloseTo(.5, 1);
});

test('falls back when a native media decoder rejects a file', async ({ page }) => {
  await openFixture(page, 'Bad codec smoke fixture', 'original.mp4', 'not a decodable MP4', 'video', 'video/mp4');
  await expect(page.getByRole('alert')).toContainText('could not decode');
  await expect(page.getByRole('button', { name: 'Download original' })).toBeVisible();
});

test('keeps an original download fallback for unsupported formats', async ({ page }) => {
  const original = 'unsupported original bytes';
  const posts = await openFixture(page, 'Unsupported smoke fixture', 'original.docx', original);
  await expect(page.getByRole('alert')).toContainText('no safe built-in preview');
  const downloaded = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download original' }).click();
  expect((await downloaded).suggestedFilename()).toBe('original.docx');
  expect(posts).toEqual([]);
});
