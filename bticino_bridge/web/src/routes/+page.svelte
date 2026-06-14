<script>
  import { onMount, onDestroy } from 'svelte'
  
  let status = null
  let loading = true
  let error = null
  let eventSource = null
  
  onMount(async () => {
    await loadStatus()
    
    // Connect to SSE for real-time LED/GPIO updates
    if (typeof EventSource !== 'undefined') {
      eventSource = new EventSource('/api/events')
      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data)
          if (data.type === 'leds' && data.leds) {
            status.leds = data.leds
          } else if (data.type === 'gpio' && data.gpio) {
            status.gpio = data.gpio
          }
        } catch (e) {
          console.error('SSE parse error:', e)
        }
      }
      eventSource.onerror = () => {
        console.log('SSE connection error, falling back to polling')
        setInterval(loadStatus, 30000)
      }
    } else {
      setInterval(loadStatus, 30000)
    }
  })
  
  onDestroy(() => {
    if (eventSource) {
      eventSource.close()
    }
  })
  
  async function loadStatus() {
    try {
      const response = await fetch('/api/status')
      if (!response.ok) throw new Error('Failed to fetch status')
      status = await response.json()
      error = null
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
</script>

<div class="dashboard">
  <div class="container">
    {#if loading}
      <div class="loading">Loading...</div>
    {:else if error}
      <div class="error">Error: {error}</div>
    {:else if status}
      <h2>System Status</h2>
      
      <div class="status-grid">
        <div class="status-card">
          <h3>📊 Version</h3>
          <p class="value">{status.version}</p>
        </div>
        
        <div class="status-card">
          <h3>⏱️ Uptime</h3>
          <p class="value">{status.uptime}</p>
        </div>
        
        <div class="status-card">
          <h3>💾 Storage</h3>
          <p class="value">{status.storage_used}</p>
        </div>
        
        {#if status.mqtt}
        <div class="status-card">
          <h3>📡 MQTT</h3>
          <p class="value">
            {#if status.mqtt.connected}
              ✅ {status.mqtt.broker}
            {:else}
              ❌ Disconnected
            {/if}
          </p>
        </div>
        {/if}
      </div>
      
      {#if status.components}
      <h3>Components</h3>
      <div class="components">
        {#each Object.entries(status.components) as [name, info]}
        <div class="component">
          <strong>{name}</strong>
          <span class="status">{info.status}</span>
        </div>
        {/each}
      </div>
      {/if}
      
      {#if status.leds}
      <h3>LEDs Status</h3>
      <div class="leds">
        {#each Object.entries(status.leds) as [name, on]}
        <div class="led {on ? 'on' : 'off'}">
          <span class="led-name">{name}</span>
          <span class="led-status">{on ? '✅ On' : '❌ Off'}</span>
        </div>
        {/each}
      </div>
      {/if}
      
      {#if status.gpio}
      <h3>GPIO Status</h3>
      <div class="gpios">
        {#each Object.entries(status.gpio) as [pin, on]}
        <div class="gpio {on ? 'on' : 'off'}">
          <span class="gpio-pin">GPIO {pin}</span>
          <span class="gpio-status">{on ? '✅ High' : '❌ Low'}</span>
        </div>
        {/each}
      </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .dashboard { padding: 2rem; }
  .container { max-width: 1200px; margin: 0 auto; }
  h2 { color: #333; margin-bottom: 1.5rem; }
  h3 { color: #666; margin-top: 2rem; margin-bottom: 1rem; }
  
  .status-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: 1.5rem;
    margin-bottom: 2rem;
  }
  
  .status-card {
    background: white;
    padding: 1.5rem;
    border-radius: 12px;
    box-shadow: 0 4px 6px rgba(0,0,0,0.1);
    transition: transform 0.3s, box-shadow 0.3s;
  }
  
  .status-card:hover {
    transform: translateY(-4px);
    box-shadow: 0 8px 12px rgba(0,0,0,0.15);
  }
  
  .status-card h3 {
    margin: 0 0 1rem 0;
    color: #666;
    font-size: 0.9rem;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  
  .value {
    font-size: 1.5rem;
    font-weight: bold;
    color: #2196F3;
    margin: 0;
  }
  
  .components {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
  }
  
  .component {
    background: white;
    padding: 1rem;
    border-radius: 8px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
  }
  
  .status {
    color: #4CAF50;
    font-weight: bold;
    padding: 0.25rem 0.75rem;
    background: #E8F5E9;
    border-radius: 12px;
    font-size: 0.85rem;
  }
  
  .leds {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 1rem;
  }
  
  .led {
    background: white;
    padding: 1rem;
    border-radius: 8px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
  }
  
  .led.on .led-status { color: #4CAF50; }
  .led.off .led-status { color: #999; }
  .led-name { color: #666; font-size: 0.9rem; }
  
  .gpios {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 0.75rem;
  }
  
  .gpio {
    background: white;
    padding: 0.75rem;
    border-radius: 6px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
  }
  
  .gpio.on .gpio-status { color: #4CAF50; }
  .gpio.off .gpio-status { color: #999; }
  .gpio-pin { color: #666; font-size: 0.85rem; }
  
  .loading, .error {
    text-align: center;
    padding: 3rem;
    font-size: 1.2rem;
  }
  
  .error {
    color: #f44336;
    background: #FFEBEE;
    border-radius: 8px;
    border: 1px solid #f44336;
  }
</style>
