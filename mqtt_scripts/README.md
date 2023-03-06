# Publish the BTicino C100X/C300X commands in MQTT

The goal of this guide is to export to an MQTT broker (mosquitto or others) the commands managed by the BTicino C100X/C300X video door entry unit, in order to allow integration of the system into a home automation software (eg: Homeassistant).

These scripts use utilities already present in the video door entry unit (mosquitto_pub, mosquitto_sub, python3, tcpdump), in fact it is not necessary to install any additional software.
To export the commands of the video door entry unit to the network in MQTT I have prepared the following script files

## Files explanation

### TcpDump2Mqtt

This is the main script that checks every 10 minutes:
* that the script for publishing and receiving commands are active and alternatively executes them.
* if the gateway to which the video door entry unit is connected can be reached. The management of the polling to the gateway has been implemented because if the connection with the Wifi is lost, the **StartMqttSend** and **StartMqttReceive** scripts would no longer work. If the gateway is not reachable, the currently active scripts are killed and then run again when the connection is restored. In my specific case, I turn off the WiFi in the evening to reactivate it in the morning, and thus I have solved in this way the problem of blocking the scripts on disconnection.

### TcpDump2Mqtt.conf

Contains the following configuration parameters for the main script operation:

* **MQTT_HOST**: IPv4 address of the destination MQTT broker (mosquitto or others).
* **MQTT_USER**: Username to authenticate to the destination MQTT broker. If specified, authentication will be used.
* **MQTT_PASS**: Password to authenticate to the destination MQTT broker.
* **TOPIC_RX** (default "Bticino/rx"): MQTT topic with which the home automation software (eg: Homeassistant) sends commands to the video door entry unit (eg: gate opening, light activation, ..).
* **TOPIC_DUMP** (default "Bticino/tx"): MQTT topic with which the video door entry unit sends all the commands that are executed from the internal unit, from the application or from the external unit.
* **TOPIC_STARTD** (default "Bticino/start_date"): MQTT topic which is updated with the date and time of activation of the script
* **TOPIC_LASTWILL** (default "Bticino/LastWillT"): MQTT topic set to "online" or "offline" in case of connection / disconnection of the video door entry unit from the WiFi network.

### StartMqttSend

This script listens to the network traffic of the video door entry unit (commands from the app, from the internal unit, from the external unit), filters the packets and extracts the commands, sending them to the MQTT broker through the topic defined in the TOPIC_DUMP variable of the main script TcpDump2Mqtt.

### StartMqttReceive

This script listens to the commands coming from the broker, therefore from the home automation software and, through the MQTT topic defined in the TOPIC_RX variable of the main script TcpDump2Mqt, it executes them on the video door entry unit.

### TcpDump2Mqtt.sh

This script is only used to launch the main TcpDump2Mqtt script in background mode. A symbolic link to this file is created in the /etc/rc5.d folder to automatically start on boot.

### filter.py

This file is in python and is used by the StartMqttSend script to filter network packets by eliminating unnecessary parts and isolating the commands to be sent to the home automation software.

## Insertion of scripts in the video door entry unit

**NOTE**: In order to insert the scripts in the video door entry unit, SSH must be enabled according to the guide of [@fquinto](https://github.com/fquinto/) [https://github.com/fquinto/bticinoClasse300x](https://github.com/fquinto/bticinoClasse300x)

**NOTE #2**: If you follow the "new" procedure in order to create the firmware (the one with the activation of the MQTT server), you don't need to go throught all the steps: go directly to the end of the following procedure

1. Download files described above to a folder on your PC:
	* TcpDump2Mqtt.sh
	* TcpDump2Mqtt.conf
	* TcpDump2Mqtt
	* StartMqttSend
	* StartMqttReceive
	* filter.py

2. Open the __TcpDump2Mqtt.conf__ file with an editor and modify at least the parameter **MQTT_HOST** with the IP addresse of your destination MQTT broker.

3. Transfer the files from the PC to the video door entry unit.
  The files will be transferred to the /tmp folder of the video door entry unit as the rest of the filesystem is read-only. For the transfer you can use the scp command present in linux and windows 10 (from a "Command Prompt" window).

    **Transfer of files to the video door entry unit**

    ```sh
    scp TcpDump2Mqtt root2@<intercom_ip>:/tmp/TcpDump2Mqtt
    scp TcpDump2Mqtt.conf root2@<intercom_ip>:/tmp/TcpDump2Mqtt.conf
    scp TcpDump2Mqtt.sh root2@<intercom_ip>:/tmp/TcpDump2Mqtt.sh
    scp StartMqttSend root2@<intercom_ip>:/tmp/StartMqttSend
    scp StartMqttReceive root2@<intercom_ip>:/tmp/StartMqttReceive
    scp filter.py root2@<intercom_ip>:/tmp/filter.py
    ```

    **NOTE**: <intercom_ip> is the IP address of the video intercom door entry unit.

    When prompted for a password, enter the one you defined in the SSH enabling procedure.

4. Once the 6 files have been transferred, connect via the terminal to the video door entry unit and execute the following commands

    ```sh
    # Move to the /tmp folder where we transferred the files.
    cd /tmp

    # Change the permissions of the scripts to make them executable.
    chmod 755 TcpDump2Mqtt TcpDump2Mqtt.sh StartMqttSend StartMqttReceive

    # Make the filesystem writable.
    mount -oremount, rw /
    
    # Create the destination directory and transfer the files there.
    mkdir /etc/tcpdump2mqtt    
    cp ./TcpDump2Mqtt /etc/tcpdump2mqtt
    cp ./TcpDump2Mqtt.conf /etc/tcpdump2mqtt
    cp ./TcpDump2Mqtt.sh /etc/tcpdump2mqtt
    cp ./StartMqttSend /etc/tcpdump2mqtt
    cp ./StartMqttReceive /etc/tcpdump2mqtt

    # Transfer the python file to the /home/root directory.
    cp ./filter.py /home/root

    # Move to the /etc/rc5.d folder.
    cd /etc/rc5.d

    # Create the symbolic link for autorun on startup.
    ln -s ../tcpdump2mqtt/TcpDump2Mqtt.sh S99TcpDump2Mqtt

    # Modify flexisipsh service to create a file in /tmp/ folder when flexisip restart
    cp /etc/init.d/flexisipsh /etc/init.d/flexisipsh_bak
    awk 'NR == 25 {$0="\t/bin/touch /tmp/flexisip_restarted\n\t;;"} 1' /etc/init.d/flexisipsh > /etc/init.d/flexisipsh_new
    mv /etc/init.d/flexisipsh_new /etc/init.d/flexisipsh
    chmod 775 /etc/init.d/flexisipsh
    chown bticino:bticino /etc/init.d/flexisipsh

    # Make the filesystem read-only again.
    mount -oremount, ro /

    # Restart the video door entry unit.
    reboot
    ```

## New modified procedure (see note 2 above)
1. Once you uploaded the new firmware, establish a connection with your intercom with SSH
    ```sh
    ssh root2@<intercom_ip>
    ```
   If you're using a mac (OSx) 
    ```sh
    # First create a RSA key if you never done before
    ssh-keygen -t rsa
    
    # Do the connection
    ssh -oHostKeyAlgorithms=+ssh-rsa root2@<intercom_ip>
    ```
2. proceed with all the following

    ```sh
    # Move to the folder
    cd /etc/tcpdump2mqtt
    
    # Make the filesystem writable.
    mount -oremount, rw /  
    
    # Modify the config file with your MQTT parameters (server, user and password)
    vi TcpDump2Mqtt.conf 
    
    # Make the filesystem read-only again.
    mount -oremount, ro /

    # Restart the video door entry unit.
    reboot    
    ```

## Managing the unit remotely from Homeassistant

Our video door entry unit is now sending / receiving any commands to the MQTT broker.

**NOTE**: In order to manage MQTT topics in Homeassistant it is necessary to have the MQTT integration installed.

### Basic configuration

In the Homeassistant **configuration.yaml** file, in the **mqtt:** block it is necessary to insert the following lines to instruct MQTT to receive / transmit on the topics we have defined in the scripts:

```yaml
mqtt:
  sensor:
    - unique_id: '14532784978700'
      name: "Video intercom TX"
      state_topic: "Bticino/tx"
      availability_topic: "Bticino/LastWillT"
      icon: mdi:phone-outgoing

    - unique_id: '13454564689485'
      name: "Video intercom RX"
      state_topic: "Bticino/rx"
      availability_topic: "Bticino/LastWillT"
      icon: mdi:phone-incoming
```

### Automations

We need to create automations that allow us to interact with the video door entry unit.

#### Open the door

The following automation creates a button that allows the gate to be opened and creates a notification in the Homeassistant notification area.

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

#### Recognize the commands

The following automation recognizes some commands received from the video door entry unit and notifies the event via text and voice on Alexa. Obviously the notification scripts shown in the automation will have to be replaced with the one you want. If you trace the commands you receive on the **sensor.video_intercom_tx** you will discover others !! For now I have identified the following:

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

Telegram user: "Cico ScS" with some improvements of [@fquinto](https://github.com/fquinto/) and [@gzone](https://github.com/gzone156)
