import { apiFetch } from './api';
import { API_BASE_URL } from './config';
import type { LibraryItem } from './libraryTypes';

/** reextractText refreshes a stored original's derived text, not its editable metadata. */
export async function reextractText(id: string, signal: AbortSignal): Promise<LibraryItem> {
  const response = await apiFetch(`${API_BASE_URL}/file/reextract/${encodeURIComponent(id)}`, {
    method: 'POST', signal
  });
  const data = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error(data?.error || `Could not refresh extracted text (${response.status}).`);
  }
  if (!data?.metadata || data.metadata.Uuid !== id) {
    throw new Error('The server returned invalid extraction metadata. Refresh the library to check its current status.');
  }
  return data.metadata;
}
