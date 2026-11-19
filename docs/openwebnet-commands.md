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
| `*8*1#1#4#21*10##` | incoming external call |
| `*8*1#1#4#21*11##` | incoming external call |
| `*8*1#1#4#21*16##` | incoming external call |
| `*7*59#12#0#0*##` | incoming internal call |
| `*8*9#1#4*20##` | ring confirmation: WHERE is the entrance panel that rang |
| `*8*4#11*20##` | camera on, as sent to unit 11 for panel 20 (`*8*4#<unit>*<PE>##`) |

### Ring frames and the calling entrance panel

A real ring from an entrance panel (PE) appears as two frames, back to back:

```
*8*1#1#4#21*11##     call to unit 11
*8*9#1#4*20##        confirmation, WHERE = the panel that rang
```

- In the call frame `*8*1#1#4#21*<unit>##` the `21` is constant: it is identical
  for units 10, 11 and 16 in the table above, so it is not the panel.
  The `<unit>` at the end is the unit being called. The bridge treats any
  `*8*1#1#…` frame as a doorbell ring.
- The panel is the WHERE of the `*8*9#1#4*<WHERE>##` confirmation. Camera-on
  (autoaccensione) uses the same convention with mode `5` instead of `1`:

  ```
  *8*4#11*20##         request: camera on for panel 20, unit 11
  *8*1#5#4#20*11##     call-like frame, mode 5, panel 20 in the WHAT
  *8*9#5#4*20##        confirmation, WHERE = 20
  ```

  and for panel 21: `*8*1#5#4#21*11##` then `*8*9#5#4*21##`.

Observed on a Classe 300X13E, firmware 1.7.19, unit address 11, two panels
(20 and 21): camera-on for both panels, and two real rings from panel 20
(`*8*1#1#4#21*11##` followed by `*8*9#1#4*20##`). A ring from panel 21 has not
been captured yet; `*8*9#1#4*21##` is expected from the pattern above.

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
