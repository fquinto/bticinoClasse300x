# Publish the Bticino 300X commands in MQTT

The goal of this guide is to export to an MQTT broker (mosquitto or others)
the commands managed by the Bticino 300X video door entry unit in order to allow integration of the system into a home automation software (eg: Homeassistant).

These scripts use utilities already present in the video door entry unit (mosquitto_pub, mosquitto_sub, python3, tcpdump), in fact it is not necessary to install any additional software.
To export the commands of the video door entry unit to the network in MQTT I have prepared the following script files

## TcpDump2Mqtt.sh

This script is only used to launch the main TcpDump2Mqtt script in background mode. A symbolic link to this file is created in the /etc/rc5.d folder to automatically start on boot.

## TcpDump2Mqtt

This is the main script that checks every 10 minutes:
* that the script for publishing and receiving commands are active and alternatively executes them.
* if the gateway to which the video door entry unit is connected (parameter GATEWAYADDR = 192.168.1.1) can be reached. The management of the polling to the gateway has been implemented because if the connection with the Wifi is lost, the ** StartMqttSend ** and ** StartMqttReceive ** scripts would no longer work. If the gateway is not reachable, the currently active scripts are killed and then run again when the connection is restored. In my specific case, I turn off the WiFi in the evening to reactivate it in the morning, and thus I have solved in this way the problem of blocking the scripts on disconnection.

The configuration parameters for the script operation are at the top of the file and are the following:

* **BROKERADDR** = 192.168.1.2
   Ip address of the mqtt broker (mosquitto or others).
* **RXTOPIC** = Bticino / rx
   MQTT topic with which the home automation software (eg: homeassistant) sends commands to the video door entry unit (eg gate opening, light activation, ..).
* **DUMPTOPIC** = Bticino / tx
   MQTT topic with which the video door entry unit sends all the commands that are executed from the internal unit, from the application or from the external unit.
* **TOPICSRD** = Bticino / start_date
   MQTT topic which is updated with the date and time of activation of the script
* **LASTWILLTOPIC** = Bticino / LastWillT
   MQTT topic set to online / offline in case of connection / disconnection of the video door entry unit from the WiFi network.
* **GATEWAYADDR** = 192.168.1.1
   Address of the router to which the video door entry unit is connected.

## StartMqttSend

This script listens to the network traffic of the video door entry unit (commands from the app, from the internal unit, from the external unit), filters the packets and extracts the commands, sending them to the MQTT broker through the topic defined in the DUMPTOPIC variable of the main script TcpDump2Mqtt.

## StartMqttReceive

This script listens to the commands coming from the broker, therefore from the home automation software and, through the MQTT topic defined in the RXTOPIC variable of the main script TcpDump2Mqt, it executes them on the video door entry unit.

## filter.py

This file is in python and is used by the StartMqttSend script to filter network packets by eliminating unnecessary parts and isolating the commands to be sent to the home automation software.

## Insertion of scripts in the video door entry unit

**NOTE**: In order to insert the scripts in the video door entry unit, SSH must be enabled according to the guide of [@fquinto](https://github.com/fquinto/) [https://github.com/fquinto/bticinoClasse300x](https://github.com/fquinto/bticinoClasse300x)

1. Download files described above to a folder on your PC:
	* TcpDump2Mqtt.sh
	* TcpDump2Mqtt
	* StartMqttSend
	* StartMqttReceive
	* filter.py

2. Open the __TcpDump2Mqtt__ script with an editor and obligatorily modify the two variables **BROKERADDR** and **GATEWAYADDR** with the Ip addresses of your MQTT broker and the gateway of your network.

3. Transfer the files from the PC to the video door entry unit.
  The files will be transferred to the / tmp folder of the video door entry unit as the rest of the filesystem is read-only. For the transfer you can use the scp command present in linux and windows 10 (from a "Command Prompt" window).

    **Transfer of files to the video door entry unit**

    ```sh
    scp TcpDump2Mqtt.sh root2@192.168.1.97:/tmp/TcpDump2Mqtt.sh
    scp TcpDump2Mqtt root2@192.168.1.97:/tmp/TcpDump2Mqtt
    scp StartMqttSend root2@192.168.1.97:/tmp/StartMqttSend
    scp StartMqttReceive root2@192.168.1.97:/tmp/StartMqttReceive
    scp filter.py root2@192.168.1.97:/tmp/filter.py
    ```

    **NOTE**: 192.168.1.97 is the IP address of the video intercom door entry unit.
    When prompted for a password, enter the one you defined in the SSH enabling procedure.

4. Once the 5 files have been transferred, connect via the terminal to the video door entry unit and execute the following commands

    ```sh
    # We move to the / tmp folder where we transferred the files.
    cd / tmp
    # We change the permissions of the scripts to make them executable.
    chmod 755 TcpDump2Mqtt TcpDump2Mqtt.sh StartMqttSend StartMqttReceive
    # make the filesystem writable.
    mount -oremount, rw /
    # Transfer the scripts to the / etc directory.
    cp ./TcpDump2Mqtt / etc
    cp ./TcpDump2Mqtt.sh / etc
    cp ./StartMqttSend / etc
    cp ./StartMqttReceive / etc
    # Transfer the python file to the / home / root directory.
    cp ./filter.py / home / root
    # We move to the /etc/rc5.d folder.
    cd /etc/rc5.d
    # We create the symbolic link for autorun on startup.
    ln -s ../TcpDump2Mqtt.sh S99TcpDump2Mqtt
    # Make the filesystem read-only.
    mount -oremount, ro /
    # Let's restart the video door entry unit.
    reboot
    ```

## Gestione del videocitofono da Homeassistant

Our video door entry unit is now sending / receiving any commands to the MQTT broker.

**NOTE**: In order to manage MQTT topics in Homeassistant it is necessary to have installed the MQTT integration.

### configuration.yaml

In the homeassistant **configuration.yaml** file, in the **sensor:** block it is necessary to insert the following lines to instruct MQTT to receive / transmit on the topics we have defined in the scripts.

**modify the configuration.yaml file or inside sensors.yaml**

```yaml
#
#  Video intercom Bticino
#
sensors:
  - platform: mqtt
    unique_id: '14532784978700'
    name: "Video intercom TX"
    state_topic: "Bticino/tx"
    availability_topic: "Bticino/LastWillT"
    icon: mdi:phone-outgoing

  - platform: mqtt
    unique_id: '13454564689485'
    name: "Video intercom RX"
    state_topic: "Bticino/rx"
    availability_topic: "Bticino/LastWillT"
    icon: mdi:phone-incoming
```

## Automations

We need to create automations that allow us to interact with the video door entry unit.

### Open the door

The following automation creates a button that allows the gate to be opened and creates a notification in the Homeassistant notification area.

**Automation to open the gate**

```yaml
    - id: '1656918057723'
      alias: Apertura Cancelletto Pedonale
      description: ''
      trigger:
      - platform: state
        entity_id:
        - input_button.cancelletto_pedonale
      condition: []
      action:
      - service: notify.persistent_notification
        data:
          message: Il cancello pedonale è aperto
      - service: mqtt.publish
        data:
          topic: Bticino/rx
          payload: '*8*19*20##'
      - delay:
          hours: 0
          minutes: 0
          seconds: 1
          milliseconds: 0
      - service: mqtt.publish
        data:
          payload: '*8*20*20##'
          topic: Bticino/rx
      mode: single
```

### Recognize the commands

The following automation recognizes some commands received from the video door entry unit and notifies the event via text and voice on Alexa. Obviously the notification scripts shown in the automation will have to be replaced with the one you want. If you trace the commands you receive on the **sensor.video_intercom_tx** you will discover others !! For now I have identified the following.

**Automation to recognize commands**

```yaml
    - id: '1657896199804'
      alias: Notifiche dal citofono
      description: ''
      trigger:
      - platform: state
        entity_id:
        - sensor.video_intercom_tx
      action:
      - choose:
        - conditions:
          - condition: state
            entity_id: sensor.video_intercom_tx
            state: '*8*21*10##'
          sequence:
          - service: script.notifica_voce_evento
            data:
              notification_message: "La luce scala è stata attivata"
          - service: script.notifica_testo_evento
            data:
              notification_message: "La luce scala è stata attivata"
        - conditions:
          - condition: state
            entity_id: sensor.video_intercom_tx
            state: '*8*19*20##'
          sequence:
          - service: script.notifica_voce_evento
            data:
              notification_message: "Il cancelletto è stato aperto"
          - service: script.notifica_testo_evento
            data:
              notification_message: "Il cancelletto è stato aperto"
        - conditions:
          - condition: state
            entity_id: sensor.video_intercom_tx
            state: '*8*1#5#4#20*10##'
          sequence:
          - service: script.notifica_voce_evento
            data:
              notification_message: "La telecamera del videocitofono è stata accesa"
          - service: script.notifica_testo_evento
            data:
              notification_message: "La telecamera del videocitofono è stata accesa"
        - conditions:
          - condition: state
            entity_id: sensor.video_intercom_tx
            state: '*8*3#5#4*420##'
          sequence:
          - service: script.notifica_voce_evento
            data:
              notification_message: "La telecamera del videocitofono è stata spenta"
          - service: script.notifica_testo_evento
            data:
              notification_message: "La telecamera del videocitofono è stata spenta"
        - conditions:
          - condition: state
            entity_id: sensor.video_intercom_tx
            state: '*8*1#1#4#21*10##'
          sequence:
          - service: script.notifica_voce_evento
            data:
              notification_message: "Qualcuno ha suonato al citofono."
          - service: script.notifica_testo_evento
            data:
              notification_message: "Qualcuno ha suonato al citofono."
        - conditions:
          - condition: state
            entity_id: sensor.video_intercom_tx
            state: '*7*59#12#0#0*##'
          sequence:
          - service: script.notifica_voce_evento
            data:
              notification_message: "Chiamata interna al citofono!"
          - service: script.notifica_testo_evento
            data:
              notification_message: "Chiamata interna al citofono!"
        default: []
      mode: single
```

## Author

Telegram user: "Cico ScS" with some improvements of [@fquinto](https://github.com/fquinto/)
