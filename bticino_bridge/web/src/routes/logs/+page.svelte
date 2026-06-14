<script>
  import { onMount, onDestroy } from 'svelte'
  
  let logs = []
  let loading = true
  let error = null
  let autoRefresh = true
  let filterLevel = 'all'
  let searchTerm = ''
  let refreshInterval = null
  
  const levels = ['all', 'info', 'warn', 'error', 'debug']
  
  onMount(async () => {
    await loadLogs()
    
    // Auto-refresh every 5 seconds if enabled
    if (autoRefresh) {
      refreshInterval = setInterval(loadLogs, 5000)
    }
  })
  
  onDestroy(() => {
    if (refreshInterval) {
      clearInterval(refreshInterval)
    }
  })
  
  async function loadLogs() {
    try {
      const response = await fetch('/api/logs?count=100&level=' + filterLevel)
      if (!response.ok) throw new Error('Failed to load logs')
      const data = await response.json()
      logs = data.logs || []
      error = null
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
  
  function downloadLogs() {
    window.location.href = '/api/logs/download?count=500'
  }
  
  function clearLogs() {
    logs = []
  }
  
  // Filter logs by search term
  $: filteredLogs = logs.filter(log => {
    if (!searchTerm) return true
    const search = searchTerm.toLowerCase()
    return (
      log.message?.toLowerCase().includes(search) ||
      log.level?.toLowerCase().includes(search) ||
      log.timestamp?.toLowerCase().includes(search)
    )
  })
</script>

<div class="logs-page">
  <div class="page-header">
    <h1>📋 Logs</h1>
  </div>

  <div class="container">
    {#if loading}
      <div class="loading">Loading logs...</div>
    {:else if error}
      <div class="error">Error: {error}</div>
    {:else}
      <!-- Controls -->
      <div class="controls-bar">
        <div class="control-group">
          <label for="filter-level">Level:</label>
          <select id="filter-level" bind:value={filterLevel} on:change={loadLogs}>
            {#each levels as level}
            <option value={level}>{level.toUpperCase()}</option>
            {/each}
          </select>
        </div>
        
        <div class="control-group">
          <label for="search">Search:</label>
          <input 
            type="text" 
            id="search"
            bind:value={searchTerm}
            placeholder="Search logs..."
          />
        </div>
        
        <div class="control-group">
          <label>
            <input type="checkbox" bind:checked={autoRefresh} />
            Auto-refresh (5s)
          </label>
        </div>
        
        <div class="control-group buttons">
          <button class="btn btn-sm" on:click={loadLogs}>🔄 Refresh</button>
          <button class="btn btn-sm" on:click={downloadLogs}>💾 Download</button>
          <button class="btn btn-sm" on:click={clearLogs}>🗑️ Clear</button>
        </div>
      </div>

      <!-- Stats -->
      <div class="stats-bar">
        <span class="stat">Total: <strong>{logs.length}</strong></span>
        <span class="stat">Showing: <strong>{filteredLogs.length}</strong></span>
        <span class="stat">
          Errors: <strong class="error">{logs.filter(l => l.level === 'error').length}</strong>
        </span>
        <span class="stat">
          Warnings: <strong class="warn">{logs.filter(l => l.level === 'warn').length}</strong>
        </span>
      </div>

      <!-- Logs Table -->
      <div class="logs-container">
        {#if filteredLogs.length === 0}
          <div class="no-logs">No logs found</div>
        {:else}
          <table class="logs-table">
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Level</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {#each filteredLogs as log}
              <tr class="log-row level-{log.level}">
                <td class="timestamp">{log.timestamp}</td>
                <td class="level">
                  <span class="badge badge-{log.level}">{log.level}</span>
                </td>
                <td class="message">{log.message}</td>
              </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  :global(body) {
    margin: 0;
    font-family: 'Consolas', 'Monaco', monospace;
    background: #f5f5f5;
  }
  
  .page-header {
    background: linear-gradient(135deg, #2196F3, #1976D2);
    color: white;
    padding: 1rem 2rem;
  }
  
  .page-header h1 {
    margin: 0;
    font-size: 1.5rem;
  }
  
  .container {
    max-width: 1400px;
    margin: 2rem auto;
    padding: 0 2rem;
  }
  
  .controls-bar {
    background: white;
    padding: 1rem;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    display: flex;
    gap: 1.5rem;
    align-items: center;
    flex-wrap: wrap;
    margin-bottom: 1rem;
  }
  
  .control-group {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .control-group label {
    font-weight: 600;
    color: #555;
  }
  
  .control-group select,
  .control-group input[type="text"] {
    padding: 0.5rem;
    border: 1px solid #ddd;
    border-radius: 4px;
    font-size: 0.9rem;
  }
  
  .control-group input[type="text"] {
    width: 250px;
  }
  
  .control-group.buttons {
    margin-left: auto;
  }
  
  .btn {
    padding: 0.5rem 1rem;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.9rem;
    transition: all 0.3s;
  }
  
  .btn-sm {
    padding: 0.4rem 0.8rem;
    font-size: 0.85rem;
  }
  
  .stats-bar {
    background: white;
    padding: 1rem;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    display: flex;
    gap: 2rem;
    margin-bottom: 1rem;
  }
  
  .stat {
    color: #666;
  }
  
  .stat strong {
    color: #2196F3;
  }
  
  .stat strong.error {
    color: #f44336;
  }
  
  .stat strong.warn {
    color: #ff9800;
  }
  
  .logs-container {
    background: white;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    overflow: hidden;
  }
  
  .logs-table {
    width: 100%;
    border-collapse: collapse;
  }
  
  .logs-table thead {
    background: #f5f5f5;
    position: sticky;
    top: 0;
    z-index: 10;
  }
  
  .logs-table th {
    padding: 1rem;
    text-align: left;
    font-weight: 600;
    color: #555;
    border-bottom: 2px solid #ddd;
  }
  
  .logs-table td {
    padding: 0.75rem 1rem;
    border-bottom: 1px solid #eee;
  }
  
  .log-row:hover {
    background: #f9f9f9;
  }
  
  .timestamp {
    color: #666;
    font-size: 0.85rem;
    white-space: nowrap;
  }
  
  .level {
    white-space: nowrap;
  }
  
  .badge {
    padding: 0.25rem 0.75rem;
    border-radius: 12px;
    font-size: 0.75rem;
    font-weight: bold;
    text-transform: uppercase;
  }
  
  .badge-info {
    background: #E3F2FD;
    color: #1565C0;
  }
  
  .badge-debug {
    background: #F3E5F5;
    color: #7B1FA2;
  }
  
  .badge-warn {
    background: #FFF3E0;
    color: #E65100;
  }
  
  .badge-error {
    background: #FFEBEE;
    color: #C62828;
  }
  
  .message {
    color: #333;
    word-break: break-word;
  }
  
  .no-logs {
    text-align: center;
    padding: 3rem;
    color: #999;
    font-size: 1.2rem;
  }
  
  .loading, .error {
    text-align: center;
    padding: 3rem;
    font-size: 1.2rem;
  }
  
  .error {
    color: #f44336;
    background: #FFEBEE;
    border-radius: 8px;
  }
</style>
