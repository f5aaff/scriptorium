<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import { loadPreview, type LoadedPreview } from '../preview';
  import type { LibraryItem } from '../libraryTypes';
  import PdfView from './PdfView.svelte';

  export let item: LibraryItem;
  const dispatch = createEventDispatcher<{ close: void; download: LibraryItem }>();
  let dialog: HTMLElement;
  let closeButton: HTMLButtonElement;
  let request: AbortController | undefined;
  let preview: LoadedPreview | null = null;
  let objectURL = '';
  let loading = true;
  let error = '';
  let mediaFailed = false;

  function revokeObjectURL() {
    if (objectURL) URL.revokeObjectURL(objectURL);
    objectURL = '';
  }

  async function load() {
    request?.abort();
    const current = new AbortController();
    request = current;
    revokeObjectURL();
    preview = null;
    error = '';
    mediaFailed = false;
    loading = true;
    try {
      const result = await loadPreview(item.Uuid, current.signal);
      if (current.signal.aborted || request !== current) return;
      if (result.kind !== 'text' && result.kind !== 'pdf') {
        objectURL = URL.createObjectURL(new Blob([result.data], { type: result.mediaType }));
      }
      preview = result;
    } catch (failure) {
      if (!current.signal.aborted && request === current) error = failure instanceof Error ? failure.message : String(failure);
    } finally {
      if (!current.signal.aborted && request === current) loading = false;
    }
  }

  function handleKey(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      dispatch('close');
    } else if (event.key === 'Tab') {
      const controls = Array.from(dialog.querySelectorAll<HTMLElement>(
        'button:not([disabled]), input:not([disabled]), audio[controls], video[controls], textarea:not([disabled]), [tabindex="0"]'
      )).filter(element => !element.closest('[hidden]'));
      const first = controls[0];
      const last = controls[controls.length - 1];
      if (!first) return;
      if (!dialog.contains(document.activeElement) || (event.shiftKey && document.activeElement === first)) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
  }

  onMount(() => {
    const previousFocus = document.activeElement;
    closeButton.focus();
    window.addEventListener('keydown', handleKey);
    void load();
    return () => {
      request?.abort();
      revokeObjectURL();
      window.removeEventListener('keydown', handleKey);
      // Let the parent reveal the underlying details before restoring focus.
      queueMicrotask(() => {
        if (previousFocus instanceof HTMLElement && previousFocus.isConnected) previousFocus.focus();
      });
    };
  });
</script>

<div class="viewer-overlay">
  <section bind:this={dialog} class="viewer" role="dialog" aria-modal="true" aria-labelledby="file-viewer-title" tabindex="-1">
    <header>
      <div>
        <h2 id="file-viewer-title">{item.Title || item.OriginalFilename || item.Uuid}</h2>
        <p>{item.OriginalFilename || item.FileType}</p>
      </div>
      <button on:click={() => dispatch('download', item)}>Download original</button>
      <button bind:this={closeButton} on:click={() => dispatch('close')}>Close viewer</button>
    </header>
    <div class="content" aria-busy={loading}>
      {#if loading}<p role="status">Loading file preview…</p>{/if}
      {#if error}
        <div role="alert"><p>{error}</p></div>
        <button on:click={load}>Retry preview</button>
      {/if}
      {#if preview}
        {#if preview.kind === 'pdf'}
          <PdfView data={preview.data} />
        {:else if preview.kind === 'text'}
          {#if preview.truncated}<p role="status">Showing the first 1 MiB of text. Download the original for the complete file.</p>{/if}
          <p class="hint">Source text only: markup, scripts, and external resources are not executed.</p>
          <textarea readonly aria-label="File source text" class="source-text" rows="20" value={preview.text} placeholder="(Empty text file)"></textarea>
        {:else if mediaFailed}
          <p role="alert">Your browser could not decode this file or codec. Download the original instead.</p>
        {:else if preview.kind === 'image'}
          <img src={objectURL} alt={item.Title || item.OriginalFilename || 'Library image'} on:error={() => mediaFailed = true} />
        {:else if preview.kind === 'audio'}
          <audio controls src={objectURL} preload="metadata" on:error={() => mediaFailed = true}>Your browser does not support audio playback.</audio>
        {:else if preview.kind === 'video'}
          <!-- Uploaded originals do not yet have associated caption tracks. -->
          <!-- svelte-ignore a11y-media-has-caption -->
          <video controls playsinline src={objectURL} preload="metadata" on:error={() => mediaFailed = true}>Your browser does not support video playback.</video>
        {/if}
      {/if}
    </div>
  </section>
</div>

<style>
  .viewer-overlay { position: fixed; inset: 0; z-index: 2500; background: rgba(0,0,0,.85); display: grid; place-items: center; padding: 20px; }
  .viewer { width: min(1200px, 100%); max-height: 95vh; display: flex; flex-direction: column; border: 1px solid #666; border-radius: 12px; background: #202024; color: #eee; min-width: 0; }
  header { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; padding: 16px; border-bottom: 1px solid #555; }
  header div { flex: 1; min-width: 120px; overflow-wrap: anywhere; }
  h2 { font-size: 1.2rem; margin: 0; }
  p { margin: 10px 0; }
  header p { font-size: .85rem; color: #bbb; }
  button { background: #303036; color: white; border: 1px solid #777; border-radius: 6px; padding: 10px; cursor: pointer; }
  button:focus-visible, textarea:focus-visible { outline: 2px solid #64aaff; outline-offset: 2px; }
  .content { padding: 16px; overflow: auto; min-height: 120px; min-width: 0; }
  img, video { display: block; max-width: 100%; max-height: 70vh; margin: auto; object-fit: contain; }
  audio { width: 100%; }
  .source-text { display: block; width: 100%; max-height: 65vh; padding: 12px; box-sizing: border-box; resize: vertical; white-space: pre-wrap; font-family: monospace; color: #eee; background: #17171b; border: 1px solid #666; }
  [role="alert"] { color: #ffb5b5; }
  .hint { color: #bbb; font-size: .85rem; }
  @media (max-width: 600px) { .viewer-overlay { padding: 4px; } .viewer { max-height: 99vh; } }
</style>
