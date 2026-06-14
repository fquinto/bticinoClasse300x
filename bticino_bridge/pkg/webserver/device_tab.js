// BTicino Bridge - Device Tab Enhancement
// Agrega soporte para NTP, Timezone, Language, Ringtone

// Leer configuración del dispositivo QML
async function loadDeviceConfig() {
    try {
        // Leer NTP desde global.guiSettings.ntp
        const ntpResponse = await fetch('/api/device/ntp');
        const ntpData = await ntpResponse.json();
        setValue('ntp_enabled', ntpData.enabled || false);
        setValue('ntp_server', ntpData.server || 'pool.ntp.org');
        
        // Leer Timezone
        const tzResponse = await fetch('/api/device/timezone');
        const tzData = await tzResponse.json();
        setValue('timezone', tzData.timezone || 'UTC');
        setValue('gmt_offset', tzData.gmt_offset || 0);
        
        // Leer idioma
        const langResponse = await fetch('/api/device/language');
        const langData = await langResponse.json();
        setValue('language', langData.language || 'en');
        
        // Leer timbre actual
        const ringtoneResponse = await fetch('/api/device/ringtone');
        const ringtoneData = await ringtoneResponse.json();
        setValue('ringtone', ringtoneData.ringtone || 'default');
        
        console.log('Device config loaded');
    } catch (error) {
        console.error('Error loading device config:', error);
    }
}

// Guardar configuración del dispositivo
async function saveDeviceConfig() {
    try {
        const config = {
            ntp: {
                enabled: getToggle('ntp_enabled'),
                server: getValue('ntp_server')
            },
            timezone: {
                timezone: getValue('timezone'),
                gmt_offset: parseInt(getValue('gmt_offset')) || 0
            },
            language: getValue('language'),
            ringtone: getValue('ringtone')
        };
        
        const response = await fetch('/api/device/save', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ config: config })
        });
        
        const result = await response.json();
        if (result.success) {
            showToast('Device configuration saved', 'success');
        } else {
            showToast('Error saving device config: ' + result.error, 'error');
        }
    } catch (error) {
        showToast('Error saving device config: ' + error.message, 'error');
    }
}

// Listar timbres disponibles
async function loadRingtones() {
    try {
        const response = await fetch('/api/device/ringtones');
        const data = await response.json();
        
        const select = document.getElementById('ringtone');
        if (select && data.ringtones) {
            select.innerHTML = '';
            data.ringtones.forEach(ringtone => {
                const option = document.createElement('option');
                option.value = ringtone;
                option.textContent = ringtone;
                select.appendChild(option);
            });
        }
    } catch (error) {
        console.error('Error loading ringtones:', error);
    }
}

// Listar idiomas disponibles
async function loadLanguages() {
    try {
        const response = await fetch('/api/device/languages');
        const data = await response.json();
        
        const select = document.getElementById('language');
        if (select && data.languages) {
            select.innerHTML = '';
            data.languages.forEach(lang => {
                const option = document.createElement('option');
                option.value = lang.code;
                option.textContent = lang.name;
                select.appendChild(option);
            });
        }
    } catch (error) {
        console.error('Error loading languages:', error);
    }
}

// Integrar con loadConfig existente
const originalLoadConfig = window.loadConfig;
window.loadConfig = async function() {
    if (originalLoadConfig) {
        await originalLoadConfig();
    }
    // Cargar configuración del dispositivo después de la config principal
    await loadDeviceConfig();
    await loadRingtones();
    await loadLanguages();
};

// Integrar con saveConfig existente
const originalSaveConfig = window.saveConfig;
window.saveConfig = async function() {
    // Guardar configuración del dispositivo junto con la principal
    await saveDeviceConfig();
    if (originalSaveConfig) {
        await originalSaveConfig();
    }
};
