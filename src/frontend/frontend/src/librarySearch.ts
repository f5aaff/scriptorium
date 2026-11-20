/** SearchCriteria describes the active catalogue query, independent of its page. */
export interface SearchCriteria {
  query?: string;
  key?: string;
  value?: string;
}

/** parseSearch interprets supported field prefixes and trims surrounding whitespace. */
export function parseSearch(input: string): SearchCriteria {
  const query = input.trim();
  const match = /^(author|type|filetype|dewey):\s*(.*)$/i.exec(query);
  if (match) {
    const keys: Record<string, string> = {
      author: 'Author', type: 'DocType', filetype: 'FileType', dewey: 'DeweyDecimal'
    };
    return { key: keys[match[1].toLowerCase()], value: match[2].trim() };
  }
  return query ? { query } : {};
}

/** searchParams applies the same criteria to every page of a search. */
export function searchParams(criteria: SearchCriteria, page: number): URLSearchParams {
  const params = new URLSearchParams({ page: String(page), limit: '20' });
  if (criteria.query) {
    params.set('q', criteria.query);
  } else if (criteria.key && criteria.value) {
    params.set('key', criteria.key);
    params.set('value', criteria.value);
  }
  return params;
}

/** SearchPage is one page returned by the catalogue API. */
export interface SearchPage<T> {
  items: T[];
  hasMore: boolean;
}

/** SearchState describes the results and request status displayed by the library. */
export interface SearchState<T> extends SearchPage<T> {
  loading: boolean;
  error: string;
  loaded: boolean;
}

export type FetchSearchPage<T> = (
  criteria: SearchCriteria, page: number, signal: AbortSignal
) => Promise<SearchPage<T>>;

/** LibrarySearch owns pagination and rejects responses from superseded queries. */
export class LibrarySearch<T> {
  private criteria: SearchCriteria = {};
  private page = 1;
  private generation = 0;
  private request?: AbortController;
  private timer?: ReturnType<typeof setTimeout>;
  private disposed = false;
  private state: SearchState<T> = {
    items: [], hasMore: true, loading: false, error: '', loaded: false
  };

  constructor(
    private fetchPage: FetchSearchPage<T>,
    private onChange: (state: SearchState<T>) => void
  ) {}

  /** setQuery immediately invalidates old work, including during the debounce delay. */
  setQuery(query: string, delay = 0): void {
    if (this.disposed) return;
    this.invalidate();
    this.criteria = parseSearch(query);
    this.page = 1;
    this.state = { items: [], hasMore: true, loading: delay > 0, error: '', loaded: false };
    this.publish();
    if (delay > 0) {
      this.timer = setTimeout(() => {
        this.timer = undefined;
        this.state.loading = false;
        void this.loadMore();
      }, delay);
    } else {
      void this.loadMore();
    }
  }

  /** loadMore fetches the next page once; failed pages remain available for retry. */
  async loadMore(): Promise<void> {
    if (this.disposed || this.state.loading || !this.state.hasMore) return;
    const generation = this.generation;
    const request = new AbortController();
    this.request = request;
    this.state = { ...this.state, loading: true, error: '' };
    this.publish();
    try {
      const result = await this.fetchPage(this.criteria, this.page, request.signal);
      // Abort is best-effort: a response may already be parsed when the query changes.
      if (this.disposed || generation !== this.generation) return;
      this.state = {
        ...this.state,
        items: [...this.state.items, ...result.items],
        hasMore: result.hasMore && result.items.length > 0,
        loaded: true
      };
      this.page++;
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      this.state.error = error instanceof Error ? error.message : String(error);
    } finally {
      if (!this.disposed && generation === this.generation) {
        this.request = undefined;
        this.state.loading = false;
        this.publish();
      }
    }
  }

  /** updateItems applies local catalogue edits without losing pagination state. */
  updateItems(update: (items: T[]) => T[]): void {
    this.state = { ...this.state, items: update(this.state.items) };
    this.publish();
  }

  /** dispose cancels pending work and prevents updates after the view is removed. */
  dispose(): void {
    this.disposed = true;
    this.invalidate();
  }

  private invalidate(): void {
    this.generation++;
    this.request?.abort();
    clearTimeout(this.timer);
    this.timer = undefined;
  }

  private publish(): void {
    this.onChange({ ...this.state });
  }
}
