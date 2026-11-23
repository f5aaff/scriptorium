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

describe('in-app file viewer', () => {
  it('opens an authenticated preview for read-only users and returns to the same details', async () => {
    vi.mocked(apiFetch).mockImplementation(async input => String(input).includes('/file/reading-state/')
      ? new Response(JSON.stringify({ source_revision: 'source', version: 0, position: null }))
      : String(input).includes('/file/preview/')
      ? new Response('inert document source', { headers: { 'Content-Type': 'text/plain', 'X-Scriptorium-Preview-Kind': 'text' } })
      : response([{ ...item('Readable document'), Path: 'original-key' }]));
    render(Library, { permissions: { upload: false, edit: false, delete: false } });
    await fireEvent.click(await screen.findByText('Readable document'));
    await fireEvent.click(screen.getByRole('button', { name: 'Open' }));
    expect((await screen.findByRole('textbox', { name: 'Document text' }) as HTMLTextAreaElement).value).toBe('inert document source');
    expect(screen.getAllByRole('dialog')).toHaveLength(1);
    await fireEvent.click(screen.getByRole('button', { name: 'Close viewer' }));
    await waitFor(() => expect(screen.getByRole('dialog', { name: 'Readable document' })).toBeTruthy());
    const urls = vi.mocked(apiFetch).mock.calls.map(([url]) => String(url));
    expect(urls.some(url => url.includes('/file/preview/Readable%20document'))).toBe(true);
    expect(urls.some(url => url.includes('/file/reading-state/Readable%20document'))).toBe(true);
    expect(urls.some(url => url.includes('/file/download/') || url.includes('/file/convert/'))).toBe(false);
  });

  it('cancels pending previews when navigating away from the library', async () => {
    vi.mocked(apiFetch).mockImplementation(async input => String(input).includes('/file/reading-state/')
      ? new Response(JSON.stringify({ source_revision: 'source', version: 0, position: null }))
      : String(input).includes('/file/preview/')
      ? new Promise<Response>(() => {}) : response([{ ...item('Pending preview'), Path: 'original-key' }]));
    const { unmount } = render(Library, { permissions });
    await fireEvent.click(await screen.findByText('Pending preview'));
    await fireEvent.click(screen.getByRole('button', { name: 'Open' }));
    const request = vi.mocked(apiFetch).mock.calls.find(([url]) => String(url).includes('/file/preview/'));
    unmount();
    expect(request?.[1]?.signal?.aborted).toBe(true);
  });

  it('does not offer to preview metadata-only records without an original', async () => {
    vi.mocked(apiFetch).mockResolvedValue(response([item('Metadata only')]));
    render(Library, { permissions });
    await fireEvent.click(await screen.findByText('Metadata only'));
    expect((screen.getByRole('button', { name: 'Open' }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe('re-extraction', () => {
  it.each([
    [false, 'original-key', false],
    [true, '', false],
    [true, 'original-key', true]
  ])('only offers the action with edit permission and an original', async (edit, path, visible) => {
    vi.mocked(apiFetch).mockImplementation(async input => String(input).includes('/data/read/')
      ? new Response(JSON.stringify({ value: JSON.stringify({ ...item('Existing PDF'), Path: path, ExtractionStatus: 'failed' }) }))
      : response());
    render(Library, { permissions: { ...permissions, edit }, initialDocumentID: 'pdf-id' });
    await screen.findByRole('dialog', { name: 'Existing PDF' });
    expect(!!screen.queryByRole('button', { name: 'Re-extract text' })).toBe(visible);
  });

  it('refreshes document details and reloads the active search after success', async () => {
    const original = { ...item('Existing PDF'), Path: 'original-key', ExtractionStatus: 'failed', DeweyDecimal: '900' };
    vi.mocked(apiFetch).mockImplementation(async input => String(input).includes('/file/reextract/')
      ? new Response(JSON.stringify({ metadata: { ...original, ExtractionStatus: 'complete' } }))
      : response([original]));
    render(Library, { permissions });
    await screen.findByText('Existing PDF');
    await fireEvent.input(screen.getByRole('textbox', { name: 'Search library' }), { target: { value: 'author: User author' } });
    await waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(2));
    await screen.findByText('Existing PDF');
    await fireEvent.click(screen.getByText('Existing PDF'));
    await fireEvent.click(screen.getByRole('button', { name: 'Re-extract text' }));
    await screen.findByText('Text extraction and search index refreshed.');
    expect(screen.getByText('Text extracted and indexed.')).toBeTruthy();
    expect(screen.getByText('900')).toBeTruthy();
    await waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(4));
    const lastSearch = new URL(String(vi.mocked(apiFetch).mock.calls[3][0]));
    expect(lastSearch.searchParams.get('key')).toBe('Author');
    expect(lastSearch.searchParams.get('value')).toBe('User author');
    expect(lastSearch.searchParams.get('page')).toBe('1');
  });

  it('keeps previous metadata visible when a re-extraction attempt fails', async () => {
    vi.mocked(apiFetch).mockImplementation(async input => {
      if (String(input).includes('/file/reextract/')) {
        return new Response(JSON.stringify({ error: 'Missing tool; previous indexed text retained.' }), { status: 503 });
      }
      if (String(input).includes('/data/read/')) {
        return new Response(JSON.stringify({ value: JSON.stringify({ ...item('Existing PDF'), Path: 'original-key', ExtractionStatus: 'complete' }) }));
      }
      return response();
    });
    render(Library, { permissions, initialDocumentID: 'pdf-id' });
    await screen.findByRole('dialog', { name: 'Existing PDF' });
    await fireEvent.click(screen.getByRole('button', { name: 'Re-extract text' }));
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(screen.getByText('Text extracted and indexed.')).toBeTruthy();
    expect(screen.getByText('Missing tool; previous indexed text retained.')).toBeTruthy();
    expect(apiFetch).toHaveBeenCalledTimes(3);
  });
});

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
