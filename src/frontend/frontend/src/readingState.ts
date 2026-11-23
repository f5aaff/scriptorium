import { apiFetch } from './api';
import { API_BASE_URL } from './config';

export interface ReadingPosition {
  kind: 'pdf' | 'text' | 'epub' | 'audio' | 'video';
  page: number;
  offset: number;
  scale: number;
  locator?: string;
}
export interface ReadingState {
  source_revision: string;
  version: number;
  position: ReadingPosition | null;
}
export interface ReadingStatus { loading: boolean; saving: boolean; saved: boolean; error: string }

async function result(response: Response): Promise<ReadingState> {
  const body = await response.json().catch(() => null);
  if (!response.ok) throw new Error(body?.error || 'Reading position could not be saved.');
  if (!body || typeof body.source_revision !== 'string' || !Number.isSafeInteger(body.version) || body.version < 0) {
    throw new Error('Invalid reading-state response.');
  }
  return body;
}

/** ReadingSession coalesces activity into serial optimistic writes. Failed writes
 * are never silently rebased over a newer window; retry is an explicit user action.
 */
export class ReadingSession {
  private state?: ReadingState;
  private current?: ReadingPosition;
  private lastSaved = '';
  private pending?: ReadingPosition;
  private timer?: ReturnType<typeof setTimeout>;
  private active?: Promise<void>;
  private keepalive = false;
  private closed = false;
  private status: ReadingStatus = { loading: true, saving: false, saved: false, error: '' };
  private readonly url: string;

  constructor(id: string, private notify: (status: ReadingStatus) => void) {
    this.url = `${API_BASE_URL}/file/reading-state/${encodeURIComponent(id)}`;
  }
  get position(): ReadingPosition | null { return this.state?.position || null; }
  private publish() { if (!this.closed) this.notify({ ...this.status }); }

  async load(signal: AbortSignal): Promise<void> {
    // A broken state request must not indefinitely prevent reading file bytes
    // that have already loaded. Keep state cancellation separate from file I/O.
    const controller = new AbortController();
    let interrupt!: (error: Error) => void;
    const interrupted = new Promise<never>((_, reject) => { interrupt = reject; });
    const abort = () => { interrupt(new DOMException('Reader closed', 'AbortError')); controller.abort(); };
    const timer = setTimeout(() => {
      interrupt(new Error('Saved-place loading timed out. Reading remains available.'));
      controller.abort();
    }, 5000);
    signal.addEventListener('abort', abort, { once: true });
    if (signal.aborted) abort();
    try {
      const state = await Promise.race([apiFetch(this.url, { signal: controller.signal }).then(result), interrupted]);
      if (signal.aborted || this.closed) return;
      this.state = state;
      this.lastSaved = JSON.stringify(state.position);
      this.status.saved = !!state.position;
      this.status.error = '';
    } catch (error) {
      if (!signal.aborted && !this.closed) this.status.error = error instanceof Error ? error.message : String(error);
    } finally {
      clearTimeout(timer);
      signal.removeEventListener('abort', abort);
      if (!signal.aborted) { this.status.loading = false; this.publish(); }
    }
  }

  update(position: ReadingPosition) {
    if (this.closed) return;
    this.current = { ...position };
    if (!this.active && JSON.stringify(position) === this.lastSaved) {
      this.pending = undefined;
      clearTimeout(this.timer);
      this.timer = undefined;
      this.status.saved = true;
      this.publish();
      return;
    }
    this.pending = this.current;
    this.status.saved = false;
    // Throttle rather than trailing-debounce: continuous playback/scrolling must
    // checkpoint regularly even if activity never becomes idle.
    if (!this.timer && !this.status.error && this.state) this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.flush();
    }, 1000);
    this.publish();
  }

  flush(keepalive = false): Promise<void> {
    clearTimeout(this.timer);
    this.timer = undefined;
    this.keepalive ||= keepalive;
    if (this.active) return this.active;
    this.active = this.writePending().finally(() => { this.active = undefined; });
    return this.active;
  }

  private async writePending() {
    while (this.pending && this.state && !this.status.error) {
      const position = this.pending;
      this.pending = undefined;
      this.status.saving = true;
      this.publish();
      try {
        const state = await result(await apiFetch(this.url, {
          method: 'PUT', keepalive: this.keepalive,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ source_revision: this.state.source_revision, expected_version: this.state.version, position })
        }));
        this.state = state;
        this.lastSaved = JSON.stringify(position);
        this.status.saved = !this.pending;
      } catch (error) {
        this.pending ||= position;
        this.status.error = error instanceof Error ? error.message : String(error);
        this.status.saved = false;
      } finally {
        this.status.saving = false;
        this.publish();
      }
    }
  }

  /** retry explicitly elects this window's current place over a conflicting save. */
  async retry() {
    if (this.active || !this.current) return;
    try {
      const latest = await result(await apiFetch(this.url));
      if (this.closed) return;
      if (this.state && latest.source_revision !== this.state.source_revision) throw new Error('The original changed. Reopen the reader before saving a place.');
      this.state = latest;
      this.status.error = '';
      this.pending = this.current;
      await this.flush();
    } catch (error) {
      this.status.error = error instanceof Error ? error.message : String(error);
      this.publish();
    }
  }

  close() {
    this.closed = true;
    clearTimeout(this.timer);
    void this.flush(true);
  }
}
