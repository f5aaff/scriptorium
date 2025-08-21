<script lang="ts">
  import { onMount } from 'svelte';
  import ItemCard from './ItemCard.svelte';

  interface LibraryItem {
    Title: string;
    Author: string;
    PublishDate: string;
    LastUpdated: string;
    FileType: string;
    DocType: string;
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

  // API function to fetch items from the search endpoint
  async function fetchItems(pageNum: number, searchKeyParam?: string, searchValueParam?: string): Promise<{ items: LibraryItem[], hasMore: boolean }> {
    const params = new URLSearchParams();
    params.append('page', pageNum.toString());
    params.append('limit', '20'); // Default limit

    if (searchKeyParam && searchValueParam) {
      params.append('key', searchKeyParam);
      params.append('value', searchValueParam);
    }

    const url = `http://localhost:8080/data/search?${params.toString()}`;
    console.log('Fetching items from URL:', url);

    try {
      const response = await fetch(url);

      if (!response.ok) {
        console.error('API response not ok:', response.status, response.statusText);
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data: SearchResponse = await response.json();
      console.log('API Response:', data);
      console.log('Results count:', data.results?.length || 0);

      return {
        items: data.results || [],
        hasMore: data.has_next || false
      };
    } catch (error) {
      console.error('Error fetching items:', error);
      throw error;
    }
  }

  // Search function that uses the API
  async function performSearch() {
    if (!searchQuery.trim()) {
      // If no search query, load all items
      await loadMoreItems();
      return;
    }

    loading = true;
    try {
      // Try to determine search key based on query content
      let key = 'Title'; // Default to title search
      let value = searchQuery;

      // Simple heuristic to determine search type
      if (searchQuery.toLowerCase().includes('author:')) {
        key = 'Author';
        value = searchQuery.replace(/author:\s*/i, '');
      } else if (searchQuery.toLowerCase().includes('type:')) {
        key = 'DocType';
        value = searchQuery.replace(/type:\s*/i, '');
      } else if (searchQuery.toLowerCase().includes('filetype:')) {
        key = 'FileType';
        value = searchQuery.replace(/filetype:\s*/i, '');
      }

      const { items: searchResults, hasMore: hasMoreResults } = await fetchItems(1, key, value);

      items = searchResults;
      filteredItems = searchResults;
      hasMore = hasMoreResults;
      page = 1;
    } catch (error) {
      console.error('Error performing search:', error);
    } finally {
      loading = false;
    }
  }

  // Update filtered items when search query changes
  $: {
    console.log('Reactive statement triggered, searchQuery:', searchQuery, 'isInitialLoad:', isInitialLoad);
    if (searchQuery.trim()) {
      // Clear existing timeout
      if (searchTimeout) {
        clearTimeout(searchTimeout);
      }

      // Set new timeout for debounced search
      searchTimeout = setTimeout(() => {
        performSearch();
      }, 300);
    } else if (!isInitialLoad) {
      // Clear search and load all items (but not on initial load)
      if (searchTimeout) {
        clearTimeout(searchTimeout);
        searchTimeout = null;
      }
      console.log('Clearing items and loading all items');
      items = [];
      filteredItems = [];
      page = 1;
      hasMore = true;
      loadMoreItems();
    }
  }

  async function loadMoreItems() {
    if (loading || !hasMore) return;

    console.log('Loading more items, page:', page, 'hasMore:', hasMore);
    loading = true;
    try {
      const { items: newItems, hasMore: hasMoreResults } = await fetchItems(page);

      console.log('Received new items:', newItems.length);
      console.log('New items:', newItems);

      if (newItems.length === 0) {
        hasMore = false;
        console.log('No more items, setting hasMore to false');
      } else {
        items = [...items, ...newItems];
        filteredItems = [...filteredItems, ...newItems];
        page++;
        hasMore = hasMoreResults;
        console.log('Updated items array length:', items.length);
        console.log('Updated filteredItems array length:', filteredItems.length);
      }
    } catch (error) {
      console.error('Error loading items:', error);
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
    // Reset to load all items
    items = [];
    filteredItems = [];
    page = 1;
    hasMore = true;
    loadMoreItems();
  }

  onMount(() => {
    console.log('Library component mounted. Loading initial items.');
    loadMoreItems();
    isInitialLoad = false; // Set to false after initial load
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
        placeholder="Search by title, author, or type... (e.g., author:John, type:Notes)"
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
        <div class="grid-item" on:click={() => selectItem(item)}>
          <div class="item-thumbnail">
              <div class="placeholder">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
                  <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                  <polyline points="14,2 14,8 20,8"></polyline>
                </svg>
              </div>
          </div>
          <div class="item-info">
            <h3 class="item-title">{item.Title}</h3>
            <p class="item-type">{item.FileType}</p>
          </div>
        </div>
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
            <button class="action-button primary">Open</button>
            <button class="action-button">Download</button>
            <button class="action-button">Share</button>
          </div>
        </div>
      </div>
    </div>
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

  .grid-item {
    background: rgba(44, 44, 46, 0.8);
    border-radius: 12px;
    padding: 16px;
    cursor: pointer;
    transition: all 0.2s ease;
    border: 1px solid rgba(255, 255, 255, 0.1);
  }

  .grid-item:hover {
    background: rgba(44, 44, 46, 0.95);
    transform: translateY(-2px);
    box-shadow: 0 8px 25px rgba(0, 0, 0, 0.2);
  }

  .item-thumbnail {
    width: 100%;
    height: 120px;
    border-radius: 8px;
    overflow: hidden;
    margin-bottom: 12px;
    background: rgba(255, 255, 255, 0.05);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .item-thumbnail img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .placeholder {
    color: rgba(255, 255, 255, 0.5);
  }

  .item-info {
    text-align: center;
  }

  .item-title {
    font-size: 14px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 4px 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .item-type {
    font-size: 12px;
    color: rgba(255, 255, 255, 0.6);
    margin: 0;
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

  .card-thumbnail {
    width: 80px;
    height: 80px;
    border-radius: 8px;
    object-fit: cover;
    margin-right: 16px;
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
    gap: 12px;
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

  .action-button:not(.primary) {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .action-button:not(.primary):hover {
    background: rgba(255, 255, 255, 0.2);
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
