<script lang="ts">
  import { onMount } from 'svelte';
  import { API_BASE_URL } from '../config';

  export let item: any;
  export let onSave: () => void;
  export let onCancel: () => void;

  let title = item.Title || '';
  let author = item.Author || '';
  let docType = item.DocType || '';
  let deweyDecimal = item.DeweyDecimal || '';
  let publishDate = item.PublishDate || '';
  let saving = false;
  let error = '';

  let docTypes: string[] = [];
  let deweyCategories: { code: string; name: string }[] = [];

  async function loadOptions() {
    try {
      const [typesRes, deweyRes] = await Promise.all([
        fetch(`${API_BASE_URL}/data/types`),
        fetch(`${API_BASE_URL}/data/dewey`)
      ]);
      if (typesRes.ok) {
        const data = await typesRes.json();
        docTypes = data.types || [];
      }
      if (deweyRes.ok) {
        const data = await deweyRes.json();
        deweyCategories = data.categories || [];
      }
    } catch {
      // use defaults if API is unavailable
    }
  }

  async function save() {
    saving = true;
    error = '';
    try {
      const response = await fetch(`${API_BASE_URL}/data/update`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          Uuid: item.Uuid,
          DocType: docType,
          Title: title,
          Author: author,
          PublishDate: publishDate,
          DeweyDecimal: deweyDecimal,
          Path: item.Path,
          FileType: item.FileType,
        })
      });

      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error || 'Update failed');
      }
      onSave();
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  function getDeweyLabel(code: string): string {
    const cat = deweyCategories.find(c => c.code === code);
    return cat ? `${cat.code} - ${cat.name}` : code;
  }

  onMount(loadOptions);
</script>

<div class="edit-overlay" on:click={onCancel}>
  <div class="edit-panel" on:click|stopPropagation>
    <div class="edit-header">
      <h2>Edit Document</h2>
      <button class="close-btn" on:click={onCancel}>
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>

    {#if error}
      <div class="error-banner">{error}</div>
    {/if}

    <div class="edit-form">
      <div class="form-group">
        <label for="edit-title">Title</label>
        <input id="edit-title" type="text" bind:value={title} placeholder="Document title" />
      </div>

      <div class="form-group">
        <label for="edit-author">Author</label>
        <input id="edit-author" type="text" bind:value={author} placeholder="Author name" />
      </div>

      <div class="form-row">
        <div class="form-group">
          <label for="edit-doctype">Document Type</label>
          {#if docTypes.length > 0}
            <select id="edit-doctype" bind:value={docType}>
              {#each docTypes as dt}
                <option value={dt}>{dt}</option>
              {/each}
            </select>
          {:else}
            <input id="edit-doctype" type="text" bind:value={docType} placeholder="e.g. Notes" />
          {/if}
        </div>

        <div class="form-group">
          <label for="edit-date">Publish Date</label>
          <input id="edit-date" type="date" bind:value={publishDate} />
        </div>
      </div>

      <div class="form-group">
        <label for="edit-dewey">Dewey Decimal Classification</label>
        {#if deweyCategories.length > 0}
          <select id="edit-dewey" bind:value={deweyDecimal}>
            <option value="">-- None --</option>
            {#each deweyCategories as cat}
              <option value={cat.code}>{cat.code} - {cat.name}</option>
            {/each}
          </select>
        {:else}
          <input id="edit-dewey" type="text" bind:value={deweyDecimal} placeholder="e.g. 510" />
        {/if}
      </div>

      <div class="form-info">
        <div class="info-row">
          <span class="info-label">File Type:</span>
          <span>{item.FileType || 'N/A'}</span>
        </div>
        <div class="info-row">
          <span class="info-label">UUID:</span>
          <span class="mono">{item.Uuid}</span>
        </div>
      </div>
    </div>

    <div class="edit-actions">
      <button class="btn btn-secondary" on:click={onCancel} disabled={saving}>Cancel</button>
      <button class="btn btn-primary" on:click={save} disabled={saving}>
        {#if saving}Saving...{:else}Save Changes{/if}
      </button>
    </div>
  </div>
</div>

<style>
  .edit-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(10px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 3000;
    padding: 24px;
  }

  .edit-panel {
    background: rgba(44, 44, 46, 0.98);
    border-radius: 16px;
    max-width: 560px;
    width: 100%;
    max-height: 85vh;
    overflow-y: auto;
    border: 1px solid rgba(255, 255, 255, 0.15);
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.4);
  }

  .edit-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 20px 24px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  }

  .edit-header h2 {
    font-size: 20px;
    font-weight: 700;
    color: #ffffff;
    margin: 0;
  }

  .close-btn {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    padding: 6px;
    border-radius: 6px;
    transition: all 0.2s ease;
  }

  .close-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .error-banner {
    margin: 16px 24px 0;
    padding: 10px 14px;
    background: rgba(255, 59, 48, 0.12);
    border: 1px solid rgba(255, 59, 48, 0.25);
    border-radius: 8px;
    color: #FF3B30;
    font-size: 14px;
  }

  .edit-form {
    padding: 20px 24px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .form-group label {
    font-size: 13px;
    font-weight: 600;
    color: rgba(255, 255, 255, 0.7);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .form-group input,
  .form-group select {
    padding: 10px 12px;
    background: rgba(255, 255, 255, 0.08);
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 8px;
    color: #ffffff;
    font-size: 15px;
    font-family: inherit;
    transition: border-color 0.2s ease;
  }

  .form-group input:focus,
  .form-group select:focus {
    outline: none;
    border-color: #007AFF;
    background: rgba(255, 255, 255, 0.12);
  }

  .form-group select option {
    background: #2c2c2e;
    color: #ffffff;
  }

  .form-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }

  .form-info {
    padding: 12px;
    background: rgba(255, 255, 255, 0.04);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .info-row {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.6);
  }

  .info-label {
    font-weight: 600;
    color: rgba(255, 255, 255, 0.5);
    min-width: 70px;
  }

  .mono {
    font-family: 'Monaco', 'Menlo', monospace;
    font-size: 12px;
  }

  .edit-actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    padding: 16px 24px;
    border-top: 1px solid rgba(255, 255, 255, 0.1);
  }

  .btn {
    padding: 10px 20px;
    border: none;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
  }

  .btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-primary {
    background: #007AFF;
    color: #ffffff;
  }

  .btn-primary:hover:not(:disabled) {
    background: #0056CC;
  }

  .btn-secondary {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .btn-secondary:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.18);
  }

  @media (max-width: 480px) {
    .form-row {
      grid-template-columns: 1fr;
    }
  }
</style>
