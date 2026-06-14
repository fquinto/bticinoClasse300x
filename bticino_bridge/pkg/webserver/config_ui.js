// BTicino Bridge - Configuration Management UI
// v0.13.0 - Web-based configuration management

document.addEventListener('DOMContentLoaded', function() {
    console.log('Configuration Management UI loaded');
    initConfigManager();
});

// Global state
let currentConfig = null;
let configBackup = null;
let unsavedChanges = false;

// Initialize configuration manager
function initConfigManager() {
    loadConfig();
    setupEventListeners();
    setupAutoSave();
    // Pre-load device config for faster tab switching
    loadDeviceConfig();
}

// Setup event listeners
function setupEventListeners() {
    // Save button
    const saveBtn = document.getElementById('config-save-btn');
    if (saveBtn) {
        saveBtn.addEventListener('click', saveConfig);
    }

    // Validate button
    const validateBtn = document.getElementById('config-validate-btn');
    if (validateBtn) {
        validateBtn.addEventListener('click', validateConfig);
    }

    // Backup button
    const backupBtn = document.getElementById('config-backup-btn');
    if (backupBtn) {
        backupBtn.addEventListener('click', createBackup);
    }

    // Restore button
    const restoreBtn = document.getElementById('config-restore-btn');
    if (restoreBtn) {
        restoreBtn.addEventListener('click', restoreBackup);
    }

    // Cancel button
    const cancelBtn = document.getElementById('config-cancel-btn');
    if (cancelBtn) {
        cancelBtn.addEventListener('click', cancelChanges);
    }

    // Reload button
    const reloadBtn = document.getElementById('config-reload-btn');
    if (reloadBtn) {
        reloadBtn.addEventListener('click', reloadConfig);
    }

    // Form change detection
    const configForm = document.getElementById('config-form');
    if (configForm) {
        configForm.addEventListener('change', markAsChanged);
        configForm.addEventListener('input', markAsChanged);
    }

    // Tab switching
    const tabLinks = document.querySelectorAll('.config-tab-link');
    tabLinks.forEach(link => {
        link.addEventListener('click', function(e) {
            e.preventDefault();
            const tabId = this.getAttribute('data-tab');
            switchTab(tabId);
        });
    });

    // Prevent navigation with unsaved changes
    window.addEventListener('beforeunload', function(e) {
        if (unsavedChanges) {
            e.preventDefault();
            e.returnValue = '';
            return '';
        }
    });
}

// Load configuration from server
function loadConfig() {
    showLoading(true);
    
    fetch('/api/config')
        .then(response => response.json())
        .then(data => {
            currentConfig = data.config;
            configBackup = JSON.parse(JSON.stringify(data.config)); // Deep copy
            renderConfigForm(data.config);
            showLoading(false);
            console.log('Configuration loaded successfully');
        })
        .catch(error => {
            console.error('Error loading config:', error);
            showError('Failed to load configuration: ' + error.message);
            showLoading(false);
        });
}

// Render configuration form
function renderConfigForm(config) {
    // Bridge section
    setValue('bridge_name', config.bridge?.name);
    setValue('bridge_log_level', config.bridge?.log_level);
    
    // OpenWebNet section
    setValue('openwebnet_host', config.openwebnet?.host);
    setValue('openwebnet_port', config.openwebnet?.port);
    setValue('openwebnet_timeout', config.openwebnet?.timeout);
    
    // SIP section
    setValue('sip_enabled', config.sip?.enabled);
    setValue('sip_server_host', config.sip?.server_host);
    setValue('sip_server_port', config.sip?.server_port);
    setValue('sip_transport', config.sip?.transport);
    setValue('sip_domain', config.sip?.domain);
    setValue('sip_username', config.sip?.username);
    setValue('sip_password', config.sip?.password);
    setValue('sip_dev_addr', config.sip?.dev_addr);
    setValue('sip_use_ha1', config.sip?.use_ha1);
    
    // MQTT section
    setValue('mqtt_enabled', config.mqtt?.enabled);
    setValue('mqtt_host', config.mqtt?.host);
    setValue('mqtt_port', config.mqtt?.port);
    setValue('mqtt_username', config.mqtt?.username);
    setValue('mqtt_password', config.mqtt?.password);
    setValue('mqtt_topic_prefix', config.mqtt?.topic_prefix);
    
    // Web section
    setValue('web_enabled', config.web?.enabled);
    setValue('web_port', config.web?.port);
    
    // HomeKit section
    setValue('homekit_enabled', config.homekit?.enabled);
    setValue('homekit_name', config.homekit?.name);
    setValue('homekit_pin', config.homekit?.pin);
    
    // Streaming section
    setValue('streaming_enabled', config.streaming?.enabled);
    setValue('streaming_rtsp_port', config.streaming?.rtsp_port);
    setValue('streaming_recording_path', config.streaming?.recording_path);
    setValue('streaming_max_duration', config.streaming?.max_duration);
    
    // Network section (if exists)
    if (config.network) {
        setValue('network_ntp_enabled', config.network.ntp?.enabled);
        setValue('network_ntp_server', config.network.ntp?.server);
        setValue('network_firewall_block_telemetry', config.network.firewall?.block_telemetry);
        setValue('network_firewall_block_cloud', config.network.firewall?.block_cloud);
    }
    
    // Servers section (if exists)
    if (config.servers) {
        setValue('servers_cloud_enabled', config.servers.cloud?.enabled);
        setValue('servers_logging_enabled', config.servers.logging?.enabled);
    }
    
    // Privacy section (if exists)
    if (config.privacy) {
        setValue('privacy_block_telemetry', config.privacy.block_external_telemetry);
        setValue('privacy_block_cloud', config.privacy.block_cloud);
    }
    
    console.log('Configuration form rendered');
}

// Save configuration to server
function saveConfig() {
    if (!validateForm()) {
        showError('Please fix validation errors before saving');
        return;
    }
    
    showLoading(true);
    
    const config = collectFormData();
    
    fetch('/api/config/save', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            config: config,
            user: 'admin'
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showSuccess('Configuration saved successfully!' + (data.warnings?.length ? ' Warnings: ' + data.warnings.join(', ') : ''));
            currentConfig = config;
            configBackup = JSON.parse(JSON.stringify(config));
            unsavedChanges = false;
            
            // Show restart notification if needed
            if (data.restart_required) {
                showWarning('Configuration saved. A restart may be required for changes to take effect.');
            }
        } else {
            showError('Failed to save configuration: ' + data.error);
        }
        showLoading(false);
    })
    .catch(error => {
        console.error('Error saving config:', error);
        showError('Failed to save configuration: ' + error.message);
        showLoading(false);
    });
}

// Validate configuration on server
function validateConfig() {
    showLoading(true);
    
    const config = collectFormData();
    
    fetch('/api/config/validate', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ config: config })
    })
    .then(response => response.json())
    .then(data => {
        if (data.valid) {
            showSuccess('Configuration is valid!' + (data.warnings?.length ? ' Warnings: ' + data.warnings.join(', ') : ''));
        } else {
            showError('Configuration validation failed: ' + data.errors.join(', '));
        }
        showLoading(false);
    })
    .catch(error => {
        console.error('Error validating config:', error);
        showError('Failed to validate configuration: ' + error.message);
        showLoading(false);
    });
}

// Create backup
function createBackup() {
    if (!confirm('Create a backup of the current configuration?')) {
        return;
    }
    
    showLoading(true);
    
    fetch('/api/config/backup', { method: 'POST' })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showSuccess('Backup created: ' + data.backup_file);
        } else {
            showError('Failed to create backup: ' + data.error);
        }
        showLoading(false);
    })
    .catch(error => {
        console.error('Error creating backup:', error);
        showError('Failed to create backup: ' + error.message);
        showLoading(false);
    });
}

// List available backups
function listBackups() {
    fetch('/api/config/backups')
    .then(response => response.json())
    .then(data => {
        const backupList = document.getElementById('backup-list');
        if (backupList && data.backups?.length) {
            backupList.innerHTML = data.backups.map(file => 
                `<div class="backup-item">
                    <span>${file}</span>
                    <button onclick="restoreBackup('${file}')">Restore</button>
                </div>`
            ).join('');
        }
    })
    .catch(error => console.error('Error listing backups:', error));
}

// Restore from backup
function restoreBackup(backupFile) {
    if (!confirm('Restore configuration from backup? This will overwrite current settings.')) {
        return;
    }
    
    showLoading(true);
    
    fetch('/api/config/restore', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ backup_file: backupFile })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showSuccess('Backup restored successfully! Restarting service...');
            loadConfig();
        } else {
            showError('Failed to restore backup: ' + data.error);
        }
        showLoading(false);
    })
    .catch(error => {
        console.error('Error restoring backup:', error);
        showError('Failed to restore backup: ' + error.message);
        showLoading(false);
    });
}

// Cancel changes
function cancelChanges() {
    if (unsavedChanges && !confirm('Discard unsaved changes?')) {
        return;
    }
    
    renderConfigForm(currentConfig);
    unsavedChanges = false;
    showInfo('Changes discarded');
}

// Reload configuration from server
function reloadConfig() {
    if (unsavedChanges && !confirm('Reload will discard unsaved changes. Continue?')) {
        return;
    }
    
    loadConfig();
    showInfo('Configuration reloaded');
}

// Collect form data
function collectFormData() {
    const config = {
        bridge: {
            name: getValue('bridge_name'),
            log_level: getValue('bridge_log_level')
        },
        openwebnet: {
            host: getValue('openwebnet_host'),
            port: getIntValue('openwebnet_port'),
            timeout: getValue('openwebnet_timeout')
        },
        sip: {
            enabled: getBoolValue('sip_enabled'),
            server_host: getValue('sip_server_host'),
            server_port: getIntValue('sip_server_port'),
            transport: getValue('sip_transport'),
            domain: getValue('sip_domain'),
            username: getValue('sip_username'),
            password: getValue('sip_password'),
            dev_addr: getValue('sip_dev_addr'),
            use_ha1: getBoolValue('sip_use_ha1')
        },
        mqtt: {
            enabled: getBoolValue('mqtt_enabled'),
            host: getValue('mqtt_host'),
            port: getIntValue('mqtt_port'),
            username: getValue('mqtt_username'),
            password: getValue('mqtt_password'),
            topic_prefix: getValue('mqtt_topic_prefix')
        },
        web: {
            enabled: getBoolValue('web_enabled'),
            port: getIntValue('web_port')
        },
        homekit: {
            enabled: getBoolValue('homekit_enabled'),
            name: getValue('homekit_name'),
            pin: getValue('homekit_pin')
        },
        streaming: {
            enabled: getBoolValue('streaming_enabled'),
            rtsp_port: getIntValue('streaming_rtsp_port'),
            recording_path: getValue('streaming_recording_path'),
            max_duration: getIntValue('streaming_max_duration')
        }
    };
    
    // Add network section if fields exist
    if (document.getElementById('network_ntp_enabled')) {
        config.network = {
            ntp: {
                enabled: getBoolValue('network_ntp_enabled'),
                server: getValue('network_ntp_server')
            },
            firewall: {
                block_telemetry: getBoolValue('network_firewall_block_telemetry'),
                block_cloud: getBoolValue('network_firewall_block_cloud')
            }
        };
    }
    
    // Add servers section if fields exist
    if (document.getElementById('servers_cloud_enabled')) {
        config.servers = {
            cloud: {
                enabled: getBoolValue('servers_cloud_enabled')
            },
            logging: {
                enabled: getBoolValue('servers_logging_enabled')
            }
        };
    }
    
    // Add privacy section if fields exist
    if (document.getElementById('privacy_block_telemetry')) {
        config.privacy = {
            block_external_telemetry: getBoolValue('privacy_block_telemetry'),
            block_cloud: getBoolValue('privacy_block_cloud')
        };
    }
    
    return config;
}

// Validate form
function validateForm() {
    let valid = true;
    const errors = [];
    
    // Validate ports (1-65535)
    const ports = ['openwebnet_port', 'sip_server_port', 'mqtt_port', 'web_port', 'streaming_rtsp_port'];
    ports.forEach(portId => {
        const value = getIntValue(portId);
        if (value && (value < 1 || value > 65535)) {
            errors.push(`${portId} must be between 1-65535`);
            valid = false;
        }
    });
    
    // Validate required fields
    const required = ['bridge_name', 'sip_domain', 'sip_username'];
    required.forEach(fieldId => {
        const value = getValue(fieldId);
        if (!value) {
            errors.push(`${fieldId} is required`);
            valid = false;
        }
    });
    
    // Validate HomeKit PIN (8 digits)
    const pin = getValue('homekit_pin');
    if (pin && !/^\d{8}$/.test(pin)) {
        errors.push('HomeKit PIN must be 8 digits');
        valid = false;
    }
    
    if (!valid) {
        showError('Validation errors: ' + errors.join(', '));
    }
    
    return valid;
}

// Mark as changed
function markAsChanged() {
    unsavedChanges = true;
    const saveBtn = document.getElementById('config-save-btn');
    if (saveBtn) {
        saveBtn.textContent = 'Save*';
        saveBtn.classList.add('unsaved');
    }
}

// Switch tab
function switchTab(tabId) {
    // Hide all tab content
    document.querySelectorAll('.config-tab-content').forEach(tab => {
        tab.classList.remove('active');
    });
    
    // Remove active class from all links
    document.querySelectorAll('.config-tab-link').forEach(link => {
        link.classList.remove('active');
    });
    
    // Show selected tab
    const tabContent = document.getElementById(tabId + '-tab');
    if (tabContent) {
        tabContent.classList.add('active');
    }
    
    // Add active class to clicked link
    const activeLink = document.querySelector(`[data-tab="${tabId}"]`);
    if (activeLink) {
        activeLink.classList.add('active');
    }
    
    // Auto-load device config when switching to specific tabs
    switch(tabId) {
        case 'device':
            loadDeviceConfig();
            break;
        case 'audio':
            loadRingtones();
            loadVolumes();
            break;
        case 'display':
            loadDisplayConfig();
            loadCameras();
            break;
    }
}

// Helper functions
function getValue(id) {
    const element = document.getElementById(id);
    if (!element) return null;
    
    if (element.type === 'checkbox') {
        return element.checked;
    }
    
    return element.value;
}

function setValue(id, value) {
    const element = document.getElementById(id);
    if (!element || value === undefined) return;
    
    if (element.type === 'checkbox') {
        element.checked = value;
    } else {
        element.value = value;
    }
}

function getIntValue(id) {
    const value = getValue(id);
    return value ? parseInt(value) : null;
}

function getBoolValue(id) {
    return getValue(id) === true;
}

function showLoading(show) {
    const overlay = document.getElementById('loading-overlay');
    if (overlay) {
        overlay.style.display = show ? 'flex' : 'none';
    }
}

function showMessage(message, type) {
    const messageEl = document.getElementById('config-message');
    if (messageEl) {
        messageEl.textContent = message;
        messageEl.className = `config-message ${type}`;
        messageEl.style.display = 'block';
        
        setTimeout(() => {
            messageEl.style.display = 'none';
        }, 5000);
    }
}

function showError(message) {
    showMessage(message, 'error');
}

function showSuccess(message) {
    showMessage(message, 'success');
}

function showWarning(message) {
    showMessage(message, 'warning');
}

function showInfo(message) {
    showMessage(message, 'info');
}

// Auto-save setup (optional)
function setupAutoSave() {
    // Auto-save every 5 minutes if there are changes
    setInterval(() => {
        if (unsavedChanges && confirm('You have unsaved changes. Save now?')) {
            saveConfig();
        }
    }, 300000); // 5 minutes
}

// ==================== DEVICE CONFIG FUNCTIONS (Fase 2) ====================

// Load all device configuration
async function loadDeviceConfig() {
    try {
        const response = await fetch('/api/config/device');
        const data = await response.json();
        
        if (!data.success) {
            showError('Failed to load device config: ' + data.error);
            return;
        }
        
        const config = data.config;
        
        // Show content, hide loading
        document.getElementById('device-info-loading').style.display = 'none';
        document.getElementById('device-info-content').style.display = 'block';
        
        // Fill Device System info
        document.getElementById('device_language').value = config.language || 'N/A';
        document.getElementById('device_timezone').value = config.timezone || 'N/A';
        document.getElementById('device_ntp_server').value = config.ntp_server || 'N/A';
        document.getElementById('device_ntp_algo').value = config.ntp_algo || 'N/A';
        document.getElementById('device_model').value = config.device_info?.model || 'Classe 300 X13E';
        document.getElementById('device_ip').value = config.device_info?.ip || 'N/A';
        document.getElementById('device_firmware').value = config.device_info?.version || 'N/A';
        
        // Fill Date/Time
        const now = new Date(config.datetime || Date.now());
        document.getElementById('current-datetime').textContent = now.toLocaleString();
        
        // Fill Answering Machine info
        const answeringContainer = document.getElementById('answering-info-content');
        answeringContainer.innerHTML = '';
        if (config.answering) {
            const answeringFields = [
                { label: 'Activated', value: config.answering.activated ? 'Yes' : 'No' },
                { label: 'Ring Enabled', value: config.answering.ring_enabled ? 'Yes' : 'No' },
                { label: 'LED Enabled', value: config.answering.led_enable ? 'Yes' : 'No' },
                { label: 'Memory Used', value: config.answering.memory_used || 'N/A' }
            ];
            answeringFields.forEach(field => {
                const div = document.createElement('div');
                div.className = 'config-field';
                div.innerHTML = `<label>${field.label}</label><input type="text" value="${field.value}" readonly>`;
                answeringContainer.appendChild(div);
            });
        }
        
        showInfo('Device configuration loaded');
        
    } catch (error) {
        console.error('Error loading device config:', error);
        showError('Error loading device configuration');
    }
}

// Load ringtones configuration
async function loadRingtones() {
    try {
        const response = await fetch('/api/config/ringtones');
        const data = await response.json();
        
        if (!data.success) {
            showError('Failed to load ringtones: ' + data.error);
            return;
        }
        
        const container = document.getElementById('ringtones-content');
        container.innerHTML = '';
        
        const ringtoneNames = {
            s0: 'Floor 0 (S0)',
            s1: 'Floor 1 (S1)',
            s2: 'Floor 2 (S2)',
            s3: 'Floor 3 (S3)',
            door: 'Door',
            external: 'External',
            alarm: 'Alarm',
            message: 'Message'
        };
        
        const ringtones = data.ringtones;
        for (const [key, value] of Object.entries(ringtones)) {
            const fieldDiv = document.createElement('div');
            fieldDiv.className = 'config-field';
            fieldDiv.innerHTML = `
                <label>${ringtoneNames[key] || key}</label>
                <input type="number" id="ringtone_${key}" value="${value || 0}" readonly min="1" max="15">
            `;
            container.appendChild(fieldDiv);
        }
        
        showInfo('Ringtones loaded');
        
    } catch (error) {
        console.error('Error loading ringtones:', error);
        showError('Error loading ringtones');
    }
}

// Load volumes configuration
async function loadVolumes() {
    try {
        const response = await fetch('/api/config/volumes');
        const data = await response.json();
        
        if (!data.success) {
            showError('Failed to load volumes: ' + data.error);
            return;
        }
        
        const container = document.getElementById('volumes-content');
        container.innerHTML = '';
        
        const volumeNames = {
            s0: 'Floor 0 (S0)',
            s1: 'Floor 1 (S1)',
            s2: 'Floor 2 (S2)',
            intercom: 'Intercom',
            door: 'Door',
            sip: 'SIP'
        };
        
        const volumes = data.volumes;
        for (const [key, value] of Object.entries(volumes)) {
            const fieldDiv = document.createElement('div');
            fieldDiv.className = 'config-field';
            fieldDiv.innerHTML = `
                <label>${volumeNames[key] || key}</label>
                <input type="number" id="volume_${key}" value="${value || 0}" readonly min="0" max="100">
            `;
            container.appendChild(fieldDiv);
        }
        
        showInfo('Volumes loaded');
        
    } catch (error) {
        console.error('Error loading volumes:', error);
        showError('Error loading volumes');
    }
}

// Load display configuration
async function loadDisplayConfig() {
    try {
        const response = await fetch('/api/config/display');
        const data = await response.json();
        
        if (!data.success) {
            showError('Failed to load display config: ' + data.error);
            return;
        }
        
        const container = document.getElementById('display-content');
        container.innerHTML = '';
        
        const display = data.display;
        
        const brightnessField = document.createElement('div');
        brightnessField.className = 'config-field';
        brightnessField.innerHTML = `
            <label>Brightness</label>
            <div style="display: flex; align-items: center; gap: 10px;">
                <input type="range" id="display_brightness" value="${display.brightness || 50}" min="0" max="100" style="flex: 1;">
                <span id="display_brightness_value" style="min-width: 40px;">${display.brightness || 50}%</span>
            </div>
        `;
        container.appendChild(brightnessField);
        
        const cleanTimeField = document.createElement('div');
        cleanTimeField.className = 'config-field';
        cleanTimeField.innerHTML = `
            <label>Clean Screen Time (ms)</label>
            <input type="number" id="display_clean_time" value="${display.clean_time || 10000}" readonly min="0">
        `;
        container.appendChild(cleanTimeField);
        
        const brightnessSlider = document.getElementById('display_brightness');
        if (brightnessSlider) {
            brightnessSlider.addEventListener('input', function() {
                document.getElementById('display_brightness_value').textContent = this.value + '%';
            });
        }
        
        showInfo('Display configuration loaded');
        
    } catch (error) {
        console.error('Error loading display config:', error);
        showError('Error loading display configuration');
    }
}

// Load cameras configuration
async function loadCameras() {
    try {
        const response = await fetch('/api/config/cameras');
        const data = await response.json();
        
        if (!data.success) {
            showError('Failed to load cameras: ' + data.error);
            return;
        }
        
        const container = document.getElementById('cameras-content');
        container.innerHTML = '';
        
        const cameras = data.cameras;
        for (const [key, config] of Object.entries(cameras)) {
            const cameraSection = document.createElement('div');
            cameraSection.className = 'config-section';
            cameraSection.innerHTML = `
                <h4 style="margin: 10px 0;">${key}</h4>
                <div class="config-grid">
                    <div class="config-field">
                        <label>Brightness</label>
                        <input type="number" value="${config.brightness || 0}" readonly min="0" max="100">
                    </div>
                    <div class="config-field">
                        <label>Contrast</label>
                        <input type="number" value="${config.contrast || 0}" readonly min="0" max="100">
                    </div>
                    <div class="config-field">
                        <label>Saturation</label>
                        <input type="number" value="${config.saturation || 0}" readonly min="0" max="100">
                    </div>
                    <div class="config-field">
                        <label>Quality</label>
                        <input type="number" value="${config.quality || 0}" readonly min="0" max="100">
                    </div>
                </div>
            `;
            container.appendChild(cameraSection);
        }
        
        showInfo('Cameras loaded');
        
    } catch (error) {
        console.error('Error loading cameras:', error);
        showError('Error loading cameras');
    }
}
