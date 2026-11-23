<script lang="ts">
  import { createEventDispatcher, onDestroy } from 'svelte';
  import { reextractText } from '../reextract';
  import type { LibraryItem } from '../libraryTypes';

  export let documentID: string;
  const dispatch = createEventDispatcher<{ updated: LibraryItem }>();
  let request: AbortController | undefined;
  let busy = false;
  let error = '';
  let notice = '';

  async function reextract() {
    if (busy) return;
    const controller = new AbortController();
    request = controller;
    busy = true;
    error = '';
    notice = '';
    try {
      const metadata = await reextractText(documentID, controller.signal);
      if (controller.signal.aborted) return;
      dispatch('updated', metadata);
      notice = 'Text extraction and search index refreshed.';
    } catch (failure) {
      if (!controller.signal.aborted) {
        error = failure instanceof Error ? failure.message : String(failure);
      }
    } finally {
      if (!controller.signal.aborted) busy = false;
    }
  }

  // The parent keys this component by document ID, so navigation and switching
  // records abort old work rather than applying a late response to another item.
  onDestroy(() => request?.abort());
</script>

<div class="reextract-action" aria-busy={busy}>
  <button type="button" on:click={reextract} disabled={busy}>
    {busy ? 'Re-extracting text…' : 'Re-extract text'}
  </button>
  {#if busy}<p role="status">Refreshing text from the stored original…</p>{/if}
  {#if notice}<p role="status">{notice}</p>{/if}
  {#if error}<p role="alert">{error}</p>{/if}
</div>

<style>
  .reextract-action { flex-basis: 100%; }
  button { padding: 12px 24px; border-radius: 8px; border: 1px solid #666; background: #303036; color: #fff; cursor: pointer; }
  button:disabled { opacity: .6; cursor: wait; }
  button:focus-visible { outline: 2px solid #64aaff; outline-offset: 3px; }
  p { margin: 10px 0; font-size: .9rem; }
  [role="alert"] { color: #ffb5b5; }
</style>
