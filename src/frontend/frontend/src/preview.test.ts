import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiFetch } from './api';
import { loadPreview, MAX_PREVIEW_BYTES, MAX_TEXT_PREVIEW_BYTES } from './preview';

vi.mock('./api', () => ({ apiFetch: vi.fn() }));
beforeEach(() => vi.mocked(apiFetch).mockReset());
const headers = (kind: string, type: string) => ({ 'X-Scriptorium-Preview-Kind': kind, 'Content-Type': type });

describe('preview loading boundary', () => {
  it('authenticates through apiFetch and returns inert text', async () => {
    vi.mocked(apiFetch).mockResolvedValue(new Response('<script>alert(1)</script>', { headers: headers('text', 'text/plain; charset=utf-8') }));
    const signal = new AbortController().signal;
    expect(await loadPreview('a/b', signal)).toEqual({ kind: 'text', text: '<script>alert(1)</script>', truncated: false });
    expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining('/file/preview/a%2Fb'), { signal });
  });

  it.each([
    ['image', 'image/svg+xml'], ['text', 'text/html'], ['pdf', 'text/html'], ['unknown', 'text/plain'], ['__proto__', 'text/plain']
  ])('rejects unsafe or mismatched %s / %s responses', async (kind, type) => {
    const response = new Response('unsafe', { headers: headers(kind, type) });
    vi.mocked(apiFetch).mockResolvedValue(response);
    await expect(loadPreview('id', new AbortController().signal)).rejects.toThrow('safely previewed');
    expect(response.body?.locked).toBe(false);
  });

  it('truncates text at a valid UTF-8 boundary and cancels the remainder', async () => {
    const source = 'a'.repeat(MAX_TEXT_PREVIEW_BYTES - 1) + 'é and more';
    vi.mocked(apiFetch).mockResolvedValue(new Response(source, { headers: headers('text', 'text/plain') }));
    const preview = await loadPreview('id', new AbortController().signal);
    expect(preview.kind).toBe('text');
    if (preview.kind === 'text') {
      expect(preview.truncated).toBe(true);
      expect(preview.text).toBe('a'.repeat(MAX_TEXT_PREVIEW_BYTES - 1));
    }
  });

  it('rejects invalid UTF-8 rather than rendering guessed text', async () => {
    vi.mocked(apiFetch).mockResolvedValue(new Response(new Uint8Array([0xff]), { headers: headers('text', 'text/plain') }));
    await expect(loadPreview('id', new AbortController().signal)).rejects.toThrow('not valid UTF-8');
  });

  it('rejects declared oversized files before reading content', async () => {
    vi.mocked(apiFetch).mockResolvedValue(new Response('bytes', { headers: { ...headers('pdf', 'application/pdf'), 'Content-Length': String(MAX_PREVIEW_BYTES + 1) } }));
    await expect(loadPreview('id', new AbortController().signal)).rejects.toThrow('100 MiB');
  });

  it('bounds actual streamed bytes even when Content-Length is absent or misleading', async () => {
    const chunk = new Uint8Array(1024 * 1024);
    let count = 0;
    let cancelled = false;
    const stream = new ReadableStream({
      pull(controller) { controller.enqueue(chunk); count++; },
      cancel() { cancelled = true; }
    });
    vi.mocked(apiFetch).mockResolvedValue(new Response(stream, { headers: { ...headers('pdf', 'application/pdf'), 'Content-Length': '1' } }));
    await expect(loadPreview('id', new AbortController().signal)).rejects.toThrow('100 MiB');
    expect(cancelled).toBe(true);
    expect(count).toBeLessThanOrEqual(102);
  });

  it('cancels an in-flight stream on navigation', async () => {
    let cancelled = false;
    const response = new Response(new ReadableStream({
      pull() { return new Promise(() => {}); },
      cancel() { cancelled = true; }
    }), { headers: headers('pdf', 'application/pdf') });
    vi.mocked(apiFetch).mockResolvedValue(response);
    const controller = new AbortController();
    const pending = loadPreview('id', controller.signal);
    await vi.waitFor(() => expect(response.body?.locked).toBe(true));
    controller.abort();
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    expect(cancelled).toBe(true);
  });

  it('preserves public fallback errors for unsupported formats', async () => {
    vi.mocked(apiFetch).mockResolvedValue(new Response(JSON.stringify({ error: 'Download this unsupported original.' }), { status: 415 }));
    await expect(loadPreview('id', new AbortController().signal)).rejects.toThrow('Download this unsupported original.');
  });
});
