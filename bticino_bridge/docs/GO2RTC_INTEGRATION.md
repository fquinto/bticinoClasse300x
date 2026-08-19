# go2rtc Integration Guide

## Overview

go2rtc is a streaming server that provides WebRTC, HLS, and MSE streaming from RTSP sources. It integrates with our bticino_bridge RTSP server to enable video viewing in Home Assistant.

go2rtc runs **on the device** (ARM binary in `/home/bticino/cfg/extra/go2rtc/`) and, since v0.18.0, auto-starts from `start-bridge.sh` alongside the bridge. Its API listens on `:1984` and WebRTC on `:8555`.

## Architecture (v0.18.0 — live video on call)

Since v0.18.0 the **live video during a call** does **not** use a self-INVITE against an external SIP server. When a call rings your unit, a passive AF_PACKET tap mirrors the native video leg (cleartext H.264 RTP on `lo:5007`) into the bridge's RTP relay, which serves it over RTSP as **`/doorbell-video`**. No commands are sent to the device bus. See `docs/HOME_ASSISTANT.md` and the CHANGELOG v0.18.0 entry.

```
┌─────────────────────────────────────────────────────────────────┐
│                    BTicino Classe 300X                             │
├─────────────────────────────────────────────────────────────────┤
│  Native call ringing  ──►  bt_av_media streams H.264 RTP         │
│                            (cleartext) to  lo:5007               │
│         │                                                        │
│         │  passive AF_PACKET tap (read-only, zero bus commands)  │
│         ▼                                                        │
│  bridge RTP relay (10002)                                        │
│         │                                                        │
│         ▼                                                        │
│  bticino_bridge RTSP server ──► :6554  path /doorbell-video      │
└─────────────────────────────────────────────────────────────────┘
                               │
                               │ rtsp://127.0.0.1:6554/doorbell-video
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                         go2rtc (on device)                       │
├─────────────────────────────────────────────────────────────────┤
│  streams:                                                        │
│    doorbell-video: rtsp://127.0.0.1:6554/doorbell-video          │
│                                                                  │
│  Ports:                                                          │
│  - :8555 (WebRTC)                                                │
│  - :1984 (API)                                                   │
└─────────────────────────────────────────────────────────────────┘
                               │
                               │ WebRTC / HLS
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Home Assistant                                 │
│  Camera entity with live streaming (video-only during a call)    │
└─────────────────────────────────────────────────────────────────┘
```

> **Note on `/doorbell` vs `/doorbell-video`:** the older `/doorbell` stream (self-INVITE + audio, described in the legacy sections below) is superseded for live-on-call by `/doorbell-video`, which carries the passively-tapped video only. Point Home Assistant at `doorbell-video` for the live-video-on-call feature.

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
# go2rtc.yaml — live video on call (v0.18.0). Runs on the device, so use loopback.
streams:
  doorbell-video: rtsp://127.0.0.1:6554/doorbell-video
```

The `/doorbell-video` stream only carries frames while a call is ringing/active
(that is when the passive tap has RTP to mirror). Outside a call the camera card
shows the last snapshot as a preview. The legacy `/doorbell` stream (with the
external self-INVITE and audio backchannel) is kept below for reference.

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
