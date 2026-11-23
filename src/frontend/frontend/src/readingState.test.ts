import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { apiFetch } from './api';
import { ReadingSession, type ReadingPosition, type ReadingStatus } from './readingState';
vi.mock('./api', () => ({ apiFetch: vi.fn() }));
const position = (page: number): ReadingPosition => ({ kind: 'pdf', page, offset: .25, scale: 1.25 });
const response = (version: number, place: ReadingPosition | null = null) => new Response(JSON.stringify({ source_revision: 'source', version, position: place }));
beforeEach(() => { vi.useFakeTimers(); vi.mocked(apiFetch).mockReset(); });
afterEach(() => vi.useRealTimers());

it('loads a saved place without rewriting it on restore', async () => {
  vi.mocked(apiFetch).mockResolvedValue(response(3, position(12)));
  const session = new ReadingSession('doc', () => {});
  await session.load(new AbortController().signal);
  expect(session.position).toEqual(position(12));
  session.update(position(12));
  await session.flush();
  expect(apiFetch).toHaveBeenCalledTimes(1);
  session.close();
});

it('serializes writes and preserves a return to the previous page during an in-flight save', async () => {
  let resolve!: (response: Response) => void;
  vi.mocked(apiFetch).mockResolvedValueOnce(response(0, position(1)))
    .mockImplementationOnce(() => new Promise(res => { resolve = res; }))
    .mockResolvedValueOnce(response(2, position(1)));
  const session = new ReadingSession('doc', () => {});
  await session.load(new AbortController().signal);
  session.update(position(2));
  const saving = session.flush();
  session.update(position(1));
  expect(apiFetch).toHaveBeenCalledTimes(2);
  resolve(response(1, position(2)));
  await saving;
  const writes = vi.mocked(apiFetch).mock.calls.slice(1).map(([, init]) => JSON.parse(String(init?.body)));
  expect(writes.map(write => [write.expected_version, write.position.page])).toEqual([[0, 2], [1, 1]]);
  session.close();
});

it('does not silently overwrite another window, but permits an explicit save-current-place retry', async () => {
  vi.mocked(apiFetch).mockResolvedValueOnce(response(0))
    .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'Changed in another window' }), { status: 409 }))
    .mockResolvedValueOnce(response(5, position(20)))
    .mockResolvedValueOnce(response(6, position(3)));
  let status: ReadingStatus | undefined;
  const session = new ReadingSession('doc', value => status = value);
  await session.load(new AbortController().signal);
  session.update(position(2));
  await session.flush();
  expect(status?.error).toContain('another window');
  session.update(position(3));
  await session.flush();
  expect(apiFetch).toHaveBeenCalledTimes(2);
  await session.retry();
  expect(JSON.parse(String(vi.mocked(apiFetch).mock.calls[3][1]?.body))).toMatchObject({ expected_version: 5, position: { page: 3 } });
  session.close();
});

it('never writes without first knowing the saved-state version', async () => {
  vi.mocked(apiFetch).mockRejectedValue(new Error('Offline'));
  const session = new ReadingSession('doc', () => {});
  await session.load(new AbortController().signal);
  session.update(position(2));
  await session.flush();
  expect(apiFetch).toHaveBeenCalledTimes(1);
  session.close();
});

it('checkpoints continuous activity rather than waiting for scrolling or playback to stop', async () => {
  vi.mocked(apiFetch).mockResolvedValueOnce(response(0)).mockResolvedValue(response(1, position(5)));
  const session = new ReadingSession('doc', () => {});
  await session.load(new AbortController().signal);
  for (let page = 2; page <= 5; page++) {
    session.update(position(page));
    await vi.advanceTimersByTimeAsync(250);
  }
  expect(apiFetch).toHaveBeenCalledTimes(2);
  expect(JSON.parse(String(vi.mocked(apiFetch).mock.calls[1][1]?.body)).position.page).toBe(5);
  session.close();
});

it('times out unavailable saved-state loading without blocking the reader forever', async () => {
  vi.mocked(apiFetch).mockImplementation(() => new Promise(() => {}));
  let status: ReadingStatus | undefined;
  const session = new ReadingSession('doc', value => status = value);
  const loading = session.load(new AbortController().signal);
  await vi.advanceTimersByTimeAsync(5000);
  await loading;
  expect(status?.error).toContain('timed out');
  expect(status?.loading).toBe(false);
  session.close();
});

it('flushes a pending position with keepalive on close', async () => {
  vi.mocked(apiFetch).mockResolvedValueOnce(response(0)).mockResolvedValueOnce(response(1, position(8)));
  const session = new ReadingSession('doc', () => {});
  await session.load(new AbortController().signal);
  session.update(position(8));
  session.close();
  expect(vi.mocked(apiFetch).mock.calls[1][1]?.keepalive).toBe(true);
  await session.flush();
});
