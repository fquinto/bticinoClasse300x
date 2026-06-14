<script>
  import { onMount } from 'svelte'
  
  let memos = []
  let loading = true
  let error = null
  let filter = 'all'
  let selectedMemo = null
  
  onMount(async () => {
    await loadMemos()
  })
  
  async function loadMemos() {
    try {
      loading = true
      const response = await fetch('/api/memos')
      const data = await response.json()
      
      memos = data.memos || []
      error = null
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
  
  async function toggleRead(memo, event) {
    event.stopPropagation()
    try {
      const newReadState = !memo.read
      const response = await fetch(`/api/memos/mark-read/${memo.id}/${newReadState ? 'read' : 'unread'}/${memo.type}`, {
        method: 'POST'
      })
      const result = await response.json()
      
      if (result.success) {
        memo.read = newReadState
        memos = [...memos]
        if (selectedMemo && selectedMemo.id === memo.id) {
          selectedMemo.read = newReadState
        }
      }
    } catch (e) {
      error = e.message
    }
  }
  
  async function deleteMemo(memo, event) {
    event.stopPropagation()
    if (!confirm(`¿Eliminar esta nota de ${memo.type}?`)) {
      return
    }
    try {
      const response = await fetch(`/api/memos/delete/${memo.id}/${memo.type}`, {
        method: 'DELETE'
      })
      const result = await response.json()
      
      if (result.success) {
        memos = memos.filter(m => !(m.id === memo.id && m.type === memo.type))
        if (selectedMemo && selectedMemo.id === memo.id) {
          selectedMemo = null
        }
      }
    } catch (e) {
      error = e.message
    }
  }
  
  $: filteredMemos = filter === 'all' ? memos : memos.filter(m => m.type === filter)
  
  function formatDate(timestamp) {
    if (!timestamp || timestamp === '0001-01-01T00:00:00Z') {
      return 'Unknown'
    }
    return new Date(timestamp).toLocaleString()
  }
  
  function getTypeIcon(type) {
    return type === 'voice' ? '🎤' : '📝'
  }
  
  function viewMemo(memo) {
    selectedMemo = memo
  }
  
  function closeModal() {
    selectedMemo = null
  }
</script>

<div class="memos-page">
  <div class="container">
    <h2>📝 Notas (Notes)</h2>
    
    <div class="filters">
      <button class="filter-btn" class:active={filter === 'all'} on:click={() => filter = 'all'}>
        All ({memos.length})
      </button>
      <button class="filter-btn" class:active={filter === 'voice'} on:click={() => filter = 'voice'}>
        🎤 Voice ({memos.filter(m => m.type === 'voice').length})
      </button>
      <button class="filter-btn" class:active={filter === 'text'} on:click={() => filter = 'text'}>
        📝 Text ({memos.filter(m => m.type === 'text').length})
      </button>
    </div>
    
    {#if loading}
      <div class="loading">Loading memos...</div>
    {:else if error}
      <div class="error">Error: {error}</div>
    {:else if filteredMemos.length === 0}
      <div class="empty">No memos found</div>
    {:else}
      <div class="memos-grid">
        {#each filteredMemos as memo}
          <div class="memo-card" class:unread={!memo.read} on:click={() => viewMemo(memo)}>
            <div class="memo-header">
              <span class="memo-icon">{getTypeIcon(memo.type)}</span>
              <span class="memo-type">{memo.type}</span>
              {#if !memo.read}
                <span class="unread-badge">New</span>
              {/if}
            </div>
            <div class="memo-date">{formatDate(memo.timestamp)}</div>
            {#if memo.type === 'text' && memo.content}
              <div class="memo-preview">{memo.content.substring(0, 50)}...</div>
            {:else if memo.type === 'voice'}
              <div class="memo-audio">🎵 Audio memo</div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

{#if selectedMemo}
<div class="modal" on:click={closeModal}>
  <div class="modal-content" on:click|stopPropagation>
    <div class="modal-header">
      <h2>{getTypeIcon(selectedMemo.type)} {selectedMemo.type === 'voice' ? 'Nota de voz' : 'Nota de texto'} #{selectedMemo.id}</h2>
      <button class="close-btn" on:click={closeModal}>×</button>
    </div>
    
    <div class="modal-body">
      <div class="detail-row">
        <strong>Estado:</strong>
        <span class:unread={!selectedMemo.read}>{selectedMemo.read ? 'Leído' : 'No leído'}</span>
        <button class="toggle-read-btn" on:click={(e) => toggleRead(selectedMemo, e)}>
          {selectedMemo.read ? 'Marcar como no leído' : 'Marcar como leído'}
        </button>
      </div>
      <div class="detail-row">
        <strong>Fecha:</strong>
        <span>{formatDate(selectedMemo.timestamp)}</span>
      </div>
      
      {#if selectedMemo.type === 'text' && selectedMemo.content}
        <div class="text-content">
          <h3>Contenido:</h3>
          <p>{selectedMemo.content}</p>
        </div>
      {/if}
      
      {#if selectedMemo.type === 'voice' && selectedMemo.audio_path}
        <div class="audio-section">
          <h3>Audio:</h3>
          <audio controls src="/api/memos/audio/{selectedMemo.id}">
            Tu navegador no soporta audio
          </audio>
        </div>
      {/if}
      
      <div class="actions-row">
        <button class="delete-btn" on:click={(e) => deleteMemo(selectedMemo, e)}>
          🗑️ Eliminar nota
        </button>
      </div>
    </div>
  </div>
</div>
{/if}

<style>
  .memos-page { padding: 2rem; }
  .container { max-width: 1200px; margin: 0 auto; }
  h2 { color: #333; margin-bottom: 1.5rem; }
  
  .filters {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1.5rem;
  }
  
  .filter-btn {
    padding: 0.5rem 1rem;
    border: 1px solid #ddd;
    background: white;
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.3s;
  }
  
  .filter-btn:hover { background: #f5f5f5; }
  .filter-btn.active { background: #2196F3; color: white; border-color: #2196F3; }
  
  .loading, .error, .empty {
    text-align: center;
    padding: 3rem;
    font-size: 1.2rem;
  }
  
  .error { color: #f44336; background: #FFEBEE; border-radius: 8px; }
  .empty { color: #999; }
  
  .memos-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
    gap: 1rem;
  }
  
  .memo-card {
    background: white;
    padding: 1rem;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    cursor: pointer;
    transition: all 0.3s;
  }
  
  .memo-card:hover { transform: translateY(-2px); box-shadow: 0 4px 8px rgba(0,0,0,0.15); }
  
  .memo-card.unread { border-left: 4px solid #2196F3; }
  
  .memo-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }
  
  .memo-icon { font-size: 1.5rem; }
  .memo-type { text-transform: uppercase; font-size: 0.8rem; color: #666; }
  
  .unread-badge {
    background: #2196F3;
    color: white;
    font-size: 0.7rem;
    padding: 0.2rem 0.5rem;
    border-radius: 10px;
    margin-left: auto;
  }
  
  .memo-date { font-size: 0.85rem; color: #999; margin-bottom: 0.5rem; }
  .memo-preview { font-size: 0.9rem; color: #666; }
  .memo-audio { font-size: 0.9rem; color: #666; }
  
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
    width: 90%;
    max-width: 500px;
    max-height: 80vh;
    overflow-y: auto;
  }
  
  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem 1.5rem;
    border-bottom: 1px solid #eee;
  }
  
  .modal-header h2 { margin: 0; font-size: 1.2rem; }
  .close-btn { background: none; border: none; font-size: 1.5rem; cursor: pointer; }
  
  .modal-body { padding: 1.5rem; }
  
  .detail-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.5rem 0;
    border-bottom: 1px solid #eee;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  
  .detail-row .unread { color: #2196F3; font-weight: bold; }
  
  .toggle-read-btn {
    background: #2196F3;
    color: white;
    border: none;
    padding: 0.4rem 0.8rem;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.85rem;
  }
  
  .toggle-read-btn:hover { background: #1976D2; }
  
  .text-content, .audio-section { margin-top: 1rem; }
  .text-content h3, .audio-section h3 { margin: 0 0 0.5rem; color: #666; }
  .text-content p { background: #f5f5f5; padding: 1rem; border-radius: 8px; white-space: pre-wrap; }
  
  .actions-row { margin-top: 1.5rem; padding-top: 1rem; border-top: 1px solid #eee; }
  
  .delete-btn {
    background: #f44336;
    color: white;
    border: none;
    padding: 0.6rem 1rem;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.9rem;
    width: 100%;
  }
  
  .delete-btn:hover { background: #d32f2f; }
  
  audio { width: 100%; }
</style>
