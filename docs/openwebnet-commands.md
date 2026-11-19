# OpenWebNet commands and discovery

The Classe 300X speaks OpenWebNet (frames of the form `*WHO*WHAT*WHERE##`) on
its local bus. These are the commands and notifications observed on a Classe
300X, useful whether you drive the device through the bridge or through plain
MQTT/shell. They are a reference, not a guarantee: WHERE values depend on your
installation.

## Known commands and notifications

| Frame | Meaning |
| --- | --- |
| `*8*19*20##` | main door opening button pressed |
| `*8*20*20##` | main door opening button released |
| `*8*19*21##` | secondary door opening button pressed |
| `*8*20*21##` | secondary door opening button released |
| `*8*21*10##` | stairs light activated |
| `*8*1#5#4#20*10##` | intercom camera on |
| `*8*3#5#4*420##` | intercom camera off |
| `*8*3#1#4*420##` | end of an incoming call (observed on fw 1.7.19) |
| `*8*3#1#4*411##` | end of a call answered on the monitor (observed on fw 1.7.19) |
| `*8*1#1#4#21*10##` | incoming external call |
| `*8*1#1#4#21*11##` | incoming external call |
| `*8*1#1#4#21*16##` | incoming external call |
| `*7*59#12#0#0*##` | incoming internal call |

Call-end frames follow the pattern `*8*3#<mode>#4*<where>##`. The WHERE part
differs between installations (416, 420, 411 and others seen), so the bridge
matches any `*8*3#...` frame instead of a fixed list.

Opening a door is usually a press frame followed by a release frame about a
second later, for example `*8*19*20##` then `*8*20*20##`.

## Sending a command on the device

The native processes listen on localhost. The non-interfering way to send a
frame is to pipe it to the command port with netcat, which is also how the
bridge does it:

```sh
echo '*8*19*20##' | nc 0 30006
```

## Discovering new commands

- **MQTT sniffing:** watch the MQTT topics (the bridge or `mqtt_scripts/`
  republish bus traffic) with any MQTT listener while you trigger the action
  from the official app or a physical button.
- **TCP sniffing on the device:** capture loopback traffic, trigger the action,
  then read the capture in Wireshark:

  ```sh
  ssh root2@<intercom_ip> '/usr/sbin/tcpdump -i lo -U -w - "not port 22"' > capture.pcap
  # trigger the action from the app, then Ctrl+C
  wireshark capture.pcap
  ```

Replace `<intercom_ip>` with your unit's address. Match the new frame against
the WHO/WHAT/WHERE structure above to label it.
