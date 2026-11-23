<script lang="ts">
  import { createEventDispatcher, onMount, tick } from 'svelte';
  import type { ReadingPosition } from '../readingState';

  export let text: string;
  export let initialPosition: ReadingPosition | null = null;
  const dispatch = createEventDispatcher<{ position: ReadingPosition }>();
  let area: HTMLTextAreaElement;
  let page = 1;
  let requestedPage = 1;
  let ready = false;
  let closed = false;

  // Keep every character, including newlines and surrogate pairs, while limiting
  // DOM/layout work to one section. This is pagination, never prefix truncation.
  function sections(value: string) {
    const offsets = [0];
    let start = 0;
    while (start < value.length) {
      let end = Math.min(start + 32768, value.length);
      if (end < value.length) {
        const newline = value.slice(start, end).lastIndexOf('\n');
        if (newline >= 24576) end = start + newline + 1;
        if (/[\uD800-\uDBFF]/.test(value[end - 1])) end--;
      }
      offsets.push(end);
      start = end;
    }
    if (offsets.length === 1) offsets.push(0);
    return offsets;
  }
  $: offsets = sections(text);
  $: pages = offsets.length - 1;
  $: content = text.slice(offsets[page - 1], offsets[page]);

  function capture() {
    if (!ready || closed) return;
    const distance = area.scrollHeight - area.clientHeight;
    dispatch('position', { kind: 'text', page, offset: distance > 0 ? Math.min(1, Math.max(0, area.scrollTop / distance)) : 0, scale: 1, locator: String(offsets[page - 1]) });
  }
  async function navigate(next: number, offset = 0) {
    if (!Number.isInteger(next) || next < 1 || next > pages) return;
    ready = false;
    page = requestedPage = next;
    await tick();
    if (closed) return;
    area.scrollTop = offset * Math.max(0, area.scrollHeight - area.clientHeight);
    ready = true;
    capture();
  }
  onMount(() => {
    const saved = initialPosition?.kind === 'text' ? initialPosition : null;
    let section = Math.min(pages, Math.max(1, saved?.page || 1));
    if (saved?.locator) {
      const character = Number(saved.locator);
      if (Number.isSafeInteger(character) && character >= 0 && character < text.length) {
        section = offsets.findIndex((end, index) => index > 0 && end > character);
      }
    }
    void navigate(section, saved?.offset || 0);
    return () => { closed = true; };
  });
</script>

<section aria-label="Text reader">
  <div class="controls">
    <button disabled={page <= 1} on:click={() => navigate(page - 1)}>Previous section</button>
    <form on:submit|preventDefault={() => navigate(requestedPage)}>
      <label>Section <input type="number" min="1" max={pages} bind:value={requestedPage} required /></label>
      <span>of {pages}</span><button type="submit">Go</button>
    </form>
    <button disabled={page >= pages} on:click={() => navigate(page + 1)}>Next section</button>
  </div>
  <textarea bind:this={area} readonly aria-label="Document text" rows="25" value={content} placeholder="(Empty document)" on:scroll={capture}></textarea>
  <p>The complete document is available across {pages} {pages === 1 ? 'section' : 'sections'}. Text is selectable; no markup or scripts are executed.</p>
</section>

<style>
  .controls, form { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-bottom: 8px; }
  button, input { background: #303036; color: white; border: 1px solid #777; border-radius: 6px; padding: 8px; }
  input { width: 85px; }
  button:disabled { opacity: .5; }
  textarea { display: block; width: 100%; height: 60vh; padding: 16px; box-sizing: border-box; resize: vertical; white-space: pre-wrap; font: 1rem/1.6 monospace; color: #eee; background: #17171b; border: 1px solid #666; }
  button:focus-visible, input:focus-visible, textarea:focus-visible { outline: 2px solid #64aaff; outline-offset: 2px; }
  p { font-size: .85rem; color: #bbb; }
</style>
