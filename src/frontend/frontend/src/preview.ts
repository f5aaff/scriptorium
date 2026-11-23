import { apiFetch } from './api';
import { API_BASE_URL } from './config';

export const MAX_PREVIEW_BYTES = 100 * 1024 * 1024;
export const MAX_TEXT_PREVIEW_BYTES = 1024 * 1024;
export type PreviewKind = 'pdf' | 'image' | 'text' | 'audio' | 'video';
export type LoadedPreview =
  | { kind: 'text'; text: string; truncated: boolean }
  | { kind: 'pdf' | 'image' | 'audio' | 'video'; data: ArrayBuffer; mediaType: string };

const mediaTypes: Record<PreviewKind, readonly string[]> = {
  pdf: ['application/pdf'],
  image: ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/bmp', 'image/x-icon'],
  text: ['text/plain'],
  audio: ['audio/mpeg', 'audio/wave', 'audio/aiff', 'audio/midi', 'audio/ogg'],
  video: ['video/mp4', 'video/webm', 'video/avi', 'video/ogg']
};

/** loadPreview fetches an authenticated original without placing credentials in URLs.
 * Text is a bounded UTF-8 prefix; other inputs are rejected beyond 100 MiB.
 * The server's preview kind and MIME must agree with the client's inert allowlist.
 */
export async function loadPreview(id: string, signal: AbortSignal): Promise<LoadedPreview> {
  const response = await apiFetch(`${API_BASE_URL}/file/preview/${encodeURIComponent(id)}`, { signal });
  if (!response.ok) {
    const error = await response.json().catch(() => null);
    throw new Error(error?.error || `Preview unavailable (${response.status}). Download the original instead.`);
  }
  const kind = response.headers.get('X-Scriptorium-Preview-Kind') as PreviewKind;
  const mediaType = (response.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase();
  if (!Object.hasOwn(mediaTypes, kind) || !mediaTypes[kind].includes(mediaType)) {
    await response.body?.cancel();
    throw new Error('This file cannot be safely previewed. Download the original instead.');
  }
  const text = kind === 'text';
  const limit = text ? MAX_TEXT_PREVIEW_BYTES : MAX_PREVIEW_BYTES;
  if (!text && Number(response.headers.get('Content-Length')) > limit) {
    await response.body?.cancel();
    throw new Error('This file exceeds the 100 MiB preview limit. Download the original instead.');
  }
  if (!response.body) throw new Error('No preview content was returned.');
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  let truncated = false;
  const abort = () => { void reader.cancel().catch(() => {}); };
  signal.addEventListener('abort', abort, { once: true });
  try {
    while (true) {
      if (signal.aborted) throw new DOMException('Preview cancelled', 'AbortError');
      const { value, done } = await reader.read();
      if (signal.aborted) throw new DOMException('Preview cancelled', 'AbortError');
      if (done) break;
      if (size + value.byteLength > limit) {
        if (!text) {
          await reader.cancel();
          throw new Error('This file exceeds the 100 MiB preview limit. Download the original instead.');
        }
        chunks.push(value.slice(0, limit - size));
        size = limit;
        truncated = true;
        await reader.cancel();
        break;
      }
      chunks.push(value);
      size += value.byteLength;
    }
  } finally {
    signal.removeEventListener('abort', abort);
    if (signal.aborted) await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  if (kind === 'text') {
    try {
      // Streaming mode holds back an incomplete final rune on truncated input.
      const decoded = new TextDecoder('utf-8', { fatal: true }).decode(bytes, { stream: truncated });
      return { kind, text: decoded, truncated };
    } catch {
      throw new Error('This file is not valid UTF-8 text. Download the original instead.');
    }
  }
  return { kind, data: bytes.buffer, mediaType };
}
