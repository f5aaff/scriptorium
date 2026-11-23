import { beforeEach, expect, it, vi } from 'vitest';
import { apiFetch } from './api';
import { reextractText } from './reextract';

vi.mock('./api', () => ({ apiFetch: vi.fn() }));
beforeEach(() => vi.mocked(apiFetch).mockReset());

it('posts only a document ID and forwards cancellation', async () => {
  const signal = new AbortController().signal;
  const metadata = { Uuid: 'doc/id', ExtractionStatus: 'complete' };
  vi.mocked(apiFetch).mockResolvedValue(new Response(JSON.stringify({ metadata })));
  expect(await reextractText('doc/id', signal)).toEqual(metadata);
  expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining('/file/reextract/doc%2Fid'), { method: 'POST', signal });
});

it('reports public API failures and handles non-JSON failures', async () => {
  const signal = new AbortController().signal;
  vi.mocked(apiFetch).mockResolvedValueOnce(new Response(JSON.stringify({ error: 'Document changed; retry.' }), { status: 409 }))
    .mockResolvedValueOnce(new Response('unavailable', { status: 503 }));
  await expect(reextractText('id', signal)).rejects.toThrow('Document changed; retry.');
  await expect(reextractText('id', signal)).rejects.toThrow('Could not refresh extracted text (503).');
});

it('rejects missing or mismatched response metadata', async () => {
  const signal = new AbortController().signal;
  vi.mocked(apiFetch).mockResolvedValueOnce(new Response('{}'))
    .mockResolvedValueOnce(new Response(JSON.stringify({ metadata: { Uuid: 'other' } })));
  await expect(reextractText('id', signal)).rejects.toThrow('invalid extraction metadata');
  await expect(reextractText('id', signal)).rejects.toThrow('invalid extraction metadata');
});
