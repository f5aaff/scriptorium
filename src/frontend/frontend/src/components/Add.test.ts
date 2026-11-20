// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, expect, it, vi } from 'vitest';
import Add from './Add.svelte';
import { apiFetch } from '../api';

vi.mock('../api', () => ({ apiFetch: vi.fn(), getSessionToken: () => '' }));
afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals(); });

it('links queued uploads to import details and stops polling after navigation', async () => {
  vi.useFakeTimers();
  const job = { id: 'queued-job', status: 'running', total: 2, processed: 0, failed: 0 };
  class UploadRequest {
    upload = {};
    status = 202;
    responseText = JSON.stringify({ job });
    onload = () => {};
    open() {}
    setRequestHeader() {}
    send() { queueMicrotask(() => this.onload()); }
  }
  vi.stubGlobal('XMLHttpRequest', UploadRequest);
  vi.mocked(apiFetch).mockImplementation(async input => {
    if (String(input).includes('/file/imports/jobs/')) return new Response(JSON.stringify({ job }));
    return new Response(JSON.stringify({ types: [], categories: [] }));
  });
  const { container, component, unmount } = render(Add);
  const openImport = vi.fn();
  component.$on('openImport', openImport);
  await fireEvent.drop(container.querySelector('.upload-area')!, {
    dataTransfer: { files: [new File(['one'], 'one.txt'), new File(['two'], 'two.txt')] }
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Import 2 files' }));
  await act(async () => {});
  await fireEvent.click(screen.getByRole('button', { name: 'View import details' }));
  expect(openImport.mock.calls[0][0].detail).toBe('queued-job');
  const request = vi.mocked(apiFetch).mock.calls.find(([url]) => String(url).includes('/file/imports/jobs/'));
  unmount();
  expect(request?.[1]?.signal?.aborted).toBe(true);
  await vi.advanceTimersByTimeAsync(5000);
  expect(vi.mocked(apiFetch).mock.calls.filter(([url]) => String(url).includes('/file/imports/jobs/'))).toHaveLength(1);
});
