# Legacy builder and flashing method

This is the original way to get root on a Classe 300X: a script rebuilds an
official `.fwz` with SSH access, and BTicino's MyHOME Suite (Windows) flashes
it over USB.

**It is still the working path today.** The Go flasher and the on-the-fly
builder described in the top-level `README.md` are in development; until they
land, use the steps below. Nothing in this directory ships firmware: you bring
your own `.fwz`.

Read the [threat model](../THREAT_MODEL.md) first. Flashing can brick the unit.

## 1. Build the rooted image

You need Linux with Docker, or Python 3 with `sudo` (the script loop-mounts the
firmware's root filesystem, so it needs root).

From the **repository root**:

```sh
git clone https://github.com/fquinto/bticinoClasse300x.git
cd bticinoClasse300x
docker compose run bticino
```

Without Docker:

```sh
python3 -m pip install -r requirements.txt
sudo python3 legacy/main.py
```

The script asks a few questions and then does everything by itself:

- **Model and firmware version** — C300X (1.7.17 or 1.7.19) or C100X (1.5.x).
  It downloads the official firmware from the vendor, or uses the file you
  already have.
- **Root password** and whether to install an **SSH key**.
- Whether to remove the **signature files**.
- Whether to add the **MQTT scripts** ([`mqtt_scripts/`](../mqtt_scripts/README.md)).

The result is saved as `custom_firmware/NEW_<model>_<version>_<date>.fwz`.
Check the official file against the [known-good hashes](../hashes/README.md)
before building.

## 2. Flash it with MyHOME Suite

1. Take the unit off the wall bracket and connect its mini-USB port to a
   Windows computer. It shows up as a USB network card.
2. In MyHOME Suite, pick the device, move it to the working area, and select
   firmware version **1.7** for it.
3. Choose "update firmware", select the `.fwz` you built, and let it run to the
   end. The unit reboots during the process; do not unplug it.

![Flashing with MyHOME Suite](../myhome_flash.gif)

Known problems:

- "The selected service pack is not suitable for updating the device." — see
  [issue #43](https://github.com/fquinto/bticinoClasse300x/issues/43).
- Windows loses the USB network driver when the unit reboots mid-flash — see
  [issue #62](https://github.com/fquinto/bticinoClasse300x/issues/62).

## 3. Log in

After the flash, the unit answers SSH on your network as user `root2`, with
the password (or key) you chose. Do not keep the default password the script
suggests. From there you can install the
[bridge](../bticino_bridge/README.md).

```sh
ssh -i bticinokey -o PubkeyAcceptedAlgorithms=+ssh-rsa root2@<unit-ip>
```

The unit runs dropbear 2017.75, which only accepts RSA user keys signed with
`ssh-rsa` (SHA-1). OpenSSH 8.8 and later no longer offer that by default, so
without `-o PubkeyAcceptedAlgorithms=+ssh-rsa` the key is refused and you are
asked for the password. The host key is ECDSA, so nothing else is needed.

How SSH is started: the builder installs
[`patches/ssh/c300x-ssh`](../patches/ssh/c300x-ssh) as `/etc/init.d/c300x-ssh`
(`rc5.d/S99c300x-ssh`) and does **not** enable the factory
`/etc/init.d/dropbear`. The native `bt_daemon` stops the factory dropbear a few
minutes after boot when SSH is not enabled in its `conf.xml`
([issue #66](https://github.com/fquinto/bticinoClasse300x/issues/66)). The
`c300x-ssh` script runs its own dropbear with its own pidfile, so that stop
does not reach it, and starts it again within 10 seconds if nothing listens on
port 22. Its host keys are generated on first boot in
`/home/bticino/cfg/extra/dropbear/` (persistent partition), so the host
fingerprint stays the same across reboots. The first boot takes a few extra
seconds while the keys are generated. To refuse password logins and allow keys
only, add `-s` to the dropbear command line in that script before building.

## What else is in this folder

- [`old_information.md`](old_information.md) — the fully manual method: unpack
  the `.fwz`, edit the ext4 root filesystem by hand, repack.
- `unpack01_fwz.sh`, `unpack02_btweb-ext4.sh`, `pack01_btweb-ext4.sh`,
  `pack02_fwz.sh` — the shell helpers for that manual method.
- `bticino.py`, `diagnose_mqtt.py` — small OpenWebNet and MQTT diagnostic tools.
- [`firmware_versions.md`](firmware_versions.md),
  [`CompareFirmwareVersions.md`](CompareFirmwareVersions.md),
  [`samples_json.md`](samples_json.md) — reference notes.
