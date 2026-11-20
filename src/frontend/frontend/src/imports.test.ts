import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiFetch } from './api';
import { getImportJob, listImportJobs } from './imports';

vi.mock('./api', () => ({ apiFetch: vi.fn() }));
beforeEach(() => vi.mocked(apiFetch).mockReset());

describe('import API client', () => {
  it('loads job summaries with an abort signal', async () => {
    const signal = new AbortController().signal;
    vi.mocked(apiFetch).mockResolvedValue(new Response(JSON.stringify({ jobs: [{ id: 'job-1' }] })));
    expect(await listImportJobs(signal)).toEqual([{ id: 'job-1' }]);
    expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining('/file/imports/jobs'), { signal });
  });

  it('encodes job IDs when retrieving details', async () => {
    const signal = new AbortController().signal;
    vi.mocked(apiFetch).mockResolvedValue(new Response(JSON.stringify({ job: { id: 'a/b' } })));
    expect(await getImportJob('a/b', signal)).toEqual({ id: 'a/b' });
    expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining('/jobs/a%2Fb'), { signal });
  });

  it('reports list and detail failures', async () => {
    vi.mocked(apiFetch).mockResolvedValue(new Response('', { status: 503 }));
    const signal = new AbortController().signal;
    await expect(listImportJobs(signal)).rejects.toThrow('Unable to retrieve imports (503).');
    await expect(getImportJob('missing', signal)).rejects.toThrow('Unable to retrieve import details (503).');
  });
});
