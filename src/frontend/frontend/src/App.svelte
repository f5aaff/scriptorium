<script lang="ts">
  import { onMount } from 'svelte';
  import Library from './components/Library.svelte';
  import Add from './components/Add.svelte';
  import Settings from './components/Settings.svelte';
  import Sidebar from './components/Sidebar.svelte';

  let currentPage = 'library';
  let sidebarOpen = false;

  function toggleSidebar() {
    sidebarOpen = !sidebarOpen;
  }

  function navigateTo(page: string) {
    currentPage = page;
    sidebarOpen = false;
  }
</script>

<svelte:head>
  <title>Scriptorium</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
</svelte:head>

<div class="app" class:sidebar-open={sidebarOpen}>
  <Sidebar 
    {sidebarOpen} 
    {currentPage} 
    on:navigate={({ detail }) => navigateTo(detail)}
    on:toggle={toggleSidebar}
  />
  
  <main class="main-content">
    <header class="header">
      <button class="menu-button" on:click={toggleSidebar}>
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="3" y1="6" x2="21" y2="6"></line>
          <line x1="3" y1="12" x2="21" y2="12"></line>
          <line x1="3" y1="18" x2="21" y2="18"></line>
        </svg>
      </button>
      <h1 class="title">{currentPage.charAt(0).toUpperCase() + currentPage.slice(1)}</h1>
    </header>
    
    <div class="page-content">
      {#if currentPage === 'library'}
        <Library />
      {:else if currentPage === 'add'}
        <Add />
      {:else if currentPage === 'settings'}
        <Settings />
      {/if}
    </div>
  </main>
</div>

<style>
  :global(*) {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: transparent;
    color: #ffffff;
    overflow: hidden;
  }

  .app {
    display: flex;
    height: 100vh;
    background: rgba(28, 28, 30, 0.95);
    backdrop-filter: blur(20px);
    -webkit-backdrop-filter: blur(20px);
    border-radius: 12px;
    overflow: hidden;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
  }

  .main-content {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .header {
    display: flex;
    align-items: center;
    padding: 16px 24px;
    background: rgba(44, 44, 46, 0.8);
    backdrop-filter: blur(10px);
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
    -webkit-app-region: drag;
  }

  .menu-button {
    background: none;
    border: none;
    color: #ffffff;
    cursor: pointer;
    padding: 8px;
    border-radius: 6px;
    margin-right: 16px;
    transition: background-color 0.2s ease;
    -webkit-app-region: no-drag;
  }

  .menu-button:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .title {
    font-size: 18px;
    font-weight: 600;
    color: #ffffff;
    -webkit-app-region: no-drag;
  }

  .page-content {
    flex: 1;
    overflow: hidden;
    padding: 24px;
  }

  .sidebar-open .main-content {
    margin-left: 0;
  }

  @media (max-width: 768px) {
    .app {
      border-radius: 0;
    }
  }
</style>
