// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import App from './App.svelte';
import { apiFetch, getCurrentUser } from './api';

vi.mock('./api', () => ({
  apiFetch: vi.fn(), getCurrentUser: vi.fn(), setCSRFToken: vi.fn(),
  setSessionToken: vi.fn(), getSessionToken: vi.fn()
}));

afterEach(cleanup);
beforeEach(() => {
  vi.mocked(getCurrentUser).mockResolvedValue({
    username: 'reader', admin: false, permissions: { upload: false, edit: false, delete: false }
  });
  vi.mocked(apiFetch).mockImplementation(async input => {
    const url = String(input);
    const job = {
      id: 'job-1', status: 'completed', total: 1, processed: 1, failed: 0,
      created_at: '2026-11-20T15:00:00Z', updated_at: '2026-11-20T15:01:00Z'
    };
    if (url.endsWith('/file/imports/jobs')) return new Response(JSON.stringify({ jobs: [job] }));
    if (url.endsWith('/file/imports/jobs/job-1')) return new Response(JSON.stringify({ job: {
      ...job, results: [{ filename: 'book.txt', status: 'imported', document_id: 'document-1' }]
    } }));
    if (url.endsWith('/data/read/document-1')) return new Response(JSON.stringify({ value: JSON.stringify({
      Uuid: 'document-1', Title: 'Imported book', FileType: '.txt'
    }) }));
    return new Response(JSON.stringify({ results: [], has_next: false }));
  });
});

it('lets read-only users inspect imports and open a catalogue record', async () => {
  render(App);
  await fireEvent.click(await screen.findByRole('button', { name: 'Imports' }));
  expect(screen.queryByRole('button', { name: 'Add' })).toBeNull();
  await fireEvent.click(await screen.findByRole('button', { name: /Import job-1/ }));
  await fireEvent.click(await screen.findByRole('button', { name: 'Open book.txt in library' }));
  expect(await screen.findByRole('dialog', { name: 'Imported book' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: /Edit/ })).toBeNull();
});
