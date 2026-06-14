# 🏠 BTicino Classe 300X - Home Assistant Integration Guide
## Complete Installation and Usage Manual

---

## ✅ INTEGRATION STATUS: **FULLY OPERATIONAL**

**Last Tested**: December 19, 2025  
**Hardware**: BTicino Classe 300X (C3X-00-03-50-a8-a7-52-1754162)  
**Home Assistant**: 192.168.1.3:1883 (MQTT Broker)  
**Status**: ✅ Production Ready

---

## 🎯 What's Working

### **✅ Real-time Monitoring**
- **System Online Status**: Live connection status
- **SIP Connection**: VoIP phone system status 
- **SIP Codec**: Audio codec status
- **Audio Channels**: 7 audio channels (0-6) monitoring
- **Auto-Discovery**: Automatic device detection in Home Assistant
- **Bi-directional Commands**: Remote control from Home Assistant

### **✅ Home Assistant Entities Created**
| Entity Type | Name | Description |
|-------------|------|-------------|
| `binary_sensor.bticino_online` | BTicino Online | System connectivity |
| `sensor.bticino_sip_connection` | BTicino SIP Connection | VoIP status |
| `sensor.bticino_sip_codec` | BTicino SIP Codec | Audio codec |
| `sensor.bticino_last_update` | BTicino Last Update | Timestamp |
| `binary_sensor.bticino_audio_ch0-6` | Audio Channels 0-6 | 7 audio channels |

### **✅ Remote Commands Available**
| Command Topic | Function |
|---------------|----------|
| `homeassistant/sensor/bticino/commands/system/ping` | Health check |
| `homeassistant/sensor/bticino/commands/refresh` | Full status refresh |
| `homeassistant/sensor/bticino/commands/sip/status` | SIP status check |
| `homeassistant/sensor/bticino/commands/audio/+/status` | Audio channel check |

---

## 🚀 Quick Start Guide

### **Step 1: Verify BTicino Service is Running**
```bash
ssh root2@bticino
cd /home/bticino/cfg/extra
./bticino_bridge_control.sh status
```

### **Step 2: Check Home Assistant Entities**
1. Go to **Settings** → **Devices & Services** → **MQTT**
2. Look for **"BTicino Classe 300X"** device
3. Verify 11 entities are present and working

### **Step 3: Test Remote Commands**
From Home Assistant Developer Tools → Services:
```yaml
service: mqtt.publish
data:
  topic: "homeassistant/sensor/bticino/commands/system/ping"
  payload: "ping"
```

### **Step 4: Add Dashboard**
Copy the dashboard YAML from `HOME_ASSISTANT_DASHBOARD.yaml` into your Lovelace UI.

---

## 🔧 Management Commands

### **BTicino Hardware Commands**
```bash
# SSH into BTicino
ssh -i ~/.ssh/llave_broker root2@bticino

# Service Management
./bticino_bridge_control.sh start      # Start service
./bticino_bridge_control.sh stop       # Stop service  
./bticino_bridge_control.sh restart    # Restart service
./bticino_bridge_control.sh status     # Check status
./bticino_bridge_control.sh logs       # View live logs

# Check OpenWebNet directly
./bticino_quicktest_arm                 # Interactive testing
```

### **Home Assistant Commands**
```bash
# Test MQTT from command line
mosquitto_pub -h 192.168.1.3 -p 1883 -u mqtt_user -P CHANGE_ME \
  -t "homeassistant/sensor/bticino/commands/refresh" -m "refresh"

# Monitor MQTT data
mosquitto_sub -h 192.168.1.3 -p 1883 -u mqtt_user -P CHANGE_ME \
  -t "homeassistant/sensor/bticino/+/state" -v
```

---

## 📊 Current System Status

**Based on last test run:**
```json
{
  "timestamp": "2025-12-19T07:13:40+01:00",
  "online": true,
  "sip_connection": "connected", 
  "sip_codec": "active",
  "audio_channels": {
    "0": "online", "1": "online", "2": "online", "3": "online",
    "4": "online", "5": "online", "6": "online"
  },
  "error_count": 0
}
```

**Update Frequency:**
- Status monitoring: Every 45 seconds
- MQTT publishing: Every 60 seconds  
- Home Assistant discovery: Every 60 seconds
- Command processing: Immediate

---

## 🏗️ Architecture Overview

```
Home Assistant (192.168.1.3:1883)
           ↕ MQTT
BTicino MQTT Bridge (bticino_mqtt_bridge_arm)
           ↕ OpenWebNet Protocol
BTicino Hardware (127.0.0.1:30006)
           ↕ Physical Hardware
Door/Audio/SIP Systems
```

**Data Flow:**
1. **BTicino → Home Assistant**: Status updates, sensor data
2. **Home Assistant → BTicino**: Remote commands, control signals
3. **Auto-Discovery**: Automatic device/entity creation
4. **Persistence**: Retained MQTT messages, service auto-restart

---

## ⚠️ Security & Safety Notes

### **✅ Safe Operations (Currently Active)**
- Audio system monitoring
- SIP system monitoring  
- System health checks
- Status queries
- Non-invasive commands only

### **🔐 Restricted Operations (Not Implemented)**
- Door control commands (`*8*19*20##`, `*8*20*20##`)
- System configuration changes
- Authentication required commands
- OpenWebNet session commands

### **🛡️ Security Measures**
- Local BTicino network binding (127.0.0.1)
- Command validation and filtering
- Error handling and recovery
- Logging for audit trail
- Process isolation

---

## 🔄 Maintenance & Monitoring

### **Daily Checks**
- ✅ Home Assistant entities updating
- ✅ MQTT connectivity maintained  
- ✅ BTicino service running
- ✅ Error count low (< 5% of total commands)

### **Weekly Maintenance**
```bash
# Check log file size
du -h /var/log/bticino_bridge.log

# Restart service for memory cleanup
./bticino_bridge_control.sh restart

# Verify all Home Assistant entities
# (Check in HA Developer Tools → States)
```

### **Monthly Tasks**
- Review log files for patterns
- Update documentation if needed
- Test remote commands functionality
- Verify backup procedures

---

## 🆘 Troubleshooting

### **Service Not Starting**
```bash
# Check if binary exists and is executable
ls -la /home/bticino/cfg/extra/bticino_mqtt_bridge_arm

# Check configuration
cat /home/bticino/cfg/extra/configs/config.yaml

# Test MQTT connectivity
echo "test" | nc -w 1 192.168.1.3 1883
```

### **Home Assistant Entities Missing**
1. Check MQTT integration is enabled with auto-discovery
2. Verify MQTT credentials: `mqtt_user` / `CHANGE_ME`
3. Restart Home Assistant MQTT integration
4. Check MQTT broker logs

### **Commands Not Working**
```bash
# Test MQTT publish manually
mosquitto_pub -h 192.168.1.3 -p 1883 -u mqtt_user -P CHANGE_ME \
  -t "homeassistant/sensor/bticino/commands/system/ping" -m "test"

# Check BTicino logs
./bticino_bridge_control.sh logs
```

---

## 📈 Future Enhancements

### **Phase 2: Advanced Features** (Not Yet Implemented)
- [ ] OpenWebNet authentication for door control
- [ ] Event-based monitoring (button presses, door events)
- [ ] Historical data logging and analytics
- [ ] Mobile app notifications
- [ ] Integration with security systems

### **Phase 3: Extended Integration** (Future)
- [ ] Video streaming integration
- [ ] Voice control (Alexa, Google Home)
- [ ] Advanced automation rules
- [ ] Multi-device support
- [ ] Cloud connectivity options

---

## 📋 Complete File Reference

**On BTicino Hardware:**
- `/home/bticino/cfg/extra/bticino_mqtt_bridge_arm` - Main service binary
- `/home/bticino/cfg/extra/bticino_bridge_control.sh` - Control script
- `/home/bticino/cfg/extra/configs/config.yaml` - Configuration
- `/var/log/bticino_bridge.log` - Service logs
- `/tmp/bticino_bridge.pid` - Process ID file

**Documentation:**
- `docs/OPENWEBNET_COMMANDS.md` - Command reference
- `docs/HOME_ASSISTANT_INTEGRATION.yaml` - HA configuration
- `docs/HOME_ASSISTANT_DASHBOARD.yaml` - Dashboard templates

---

## ✅ Integration Success Metrics

- **✅ MQTT Connectivity**: Stable connection to Home Assistant
- **✅ Real-time Data**: 11 entities updating every 45-60 seconds  
- **✅ Bi-directional Control**: Commands from HA to BTicino working
- **✅ Auto-Discovery**: Entities automatically created in Home Assistant
- **✅ Service Management**: Start/stop/restart capabilities
- **✅ Error Handling**: Graceful failure recovery
- **✅ Logging**: Complete audit trail
- **✅ Safety**: No interference with BTicino core functionality

## 🎉 **Integration Status: PRODUCTION READY**

The BTicino Classe 300X is now fully integrated with Home Assistant via MQTT, providing real-time monitoring and remote control capabilities while maintaining system safety and security.

---

*Last Updated: March 20, 2026*  
*Integration Version: 0.11.4*  
*Hardware Tested: BTicino Classe 300X*

---

## 🚀 Deployment Guide (v0.11.x)

### Build Binary

```bash
cd bticino_bridge
make build-arm
# Output: /tmp/bticino-bridge-vX.X.X-arm
```

### Deploy to Device

**IMPORTANT**: Deploy both the binary AND the VERSION file.

```bash
# Build
make build-arm

# Deploy binary
dd if=/tmp/bticino-bridge-vX.X.X-arm | ssh bticino 'dd of=/tmp/bridge_bin'
ssh bticino 'cp /tmp/bridge_bin /home/bticino/cfg/extra/bticino-bridge && chmod +x /home/bticino/cfg/extra/bticino-bridge'

# Deploy VERSION file
cat VERSION | ssh bticino 'cat > /home/bticino/cfg/extra/VERSION'
```

### Start/Stop Bridge

```bash
# Stop
ssh bticino 'pkill -9 bticino-bridge'

# Start
ssh bticino 'cd /home/bticino/cfg/extra && ./bticino-bridge -config=/home/bticino/cfg/extra/configs/config.yaml >> /tmp/bticino-bridge.log 2>&1 &'

# Check status
ssh bticino 'pgrep -la bticino-bridge && tail /tmp/bticino-bridge.log'
```

### Files on Device

```
/home/bticino/cfg/extra/           # Persistent (ext4)
├── bticino-bridge                 # Main binary
├── VERSION                        # Version file (must deploy with binary)
├── configs/config.yaml            # Configuration
├── .bridge_autostart              # Autostart flag
├── iptables.rules                 # Firewall rules
├── bridge-autostart.sh            # Autostart script
└── bticino-bridge-service        # Service script

/etc/init.d/                       # Persistent (ext4)
├── bt_daemon-apps.sh             # Modified to start bridge
└── firewall-bticino              # Firewall init script
```

### Autostart

- Bridge auto-starts on device boot via `/etc/init.d/bt_daemon-apps.sh`
- Watchdog loop monitors bridge every 60s, restarts if crashed
- Enable/disable autostart: `touch /home/bticino/cfg/extra/.bridge_autostart`

### Firewall

The device has iptables with INPUT DROP policy. The following ports are opened:
- **8082**: Web Dashboard
- **8081**: HomeKit

Rules are persisted in `/home/bticino/cfg/extra/iptables.rules` and restored on boot.

### Troubleshooting

```bash
# Check if running
ssh bticino 'pgrep -la bticino-bridge'

# Check logs
ssh bticino 'tail -50 /tmp/bticino-bridge.log'

# Check MQTT status
ssh bticino 'wget -qO- http://127.0.0.1:8082/api/status'

# Check web dashboard
ssh bticino 'wget -qO- http://127.0.0.1:8082/'

# External access (from host)
curl http://192.168.1.38:8082/
```