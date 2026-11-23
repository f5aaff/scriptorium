// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import FileViewer from './FileViewer.svelte';
import { loadPreview, type LoadedPreview } from '../preview';
import { openPdf } from '../pdfRenderer';
import type { LibraryItem } from '../libraryTypes';

vi.mock('../preview', () => ({ loadPreview: vi.fn() }));
vi.mock('../pdfRenderer', () => ({ openPdf: vi.fn() }));
const item: LibraryItem = {
  Uuid: 'doc-1', Title: 'My document', OriginalFilename: 'original.pdf', Author: '', PublishDate: '',
  LastUpdated: '', DocType: 'Book', DeweyDecimal: '', Path: 'original-key', FileType: '.pdf'
};
const createURL = vi.fn(() => 'blob:preview');
const revokeURL = vi.fn();
beforeEach(() => {
  vi.mocked(loadPreview).mockReset();
  createURL.mockClear(); revokeURL.mockClear();
  URL.createObjectURL = createURL;
  URL.revokeObjectURL = revokeURL;
  vi.mocked(openPdf).mockResolvedValue({
    pageCount: 1, renderPage: vi.fn(async () => 1), pageText: vi.fn(async () => ({ text: '', truncated: false })), destroy: vi.fn(async () => {})
  });
});
afterEach(cleanup);

it('shows source markup as inert selectable text', async () => {
  const source = '<script>window.compromised = true</script><svg onload="alert(1)"></svg>';
  vi.mocked(loadPreview).mockResolvedValue({ kind: 'text', text: source, truncated: true });
  const { container } = render(FileViewer, { item });
  const textbox = await screen.findByRole('textbox', { name: 'File source text' }) as HTMLTextAreaElement;
  expect(textbox.value).toBe(source);
  expect(container.querySelector('script, svg')).toBeNull();
  expect(screen.getByText(/Showing the first 1 MiB/)).toBeTruthy();
});

it.each(['image', 'audio', 'video'] as const)('renders %s using a local object URL and revokes it', async kind => {
  const types = { image: 'image/png', audio: 'audio/wave', video: 'video/mp4' };
  vi.mocked(loadPreview).mockResolvedValue({ kind, data: new ArrayBuffer(4), mediaType: types[kind] });
  const { container, unmount } = render(FileViewer, { item });
  await waitFor(() => expect(createURL).toHaveBeenCalledTimes(1));
  const media = container.querySelector(kind === 'image' ? 'img' : kind)!;
  expect(media.getAttribute('src')).toBe('blob:preview');
  await fireEvent.error(media);
  expect(await screen.findByRole('alert')).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Download original' })).toBeTruthy();
  unmount();
  expect(revokeURL).toHaveBeenCalledWith('blob:preview');
});

it('delegates PDFs to PDF.js instead of using an iframe or conversion', async () => {
  const data = new ArrayBuffer(8);
  vi.mocked(loadPreview).mockResolvedValue({ kind: 'pdf', data, mediaType: 'application/pdf' });
  const { container } = render(FileViewer, { item });
  expect(await screen.findByRole('region', { name: 'PDF reader' })).toBeTruthy();
  await waitFor(() => expect(openPdf).toHaveBeenCalledWith(data, expect.any(AbortSignal)));
  expect(container.querySelector('iframe, object, embed')).toBeNull();
});

it('keeps download available for unsupported formats and supports retry', async () => {
  vi.mocked(loadPreview).mockRejectedValueOnce(new Error('Unsupported format; download the original.'))
    .mockResolvedValueOnce({ kind: 'text', text: 'recovered text', truncated: false });
  const { component } = render(FileViewer, { item });
  const downloaded = vi.fn();
  component.$on('download', downloaded);
  await screen.findByRole('alert');
  await fireEvent.click(screen.getByRole('button', { name: 'Download original' }));
  expect(downloaded.mock.calls[0][0].detail).toEqual(item);
  await fireEvent.click(screen.getByRole('button', { name: 'Retry preview' }));
  expect((await screen.findByRole('textbox', { name: 'File source text' }) as HTMLTextAreaElement).value).toBe('recovered text');
});

it('aborts loading and ignores late responses after closing', async () => {
  let resolve!: (preview: LoadedPreview) => void;
  vi.mocked(loadPreview).mockReturnValue(new Promise(res => { resolve = res; }));
  const { unmount } = render(FileViewer, { item });
  unmount();
  expect(vi.mocked(loadPreview).mock.calls[0][1].aborted).toBe(true);
  await act(async () => { resolve({ kind: 'image', data: new ArrayBuffer(4), mediaType: 'image/png' }); });
  expect(createURL).not.toHaveBeenCalled();
});

it('traps keyboard focus, closes with Escape, and restores the previous focus', async () => {
  vi.mocked(loadPreview).mockResolvedValue({ kind: 'text', text: 'source', truncated: false });
  const trigger = document.createElement('button');
  document.body.append(trigger);
  trigger.focus();
  const { component, unmount } = render(FileViewer, { item });
  const closed = vi.fn();
  component.$on('close', closed);
  const source = await screen.findByRole('textbox', { name: 'File source text' });
  source.focus();
  await fireEvent.keyDown(window, { key: 'Tab' });
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Download original' }));
  await fireEvent.keyDown(window, { key: 'Tab', shiftKey: true });
  expect(document.activeElement).toBe(source);
  await fireEvent.keyDown(window, { key: 'Escape' });
  expect(closed).toHaveBeenCalledTimes(1);
  unmount();
  await act(async () => {});
  expect(document.activeElement).toBe(trigger);
  trigger.remove();
});
