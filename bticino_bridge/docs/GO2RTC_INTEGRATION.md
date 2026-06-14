# go2rtc Integration Guide

## Overview

go2rtc is a streaming server that provides WebRTC, HLS, and MSE streaming from RTSP sources. It integrates with our bticino_bridge RTSP server to enable video viewing in Home Assistant.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    BTicino Classe 300X                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  bticino_bridge (RTSP Server) ──► :6554                        │
│         │                                                      │
│         │ SIP call initiates when client connects               │
│         ▼                                                      │
│  SIP ◄──► sipserver.bs.iotleg.com                             │
│         │                                                      │
│         │ RTP (H.264 + Speex)                                  │
│         ▼                                                      │
│  Stream ready for RTSP clients                                 │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                               │
                               │ rtsp://192.168.1.38:6554/doorbell
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                         go2rtc                                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  streams:                                                       │
│    doorbell: rtsp://192.168.1.38:6554/doorbell               │
│                                                                  │
│  Ports:                                                         │
│  - :8554 (RTSP server - disabled, we use as client only)       │
│  - :8555 (WebRTC)                                              │
│  - :1984 (API)                                                 │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                               │
                               │ WebRTC / HLS
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Home Assistant                                 │
│  Camera entity with live streaming                              │
└─────────────────────────────────────────────────────────────────┘
```

## Installation Options

### Option 1: Docker (Recommended)

```yaml
# docker-compose.yml
services:
  go2rtc:
    image: alexxit/go2rtc:latest
    container_name: go2rtc
    restart: unless-stopped
    ports:
      - "1984:1984"  # API
      - "8555:8555"  # WebRTC
    volumes:
      - ./go2rtc.yaml:/config/go2rtc.yaml:ro
```

### Option 2: Standalone Binary

Download from: https://github.com/AlexxIT/go2rtc/releases

```bash
# Download for ARM7 (our device)
curl -L -o go2rtc https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_arm
chmod +x go2rtc
./go2rtc
```

## Configuration

### Minimal go2rtc.yaml

```yaml
# go2rtc.yaml
streams:
  doorbell: rtsp://192.168.1.38:6554/doorbell
```

### Full Configuration with Audio Support

```yaml
# go2rtc.yaml
streams:
  doorbell:
    - rtsp://192.168.1.38:6554/doorbell#video=copy#audio=copy
    # Backchannel for two-way audio (optional)
    - exec:ffmpeg -re -fflags nobuffer -f alaw -ar 8000 -i - -ar 8000 -acodec speex -f rtp -payload_type 97 rtp://192.168.1.38:40004

# WebRTC configuration
webrtc:
  listen: :8555
  candidates:
    - 192.168.1.38:8555
    - stun:stun.l.google.com:19302

# API configuration
api:
  listen: :1984

# RTSP server (disabled - we only use as client)
rtsp:
  listen: ""

# Log configuration
log:
  level: info
```

## Home Assistant Integration

### Option 1: Native go2rtc Integration (HA 2024.11+)

Add to `configuration.yaml`:

```yaml
go2rtc:
  streams:
    doorbell: rtsp://192.168.1.38:6554/doorbell
```

Then add camera:

```yaml
camera:
  - name: Videoportero
    icon: mdi:doorbell-video
    stream_source: go2rtc://doorbell
```

### Option 2: Manual Configuration

Create camera with WebRTC URL:

```yaml
camera:
  - name: Videoportero
    icon: mdi:doorbell-video
    stream_source: rtsp://192.168.1.38:1984/stream/doorbell
```

Or use Picture Glance card with WebRTC:

```yaml
type: custom:webrtc-camera
entity: camera.videoportero
url: rtsp://192.168.1.38:1984/stream/doorbell
```

## Testing

### Test RTSP Stream

```bash
# With VLC or ffmpeg
ffplay rtsp://192.168.1.38:6554/doorbell
```

### Test WebRTC

Open in browser: `http://192.168.1.38:1984`

### API Endpoints

```bash
# List streams
curl http://192.168.1.38:1984/api/streams

# Get stream info
curl http://192.168.1.38:1984/api/stream/doorbell

# Get snapshot
curl http://192.168.1.38:1984/apiframe/doorbell?jpg
```

## Troubleshooting

### Stream not connecting

1. Check bticino_bridge is running: `ps aux | grep bticino-bridge`
2. Check RTSP port: `netstat -tlnp | grep 6554`
3. Check go2rtc logs

### Audio not working

1. Verify Speex codec support in go2rtc
2. Check backchannel configuration
3. Try with audio disabled first:

```yaml
streams:
  doorbell: rtsp://192.168.1.38:6554/doorbell#video=copy#audio=off
```

### High latency

1. Use WebRTC instead of HLS
2. Reduce quality in camera settings
3. Use TCP transport for RTSP:

```yaml
streams:
  doorbell: rtsp://192.168.1.38:6554/doorbell#transport=tcp
```

## Advanced: Two-Way Audio

For two-way audio with the intercom:

1. Enable UDP proxy in bticino_bridge config:

```yaml
sip:
  enabled: true
  # ... other config

udp_proxy:
  enabled: true
  listen_port: 40004
  target_port: 4000
```

2. Configure go2rtc backchannel:

```yaml
streams:
  doorbell:
    - rtsp://192.168.1.38:6554/doorbell
    - exec:ffmpeg -re -fflags nobuffer -f alaw -ar 8000 -i - -ar 8000 -acodec speex -f rtp rtp://192.168.1.38:40004
```

3. Use WebRTC with backchannel in HA

## Security Considerations

- RTSP port 6554 should be accessible only from local network
- Consider firewall rules to restrict access
- go2rtc API port 1984 should have authentication if exposed

```yaml
api:
  listen: 192.168.1.38:1984
  username: admin
  password: your_secure_password
```
