# BTicino Classe 300X and C100X

## 1. Prepare and create firmware script

[![asciicast](https://asciinema.org/a/514007.svg)](https://asciinema.org/a/514007)

(Using GNU/Linux):

```bash
git clone https://github.com/fquinto/bticinoClasse300x.git
cd bticinoClasse300x
sudo python3 -m pip install --upgrade pip
sudo python3 -m pip install -r requirements.txt
sudo python3 main.py
```

## 2. Flash firmware using MyHomeSuite

Get some tools: firmware and flashing tool. https://www.bticino.com/software-and-app/configuration-software/

Product page: https://www.homesystems-legrandgroup.com/en/home/-/productsheets/2493426

- [Version MyHomeSuite 3.5.5](http://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName=MyHOME_Suite_030505.exe&fileId=58107.23188.29881.48619)

- [Version MyHomeSuite 3.5.19](https://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName=MyHOME_Suite_030519.exe&fileId=58107.23188.31182.6881)

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

## Telegram Channel for findings

- https://t.me/bTicinoClasse300x
