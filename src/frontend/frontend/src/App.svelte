<script lang="ts">
  import { onMount } from 'svelte';
  import Library from './components/Library.svelte';
  import Add from './components/Add.svelte';
  import Settings from './components/Settings.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import { API_BASE_URL } from './config';
  import { apiFetch, checkAuthentication, setCSRFToken, setSessionToken } from './api';

  let currentPage = 'library';
  let sidebarOpen = false;
  let authState: 'checking' | 'signed-out' | 'signed-in' = 'checking';
  let username = '';
  let password = '';
  let loginError = '';
  let signingIn = false;

  onMount(() => {
    const handleUnauthorized = () => {
      setCSRFToken('');
      setSessionToken('');
      authState = 'signed-out';
    };
    window.addEventListener('scriptorium:unauthorized', handleUnauthorized);
    checkAuthentication().then((authenticated) => {
      authState = authenticated ? 'signed-in' : 'signed-out';
    });
    return () => window.removeEventListener('scriptorium:unauthorized', handleUnauthorized);
  });

  async function signIn(event: Event) {
    event.preventDefault();
    loginError = '';
    signingIn = true;
    try {
      const response = await fetch(`${API_BASE_URL}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ username, password })
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || 'Sign in failed');
      setCSRFToken(result.csrf_token || '');
      setSessionToken(result.session_token || '');
      password = '';
      authState = 'signed-in';
    } catch (error) {
      loginError = error instanceof Error ? error.message : 'Sign in failed';
    } finally {
      signingIn = false;
    }
  }

  async function signOut() {
    await apiFetch(`${API_BASE_URL}/auth/logout`, { method: 'POST' });
    setCSRFToken('');
    setSessionToken('');
    authState = 'signed-out';
    currentPage = 'library';
  }

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

{#if authState === 'checking'}
  <div class="auth-screen"><p>Connecting to Scriptorium…</p></div>
{:else if authState === 'signed-out'}
  <div class="auth-screen">
    <form class="login-card" on:submit={signIn}>
      <div class="brand-mark">S</div>
      <h1>Scriptorium</h1>
      <p>Sign in to your library</p>
      <label for="username">Username</label>
      <input id="username" bind:value={username} autocomplete="username" required />
      <label for="password">Password</label>
      <input id="password" type="password" bind:value={password} autocomplete="current-password" required />
      {#if loginError}<p class="login-error" role="alert">{loginError}</p>{/if}
      <button type="submit" disabled={signingIn}>{signingIn ? 'Signing in…' : 'Sign in'}</button>
    </form>
  </div>
{:else}
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
      <button class="sign-out" on:click={signOut}>Sign out</button>
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
{/if}

<style>
  :global(*) { margin: 0; padding: 0; box-sizing: border-box; }
  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: transparent;
    color: #fff;
    overflow: hidden;
  }
  .auth-screen {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: 24px;
    background: #1c1c1e;
    color: #fff;
  }
  .login-card {
    width: min(100%, 380px);
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 32px;
    border: 1px solid rgba(255,255,255,.12);
    border-radius: 16px;
    background: #2c2c2e;
  }
  .login-card h1 { font-size: 24px; }
  .login-card p { color: #aaa; margin-bottom: 8px; }
  .login-card label { font-size: 13px; color: #ddd; }
  .login-card input {
    padding: 12px;
    color: #fff;
    background: #1c1c1e;
    border: 1px solid #555;
    border-radius: 8px;
  }
  .login-card button, .sign-out {
    border: 0;
    border-radius: 8px;
    padding: 11px 16px;
    color: #fff;
    background: #5865d8;
    cursor: pointer;
  }
  .login-card button:disabled { opacity: .6; cursor: wait; }
  .brand-mark {
    width: 42px;
    height: 42px;
    display: grid;
    place-items: center;
    border-radius: 12px;
    background: #5865d8;
    font-size: 22px;
    font-weight: 700;
  }
  .login-card .login-error { color: #ff8c8c; margin: 0; }
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
  .main-content { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
  .header {
    display: flex;
    align-items: center;
    padding: 16px 24px;
    background: rgba(44, 44, 46, 0.8);
    backdrop-filter: blur(10px);
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
    -webkit-app-region: drag;
  }
  .sign-out { margin-left: auto; -webkit-app-region: no-drag; }
  .menu-button {
    background: none;
    border: none;
    color: #fff;
    cursor: pointer;
    padding: 8px;
    border-radius: 6px;
    margin-right: 16px;
    transition: background-color 0.2s ease;
    -webkit-app-region: no-drag;
  }
  .menu-button:hover { background: rgba(255, 255, 255, 0.1); }
  .title { font-size: 18px; font-weight: 600; color: #fff; -webkit-app-region: no-drag; }
  .page-content { flex: 1; overflow: hidden; padding: 24px; }
  .sidebar-open .main-content { margin-left: 0; }
  @media (max-width: 768px) { .app { border-radius: 0; } }
</style>
