// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Imports from './Imports.svelte';
import { getImportJob, listImportJobs, type ImportJob } from '../imports';

vi.mock('../imports', async importOriginal => ({
  ...await importOriginal<typeof import('../imports')>(),
  getImportJob: vi.fn(), listImportJobs: vi.fn()
}));

const job = (status: ImportJob['status'] = 'completed'): ImportJob => ({
  id: 'job-1', status, total: 3, processed: 3, failed: 1,
  created_at: '2026-11-20T15:00:00Z', updated_at: '2026-11-20T15:01:00Z'
});

beforeEach(() => {
  vi.mocked(listImportJobs).mockReset();
  vi.mocked(getImportJob).mockReset();
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe('Imports', () => {
  it('loads persisted history again after leaving and returning', async () => {
    vi.mocked(listImportJobs).mockResolvedValue([job()]);
    const first = render(Imports);
    expect(await screen.findByText('Import job-1')).toBeTruthy();
    first.unmount();
    render(Imports);
    expect(await screen.findByText('Import job-1')).toBeTruthy();
    expect(listImportJobs).toHaveBeenCalledTimes(2);
  });

  it('shows imported, duplicate and failed outcomes and emits document links', async () => {
    vi.mocked(listImportJobs).mockResolvedValue([job()]);
    vi.mocked(getImportJob).mockResolvedValue({ ...job(), results: [
      { filename: 'book.txt', status: 'imported', document_id: 'document-1' },
      { filename: 'copy.txt', status: 'duplicate', document_id: 'document-1' },
      { filename: 'broken.txt', status: 'failed', error: 'Could not import file' }
    ] });
    const { component } = render(Imports);
    const opened = vi.fn();
    component.$on('openDocument', opened);
    await fireEvent.click(await screen.findByRole('button', { name: /Import job-1/ }));
    expect(await screen.findByText('book.txt')).toBeTruthy();
    expect(screen.getByText('Duplicate — already in library')).toBeTruthy();
    expect(screen.getByText('Could not import file')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Open copy.txt in library' }));
    expect(opened.mock.calls[0][0].detail).toBe('document-1');
    expect(screen.getAllByRole('button', { name: /in library/ })).toHaveLength(2);
  });

  it('reports errors without claiming that history is empty and allows retry', async () => {
    vi.mocked(listImportJobs).mockRejectedValueOnce(new Error('Offline')).mockResolvedValueOnce([]);
    render(Imports);
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(screen.queryByText(/No imports yet/)).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Retry imports' }));
    expect(await screen.findByText(/No imports yet/)).toBeTruthy();
  });

  it('explains missing per-file results for legacy jobs', async () => {
    vi.mocked(listImportJobs).mockResolvedValue([job()]);
    vi.mocked(getImportJob).mockResolvedValue(job());
    render(Imports, { initialJobID: 'job-1' });
    expect(await screen.findByText(/Older jobs retain aggregate progress only/)).toBeTruthy();
    expect(getImportJob).toHaveBeenCalledWith('job-1', expect.any(AbortSignal));
  });

  it('polls active jobs without overlapping requests and stops after completion', async () => {
    vi.useFakeTimers();
    let resolve!: (jobs: ImportJob[]) => void;
    vi.mocked(listImportJobs).mockResolvedValueOnce([job('running')])
      .mockImplementationOnce(() => new Promise(res => { resolve = res; }));
    render(Imports);
    await act(async () => {});
    await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
    expect(listImportJobs).toHaveBeenCalledTimes(2);
    await act(async () => { await vi.advanceTimersByTimeAsync(10000); });
    expect(listImportJobs).toHaveBeenCalledTimes(2);
    await act(async () => { resolve([job('completed')]); });
    await act(async () => { await vi.advanceTimersByTimeAsync(10000); });
    expect(listImportJobs).toHaveBeenCalledTimes(2);
  });

  it('starts polling when selected details reveal an active job', async () => {
    vi.useFakeTimers();
    vi.mocked(listImportJobs).mockResolvedValue([job('completed')]);
    vi.mocked(getImportJob).mockResolvedValue(job('running'));
    render(Imports);
    await act(async () => {});
    await fireEvent.click(screen.getByRole('button', { name: /Import job-1/ }));
    await act(async () => {});
    await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
    expect(listImportJobs).toHaveBeenCalledTimes(2);
    expect(getImportJob).toHaveBeenCalledTimes(2);
  });

  it('aborts requests and stops polling when removed', async () => {
    vi.useFakeTimers();
    vi.mocked(listImportJobs).mockResolvedValue([job('running')]);
    const { unmount } = render(Imports);
    await act(async () => {});
    unmount();
    expect(vi.mocked(listImportJobs).mock.calls[0][0].aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(10000);
    expect(listImportJobs).toHaveBeenCalledTimes(1);
  });

  it('discards late details when a different job is selected', async () => {
    let resolve!: (job: ImportJob) => void;
    vi.mocked(listImportJobs).mockResolvedValue([job(), { ...job(), id: 'job-2' }]);
    vi.mocked(getImportJob).mockImplementationOnce(() => new Promise(res => { resolve = res; }))
      .mockResolvedValueOnce({ ...job(), id: 'job-2', results: [{ filename: 'current.txt', status: 'imported' }] });
    render(Imports);
    await fireEvent.click(await screen.findByRole('button', { name: /Import job-1/ }));
    await fireEvent.click(screen.getByRole('button', { name: /Import job-2/ }));
    await screen.findByText('current.txt');
    await act(async () => { resolve({ ...job(), results: [{ filename: 'stale.txt', status: 'imported' }] }); });
    expect(screen.queryByText('stale.txt')).toBeNull();
    expect(screen.getByText('current.txt')).toBeTruthy();
    expect(vi.mocked(getImportJob).mock.calls[0][1].aborted).toBe(true);
  });
});
