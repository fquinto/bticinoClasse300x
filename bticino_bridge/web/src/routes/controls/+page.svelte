<script>
  import { onMount } from 'svelte'
  
  let controls = {
    door: { locked: true, loading: false },
    voicemail: { enabled: false, loading: false },
    display: { on: true, loading: false },
    mute: { on: false, loading: false },
    doorbell_sound: { on: true, loading: false },
    light: { on: false, loading: false }
  }
  
  let loading = true
  let error = null
  let success = null
  
  let mqttStatus = 'Loading...'
  let componentsStatus = 'Loading...'
  
  onMount(async () => {
    await loadControls()
    await updateStatus()
    setInterval(updateStatus, 10000)
  })
  
  async function loadControls() {
    try {
      loading = true
      
      const [deviceRes, statusRes] = await Promise.all([
        fetch('/api/config/device'),
        fetch('/api/status')
      ])
      
      const deviceData = await deviceRes.json()
      const statusData = await statusRes.json()
      
      if (deviceData.success && deviceData.config) {
        const answering = deviceData.config.answering || {}
        controls.voicemail.enabled = answering.activated === true || answering.activated === 1
        
        const volumes = deviceData.config.volumes || {}
        controls.mute.on = volumes.s0 === 0 && volumes.s1 === 0
      }
      
      if (statusData.leds) {
        controls.doorbell_sound.on = statusData.leds.led_ans_machine !== false
      }
      
      error = null
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
  
  async function updateStatus() {
    try {
      const res = await fetch('/api/status')
      const data = await res.json()
      
      mqttStatus = data.mqtt?.connected ? '✅ Connected' : '❌ Disconnected'
      
      const components = data.components || {}
      componentsStatus = Object.entries(components).map(([k, v]) => `${k}: ${v.status}`).join(', ') || 'N/A'
    } catch (e) {
      console.error('Failed to update status:', e)
    }
  }
  
  async function toggleVoicemail() {
    const action = controls.voicemail.enabled ? 'disable' : 'enable'
    try {
      controls.voicemail.loading = true
      const response = await fetch(`/api/controls/answering-machine/toggle?action=${action}`, { method: 'POST' })
      const result = await response.json()
      
      if (result.success) {
        controls.voicemail.enabled = !controls.voicemail.enabled
        success = `Voicemail ${action === 'enable' ? 'enabled' : 'disabled'}`
        setTimeout(() => success = null, 3000)
      } else {
        error = result.message || 'Failed'
      }
    } catch (e) {
      error = e.message
    } finally {
      controls.voicemail.loading = false
    }
  }
  
  async function toggleMute() {
    const action = controls.mute.on ? 'off' : 'on'
    try {
      controls.mute.loading = true
      const response = await fetch(`/api/controls/mute/${action}`, { method: 'POST' })
      const result = await response.json()
      
      if (result.success) {
        controls.mute.on = !controls.mute.on
        success = `Mute ${action === 'on' ? 'activated' : 'deactivated'}`
        setTimeout(() => success = null, 3000)
      } else {
        error = result.message || 'Failed'
      }
    } catch (e) {
      error = e.message
    } finally {
      controls.mute.loading = false
    }
  }
  
  async function toggleDoorbellSound() {
    const action = controls.doorbell_sound.on ? 'off' : 'on'
    try {
      controls.doorbell_sound.loading = true
      const response = await fetch(`/api/controls/doorbell/${action}`, { method: 'POST' })
      const result = await response.json()
      
      if (result.success) {
        controls.doorbell_sound.on = !controls.doorbell_sound.on
        success = `Doorbell sound ${action === 'on' ? 'enabled' : 'disabled'}`
        setTimeout(() => success = null, 3000)
      } else {
        error = result.message || 'Failed'
      }
    } catch (e) {
      error = e.message
    } finally {
      controls.doorbell_sound.loading = false
    }
  }
  
  async function toggleDisplay() {
    const action = controls.display.on ? 'off' : 'on'
    try {
      controls.display.loading = true
      const response = await fetch(`/api/controls/display/${action}`, { method: 'POST' })
      const result = await response.json()
      
      if (result.success) {
        controls.display.on = !controls.display.on
        success = `Display ${action === 'on' ? 'turned on' : 'turned off'}`
        setTimeout(() => success = null, 3000)
      } else {
        error = result.message || 'Failed'
      }
    } catch (e) {
      error = e.message
    } finally {
      controls.display.loading = false
    }
  }
  
  async function unlockDoor() {
    try {
      controls.door.loading = true
      const response = await fetch('/api/controls/door/unlock', { method: 'POST' })
      const result = await response.json()
      
      if (result.success) {
        success = '🚪 Door unlocked!'
        setTimeout(() => success = null, 3000)
      } else {
        error = result.message || 'Failed'
      }
    } catch (e) {
      error = e.message
    } finally {
      controls.door.loading = false
    }
  }
  
  async function toggleLight() {
    try {
      controls.light.loading = true
      const response = await fetch('/api/controls/light/on', { method: 'POST' })
      const result = await response.json()
      
      if (result.success) {
        success = '💡 Staircase light activated'
        setTimeout(() => success = null, 3000)
      } else {
        error = result.message || 'Failed'
      }
    } catch (e) {
      error = e.message
    } finally {
      controls.light.loading = false
    }
  }
</script>

<div class="controls-page">
  <div class="page-header">
    <h1>🎮 Controls</h1>
    <button class="refresh-btn" on:click={loadControls}>🔄</button>
  </div>

  <div class="container">
    {#if loading}
      <div class="loading">Loading controls...</div>
    {:else if error}
      <div class="error">Error: {error}</div>
    {:else}
      {#if success}
      <div class="success">✅ {success}</div>
      {/if}
      
      <h2>Quick Controls</h2>
      
      <div class="controls-grid">
        <div class="control-card">
          <div class="control-icon">🚪</div>
          <h3>Door Lock</h3>
          <p class="status">🔒 Locked</p>
          <button class="btn btn-primary" on:click={unlockDoor} disabled={controls.door.loading}>
            {controls.door.loading ? '⏳ Opening...' : '🔓 Unlock Door'}
          </button>
        </div>

        <div class="control-card">
          <div class="control-icon">📼</div>
          <h3>Voicemail</h3>
          <p class="status">{controls.voicemail.enabled ? '✅ Enabled' : '❌ Disabled'}</p>
          <button class="btn btn-secondary" on:click={toggleVoicemail} disabled={controls.voicemail.loading}>
            {controls.voicemail.loading ? '⏳...' : (controls.voicemail.enabled ? 'Disable' : 'Enable')}
          </button>
        </div>

        <div class="control-card">
          <div class="control-icon">📱</div>
          <h3>Display</h3>
          <p class="status">{controls.display.on ? '✅ On' : '⬛ Off'}</p>
          <button class="btn btn-secondary" on:click={toggleDisplay} disabled={controls.display.loading}>
            {controls.display.loading ? '⏳...' : (controls.display.on ? 'Turn Off' : 'Turn On')}
          </button>
        </div>

        <!-- Mute eliminado - integrado en doorbell_sound -->
        <!--
        <div class="control-card">
          <div class="control-icon">🔇</div>
          <h3>Mute</h3>
          <p class="status">{controls.mute.on ? '🔇 Muted' : '🔊 Unmuted'}</p>
          <button class="btn btn-secondary" on:click={toggleMute} disabled={controls.mute.loading}>
            {controls.mute.loading ? '⏳...' : (controls.mute.on ? 'Unmute' : 'Mute')}
          </button>
        </div>
        -->

        <div class="control-card">
          <div class="control-icon">🔔</div>
          <h3>Doorbell Sound</h3>
          <p class="status">{controls.doorbell_sound.on ? '🔔 Enabled' : '🔕 Disabled'}</p>
          <button class="btn btn-secondary" on:click={toggleDoorbellSound} disabled={controls.doorbell_sound.loading}>
            {controls.doorbell_sound.loading ? '⏳...' : (controls.doorbell_sound.on ? 'Disable' : 'Enable')}
          </button>
        </div>

        <div class="control-card">
          <div class="control-icon">💡</div>
          <h3>Staircase Light</h3>
          <p class="status">{controls.light.on ? '💡 On' : '⚫ Off'}</p>
          <button class="btn btn-primary" on:click={toggleLight} disabled={controls.light.loading}>
            {controls.light.loading ? '⏳...' : 'Turn On'}
          </button>
        </div>
      </div>

      <h2>Device Status</h2>
      <div class="status-section">
        <div class="status-item">
          <span class="label">MQTT</span>
          <span class="value">{mqttStatus}</span>
        </div>
        <div class="status-item">
          <span class="label">Components</span>
          <span class="value">{componentsStatus}</span>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .controls-page { min-height: 100vh; }
  
  .page-header {
    background: linear-gradient(135deg, #2196F3, #1976D2);
    color: white;
    padding: 1rem 2rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  
  .page-header h1 { margin: 0; font-size: 1.5rem; }
  
  .refresh-btn {
    background: rgba(255,255,255,0.2);
    border: none;
    color: white;
    padding: 0.5rem 1rem;
    border-radius: 4px;
    cursor: pointer;
    font-size: 1.2rem;
  }
  
  .container { max-width: 1200px; margin: 0 auto; padding: 1rem 2rem 2rem; }
  
  h2 {
    color: #333;
    margin-bottom: 1rem;
    border-bottom: 2px solid #2196F3;
    padding-bottom: 0.5rem;
  }
  
  .controls-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 1.5rem;
    margin-bottom: 2rem;
  }
  
  .control-card {
    background: white;
    padding: 1.5rem;
    border-radius: 12px;
    box-shadow: 0 4px 6px rgba(0,0,0,0.1);
    text-align: center;
    transition: transform 0.3s;
  }
  
  .control-card:hover { transform: translateY(-4px); }
  .control-icon { font-size: 3rem; margin-bottom: 0.5rem; }
  .control-card h3 { margin: 0.5rem 0; color: #333; }
  .status { color: #666; margin-bottom: 1rem; font-weight: 600; }
  
  .btn {
    padding: 0.75rem 1.5rem;
    border: none;
    border-radius: 6px;
    font-size: 1rem;
    cursor: pointer;
    transition: all 0.3s;
    width: 100%;
  }
  
  .btn-primary { background: #2196F3; color: white; }
  .btn-primary:hover { background: #1976D2; }
  .btn-secondary { background: #4CAF50; color: white; }
  .btn-secondary:hover { background: #388E3C; }
  .btn:disabled { opacity: 0.6; cursor: not-allowed; }
  
  .status-section {
    background: white;
    padding: 1.5rem;
    border-radius: 12px;
    box-shadow: 0 4px 6px rgba(0,0,0,0.1);
  }
  
  .status-item {
    display: flex;
    justify-content: space-between;
    padding: 0.75rem 0;
    border-bottom: 1px solid #eee;
  }
  
  .status-item:last-child { border-bottom: none; }
  .label { color: #666; font-weight: 600; }
  .value { color: #2196F3; font-weight: bold; }
  
  .loading, .error, .success {
    text-align: center;
    padding: 2rem;
    font-size: 1.2rem;
    border-radius: 8px;
    margin-bottom: 1.5rem;
  }
  
  .error { background: #FFEBEE; color: #f44336; border: 1px solid #f44336; }
  .success { background: #E8F5E9; color: #2E7D32; border: 1px solid #4CAF50; }
</style>
