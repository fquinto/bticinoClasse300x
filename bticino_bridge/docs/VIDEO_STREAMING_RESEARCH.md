# BTicino Classe 300X - Complete Technical Research

> Consolidated from: slyoldfox/c300x-controller source code, Telegram chat exports,
> decompiled Android app (DoorEntryCLASSE300X v1.10.8), and direct device investigation.
> Last updated: April 4, 2026

---

## Table of Contents

1. [Video Streaming Pipeline](#1-video-streaming-pipeline)
2. [OpenWebNet Protocol](#2-openwebnet-protocol)
3. [SIP Configuration](#3-sip-configuration)
4. [RTSP Server](#4-rtsp-server)
5. [Device Configuration & Filesystem](#5-device-configuration--filesystem)
6. [Network & Firewall](#6-network--firewall)
7. [Cloud API (Eliot Platform)](#7-cloud-api-eliot-platform)
8. [Firmware Update Protocol](#8-firmware-update-protocol)
9. [Security Findings](#9-security-findings)
10. [Android App Internals](#10-android-app-internals)
11. [Hardware](#11-hardware)
12. [Internal MQTT Bus (Port 60000)](#12-internal-mqtt-bus-port-60000)
13. [ARM Disassembly: bt_av_media](#13-arm-disassembly-bt_av_media)
14. [ARM Disassembly: libjel.so (MQTT Client Library)](#14-arm-disassembly-lijelso-mqtt-client-library)
15. [ARM Disassembly: libjsonmsg.so (JSON-RPC Library)](#15-arm-disassembly-libjsonmsgso-json-rpc-library)
16. [The Pipeline Activation Problem](#16-the-pipeline-activation-problem)
17. [Current Implementation Status](#17-current-implementation-status)

---

## 1. Video Streaming Pipeline

### 1.1 Architecture Overview (from slyoldfox)

```
RTSP Client (ffplay/go2rtc/HomeKit)
    |
    v
RTSP Server (port 6554)  --triggers-->  SIP INVITE (port 5060, TCP)
    |                                        |
    |                                        v
    |                              Flexisip SIP Proxy
    |                                        |
    |                                        v
    |                              OpenWebNet (port 30007) --> bt_av_media daemon
    |                                        |
    <------------ RTP audio/video -----------+
```

### 1.2 The Dual-Path Trick (Critical Design Insight)

The SIP INVITE uses `RTP/SAVP` (SRTP) with a **dummy crypto key** and **throwaway ports** (65000/65002).
The intercom sends encrypted SRTP to those ports (nothing listens). Meanwhile, the OpenWebNet
`*7*300#...` command tells `bt_av_media` to send **unencrypted plain RTP** to the real target ports
(10000/10002). This completely sidesteps SRTP decryption.

The SIP call is only needed to:
1. **Trigger** the intercom to start its media pipeline
2. **Keep the call alive** (intercom stops streaming on BYE)
3. Signal `a=DEVADDR:20` for `bt_answering_machine`

### 1.3 Complete Outgoing Call Sequence

```
1. RTSP client connects to rtsp://IP:6554/doorbell
2. checkMount() fires
3. registry.updateStreamEndpoint('127.0.0.1', 10000, 10002)
4. SipManager.invite() sends SIP INVITE with dummy SRTP SDP
   - Audio port 65000 (throwaway), Video port 65002 (throwaway)
   - a=DEVADDR:20 prepended for bt_answering_machine
5. Intercom answers with 200 OK (contains its real SRTP SDP)
6. Controller sends ACK
7. Intercom's OpenWebNet emits: *7*300#127#0#0#1#5007#0*##
8. openwebnet-handler intercepts -> calls enableStream()
9. bt-av-media sends to port 30007: *7*300#<IP_HASH>#10002#0*## (video)
10. After 300ms delay: *7*300#<IP_HASH>#10000#2*## (audio)
11. bt_av_media daemon starts sending UNENCRYPTED RTP to ports 10000/10002
12. RTSP server receives RTP and re-serves to connected RTSP clients
13. On client disconnect: 7.5s grace period -> SIP BYE
```

### 1.4 Complete Incoming Call Sequence (Doorbell Pressed)

```
1. Intercom's OpenWebNet emits: *8*1#1#4#*  (doorbell pressed)
2. openwebnet-handler: set streamEndpoint='all', dispatch 'pressed'
3. Intercom sends SIP INVITE to controller
4. Controller sends 180 Ringing
5. Controller registers temp endpoint: videoPort=10002, audioPort=10000
6. Emits 'homekit:pressed' event
7. RTSP client connects (e.g., HomeKit requests stream)
8. checkMount() -> SipManager.invite() with incomingCallRequest
9. Controller sends 200 OK with SDP to intercom
10. OpenWebNet triggers stream activation -> bt-av-media
11. Unencrypted RTP flows to ports 10000/10002
12. RTSP server re-serves to clients
13. 60-second timeout auto-resets incoming call if no RTSP client connects
```

### 1.5 Video Parameters

| Parameter     | High-res | Low-res |
|---------------|----------|---------|
| Resolution    | 800x480  | 400x288 |
| Bitrate       | 2500kbps | -       |
| Codec         | H.264    | H.264   |
| Profile       | Baseline, Level 3.1 (`42801F`) | - |
| Payload Type  | 96       | 96      |
| Clock Rate    | 90000 Hz | 90000 Hz|

### 1.6 Audio Parameters

| Parameter     | Value           |
|---------------|-----------------|
| Codec         | Speex           |
| Payload Type  | 110             |
| Clock Rate    | 8000 Hz         |
| Mode          | Narrowband      |

### 1.7 Video Capture on Device

```bash
# Direct video capture test
yavta -f UYVY -s 720x576 -n 4 --capture=10 -F /dev/video2

# Video config query response:
*7*77#800#480#2500#148#83#2#800#180#10#15#400#288#0#4000*##
# Decoded: width=800, height=480, bitrate=2500, ..., low_width=400, low_height=288, port=4000
```

Video devices: `/dev/video0`, `/dev/video2`

---

## 2. OpenWebNet Protocol

### 2.1 General Frame Format

```
*WHO*WHAT*WHERE##           (command)
*#WHO*WHERE*DIM*VALUE##     (dimension request/response)
*#*1##                      (ACK)
*#*0##                      (NACK)
```

### 2.2 Media Commands (WHO=7) - Port 30007

Sent to `127.0.0.1:30007` (bt_av_media daemon).

| Command | Format | Purpose |
|---------|--------|---------|
| High-res video | `*7*300#<IP_HASH>#<PORT>#0*##` | Start H.264 high-res video stream |
| Low-res video  | `*7*300#<IP_HASH>#<PORT>#1*##` | Start H.264 low-res video stream |
| Audio          | `*7*300#<IP_HASH>#<PORT>#2*##` | Start Speex/8000 audio stream |

- **IP hash format**: `192.168.1.38` -> `192#168#1#38`
- **Stream type** (last digit before `*##`): `0`=high video, `1`=low video, `2`=audio

Response codes:
- `*#*1##` = Success (ACK)
- `*#*1##*#*1##` = Double ACK (known quirk, treat as success)
- `*#*0##` = NACK (retry up to 3 times, 1s delay between)

**Socket behavior**: idle timeout 5000ms, retry 3x on error with 1s delay.

**Timing**: Video first, then audio after 300ms delay.

### 2.3 OpenWebNet Events (intercepted on port 20000/30007)

| Event Pattern | Meaning |
|---------------|---------|
| `*8*19*<ID>##` | Door **unlock** |
| `*8*20*<ID>##` | Door **lock** (close) |
| `*8*21*<devdev><devaddr>##` | Light **on** |
| `*8*22*<devdev><devaddr>##` | Light **off** |
| `*8*1#5#4#*` | **View doorbell** (camera preview, not a call) |
| `*8*1#1#4#*` | **Incoming call** (doorbell pressed) |
| `*7*300#127#0#0#1#5007#0*##` | Intercom requests its own high-res video |
| `*7*300#127#0#0#1#5000#2*##` | Intercom requests its own audio |
| `*7*73#0#0*##` | Streams closing (intermediate) |
| `*7*0*##` | **Streams closed** (final cleanup) |
| `*#8**33*0##` | Mute ON |
| `*#8**33*1##` | Mute OFF |

### 2.4 Lock/Door Commands (WHO=8)

```
*8*19*<devdev><devaddr>##    # Unlock (open door)
*8*20*<devdev><devaddr>##    # Lock (close door)
*8*21*<devdev><devaddr>##    # Light on
*8*22*<devdev><devaddr>##    # Light off
```

### 2.5 FTP Control (WHO=13)

```
*#13**#31*0##    # Start FTP service
*#13**#33*0##    # Stop FTP service
```

### 2.6 Device Discovery (WHO=1013)

```
*#1013**1##                              # Query device model
*#1013*<model_id>*...##                  # Response: model 68 = C300X
*#13**12##                               # Query MAC address
*#13**12*<b1>*<b2>*<b3>*<b4>*<b5>*<b6>## # MAC response (6 decimal bytes)
```

### 2.7 Authentication (Port 20000)

**HMAC mode** (modern):
```
Client                              Server (port 20000)
  |  <--- *#*1##                       |  (ACK - session ready)
  |  ---> *99*0##                      |  (request auth session)
  |  <--- *98*2##                      |  (HMAC mode selected)
  |  ---> *#*1##                       |  (ACK)
  |  <--- *#<server_challenge>##       |  (challenge + nonce)
  |  ---> *#<Ra>*<client_response>##   |  (client random + HMAC)
  |  <--- *#<server_response>##        |  (server HMAC proof)
  |  ---> *#*1##                       |  (mutual auth complete)
```

**Legacy mode** (simple):
```
  |  <--- *#*1##                       |  (ACK)
  |  ---> *99*0##                      |  (request auth)
  |  <--- *#<nonce>##                  |  (challenge)
  |  ---> *#<hashed_response>##        |  (password hash)
  |  <--- *#*1##                       |  (ACK = success)
```

- Password is **numeric only**, range `1` to `999999999`
- Brute-forceable offline in ~4 minutes (slyoldfox confirmed)
- Reference: `openwebnet4j` BUSConnector.java#L364

### 2.8 Multicast Data

- LC02 header format with OPEN commands in payload
- Used for inter-device communication on the SCS bus

---

## 3. SIP Configuration

### 3.1 SIP Parameters (from slyoldfox)

| Parameter | Value |
|-----------|-------|
| `from` | `webrtc@127.0.0.1` |
| `to` | `c300x@127.0.0.1` (c300x) or `c100x@127.0.0.1` (c100x) |
| `domain` | Auto-detected from `/etc/flexisip/domain-registration.conf` |
| `transport` | **TCP** (`useTcp: true`) |
| `localPort` | `5060` |
| `expire` | `600` (clamped to 300 in code) |
| `devaddr` | `20` (hardcoded on c300x, auto-detected on c100x) |
| `gruuInstanceId` | `19609c0e-f27b-7595-e9c8269557c4240b` |

### 3.2 REGISTER Request Format

```
REGISTER sip:<domain> SIP/2.0
From: sip:webrtc@<domain>
To: sip:webrtc@<domain>
Contact: <sip:webrtc@127.0.0.1>;+sip.instance="<urn:uuid:19609c0e-f27b-7595-e9c8269557c4240b>"
Supported: replaces, outbound, gruu
Allow: INVITE, ACK, CANCEL, OPTIONS, BYE, REFER, NOTIFY, MESSAGE, SUBSCRIBE, INFO, UPDATE
Expires: 600
```

### 3.3 SIP Header Rewriting Rules

The `send` hook rewrites headers before transmission because flexisip uses an unresolvable public domain:

| Method | Rewrite Rules |
|--------|---------------|
| REGISTER | `uri` -> `sip:<domain>`, `to.uri` -> `from@domain`, append GRUU to Contact |
| INVITE/MESSAGE | `uri` and `to.uri` -> `to@domain` |
| ACK/BYE | `to.uri` -> `to@domain`, `uri` -> `registrarContact` |
| 200 OK (response) | Append GRUU with `gr=urn:uuid:` parameter |
| All (if not UDP) | Append `;transport=<protocol>` to Contact |

### 3.4 SIP INVITE SDP (Outgoing)

```
v=0
o=webrtc 3747 461 IN IP4 127.0.0.1
s=ScryptedSipPlugin
c=IN IP4 127.0.0.1
t=0 0
a=DEVADDR:20
m=audio 65000 RTP/SAVP 110
a=rtpmap:110 speex/8000
a=crypto:1 AES_CM_128_HMAC_SHA1_80 inline:dummykey
m=video 65002 RTP/SAVP 96
a=rtpmap:96 H264/90000
a=fmtp:96 profile-level-id=42801F
a=crypto:1 AES_CM_128_HMAC_SHA1_80 inline:dummykey
a=recvonly
```

Key points:
- `RTP/SAVP` (SRTP) with **dummy key** - encrypted data goes to throwaway ports
- Audio port `65000` and video port `65002` are **throwaway** (nothing listens)
- `a=DEVADDR:20` is **required** by `bt_answering_machine`
- `a=recvonly` - controller only receives, doesn't send video
- Real unencrypted streams come via OpenWebNet `*7*300` commands on separate ports

### 3.5 DTMF via SIP INFO

```
INFO sip:... SIP/2.0
Content-Type: application/dtmf-relay

Signal=<key>
Duration=250
```

### 3.6 Registration Lifecycle

- Initial delay: 2 seconds after start
- Re-register when: `now - lastRegistration >= (expire * 1000) - 10000`
- Check interval: every 10 seconds
- Registration timeout: 3 seconds
- On error: delay next attempt by 60 seconds
- Will NOT re-register if there's an active call or incoming call request

### 3.7 SIP Credentials

- Cloud API: `https://www.myhomeweb.com/eliot/sip/users/plants/<plantid>/gateway/<GatewayId>`
- Local DB: `/etc/flexisip/users/users.db.txt`
- Format: `username@domain md5:hash` or `username@domain plain:password`
- HA1 = `MD5(username:realm:password)`
- SIP certs for port 5061: obtained by sending CSR to Bticino cloud

### 3.8 Flexisip Setup Requirements

After flashing, 3 changes needed:
1. Open port 5060
2. Add trusted-hosts for flexisip
3. Disable firewall

---

## 4. RTSP Server

### 4.1 Server Configuration

| Parameter | Value |
|-----------|-------|
| Listen port | `6554` |
| RTP port start | `10000` |
| RTP port range | `10000-19999` |
| Audio RTP port | `10000` |
| Video RTP port | `10002` |

### 4.2 Mount Points

| Path | Content | Streams | Requires SIP? |
|------|---------|---------|----------------|
| `/doorbell` | Audio + Video | streamid=0 (audio), streamid=1 (video) | Yes (always) |
| `/doorbell-video` | Video only | streamid=1 (video) | No, if incoming call active |
| `/doorbell-recorder` | Audio + Video | Reuses `/doorbell` streams | No, if incoming call active |

### 4.3 SDP Offered to Clients

**Full (audio + video):**
```
v=0
o=- 0 0 IN IP4 127.0.0.1
s=No Name
c=IN IP4 127.0.0.1
t=0 0
m=audio 0 RTP/AVP 110
a=rtpmap:110 speex/8000
a=control:streamid=0
m=video 0 RTP/AVP 96
a=rtpmap:96 H264/90000
a=control:streamid=1
```

Note: `RTP/AVP` (not SAVP) - RTSP provides **unencrypted** RTP to clients.

### 4.4 Stream Lifecycle

- Kill handler delay: **7.5 seconds** after last client disconnects before sending SIP BYE
- Polls every 1 second during grace period
- On remote BYE: immediately close all client wrappers and destroy sockets

### 4.5 go2rtc Integration

```yaml
streams:
  doorbell:
    - "ffmpeg:rtsp://127.0.0.1:6554/doorbell#video=copy#audio=pcma"
    - "exec:/usr/local/bin/ffmpeg -re -fflags nobuffer -f alaw -ar 8000 -i - -ar 8000 -acodec speex -f rtp -payload_type 97 rtp://127.0.0.1:40004#backchannel=1"
webrtc:
  listen: :8555
  candidates:
    - 192.168.1.38:8555
    - stun:stun.l.google.com:19302
api:
  listen: :1984
ffmpeg:
  bin: /usr/local/bin/ffmpeg
```

Usage hint from slyoldfox: `ffmpeg:rtsp://192.168.0.XX:6554/doorbell#video=copy#audio=pcma`

---

## 5. Device Configuration & Filesystem

### 5.0 Filesystem Partitions

| Partition | Mount | Type | Mode | Persistent |
|-----------|-------|------|------|------------|
| `/dev/mmcblk2p6` | `/` | ext4 | **ro** (read-only by default) | Yes, but read-only |
| `/dev/mmcblk2p7` | `/home/bticino/cfg/extra` | ext4 | **rw** (sync) | **Yes** — survives reboots |
| tmpfs | `/var/volatile` | tmpfs | rw | **No** — wiped on reboot |

To modify root filesystem: `mount -oremount,rw /` then `mount -oremount,ro /` after changes.

### 5.1 Key Paths

| Path | Description |
|------|-------------|
| `/home/bticino/cfg/extra/conf/conf.img` | Persistent config (ZIP with 1024-byte header, password: `bticino`) |
| `/home/bticino/cfg/extra/conf/conf_copy.img` | Backup copy |
| `/var/tmp/conf.zip` | Temporary extracted config zip |
| `/var/tmp/conf.xml` | Runtime configuration XML |
| `/home/bticino/cfg/extra/0/archive.xml` | Archive config |
| `/home/bticino/cfg/extra/0/layout.xml` | Layout config |
| `/home/bticino/cfg/extra/0/settings.xml` | Settings |
| `/home/bticino/cfg/extra/0/current_modality` | Current modality |
| `/home/bticino/cfg/extra/0/read-only-par.txt` | Read-only parameters |
| `/home/bticino/cfg/bt_mhportal` | bt_mhportal service config |
| `/var/tmp/bt_mhportal.log` | bt_mhportal log |
| `/etc/flexisip/users/users.db.txt` | SIP user database |
| `/etc/flexisip/domain-registration.conf` | SIP domain config |
| `/home/bticino/bt_mhportal/wacx/xml_bodies/notify_call_*.xml` | Call notification templates |
| `/etc/mosquitto/mosquitto.conf` | MQTT broker config: port 60000, bind 127.0.0.1 |
| `/home/bticino/cfg/stack_open.xml` | Full device config with all process settings, enable_mqtt=1 |

### 5.1b Key Shared Libraries

| Library | Path | Purpose |
|---------|------|---------|
| `libjel.so` | `/home/bticino/lib/` | MQTT client (wraps libmosquitto): init_mqtt, mqtt_publish, register_topic |
| `libjsonmsg.so` | `/home/bticino/lib/` | JSON-RPC protocol: jsonbuf_to_jsonmsg, generate_json_ack, generate_json_error |
| `libcommon.so.0` | `/home/bticino/lib/` | Common: create_socket_client, send_ID_toscsserver |
| `liblghal.so.0.0` | `/home/bticino/lib/` | HAL: hal_board, hal_board_reboot |
| `libutils.so` | `/home/bticino/lib/` | Utility functions |

### 5.2 conf.img Format

- ZIP file with a **custom 1024-byte header** prepended
- Password: **`bticino`**
- Extraction: `FUN_0001c6a8("/home/bticino/cfg/extra/conf/conf.img", 0, "/var/tmp/conf.zip")`
- Contains: `archive.xml`, `layout.xml`, `settings.xml`, `current_modality`, `read-only-par.txt`, `conf.xml`

### 5.3 Enabling SSH

Write `<abil_ssh>1</abil_ssh>` into `/var/tmp/conf.xml`

### 5.4 Triggering Config Re-upload

```bash
touch /var/tmp/conf.xml
# Forces: GUI re-read + HTTP PUT to cloud
```

### 5.5 Key Binaries

| Binary | PID | Purpose | MQTT Client ID |
|--------|-----|---------|----------------|
| `bt_daemon` | 494/640 | Main daemon, conf.img extraction | `9002` |
| `openserver` | 660 | OpenWebNet server on port 20000 | - |
| `bt_device` | 662 | Device management, lock/light control | `1001` |
| `bt_vct` | 663 | VCT (video communication terminal) | - |
| `bt_av_media` | 664 | Video/audio media daemon, GStreamer pipelines | `13242623` |
| `bt_answering_machine` | 665 | SIP/Linphone (broken - ports disabled) | `c300x` |
| `bt_mhportal` | 667 | Cloud communication (Eliot API) | - |
| `bt_dbus_manager` | 669 | D-Bus manager | - |
| `bt_ipcamera` | 670 | Netatmo camera integration | `1050` |
| `BtClass` | - | UI/button handling | - |

### 5.6 bt_mhportal Configuration

File: `/home/bticino/cfg/bt_mhportal`
```ini
[General Parameters]
Logfile=/var/tmp/bt_mhportal.log
LogLev=1
Logtype="file"
TLSCheck=1
OwnPort=20000
```

`LogLev=1` enables verbose logging (exposes cloud communication, passwords, device ID).

### 5.7 bt_mhportal Log Reveals

```
prj_name      : C3X
uri           : https://www.myhomeweb.com:443
deviceID      : 2193475
pswd          : 322461523
enc key       : (empty)
hash type     : 1
BASE_PATH     : eliot/gateway
SERVICE_PATH  : eliot/urlforservice
polling_timer : 600
```

---

## 6. Network & Firewall

### 6.1 Interfaces

- `usb0` - USB gadget network (Ethernet-over-USB)
- `wlan0` - WiFi

### 6.2 Port Map

| Port | Service | Access | Process |
|------|---------|--------|---------|
| 20 | FTP data | USB only | - |
| 21 | FTP control | USB only | - |
| 5060 | SIP (UDP/TCP) | Must be opened manually | flexisip |
| 5061 | SIP (TLS) | Open on WiFi | flexisip |
| 11111 | Unknown | USB only | - |
| 20000 | OpenWebNet main | WiFi accessible | openserver (660) |
| 20001 | OpenWebNet secondary | Local only | openserver |
| 30006 | bt_av_media config | Local only | bt_av_media (664) |
| 30007 | bt_av_media video | Local only | bt_av_media (664) |
| 30013 | bt_device | Local only | bt_device (662) |
| 31007 | bt_av_media secondary | Local only | bt_av_media (664) |
| 40000 | Unknown | All (UID 1000) | bticino user |
| 40005 | Unknown | All (UID 1000) | bticino user |
| 60000 | **Mosquitto MQTT broker** | **Local only (127.0.0.1)** | mosquitto (467) |

Multicast: `239.255.76.67:7667` (UDP) — OpenWebNet events between devices

### 6.3 Firewall Rules

```bash
for i in 20 21 11111; do
    iptables -A INPUT -i usb0 -p tcp -m tcp --dport $i -j ACCEPT
    iptables -A INPUT -i wlan0 -p tcp -m tcp --dport $i -j DROP
done
```

### 6.4 USB Connection

Connecting USB creates an Ethernet-type adapter (USB gadget network). FTP may auto-activate.
There is also a code like `*xx#yyy` to activate FTP remotely.

---

## 7. Cloud API (Eliot Platform)

### 7.1 Endpoints

| Environment | Base URL |
|-------------|----------|
| Production | `https://api.developer.legrand.com` |
| Staging | `https://api.pp-developer.legrand.com` |
| Stable/QA | `https://eliotqa.azure-api.net` |

Cloud config: `https://www.myhomeweb.com:443`

### 7.2 OAuth2 Authentication

Token endpoint: `POST https://login.microsoftonline.com/199686b5-bef4-4960-8786-7a6b1888fee3/oauth2/token`

| Environment | Client ID | Client Secret | Subscription Key |
|-------------|-----------|---------------|------------------|
| Production | `b0c8a85c-54a0-4a94-a872-e0020260f714` | `2klEjK4Pn4GgvR2ElmSCaglVGtkpxlltH0rs4QiEQw8=` | `3c48aa3992754ac28dab36742d5b151a` |
| Staging | `75c0d6e6-66e8-41ef-81f5-2fdc8a73026e` | `qsudGLMSUtgtMyFOmZSXMcr4HdzFImOuyOCUg27Eo7E=` | `f225719b833a446a9ce92480e02a335f` |
| Stable | `b246f402-99ae-4dd8-95bc-cea081c0e1b8` | `X2rghgFT/sxDRjXa6fzfNMFPpcs1IT1XKsNFPrznBhI=` | `59048b04e4c44a83b3d0dda177c174ee` |

### 7.3 Cloud ZIP (mhpg.zip)

- Password: **`mhpG_123!`** (found in `bt_mhportal` binary)
- Contents: `archive.xml`, `layout.xml`, `settings.xml`, `current_modality`, `read-only-par.txt`, `conf.xml`
- On restart, config uploaded via HTTP PUT to cloud
- Script: `download_ZIP_config_file_requests.py` automates download

### 7.4 API URLs

| URL Pattern | Purpose |
|-------------|---------|
| `eliot/gateway/...` | Gateway API base |
| `eliot/urlforservice/...` | Service URL discovery |
| `eliot/sip/users/plants/<plantid>/gateway/<GatewayId>` | SIP credentials |
| `https://www.config.myhomeweb.it/IndexITA.html` | Professional configuration tool |

---

## 8. Firmware Update Protocol

### 8.1 Commands (WHO=130)

| Command | Purpose |
|---------|---------|
| `*#130**1##` | Query firmware update status |
| `*130*1##` | Start firmware download |
| `*130*2##` | Apply/install downloaded firmware |

### 8.2 Status Codes

| Status | Meaning | Additional Data |
|--------|---------|-----------------|
| 1 | Up-to-date | `split[5]`=recovery, `split[6.7.8]`=version |
| 2 | Update available | Triggers download button |
| 3 | Downloading | `split[8]`=progress % |
| 4 | Downloaded / ready | Triggers install button |
| 5 | Updating/installing | `split[5.6.7]`=new version |
| 6 | Update complete | `split[5.6.7]`=new version |
| 7-9 | Error | Various error states |

### 8.3 Polling

- Status polled every **10 seconds** via `Handler.postDelayed`
- Each poll: full TCP + auth + query flow

---

## 9. Security Findings

### 9.1 Attack Surface Summary

1. **OpenWebNet password**: numeric-only (1-999999999), brute-forceable offline in ~4 minutes
2. **conf.img ZIP password**: `bticino` (hardcoded in `bt_daemon`)
3. **Cloud ZIP password**: `mhpG_123!` (hardcoded in `bt_mhportal`)
4. **Cloud API**: exposes SIP credentials with only app login (email/password)
5. **SIP MD5 hashes**: stored in plaintext on device (`users.db.txt`)
6. **SSH enable**: write `<abil_ssh>1</abil_ssh>` to `conf.xml`
7. **FTP/Telnet**: accessible over USB without authentication
8. **Config re-upload**: `touch /var/tmp/conf.xml` triggers cloud sync
9. **SIP certificates**: obtainable by sending CSR to cloud
10. **MQTT broker on port 60000**: no authentication, allows full IPC control
    - Can activate/deactivate video pipeline via `camera.startLiveStream`
    - Can control locks via `lock.setStatus`
    - Can control lights via `light.setStatus`
    - Can control WiFi via `gateway.setWifi*`
    - Can interact with Netatmo cameras via `netatmo.*`

### 9.2 Reverse Engineering Tools Used

```bash
# Trace OpenWebNet traffic
strace -p $(pidof openserver) -e trace=network -s 4096

# Trace button presses
strace -p $(pidof BtClass) -e trace=network -s 4096
```

### 9.3 HAL Library (`liblghal.so.0.0`)

Path: `/home/bticino/lib/liblghal.so.0.0`

Exported symbols:
- `_Z9hal_boardv` - returns board ID (returned `12`)
- `_Z16hal_board_rebootv` - triggers device reboot

Loading:
```bash
LD_PRELOAD="/lib/symbols.so:/usr/lib/libglib-2.0.so.0.5200.3" ./test
LD_PRELOAD="/lib/symbols.so:/usr/lib/libglib-2.0.so.0.5200.3:/home/bticino/lib/libutils.so" ./test
```

---

## 10. Android App Internals

### 10.1 Package

`com.legrandgroup.c300x` (Door Entry app)

### 10.2 SIP Stack

Uses **Linphone** (open-source SIP library):
- `VctLinphoneService` handles SIP registration
- Push notifications trigger SIP REGISTER refresh

### 10.3 Push Notification IDs (FCM)

| Message ID | Action |
|------------|--------|
| `FWAVAIL` | Firmware upgrade available |
| `NEWASWMMSG` | New ASWM message |
| `UNTIEUSER` | Force logout (all devices) |
| `UNTIEUSIP` | Force logout (specific device) |
| `PLANTCHG` | Plant config changed -> full alignment |

### 10.4 Intent Actions

| Action | Purpose |
|--------|---------|
| `com.legrandgroup.c300x.linphone.MANAGE_PUSH_NOTIF` | Linphone SIP re-registration |
| `com.legrandgroup.c300x.PROCEDURE_FULL_ALIGNMENT` | Full plant alignment sync |
| `com.legrandgroup.c300x.PROCEDURE_LOGOUT` | Forced logout |
| `com.legrandgroup.c300x.net.FWUPGRADE` | Firmware status broadcast |
| `com.legrandgroup.c300x.net.FWERROR` | Firmware error broadcast |

### 10.5 SIP Domain Pattern

`c300x@<gateway_id>.bs.iotleg.com`

---

## 11. Hardware

### 11.1 Firmware Versions

- 1.7.17 (user's version, version file: `20190517144801`)
- 1.7.19 (latest known)

### 11.2 Device Model Identification

- Model ID `68` = Classe 300X (C300X)
- Any other model ID is rejected

### 11.3 Video Devices

- `/dev/video0` - Primary video capture
- `/dev/video2` - Alternative video capture

### 11.4 System Details

| Component | Details |
|-----------|---------|
| Architecture | ARM 32-bit (GOARCH=arm, GOARM=7) |
| Network | wlan0, IP 192.168.1.38 |
| OpenSSL | 1.0.2d |
| Python | 2.7.14 and 3.5.3 (no f-strings) |
| strace | Available, but ptrace blocked on bt_* processes |
| ltrace | NOT available |
| tcpdump | `/usr/sbin/tcpdump` |
| mosquitto_pub/sub | `/usr/bin/mosquitto_pub`, `/usr/bin/mosquitto_sub` |

---

## 12. Internal MQTT Bus (Port 60000)

### 12.1 Discovery

Port 60000 on the device is a **standard Mosquitto MQTT broker** — NOT a custom bt_daemon
protocol. This is the central inter-process communication (IPC) bus for all bt_* daemons.

### 12.2 Mosquitto Configuration

File: `/etc/mosquitto/mosquitto.conf`
```ini
bind_address 127.0.0.1
port 60000
# Protocol: MQTT 3.1.1
# Authentication: none (allow_anonymous true by default)
```

- PID 467, separate from bt_daemon
- `mosquitto_pub` and `mosquitto_sub` available at `/usr/bin/`
- No TLS, no authentication required

### 12.3 Connected Clients and Topics

| Client ID | Source Port | Process (PID) | Subscribed Topics |
|-----------|------------|---------------|-------------------|
| `1050` | 44537 | bt_ipcamera (670) | netatmo.getUri, netatmo.getStatus, netatmo.setStatus, netatmo.setLogin, netatmo.getCameras, netatmo.setPresenceHome, MONITOR |
| `9002` | 59837 | bt_daemon child | gateway.getWifiList, gateway.setWifiNetwork, gateway.setWifi, gateway.getNetworkStatus, gateway.setSoftAp, gateway.clearNetwork, gateway.onNetworkSlow, MONITOR |
| `13242623` | 46943 | bt_av_media (664) | MONITOR, **camera.startLiveStream**, **camera.stopLiveStream** |
| `c300x` | 53027 | bt_answering_machine (665) | MONITOR |
| `1001` | 46565 | bt_device (662) | MONITOR, lock.setStatus, light.setStatus, gateway.setPrecommissioning, gateway.getNetworkStatus |

### 12.4 MQTT Connection Map (from `/proc/net/tcp`)

All connections are ESTABLISHED to `127.0.0.1:60000`:
```
Port 44537 (0xADF9) → 60000: inode 5082 = bt_ipcamera (client "1050")
Port 59837 (0xE9BD) → 60000: inode 5094 = bt_daemon/9002
Port 46943 (0xB75F) → 60000: inode 5115 = bt_av_media (client "13242623") ← FD 9 of PID 664
Port 53027 (0xCF23) → 60000: inode 5181 = bt_answering_machine (client "c300x")
Port 46565 (0xB5E5) → 60000: inode 5326 = bt_device (client "1001")
```

### 12.5 Message Protocol: JSON-RPC 2.0 over MQTT

The message format is JSON-RPC 2.0 with the following request/response pattern:

**Request flow:**
1. Client subscribes to `<module>.<method>.<request_id>` (response topic)
2. Client publishes to `<module>.<method>` with payload:
   ```json
   {"jsonrpc":"2.0","method":"<module>.<method>","id":"<uuid>","params":[...]}
   ```
3. Server processes request and publishes result to `<module>.<method>.<request_id>`

**Response payload (success):**
```json
{"jsonrpc":"2.0","id":"<uuid>","result":{...}}
```

**Response payload (error):**
```json
{"jsonrpc":"2.0","id":"<uuid>","error":{"code":<int>,"message":"<string>"}}
```

**Broadcast/Notification:**
Published to `MONITOR` topic, no `id` field:
```json
{"jsonrpc":"2.0","method":"<module>.<event>","params":[...]}
```

Topic format pattern in libjel.so: `%s.%s.%s` (module.method.id)

### 12.6 Confirmed bt_av_media MQTT Responsiveness

We successfully sent messages to `camera.startLiveStream` and got bt_av_media to respond:
```bash
# Subscribe to response topic first
mosquitto_sub -h 127.0.0.1 -p 60000 -t "camera.startLiveStream.test-123" &

# Publish request
mosquitto_pub -h 127.0.0.1 -p 60000 -q 1 -t "camera.startLiveStream" \
  -m '{"jsonrpc":"2.0","method":"camera.startLiveStream","id":"test-123","params":[...]}'
```

Response received:
```json
{"jsonrpc":"2.0","id":"test-123","error":{"code":-32700,"message":"Parsing JSON error"}}
```

bt_av_media IS alive, connected to MQTT, processing our messages, and responding. The error
is because our `params` structure doesn't match what `startLiveStream` expects (see Section 13).

---

## 13. ARM Disassembly: bt_av_media

### 13.1 Binary Details

- Path: `/home/bticino/bin/bt_av_media` (device), `/tmp/bt_av_media` (local copy)
- MD5: `fd5c19d90fb430f9fe0817f788add872`
- Architecture: ARM 32-bit
- PID on device: 664

### 13.2 MQTT Initialization Sequence (0x14b04-0x14c14)

```
0x14b04: init_mqtt(0, userdata)                                    ; connect to MQTT broker
0x14b3c: mqtt_register_debug(mqtt_handle, debug_func)              ; register debug logger
0x14bec: mqtt_register_subscribe_callback(mqtt_handle, 0x2b1c0)   ; register global msg handler
0x14bf8: register_topic(mqtt_handle, "MONITOR")                    ; one-shot topic
0x14c04: register_topic(mqtt_handle, "camera.startLiveStream")     ; one-shot topic
0x14c10: register_topic(mqtt_handle, "camera.stopLiveStream")      ; one-shot topic
0x14b80: g_main_loop_run()                                         ; enter main event loop
```

### 13.3 Global Subscribe Callback (0x2b1c0)

This is the main message dispatcher, registered via `mqtt_register_subscribe_callback`:

```
0x2b1c0: entry
         1. jsonmsg_to_jsonbuf(jsonmsg)        → serialize back to string for logging
         2. log "received json: <string>"
         3. Check message type:
            - is_json_ack()      → log and ignore
            - is_json_error()    → log and ignore
            - is_json_response() → log and ignore
            - is_json_command()  → call command handler at 0x2aed4
            - is_json_notify()   → log "received notification"
            - otherwise          → log "Unknown json message"
```

### 13.4 Command Handler / Router (0x2aed4)

Routes incoming JSON-RPC commands to handler functions by hashing the method name:

```
0x2aed4: entry
         1. Extract module + method from jsonmsg → create "%s.%s" string
         2. Compute hash = hash_str(method_string)
         3. Compare against known hashes:
            ┌─────────────────────────────────────────────────────────────┐
            │ Hash 0x75513706 → startLiveStream handler at 0x2a97c       │
            │ Hash 0xda60db9e → stopLiveStream handler at 0x2afc4        │
            │ Unknown hash    → generate_json_error(-32601, "Method not  │
            │                    found")                                  │
            └─────────────────────────────────────────────────────────────┘
         4. If handler returns 0 → generate_json_ack, publish to response topic
         5. If handler returns non-zero → generate_json_error with handler's
            return code as error code
```

### 13.5 startLiveStream Handler (0x2a97c) — CRITICAL

This function extracts params from the jsonmsg in a specific order. Each `get_val_*_node` call
operates on the params node from the parsed JSON-RPC message:

```
Param extraction order:
  1. uri      = get_val_string_node(params, "uri")       ← REQUIRED (NULL → return -32700)
  2. type     = get_val_string_node(params, "type")      ← REQUIRED (NULL → return -32700)
  3. sink     = get_val_string_node(params, "sink")      ← REQUIRED (NULL → return -32700)
  4. video    = get_val_bool_node(params, "video")       ← boolean flag
  5. audioEnc = get_val_string_node(params, "audioEnc")  ← audio encoder type
  6. audio    = get_val_bool_node(params, "audio")       ← boolean flag
  7. If audio==true: ipDest = get_val_string_node(params, "ipDest")

Conditional extraction based on sink and video values:
  If sink == "udpsink":
    ipDest = get_val_string_node(params, "ipDest")       ← destination IP

  If video == true:
    videoPort = get_val_string_node(params, "videoPort")  ← STRING, not int!
    width     = get_val_int_node(params, "width")
    heigth    = get_val_int_node(params, "heigth")        ← TYPO in binary! Not "height"!
    bitrate   = get_val_int_node(params, "bitrate")
    fps       = get_val_int_node(params, "fps")

  audioPort = get_val_string_node(params, "audioPort")

String comparisons performed:
  - sink against "udpsink" and "lcdsink"
  - type against "h264" and "hls"
  - audioEnc against "h264" (likely also supports "speex"?)

On success:
  - Calls virtual method at vtable offset 0x34 (create_pipeline)
  - Sets GStreamer state to PLAYING
  - Generates JSON ACK
  - Publishes notification to MONITOR topic

On param not found (any required field is NULL):
  - Returns -32700 (0xffff8044) = "Parsing JSON error"
```

**KEY OBSERVATIONS:**
- `videoPort` is extracted with `get_val_string_node` (STRING, not integer)
- `heigth` has a TYPO in the binary — NOT "height"
- `video` and `audio` are boolean fields extracted with `get_val_bool_node`
- The error code `-32700` is the handler's RETURN VALUE when a required param is missing
- This return value is passed directly to `generate_json_error` by the command router

### 13.6 stopLiveStream Handler (0x2afc4)

```
0x2afc4: entry
         1. Call vtable[0x38] method with arg=1  (destroy_pipeline)
         2. Call lcd sink function               (restore LCD display)
         3. Clear video framebuffer              (blank screen)
         4. Generate ACK response
         5. Publish notification to MONITOR topic
```

### 13.7 GStreamer Pipeline Details

Key strings found in bt_av_media binary:
```
"Could add client: no Gstreamer pipeline"       ← NACK reason for *7*300 commands
"create_pipeline_video_stream_udpsink"           ← function that creates video pipeline
"IP ADDRESS %s, PORT %d"                         ← pipeline uses IP/port parameters
"No pipeline with multiudpsink elements"         ← pipeline uses GStreamer multiudpsink
"Recording IU vid: %dx%d@25 aswm: %dbps         ← format string showing video params
 rtp hr: %dbps@%dfps rtp lr: %dx%d-%dbps@%dfps"
```

### 13.8 Key Addresses Reference

```
0x14b04  - init_mqtt() call
0x14bec  - mqtt_register_subscribe_callback(handle, 0x2b1c0)
0x14bf8  - register_topic(handle, "MONITOR")
0x14c04  - register_topic(handle, "camera.startLiveStream")
0x14c10  - register_topic(handle, "camera.stopLiveStream")
0x14b80  - g_main_loop_run()
0x2b1c0  - Global MQTT subscribe callback (message dispatcher)
0x2aed4  - Command handler / router (hash-based dispatch)
0x2a97c  - startLiveStream handler (extracts params, creates pipeline)
0x2afc4  - stopLiveStream handler (destroys pipeline, sends ACK)
0x2b070  - Hash constant for "camera.startLiveStream" = 0x75513706
0x2b074  - Hash constant for "camera.stopLiveStream" = 0xda60db9e
0x2b07c  - Error code constant 0xffff80a7 = -32601 ("Method not found")
0x2ae64  - Error code constant 0xffff8044 = -32700 ("Parsing JSON error")
```

---

## 14. ARM Disassembly: libjel.so (MQTT Client Library)

### 14.1 Library Details

- Path: `/home/bticino/lib/libjel.so` (device), `/tmp/libjel.so` (local copy)
- Dependencies: libmosquitto.so.1

### 14.2 Exported Functions

| Function | Purpose |
|----------|---------|
| `init_mqtt` | Initialize MQTT client, connect to broker |
| `destroy_mqtt` | Disconnect and cleanup |
| `mqtt_publish` | Publish JSON-RPC message to topic |
| `mqtt_publish_raw` | Publish raw bytes to topic |
| `register_topic` | Register one-shot topic handler |
| `unregister_topic` | Remove topic handler |
| `mqtt_register_subscribe_callback` | Register global JSON callback (type 1) |
| `mqtt_register_raw_subscribe_callback` | Register global raw callback (type 0) |

### 14.3 Message Processing: `my_message_callback` (0x2088)

This is the `on_message` callback registered with libmosquitto. It implements a ONE-SHOT
topic matching system:

```
0x2088: my_message_callback(mosquitto*, userdata, mosquitto_message*)
        1. Iterate through registered topic list
        2. For each registered topic:
           - Compare incoming topic string against registered topic
           - If MATCH found:
             a. REMOVE topic from list (g_list_remove at 0x2110) ← ONE-SHOT!
             b. Check topic->type (offset 24):
                - Type 0 (raw):  call topic->callback(userdata, &payload_string)
                - Type 1 (JSON): call jsonbuf_to_jsonmsg(payload)
                                  then topic->callback(userdata, &jsonmsg)
             c. Return (stop iterating)
        3. If NO topic match found → fall through to global callback:
           - Access global callback at offset 0x30 of mqtt_data struct
           - Call jsonbuf_to_jsonmsg(payload)
           - Call global_callback(userdata, &jsonmsg)
```

### 14.4 CRITICAL: Topics are ONE-SHOT

After the FIRST message on a registered topic (e.g., `camera.startLiveStream`), the topic
registration is **consumed and removed** from the list via `g_list_remove`. All subsequent
messages on the same topic fall through to the **global subscribe callback** (step 3).

Both paths (one-shot and global) call `jsonbuf_to_jsonmsg` to parse the payload, then invoke
the registered callback with the parsed jsonmsg struct. The behavior is functionally identical
for well-formed messages.

### 14.5 Key Addresses Reference

```
0x2088  - my_message_callback (mosquitto on_message handler)
0x2110  - g_list_remove (removes matched topic — ONE-SHOT behavior)
0x21f0  - jsonbuf_to_jsonmsg call for global callback path
0x2248  - jsonbuf_to_jsonmsg call for matched topic path (type=1)
0x220c  - blx r3 (call to registered subscribe callback)
0x22b4  - Raw callback path (type=0)
```

---

## 15. ARM Disassembly: libjsonmsg.so (JSON-RPC Library)

### 15.1 Library Details

- Path: `/home/bticino/lib/libjsonmsg.so` (device), `/tmp/libjsonmsg.so` (local copy)
- Dependencies: libjson-glib-1.0.so.0

### 15.2 Exported Functions

| Function | Purpose |
|----------|---------|
| `new_json_msg` | Allocate new jsonmsg struct |
| `free_json_msg` | Free jsonmsg struct |
| `jsonbuf_to_jsonmsg` | Parse JSON string → jsonmsg struct |
| `jsonmsg_to_jsonbuf` | Serialize jsonmsg struct → JSON string |
| `generate_json_ack` | Build JSON-RPC ACK response |
| `generate_json_error` | Build JSON-RPC error response |
| `is_json_command` | Check if jsonmsg is a command (has "method") |
| `is_json_response` | Check if jsonmsg is a response (has "result") |
| `is_json_ack` | Check if jsonmsg is an ACK |
| `is_json_error` | Check if jsonmsg is an error (has "error") |
| `is_json_notify` | Check if jsonmsg is a notification (method, no id) |
| `get_val_string_node` | Extract string value from jsonmsg node by key |
| `get_val_int_node` | Extract integer value from jsonmsg node by key |
| `get_val_bool_node` | Extract boolean value from jsonmsg node by key |

### 15.3 jsonmsg Struct Layout

```
Offset  Field
0x00    (unknown/flags)
0x04    module (string, e.g., "camera")
0x08    method (string, e.g., "startLiveStream")
0x0C    id (string, request ID)
0x10    params (pointer to params node tree)  ← used by get_val_*_node
0x14+   (additional fields)
```

The params field (offset 0x10 / offset 16) is set by `jsonbuf_to_jsonmsg` at address `0x3274`.

### 15.4 JSON Parser

`jsonbuf_to_jsonmsg` at `0x31e8` uses json-glib's `json_parser_load_from_data` (standard
json-glib SAX parser). It creates an internal node tree from the parsed JSON. The `params`
field in the jsonmsg struct points to this tree.

Format strings found:
```
%s"jsonrpc" : "%s"        ← Note spaces around colon when generating
"method" : "%s.%s"        ← module.method format
```

### 15.5 The Params Node Structure Problem

The `get_val_string_node(params, "key")` function traverses the params node tree looking for
a key. The critical question is how `jsonbuf_to_jsonmsg` maps different JSON structures:

- `"params":[{"uri":"..."}]` → params points to ARRAY node. `get_val_string_node(array, "uri")`
  may fail because "uri" is inside the array's first element, not a direct member.
- `"params":{"uri":"..."}` → params points to OBJECT node. `get_val_string_node(object, "uri")`
  should find "uri" as a direct member.

This difference in node tree structure is the most likely cause of the -32700 errors
(see Section 16).

---

## 16. The Pipeline Activation Problem

### 16.1 The Core Issue

OpenWebNet `*7*300` commands on port 30007 return NACK (`*#*0##`) because bt_av_media has no
active GStreamer pipeline. The pipeline is normally activated by bt_answering_machine sending
a `camera.startLiveStream` message over MQTT when it processes a SIP INVITE. But
bt_answering_machine's SIP stack is broken (see 16.2), so the pipeline is never started.

### 16.2 Why bt_answering_machine's SIP is Broken

From Linphone debug logs, bt_answering_machine on boot:
1. Creates listening points on ports 5070/5080/5090
2. Gets `bctbx_list_nth_data: no such index in list` error
3. Destroys all listening points
4. Creates new listening points on port `-2` (`LC_SIP_TRANSPORT_DISABLED`)

This happens because bt_answering_machine can't find a valid TLS certificate
(`aswm_truststore.p12`). Without SIP, it never processes INVITEs and never activates
bt_av_media's pipeline via MQTT.

### 16.3 Solution: Direct MQTT Pipeline Activation

Since we can't fix bt_answering_machine's SIP, we bypass it entirely by sending
`camera.startLiveStream` messages directly to bt_av_media over MQTT port 60000.

### 16.4 Current Blocker: -32700 Error

We successfully established MQTT communication with bt_av_media and received responses.
However, ALL our attempts return:
```json
{"jsonrpc":"2.0","id":"...","error":{"code":-32700,"message":"Parsing JSON error"}}
```

### 16.5 Error Source Analysis (from ARM disassembly)

The error flow is:
```
1. startLiveStream handler (0x2a97c) calls:
   get_val_string_node(jsonmsg->params, "uri")

2. If uri == NULL (not found in params node tree):
   → handler returns -32700 (0xffff8044)

3. Command router (0x2aed4) at 0x2b040:
   subs r2, r0, #0          ; r2 = handler return value
   beq 0x2b004              ; if 0 → success path (ACK)
   b 0x2af48                ; if non-zero → error path

4. Error path at 0x2af48:
   generate_json_error(output_jsonmsg, request_id, r2)
   ; r2 still holds -32700 from handler

5. Error response published to response topic
```

The -32700 error means `get_val_string_node` cannot find "uri" in the params node tree.
This is NOT a JSON parsing error — the JSON is parsed correctly. The error name is misleading;
it's reusing the JSON-RPC error code -32700 for "required parameter not found."

### 16.6 Hypothesis: Params Object vs Array

JSON-RPC 2.0 allows `params` to be either an array or an object:
- Array: `"params":[{"uri":"/live",...}]`
- Object: `"params":{"uri":"/live",...}`

If `jsonbuf_to_jsonmsg` stores the params node as-is from the JSON, then:
- With array form: `get_val_string_node(array_node, "uri")` fails (uri is nested inside
  the array's first element)
- With object form: `get_val_string_node(object_node, "uri")` succeeds (uri is a direct
  member of the object)

**We have NOT yet tested the object form.** All our previous attempts used the array form
`"params":[{...}]`.

### 16.7 Next Experiment: Object Params Format

The next test to run on the device:
```bash
# Subscribe to response topic
mosquitto_sub -h 127.0.0.1 -p 60000 -t "camera.startLiveStream.test-500" &

# Try OBJECT params (not wrapped in array)
mosquitto_pub -h 127.0.0.1 -p 60000 -q 1 -t "camera.startLiveStream" \
  -m '{"jsonrpc":"2.0","method":"camera.startLiveStream","id":"test-500","params":{"uri":"/live","type":"h264","sink":"udpsink","video":true,"audio":true,"audioEnc":"speex","ipDest":"127.0.0.1","videoPort":"10002","audioPort":"10000","width":800,"heigth":480,"bitrate":1024,"fps":25}}'
```

**NOTE:** Due to libjel.so's one-shot topic mechanism, after the first message,
`camera.startLiveStream` is removed from the registered topics list. Subsequent messages
go through the global subscribe callback. Both paths produce the same result, but a device
**reboot** may be needed to reset the one-shot registration for clean testing.

### 16.8 Alternative Experiment: Physical Doorbell Press Capture

If a physical doorbell is pressed, bt_answering_machine (or another process) should send
the REAL `camera.startLiveStream` message over MQTT. Capturing this would reveal the
exact message format:

```bash
# On device: subscribe to ALL MQTT topics
mosquitto_sub -h 127.0.0.1 -p 60000 -t '#' -v > /tmp/mqtt_capture.txt &

# Press physical doorbell

# Stop capture
kill %1

# Retrieve capture
scp bticino:/tmp/mqtt_capture.txt .
```

Alternatively, use tcpdump for binary-level capture:
```bash
tcpdump -i lo -w /tmp/doorbell_mqtt.pcap port 60000
```

### 16.9 Alternative: Deeper Disassembly of get_val_string_node

If both object and array params formats fail, we need to disassemble `get_val_string_node`
in libjsonmsg.so to understand exactly how it traverses the node tree. This would reveal
whether the function supports nested lookups or only flat object lookups.

### 16.10 The Complete Expected Flow (Once Pipeline Activates)

```
1. bticino_bridge publishes camera.startLiveStream via MQTT (port 60000)
2. bt_av_media creates GStreamer pipeline (udpsink)
3. bt_av_media returns ACK via MQTT
4. bticino_bridge sends *7*300#<IP>#<PORT>#0*## to port 30007 (video)
5. bt_av_media adds client to multiudpsink → sends plain RTP to <IP>:<PORT>
6. bticino_bridge sends *7*300#<IP>#<PORT>#2*## to port 30007 (audio)
7. bt_av_media adds audio client → sends plain RTP
8. go2rtc receives RTP → converts to WebRTC
9. Web dashboard shows video
```

---

## 17. Current Implementation Status

### 17.1 Working

- bticino_bridge v0.15.5 running on device (PID 765)
- `.bridge_autostart` enabled — survives reboots via watchdog in bt_daemon-apps.sh
- SIP registration via TCP (transport=tcp, username=webrtc, sip_target=c300x)
- RTSP server on port 6554 (responds to DESCRIBE with proper SDP)
- RTSP TCP/interleaved transport handling
- RTSP SETUP -> ensureSIPCallActive -> MakeCall chain
- go2rtc on ports 1984 (API), 8555 (WebRTC)
- ffmpeg at `/usr/local/bin/ffmpeg`
- OpenWebNet client connects to port 30007 (no initial ACK wait)
- MQTT broker discovered and fully characterized (port 60000)
- bt_av_media MQTT communication established (receives messages, sends responses)
- Full ARM disassembly of bt_av_media MQTT handler chain completed
- All required startLiveStream params decoded from binary

### 17.2 Blocked

- **bt_av_media pipeline not activating**: `camera.startLiveStream` returns -32700 error
  because params structure doesn't match what `get_val_string_node` expects
- **Port 30007 NACKs**: GStreamer pipeline not active, so `*7*300` commands fail
- **bt_answering_machine SIP broken**: ports disabled due to missing TLS cert

### 17.3 Next Steps (Priority Order)

1. **Try object params format** for `camera.startLiveStream` (see Section 16.7)
2. **Capture MQTT during physical doorbell press** to see real message format (see Section 16.8)
3. **Disassemble `get_val_string_node`** in libjsonmsg.so if format still fails (see Section 16.9)
4. **Implement MQTT client** in bticino_bridge once correct format is found
5. **Test full pipeline**: MQTT activate → port 30007 ACK → RTP flows → go2rtc → WebRTC
6. **Clean up**: Remove tcpdump line from `/etc/init.d/bt_daemon-apps.sh`

### 17.4 Files Modified

| File | Changes |
|------|---------|
| `cmd/main.go` | SIPTarget mapping, SetOpenWebNetClient wiring |
| `configs/config.yaml` | v0.15.5, SIP settings (username=webrtc, transport=tcp, sip_target=c300x) |
| `pkg/config/config.go` | Added SIPTarget field to SIPConfig struct |
| `pkg/sip/client.go` | SIPTarget field, buildInviteRequest, buildSDP, buildRegisterRequest |
| `pkg/sip/rtsp_server_enhanced.go` | ownClient field, SetOpenWebNetClient(), activateMediaStreams() |
| `pkg/sip/rtsp.go` | Fixed ensureSIPCallActive to use SIPTarget |
| `pkg/openwebnet/client.go` | Fixed sendToVideoPortWithRetry() — no initial ACK wait for port 30007 |

### 17.5 Device Modifications

| File | Changes |
|------|---------|
| `/etc/init.d/bt_daemon-apps.sh` | Hosts entries, bticino_bridge autostart with watchdog. **HAS TEMPORARY tcpdump line** |
| `/etc/linphone.conf` | disable_portal=1, proxy settings |
| `/etc/flexisip/users/users.db.txt` | Added webrtc + c300x users |
| `/etc/flexisip/users/route.conf` | Added webrtc, c300x static routes |
| `/etc/flexisip/users/route_int.conf` | Added webrtc to alluser line |

### 17.6 Local Analysis Files

| File | Description |
|------|-------------|
| `/tmp/bt_av_media` | ARM ELF, fully disassembled (MD5: fd5c19d90fb430f9fe0817f788add872) |
| `/tmp/libjel.so` | MQTT client library, fully disassembled |
| `/tmp/libjsonmsg.so` | JSON-RPC protocol library |
| `/tmp/libcommon.so.0.0` | Common utility library |
| `/tmp/boot_pcap_full.txt` | Full text dump of boot pcap (732 lines, fully analyzed) |

---

## Appendix A: External Resources

- slyoldfox c300x-controller: `_herramientas_externas/slyoldfox/c300x-controller-main/`
- HackMD guides:
  - https://hackmd.io/@slyoldfox/SyJUxcWza (native C function calling)
  - https://hackmd.io/jbbhjTr7QYCTimeayxNmcQ (OpenWebNet password cracking)
  - https://hackmd.io/2ZnjOdwwT3O20CJM1aca0A (research notes)
- HA integration: https://github.com/fquinto/bticinoClasse300x/tree/main/ha_config
- openwebnet4j: `BUSConnector.java#L364` (OWN auth reference)

## Appendix B: Decompiled Android App Files

Location: `_investigacion/hack/`

| File | Content |
|------|---------|
| `sip_commands.java` | OpenWebNet auth + firmware update (FwUpdate class) |
| `sip_commands2.java` | Firmware update UI state machine |
| `sip_commands3.java` | Network utilities + device discovery (NetworkUtils) |
| `cuentas_VOIP.md` | VoIP account credentials |
| `datos_device_sipphone.md` | SIP phone parameters |
| `llamadas.md` | Call flow documentation |
| `flows*` | Protocol flow captures |
| `mqtt_investigation.md` | MQTT analysis |
| `trazas*.pcapng` | Wireshark captures (10 files) |
| `abrir_puerta*` | Door-open command captures |
| `fridascript.js`, `pinning.js`, `sslpinning.js` | Frida SSL pinning bypass |
| `api.md` | API documentation |
| `dtos.md` | DTO descriptions |

## Appendix C: Deployment Procedures

### Cross-Compile

```bash
cd bticino_bridge/
GOOS=linux GOARCH=arm GOARM=7 go build -o /tmp/bticino_bridge ./cmd/main.go
```

### Deploy to Device

```bash
# Transfer via base64 chunks (SSH key auth)
base64 /tmp/bticino_bridge | ssh bticino "base64 -d > /home/bticino/cfg/extra/bticino_bridge_new && chmod +x /home/bticino/cfg/extra/bticino_bridge_new"

# On device: replace binary (must kill first, rm, then cp)
ssh bticino 'rm -f /home/bticino/cfg/extra/.bridge_autostart && pkill -9 -f bticino_bridge; sleep 2; rm -f /home/bticino/cfg/extra/bticino_bridge && cp /home/bticino/cfg/extra/bticino_bridge_new /home/bticino/cfg/extra/bticino_bridge && chmod +x /home/bticino/cfg/extra/bticino_bridge'
```

### Start Application

```bash
ssh bticino 'touch /home/bticino/cfg/extra/.bridge_autostart && cd /home/bticino/cfg/extra && nohup ./bticino_bridge > /tmp/bticino_test.log 2>&1 &'
```

### Modify Read-Only Filesystem

```bash
ssh bticino 'mount -oremount,rw / && <make changes> && mount -oremount,ro /'
```

### Force Reboot (standard reboot often fails)

```bash
ssh bticino 'sync; echo b > /proc/sysrq-trigger'
```

### Important Notes

- The app loads config from `configs/config.yaml` (relative to CWD), NOT from absolute path
- Direct `mv` doesn't always work for binary replacement — use `rm` + `cp`
- Must remove `.bridge_autostart` first to prevent watchdog loop during replacement
- `reboot` command often fails silently — use `sysrq-trigger` instead
