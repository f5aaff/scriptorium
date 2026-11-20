import { afterEach, describe, expect, it, vi } from 'vitest';
import { LibrarySearch, type FetchSearchPage, type SearchPage, type SearchState } from './librarySearch';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

function setup(fetchPage: FetchSearchPage<string>) {
  let state: SearchState<string>;
  const search = new LibrarySearch(fetchPage, next => { state = next; });
  return { search, state: () => state };
}

// setQuery starts a request without exposing its promise to the view.
async function settle() {
  await Promise.resolve();
  await Promise.resolve();
}

afterEach(() => vi.useRealTimers());

describe('LibrarySearch', () => {
  it('appends the next filtered page without repeating page one', async () => {
    const fetchPage = vi.fn<FetchSearchPage<string>>()
      .mockResolvedValueOnce({ items: ['first'], hasMore: true })
      .mockResolvedValueOnce({ items: ['second'], hasMore: false });
    const { search, state } = setup(fetchPage);
    search.setQuery('author: Le Guin');
    await settle();
    await search.loadMore();
    await search.loadMore();
    expect(fetchPage.mock.calls.map(([criteria, page]) => [criteria, page])).toEqual([
      [{ key: 'Author', value: 'Le Guin' }, 1],
      [{ key: 'Author', value: 'Le Guin' }, 2]
    ]);
    expect(state().items).toEqual(['first', 'second']);
    expect(state().hasMore).toBe(false);
  });

  it('ignores duplicate load requests while a page is in flight', async () => {
    const pending = deferred<SearchPage<string>>();
    const fetchPage = vi.fn<FetchSearchPage<string>>().mockReturnValue(pending.promise);
    const { search } = setup(fetchPage);
    search.setQuery('books');
    await search.loadMore();
    expect(fetchPage).toHaveBeenCalledTimes(1);
    pending.resolve({ items: [], hasMore: false });
    await settle();
  });

  it('ignores stale success and cleanup even if the fetcher ignores abort', async () => {
    const old = deferred<SearchPage<string>>();
    const current = deferred<SearchPage<string>>();
    const fetchPage = vi.fn<FetchSearchPage<string>>()
      .mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    const { search, state } = setup(fetchPage);
    search.setQuery('old');
    search.setQuery('new');
    expect(fetchPage.mock.calls[0][2].aborted).toBe(true);
    old.resolve({ items: ['stale'], hasMore: false });
    await settle();
    expect(state().items).toEqual([]);
    expect(state().loading).toBe(true);
    current.resolve({ items: ['current'], hasMore: false });
    await settle();
    expect(state().items).toEqual(['current']);
    expect(state().loading).toBe(false);
  });

  it('ignores an old failure after newer results arrive', async () => {
    const old = deferred<SearchPage<string>>();
    const fetchPage = vi.fn<FetchSearchPage<string>>()
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce({ items: ['current'], hasMore: false });
    const { search, state } = setup(fetchPage);
    search.setQuery('old');
    search.setQuery('new');
    await settle();
    old.reject(new Error('stale failure'));
    await settle();
    expect(state().items).toEqual(['current']);
    expect(state().error).toBe('');
  });

  it('invalidates old requests immediately and debounces only the latest input', async () => {
    vi.useFakeTimers();
    const old = deferred<SearchPage<string>>();
    const fetchPage = vi.fn<FetchSearchPage<string>>()
      .mockReturnValueOnce(old.promise)
      .mockResolvedValue({ items: ['latest'], hasMore: false });
    const { search, state } = setup(fetchPage);
    search.setQuery('old');
    search.setQuery('intermediate', 300);
    search.setQuery('latest', 300);
    old.resolve({ items: ['stale'], hasMore: false });
    await settle();
    expect(state().items).toEqual([]);
    expect(state().loading).toBe(true);
    await vi.advanceTimersByTimeAsync(299);
    expect(fetchPage).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchPage).toHaveBeenCalledTimes(2);
    expect(fetchPage.mock.calls[1][0]).toEqual({ query: 'latest' });
    expect(state().items).toEqual(['latest']);
  });

  it('clearing search cancels delayed work and restarts unfiltered at page one', async () => {
    vi.useFakeTimers();
    const fetchPage = vi.fn<FetchSearchPage<string>>()
      .mockResolvedValue({ items: ['all'], hasMore: false });
    const { search, state } = setup(fetchPage);
    search.setQuery('author: pending', 300);
    search.setQuery('');
    await vi.runAllTimersAsync();
    expect(fetchPage).toHaveBeenCalledTimes(1);
    expect(fetchPage.mock.calls[0].slice(0, 2)).toEqual([{}, 1]);
    expect(state().items).toEqual(['all']);
  });

  it('preserves earlier results and retries the failed page', async () => {
    const fetchPage = vi.fn<FetchSearchPage<string>>()
      .mockResolvedValueOnce({ items: ['first'], hasMore: true })
      .mockRejectedValueOnce(new Error('Network unavailable'))
      .mockResolvedValueOnce({ items: ['second'], hasMore: false });
    const { search, state } = setup(fetchPage);
    search.setQuery('books');
    await settle();
    await search.loadMore();
    expect(state()).toMatchObject({ items: ['first'], error: 'Network unavailable', loading: false });
    await search.loadMore();
    expect(fetchPage.mock.calls.map(call => call[1])).toEqual([1, 2, 2]);
    expect(state()).toMatchObject({ items: ['first', 'second'], error: '', loaded: true });
  });

  it('distinguishes initial failures from successfully empty results', async () => {
    const fetchPage = vi.fn<FetchSearchPage<string>>()
      .mockRejectedValueOnce(new Error('Offline'))
      .mockResolvedValueOnce({ items: [], hasMore: true });
    const { search, state } = setup(fetchPage);
    search.setQuery('books');
    await settle();
    expect(state()).toMatchObject({ loaded: false, error: 'Offline', loading: false });
    await search.loadMore();
    expect(state()).toMatchObject({ loaded: true, error: '', hasMore: false, items: [] });
  });

  it('keeps local deletions removed when subsequent pages arrive', async () => {
    const fetchPage = vi.fn<FetchSearchPage<string>>()
      .mockResolvedValueOnce({ items: ['deleted', 'kept'], hasMore: true })
      .mockResolvedValueOnce({ items: ['next'], hasMore: false });
    const { search, state } = setup(fetchPage);
    search.setQuery('');
    await settle();
    search.updateItems(items => items.filter(item => item !== 'deleted'));
    await search.loadMore();
    expect(state().items).toEqual(['kept', 'next']);
  });

  it('cancels timers and prevents publication after disposal', async () => {
    vi.useFakeTimers();
    const pending = deferred<SearchPage<string>>();
    const fetchPage = vi.fn<FetchSearchPage<string>>().mockReturnValue(pending.promise);
    const onChange = vi.fn();
    const search = new LibrarySearch(fetchPage, onChange);
    search.setQuery('old');
    search.setQuery('pending', 300);
    search.dispose();
    onChange.mockClear();
    pending.resolve({ items: ['stale'], hasMore: false });
    await vi.runAllTimersAsync();
    await search.loadMore();
    search.setQuery('ignored');
    expect(fetchPage.mock.calls[0][2].aborted).toBe(true);
    expect(fetchPage).toHaveBeenCalledTimes(1);
    expect(onChange).not.toHaveBeenCalled();
  });
});
