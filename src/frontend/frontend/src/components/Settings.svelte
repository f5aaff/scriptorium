<script lang="ts">
  let darkMode = true;
  let autoSave = true;
  let notifications = true;
  let syncEnabled = false;
  let language = 'en';
  let theme = 'dark';
  let fontSize = 'medium';
  let selectedFolder = '';
  
  const languages = [
    { code: 'en', name: 'English' },
    { code: 'es', name: 'Español' },
    { code: 'fr', name: 'Français' },
    { code: 'de', name: 'Deutsch' },
    { code: 'ja', name: '日本語' }
  ];
  
  const themes = [
    { value: 'dark', name: 'Dark' },
    { value: 'light', name: 'Light' },
    { value: 'auto', name: 'Auto' }
  ];
  
  const fontSizes = [
    { value: 'small', name: 'Small' },
    { value: 'medium', name: 'Medium' },
    { value: 'large', name: 'Large' }
  ];
  
  function selectFolder() {
    // In a real app, this would open a folder picker
    // For now, we'll simulate it
    selectedFolder = '/Users/username/Documents/Scriptorium';
  }
  
  function exportSettings() {
    const settings = {
      darkMode,
      autoSave,
      notifications,
      syncEnabled,
      language,
      theme,
      fontSize,
      selectedFolder
    };
    
    const blob = new Blob([JSON.stringify(settings, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'scriptorium-settings.json';
    a.click();
    URL.revokeObjectURL(url);
  }
  
  function importSettings(event: Event) {
    const target = event.target as HTMLInputElement;
    if (target.files && target.files[0]) {
      const file = target.files[0];
      const reader = new FileReader();
      reader.onload = (e) => {
        try {
          const settings = JSON.parse(e.target?.result as string);
          // Apply settings
          darkMode = settings.darkMode ?? true;
          autoSave = settings.autoSave ?? true;
          notifications = settings.notifications ?? true;
          syncEnabled = settings.syncEnabled ?? false;
          language = settings.language ?? 'en';
          theme = settings.theme ?? 'dark';
          fontSize = settings.fontSize ?? 'medium';
          selectedFolder = settings.selectedFolder ?? '';
        } catch (error) {
          console.error('Error importing settings:', error);
        }
      };
      reader.readAsText(file);
    }
  }
</script>

<div class="settings-container">
  <div class="settings-grid">
    <!-- Appearance Section -->
    <div class="settings-section">
      <h3 class="section-title">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="3"></circle>
          <path d="M12 1v2"></path>
          <path d="M12 21v2"></path>
          <path d="M4.22 4.22l1.42 1.42"></path>
          <path d="M18.36 18.36l1.42 1.42"></path>
          <path d="M1 12h2"></path>
          <path d="M21 12h2"></path>
          <path d="M4.22 19.78l1.42-1.42"></path>
          <path d="M18.36 5.64l1.42-1.42"></path>
        </svg>
        Appearance
      </h3>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Dark Mode</label>
          <p class="setting-description">Use dark theme for the application</p>
        </div>
        <label class="toggle">
          <input type="checkbox" bind:checked={darkMode}>
          <span class="slider"></span>
        </label>
      </div>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Theme</label>
          <p class="setting-description">Choose your preferred theme</p>
        </div>
        <select class="setting-select" bind:value={theme}>
          {#each themes as themeOption}
            <option value={themeOption.value}>{themeOption.name}</option>
          {/each}
        </select>
      </div>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Font Size</label>
          <p class="setting-description">Adjust the text size</p>
        </div>
        <select class="setting-select" bind:value={fontSize}>
          {#each fontSizes as sizeOption}
            <option value={sizeOption.value}>{sizeOption.name}</option>
          {/each}
        </select>
      </div>
    </div>
    
    <!-- General Section -->
    <div class="settings-section">
      <h3 class="section-title">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
          <circle cx="9" cy="9" r="2"></circle>
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
          <polyline points="7,10 12,15 17,10"></polyline>
          <line x1="12" y1="15" x2="12" y2="3"></line>
        </svg>
        General
      </h3>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Auto Save</label>
          <p class="setting-description">Automatically save changes</p>
        </div>
        <label class="toggle">
          <input type="checkbox" bind:checked={autoSave}>
          <span class="slider"></span>
        </label>
      </div>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Notifications</label>
          <p class="setting-description">Show system notifications</p>
        </div>
        <label class="toggle">
          <input type="checkbox" bind:checked={notifications}>
          <span class="slider"></span>
        </label>
      </div>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Language</label>
          <p class="setting-description">Choose your preferred language</p>
        </div>
        <select class="setting-select" bind:value={language}>
          {#each languages as lang}
            <option value={lang.code}>{lang.name}</option>
          {/each}
        </select>
      </div>
    </div>
    
    <!-- Sync Section -->
    <div class="settings-section">
      <h3 class="section-title">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 2v4"></path>
          <path d="M12 18v4"></path>
          <path d="M4.93 4.93l2.83 2.83"></path>
          <path d="M16.24 16.24l2.83 2.83"></path>
          <path d="M2 12h4"></path>
          <path d="M18 12h4"></path>
          <path d="M4.93 19.07l2.83-2.83"></path>
          <path d="M16.24 7.76l2.83-2.83"></path>
        </svg>
        Sync & Storage
      </h3>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Cloud Sync</label>
          <p class="setting-description">Sync your library across devices</p>
        </div>
        <label class="toggle">
          <input type="checkbox" bind:checked={syncEnabled}>
          <span class="slider"></span>
        </label>
      </div>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Default Folder</label>
          <p class="setting-description">Choose where files are saved</p>
        </div>
        <button class="folder-button" on:click={selectFolder}>
          {selectedFolder || 'Select Folder'}
        </button>
      </div>
    </div>
    
    <!-- Import/Export Section -->
    <div class="settings-section">
      <h3 class="section-title">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
          <polyline points="7,10 12,15 17,10"></polyline>
          <line x1="12" y1="15" x2="12" y2="3"></line>
        </svg>
        Import & Export
      </h3>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Export Settings</label>
          <p class="setting-description">Save your current settings</p>
        </div>
        <button class="action-button" on:click={exportSettings}>
          Export
        </button>
      </div>
      
      <div class="setting-item">
        <div class="setting-info">
          <label class="setting-label">Import Settings</label>
          <p class="setting-description">Load settings from a file</p>
        </div>
        <label class="action-button">
          Import
          <input type="file" accept=".json" on:change={importSettings} style="display: none;">
        </label>
      </div>
    </div>
  </div>
</div>

<style>
  .settings-container {
    height: 100%;
    overflow-y: auto;
    padding: 0 8px;
  }
  
  .settings-container::-webkit-scrollbar {
    width: 8px;
  }
  
  .settings-container::-webkit-scrollbar-track {
    background: rgba(255, 255, 255, 0.1);
    border-radius: 4px;
  }
  
  .settings-container::-webkit-scrollbar-thumb {
    background: rgba(255, 255, 255, 0.3);
    border-radius: 4px;
  }
  
  .settings-container::-webkit-scrollbar-thumb:hover {
    background: rgba(255, 255, 255, 0.5);
  }
  
  .settings-grid {
    display: grid;
    gap: 24px;
    padding: 16px 0;
  }
  
  .settings-section {
    background: rgba(44, 44, 46, 0.8);
    border-radius: 16px;
    padding: 24px;
    border: 1px solid rgba(255, 255, 255, 0.1);
  }
  
  .section-title {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 18px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 20px 0;
  }
  
  .section-title svg {
    color: #007AFF;
  }
  
  .setting-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 0;
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  }
  
  .setting-item:last-child {
    border-bottom: none;
  }
  
  .setting-info {
    flex: 1;
    margin-right: 16px;
  }
  
  .setting-label {
    display: block;
    font-size: 16px;
    font-weight: 600;
    color: #ffffff;
    margin-bottom: 4px;
  }
  
  .setting-description {
    font-size: 14px;
    color: rgba(255, 255, 255, 0.6);
    margin: 0;
    line-height: 1.4;
  }
  
  /* Toggle Switch */
  .toggle {
    position: relative;
    display: inline-block;
    width: 50px;
    height: 24px;
  }
  
  .toggle input {
    opacity: 0;
    width: 0;
    height: 0;
  }
  
  .slider {
    position: absolute;
    cursor: pointer;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-color: rgba(255, 255, 255, 0.2);
    transition: 0.3s;
    border-radius: 24px;
  }
  
  .slider:before {
    position: absolute;
    content: "";
    height: 18px;
    width: 18px;
    left: 3px;
    bottom: 3px;
    background-color: white;
    transition: 0.3s;
    border-radius: 50%;
  }
  
  input:checked + .slider {
    background-color: #007AFF;
  }
  
  input:checked + .slider:before {
    transform: translateX(26px);
  }
  
  /* Select Dropdown */
  .setting-select {
    background: rgba(255, 255, 255, 0.1);
    border: 1px solid rgba(255, 255, 255, 0.2);
    border-radius: 8px;
    color: #ffffff;
    padding: 8px 12px;
    font-size: 14px;
    cursor: pointer;
    min-width: 120px;
  }
  
  .setting-select:focus {
    outline: none;
    border-color: #007AFF;
  }
  
  .setting-select option {
    background: rgba(44, 44, 46, 0.95);
    color: #ffffff;
  }
  
  /* Buttons */
  .action-button {
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
  
  .action-button:hover {
    background: #0056CC;
  }
  
  .folder-button {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
    border: 1px solid rgba(255, 255, 255, 0.2);
    padding: 8px 16px;
    border-radius: 8px;
    font-size: 14px;
    cursor: pointer;
    transition: all 0.2s ease;
    min-width: 120px;
  }
  
  .folder-button:hover {
    background: rgba(255, 255, 255, 0.2);
    border-color: rgba(255, 255, 255, 0.3);
  }
  
  @media (max-width: 768px) {
    .settings-grid {
      gap: 16px;
    }
    
    .settings-section {
      padding: 20px;
    }
    
    .setting-item {
      flex-direction: column;
      align-items: flex-start;
      gap: 12px;
    }
    
    .setting-info {
      margin-right: 0;
    }
  }
</style> 