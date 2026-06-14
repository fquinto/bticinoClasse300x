<script>
  import { onMount } from 'svelte'
  import Dashboard from './routes/+page.svelte'
  import Settings from './routes/settings/+page.svelte'
  import Controls from './routes/controls/+page.svelte'
  import Messages from './routes/messages/+page.svelte'
  import Memos from './routes/memos/+page.svelte'
  import Logs from './routes/logs/+page.svelte'
  
  let currentRoute = 'dashboard'
  let status = null
  
  onMount(() => {
    window.addEventListener('hashchange', handleRouteChange)
    handleRouteChange()
    loadStatus()
    setInterval(loadStatus, 30000)
  })
  
  function handleRouteChange() {
    const hash = window.location.hash.slice(1) || 'dashboard'
    currentRoute = hash
  }
  
  async function loadStatus() {
    try {
      const response = await fetch('/api/status')
      if (response.ok) {
        status = await response.json()
      }
    } catch (e) {
      console.error('Failed to load status:', e)
    }
  }
</script>

<main class="app">
  <nav class="navbar">
    <h1>
      🚀 BTicino Bridge
      {#if status?.version}
      <span class="version">{status.version}</span>
      {/if}
    </h1>
    <div class="nav-links">
      <a href="#dashboard" class={currentRoute === 'dashboard' ? 'active' : ''}>Dashboard</a>
      <a href="#settings" class={currentRoute === 'settings' ? 'active' : ''}>Settings</a>
      <a href="#controls" class={currentRoute === 'controls' ? 'active' : ''}>Controls</a>
      <a href="#messages" class={currentRoute === 'messages' ? 'active' : ''}>Messages</a>
      <a href="#memos" class={currentRoute === 'memos' ? 'active' : ''}>Notas</a>
      <a href="#logs" class={currentRoute === 'logs' ? 'active' : ''}>Logs</a>
      <a href="/api/docs/" target="_blank" class="external-link">API Docs ↗</a>
    </div>
  </nav>

  <div class="content">
    {#if currentRoute === 'dashboard'}
      <Dashboard />
    {:else if currentRoute === 'settings'}
      <Settings />
    {:else if currentRoute === 'controls'}
      <Controls />
    {:else if currentRoute === 'messages'}
      <Messages />
    {:else if currentRoute === 'memos'}
      <Memos />
    {:else if currentRoute === 'logs'}
      <Logs />
    {:else}
      <div class="not-found">
        <h2>404 - Page Not Found</h2>
        <a href="#dashboard">← Back to Dashboard</a>
      </div>
    {/if}
  </div>
</main>

<style>
  :global(body) {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #f5f5f5;
  }
  
  .app {
    min-height: 100vh;
  }
  
  .navbar {
    background: linear-gradient(135deg, #2196F3, #1976D2);
    color: white;
    padding: 1rem 2rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
    box-shadow: 0 2px 8px rgba(0,0,0,0.15);
    position: sticky;
    top: 0;
    z-index: 100;
  }
  
  .navbar h1 {
    margin: 0;
    font-size: 1.5rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  
  .version {
    font-size: 0.8rem;
    opacity: 0.8;
    font-weight: normal;
    background: rgba(255,255,255,0.2);
    padding: 0.25rem 0.75rem;
    border-radius: 12px;
  }
  
  .nav-links {
    display: flex;
    gap: 0.5rem;
  }
  
  .nav-links a {
    color: white;
    text-decoration: none;
    padding: 0.5rem 1rem;
    border-radius: 4px;
    transition: all 0.3s;
    font-weight: 500;
  }
  
  .nav-links a:hover {
    background: rgba(255,255,255,0.2);
  }
  
  .nav-links a.active {
    background: rgba(255,255,255,0.3);
    font-weight: 600;
  }
  
  .nav-links a.external-link {
    background: rgba(255,255,255,0.15);
    border: 1px solid rgba(255,255,255,0.3);
  }
  
  .nav-links a.external-link:hover {
    background: rgba(255,255,255,0.3);
  }
  
  .content {
    padding: 0;
  }
  
  .not-found {
    text-align: center;
    padding: 4rem 2rem;
  }
  
  .not-found h2 {
    color: #333;
    font-size: 2rem;
    margin-bottom: 1rem;
  }
  
  .not-found a {
    color: #2196F3;
    text-decoration: none;
    font-weight: 600;
  }
  
  .not-found a:hover {
    text-decoration: underline;
  }
</style>
