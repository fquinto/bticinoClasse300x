# BTicino Bridge - Deployment Guide for Real Device

**Device**: BTicino Classe 300X (C300X-00-03-50-a8-a7-52-1754162)  
**Access**: `ssh bticino` (special device with limited commands)  
**Version**: v0.12.0 with WebRTC/RTSP Streaming

---

## ⚠️ Device Limitations

The BTicino Classe 300X has a **minimal embedded Linux** environment:

### ❌ Commands NOT Available (or limited)
- `scp` - May not be installed
- `wget` / `curl` - May not be available
- `systemctl` - Uses SysV init (`/etc/init.d/`)
- `docker` - Not available
- `rsync` - Not available
- `git` - Not available
- `go` / `npm` / `node` - Not available (unless manually installed)

### ✅ Commands Available (confirmed)
- `ssh` - Works for remote access
- `base64` - **CRITICAL** for file transfers
- `echo` - Works with redirection
- `cat` - Read files
- `ls` - List files
- `chmod` - Change permissions
- `mkdir` - Create directories
- `mount` - Remount filesystem (read-write)
- `/etc/init.d/*` - Service management
- `netcat` / `nc` - Network communication
- `kill` - Process management

---

## 📋 Pre-Deployment Checklist

### 1. Verify SSH Access

```bash
# Test SSH connection
ssh bticino

# If that doesn't work, try:
ssh root2@192.168.1.38
# or
ssh bticino@192.168.1.38
```

**Expected output**: Shell prompt (may be limited)

### 2. Check Available Disk Space

```bash
ssh bticino "df -h /home"
```

**Minimum required**: ~20 MB for binary + recordings

### 3. Verify Filesystem is Writable

```bash
# Check if /home is writable
ssh bticino "touch /home/bticino/test_write && echo 'WRITABLE' || echo 'READONLY'"
```

If read-only, remount:
```bash
ssh bticino "mount -o remount,rw /"
```

---

## 🚀 Deployment Methods

### Method 1: Direct SCP (if available)

**Try this first** - it's the easiest:

```bash
# From your development machine
cd /path/to/bticino_bridge

# Copy binary
scp bticino_bridge bticino:/home/bticino/cfg/extra/bticino_bridge.new

# Copy config (if updated)
scp configs/config.yaml bticino:/home/bticino/cfg/extra/config.yaml

# Verify transfer
ssh bticino "ls -lh /home/bticino/cfg/extra/bticino_bridge.new"
```

**If SCP works**: Skip to [Post-Deployment Steps](#post-deployment-steps)  
**If SCP fails**: Use Method 2 (Base64)

---

### Method 2: Base64 Transfer (guaranteed to work)

This method works even with minimal SSH access.

#### Step 1: Encode Binary on Development Machine

```bash
# On your development machine (NOT on BTicino)
cd /path/to/bticino_bridge

# Encode binary to base64
base64 bticino_bridge > bticino_bridge.base64

# Verify encoding
wc -l bticino_bridge.base64
# Expected: ~18000-20000 lines for 14MB binary
```

#### Step 2: Transfer Base64 File

**Option A: If `scp` works for text files**

```bash
scp bticino_bridge.base64 bticino:/home/bticino/cfg/extra/
```

**Option B: Copy-paste via SSH (works 100%)**

```bash
# On your development machine
cat bticino_bridge.base64 | xclip -selection clipboard
# or
cat bticino_bridge.base64   # and manually copy output
```

Then on BTicino:
```bash
ssh bticino

# Create file (paste content, then Ctrl+D to save)
cat > /home/bticino/cfg/extra/bticino_bridge.base64
# [PASTE CONTENT HERE]
# [Press Ctrl+D when done]
```

**Option C: Split into chunks (for very large files)**

```bash
# On development machine
split -b 1M bticino_bridge.base64 bticino_bridge.base64.chunk.

# Transfer each chunk
for chunk in bticino_bridge.base64.chunk.*; do
  scp "$chunk" "bticino:/home/bticino/cfg/extra/"
done

# On BTicino, reassemble
ssh bticino
cd /home/bticino/cfg/extra/
cat bticino_bridge.base64.chunk.* > bticino_bridge.base64
```

#### Step 3: Decode on BTicino Device

```bash
ssh bticino

# Navigate to deployment directory
cd /home/bticino/cfg/extra/

# Decode base64 back to binary
base64 -d bticino_bridge.base64 > bticino_bridge.new

# Verify decoding worked
ls -lh bticino_bridge.new
# Expected: ~14M

# Make executable
chmod +x bticino_bridge.new

# Test binary runs
./bticino_bridge.new -version
# Expected: BTicino Classe 300X ENHANCED MEGA Bridge v0.12.0
```

---

### Method 3: Netcat Transfer (if base64 fails)

**Use this as last resort** - slower but works on minimal systems.

#### On BTicino (receiver):

```bash
ssh bticino
cd /home/bticino/cfg/extra/
nc -l -p 9999 > bticino_bridge.new
```

#### On Development Machine (sender):

```bash
nc bticino 9999 < bticino_bridge
```

**Note**: This method is **not encrypted** - only use on trusted networks.

---

## 📝 Post-Deployment Steps

### 1. Backup Current Installation

```bash
ssh bticino

cd /home/bticino/cfg/extra/

# Backup current binary
cp bticino_bridge bticino_bridge.backup

# Backup current config
cp config.yaml config.yaml.backup
```

### 2. Create Recording Directory

```bash
ssh bticino

# Create directory for HKSV recordings
mkdir -p /home/bticino/cfg/extra/recordings

# Set permissions
chmod 755 /home/bticino/cfg/extra/recordings

# Verify
ls -la /home/bticino/cfg/extra/recordings/
```

### 3. Update Configuration

```bash
ssh bticino

cd /home/bticino/cfg/extra/

# Edit config (if nano/vi available)
nano config.yaml

# OR use echo commands (works on all devices)
cat > config.yaml << 'EOF'
# BTicino Bridge Configuration v0.12.0
# Generated: $(date)

bridge:
  name: "BTicino Bridge Enhanced"
  log_level: "info"

openwebnet:
  host: "localhost"
  port: 30006

sip:
  enabled: true
  local_host: "192.168.1.38"
  server_host: "sipserver.bs.iotleg.com"
  server_port: 5061
  transport: "tls"
  domain: "bs.iotleg.com"
  username: "YOUR_USERNAME"
  password: "YOUR_PASSWORD"
  dev_addr: "20"

streaming:
  enabled: true
  rtsp_port: 6554
  recording_path: "/home/bticino/cfg/extra/recordings"
  max_duration: 60
  auto_stop_on_last_client: true
EOF
```

### 4. Stop Current Service

```bash
ssh bticino

# Stop service (SysV init)
/etc/init.d/bticino_bridge stop

# Verify stopped
ps aux | grep bticino_bridge
# Should show no running process (except grep itself)
```

### 5. Replace Binary

```bash
ssh bticino

cd /home/bticino/cfg/extra/

# Replace binary
mv bticino_bridge.new bticino_bridge

# Verify permissions
chmod +x bticino_bridge
ls -lh bticino_bridge
```

### 6. Test Before Starting Service

```bash
ssh bticino

cd /home/bticino/cfg/extra/

# Test configuration
./bticino_bridge -config config.yaml -test

# Expected output:
# - Configuration loaded successfully
# - No critical errors
```

### 7. Start Service

```bash
ssh bticino

# Start service
/etc/init.d/bticino_bridge start

# Check status
/etc/init.d/bticino_bridge status

# Verify running
ps aux | grep bticino_bridge
```

---

## ✅ Verification Steps

### 1. Check Service is Running

```bash
ssh bticino "ps aux | grep bticino_bridge | grep -v grep"
```

**Expected**: Process running with `-config config.yaml`

### 2. Check Logs

```bash
ssh bticino "tail -50 /var/log/bticino_bridge.log"
```

**Expected**: 
- `Enhanced RTSP server started on port 6554`
- `RTSP streams:` with list of paths

### 3. Test Web Dashboard

```bash
# From browser or curl
curl http://192.168.1.38:8082/api/status
```

**Expected**: JSON with version, uptime, components

### 4. Test RTSP Streams

```bash
# Test with ffplay (if available)
ffplay -f rtsp -i rtsp://192.168.1.38:6554/doorbell

# Test with VLC (GUI)
vlc rtsp://192.168.1.38:6554/doorbell

# Test with go2rtc (if configured)
# Check go2rtc logs for stream status
```

### 5. Test Recording

```bash
# Trigger recording (via API)
curl -X POST http://192.168.1.38:8082/api/streaming/record \
  -H "Content-Type: application/json" \
  -d '{"duration": 10}'

# Check recording directory after 10 seconds
ssh bticino "ls -lh /home/bticino/cfg/extra/recordings/"
```

**Expected**: New `.ts` file created

### 6. Test API Endpoints

```bash
# Get streaming status
curl http://192.168.1.38:8082/api/streaming

# Get active sessions
curl http://192.168.1.38:8082/api/streaming/sessions

# Get configuration
curl http://192.168.1.38:8082/api/streaming/config
```

---

## 🔧 Troubleshooting

### Problem: Binary won't execute

```bash
ssh bticino

# Check file type
file bticino_bridge
# Expected: ELF 32-bit LSB executable, ARM

# Check permissions
ls -l bticino_bridge
# Expected: -rwxr-xr-x

# Check library dependencies
ldd bticino_bridge 2>&1 || echo "ldd not available"

# Try running with debug
./bticino_bridge -log-level debug 2>&1 | head -20
```

### Problem: Configuration not loading

```bash
ssh bticino

cd /home/bticino/cfg/extra/

# Validate YAML syntax
# (if python available)
python -c "import yaml; yaml.safe_load(open('config.yaml'))"

# Check file permissions
ls -l config.yaml

# Test with explicit path
./bticino_bridge -config /home/bticino/cfg/extra/config.yaml -test
```

### Problem: RTSP port already in use

```bash
ssh bticino

# Check what's using port 6554
netstat -tlnp | grep 6554 || ss -tlnp | grep 6554

# Kill conflicting process
kill $(lsof -t -i:6554) 2>/dev/null || echo "lsof not available"

# Or use fuser
fuser -k 6554/tcp 2>/dev/null || echo "fuser not available"
```

### Problem: Recording directory not writable

```bash
ssh bticino

# Check permissions
ls -ld /home/bticino/cfg/extra/recordings

# Fix permissions
chmod 755 /home/bticino/cfg/extra/recordings
chown bticino:bticino /home/bticino/cfg/extra/recordings

# Check filesystem is writable
touch /home/bticino/cfg/extra/recordings/test_file
echo $?
# Expected: 0 (success)
```

### Problem: Service won't start

```bash
ssh bticino

# Check init script exists
ls -l /etc/init.d/bticino_bridge

# Check script syntax
bash -n /etc/init.d/bticino_bridge && echo "Syntax OK"

# Try manual start
/home/bticino/cfg/extra/bticino_bridge -config /home/bticino/cfg/extra/config.yaml &

# Check logs
tail -100 /var/log/bticino_bridge.log
```

---

## 📊 Deployment Log Template

Use this template to document each deployment:

```markdown
## Deployment Log - [DATE]

**Device**: BTicino Classe 300X  
**Version**: v0.12.0  
**Deploy Method**: [SCP / Base64 / Netcat]  
**Deployed By**: [Your Name]

### Pre-Deployment
- [ ] SSH access verified
- [ ] Disk space checked: ___ MB available
- [ ] Filesystem writable: YES/NO
- [ ] Backup created: YES/NO

### Deployment Steps
1. [ ] Binary transferred successfully
2. [ ] Binary decoded (if base64)
3. [ ] Recording directory created
4. [ ] Configuration updated
5. [ ] Service stopped
6. [ ] Binary replaced
7. [ ] Service started

### Verification
- [ ] Service running: `ps aux | grep bticino`
- [ ] Logs show no errors: `tail -50 /var/log/bticino_bridge.log`
- [ ] Web dashboard accessible: `curl http://192.168.1.38:8082/api/status`
- [ ] RTSP streams working: Tested with [VLC/ffplay/go2rtc]
- [ ] Recording working: Test file created

### Issues Encountered
[List any issues and how they were resolved]

### Rollback Plan (if needed)
1. Stop service: `/etc/init.d/bticino_bridge stop`
2. Restore backup: `cp bticino_bridge.backup bticino_bridge`
3. Restore config: `cp config.yaml.backup config.yaml`
4. Restart service: `/etc/init.d/bticino_bridge start`

**Deployment Status**: SUCCESS / PARTIAL / FAILED
```

---

## 🔄 Rollback Procedure

If deployment fails, rollback to previous version:

```bash
ssh bticino

# Stop service
/etc/init.d/bticino_bridge stop

# Restore binary
cd /home/bticino/cfg/extra/
cp bticino_bridge.backup bticino_bridge
chmod +x bticino_bridge

# Restore config
cp config.yaml.backup config.yaml

# Restart service
/etc/init.d/bticino_bridge start

# Verify rollback
ps aux | grep bticino_bridge
tail -20 /var/log/bticino_bridge.log
```

---

## 📝 Quick Reference Commands

### SSH Access
```bash
ssh bticino
# or
ssh root2@192.168.1.38
```

### File Transfer (Base64 method)
```bash
# Encode
base64 bticino_bridge > bticino_bridge.base64

# Transfer
scp bticino_bridge.base64 bticino:/home/bticino/cfg/extra/

# Decode
ssh bticino "base64 -d /home/bticino/cfg/extra/bticino_bridge.base64 > /home/bticino/cfg/extra/bticino_bridge"
```

### Service Control
```bash
ssh bticino "/etc/init.d/bticino_bridge {start|stop|restart|status}"
```

### Log Monitoring
```bash
ssh bticino "tail -f /var/log/bticino_bridge.log"
```

### Process Monitoring
```bash
ssh bticino "ps aux | grep bticino_bridge | grep -v grep"
```

---

## 🔨 Build Process (Cross-compilation for ARM)

### From development machine (Linux/macOS):

```bash
# 1. Navigate to project directory
cd bticino_bridge

# 2. Build ARM binary
GOOS=linux GOARCH=arm GOARM=7 go build -v -o bticino_bridge ./cmd/...

# 3. Verify binary is ARM
file bticino_bridge
# Expected: ELF 32-bit LSB executable, ARM

# 4. Create base64 chunks for transfer
rm -f /tmp/bticino_bridge.base64*
base64 bticino_bridge > /tmp/bticino_bridge.base64
split -b 2097152 /tmp/bticino_bridge.base64 /tmp/bticino_bridge.base64.chunk.

# 5. Deploy using auto script
./scripts/deploy_auto.sh
```

### Manual deployment steps:

```bash
# 1. Kill existing process
ssh bticino "pkill -9 -f bticino_bridge"

# 2. Update VERSION file
ssh bticino "echo '0.12.1' > /home/bticino/cfg/extra/VERSION"

# 3. Start with correct config
ssh bticino "cd /home/bticino/cfg/extra && nohup ./bticino_bridge -config config_streaming.yaml > /var/log/bticino_bridge.log 2>&1 &"
```

### Verify deployment:

```bash
# Check process
ssh bticino "ps aux | grep bticino_bridge | grep -v grep"

# Check version
ssh bticino "/home/bticino/cfg/extra/bticino_bridge -version"

# Check web UI
curl http://192.168.1.38:8082/

# Check logs
ssh bticino "tail -20 /var/log/bticino_bridge.log"
```

---

**Last Updated**: 2026-03-24  
**Version**: bticino_bridge v0.12.1  
**Tested On**: BTicino Classe 300X (C300X-00-03-50-a8-a7-52-1754162)
