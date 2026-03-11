<script lang="ts">
  import { onMount } from 'svelte';
  import ItemCard from './ItemCard.svelte';
  import EditModal from './EditModal.svelte';
  import { API_BASE_URL } from '../config';

  interface LibraryItem {
    Title: string;
    Author: string;
    PublishDate: string;
    LastUpdated: string;
    FileType: string;
    DocType: string;
    DeweyDecimal: string;
    Path: string;
    Uuid: string;
  }

  interface SearchResponse {
    message: string;
    count: number;
    total_count: number;
    page: number;
    limit: number;
    total_pages: number;
    has_next: boolean;
    has_prev: boolean;
    results: LibraryItem[];
  }

  let items: LibraryItem[] = [];
  let filteredItems: LibraryItem[] = [];
  let loading = false;
  let selectedItem: LibraryItem | null = null;
  let showCard = false;
  let page = 1;
  let hasMore = true;
  let searchQuery = '';
  let searchFocused = false;
  let searchKey = '';
  let searchValue = '';
  let searchTimeout: number | null = null;
  let isInitialLoad = true;
  let editingItem: LibraryItem | null = null;
  let converting = false;

  async function fetchItems(pageNum: number, searchKeyParam?: string, searchValueParam?: string, fuzzyQuery?: string): Promise<{ items: LibraryItem[], hasMore: boolean }> {
    const params = new URLSearchParams();
    params.append('page', pageNum.toString());
    params.append('limit', '20');

    if (fuzzyQuery) {
      params.append('q', fuzzyQuery);
    } else if (searchKeyParam && searchValueParam) {
      params.append('key', searchKeyParam);
      params.append('value', searchValueParam);
    }

    const url = `${API_BASE_URL}/data/search?${params.toString()}`;

    const response = await fetch(url);

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const data: SearchResponse = await response.json();

    return {
      items: data.results || [],
      hasMore: data.has_next || false
    };
  }

  async function performSearch() {
    if (!searchQuery.trim()) {
      await loadMoreItems();
      return;
    }

    loading = true;
    try {
      let key = '';
      let value = '';
      let fuzzy = '';

      if (searchQuery.toLowerCase().startsWith('author:')) {
        key = 'Author';
        value = searchQuery.replace(/^author:\s*/i, '');
      } else if (searchQuery.toLowerCase().startsWith('type:')) {
        key = 'DocType';
        value = searchQuery.replace(/^type:\s*/i, '');
      } else if (searchQuery.toLowerCase().startsWith('filetype:')) {
        key = 'FileType';
        value = searchQuery.replace(/^filetype:\s*/i, '');
      } else if (searchQuery.toLowerCase().startsWith('dewey:')) {
        key = 'DeweyDecimal';
        value = searchQuery.replace(/^dewey:\s*/i, '');
      } else {
        fuzzy = searchQuery;
      }

      const { items: searchResults, hasMore: hasMoreResults } = fuzzy
        ? await fetchItems(1, undefined, undefined, fuzzy)
        : await fetchItems(1, key, value);

      items = searchResults;
      filteredItems = searchResults;
      hasMore = hasMoreResults;
      page = 1;
    } catch (error) {
      console.error('Search failed:', error);
    } finally {
      loading = false;
    }
  }

  $: {
    if (searchQuery.trim()) {
      if (searchTimeout) {
        clearTimeout(searchTimeout);
      }
      searchTimeout = setTimeout(() => {
        performSearch();
      }, 300);
    } else if (!isInitialLoad) {
      if (searchTimeout) {
        clearTimeout(searchTimeout);
        searchTimeout = null;
      }
      items = [];
      filteredItems = [];
      page = 1;
      hasMore = true;
      loadMoreItems();
    }
  }

  async function loadMoreItems() {
    if (loading || !hasMore) return;

    loading = true;
    try {
      const { items: newItems, hasMore: hasMoreResults } = await fetchItems(page);

      if (newItems.length === 0) {
        hasMore = false;
      } else {
        items = [...items, ...newItems];
        filteredItems = [...filteredItems, ...newItems];
        page++;
        hasMore = hasMoreResults;
      }
    } catch (error) {
      console.error('Failed to load items:', error);
    } finally {
      loading = false;
    }
  }

  function selectItem(item: LibraryItem) {
    selectedItem = item;
    showCard = true;
  }

  function closeCard() {
    showCard = false;
    selectedItem = null;
  }

  function handleScroll(event: Event) {
    const target = event.target as HTMLElement;
    const { scrollTop, scrollHeight, clientHeight } = target;

    if (scrollHeight - scrollTop <= clientHeight * 1.5) {
      loadMoreItems();
    }
  }

  function clearSearch() {
    searchQuery = '';
    searchKey = '';
    searchValue = '';
    items = [];
    filteredItems = [];
    page = 1;
    hasMore = true;
    loadMoreItems();
  }

  async function openItem(item: LibraryItem) {
    try {
      const response = await fetch(`${API_BASE_URL}/file/download/${item.Uuid}`);
      if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      window.open(url, '_blank');
    } catch (error) {
      alert(`Failed to open file: ${error.message}`);
    }
  }

  async function convertToPdf(item: LibraryItem) {
    if (item.FileType === '.pdf') {
      await openItem(item);
      return;
    }
    converting = true;
    try {
      const response = await fetch(`${API_BASE_URL}/file/convert/${item.Uuid}?format=pdf`);
      if (!response.ok) {
        const errData = await response.json().catch(() => ({}));
        throw new Error(errData.error || `Conversion failed (${response.status})`);
      }
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      window.open(url, '_blank');
    } catch (error) {
      alert(`PDF conversion failed: ${error.message}`);
    } finally {
      converting = false;
    }
  }

  function openEditModal(item: LibraryItem) {
    editingItem = item;
  }

  async function handleEditSave() {
    const savedItem = editingItem;
    editingItem = null;
    showCard = false;
    selectedItem = null;
    items = [];
    filteredItems = [];
    page = 1;
    hasMore = true;
    await loadMoreItems();
  }

  function handleEditCancel() {
    editingItem = null;
  }

  async function downloadItem(item: LibraryItem) {
    try {
      const response = await fetch(`${API_BASE_URL}/file/download/${item.Uuid}`);

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const blob = await response.blob();

      if (blob.size === 0) {
        throw new Error('Received empty file');
      }

      let filename = item.Title || item.Uuid;
      if (item.FileType && !filename.includes('.')) {
        filename += `.${item.FileType}`;
      }

      if ('showSaveFilePicker' in window) {
        try {
          const fileHandle = await (window as any).showSaveFilePicker({
            suggestedName: filename,
            types: [{
              description: 'File',
              accept: {
                [blob.type || 'application/octet-stream']: [`.${item.FileType}`]
              }
            }]
          });

          const writable = await fileHandle.createWritable();
          await writable.write(blob);
          await writable.close();
        } catch (saveError) {
          fallbackDownload(blob, filename);
        }
      } else {
        fallbackDownload(blob, filename);
      }

    } catch (error) {
      alert(`Download failed: ${error.message}`);
    }
  }

  function fallbackDownload(blob: Blob, filename: string) {
    const blobUrl = window.URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = blobUrl;
    link.download = filename;
    link.style.display = 'none';
    document.body.appendChild(link);
    link.click();
    setTimeout(() => {
      document.body.removeChild(link);
      window.URL.revokeObjectURL(blobUrl);
    }, 1000);
  }

  async function deleteItem(item: LibraryItem) {
    try {
      if (!confirm(`Are you sure you want to delete "${item.Title}"? This action cannot be undone.`)) {
        return;
      }

      const requestBody = { uuids: [item.Uuid] };

      const response = await fetch(`${API_BASE_URL}/data/delete`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody)
      });

      const result = await response.json();

      if (!response.ok) {
        if (result.deleted_count > 0) {
          const hasFileErrors = result.errors && result.errors.some(error =>
            error.includes('Failed to delete file') || error.includes('file')
          );

          if (hasFileErrors) {
            alert(`Warning: "${item.Title}" was removed from the library, but the physical file could not be deleted.`);
          } else {
            alert(`Warning: "${item.Title}" was deleted with some issues.`);
          }
        } else {
          const errorMessage = result.errors ? result.errors.join(', ') : result.error || response.statusText;
          throw new Error(`Delete failed: ${errorMessage}`);
        }
      } else {
        alert(`Successfully deleted "${item.Title}"`);
      }

      items = items.filter(libItem => libItem.Uuid !== item.Uuid);
      filteredItems = filteredItems.filter(libItem => libItem.Uuid !== item.Uuid);

      if (selectedItem && selectedItem.Uuid === item.Uuid) {
        selectedItem = null;
      }

    } catch (error) {
      alert(`Delete failed: ${error.message}`);
    }
  }

  onMount(() => {
    loadMoreItems();
    isInitialLoad = false;
  });
</script>

<div class="library-container">
  <!-- Fixed Search Bar -->
  <div class="search-container">
    <div class="search-bar" class:focused={searchFocused}>
      <svg class="search-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="8"></circle>
        <path d="m21 21-4.35-4.35"></path>
      </svg>
      <input
        type="text"
        placeholder="Search... (or use author:, type:, dewey: prefixes)"
        bind:value={searchQuery}
        on:focus={() => searchFocused = true}
        on:blur={() => searchFocused = false}
        class="search-input"
      />
      {#if searchQuery}
        <button class="clear-button" on:click={clearSearch}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="18" y1="6" x2="6" y2="18"></line>
            <line x1="6" y1="6" x2="18" y2="18"></line>
          </svg>
        </button>
      {/if}
    </div>

    {#if searchQuery}
      <div class="search-results-info">
        <span class="results-count">
          {filteredItems.length} {filteredItems.length === 1 ? 'item' : 'items'} found
        </span>
        {#if filteredItems.length === 0}
          <span class="no-results">No items match your search</span>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Gallery with adjusted top margin for search bar -->
  <div class="gallery" on:scroll={handleScroll}>
    <div class="grid">
      {#each filteredItems as item (item.Uuid)}
        <ItemCard {item} onSelect={selectItem} />
      {/each}
    </div>

    {#if loading}
      <div class="loading">
        <div class="spinner"></div>
        <p>Loading more items...</p>
      </div>
    {/if}

    {#if searchQuery && filteredItems.length === 0 && !loading}
      <div class="empty-state">
        <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
          <circle cx="11" cy="11" r="8"></circle>
          <path d="m21 21-4.35-4.35"></path>
        </svg>
        <h3>No results found</h3>
        <p>Try adjusting your search terms or browse all items</p>
        <button class="clear-search-button" on:click={clearSearch}>
          Clear Search
        </button>
      </div>
    {/if}
  </div>

  {#if showCard && selectedItem}
    <div class="overlay" on:click={closeCard}>
      <div class="card" on:click|stopPropagation>
        <button class="close-button" on:click={closeCard}>
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="18" y1="6" x2="6" y2="18"></line>
            <line x1="6" y1="6" x2="18" y2="18"></line>
          </svg>
        </button>

        <div class="card-content">
          <div class="card-header">
              <div class="card-placeholder">
                <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
                  <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                  <polyline points="14,2 14,8 20,8"></polyline>
                </svg>
              </div>
            <div class="card-title-section">
              <h2>{selectedItem.Title}</h2>
              <span class="file-type">{selectedItem.FileType}</span>
            </div>
          </div>

          <div class="card-details">
            <div class="detail-row">
              <span class="label">Author:</span>
              <span>{selectedItem.Author}</span>
            </div>
            <div class="detail-row">
              <span class="label">Document Type:</span>
              <span>{selectedItem.DocType}</span>
            </div>
            <div class="detail-row">
              <span class="label">File Type:</span>
              <span>{selectedItem.FileType}</span>
            </div>
            <div class="detail-row">
              <span class="label">Publish Date:</span>
              <span>{new Date(selectedItem.PublishDate).toLocaleDateString()}</span>
            </div>
            <div class="detail-row">
              <span class="label">Last Updated:</span>
              <span>{new Date(selectedItem.LastUpdated).toLocaleDateString()}</span>
            </div>
            {#if selectedItem.DeweyDecimal}
              <div class="detail-row">
                <span class="label">Dewey:</span>
                <span>{selectedItem.DeweyDecimal}</span>
              </div>
            {/if}
            <div class="detail-row">
              <span class="label">Path:</span>
              <span class="path">{selectedItem.Path}</span>
            </div>
            <div class="detail-row">
              <span class="label">UUID:</span>
              <span class="id">{selectedItem.Uuid}</span>
            </div>
          </div>

          <div class="card-actions">
            <button class="action-button primary" on:click={() => openItem(selectedItem)}>Open</button>
            <button class="action-button accent" on:click={() => convertToPdf(selectedItem)} disabled={converting}>
              {converting ? 'Converting...' : 'View as PDF'}
            </button>
            <button class="action-button" on:click={() => openEditModal(selectedItem)}>Edit</button>
            <button class="action-button" on:click={() => downloadItem(selectedItem)}>Download</button>
            <button class="action-button danger" on:click={() => deleteItem(selectedItem)}>Delete</button>
          </div>
        </div>
      </div>
    </div>
  {/if}

  {#if editingItem}
    <EditModal
      item={editingItem}
      onSave={handleEditSave}
      onCancel={handleEditCancel}
    />
  {/if}
</div>

<style>
  .library-container {
    height: 100%;
    position: relative;
    display: flex;
    flex-direction: column;
  }

  .search-container {
    position: sticky;
    top: 0;
    z-index: 100;
    background: rgba(28, 28, 30, 0.95);
    backdrop-filter: blur(20px);
    -webkit-backdrop-filter: blur(20px);
    padding: 16px 0;
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  }

  .search-bar {
    display: flex;
    align-items: center;
    background: rgba(255, 255, 255, 0.1);
    border: 1px solid rgba(255, 255, 255, 0.2);
    border-radius: 12px;
    padding: 12px 16px;
    transition: all 0.2s ease;
    margin-bottom: 8px;
  }

  .search-bar.focused {
    border-color: #007AFF;
    background: rgba(255, 255, 255, 0.15);
    box-shadow: 0 0 0 3px rgba(0, 122, 255, 0.1);
  }

  .search-icon {
    color: rgba(255, 255, 255, 0.6);
    margin-right: 12px;
    flex-shrink: 0;
  }

  .search-input {
    flex: 1;
    background: none;
    border: none;
    color: #ffffff;
    font-size: 16px;
    outline: none;
  }

  .search-input::placeholder {
    color: rgba(255, 255, 255, 0.5);
  }

  .clear-button {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
    transition: all 0.2s ease;
    margin-left: 8px;
  }

  .clear-button:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .search-results-info {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 14px;
  }

  .results-count {
    color: rgba(255, 255, 255, 0.7);
  }

  .no-results {
    color: rgba(255, 255, 255, 0.5);
    font-style: italic;
  }

  .gallery {
    flex: 1;
    overflow-y: auto;
    padding: 0 8px;
  }

  .gallery::-webkit-scrollbar {
    width: 8px;
  }

  .gallery::-webkit-scrollbar-track {
    background: rgba(255, 255, 255, 0.1);
    border-radius: 4px;
  }

  .gallery::-webkit-scrollbar-thumb {
    background: rgba(255, 255, 255, 0.3);
    border-radius: 4px;
  }

  .gallery::-webkit-scrollbar-thumb:hover {
    background: rgba(255, 255, 255, 0.5);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 16px;
    padding: 16px 0;
  }

  .loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 32px;
    color: rgba(255, 255, 255, 0.6);
  }

  .spinner {
    width: 32px;
    height: 32px;
    border: 3px solid rgba(255, 255, 255, 0.1);
    border-top: 3px solid #007AFF;
    border-radius: 50%;
    animation: spin 1s linear infinite;
    margin-bottom: 16px;
  }

  @keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 64px 32px;
    text-align: center;
    color: rgba(255, 255, 255, 0.6);
  }

  .empty-state svg {
    margin-bottom: 16px;
    color: rgba(255, 255, 255, 0.4);
  }

  .empty-state h3 {
    font-size: 18px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 8px 0;
  }

  .empty-state p {
    margin: 0 0 24px 0;
    line-height: 1.5;
  }

  .clear-search-button {
    background: #007AFF;
    color: #ffffff;
    border: none;
    padding: 8px 16px;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: background-color 0.2s ease;
  }

  .clear-search-button:hover {
    background: #0056CC;
  }

  .overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(10px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 2000;
    padding: 24px;
  }

  .card {
    background: rgba(44, 44, 46, 0.95);
    backdrop-filter: blur(20px);
    border-radius: 16px;
    max-width: 500px;
    width: 100%;
    max-height: 80vh;
    overflow-y: auto;
    position: relative;
    border: 1px solid rgba(255, 255, 255, 0.1);
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  }

  .close-button {
    position: absolute;
    top: 16px;
    right: 16px;
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    padding: 8px;
    border-radius: 6px;
    transition: all 0.2s ease;
    z-index: 1;
  }

  .close-button:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .card-content {
    padding: 24px;
  }

  .card-header {
    display: flex;
    align-items: flex-start;
    margin-bottom: 24px;
  }

  .card-placeholder {
    width: 80px;
    height: 80px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.05);
    display: flex;
    align-items: center;
    justify-content: center;
    margin-right: 16px;
    color: rgba(255, 255, 255, 0.5);
  }

  .card-title-section {
    flex: 1;
  }

  .card-title-section h2 {
    font-size: 20px;
    font-weight: 700;
    color: #ffffff;
    margin: 0 0 8px 0;
  }

  .file-type {
    display: inline-block;
    background: rgba(0, 122, 255, 0.2);
    color: #007AFF;
    padding: 4px 8px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 600;
  }

  .card-details {
    margin-bottom: 24px;
  }

  .detail-row {
    display: flex;
    margin-bottom: 12px;
    align-items: flex-start;
  }

  .detail-row:last-child {
    margin-bottom: 0;
  }

  .label {
    font-weight: 600;
    color: rgba(255, 255, 255, 0.8);
    min-width: 100px;
    margin-right: 12px;
  }

  .detail-row p {
    margin: 0;
    color: rgba(255, 255, 255, 0.9);
    line-height: 1.5;
  }

  .detail-row span {
    color: rgba(255, 255, 255, 0.9);
  }

  .id {
    font-family: 'Monaco', 'Menlo', monospace;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.6);
  }

  .path {
    font-family: 'Monaco', 'Menlo', monospace;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.6);
    word-break: break-all;
  }

  .card-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .action-button {
    flex: 1;
    padding: 12px 16px;
    border: none;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
  }

  .action-button.primary {
    background: #007AFF;
    color: #ffffff;
  }

  .action-button.primary:hover {
    background: #0056CC;
  }

  .action-button.accent {
    background: rgba(88, 86, 214, 0.2);
    color: #5856D6;
  }

  .action-button.accent:hover:not(:disabled) {
    background: rgba(88, 86, 214, 0.35);
  }

  .action-button.danger {
    background: rgba(255, 59, 48, 0.15);
    color: #FF3B30;
  }

  .action-button.danger:hover {
    background: rgba(255, 59, 48, 0.3);
  }

  .action-button:not(.primary):not(.accent):not(.danger) {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .action-button:not(.primary):not(.accent):not(.danger):hover {
    background: rgba(255, 255, 255, 0.2);
  }

  .action-button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  @media (max-width: 768px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
      gap: 12px;
    }

    .card {
      margin: 16px;
      max-height: calc(100vh - 32px);
    }

    .search-bar {
      padding: 10px 14px;
    }

    .search-input {
      font-size: 14px;
    }
  }
</style>
