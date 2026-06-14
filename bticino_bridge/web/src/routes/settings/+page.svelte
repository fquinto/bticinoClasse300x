<script>
  import { onMount } from 'svelte'
  
  let config = null
  let loading = true
  let saving = false
  let error = null
  let success = null
  
  const tabs = [
    { id: 'bridge', label: '🌉 Bridge' },
    { id: 'network', label: '🌐 Network' },
    { id: 'sip', label: '📞 SIP' },
    { id: 'mqtt', label: '📡 MQTT' },
    { id: 'homekit', label: '🏠 HomeKit' },
    { id: 'streaming', label: '📹 Streaming' },
    { id: 'hardware', label: '🔧 Hardware' },
    { id: 'security', label: '🔒 Security' },
    { id: 'openwebnet', label: '🔌 OpenWebNet' }
  ]
  
  let activeTab = 'bridge'
  
  onMount(async () => {
    await loadConfig()
  })
  
  async function loadConfig() {
    try {
      loading = true
      const response = await fetch('/api/config')
      if (!response.ok) throw new Error('Failed to load config')
      config = await response.json()
      error = null
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
  
  async function saveConfig() {
    try {
      saving = true
      const response = await fetch('/api/config/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ config: config.config, user: 'admin' })
      })
      const result = await response.json()
      if (result.success) {
        success = '✅ Configuration saved! Restart required.'
        error = null
        setTimeout(() => success = null, 5000)
      } else {
        error = result.error || 'Failed to save'
        if (result.errors) error += '\n' + result.errors.join('\n')
      }
    } catch (e) {
      error = e.message
    } finally {
      saving = false
    }
  }
  
  function hasChanges() {
    return config && config.config
  }
</script>

<div class="settings-page">
  <div class="page-header">
    <h1>⚙️ Settings</h1>
    {#if success}
      <div class="success-banner">{success}</div>
    {/if}
  </div>

  <div class="container">
    {#if loading}
      <div class="loading">Loading configuration...</div>
    {:else if error && !config}
      <div class="error">Error: {error}</div>
    {:else if config}
      {#if error}
      <div class="error-banner">{error}</div>
      {/if}
      
      <div class="tabs">
        {#each tabs as tab}
        <button 
          class="tab {activeTab === tab.id ? 'active' : ''}"
          on:click={() => activeTab = tab.id}
        >
          {tab.label}
        </button>
        {/each}
      </div>

      <div class="tab-content">
        <!-- BRIDGE -->
        {#if activeTab === 'bridge'}
        <div class="section">
          <h2>🌉 Bridge Configuration</h2>
          <div class="form-grid">
            <div class="form-group">
              <label for="bridge_name">Bridge Name</label>
              <input type="text" id="bridge_name" bind:value={config.config.Bridge.Name} />
            </div>
            <div class="form-group">
              <label for="bridge_log_level">Log Level</label>
              <select id="bridge_log_level" bind:value={config.config.Bridge.LogLevel}>
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warning</option>
                <option value="error">Error</option>
              </select>
            </div>
            <div class="form-group">
              <label for="log_file">Log File</label>
              <input type="text" id="log_file" bind:value={config.config.Logging.File} />
            </div>
            <div class="form-group">
              <label for="log_size">Max Log Size</label>
              <input type="text" id="log_size" bind:value={config.config.Logging.MaxSize} />
            </div>
          </div>
        </div>
        {/if}

        <!-- NETWORK -->
        {#if activeTab === 'network'}
        <div class="section">
          <h2>🌐 Network Configuration</h2>
          <h3>NTP</h3>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Network.NTP.Enabled} />
                NTP Enabled
              </label>
            </div>
            <div class="form-group">
              <label for="ntp_server">NTP Server</label>
              <input type="text" id="ntp_server" bind:value={config.config.Network.NTP.Server} />
            </div>
          </div>
          <h3>DNS</h3>
          <div class="form-grid">
            <div class="form-group">
              <label for="dns_servers">DNS Servers (comma separated)</label>
              <input type="text" id="dns_servers" value={config.config.Network.DNS.Servers.join(', ')} 
                on:change={(e) => config.config.Network.DNS.Servers = e.target.value.split(',').map(s => s.trim())} />
            </div>
          </div>
          <h3>Firewall</h3>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Network.Firewall.BlockTelemetry} />
                Block Telemetry
              </label>
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Network.Firewall.BlockCloud} />
                Block Cloud
              </label>
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Network.Firewall.AllowUpdates} />
                Allow Updates
              </label>
            </div>
          </div>
        </div>
        {/if}

        <!-- SIP -->
        {#if activeTab === 'sip'}
        <div class="section">
          <h2>📞 SIP Configuration</h2>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.SIP.Enabled} />
                SIP Enabled
              </label>
            </div>
            <div class="form-group">
              <label for="sip_server">Server Host</label>
              <input type="text" id="sip_server" bind:value={config.config.SIP.ServerHost} />
            </div>
            <div class="form-group">
              <label for="sip_port">Server Port</label>
              <input type="number" id="sip_port" bind:value={config.config.SIP.ServerPort} />
            </div>
            <div class="form-group">
              <label for="sip_transport">Transport</label>
              <select id="sip_transport" bind:value={config.config.SIP.Transport}>
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
                <option value="tls">TLS</option>
              </select>
            </div>
            <div class="form-group">
              <label for="sip_domain">Domain</label>
              <input type="text" id="sip_domain" bind:value={config.config.SIP.Domain} />
            </div>
            <div class="form-group">
              <label for="sip_username">Username</label>
              <input type="text" id="sip_username" bind:value={config.config.SIP.Username} />
            </div>
            <div class="form-group">
              <label for="sip_password">Password</label>
              <input type="password" id="sip_password" bind:value={config.config.SIP.Password} />
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.SIP.InsecureTLS} />
                Insecure TLS
              </label>
            </div>
          </div>
        </div>
        {/if}

        <!-- MQTT -->
        {#if activeTab === 'mqtt'}
        <div class="section">
          <h2>📡 MQTT Configuration</h2>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.MQTT.Enabled} />
                MQTT Enabled
              </label>
            </div>
            <div class="form-group">
              <label for="mqtt_host">Broker Host</label>
              <input type="text" id="mqtt_host" bind:value={config.config.MQTT.Host} />
            </div>
            <div class="form-group">
              <label for="mqtt_port">Broker Port</label>
              <input type="number" id="mqtt_port" bind:value={config.config.MQTT.Port} />
            </div>
            <div class="form-group">
              <label for="mqtt_username">Username</label>
              <input type="text" id="mqtt_username" bind:value={config.config.MQTT.Username} />
            </div>
            <div class="form-group">
              <label for="mqtt_password">Password</label>
              <input type="password" id="mqtt_password" bind:value={config.config.MQTT.Password} />
            </div>
            <div class="form-group">
              <label for="mqtt_client_id">Client ID</label>
              <input type="text" id="mqtt_client_id" bind:value={config.config.MQTT.ClientID} />
            </div>
            <div class="form-group">
              <label for="mqtt_prefix">Topic Prefix</label>
              <input type="text" id="mqtt_prefix" bind:value={config.config.MQTT.TopicPrefix} />
            </div>
          </div>
        </div>
        {/if}

        <!-- HOMEKIT -->
        {#if activeTab === 'homekit'}
        <div class="section">
          <h2>🏠 HomeKit Configuration</h2>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.HomeKit.Enabled} />
                HomeKit Enabled
              </label>
            </div>
            <div class="form-group">
              <label for="hk_name">Bridge Name</label>
              <input type="text" id="hk_name" bind:value={config.config.HomeKit.Name} />
            </div>
            <div class="form-group">
              <label for="hk_port">Port</label>
              <input type="number" id="hk_port" bind:value={config.config.HomeKit.Port} />
            </div>
            <div class="form-group">
              <label for="hk_pin">PIN</label>
              <input type="text" id="hk_pin" bind:value={config.config.HomeKit.Pin} />
            </div>
            <div class="form-group">
              <label for="hk_model">Model</label>
              <input type="text" id="hk_model" bind:value={config.config.HomeKit.Model} />
            </div>
            <div class="form-group">
              <label for="hk_manufacturer">Manufacturer</label>
              <input type="text" id="hk_manufacturer" bind:value={config.config.HomeKit.Manufacturer} />
            </div>
          </div>
        </div>
        {/if}

        <!-- STREAMING -->
        {#if activeTab === 'streaming'}
        <div class="section">
          <h2>📹 Streaming Configuration</h2>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Streaming.Enabled} />
                Streaming Enabled
              </label>
            </div>
            <div class="form-group">
              <label for="rtsp_port">RTSP Port</label>
              <input type="number" id="rtsp_port" bind:value={config.config.Streaming.RTSPPort} />
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Streaming.WebRTCEnabled} />
                WebRTC Enabled
              </label>
            </div>
            <div class="form-group">
              <label for="webrtc_port">WebRTC Port</label>
              <input type="number" id="webrtc_port" bind:value={config.config.Streaming.WebRCTPPort} />
            </div>
            <div class="form-group">
              <label for="recording_path">Recording Path</label>
              <input type="text" id="recording_path" bind:value={config.config.Streaming.RecordingPath} />
            </div>
            <div class="form-group">
              <label for="max_duration">Max Duration (seconds)</label>
              <input type="number" id="max_duration" bind:value={config.config.Streaming.MaxDuration} />
            </div>
          </div>
        </div>
        {/if}

        <!-- HARDWARE -->
        {#if activeTab === 'hardware'}
        <div class="section">
          <h2>🔧 Hardware Configuration</h2>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Hardware.Enabled} />
                Hardware Monitoring Enabled
              </label>
            </div>
            <div class="form-group">
              <label for="input_device">Input Device</label>
              <input type="text" id="input_device" bind:value={config.config.Hardware.InputDevice} />
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Hardware.GPIOMonitoring} />
                GPIO Monitoring
              </label>
            </div>
            <div class="form-group">
              <label for="poll_interval">Polling Interval (ns)</label>
              <input type="number" id="poll_interval" bind:value={config.config.Hardware.PollingInterval} />
            </div>
          </div>
        </div>
        {/if}

        <!-- SECURITY -->
        {#if activeTab === 'security'}
        <div class="section">
          <h2>🔒 Security</h2>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Security.WebAuthRequired} />
                Web Auth Required
              </label>
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Security.WebHTTPSEnabled} />
                Web HTTPS Enabled
              </label>
            </div>
            <div class="form-group">
              <label for="rate_limit">API Rate Limit</label>
              <input type="number" id="rate_limit" bind:value={config.config.Security.APIRateLimit} />
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Security.ConfigAuditLog} />
                Config Audit Log
              </label>
            </div>
          </div>
          <h3>Privacy</h3>
          <div class="form-grid">
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Privacy.BlockExternalTelemetry} />
                Block External Telemetry
              </label>
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Privacy.BlockLogServer} />
                Block Log Server
              </label>
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Privacy.DisableAutoUpdates} />
                Disable Auto Updates
              </label>
            </div>
            <div class="form-group">
              <label>
                <input type="checkbox" bind:checked={config.config.Privacy.LocalLogging} />
                Local Logging Only
              </label>
            </div>
          </div>
        </div>
        {/if}

        <!-- OPENWEBNET -->
        {#if activeTab === 'openwebnet'}
        <div class="section">
          <h2>🔌 OpenWebNet Configuration</h2>
          <div class="form-grid">
            <div class="form-group">
              <label for="own_host">OpenWebNet Host</label>
              <input type="text" id="own_host" bind:value={config.config.OpenWebNet.Host} />
            </div>
            <div class="form-group">
              <label for="own_port">OpenWebNet Port</label>
              <input type="number" id="own_port" bind:value={config.config.OpenWebNet.Port} />
            </div>
            <div class="form-group">
              <label for="own_timeout">Timeout (ns)</label>
              <input type="number" id="own_timeout" bind:value={config.config.OpenWebNet.Timeout} />
            </div>
            <div class="form-group">
              <label for="retry_attempts">Retry Attempts</label>
              <input type="number" id="retry_attempts" bind:value={config.config.OpenWebNet.RetryAttempts} />
            </div>
            <div class="form-group">
              <label for="retry_delay">Retry Delay (ns)</label>
              <input type="number" id="retry_delay" bind:value={config.config.OpenWebNet.RetryDelay} />
            </div>
          </div>
        </div>
        {/if}
      </div>

      <div class="actions">
        <button class="btn btn-primary" on:click={saveConfig} disabled={saving}>
          {#if saving}⏳ Saving...{:else}💾 Save Configuration{/if}
        </button>
        <button class="btn btn-secondary" on:click={loadConfig}>🔄 Reload</button>
      </div>
    {/if}
  </div>
</div>

<style>
  :global(body) {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #f5f5f5;
  }
  
  .page-header {
    background: linear-gradient(135deg, #2196F3, #1976D2);
    color: white;
    padding: 1rem 2rem;
    position: relative;
  }
  
  .page-header h1 {
    margin: 0;
    font-size: 1.5rem;
  }
  
  .success-banner {
    position: absolute;
    right: 2rem;
    top: 50%;
    transform: translateY(-50%);
    background: #4CAF50;
    padding: 0.5rem 1rem;
    border-radius: 4px;
  }
  
  .container {
    max-width: 1000px;
    margin: 0 auto;
    padding: 1rem 2rem 2rem;
  }
  
  .tabs {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1rem;
    border-bottom: 2px solid #ddd;
    padding-bottom: 0.5rem;
    overflow-x: auto;
    flex-wrap: wrap;
  }
  
  .tab {
    padding: 0.5rem 1rem;
    border: none;
    background: #e0e0e0;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.9rem;
    white-space: nowrap;
  }
  
  .tab.active {
    background: #2196F3;
    color: white;
  }
  
  .tab-content {
    background: white;
    padding: 1.5rem;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
  }
  
  .section h2 {
    margin: 0 0 1rem 0;
    color: #333;
    border-bottom: 2px solid #2196F3;
    padding-bottom: 0.5rem;
  }
  
  .section h3 {
    margin: 1.5rem 0 1rem 0;
    color: #666;
    font-size: 1rem;
  }
  
  .form-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
    gap: 1rem;
  }
  
  .form-group {
    margin-bottom: 0.5rem;
  }
  
  .form-group label {
    display: block;
    margin-bottom: 0.25rem;
    font-weight: 600;
    color: #555;
    font-size: 0.9rem;
  }
  
  .form-group input[type="text"],
  .form-group input[type="number"],
  .form-group input[type="password"],
  .form-group select {
    width: 100%;
    padding: 0.5rem;
    border: 1px solid #ddd;
    border-radius: 4px;
    font-size: 0.95rem;
    box-sizing: border-box;
  }
  
  .form-group input[type="checkbox"] {
    width: auto;
    margin-right: 0.5rem;
  }
  
  .error-banner {
    background: #FFEBEE;
    color: #f44336;
    padding: 1rem;
    border-radius: 4px;
    margin-bottom: 1rem;
    white-space: pre-wrap;
  }
  
  .actions {
    margin-top: 1.5rem;
    display: flex;
    gap: 1rem;
  }
  
  .btn {
    padding: 0.75rem 2rem;
    border: none;
    border-radius: 4px;
    font-size: 1rem;
    cursor: pointer;
  }
  
  .btn-primary {
    background: #2196F3;
    color: white;
  }
  
  .btn-secondary {
    background: #e0e0e0;
    color: #333;
  }
  
  .btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .loading, .error {
    text-align: center;
    padding: 3rem;
    font-size: 1.2rem;
    border-radius: 8px;
  }
  
  .error {
    background: #FFEBEE;
    color: #f44336;
  }
</style>
