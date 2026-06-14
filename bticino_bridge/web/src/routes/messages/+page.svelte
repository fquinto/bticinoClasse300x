<script>
  import { onMount } from 'svelte'
  
  let messages = []
  let pagination = { page: 1, total: 0, total_pages: 1, limit: 10 }
  let loading = true
  let error = null
  let selectedMessage = null
  let filter = 'all'
  
  let page = 1
  const limit = 10
  
  onMount(async () => {
    await loadMessages()
  })
  
  async function loadMessages() {
    try {
      loading = true
      const url = `/api/messages/list?page=${page}&limit=${limit}${filter !== 'all' ? '&unread_only=' + (filter === 'unread') : ''}`
      const response = await fetch(url)
      const data = await response.json()
      
      messages = data.messages || []
      pagination = data.pagination || { page: 1, total: 0, total_pages: 1, limit: 10 }
      error = null
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
  
  async function markAsRead(msgId) {
    try {
      const response = await fetch(`/api/messages/mark-read/${msgId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ read: true })
      })
      const result = await response.json()
      
      if (result.success) {
        messages = messages.map(m => m.id === msgId ? { ...m, read: true } : m)
      }
    } catch (e) {
      error = e.message
    }
  }
  
  async function markAsUnread(msgId) {
    try {
      const response = await fetch(`/api/messages/mark-read/${msgId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ read: false })
      })
      const result = await response.json()
      
      if (result.success) {
        messages = messages.map(m => m.id === msgId ? { ...m, read: false } : m)
      }
    } catch (e) {
      error = e.message
    }
  }
  
  async function deleteMessage(msgId) {
    if (!confirm('Delete this message?')) return
    
    try {
      const response = await fetch(`/api/messages/delete/${msgId}`, { method: 'DELETE' })
      const result = await response.json()
      
      if (result.success) {
        messages = messages.filter(m => m.id !== msgId)
      }
    } catch (e) {
      error = e.message
    }
  }
  
  function viewMessage(msg) {
    selectedMessage = msg
    if (!msg.read) {
      markAsRead(msg.id)
    }
  }
  
  function closeModal() {
    selectedMessage = null
  }
  
  function changePage(newPage) {
    page = newPage
    loadMessages()
  }
</script>

<div class="messages-page">
  <div class="page-header">
    <h1>📬 Messages</h1>
    <div class="header-stats">
      {pagination.total} messages ({pagination.total - messages.filter(m => m.read).length} unread)
    </div>
  </div>

  <div class="container">
    <div class="controls-bar">
      <div class="filter-group">
        <label>Filter:</label>
        <select bind:value={filter} on:change={() => { page = 1; loadMessages() }}>
          <option value="all">All</option>
          <option value="unread">Unread</option>
          <option value="read">Read</option>
        </select>
      </div>
      <button class="btn btn-secondary" on:click={loadMessages}>🔄 Refresh</button>
    </div>
    
    {#if loading}
      <div class="loading">Loading messages...</div>
    {:else if error}
      <div class="error">Error: {error}</div>
    {:else if messages.length === 0}
      <div class="empty">No messages found</div>
    {:else}
      <div class="messages-grid">
        {#each messages as msg}
          <div class="message-card" class:unread={!msg.read} on:click={() => viewMessage(msg)}>
            <div class="message-header">
              <span class="caller-id">{msg.caller_id || 'Unknown'}</span>
              <span class="timestamp">{msg.timestamp?.split('T')[0] || ''}</span>
            </div>
            <div class="message-preview">{msg.message?.substring(0, 80) || ''}...</div>
            <div class="message-meta">
              {#if msg.has_image}
                <span class="badge">📷 Photo</span>
              {/if}
              {#if msg.has_video}
                <span class="badge">🎥 Video</span>
              {/if}
              {#if msg.duration}
                <span class="badge">⏱️ {msg.duration}</span>
              {/if}
            </div>
            <div class="message-actions">
              {#if msg.read}
                <button class="btn-sm" on:click|stopPropagation={() => markAsUnread(msg.id)}>Mark Unread</button>
              {:else}
                <button class="btn-sm" on:click|stopPropagation={() => markAsRead(msg.id)}>Mark Read</button>
              {/if}
              <button class="btn-sm btn-danger" on:click|stopPropagation={() => deleteMessage(msg.id)}>Delete</button>
            </div>
          </div>
        {/each}
      </div>
      
      {#if pagination.total_pages > 1}
        <div class="pagination">
          <button disabled={page === 1} on:click={() => changePage(page - 1)}>← Previous</button>
          <span>Page {page} of {pagination.total_pages}</span>
          <button disabled={page >= pagination.total_pages} on:click={() => changePage(page + 1)}>Next →</button>
        </div>
      {/if}
    {/if}
  </div>
</div>

{#if selectedMessage}
<div class="modal" on:click={closeModal}>
  <div class="modal-content" on:click|stopPropagation>
    <div class="modal-header">
      <h2>{selectedMessage.caller_id}</h2>
      <button class="close-btn" on:click={closeModal}>×</button>
    </div>
    <div class="modal-body">
      <p><strong>Date:</strong> {selectedMessage.timestamp}</p>
      <p><strong>Duration:</strong> {selectedMessage.duration}</p>
      <p><strong>Message:</strong> {selectedMessage.message}</p>
      
      {#if selectedMessage.image_base64}
        <div class="media-section">
          <h3>📷 Photo</h3>
          <img src="data:image/jpeg;base64,{selectedMessage.image_base64}" alt="Message photo" />
        </div>
      {/if}
      
      {#if selectedMessage.has_video}
        <div class="media-section">
          <h3>🎥 Video</h3>
          <div class="download-buttons">
            <a href="/api/messages/download/{selectedMessage.id}/video" class="btn btn-primary" download>
              ⬇️ Download Video
            </a>
          </div>
        </div>
      {/if}
      
      <div class="modal-actions">
        {#if selectedMessage.read}
          <button class="btn btn-secondary" on:click={() => markAsUnread(selectedMessage.id)}>Mark Unread</button>
        {:else}
          <button class="btn btn-secondary" on:click={() => markAsRead(selectedMessage.id)}>Mark Read</button>
        {/if}
        <button class="btn btn-danger" on:click={() => deleteMessage(selectedMessage.id)}>Delete</button>
      </div>
    </div>
  </div>
</div>
{/if}

<style>
  .messages-page { min-height: 100vh; }
  
  .page-header {
    background: linear-gradient(135deg, #2196F3, #1976D2);
    color: white;
    padding: 1rem 2rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  
  .page-header h1 { margin: 0; font-size: 1.5rem; }
  .header-stats { font-size: 0.9rem; opacity: 0.9; }
  
  .container { max-width: 1200px; margin: 0 auto; padding: 1rem 2rem 2rem; }
  
  .controls-bar {
    background: white;
    padding: 1rem;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    display: flex;
    gap: 1rem;
    margin-bottom: 1rem;
    align-items: center;
  }
  
  .filter-group { display: flex; align-items: center; gap: 0.5rem; }
  .filter-group select { padding: 0.5rem; border: 1px solid #ddd; border-radius: 4px; }
  
  .messages-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 1rem;
  }
  
  .message-card {
    background: white;
    padding: 1rem;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    cursor: pointer;
    transition: transform 0.2s, box-shadow 0.2s;
    border-left: 4px solid #4CAF50;
  }
  
  .message-card:hover { transform: translateY(-2px); box-shadow: 0 4px 8px rgba(0,0,0,0.15); }
  .message-card.unread { border-left-color: #f44336; background: #fff8f8; }
  
  .message-header {
    display: flex;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }
  
  .caller-id { font-weight: bold; color: #333; }
  .timestamp { color: #666; font-size: 0.85rem; }
  .message-preview { color: #555; font-size: 0.9rem; margin-bottom: 0.5rem; }
  
  .message-meta { display: flex; gap: 0.5rem; margin-bottom: 0.5rem; flex-wrap: wrap; }
  .badge { background: #e0e0e0; padding: 0.2rem 0.5rem; border-radius: 4px; font-size: 0.75rem; }
  
  .message-actions { display: flex; gap: 0.5rem; }
  
  .btn { padding: 0.5rem 1rem; border: none; border-radius: 4px; cursor: pointer; font-size: 0.9rem; }
  .btn-secondary { background: #e0e0e0; color: #333; }
  .btn-danger { background: #f44336; color: white; }
  
  .btn-sm { padding: 0.3rem 0.6rem; font-size: 0.8rem; background: #2196F3; color: white; border: none; border-radius: 4px; cursor: pointer; }
  .btn-sm.btn-danger { background: #f44336; }
  
  .pagination { display: flex; justify-content: center; align-items: center; gap: 1rem; margin-top: 1.5rem; }
  .pagination button { padding: 0.5rem 1rem; background: #2196F3; color: white; border: none; border-radius: 4px; cursor: pointer; }
  .pagination button:disabled { opacity: 0.5; cursor: not-allowed; }
  
  .loading, .error, .empty { text-align: center; padding: 3rem; font-size: 1.2rem; border-radius: 8px; }
  .error { background: #FFEBEE; color: #f44336; }
  .empty { background: #f5f5f5; color: #666; }
  
  .modal {
    position: fixed;
    top: 0; left: 0; right: 0; bottom: 0;
    background: rgba(0,0,0,0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }
  
  .modal-content {
    background: white;
    border-radius: 12px;
    max-width: 600px;
    width: 90%;
    max-height: 90vh;
    overflow: auto;
  }
  
  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem;
    border-bottom: 1px solid #eee;
  }
  
  .modal-header h2 { margin: 0; }
  .close-btn { background: none; border: none; font-size: 1.5rem; cursor: pointer; }
  
  .modal-body { padding: 1rem; }
  .modal-body p { margin: 0.5rem 0; }
  
  .media-section { margin: 1rem 0; }
  .media-section img { max-width: 100%; border-radius: 8px; }
  .download-buttons { margin: 0.5rem 0; }
  .download-buttons a { text-decoration: none; }
  
  .modal-actions { display: flex; gap: 1rem; margin-top: 1rem; }
</style>
