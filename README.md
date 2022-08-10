# BTicino Classe 300X

## Telegram Channel for findings

- https://t.me/bTicinoClasse300x

## Main creation firmware script

[![asciicast](https://asciinema.org/a/514007.svg)](https://asciinema.org/a/514007)

(Using GNU/Linux):

```bash
git clone https://github.com/fquinto/bticinoClasse300x.git
cd bticinoClasse300x
pip3 install virtualenv
python3 -m venv bticinoenv
source bticinoenv/bin/activate
pip3 install -r requirements.txt
python3 -m pip install --upgrade pip
sudo python3 main.py
```

## Basics steps to get root

Get some tools: firmware and flashing tool. https://www.bticino.com/software-and-app/configuration-software/

### Firmware BTICINO

Search for new firmware in this website: https://www.homesystems-legrandgroup.com/ and write the next "model number" in search bar.

Model 344642 = CLASSE 300X13E Touch Screen handsfree video intern
https://www.homesystems-legrandgroup.com/home/-/productsheets/2486279

Model 344643 = CLASSE 300X13E Touch Screen handsfree video intern
https://www.homesystems-legrandgroup.com/home/-/productsheets/2486306

- [Version 1.7.17](https://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName=C300X_010717.fwz&fileId=58107.23188.15908.12349)

- [Version 1.7.17](https://prodlegrandressourcespkg.blob.core.windows.net/packagecontainer/package_343bb0abacf05a27c6c146848e85d1de2425700e_h.tar.gz)

### Flashing with MyHomeSuite

Product page: https://www.homesystems-legrandgroup.com/en/home/-/productsheets/2493426

- [Version MyHomeSuite 3.5.5](http://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName=MyHOME_Suite_030505.exe&fileId=58107.23188.29881.48619)

- [Version MyHomeSuite 3.5.19](https://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName=MyHOME_Suite_030519.exe&fileId=58107.23188.31182.6881)

### Steps for modify firmware before flash

- Open firmware ZIP using password: `C300X`
- Note for C100X use password: `C100X`

- unGZ file: btweb_only.ext4.gz to btweb_only.ext4

- Mount root filesystem:
  `sudo mount -o loop btweb_only.ext4 /media/mounted/`

- Select your password, example: `pwned123`
- See the salt of your selected password:

```sh
openssl passwd -1 -salt root pwned123
$1$root$0i6hbFPn3JOGMeEF0LgEV1
```

```sh
cd /media/mounted/etc/
sudo vim shadow
```

- Set to:

```sh
root2:$1$root$0i6hbFPn3JOGMeEF0LgEV1:18033:0:99999:7:::
bticino2:$1$root$0i6hbFPn3JOGMeEF0LgEV1:18033:0:99999:7:::
sudo vim passwd
root2:x:0:0:root:/home/root:/bin/sh
bticino2:x:1000:1000::/home/bticino:/bin/sh
```

- Setup dropbear (is a SSH server)

```sh
cd /media/mounted/etc/rc5.d/
sudo ln -s ../init.d/dropbear S98dropbear
```

### Access SSH and scripts for open door

In this examples, we are using next:
<p>
UNIT = yyyyyyy<br>
MAC_ADDRESS = 00-03-50-xx-xx-xx<br>
IP = 192.168.1.97
PASSWORD = pwned123
</p>

**Replace with your needs.**

#### Change password for user root2

  ```
  mount -oremount,rw /
  passwd root2
  mount -oremount,ro /
  ```

#### Adapt your SSH access

- In terminal 1: access inside bticino and setup access for RW:
  - `mount -oremount,rw /`
  - **Not close this terminal**

- In terminal 2: create SSH key:

  `ssh-keygen -o -b 4096 -t rsa -f ./keys/bticinokey`

  Touch: [INTRO]+[INTRO]

- In terminal 2: copy SSH key inside the device:

  `ssh-copy-id -i ./keys/bticinokey.pub root2@192.168.1.97`

- In terminal 2: setup SSH key with rights and in your user home:

  `cp ./keys/bticinokey ~/.ssh/bticinokey`
  `chmod 600 ~/.ssh/bticinokey`

- In terminal 2: config SSH easy access:

  `$ cat ~/.ssh/config`

  ```sh
  Host bticino
    HostName 192.168.1.97
    User root2
    StrictHostKeyChecking no
    IdentityFile ~/.ssh/bticinokey
  ```

- In terminal 1: (inside your Bticino)
  ```sh
  cd
  mkdir .ssh
  cp /etc/dropbear/authorized_keys .ssh/authorized_keys
  ```

- In terminal 2: test:

  `ssh bticino`

- In terminal 1: (inside your Bticino) change to RO access:
  - `mount -oremount,ro /`

#### Scripts for open door

Use name of the script: `openbuildingdoor.sh`

- Script one:

```sh
#!/usr/bin/expect -f
spawn ssh bticino
expect "assword:"
send "pwned123\r"
expect "root@C3X-00-03-50-xx-xx-xx-yyyyyyy:~#"
send "echo *8*19*20## |nc 0 30006\r"
send "sleep 1\r"
send "echo *8*20*20##|nc 0 30006\r"
send "exit\r"
interact
```

- Script two:

```sh
#!/bin/bash
sshpass -p pwned123 ssh -o StrictHostKeyChecking=no bticino "echo *8*19*20## |nc 0 30006; sleep 1; echo *8*20*20##|nc 0 30006"
```

- Direct test:

```sh
ssh bticino 'echo *8*19*20## |nc 0 30006; sleep 1; echo *8*20*20##|nc 0 30006'
```

## Home Assistant

Depends of your configuration you need to create a script of write directly command in HA.

In next example are both. You can use only one (you don't need both if one it's running ok).

- Add your script in `shell_components` folder
- Configure in HA:
  ```yaml
  shell_command:
    openbuildingdoor: "/home/homeassistant/.homeassistant/shell_commands/openbuildingdoor.sh"
    openbuildingdoor2: "ssh bticino 'echo *8*19*20## |nc 0 30006; sleep 1; echo *8*20*20##|nc 0 30006'"
  ```
- In your `ui-lovelace.yaml`:
  ```yaml
  cards:
    - type: button
        name: Open building door CMD1
        show_state: false
        show_name: true
        show_icon: true
        tap_action:
          action: call-service
          service: shell_command.openbuildingdoor
    - type: button
        name: Open building door CMD2
        show_state: false
        show_name: true
        show_icon: true
        tap_action:
          action: call-service
          service: shell_command.openbuildingdoor2
  ```

## Updated guide to connect to Home Assistant (using MQTT)

Go inside **mqtt_scripts** folder and follow steps: [MQTT](https://github.com/fquinto/bticinoClasse300x/tree/main/mqtt_scripts)

## Explore new commands

0) Go to your home: `cd ~`
1) On your Linux computer type:
`ssh root2@192.168.1.97 '/usr/sbin/tcpdump -i lo -U -w - "not port 22"' > recordingsFILE`
2) Use the App on your mobile and open the door or whatever you want to "learn"
3) Stop data recording: CTRL + C
4) Open recordingsFILE: `wireshark ~/recordingsFILE`
