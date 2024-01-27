# Publish the BTicino C100X/C300X commands in MQTT

The goal of this guide is to export to an MQTT broker (mosquitto or others) the commands managed by the BTicino C100X/C300X video door entry unit, in order to allow integration of the system into a home automation software (eg: Homeassistant).

These scripts use utilities already present in the video door entry unit (mosquitto_pub, mosquitto_sub, python 2/3, tcpdump), in fact it is not necessary to install any additional software.

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

## Author

Telegram user: "Cico ScS" with some improvements of [@fquinto](https://github.com/fquinto/) and [@gzone](https://github.com/gzone156)
