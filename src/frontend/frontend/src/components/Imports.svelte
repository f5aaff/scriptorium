<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import {
    getImportJob, listImportJobs, isActiveImport, importStatusLabels, type ImportJob
  } from '../imports';

  export let initialJobID = '';
  const dispatch = createEventDispatcher<{ openDocument: string }>();
  let jobs: ImportJob[] = [];
  let selectedID = initialJobID;
  let selected: ImportJob | null = null;
  let loading = true;
  let detailLoading = false;
  let loaded = false;
  let error = '';
  let detailError = '';
  let disposed = false;
  let listRequest: AbortController | undefined;
  let detailRequest: AbortController | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  async function loadDetail(id: string) {
    detailRequest?.abort();
    const request = new AbortController();
    detailRequest = request;
    detailLoading = true;
    detailError = '';
    try {
      const job = await getImportJob(id, request.signal);
      if (!disposed && detailRequest === request) selected = job;
    } catch (failure) {
      if (!disposed && detailRequest === request) detailError = message(failure);
    } finally {
      if (!disposed && detailRequest === request) detailLoading = false;
    }
  }

  async function selectJob(id: string) {
    selectedID = id;
    selected = null;
    await loadDetail(id);
  }

  async function refresh() {
    clearTimeout(timer);
    listRequest?.abort();
    const request = new AbortController();
    listRequest = request;
    loading = true;
    error = '';
    try {
      const recent = await listImportJobs(request.signal);
      if (disposed || listRequest !== request) return;
      jobs = recent;
      loaded = true;
      if (selectedID) await loadDetail(selectedID);
      if (disposed || listRequest !== request) return;
      // Schedule after completion rather than using an interval: slow requests
      // must not overlap or replace more recent progress with older responses.
      if (jobs.some(isActiveImport) || (selected && isActiveImport(selected))) {
        timer = setTimeout(() => { void refresh(); }, 2000);
      }
    } catch (failure) {
      if (!disposed && listRequest === request) error = message(failure);
    } finally {
      if (!disposed && listRequest === request) loading = false;
    }
  }

  onMount(() => {
    void refresh();
    return () => {
      disposed = true;
      clearTimeout(timer);
      listRequest?.abort();
      detailRequest?.abort();
    };
  });
</script>

<section class="imports" aria-label="Import management">
  <div class="toolbar">
    <div>
      <h2>Import history</h2>
      <p>The latest 25 jobs in this shared library. Active jobs update automatically.</p>
    </div>
    <button on:click={refresh} disabled={loading}>Refresh imports</button>
  </div>

  {#if error}
    <div role="alert">
      <p>{error}</p>
      <button on:click={refresh} disabled={loading}>Retry imports</button>
    </div>
  {/if}
  {#if loading}<p role="status">Updating imports…</p>{/if}
  {#if loaded && !loading && !error && jobs.length === 0}
    <p role="status">No imports yet. Batch uploads will appear here.</p>
  {/if}

  <div class="jobs" aria-busy={loading}>
    {#each jobs as job (job.id)}
      <button class="job" class:selected={selectedID === job.id} on:click={() => selectJob(job.id)} aria-pressed={selectedID === job.id}>
        <strong>Import {job.id}</strong>
        <span>{importStatusLabels[job.status]}</span>
        <span>{job.processed} / {job.total} processed · {job.failed} failed</span>
        <time datetime={job.created_at}>{job.created_at}</time>
      </button>
    {/each}
  </div>

  {#if selectedID}
    <section class="details" aria-label="Import details" aria-busy={detailLoading}>
      <h3>Import {selectedID}</h3>
      {#if detailLoading}<p role="status">Updating file results…</p>{/if}
      {#if detailError}
        <div role="alert">
          <p>{detailError}</p>
          <button on:click={() => loadDetail(selectedID)}>Retry details</button>
        </div>
      {/if}
      {#if selected}
        <p>{importStatusLabels[selected.status]} · {selected.processed} / {selected.total} processed · {selected.failed} failed</p>
        <progress max={selected.total || 1} value={selected.processed} aria-label="Import progress"></progress>
        {#if selected.error}<p class="job-error">{selected.error}</p>{/if}
        {#if selected.results?.length}
          <ul class="results">
            {#each selected.results as result}
              <li>
                <strong>{result.filename}</strong>
                <span>{result.status === 'duplicate' ? 'Duplicate — already in library' : result.status === 'imported' ? 'Imported' : 'Failed'}</span>
                {#if result.error}<p>{result.error}</p>{/if}
                {#if result.document_id}
                  <button on:click={() => dispatch('openDocument', result.document_id || '')} aria-label={`Open ${result.filename} in library`}>View in library</button>
                {/if}
              </li>
            {/each}
          </ul>
        {:else if !detailError}
          <p>{isActiveImport(selected) ? 'File results will appear as processing completes.' : 'Per-file results are not available for this job. Older jobs retain aggregate progress only.'}</p>
        {/if}
      {/if}
    </section>
  {/if}
</section>

<style>
  .imports { height: 100%; overflow-y: auto; padding: 24px; color: #eee; }
  .toolbar { display: flex; justify-content: space-between; align-items: start; gap: 16px; margin-bottom: 20px; }
  h2, h3 { margin-bottom: 10px; }
  p { margin: 8px 0; }
  button { background: #303036; color: #fff; border: 1px solid #666; border-radius: 8px; padding: 10px 14px; cursor: pointer; }
  button:disabled { opacity: .6; cursor: wait; }
  button:focus-visible { outline: 2px solid #64aaff; outline-offset: 3px; }
  .jobs { display: grid; gap: 12px; margin: 16px 0; }
  .job { display: flex; flex-wrap: wrap; gap: 12px; text-align: left; overflow-wrap: anywhere; }
  .job.selected { border-color: #64aaff; }
  time { font-size: .85rem; color: #bbb; }
  .details { border-top: 1px solid #666; padding-top: 20px; overflow-wrap: anywhere; }
  progress { width: 100%; margin: 12px 0; }
  .results { list-style: none; padding: 0; }
  .results li { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 14px 0; border-bottom: 1px solid #444; }
  .results li p { flex-basis: 100%; }
  [role="alert"], .job-error { color: #ffb5b5; }
</style>
