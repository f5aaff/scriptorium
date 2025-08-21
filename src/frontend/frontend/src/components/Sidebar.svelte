<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  
  export let sidebarOpen: boolean = false;
  export let currentPage: string = 'library';
  
  const dispatch = createEventDispatcher();
  
  function navigate(page: string) {
    dispatch('navigate', page);
  }
  
  function toggle() {
    dispatch('toggle');
  }
</script>

<div class="sidebar" class:open={sidebarOpen}>
  <div class="sidebar-header">
    <h2 class="app-title">Scriptorium</h2>
    <button class="close-button" on:click={toggle}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <line x1="18" y1="6" x2="6" y2="18"></line>
        <line x1="6" y1="6" x2="18" y2="18"></line>
      </svg>
    </button>
  </div>
  
  <nav class="sidebar-nav">
    <button 
      class="nav-item" 
      class:active={currentPage === 'library'}
      on:click={() => navigate('library')}
    >
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
        <line x1="9" y1="3" x2="9" y2="21"></line>
      </svg>
      <span>Library</span>
    </button>
    
    <button 
      class="nav-item" 
      class:active={currentPage === 'add'}
      on:click={() => navigate('add')}
    >
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
        <polyline points="14,2 14,8 20,8"></polyline>
        <line x1="12" y1="18" x2="12" y2="12"></line>
        <line x1="9" y1="15" x2="15" y2="15"></line>
      </svg>
      <span>Add</span>
    </button>
    
    <button 
      class="nav-item" 
      class:active={currentPage === 'settings'}
      on:click={() => navigate('settings')}
    >
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="3"></circle>
        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1 1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
      </svg>
      <span>Settings</span>
    </button>
  </nav>
</div>

<style>
  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    width: 280px;
    height: 100vh;
    background: rgba(44, 44, 46, 0.95);
    backdrop-filter: blur(20px);
    -webkit-backdrop-filter: blur(20px);
    border-right: 1px solid rgba(255, 255, 255, 0.1);
    transform: translateX(-100%);
    transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    z-index: 1000;
    overflow-y: auto;
  }
  
  .sidebar.open {
    transform: translateX(0);
  }
  
  .sidebar-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 24px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  }
  
  .app-title {
    font-size: 20px;
    font-weight: 700;
    color: #ffffff;
    margin: 0;
  }
  
  .close-button {
    background: none;
    border: none;
    color: #ffffff;
    cursor: pointer;
    padding: 8px;
    border-radius: 6px;
    transition: background-color 0.2s ease;
  }
  
  .close-button:hover {
    background: rgba(255, 255, 255, 0.1);
  }
  
  .sidebar-nav {
    padding: 16px 0;
  }
  
  .nav-item {
    display: flex;
    align-items: center;
    width: 100%;
    padding: 12px 24px;
    background: none;
    border: none;
    color: #ffffff;
    cursor: pointer;
    transition: background-color 0.2s ease;
    text-align: left;
    font-size: 16px;
    font-weight: 500;
  }
  
  .nav-item:hover {
    background: rgba(255, 255, 255, 0.1);
  }
  
  .nav-item.active {
    background: rgba(0, 122, 255, 0.2);
    color: #007AFF;
  }
  
  .nav-item svg {
    margin-right: 12px;
    flex-shrink: 0;
  }
  
  .nav-item span {
    flex: 1;
  }
  
  @media (max-width: 768px) {
    .sidebar {
      width: 100%;
    }
  }
</style> 