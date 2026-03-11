<script lang="ts">
  import { onMount } from 'svelte';
  import { API_BASE_URL } from '../config';

  let selectedFile: File | null = null;
  let dragOver = false;
  let uploading = false;
  let uploadProgress = 0;
  let uploadSuccess = false;
  let uploadError = '';
  let fileName = '';
  let fileSize = '';
  let fileType = '';
  let previewUrl = '';

  let metadataTitle = '';
  let metadataAuthor = '';
  let metadataDocType = 'Notes';
  let metadataDewey = '';
  let metadataPublishDate = new Date().toISOString().split('T')[0];
  let metadataContent = '';

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
      docTypes = ['Notes', 'Book', 'Article', 'Report', 'Manual', 'Reference'];
    }
  }

  function handleFileSelect(event: Event) {
    const target = event.target as HTMLInputElement;
    if (target.files && target.files[0]) {
      setFileInfo(target.files[0]);
    }
  }

  function handleDragOver(event: DragEvent) {
    event.preventDefault();
    dragOver = true;
  }

  function handleDragLeave() {
    dragOver = false;
  }

  function handleDrop(event: DragEvent) {
    event.preventDefault();
    dragOver = false;
    if (event.dataTransfer && event.dataTransfer.files[0]) {
      setFileInfo(event.dataTransfer.files[0]);
    }
  }

  function setFileInfo(file: File) {
    selectedFile = file;
    fileName = file.name;
    fileSize = formatFileSize(file.size);
    fileType = file.type || 'Unknown';
    metadataTitle = file.name.replace(/\.[^/.]+$/, '');
    uploadSuccess = false;
    uploadError = '';

    if (previewUrl) {
      URL.revokeObjectURL(previewUrl);
      previewUrl = '';
    }
    if (file.type.startsWith('image/')) {
      previewUrl = URL.createObjectURL(file);
    }
  }

  function formatFileSize(bytes: number): string {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  function openFileDialog() {
    const input = document.createElement('input');
    input.type = 'file';
    input.multiple = false;
    input.onchange = (event) => {
      const target = event.target as HTMLInputElement;
      if (target.files && target.files[0]) {
        setFileInfo(target.files[0]);
      }
    };
    input.click();
  }

  function validateForm(): string | null {
    if (!selectedFile) return 'Please select a file';
    if (!metadataTitle.trim()) return 'Title is required';
    if (!metadataDocType) return 'Document type is required';
    if (selectedFile.size > 100 * 1024 * 1024) return 'File exceeds 100MB limit';
    return null;
  }

  async function handleUpload() {
    const validationError = validateForm();
    if (validationError) {
      uploadError = validationError;
      return;
    }

    uploading = true;
    uploadProgress = 0;
    uploadSuccess = false;
    uploadError = '';

    const formData = new FormData();
    formData.append('file', selectedFile!);

    const metadataObj: Record<string, string> = {
      DocType: metadataDocType,
      Title: metadataTitle,
      Author: metadataAuthor || 'Unknown',
      PublishDate: metadataPublishDate,
    };
    if (metadataDewey) {
      metadataObj.DeweyDecimal = metadataDewey;
    }
    if (metadataContent) {
      metadataObj.Content = metadataContent;
    }
    formData.append('metadata', JSON.stringify(metadataObj));

    const xhr = new XMLHttpRequest();
    xhr.open('POST', `${API_BASE_URL}/file/upload`, true);

    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) {
        uploadProgress = (event.loaded / event.total) * 100;
      }
    };

    xhr.onload = () => {
      uploading = false;
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          JSON.parse(xhr.responseText);
          uploadSuccess = true;
        } catch {
          uploadError = 'Invalid response format';
        }
      } else {
        try {
          const errData = JSON.parse(xhr.responseText);
          uploadError = errData.error || `Upload failed (${xhr.status})`;
        } catch {
          uploadError = `Upload failed (${xhr.status})`;
        }
      }
    };

    xhr.onerror = () => {
      uploading = false;
      uploadError = 'Network error - is the backend running?';
    };

    xhr.send(formData);
  }

  function removeFile() {
    selectedFile = null;
    fileName = '';
    fileSize = '';
    fileType = '';
    if (previewUrl) {
      URL.revokeObjectURL(previewUrl);
      previewUrl = '';
    }
    metadataTitle = '';
    metadataAuthor = '';
    metadataDocType = 'Notes';
    metadataDewey = '';
    metadataPublishDate = new Date().toISOString().split('T')[0];
    metadataContent = '';
    uploadSuccess = false;
    uploadError = '';
  }

  function resetAfterSuccess() {
    removeFile();
  }

  onMount(loadOptions);
</script>

<div class="add-container">
  <div class="upload-section">
    <h2 class="section-title">Upload File</h2>
    <p class="section-description">
      Drag and drop a file or click to browse. Supported formats include documents, images, audio, and video.
    </p>

    <div
      class="upload-area"
      class:drag-over={dragOver}
      class:has-file={selectedFile}
      on:dragover={handleDragOver}
      on:dragleave={handleDragLeave}
      on:drop={handleDrop}
    >
      {#if selectedFile}
        <div class="file-preview-section">
          {#if previewUrl}
            <div class="image-preview">
              <img src={previewUrl} alt="Preview" />
            </div>
          {:else}
            <div class="file-icon-large">
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                <polyline points="14,2 14,8 20,8"></polyline>
              </svg>
            </div>
          {/if}
          <div class="file-details">
            <h3 class="file-name">{fileName}</h3>
            <p class="file-meta">{fileSize} &middot; {fileType}</p>
          </div>
          <button class="remove-button" on:click={removeFile} aria-label="Remove file">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>
        </div>
      {:else}
        <div class="upload-placeholder" on:click={openFileDialog}>
          <div class="upload-icon">
            <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
              <polyline points="17,8 12,3 7,8"></polyline>
              <line x1="12" y1="3" x2="12" y2="15"></line>
            </svg>
          </div>
          <h3 class="upload-title">Drop your file here</h3>
          <p class="upload-subtitle">or click to browse</p>
          <button class="browse-button">Choose File</button>
        </div>
      {/if}
    </div>

    {#if selectedFile}
      <div class="metadata-section">
        <h3 class="meta-title">Document Details</h3>

        <div class="form-grid">
          <div class="form-group full-width">
            <label for="meta-title">Title <span class="required">*</span></label>
            <input id="meta-title" type="text" bind:value={metadataTitle} placeholder="Document title" />
          </div>

          <div class="form-group">
            <label for="meta-author">Author</label>
            <input id="meta-author" type="text" bind:value={metadataAuthor} placeholder="Author name" />
          </div>

          <div class="form-group">
            <label for="meta-date">Publish Date</label>
            <input id="meta-date" type="date" bind:value={metadataPublishDate} />
          </div>

          <div class="form-group">
            <label for="meta-doctype">Document Type <span class="required">*</span></label>
            {#if docTypes.length > 0}
              <select id="meta-doctype" bind:value={metadataDocType}>
                {#each docTypes as dt}
                  <option value={dt}>{dt}</option>
                {/each}
              </select>
            {:else}
              <input id="meta-doctype" type="text" bind:value={metadataDocType} placeholder="e.g. Notes" />
            {/if}
          </div>

          <div class="form-group">
            <label for="meta-dewey">Dewey Decimal</label>
            {#if deweyCategories.length > 0}
              <select id="meta-dewey" bind:value={metadataDewey}>
                <option value="">-- None --</option>
                {#each deweyCategories as cat}
                  <option value={cat.code}>{cat.code} - {cat.name}</option>
                {/each}
              </select>
            {:else}
              <input id="meta-dewey" type="text" bind:value={metadataDewey} placeholder="e.g. 510" />
            {/if}
          </div>

          <div class="form-group full-width">
            <label for="meta-content">Notes / Content</label>
            <textarea id="meta-content" bind:value={metadataContent} placeholder="Optional notes or description" rows="3"></textarea>
          </div>
        </div>
      </div>

      <div class="upload-actions">
        {#if uploadSuccess}
          <div class="success-message">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="20,6 9,17 4,12"></polyline>
            </svg>
            <span>File uploaded successfully!</span>
            <button class="link-button" on:click={resetAfterSuccess}>Upload another</button>
          </div>
        {:else if uploadError}
          <div class="error-message">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10"></circle>
              <line x1="15" y1="9" x2="9" y2="15"></line>
              <line x1="9" y1="9" x2="15" y2="15"></line>
            </svg>
            <span>{uploadError}</span>
          </div>
        {/if}

        {#if !uploadSuccess}
          <button
            class="upload-button"
            class:uploading={uploading}
            on:click={handleUpload}
            disabled={uploading}
          >
            {#if uploading}
              <div class="spinner"></div>
              <span>Uploading... {uploadProgress.toFixed(0)}%</span>
            {:else}
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                <polyline points="17,8 12,3 7,8"></polyline>
                <line x1="12" y1="3" x2="12" y2="15"></line>
              </svg>
              <span>Upload to Library</span>
            {/if}
          </button>
        {/if}
      </div>
    {/if}
  </div>

  <div class="info-section">
    <h3 class="info-title">Supported Formats</h3>
    <div class="format-grid">
      <div class="format-item">
        <span class="format-icon doc-icon">DOC</span>
        <span class="format-name">Documents</span>
        <span class="format-ext">PDF, DOCX, TXT, MD, RTF, ODT, EPUB</span>
      </div>
      <div class="format-item">
        <span class="format-icon img-icon">IMG</span>
        <span class="format-name">Images</span>
        <span class="format-ext">JPG, JPEG, PNG, GIF, SVG</span>
      </div>
      <div class="format-item">
        <span class="format-icon aud-icon">AUD</span>
        <span class="format-name">Audio</span>
        <span class="format-ext">MP3, WAV, FLAC, AAC</span>
      </div>
      <div class="format-item">
        <span class="format-icon vid-icon">VID</span>
        <span class="format-name">Video</span>
        <span class="format-ext">MP4, AVI, MOV, MKV</span>
      </div>
    </div>

    <div class="upload-tips">
      <h4>Quick Tips</h4>
      <ul>
        <li>Maximum file size: <strong>100 MB</strong></li>
        <li>Files can be converted to PDF via the Library view</li>
        <li>Use Dewey Decimal codes to organise your collection</li>
        <li>All files are securely stored and indexed</li>
      </ul>
    </div>

    <div class="search-tips">
      <h4>Search Prefixes</h4>
      <ul>
        <li><code>author:</code> search by author</li>
        <li><code>type:</code> search by document type</li>
        <li><code>dewey:</code> search by Dewey code</li>
        <li><code>filetype:</code> search by file extension</li>
        <li>No prefix = fuzzy search across all fields</li>
      </ul>
    </div>
  </div>
</div>

<style>
  .add-container {
    display: grid;
    grid-template-columns: 1fr 340px;
    gap: 32px;
    height: 100%;
    overflow-y: auto;
  }

  .upload-section {
    display: flex;
    flex-direction: column;
  }

  .section-title {
    font-size: 24px;
    font-weight: 700;
    color: #ffffff;
    margin: 0 0 8px 0;
  }

  .section-description {
    color: rgba(255, 255, 255, 0.6);
    margin: 0 0 24px 0;
    line-height: 1.5;
  }

  .upload-area {
    border: 2px dashed rgba(255, 255, 255, 0.25);
    border-radius: 16px;
    padding: 48px 24px;
    text-align: center;
    transition: all 0.3s ease;
    background: rgba(44, 44, 46, 0.5);
    min-height: 240px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .upload-area.drag-over {
    border-color: #007AFF;
    background: rgba(0, 122, 255, 0.08);
  }

  .upload-area.has-file {
    padding: 20px;
    min-height: auto;
    border-style: solid;
    border-color: rgba(255, 255, 255, 0.15);
  }

  .upload-placeholder {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 14px;
    cursor: pointer;
  }

  .upload-icon { color: rgba(255, 255, 255, 0.4); }
  .upload-title { font-size: 20px; font-weight: 600; color: #ffffff; margin: 0; }
  .upload-subtitle { color: rgba(255, 255, 255, 0.5); margin: 0; }

  .browse-button {
    background: #007AFF;
    color: #ffffff;
    border: none;
    padding: 12px 24px;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: background-color 0.2s ease;
  }

  .browse-button:hover { background: #0056CC; }

  .file-preview-section {
    display: flex;
    align-items: center;
    width: 100%;
    gap: 16px;
  }

  .image-preview {
    width: 80px;
    height: 80px;
    border-radius: 8px;
    overflow: hidden;
    flex-shrink: 0;
    background: rgba(0, 0, 0, 0.3);
  }

  .image-preview img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .file-icon-large {
    color: #007AFF;
    flex-shrink: 0;
  }

  .file-details { flex: 1; text-align: left; }
  .file-name { font-size: 16px; font-weight: 600; color: #ffffff; margin: 0 0 4px 0; }
  .file-meta { color: rgba(255, 255, 255, 0.5); margin: 0; font-size: 14px; }

  .remove-button {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.5);
    cursor: pointer;
    padding: 8px;
    border-radius: 6px;
    transition: all 0.2s ease;
  }

  .remove-button:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .metadata-section {
    margin-top: 24px;
    background: rgba(44, 44, 46, 0.6);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 12px;
    padding: 20px;
  }

  .meta-title {
    font-size: 16px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 16px 0;
  }

  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 14px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  .form-group.full-width {
    grid-column: 1 / -1;
  }

  .form-group label {
    font-size: 12px;
    font-weight: 600;
    color: rgba(255, 255, 255, 0.6);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .required { color: #FF3B30; }

  .form-group input,
  .form-group select,
  .form-group textarea {
    padding: 10px 12px;
    background: rgba(255, 255, 255, 0.08);
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 8px;
    color: #ffffff;
    font-size: 14px;
    font-family: inherit;
    transition: border-color 0.2s ease;
  }

  .form-group input:focus,
  .form-group select:focus,
  .form-group textarea:focus {
    outline: none;
    border-color: #007AFF;
    background: rgba(255, 255, 255, 0.12);
  }

  .form-group select option {
    background: #2c2c2e;
    color: #ffffff;
  }

  .form-group textarea {
    resize: vertical;
    min-height: 60px;
  }

  .upload-actions { margin-top: 20px; }

  .success-message {
    display: flex;
    align-items: center;
    gap: 8px;
    background: rgba(52, 199, 89, 0.1);
    color: #34C759;
    padding: 12px 16px;
    border-radius: 8px;
    margin-bottom: 12px;
    border: 1px solid rgba(52, 199, 89, 0.2);
  }

  .link-button {
    background: none;
    border: none;
    color: #34C759;
    text-decoration: underline;
    cursor: pointer;
    font-size: 14px;
    margin-left: auto;
  }

  .error-message {
    display: flex;
    align-items: center;
    gap: 8px;
    background: rgba(255, 59, 48, 0.1);
    color: #FF3B30;
    padding: 12px 16px;
    border-radius: 8px;
    margin-bottom: 12px;
    border: 1px solid rgba(255, 59, 48, 0.2);
  }

  .upload-button {
    display: flex;
    align-items: center;
    gap: 8px;
    background: #007AFF;
    color: #ffffff;
    border: none;
    padding: 14px 32px;
    border-radius: 8px;
    font-size: 16px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    width: 100%;
    justify-content: center;
  }

  .upload-button:hover:not(:disabled) { background: #0056CC; }
  .upload-button:disabled { opacity: 0.7; cursor: not-allowed; }

  .spinner {
    width: 20px;
    height: 20px;
    border: 2px solid rgba(255, 255, 255, 0.3);
    border-top: 2px solid #ffffff;
    border-radius: 50%;
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    0% { transform: rotate(0deg); }
    100% { transform: rotate(360deg); }
  }

  .info-section {
    background: rgba(44, 44, 46, 0.5);
    border-radius: 16px;
    padding: 24px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    height: fit-content;
  }

  .info-title {
    font-size: 18px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 20px 0;
  }

  .format-grid {
    display: grid;
    gap: 10px;
    margin-bottom: 24px;
  }

  .format-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    background: rgba(255, 255, 255, 0.04);
    border-radius: 8px;
  }

  .format-icon {
    font-size: 11px;
    font-weight: 800;
    padding: 4px 8px;
    border-radius: 4px;
    flex-shrink: 0;
  }

  .doc-icon { background: rgba(0, 122, 255, 0.2); color: #007AFF; }
  .img-icon { background: rgba(52, 199, 89, 0.2); color: #34C759; }
  .aud-icon { background: rgba(255, 149, 0, 0.2); color: #FF9500; }
  .vid-icon { background: rgba(175, 82, 222, 0.2); color: #AF52DE; }

  .format-name { font-weight: 600; color: #ffffff; flex: 1; font-size: 14px; }
  .format-ext { font-size: 11px; color: rgba(255, 255, 255, 0.45); }

  .upload-tips, .search-tips {
    border-top: 1px solid rgba(255, 255, 255, 0.08);
    padding-top: 16px;
    margin-top: 16px;
  }

  .upload-tips h4, .search-tips h4 {
    font-size: 14px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 10px 0;
  }

  .upload-tips ul, .search-tips ul {
    margin: 0;
    padding-left: 18px;
    color: rgba(255, 255, 255, 0.6);
    line-height: 1.8;
    font-size: 13px;
  }

  .search-tips code {
    background: rgba(255, 255, 255, 0.1);
    padding: 1px 5px;
    border-radius: 3px;
    font-size: 12px;
    color: #007AFF;
  }

  @media (max-width: 768px) {
    .add-container {
      grid-template-columns: 1fr;
      gap: 24px;
    }

    .form-grid {
      grid-template-columns: 1fr;
    }

    .upload-area {
      padding: 32px 16px;
      min-height: 200px;
    }
  }
</style>
