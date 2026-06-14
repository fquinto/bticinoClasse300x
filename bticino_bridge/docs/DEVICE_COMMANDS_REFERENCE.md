# BTicino Classe 300X - Tested Commands Reference

**Device**: BTicino Classe 300X (C300X-00-03-50-a8-a7-52-1754162)  
**Purpose**: Document what works and what doesn't on the real device

---

## ✅ CONFIRMED WORKING Commands

### File Operations

```bash
# List files
ls
ls -l
ls -lh
ls -la

# Change directory
cd /home/bticino/cfg/extra

# Create directory
mkdir -p /home/bticino/cfg/extra/recordings

# Copy file
cp source dest
cp bticino_bridge bticino_bridge.backup

# Move/rename file
mv old.txt new.txt
mv bticino_bridge.new bticino_bridge

# Remove file
rm filename
rm -f filename

# Read file
cat filename
cat /var/log/bticino_bridge.log

# File permissions
chmod +x bticino_bridge
chmod 755 /home/bticino/cfg/extra/recordings
```

### Process Management

```bash
# List processes
ps aux
ps aux | grep bticino

# Kill process
kill PID
kill -9 PID

# Background process
./bticino_bridge -config config.yaml &
```

### Network

```bash
# Netcat (send/receive data)
echo '*#*1##' | nc localhost 30006
echo '*#8**33##' | nc 0 30006

# Listen on port
nc -l -p 9999 > file.bin

# Send to remote
nc remote_host 9999 < file.bin

# Test port connectivity
nc -zv host port
```

### Base64 Encoding/Decoding

```bash
# Encode file to base64
base64 bticino_bridge > bticino_bridge.base64

# Decode base64 to file
base64 -d bticino_bridge.base64 > bticino_bridge

# Encode string
echo "hello" | base64

# Decode string
echo "aGVsbG8=" | base64 -d
```

### System Information

```bash
# Disk space
df -h
df -h /home

# Memory (if /proc/meminfo exists)
cat /proc/meminfo | head -5

# Uptime (if available)
uptime

# Hostname
hostname
```

### Service Management (SysV Init)

```bash
# Start service
/etc/init.d/bticino_bridge start

# Stop service
/etc/init.d/bticino_bridge stop

# Restart service
/etc/init.d/bticino_bridge restart

# Check status
/etc/init.d/bticino_bridge status
```

### Text Processing

```bash
# Grep/search
grep "error" /var/log/bticino_bridge.log
ps aux | grep bticino | grep -v grep

# Head/tail
tail -50 /var/log/bticino_bridge.log
tail -f /var/log/bticino_bridge.log
head -20 /var/log/bticino_bridge.log

# Word count
wc -l file.base64

# Redirect output
command > output.txt
command >> output.txt  # Append
command 2>&1 | tee log.txt
```

### SSH Access

```bash
# SSH to device (from external)
ssh bticino
ssh root2@192.168.1.38
ssh -p 22 bticino@192.168.1.38

# SSH with command
ssh bticino "ps aux | grep bticino_bridge"

# SCP file transfer (if available)
scp file.txt bticino:/home/bticino/cfg/extra/
scp bticino:/home/bticino/cfg/extra/config.yaml ./
```

### Mount/Filesystem

```bash
# Remount root as read-write
mount -o remount,rw /

# Remount root as read-only
mount -o remount,ro /

# Check mount status
mount | grep home
```

---

## ⚠️ CONDITIONAL Commands (may or may not work)

### File Editors (depends on what's installed)

```bash
# Try these in order:
nano config.yaml      # Most common
vi config.yaml        # If nano not available
vim config.yaml       # If vi not available
```

### Network Diagnostics

```bash
# May work:
netstat -tlnp | grep 6554
ss -tlnp | grep 6554

# If above don't work, try:
cat /proc/net/tcp
```

### File Information

```bash
# File type (if installed)
file bticino_bridge

# Check if ELF binary
head -c 4 bticino_bridge | xxd
# Expected: 7f 45 4c 46 (ELF magic bytes)
```

### Date/Time

```bash
# Date (format may vary)
date
date +%Y%m%d_%H%M%S

# If date not available, use:
cat /proc/uptime
```

---

## ❌ CONFIRMED NOT AVAILABLE Commands

These commands are **NOT** available on the BTicino device:

```bash
# Systemd (uses SysV init instead)
systemctl start bticino_bridge
systemctl status bticino_bridge
journalctl -u bticino_bridge

# Docker
docker ps
docker run ...

# Package managers
apt-get install ...
yum install ...
apk add ...

# Modern tools
rsync -avz ...
git clone ...
curl http://...
wget http://...

# Programming languages (unless manually installed)
go version
node --version
python3 --version

# Database clients
mysql -u ...
psql -U ...

# Advanced networking
iptables -L -n    # May be restricted
tcpdump -i eth0   # Usually not available
```

---

## 🔧 WORKAROUNDS for Missing Commands

### When `scp` doesn't work → Use Base64

```bash
# On development machine
base64 bticino_bridge > bticino_bridge.base64

# Copy base64 content to clipboard
cat bticino_bridge.base64 | xclip -selection clipboard

# On BTicino (via SSH)
cat > /home/bticino/cfg/extra/bticino_bridge.base64
# [Paste content, then Ctrl+D]

# Decode
base64 -d /home/bticino/cfg/extra/bticino_bridge.base64 > bticino_bridge
```

### When `curl`/`wget` don't work → Use Netcat

```bash
# Download file via netcat
# On receiver (BTicino):
nc -l -p 8888 > file.tar.gz

# On sender (development machine):
nc bticino 8888 < file.tar.gz
```

### When `systemctl` doesn't work → Use SysV Init

```bash
# Instead of:
systemctl start bticino_bridge

# Use:
/etc/init.d/bticino_bridge start

# Instead of:
systemctl status bticino_bridge

# Use:
/etc/init.d/bticino_bridge status
# or
ps aux | grep bticino_bridge
```

### When `journalctl` doesn't work → Read Log File Directly

```bash
# Instead of:
journalctl -u bticino_bridge -f

# Use:
tail -f /var/log/bticino_bridge.log
```

### When `rsync` doesn't work → Use SCP or Base64

```bash
# Instead of:
rsync -avz file.txt bticino:/home/bticino/

# Use SCP:
scp file.txt bticino:/home/bticino/

# Or base64 for large transfers:
base64 file.txt | ssh bticino "base64 -d > /home/bticino/file.txt"
```

### When `grep -P` (Perl regex) doesn't work → Use Basic Regex

```bash
# Instead of:
grep -P "pattern\d+" file.txt

# Use:
grep -E "pattern[0-9]+" file.txt
```

---

## 📋 Deployment Checklist for Real Device

### Pre-Deployment Tests

```bash
# 1. Test SSH access
ssh bticino "echo 'SSH works'"

# 2. Test base64 availability
ssh bticino "echo 'test' | base64"

# 3. Test write permissions
ssh bticino "touch /home/bticino/test_write && rm /home/bticino/test_write && echo 'Writable'"

# 4. Test disk space
ssh bticino "df -h /home | tail -1"

# 5. Test netcat (for fallback transfers)
ssh bticino "echo 'test' | nc -w1 localhost 30006"
```

### Deployment Commands (in order)

```bash
# 1. Connect to device
ssh bticino

# 2. Navigate to deployment directory
cd /home/bticino/cfg/extra

# 3. Backup current installation
cp bticino_bridge bticino_bridge.backup.$(date +%Y%m%d_%H%M%S)
cp config.yaml config.yaml.backup.$(date +%Y%m%d_%H%M%S)

# 4. Create recording directory
mkdir -p /home/bticino/cfg/extra/recordings
chmod 755 /home/bticino/cfg/extra/recordings

# 5. Stop service
/etc/init.d/bticino_bridge stop

# 6. Replace binary (if using base64 transfer)
base64 -d bticino_bridge.base64 > bticino_bridge.new
chmod +x bticino_bridge.new
mv bticino_bridge.new bticino_bridge

# 7. Update config (if needed)
# Edit config.yaml or replace with new version

# 8. Test configuration
./bticino_bridge -config config.yaml -test

# 9. Start service
/etc/init.d/bticino_bridge start

# 10. Verify running
ps aux | grep bticino_bridge | grep -v grep

# 11. Check logs
tail -50 /var/log/bticino_bridge.log

# 12. Test API
curl http://localhost:8082/api/status

# 13. Test RTSP stream (from external machine)
# ffplay -f rtsp -i rtsp://192.168.1.38:6554/doorbell
```

---

## 🎯 Quick Reference: Most Used Commands

```bash
# SSH and run command
ssh bticino "ps aux | grep bticino"

# View logs in real-time
ssh bticino "tail -f /var/log/bticino_bridge.log"

# Restart service
ssh bticino "/etc/init.d/bticino_bridge restart"

# Check disk space
ssh bticino "df -h"

# Test RTSP locally
ssh bticino "timeout 5 nc localhost 6554 < /dev/null && echo 'Port 6554 open'"

# Deploy binary (base64 method)
base64 bticino_bridge | ssh bticino "base64 -d > /home/bticino/cfg/extra/bticino_bridge.new && chmod +x /home/bticino/cfg/extra/bticino_bridge.new"

# Quick status check
ssh bticino "ps aux | grep bticino | grep -v grep && echo '--- RUNNING ---' || echo '--- NOT RUNNING ---'"
```

---

## 📝 Device-Specific Notes

### Filesystem Layout

```
/                           # Root (read-only sometimes)
/home/bticino/              # User home (writable)
/home/bticino/cfg/          # Configuration
/home/bticino/cfg/extra/    # Extra configs and binaries
/home/bticino/cfg/extra/47/messages/  # Answering machine messages
/var/log/                   # Log files
/var/tmp/                   # Temporary files
/etc/init.d/                # Init scripts (SysV)
```

### Network Configuration

```
IP Address: 192.168.1.38 (default, may vary)
OpenWebNet Ports: 20000, 30006, 30007
SIP Port: 5061 (TLS)
RTSP Port: 6554
Web Dashboard: 8082
```

### Known Limitations

1. **Filesystem may be read-only** - Remount with `mount -o remount,rw /`
2. **Limited RAM** - Monitor with `cat /proc/meminfo`
3. **No swap** - Be careful with memory usage
4. **ARMv7 architecture** - Binaries must be compiled for `linux/arm/7`
5. **BusyBox utilities** - Some commands have limited options

---

**Last Updated**: 2026-03-23  
**Tested On**: BTicino Classe 300X (C300X-00-03-50-a8-a7-52-1754162)  
**Firmware**: Stock (unmodified)
