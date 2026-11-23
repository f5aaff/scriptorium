// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import PdfView from './PdfView.svelte';
import { openPdf, type PdfDocument } from '../pdfRenderer';

vi.mock('../pdfRenderer', () => ({ openPdf: vi.fn() }));
let document: PdfDocument;
beforeEach(() => {
  document = {
    pageCount: 3,
    renderPage: vi.fn(async (_canvas, _page, scale) => scale),
    pageText: vi.fn(async () => ({ text: '<script>inert page text</script>', truncated: false })),
    destroy: vi.fn(async () => {})
  };
  vi.mocked(openPdf).mockReset().mockResolvedValue(document);
});
afterEach(cleanup);

it('renders page navigation, jump controls and zoom without conversion', async () => {
  render(PdfView, { data: new ArrayBuffer(0) });
  await waitFor(() => expect((screen.getByRole('button', { name: 'Next page' }) as HTMLButtonElement).disabled).toBe(false));
  expect((screen.getByRole('button', { name: 'Previous page' }) as HTMLButtonElement).disabled).toBe(true);
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }));
  await waitFor(() => expect(document.renderPage).toHaveBeenLastCalledWith(expect.any(HTMLCanvasElement), 2, 1, expect.any(AbortSignal)));
  await fireEvent.input(screen.getByRole('spinbutton', { name: 'Page' }), { target: { value: '3' } });
  await fireEvent.submit(screen.getByRole('button', { name: 'Go' }).closest('form')!);
  await waitFor(() => expect(document.renderPage).toHaveBeenLastCalledWith(expect.any(HTMLCanvasElement), 3, 1, expect.any(AbortSignal)));
  await fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }));
  await waitFor(() => expect(document.renderPage).toHaveBeenLastCalledWith(expect.any(HTMLCanvasElement), 3, 1.25, expect.any(AbortSignal)));
  expect((screen.getByRole('button', { name: 'Next page' }) as HTMLButtonElement).disabled).toBe(true);
});

it('exposes selectable page text as inert content', async () => {
  const { container } = render(PdfView, { data: new ArrayBuffer(0) });
  await waitFor(() => expect((screen.getByRole('button', { name: 'Show page text' }) as HTMLButtonElement).disabled).toBe(false));
  await fireEvent.click(screen.getByRole('button', { name: 'Show page text' }));
  expect(await screen.findByText('<script>inert page text</script>')).toBeTruthy();
  expect(container.querySelector('script')).toBeNull();
});

it('destroys a late-loading document after navigation', async () => {
  let resolve!: (document: PdfDocument) => void;
  vi.mocked(openPdf).mockReturnValue(new Promise(res => { resolve = res; }));
  const { unmount } = render(PdfView, { data: new ArrayBuffer(0) });
  unmount();
  expect(vi.mocked(openPdf).mock.calls[0][1].aborted).toBe(true);
  await act(async () => { resolve(document); });
  expect(document.destroy).toHaveBeenCalledTimes(1);
  expect(document.renderPage).not.toHaveBeenCalled();
});

it('cancels a previous page text read when changing pages', async () => {
  let resolve!: (value: { text: string; truncated: boolean }) => void;
  vi.mocked(document.pageText).mockReturnValue(new Promise(res => { resolve = res; }));
  render(PdfView, { data: new ArrayBuffer(0) });
  await waitFor(() => expect((screen.getByRole('button', { name: 'Show page text' }) as HTMLButtonElement).disabled).toBe(false));
  await fireEvent.click(screen.getByRole('button', { name: 'Show page text' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }));
  expect(vi.mocked(document.pageText).mock.calls[0][1].aborted).toBe(true);
  await act(async () => { resolve({ text: 'stale page text', truncated: false }); });
  expect(screen.queryByText('stale page text')).toBeNull();
});

it('offers a useful fallback for password-protected PDFs', async () => {
  vi.mocked(openPdf).mockRejectedValue(Object.assign(new Error('password'), { name: 'PasswordException' }));
  render(PdfView, { data: new ArrayBuffer(0) });
  expect(await screen.findByRole('alert')).toBeTruthy();
  expect(screen.getByText(/Password-protected PDFs cannot be previewed/)).toBeTruthy();
});
