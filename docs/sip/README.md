# Information related to SIP

From: https://gist.github.com/slyoldfox/822bc31447f800f074448055f5848169

# SIP communication between c300x and baresip

The following document will help to explain the different components that are used to setup
a two-way audio and video call to the intercom to hear/talk and view the external camera.

Thank you @R for being my monkey and test things a lot.

## Todo/Not working yet

* Receiving calls from the intercom seems broken on `baresip` 2.9.0 (although working on 0.5.10)
* When the `bt_answering_machine` closes the streams, the SIP call remains active and `baresip` starts acting weird  
* There is no way yet to passthrough the video in baresip to somewhere else (streaming to somewhere or record an avi/mp4)

## Communication setup

As investigated on https://hackmd.io/WnStgx-UTdCbFrBq4XfkCA the communication focuses on SIP communication.
It was seen that in order to setup calls and activate the camera, the easiest way was to focus on setting up a call
instead of focussing all components on the c300x via seperate commands on seperate services.

SIP communication happens with `flexisip` a SIP server made BellaDonna communications.

I will reiterate some commands from the previous findings mentioned above.

Also, you should be comfortable on making these changes directly on the system.

Some knowledge of what you are doing is certainly recommended, at least until someone else
adds tools to automatically patch firmwares to people that have less knowledge.

## Make flexisip listen on a reachable IP and add users to it

To be able to talk to our own SIP server, we need to make the SIP server on the C300X
talk to our internal network, instead of only locally (on the `lo` interface).

Mount the root system read-write

````
$ mount -oremount,rw /
````

Change the listening ports by appending some arguments in `/etc/init.d/flexisipsh`

(look at the end of the line, change to the IP of your C300X)

```
case "$1" in
  start)
    start-stop-daemon --start --quiet --exec $DAEMON -- $DAEMON_ARGS --transports "sips:$2:5061;maddr=$2;require-peer-certificate=1 sip:127.0.0.1;maddr=127.0.0.1 sip:192.168.0.XX;maddr=192.168.0.XX"
;;
```

You can also change it to - `$2`, the script will then put in the current wifi IP.

````
start-stop-daemon --start --quiet --exec $DAEMON -- $DAEMON_ARGS --transports "sips:$2:5061;maddr=$2;require-peer-certificate=1 sip:127.0.0.1;maddr=127.0.0.1 sip:$2;maddr=$2"
````

The intercom is firewalled, the easiest way is to remove the firewall file (or move it to somewhere on `/home/bticino/cfg/extra` which is a kind of permanent storage)

If you don't want to do that yet, drop the firewall rules from command line: (IMPORTANT: needs to be repeated after each reboot)

````
iptables -P INPUT ACCEPT
iptables -P FORWARD ACCEPT
iptables -P OUTPUT ACCEPT
````

If you are sick of repeating these commands every time you reboot:

````
mv /etc/network/if-pre-up.d/iptables /home/bticino/cfg/extra/iptables.bak
mv /etc/network/if-pre-up.d/iptables6 /home/bticino/cfg/extra/iptables6.bak
````

Edit the `/home/bticino/cfg/flexisip.conf` so `baresip` can authenticate with it.

Set `log-level` and `syslog-level` to `debug` (it logs to `/var/log/log_rotation.log`)

In `trusted-hosts` add the IP address of the server where you will run `baresip`.
This makes sure we don’t need to bother with the initial authentication of username/password.

Hosts in `trusted-hosts` can register without needing to authenticate.

````
[global]
...
log-level=debug
syslog-level=debug

[module::Authentication]
enabled=true
auth-domains=c300x.bs.iotleg.com
db-implementation=file
datasource=/etc/flexisip/users/users.db.txt
trusted-hosts=127.0.0.1 192.168.0.XX
hashed-passwords=true
reject-wrong-client-certificates=true
````

Now we will add a `user agent` (user) that will be used by `baresip` to register itself with `flexisip`

Edit the `/etc/flexisip/users/users.db.txt` file and create a new line by copy/pasting the c300x user.

For example:

````
c300x@1234567.bs.iotleg.com md5:ffffffffffffffffffffffffffffffff ;
baresip@1234567.bs.iotleg.com md5:ffffffffffffffffffffffffffffffff ;
````

Leave the md5 as the same value - I use `fffff....` just for this example.

Edit the `/etc/flexisip/users/route.conf` file and add a new line to it, it specifies where this user can be found on the network.
Change the IP address to the place where you will run `baresip` (same as `trusted-hosts` above)

````
<sip:baresip@1234567.bs.iotleg.com> <sip:192.168.0.XX>
````

Edit the `/etc/flexisip/users/route_int.conf` file.

This file contains one line that starts with `<sip:alluser@...` it specifies who will be called when someone rings the doorbell.

You can look at it as a group of users that is called when you call `alluser@1234567.bs.iotleg.com`

Add your username at the end (make sure you stay on the same line, NOT a new line!)
````
<sip:alluser@1234567.bs.iotleg.com> ..., <sip:baresip@1234567.bs.iotleg.com>
````

Reboot and verify flexisip is listening on the new IP address.

````
~# ps aux|grep flexis
bticino    741  0.0  0.3   9732  1988 ?        SNs  Oct28   0:00 /usr/bin/flexisip --daemon --syslog --pidfile /var/run/flexisip.pid --p12-passphrase-file /var/tmp/bt_answering_machine.fifo --transports sips:192.168.0.XX:5061;maddr=192.168.0.XX;require-peer-certificate=1 sip:127.0.0.1;maddr=127.0.0.1  sip:192.168.0.XX;maddr=192.168.0.XX
bticino    742  0.1  1.6  45684  8408 ?        SNl  Oct28   1:44 /usr/bin/flexisip --daemon --syslog --pidfile /var/run/flexisip.pid --p12-passphrase-file /var/tmp/bt_answering_machine.fifo --transports sips:192.168.0.XX:5061;maddr=192.168.0.XX;require-peer-certificate=1 sip:127.0.0.1;maddr=127.0.0.1  sip:192.168.0.XX;maddr=192.168.0.XX
````

## Compile and setup baresip 

To connect to the intercom we will use `baresip` a modular SIP server that compiles 
on a number of platforms (Linux, android, iphone, windows and Mac).

Since `flexisip` also uses an old `speex` audio codec, we will need to compile that inside it (the removed it because it was old).

Clone following projects in an empty directory, we used the 2.9.0 branch for `baresip`, `re` and `rem`.

We used `1.2.1` for Speex.

````
git clone https://github.com/baresip/baresip.git
git clone https://github.com/baresip/re.git
git clone https://github.com/baresip/rem.git
git clone https://github.com/xiph/speex.git
````

Install the development libraries to be able to compile, mine are:

````
sudo apt install cmake g++ libssl-dev pkg-config libmosquitto-dev autoconf libtool libavdevice-dev libavcodec-dev libasound2-dev  libavfilter-dev alsa-utils
````

If you are missing something please let us know so we can add it to this file.

If you want to use the snapshot module (see below) you will also need, it takes a snapshot and uses the png library.

````
sudo apt-get install libpng-dev
````

If you want `baresip` to display something you can enable the `x11` module (on linux).

If also support `sdl` and `directfb` as output.

For X11 you will need:

````
sudo apt install libx11-dev libxext-dev
````

Compile `re`, we use `-DCMAKE_INSTALL_PREFIX=` to specify where to install the libraries.

I don't like installing in /usr/local/* - or maybe you want to install this somewhere where you don't have root access...

That's why I like to install stuff in the `$HOME` dir, you can remove it if it bothers you. But change it later for the other commands.

````
cd re
cmake -B build -DCMAKE_INSTALL_PREFIX=$HOME/baresipstatic -DUSE_OPENSSL=1
cmake --build build -j
cmake --install build
````

Do the same for `rem`

````
cd ..
cd rem
cmake -B build -DCMAKE_INSTALL_PREFIX=$HOME/baresipstatic -DUSE_OPENSSL=1
cmake --build build -j
cmake --install build
````

Compile the `speex` audio codec

````
cd ..
cd speex
./autogen.sh
./configure --prefix=$HOME/baresipstatic/speex-libs
make && make install
````

Apply the `speex.patch` to the `baresip` directory you can find the patch here:

https://gist.github.com/slyoldfox/6c1c97bd43d97ffa428904523d3f63cf

Apply the patch in the `baresip` directory:

````
cd baresip
wget https://gist.githubusercontent.com/slyoldfox/6c1c97bd43d97ffa428904523d3f63cf/raw/500b0e3795da01f233517e6fe4bca6a601351028/speex.patch
git apply speex.patch
````

Next - apply the patch that is the missing part to make calls to the c300x.

If you wouldn't use this patch, and you would call the c300x from `baresip` you would get a "normal" intercom call.

You would notice it would be just audio and you can talk to the intercom, but you wouldn't be talking to the outside unit.

This is because the `bt_answering_machine` is the component that takes care of _switching_ the SIP call to the outdoor unit.

The answering machine answers this special type of call and enables the video camera outside for us and everything behind it.

So ... 

````
cd baresip
wget https://gist.githubusercontent.com/slyoldfox/e2c17fa8f4daf2900c9e0cef0341922f/raw/6abcfdb018f257e94bd1f6a2ddfc1ce92d30f67f/bt.patch
git apply bt.patch
````

If you look at the patch (https://gist.github.com/slyoldfox/e2c17fa8f4daf2900c9e0cef0341922f) you will see that we are setting an extra attribute:

The `DEVADDR:20` will add a SIP attribute to the SIP call in the form of `a=DEVADDR:20`

This will be interpreted by the `bt_answering_machine` to signal this is a call meant to connect the outdoor unit.

Build `baresip` .. you can add/remove modules to -DMODULES="", but this worked for me:

NOTE: We specify `-DSTATIC=ON` to build a static build. This makes a _fat_ baresip with all `.so` libraries built-in.

It is heavier for memory, but slightly safer (everything is linked at compile time).

With shared libraries (`.so` files in the `modules/lib` directory) it's possible to get runtime errors and segfaults.

I like it this way ... if you like the other setup better, remove that and you're on your own :-)

````
cmake -B build -DSTATIC=ON -DMODULES="x11;snapshot;presence;menu;speex;account;mqtt;debug_cmd;echo;contact;stdio;alsa;auconv;netroam;auresamp;avcodec;srtp;avfilter;fakevideo" -DCMAKE_INSTALL_PREFIX=$HOME/baresipstatic -DSPEEX_HINTS=$HOME/baresipstatic/speex-libs
cmake --build build -j
cmake --install build
````

Start `baresip` and quit with `q`, this will generate the config files in `$HOME/.baresip` directory.

Again here I favour a `local` setup by setting the `LD_LIBRARY_PATH`, other people may want to use `ldconfig`

````
cd $HOME/baresipstatic
export LD_LIBRARY_PATH=$HOME/baresipstatic/lib
./bin/baresip -v
````

Edit (and read) `$HOME/.baresip/config`

Uncomment and comment as below:

````
# SIP
sip_listen              0.0.0.0:5060
#sip_certificate        cert.pem
sip_cafile              /etc/ssl/certs/ca-certificates.crt
sip_transports          udp,tcp

audio_player            alsa,plughw:Loopback,0,1
audio_source            alsa,plughw:Loopback,1,0
audio_alert             alsa,plughw:Loopback,0,1
ausrc_srate             8000
auplay_srate            8000

# If you enabled X11 module and you are on linux
video_display   x11,nil

video_bitrate           90000
video_fps               30.00
video_fullscreen        no
videnc_format           yuv420p

# avcodec
avcodec_h264enc libx264
avcodec_h264dec h264
#avcodec_h265enc        libx265
#avcodec_h265dec        hevc
#avcodec_hwaccel        vaapi
avcodec_profile_level_id 42801F

# comment lines:
#module                 g711.so
#module                 uuid.so
#module                  stun.so
#module                  turn.so
#module                  ice.so

# uncomment:
module                  avcodec.so
module                  srtp.so
module                  fakevideo.so
module                  avformat.so
````

Enable the soundloop device for ALSA - see https://github.com/spbroot/sipdoorbell for more info

````
sudo su
echo 'snd-aloop' >> /etc/modules
exit

# or
modprobe snd-aloop
````

And this ALSA config in `/usr/share/alsa/alsa.conf`:

````
    Create or edit the /etc/asound.conf file

    # output device
    pcm.loop_out {
      type dmix
      ipc_key 100001
      slave.pcm "hw:Loopback,0,0"
    }

    # input device
    pcm.loop_in {
      type dsnoop
      ipc_key 100002
      slave.pcm "hw:Loopback,1,1"
    }

    # plug device
    pcm.sipdoorbell_return {
      type plug
      slave.pcm 'loop_out'
      hint {
        show on
        description 'sipdoorbell return channel'
      }
    }

    # plug device
    pcm.sipdoorbell_main {
      type plug
      slave.pcm loop_in
      hint {
        show on
        description 'sipdoorbell main channel'
      }
    }
````

Add the `user agent` you created on `flexisip` to `$HOME/.baresip/accounts`.

Notice that we specify a mandatory srtp encryption here - it is required by `flexisip`.

````
<sip:baresip@1234567.bs.iotleg.com>;auth_pass=notused;mediaenc=srtp-mand
````

Add the intercom as a contact, it's easier to call it, edit `$HOME/.baresip/contacts`

```
"c300x" <sip:c300x@1234567.bs.iotleg.com>
```

Start baresip again and cross your fingers

`-v`: is verbose mode
`-s`: SIP call debug

I specify all modules here again, in case you forgot some in the `config` file

````
cd $HOME/baresipstatic
./bin/baresip -m speex -m snapshot -m echo -m x11 -m stdio -m alsa -m avcodec -m srtp -m account -m contact -m debug_cmd -m menu -m netroam -v -s
baresip v2.9.0 Copyright (C) 2010 - 2022 Alfred E. Heggestad et al.
Local network addresses:
   enp4s0f0:  192.168.0.XX
   wlp3s0b1:  192.168.0.YY
pre-loading modules: 4
aucodec: speex/32000/2
aucodec: speex/16000/2
aucodec: speex/8000/2
aucodec: speex/32000/1
aucodec: speex/16000/1
aucodec: speex/8000/1
vidfilt: snapshot
echo: module loaded
vidisp: x11
uag: add local address 192.168.0.XX
uag: add local address 192.168.0.YY
ui: stdio
ausrc: alsa
auplay: alsa
vidcodec: H264
vidcodec: H264
vidcodec: H265
avcodec: using H.264 encoder 'libx264' -- libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10
avcodec: using H.264 decoder 'h264' -- H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10
avcodec: using H.265 encoder 'libx265' -- libx265 H.265 / HEVC
avcodec: using H.265 decoder 'hevc' -- HEVC (High Efficiency Video Coding)
mediaenc: srtp
mediaenc: srtp-mand
mediaenc: srtp-mandf
module: loading app account.so
baresip@1234567.bs.iotleg.com: Using media encryption 'srtp-mand'
ua: ua_register sip:baresip@1234567.bs.iotleg.com
Populated 1 account
module: loading app contact.so
Populated 6 contacts
module: loading app debug_cmd.so
module: loading app menu.so
module: loading app netroam.so
Populated 6 audio codecs
Populated 0 audio filters
Populated 3 video codecs
Populated 1 video filter
baresip is ready.
baresip@1234567.bs.iotleg.com: (prio 0) {0/UDP/v4} 200 Registration successful (Flexisip/1.0.12 (sofia-sip-nta/2.0)) [2 bindings]
All 1 useragent registered successfully! (489 ms)
````

Interesting keys (case sensitive):

* `D`: dial current contact
* `ESC`: hang up
* `n`: network debug
* `q`: quit
* `r`: registration debug
* `v`: change log level (use `DEBUG` for most)
* `h`: help
* `i`: SIP debug
* `<` and `>`: previous and next contact

Useful commands (type /command):

`/quit`
`/modules`
`/dialdir`

Check the loaded modules

````
/modules

--- Modules (13) ---
            speex type=codec        ref=1
         snapshot type=vidfilt      ref=1
             echo type=application  ref=1
              x11 type=vidisp       ref=1
            stdio type=ui           ref=1
             alsa type=sound        ref=1
          avcodec type=codec        ref=1
             srtp type=menc         ref=1
          account type=application  ref=1
          contact type=application  ref=1
        debug_cmd type=application  ref=1
             menu type=application  ref=1
          netroam type=application  ref=1
````

Use `<` and/or `>` to select the c300x contact and then press `D` to dial the intercom

````
baresip@1234567.bs.iotleg.com: selected for request
call: alloc with params laddr=192.168.0.XX, af=AF_INET, use_rtp=1
call: use_video=1
call: connecting to 'sip:c300x@1234567.bs.iotleg.com'..
================ adding DEVADDR to outgoing call ===============stream: audio: starting mediaenc 'srtp-mand' (wait_secure=0)
stream: video: starting mediaenc 'srtp-mand' (wait_secure=0)
call: SIP Progress: 100 Trying (/)
call: SIP Progress: 180 Ringing (/)
video: stopping video source ..
video: stopping video display ..
alsa: reset: srate=8000, ch=1, num_frames=320, pcmfmt=S16_LE
alsa: playback started (plughw:Loopback,0,1)
call: got SDP answer (578 bytes)
baresip@1234567.bs.iotleg.com: Call answered: sip:c300x@1234567.bs.iotleg.com
alsa: stopping playback thread (plughw:Loopback,0,1)
call: update media
stream: update 'audio'
stream: audio: starting mediaenc 'srtp-mand' (wait_secure=0)
srtp: audio: SRTP is Enabled (cryptosuite=AES_CM_128_HMAC_SHA1_80)
call: mediaenc event 'Secure' (audio,AES_CM_128_HMAC_SHA1_80)
stream: audio: starting RTCP with remote 192.168.0.XX:43169
audio: Set audio encoder: speex 8000Hz 1ch
speex: setting VBR=1 VAD=0
audio: start
alsa: reset: srate=8000, ch=1, num_frames=160, pcmfmt=S16_LE
alsa: recording started (plughw:Loopback,1,0) format=S16LE
audio: source started with sample format S16LE
audio: Set audio decoder: speex 8000Hz 1ch
audio: start
audio: create auplay buffer [20 - 160 ms] [320 - 2560 bytes]
alsa: reset: srate=8000, ch=1, num_frames=160, pcmfmt=S16_LE
alsa: playback started (plughw:Loopback,0,1)
audio: player started with sample format S16LE
audio tx pipeline:        alsa ---> aubuf ---> speex
audio rx pipeline:        alsa <--- aubuf <--- speex
audio: start
stream: audio: starting RTCP with remote 192.168.0.XX:43169
stream: update 'video'
stream: video: starting mediaenc 'srtp-mand' (wait_secure=0)
srtp: video: SRTP is Enabled (cryptosuite=AES_CM_128_HMAC_SHA1_80)
call: mediaenc event 'Secure' (video,AES_CM_128_HMAC_SHA1_80)
stream: video: starting RTCP with remote 192.168.0.XX:24361
video: update
Set video encoder: H264 packetization-mode=0 (90000 bit/s, 30.00 fps)
avcodec: h264 encoder activated
avcodec: video encoder H264: 30.00 fps, 90000 bit/s, pktsize=1280
Set video decoder: H264 packetization-mode=0
avcodec: h264 decoder activated
avcodec: decode: hardware accel disabled
avcodec: video decoder H264 ()
video: start source
video: no video source
video tx pipeline:      (src) ---> snapshot ---> H264
video rx pipeline:     (disp) <--- snapshot <--- H264
video: start display
stream: video: starting RTCP with remote 192.168.0.XX:24361
audio: update
video: update
video: start source
video: no video source
video tx pipeline:      (src) ---> snapshot ---> H264
video rx pipeline:        x11 <--- snapshot <--- H264
call: stream start (active=1)
audio: start
video: update
video: start source
video: no video source
video tx pipeline:      (src) ---> snapshot ---> H264
video rx pipeline:        x11 <--- snapshot <--- H264
stream: audio: enable RTP from remote
stream: video: enable RTP from remote
baresip@1234567.bs.iotleg.com: Call established: sip:c300x@1234567.bs.iotleg.com
stream: incoming rtp for 'video' established, receiving from 192.168.0.XX:24360
avcodec: decoder waiting for keyframe
[h264 @ 0x55f6996128c0] non-existing PPS 0 referenced
[h264 @ 0x55f6996128c0] decode_slice_header error
[h264 @ 0x55f6996128c0] no frame!
avcodec: decode: avcodec_send_packet error, packet=5351 bytes, ret=-1094995529 (Invalid data found when processing input)
video: H264 decode error (seq=7, 193 bytes): Bad message [74]
stream: incoming rtp for 'audio' established, receiving from 192.168.0.XX:43168
[h264 @ 0x55f6996128c0] non-existing PPS 0 referenced
[h264 @ 0x55f6996128c0] decode_slice_header error
[h264 @ 0x55f6996128c0] no frame!
avcodec: decode: avcodec_send_packet error, packet=1395 bytes, ret=-1094995529 (Invalid data found when processing input)
video: H264 decode error (seq=10, 100 bytes): Bad message [74]
[h264 @ 0x55f6996128c0] non-existing PPS 0 referenced
[h264 @ 0x55f6996128c0] decode_slice_header error
[h264 @ 0x55f6996128c0] no frame!
avcodec: decode: avcodec_send_packet error, packet=1510 bytes, ret=-1094995529 (Invalid data found when processing input)
video: H264 decode error (seq=13, 215 bytes): Bad message [74]
[h264 @ 0x55f6996128c0] non-existing PPS 0 referenced
[h264 @ 0x55f6996128c0] decode_slice_header error
[h264 @ 0x55f6996128c0] no frame!
avcodec: decode: avcodec_send_packet error, packet=2169 bytes, ret=-1094995529 (Invalid data found when processing input)
video: H264 decode error (seq=16, 874 bytes): Bad message [74]
[h264 @ 0x55f6996128c0] non-existing PPS 0 referenced
[h264 @ 0x55f6996128c0] decode_slice_header error
[h264 @ 0x55f6996128c0] no frame!
avcodec: decode: avcodec_send_packet error, packet=2344 bytes, ret=-1094995529 (Invalid data found when processing input)
video: H264 decode error (seq=19, 1049 bytes): Bad message [74]
[h264 @ 0x55f6996128c0] non-existing PPS 0 referenced
[h264 @ 0x55f6996128c0] decode_slice_header error
[h264 @ 0x55f6996128c0] no frame!
avcodec: decode: avcodec_send_packet error, packet=2301 bytes, ret=-1094995529 (Invalid data found when processing input)
video: H264 decode error (seq=22, 1006 bytes): Bad message [74]
[h264 @ 0x55f6996128c0] non-existing PPS 0 referenced
[h264 @ 0x55f6996128c0] decode_slice_header error
[h264 @ 0x55f6996128c0] no frame!
avcodec: decode: avcodec_send_packet error, packet=2301 bytes, ret=-1094995529 (Invalid data found when processing input)
video: H264 decode error (seq=25, 1006 bytes): Bad message [74]
video: receiving with resolution 400 x 288 and format 'yuv420p'
````

You are now receiving audio and video at about 10fps at a resolution of 400x288.

If you have enabled X11 you should be able to see it as well https://ibb.co/w7Lqymz

If you installed the `snapshot` module you can type /snapshot and it will grab a screenshot for you.

Combining this you could generate screenshots in a place where you can visualize them or create a gif or jpeg stream from them.

There is no video filter module yet that exports video frames to an external place yet, but it shouldn't be too hard to write.
