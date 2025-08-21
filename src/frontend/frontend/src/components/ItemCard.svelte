<script lang="ts">
  export let item: any;
  export let onSelect: (item: any) => void;
</script>

<div class="item-card" on:click={() => onSelect(item)}>
  <div class="item-thumbnail">
    {#if item.thumbnail}
      <img src={item.thumbnail} alt={item.title} />
    {:else}
      <div class="placeholder">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
          <polyline points="14,2 14,8 20,8"></polyline>
        </svg>
      </div>
    {/if}
  </div>
  <div class="item-info">
    <h3 class="item-title">{item.title}</h3>
    <p class="item-type">{item.type}</p>
  </div>
</div>

<style>
  .item-card {
    background: rgba(44, 44, 46, 0.8);
    border-radius: 12px;
    padding: 16px;
    cursor: pointer;
    transition: all 0.2s ease;
    border: 1px solid rgba(255, 255, 255, 0.1);
  }
  
  .item-card:hover {
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
</style> 