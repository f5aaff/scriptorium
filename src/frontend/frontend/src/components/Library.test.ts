// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Library from './Library.svelte';
import { apiFetch } from '../api';

vi.mock('../api', () => ({ apiFetch: vi.fn() }));

const permissions = { upload: true, edit: true, delete: true };
const response = (results: object[] = [], hasNext = false) => new Response(
  JSON.stringify({ results, has_next: hasNext }), { status: 200 }
);
const item = (id: string) => ({ Uuid: id, Title: id, FileType: '.txt' });

beforeEach(() => vi.mocked(apiFetch).mockReset());
afterEach(() => cleanup());

describe('text extraction feedback', () => {
  it.each([
    ['complete', 'Text extracted and indexed.'],
    ['truncated', 'Text partially indexed (extraction limit reached).'],
    ['unavailable', 'Extraction tool unavailable. PDF extraction requires Poppler (pdftotext) on the server.'],
    ['no_text', 'No extractable text found. This PDF may need OCR.'],
    ['failed', 'Text extraction failed. The original file is retained.'],
    ['unsupported', 'Text extraction is not supported for this format.']
  ])('explains the %s outcome in document details', async (status, message) => {
    vi.mocked(apiFetch).mockImplementation(async input => String(input).includes('/data/read/')
      ? new Response(JSON.stringify({ value: JSON.stringify({ ...item('PDF document'), ExtractionStatus: status }) }))
      : response());
    render(Library, { permissions, initialDocumentID: 'pdf-id' });
    await screen.findByRole('dialog', { name: 'PDF document' });
    expect(screen.getByText(message)).toBeTruthy();
  });
});

describe('imported document links', () => {
  it('reports a deleted record without breaking library browsing', async () => {
    vi.mocked(apiFetch).mockImplementation(async input => String(input).includes('/data/read/')
      ? new Response('', { status: 400 })
      : response([item('Available document')]));
    render(Library, { permissions, initialDocumentID: 'deleted-id' });
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(screen.getByText(/Could not open the imported document/)).toBeTruthy();
    expect(await screen.findByText('Available document')).toBeTruthy();
  });

  it('aborts a linked document request on navigation', async () => {
    vi.mocked(apiFetch).mockImplementation(async input => String(input).includes('/data/read/')
      ? new Promise<Response>(() => {}) : response());
    const { unmount } = render(Library, { permissions, initialDocumentID: 'pending-id' });
    const request = vi.mocked(apiFetch).mock.calls.find(([url]) => String(url).includes('/data/read/'));
    unmount();
    expect(request?.[1]?.signal?.aborted).toBe(true);
  });
});

describe('library search feedback', () => {
  it('shows loading before a successfully empty library', async () => {
    let resolve!: (value: Response) => void;
    vi.mocked(apiFetch).mockReturnValue(new Promise(res => { resolve = res; }));
    render(Library, { permissions });
    expect(screen.getByText('Loading library...')).toBeTruthy();
    expect(screen.queryByText('Your library is empty')).toBeNull();
    resolve(response());
    expect(await screen.findByText('Your library is empty')).toBeTruthy();
  });

  it('shows a request error instead of an empty state and supports retry', async () => {
    vi.mocked(apiFetch)
      .mockRejectedValueOnce(new Error('Connection lost'))
      .mockResolvedValueOnce(response([item('Recovered document')]));
    render(Library, { permissions });
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(screen.getByText('Connection lost')).toBeTruthy();
    expect(screen.queryByText('Your library is empty')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText('Recovered document')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(vi.mocked(apiFetch).mock.calls.map(([url]) => new URL(String(url)).searchParams.get('page')))
      .toEqual(['1', '1']);
  });

  it('does not claim no matches while a search is debouncing or loading', async () => {
    let resolve!: (value: Response) => void;
    vi.mocked(apiFetch).mockResolvedValueOnce(response([item('Existing document')]))
      .mockImplementationOnce(() => new Promise(res => { resolve = res; }));
    render(Library, { permissions });
    await screen.findByText('Existing document');
    await fireEvent.input(screen.getByRole('textbox', { name: 'Search library' }), {
      target: { value: 'missing' }
    });
    expect(screen.getByText('Loading library...')).toBeTruthy();
    expect(screen.queryByText('No results found')).toBeNull();
    expect(screen.queryByText('No items match your search')).toBeNull();
    await waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(2));
    expect(screen.queryByText('No results found')).toBeNull();
    resolve(response());
    expect(await screen.findByText('No results found')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Clear search' }));
    expect(screen.queryByText('No results found')).toBeNull();
  });

  it('retains existing cards after a failed next page and retries with the same filter', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce(response())
      .mockResolvedValueOnce(response([item('First match')], true))
      .mockRejectedValueOnce(new Error('Temporary failure'))
      .mockResolvedValueOnce(response([item('Second match')]));
    const { container } = render(Library, { permissions });
    await screen.findByText('Your library is empty');
    await fireEvent.input(screen.getByRole('textbox', { name: 'Search library' }), {
      target: { value: 'author: Le Guin' }
    });
    await screen.findByText('First match');
    await fireEvent.scroll(container.querySelector('.gallery')!);
    await screen.findByRole('alert');
    expect(screen.getByText('First match')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await screen.findByText('Second match');
    expect(screen.getByText('First match')).toBeTruthy();
    expect(vi.mocked(apiFetch).mock.calls.slice(1).map(([url]) => {
      const params = new URL(String(url)).searchParams;
      return [params.get('page'), params.get('key'), params.get('value')];
    })).toEqual([
      ['1', 'Author', 'Le Guin'], ['2', 'Author', 'Le Guin'], ['2', 'Author', 'Le Guin']
    ]);
  });
});
