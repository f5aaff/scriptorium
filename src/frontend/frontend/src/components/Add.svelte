<script lang="ts">
  import { createEventDispatcher } from 'svelte';

  const dispatch = createEventDispatcher();

  let selectedFile: File | null = null;
  let dragOver = false;
  let uploading = false;
  let uploadProgress = 0;
  let uploadSuccess = false;
  let uploadError = '';
  let fileName = '';
  let fileSize = '';
  let fileType = '';

  // Metadata fields
  let metadataTitle = '';
  let metadataAuthor = '';
  let metadataDocType = '';
  let metadataContent = '';

  function handleFileSelect(event: Event) {
    const target = event.target as HTMLInputElement;
    if (target.files && target.files[0]) {
      const file = target.files[0];
      setFileInfo(file);
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
      const file = event.dataTransfer.files[0];
      setFileInfo(file);
    }
  }

function setFileInfo(file: File) {
    selectedFile = file;
    fileName = file.name;
    fileSize = formatFileSize(file.size);
    fileType = file.type || 'Unknown';

    // Reset metadata defaults
    metadataTitle = file.name;
    metadataAuthor = '';
    metadataDocType = 'Notes';
    metadataContent = '';
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
async function uploadFile(file: File, metadata?: any): Promise<void> {
  uploading = true;
  uploadProgress = 0;
  uploadSuccess = false;
  uploadError = '';

  return new Promise<void>((resolve, reject) => {
    const formData = new FormData();
    formData.append("file", file);

    // Add metadata to form data if provided
    if (metadata) {
      const metadataObj = {
        DocType: metadata.DocType || "Notes",
        Title: metadata.Title || file.name,
        Author: metadata.Author || "Unknown",
        PublishDate: metadata.PublishDate || new Date().toISOString().split("T")[0],
        Content: metadata.Content || ""
      };
      formData.append("metadata", JSON.stringify(metadataObj));
    }

    const xhr = new XMLHttpRequest();
    xhr.open("POST", "http://localhost:8080/file/upload", true);

    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) {
        uploadProgress = (event.loaded / event.total) * 100;
      }
    };

    xhr.onload = () => {
      uploading = false;
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          const response = JSON.parse(xhr.responseText);
          console.log("Upload successful:", response);
          
          // Show success message with file path and document UUID
          if (response.document_uuid) {
            console.log(`File uploaded successfully! Path: ${response.file_path}, Document UUID: ${response.document_uuid}`);
          } else {
            console.log(`File uploaded successfully! Path: ${response.file_path}`);
          }
          
          uploadSuccess = true;
          resolve();
        } catch (err) {
          console.error("Failed to parse response:", err);
          uploadError = "Invalid response format";
          reject(new Error("Invalid response format"));
        }
      } else {
        console.error("Upload failed:", xhr.responseText);
        uploadError = `Upload failed: ${xhr.status}`;
        reject(new Error(`Upload failed: ${xhr.status}`));
      }
    };

    xhr.onerror = () => {
      uploading = false;
      uploadError = "Network error";
      console.error("Network error");
      reject(new Error("Network error"));
    };

    xhr.send(formData);
  });
}

  function removeFile() {
    selectedFile = null;
    fileName = '';
    fileSize = '';
    fileType = '';
    // Clear metadata
    metadataTitle = '';
    metadataAuthor = '';
    metadataDocType = '';
    metadataContent = '';
  }
    function handleUpload() {
        if (selectedFile) {
            uploadFile(selectedFile, {
                Title: metadataTitle,
                Author: metadataAuthor,
                DocType: metadataDocType,
                Content: metadataContent
        });
    }
}
</script>

<div class="add-container">
  <div class="upload-section">
    <h2 class="section-title">Upload File</h2>
    <p class="section-description">
      Drag and drop a file here, or click "Choose File" to browse. Files will be automatically saved with unique names and can be linked to database records.
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
        <div class="file-info">
          <div class="file-icon">
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
              <polyline points="14,2 14,8 20,8"></polyline>
            </svg>
          </div>
          <div class="file-details">
            <h3 class="file-name">{fileName}</h3>
            <p class="file-meta">{fileSize} • {fileType}</p>
          </div>
          <button class="remove-button" on:click={removeFile} aria-label="Remove file">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>
        </div>
      <div class="metadata-form">
          <h4>File Metadata</h4>
          <label>
            Title:
            <input type="text" bind:value={metadataTitle} placeholder={fileName} />
          </label>
          <label>
            Author:
            <input type="text" bind:value={metadataAuthor} placeholder="Unknown" />
          </label>
          <label>
            Doc Type:
            <input type="text" bind:value={metadataDocType} placeholder="Notes" />
          </label>
          
          <label>
            Content (optional):
            <textarea bind:value={metadataContent} placeholder="Optional content for the document"></textarea>
          </label>
        </div>
      {:else}
        <div class="upload-placeholder" on:click={openFileDialog}>
          <div class="upload-icon">
            <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
              <polyline points="7,10 12,15 17,10"></polyline>
              <line x1="12" y1="15" x2="12" y2="3"></line>
            </svg>
          </div>
          <h3 class="upload-title">Drop your file here</h3>
          <p class="upload-subtitle">or click to browse</p>
          <button class="browse-button">Choose File</button>
        </div>
      {/if}
    </div>

    {#if selectedFile}
      <div class="upload-actions">
        {#if uploadSuccess}
          <div class="success-message">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="20,6 9,17 4,12"></polyline>
            </svg>
            <span>File uploaded successfully!</span>
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
        
        <button
          class="upload-button"
          class:uploading={uploading}
          on:click={() => selectedFile && uploadFile(selectedFile, {
            Title: metadataTitle,
            Author: metadataAuthor,
            DocType: metadataDocType,
            Content: metadataContent
          })}
          disabled={uploading}
        >
          {#if uploading}
            <div class="spinner"></div>
            <span>Uploading... {uploadProgress.toFixed(0)}%</span>
          {:else}
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
              <polyline points="7,10 12,15 17,10"></polyline>
              <line x1="12" y1="15" x2="12" y2="3"></line>
            </svg>
            <span>Upload to Library</span>
          {/if}
        </button>
      </div>
    {/if}
  </div>

  <div class="info-section">
    <h3 class="info-title">Supported Formats</h3>
    <div class="format-grid">
      <div class="format-item">
        <span class="format-icon">📄</span>
        <span class="format-name">Documents</span>
        <span class="format-ext">PDF, DOCX, TXT, MD</span>
      </div>
      <div class="format-item">
        <span class="format-icon">🖼️</span>
        <span class="format-name">Images</span>
        <span class="format-ext">JPG, PNG, GIF, SVG</span>
      </div>
      <div class="format-item">
        <span class="format-icon">🎵</span>
        <span class="format-name">Audio</span>
        <span class="format-ext">MP3, WAV, FLAC, AAC</span>
      </div>
      <div class="format-item">
        <span class="format-icon">🎬</span>
        <span class="format-name">Video</span>
        <span class="format-ext">MP4, AVI, MOV, MKV</span>
      </div>
    </div>

    <div class="upload-tips">
      <h4>Upload Tips</h4>
      <ul>
        <li>Maximum file size: 100MB</li>
        <li>Files are automatically processed and indexed</li>
        <li>You can organize files with tags and folders</li>
        <li>All uploaded files are securely stored</li>
      </ul>
    </div>
  </div>
</div>

<style>
  .add-container {
    display: grid;
    grid-template-columns: 1fr 1fr;
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
    color: rgba(255, 255, 255, 0.7);
    margin: 0 0 24px 0;
    line-height: 1.5;
  }

  .upload-area {
    border: 2px dashed rgba(255, 255, 255, 0.3);
    border-radius: 16px;
    padding: 48px 24px;
    text-align: center;
    transition: all 0.3s ease;
    background: rgba(44, 44, 46, 0.5);
    min-height: 300px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .upload-area.drag-over {
    border-color: #007AFF;
    background: rgba(0, 122, 255, 0.1);
  }

  .upload-area.has-file {
    padding: 24px;
    min-height: auto;
  }

  .upload-placeholder {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
  }

  .upload-icon {
    color: rgba(255, 255, 255, 0.5);
  }

  .upload-title {
    font-size: 20px;
    font-weight: 600;
    color: #ffffff;
    margin: 0;
  }

  .upload-subtitle {
    color: rgba(255, 255, 255, 0.6);
    margin: 0;
  }

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

  .browse-button:hover {
    background: #0056CC;
  }

  .file-info {
    display: flex;
    align-items: center;
    width: 100%;
    gap: 16px;
  }

  .file-icon {
    color: #007AFF;
    flex-shrink: 0;
  }

  .file-details {
    flex: 1;
    text-align: left;
  }

  .file-name {
    font-size: 16px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 4px 0;
  }

  .file-meta {
    color: rgba(255, 255, 255, 0.6);
    margin: 0;
    font-size: 14px;
  }

  .remove-button {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    padding: 8px;
    border-radius: 6px;
    transition: all 0.2s ease;
  }

  .remove-button:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .upload-actions {
    margin-top: 24px;
  }

  .success-message {
    display: flex;
    align-items: center;
    gap: 8px;
    background: rgba(52, 199, 89, 0.1);
    color: #34C759;
    padding: 12px 16px;
    border-radius: 8px;
    margin-bottom: 16px;
    border: 1px solid rgba(52, 199, 89, 0.2);
  }

  .error-message {
    display: flex;
    align-items: center;
    gap: 8px;
    background: rgba(255, 59, 48, 0.1);
    color: #FF3B30;
    padding: 12px 16px;
    border-radius: 8px;
    margin-bottom: 16px;
    border: 1px solid rgba(255, 59, 48, 0.2);
  }

  .upload-button {
    display: flex;
    align-items: center;
    gap: 8px;
    background: #007AFF;
    color: #ffffff;
    border: none;
    padding: 16px 32px;
    border-radius: 8px;
    font-size: 16px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    width: 100%;
    justify-content: center;
  }

  .upload-button:hover:not(:disabled) {
    background: #0056CC;
  }

  .upload-button:disabled {
    opacity: 0.7;
    cursor: not-allowed;
  }

  .upload-button.uploading {
    background: #007AFF;
  }

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
  }

  .info-title {
    font-size: 18px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 20px 0;
  }

  .format-grid {
    display: grid;
    gap: 16px;
    margin-bottom: 32px;
  }

  .format-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px;
    background: rgba(255, 255, 255, 0.05);
    border-radius: 8px;
  }

  .format-icon {
    font-size: 20px;
    flex-shrink: 0;
  }

  .format-name {
    font-weight: 600;
    color: #ffffff;
    flex: 1;
  }

  .format-ext {
    font-size: 12px;
    color: rgba(255, 255, 255, 0.6);
  }

  .upload-tips {
    border-top: 1px solid rgba(255, 255, 255, 0.1);
    padding-top: 20px;
  }

  .upload-tips h4 {
    font-size: 16px;
    font-weight: 600;
    color: #ffffff;
    margin: 0 0 12px 0;
  }

  .upload-tips ul {
    margin: 0;
    padding-left: 20px;
    color: rgba(255, 255, 255, 0.7);
    line-height: 1.6;
  }

  .upload-tips li {
    margin-bottom: 4px;
  }

  @media (max-width: 768px) {
    .add-container {
      grid-template-columns: 1fr;
      gap: 24px;
    }

    .upload-area {
      padding: 32px 16px;
      min-height: 250px;
    }

    .upload-area.has-file {
      padding: 16px;
    }
  }
</style>
