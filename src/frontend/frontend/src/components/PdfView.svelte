<script lang="ts">
  import { onMount } from 'svelte';
  import { openPdf, type PdfDocument } from '../pdfRenderer';

  export let data: ArrayBuffer;
  let canvas: HTMLCanvasElement;
  let pdf: PdfDocument | undefined;
  let page = 1;
  let requestedPage = 1;
  let pages = 0;
  let zoom = 1;
  let actualScale = 1;
  let busy = true;
  let error = '';
  let textVisible = false;
  let textBusy = false;
  let pageText = '';
  let textTruncated = false;
  let textError = '';
  const lifetime = new AbortController();
  let rendering: AbortController | undefined;
  let readingText: AbortController | undefined;
  let renderTimer: ReturnType<typeof setTimeout> | undefined;
  let textTimer: ReturnType<typeof setTimeout> | undefined;

  async function renderCurrent() {
    if (!pdf || lifetime.signal.aborted) return;
    rendering?.abort();
    readingText?.abort();
    clearTimeout(renderTimer);
    clearTimeout(textTimer);
    const request = new AbortController();
    rendering = request;
    busy = true;
    error = '';
    textVisible = false;
    textBusy = false;
    pageText = '';
    textError = '';
    canvas.width = canvas.height = 1;
    const timer = setTimeout(() => {
      request.abort();
      if (!lifetime.signal.aborted && rendering === request) {
        error = 'PDF page rendering timed out. Try another page or download the original.';
        busy = false;
      }
    }, 30000);
    renderTimer = timer;
    try {
      const scale = await pdf.renderPage(canvas, page, zoom, request.signal);
      if (!request.signal.aborted && !lifetime.signal.aborted) actualScale = scale;
    } catch {
      if (!request.signal.aborted && !lifetime.signal.aborted) {
        error = 'This PDF page could not be rendered. Download the original instead.';
      }
    } finally {
      clearTimeout(timer);
      if (!request.signal.aborted && !lifetime.signal.aborted) busy = false;
    }
  }

  function navigate(next: number) {
    if (busy || !pdf || !Number.isInteger(next) || next < 1 || next > pages) return;
    page = requestedPage = next;
    void renderCurrent();
  }

  function changeZoom(next: number) {
    if (busy || !pdf) return;
    zoom = Math.max(.5, Math.min(3, next));
    void renderCurrent();
  }

  async function toggleText() {
    textVisible = !textVisible;
    readingText?.abort();
    clearTimeout(textTimer);
    if (!textVisible || !pdf) return;
    const request = new AbortController();
    readingText = request;
    textBusy = true;
    textError = '';
    const timer = setTimeout(() => {
      request.abort();
      if (!lifetime.signal.aborted && readingText === request) { textError = 'Page text extraction timed out.'; textBusy = false; }
    }, 30000);
    textTimer = timer;
    try {
      const result = await pdf.pageText(page, request.signal);
      if (!request.signal.aborted && !lifetime.signal.aborted) {
        pageText = result.text;
        textTruncated = result.truncated;
      }
    } catch {
      if (!request.signal.aborted && !lifetime.signal.aborted) textError = 'Selectable text is unavailable for this page.';
    } finally {
      clearTimeout(timer);
      if (!request.signal.aborted && !lifetime.signal.aborted) textBusy = false;
    }
  }

  onMount(() => {
    void openPdf(data, lifetime.signal).then(async document => {
      if (lifetime.signal.aborted) { await document.destroy(); return; }
      pdf = document;
      pages = pdf.pageCount;
      await renderCurrent();
    }).catch(failure => {
      if (lifetime.signal.aborted) return;
      error = failure?.name === 'PasswordException'
        ? 'Password-protected PDFs cannot be previewed here. Download the original instead.'
        : 'This PDF could not be loaded. Download the original instead.';
      busy = false;
    });
    return () => {
      lifetime.abort();
      clearTimeout(renderTimer);
      clearTimeout(textTimer);
      rendering?.abort();
      readingText?.abort();
      void pdf?.destroy().catch(() => {});
    };
  });
</script>

<section class="pdf-view" aria-label="PDF reader" aria-busy={busy}>
  <div class="controls">
    <button on:click={() => navigate(page - 1)} disabled={busy || page <= 1}>Previous page</button>
    <form on:submit|preventDefault={() => navigate(requestedPage)}>
      <label>Page <input type="number" min="1" max={pages || 1} bind:value={requestedPage} disabled={busy || !pdf} required /></label>
      <span>of {pages}</span>
      <button type="submit" disabled={busy || !pdf}>Go</button>
    </form>
    <button on:click={() => navigate(page + 1)} disabled={busy || !pdf || page >= pages}>Next page</button>
    <button on:click={() => changeZoom(zoom - .25)} disabled={busy || !pdf || zoom <= .5} aria-label="Zoom out">−</button>
    <span>{Math.round(zoom * 100)}%</span>
    <button on:click={() => changeZoom(zoom + .25)} disabled={busy || !pdf || zoom >= 3} aria-label="Zoom in">+</button>
    <button on:click={toggleText} disabled={busy || !pdf || !!error} aria-expanded={textVisible}>{textVisible ? 'Hide page text' : 'Show page text'}</button>
  </div>
  {#if busy}<p role="status">Loading PDF page…</p>{/if}
  {#if error}<p role="alert">{error}</p>{/if}
  {#if actualScale < zoom && !busy}<p>Large page scaled down to stay within render limits.</p>{/if}
  <div class="page" hidden={!!error} role="img" aria-label={`PDF page ${page}`}>
    <canvas bind:this={canvas} width="1" height="1" aria-hidden="true"></canvas>
  </div>
  {#if textVisible}
    <section class="page-text" aria-label="Selectable page text" aria-busy={textBusy}>
      {#if textBusy}<p role="status">Reading page text…</p>
      {:else if textError}<p role="alert">{textError}</p>
      {:else}
        {#if textTruncated}<p>Page text is truncated at the preview limit.</p>{/if}
        {#if pageText.trim()}<pre>{pageText}</pre>{:else}<p>No selectable text on this page.</p>{/if}
      {/if}
    </section>
  {/if}
  <p class="hint">Use “Show page text” for selectable text. Large pages and images are limited for safety; download the original if rendering appears incomplete. PDF scripts, forms, and links are not activated.</p>
</section>

<style>
  .pdf-view { min-width: 0; }
  .controls, form { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
  .controls { margin-bottom: 16px; }
  button, input { border: 1px solid #777; border-radius: 6px; background: #303036; color: white; padding: 8px; }
  button { cursor: pointer; }
  button:disabled { opacity: .5; cursor: default; }
  input { width: 80px; }
  button:focus-visible, input:focus-visible { outline: 2px solid #64aaff; outline-offset: 2px; }
  .page { overflow: auto; max-height: 65vh; background: #444; }
  canvas { display: block; margin: auto; background: white; }
  .page-text { margin-top: 16px; }
  pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 40vh; overflow-y: auto; }
  p { margin: 12px 0; }
  [role="alert"] { color: #ffb5b5; }
  .hint { color: #bbb; font-size: .85rem; }
</style>
