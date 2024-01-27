#!/usr/bin/env python
# -*- coding: utf-8 -*-


"""Home Assistant control."""

import time
import datetime
import os
import subprocess
import sys
import glob
import socket
import logging
from logging.handlers import RotatingFileHandler
import uuid
import json
import ipaddress
import configparser
import threading
import signal
# NOTE: Better is using lxml but difficult to install inside Bticino-Legrand
from xml.etree import ElementTree
# from lxml import html, etree
import paho.mqtt.client as mqtt
import pyinotify

# try:
#     import schedule
# except Exception as e:
#     print('Install "pip install --upgrade schedule"\n' + str(e))
#     sys.exit(1)

__version__ = "0.0.2"


class EventHandler(pyinotify.ProcessEvent):
    """Class of GPIO and LEDs detection event handler."""
    def __init__(self, logger, client):
        super().__init__()
        self.logger = logger
        self.client = client

    def process_default(self, event):
        """Process default."""
        value = None
        if event.pathname.startswith('/sys/class/leds'):
            topic = 'video_intercom/leds/state'
            # For changes in symbolic links pointing to LED brightness
            print("Change detected in symbolic link: %s", event.pathname)
            # Read and print the content of the brightness file
            with open(event.pathname, 'r', encoding='utf-8') as f:
                value = f.read().strip()
                print("Brightness value: %s", value)
        elif event.pathname.startswith('/sys/class/gpio'):
            topic = 'video_intercom/gpio/state'
            # For changes in GPIO value files
            print("Change detected in GPIO value file: %s", event.pathname)
            # Read and print the content of the value file
            with open(event.pathname, 'r', encoding='utf-8') as f:
                value = f.read().strip()
                print("Value: %s", value)
        # Sent MQTT
        message = json.dumps({"event_pathname": event.pathname, "value": value})
        self.client.publish(topic, message)
        self.logger.info('GPIO or LED: ' + event.pathname + ' = ' + value)


class GPIOLEDsDetectionThread(threading.Thread):
    """Class of GPIO and LEDs detection thread."""
    def __init__(self, logger, client, event):
        super().__init__()
        self.logger = logger
        self.client = client
        self.stop_event = event

    def stop(self):
        """Stop thread."""
        self.stop_event.set()
        self.logger.info('Thread keydetection stopped')

    def run(self):
        """Thread for key detection."""
        self.logger.info('Thread keydetection started')
        led_symlink_pattern = '/sys/class/leds/*/brightness'
        gpio_file_pattern = '/sys/class/gpio/*/value'

        # Initialize an inotify watcher
        wm = pyinotify.WatchManager()

        # Define the events you want to watch for (symbolic link modification)
        mask = pyinotify.IN_MODIFY

        # Initialize the notifier
        notifier = pyinotify.Notifier(wm, EventHandler(self.logger, self.client))

        # Add watches for symbolic links and files matching the patterns
        for symlink_path in glob.glob(led_symlink_pattern):
            wdd = wm.add_watch(symlink_path, mask)
        for gpio_file_path in glob.glob(gpio_file_pattern):
            wdd = wm.add_watch(gpio_file_path, mask)
        if wdd:
            # Start the notifier loop to watch for changes
            notifier.loop()


class KeyDetectionThread(threading.Thread):
    """Class of key detection thread."""
    def __init__(self, logger, selector, client, event):
        super().__init__()
        self.logger = logger
        self.selector = selector
        self.client = client
        self.stop_event = event

    def stop(self):
        """Stop thread."""
        self.stop_event.set()
        self.logger.info('Thread keydetection stopped')

    def run(self):
        """Thread for key detection."""
        self.logger.info('Thread keydetection started')
        selector = self.selector
        client = self.client
        while not self.stop_event.is_set():
            key_used = None
            selectors = selector.select(timeout=0.1)
            for key, mask in selectors:
                device = key.fileobj
                if self.stop_event.is_set():
                    break
                events = device.read()
                for event in events:
                    if self.stop_event.is_set():
                        break
                    etype = event.type
                    ecode = event.code
                    evalue = event.value
                    if etype == 1 and ecode == 2:
                        if evalue == 1:
                            # self.logger.info('Key1 pressed (key)')
                            key_used = 'key_PRESS'
                        else:
                            # self.logger.info('Key1 released (key)')
                            key_used = 'key_RELEASE'
                    elif etype == 1 and ecode == 3:
                        if evalue == 1:
                            # self.logger.info('Key2 pressed (star)')
                            key_used = 'star_PRESS'
                        else:
                            # self.logger.info('Key2 released (star)')
                            key_used = 'star_RELEASE'
                    elif etype == 1 and ecode == 4:
                        if evalue == 1:
                            # self.logger.info('Key3 pressed (eye)')
                            key_used = 'eye_PRESS'
                        else:
                            # self.logger.info('Key3 released (eye)')
                            key_used = 'eye_RELEASE'
                    elif etype == 1 and ecode == 5:
                        if evalue == 1:
                            # self.logger.info('Key4 pressed (phone)')
                            key_used = 'phone_PRESS'
                        else:
                            # self.logger.info('Key4 released (phone)')
                            key_used = 'phone_RELEASE'
                    # else:
                        # self.logger.info('Event: ' + str(event))
                        # self.logger.info('Event type: ' + str(etype))
                        # self.logger.info('Event code: ' + str(ecode))
                        # self.logger.info('Event value: ' + str(evalue))
                        # print(event)
                        # print(evdev.categorize(event))
            if key_used:
                topic = 'video_intercom/keypad/state'
                message = json.dumps({"event_type": key_used})
                client.publish(topic, message)
                self.logger.info('Sent key: ' + topic + ' ' + message)
            time.sleep(0.1)
            if self.stop_event.is_set():
                break
        self.logger.info('Thread keydetection finished')


class Control:
    """Class of mqtt control."""

    def __init__(self):
        """Start the class."""
        # vars
        self.stop_event = threading.Event()
        self.child_thread = None
        self.stop_main_thread = False
        # self.detect_execution()
        self.setuplogging()
        self.create_vars()
        self.check_certs_exist()
        self.normal_execution()
        self.mls = None
        self.rs = None
        self.model = None
        self.logging_level = None
        self.localfolder = None
        self.ca_cert = None
        self.certfile = None
        self.keyfile = None
        self.enable_tls = None
        self.host_mqtt = None
        self.port_mqtt = None
        self.u_mqtt = None
        self.p_mqtt = None

    def detect_execution(self):
        """Detect execution and continue or finnish."""
        cmd = 'pgrep -f python -a'
        pid = os.popen(cmd).read()
        # num_pid_python = pid.count('\n')
        process = pid.split('\n')
        found = 0
        for p in process:
            if 'mqtt_control.py' in p:
                found += 1
        if found > 2:
            print('Exit without any execution')
            exit()

    def setuplogging(self):
        """Setup logging."""
        self.logging_level = None
        self.read_ini_file()
        # Setup LOGGING
        switcher = {
            'error': logging.ERROR,
            'info': logging.INFO,
            'warning': logging.WARNING,
            'critical': logging.CRITICAL,
            'debug': logging.DEBUG
        }
        logger_level = switcher.get(self.logging_level)
        f = ('%(asctime)s - %(name)s - [%(levelname)s] '
             + '- %(funcName)s - %(message)s')
        logging.basicConfig(level=logger_level, format=f)
        self.logger = logging.getLogger(__name__)
        self.logger.setLevel(logger_level)

        # Añadiendo logging rotativo
        logpath = '/var/log/ha_control.log'
        handler = RotatingFileHandler(logpath, maxBytes=10485760,
                                      backupCount=3)
        formatter = logging.Formatter(f)
        # Añadiendo el formato al handler
        handler.setFormatter(formatter)
        # Añadiendo el handler al logger
        self.logger.addHandler(handler)
        self.logger.info('Log saving in %s version: %s', logpath, __version__)

    def create_vars(self):
        """First creation of vars."""
        self.r = str(socket.gethostname())
        # Sample: 'C3X-00-03-50-00-00-00-9999999'
        # MODEL-MAC_ADRESS-SERIAL_NUMBER
        mac = int(uuid.getnode())
        self.mac_address = (':'.join(("%012X" % mac)[i:i + 2]
                            for i in range(0, 12, 2)))
        mac_extract = self.r[4:21].upper().replace('-', ':')
        self.serial_number = self.r[22:29]
        self.model = self.r[0:3]

        if self.mac_address == mac_extract:
            self.logger.info(
                'MAC address is correct in network and hostname '
                'is : %s', self.mac_address)
        else:
            self.logger.warning(
                'MAC address is different: %s'
                ' and %s', self.mac_address, mac_extract)
        # Sample: 'C3X-9999999'
        self.id = self.model + '-' + self.serial_number

    def setupkeydetection(self):
        """Setup key detection."""
        from evdev import InputDevice
        from selectors import DefaultSelector, EVENT_READ
        selector = DefaultSelector()
        gpios = InputDevice('/dev/input/event0')
        selector.register(gpios, EVENT_READ)
        return selector

    def multicast_listener(self):
        """Multicast listener."""
        # mls = multicast listener socket
        self.mls = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        self.mls.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)

        # Bind the socket to the desired address and port
        self.mls.bind(("0.0.0.0", 7667))

        # Join the multicast group
        multicast_group = "239.255.76.67"
        self.mls.setsockopt(
            socket.IPPROTO_IP,
            socket.IP_ADD_MEMBERSHIP,
            socket.inet_aton(multicast_group) + socket.inet_aton("0.0.0.0"))

        # Set up event handling
        self.mls.settimeout(1.0)
        self.logger.info("Multicast listener socket created")

    def rawsocket(self):
        """Listener raw socket."""
        # rs = raw socket
        self.rs = socket.socket(
            socket.AF_PACKET,
            socket.SOCK_RAW, socket.ntohs(0x0003))
        # Set up event handling
        self.rs.settimeout(1.0)
        self.logger.info("Raw socket created")

    def parse_packet2(self, packet):
        """Parse packet."""
        result = None
        hex_data = packet.hex()

        # Find the starting and ending indices of the desired portion
        start_index = hex_data.find('2a')  # 2a = '*' character
        # 2323 = '##' characters, include them in the extracted data
        end_index = hex_data.find('2323') + 4

        # Extract the desired portion of the hexadecimal data
        desired_hex_data = hex_data[start_index:end_index]
        # check if extracted data is even-length
        if len(desired_hex_data) % 2 == 0:
            try:
                # Decoding the extracted hexadecimal data
                bytes_data = bytes.fromhex(desired_hex_data)
                utf8_data = bytes_data.decode('utf-8', errors='ignore')
                result = utf8_data
            except UnicodeDecodeError as e:
                self.logger.error(
                    "Error message: %s"
                    " desired_hex_data: %s", str(e), desired_hex_data)
        return result

    def parse_cmd(self, cmd):
        """Parse cmd."""
        result = None
        # states = sensors = switches
        if cmd == '*7*73#1#100*##':
            result = 'display ON'
        elif cmd == '*7*73#1#10*##':
            result = 'display OFF'
        elif cmd == '*#8**33*0##':
            result = 'bell OFF'
        elif cmd == '*#8**#33*0##':
            result = 'bell OFF event?'
        elif cmd == '*#8**33*1##':
            result = 'bell ON'
        elif cmd == '*#8**#33*1##':
            result = 'bell ON event?'
        elif cmd == '*8*92##':
            result = 'voicemail OFF'  # vde_aswm_disabled
        elif cmd == '*#8**40*0*0*9815*1*25##':
        # *#8**40*0*0*9927*1*25##
            result = 'voicemail OFF using App'
        elif cmd == '*8*91##':
            result = 'voicemail ON'  # vde_aswm_enabled
        elif cmd == '*#8**40*1*0*9815*1*25##':
        # *#8**40*1*0*9927*1*25##
            result = 'voicemail ON using App'
        # events = triggers
        elif cmd == '*8*19*20##':
            result = 'door open button press'
        elif cmd == '*8*20*20##':
            result = 'door open button release'
        elif cmd == '*8*21*16##':
            result = 'light ON button press'
        elif cmd == '*8*22*16##':
            result = 'light ON button release'
        # others
        elif cmd == '*#*1##':
            result = 'ACK'
        elif cmd == '*#*0##':
            result = 'NACK'
        elif cmd == '*8*3#6#2*416##':
            result = '(no idea) what is this? ' + cmd
        elif cmd == '*8*80#6#2*16##':
            result = '(no idea) what is this? ' + cmd
        elif cmd == '*#130**1##':
            result = 'status request'
        return result

    def parse_packet(self, packet):
        """Parse packet."""
        result = None
        doorbell = '*8*1#1#4#21*16##'
        bin_doorbell = doorbell.encode().hex()
        open_press = '*8*19*20##'
        bin_open_press = open_press.encode().hex()
        open_release = '*8*20*20##'
        bin_open_release = open_release.encode().hex()
        # Extract packet data and source address
        p_data, address = packet
        pdathx = p_data.hex()

        # Check if it's a TCP packet
        if p_data[23] == 6:  # 6 corresponds to TCP in IP header
            # if p_data[23] != 17:  # 17 corresponds to UDP in IP header
            # Check if it's not ICMP
            if p_data[20] != 1:  # 1 corresponds to ICMP in IP header
                # Extract destination port from TCP header
                dst_port = (p_data[36] << 8) + p_data[37]
                org_port = (p_data[34] << 8) + p_data[35]

                # Check if it's not one of the excluded ports
                aports = {5007, 5060, 20000, 30006}
                bports = {5007, 5060, 20000}
                if dst_port not in aports and org_port not in bports:
                    # Print the packet data (hexadecimal representation)
                    if bin_doorbell in pdathx:
                        self.logger.debug(
                            "Packet doorbell from %s: %s"
                            " Puerto de origen: %s"
                            " Puerto de destino: %s",
                            address, pdathx, org_port, dst_port)
                        result = 'DOORBELL'
                    elif bin_open_press in pdathx:
                        self.logger.debug(
                            "Packet press from %s: %s"
                            " Puerto de origen: %s"
                            " Puerto de destino: %s",
                            address, pdathx, org_port, dst_port)
                        result = 'PRESS'
                    elif bin_open_release in pdathx:
                        self.logger.debug(
                            "Packet release from %s: %s"
                            " Puerto de origen: %s"
                            " Puerto de destino: %s",
                            address, pdathx, org_port, dst_port)
                        result = 'RELEASE'
                    else:
                        pass
        return result

    def check_certs_exist(self):
        """Check if certs exist."""
        if self.enable_tls:
            if not os.path.isfile(self.ca_cert):
                self.logger.error('ca_cert not found: %s', self.ca_cert)
                sys.exit(1)
            if not os.path.isfile(self.certfile):
                self.logger.error('client_cert not found: %s', self.certfile)
                sys.exit(1)
            if not os.path.isfile(self.keyfile):
                self.logger.error('client_key not found: %s', self.keyfile)
                sys.exit(1)

    def signal_handler(self, signum, frame):
        """Signal handler."""
        # def handler(signum, frame):
        self.logger.info('Signal handler called with signal %s', signum)
        if self.child_thread:
            self.child_thread.stop()
        self.stop_main_thread = True
        self.stop_event.set()
        # return handler

    def normal_execution(self):
        """Execute normal main."""
        # Signal
        signal.signal(signal.SIGINT, self.signal_handler)
        signal.signal(signal.SIGTERM, self.signal_handler)
        # MQTT connection
        random_id = str(uuid.uuid4())
        client = mqtt.Client(random_id)
        client.on_connect = self.on_connect
        client.on_message = self.on_message
        client.on_disconnect = self.on_disconnect
        client.on_publish = self.on_publish
        client.on_subscribe = self.on_subscribe
        if self.enable_tls:
            ca_certs = self.ca_cert
            certfile = self.certfile
            keyfile = self.keyfile
            client.tls_set(ca_certs, certfile, keyfile,
                           cert_reqs=mqtt.ssl.CERT_REQUIRED,
                           tls_version=mqtt.ssl.PROTOCOL_TLSv1_2,
                           ciphers=None)
            if self.is_valid_host(self.host_mqtt):
                self.logger.info('Host is valid')
            else:
                self.logger.error('Host is not valid, change it')
                sys.exit(1)
        if self.u_mqtt != '':
            self.logger.info('User is not empty')
            client.username_pw_set(self.u_mqtt, self.p_mqtt)
        client.connect(self.host_mqtt, self.port_mqtt, 60)
        self.logger.info('I\'m %s', self.r)
        self.now = datetime.datetime.now()

        # Init CONFIG discovery messages: use retain=True
        jsondata = self.read_xml_file_version()
        # Set config lock
        (t, m) = self.sent_mqtt_config_lock(jsondata)
        client.publish(t, m, retain=True)
        # Set config display (sensor)
        (t, m) = self.sent_mqtt_config_display(jsondata)
        client.publish(t, m, retain=True)
        # Set config voicemail (switch)
        (t, m) = self.sent_mqtt_config_voicemail(jsondata)
        client.publish(t, m, retain=True)
        # Set config doorbell sound (switch)
        (t, m) = self.sent_mqtt_config_doorbell_sound(jsondata)
        client.publish(t, m, retain=True)
        # Set config doorbell (trigger)
        (t, m) = self.sent_mqtt_config_doorbell_trigger(jsondata)
        client.publish(t, m, retain=True)
        # Set config keypad (trigger)
        (t, m) = self.sent_mqtt_config_keypad(jsondata)
        client.publish(t, m, retain=True)
        # Init multicast listener
        self.multicast_listener()
        # Init raw socket
        self.rawsocket()

        selector = self.setupkeydetection()
        # Create new thread for key detection
        # self.child_thread = threading.Thread(
        #    target=self.keydetection, args=(selector, client))
        self.child_thread = KeyDetectionThread(
            self.logger, selector, client, self.stop_event)
        # self.child_thread.run(selector, client)
        self.child_thread.start()
        # GPIO and LEDs
        self.child_thread_2 = GPIOLEDsDetectionThread(
            self.logger, client, self.stop_event)
        self.child_thread_2.start()

        r_cmd = None
        last_rcmd = None
        while True:
            # multicast listener socket
            try:
                # Adjust the buffer size as needed
                data, addr = self.mls.recvfrom(1024)
                # data
                r_cmd = self.parse_packet2(data)
                if r_cmd:
                    if r_cmd == last_rcmd:
                        pass
                    else:
                        msg_data = self.parse_cmd(r_cmd)
                        if msg_data:
                            self.logger.info(
                                "Received (mls): %s "
                                "from %s", msg_data, str(addr))
                            if msg_data == 'display ON':
                                topic = 'video_intercom/display/state'
                                message = json.dumps({"display": "ON"})
                                client.publish(topic, message)
                            elif msg_data == 'display OFF':
                                topic = 'video_intercom/display/state'
                                message = json.dumps({"display": "OFF"})
                                client.publish(topic, message)
                            elif 'voicemail ON' in msg_data:
                                topic = 'video_intercom/voicemail/state'
                                message = "ON"
                                client.publish(topic, message)
                            elif 'voicemail OFF' in msg_data:
                                topic = 'video_intercom/voicemail/state'
                                message = "OFF"
                                client.publish(topic, message)
                            elif msg_data == 'bell ON':
                                topic = 'video_intercom/doorbellsound/state'
                                message = "ON"
                                client.publish(topic, message)
                            elif msg_data == 'bell OFF':
                                topic = 'video_intercom/doorbellsound/state'
                                message = "OFF"
                                client.publish(topic, message)
                            else:
                                pass
                        else:
                            self.logger.info(
                                "Received (mls): %s"
                                " from %s", r_cmd, str(addr))
            except socket.timeout:
                pass
            except socket.error as e:
                self.logger.error("Error message (mls): %s", str(e))
            last_rcmd = r_cmd

            # raw socket
            try:
                # Adjust the buffer size as needed
                packet = self.rs.recvfrom(65535)
                # packet trigger
                trigger = self.parse_packet(packet)
                if trigger:
                    self.logger.info("Received (rs): %s", trigger)
                    if trigger == 'DOORBELL':
                        topic = 'video_intercom/doorbell/state'
                        message = trigger
                        client.publish(topic, message)
                    elif trigger == 'PRESS':
                        topic = 'video_intercom/lock/state'
                        message = 'UNLOCKING'
                        client.publish(topic, message)
                        time.sleep(1)
                        message = 'UNLOCKED'
                        client.publish(topic, message)
                    elif trigger == 'RELEASE':
                        topic = 'video_intercom/lock/state'
                        message = 'LOCKING'
                        client.publish(topic, message)
                        time.sleep(1)
                        message = 'LOCKED'
                        client.publish(topic, message)
                    else:
                        pass
            except socket.timeout:
                pass
            except socket.error as e:
                self.logger.error("Error message (rs): %s", str(e))

            try:
                client.loop_start()
            except KeyboardInterrupt:
                client.loop_stop()
                self.logger.info('Salida debido a CTRL+C')
                break
            if self.stop_main_thread:
                break
            time.sleep(0.1)

        # End while
        self.logger.info('Normal exit')
        # Wait for child thread to terminate
        if self.child_thread:
            self.child_thread.join()
        client.disconnect()
        client.loop_stop()
        self.mls.close()
        self.rs.close()

    def sent_mqtt_config_lock(self, jsondata):
        """Sent mqtt config lock."""
        # Get data
        version = jsondata['version']
        model = jsondata['model']
        self.model = model
        availability_topic = 'video_intercom/state'
        # Lock config
        # https://www.home-assistant.io/integrations/lock.mqtt/
        t_config = 'homeassistant/lock/intercom/door/config'
        m_config_json = {
            "name": "Lock",
            "unique_id": self.r + "_intercom_lock",
            "command_topic": "video_intercom/lock/set",
            "state_topic": "video_intercom/lock/state",
            "availability_topic": availability_topic,
            "payload_available": "online",
            "payload_not_available": "offline",
            "state_locked": "LOCKED",
            "state_unlocked": "UNLOCKED",
            "state_locking": "LOCKING",
            "state_unlocking": "UNLOCKING",
            "payload_lock": "LOCK",
            "payload_unlock": "UNLOCK",
            "device_class": "lock",
            "icon": "mdi:lock",
            "device": {
                "identifiers": [self.id],
                "manufacturer": "Bticino-Legrand",
                "name": "Intercom " + self.id,
                "model": model,
                "sw_version": version,
                "configuration_url": "http://" + self.get_local_ip(),
            },
            "json_attributes_topic": "video_intercom/lock/attributes",
            "json_attributes_template": "{{ value_json }}",
            "availability_template": "{{ value_json.availability | to_json }}",
            "platform": "mqtt"
        }
        # payload must be a string, bytearray, int, float or None.
        m_config = json.dumps(m_config_json)
        return (t_config, m_config)

    def sent_mqtt_config_display(self, jsondata):
        """Display config sensor."""
        # Get data
        version = jsondata['version']
        model = jsondata['model']
        self.model = model
        availability_topic = 'video_intercom/state'
        # Display config = sensor
        # https://www.home-assistant.io/integrations/sensor.mqtt/
        t_config = 'homeassistant/sensor/intercom/display/config'
        m_config_json = {
            "name": "Display",
            "unique_id": self.r + "_intercom_display",
            "state_topic": "video_intercom/display/state",
            "availability_topic": availability_topic,
            "payload_available": "online",
            "payload_not_available": "offline",
            "icon": "mdi:tablet",
            "device": {
                "identifiers": [self.id],
                "manufacturer": "Bticino-Legrand",
                "name": "Intercom " + self.id,
                "model": model,
                "sw_version": version,
                "configuration_url": "http://" + self.get_local_ip(),
            },
            "value_template": "{{ value_json.display }}",
            "json_attributes_topic": "video_intercom/display/attributes",
            "json_attributes_template": "{{ value_json }}",
            "availability_template": "{{ value_json.availability | to_json }}",
            "entity_category": "diagnostic",
            "platform": "mqtt"
        }
        m_config = json.dumps(m_config_json)
        return (t_config, m_config)

    def sent_mqtt_config_voicemail(self, jsondata):
        """Send mqtt config answer machine (voicemail) switch."""
        # Get data
        version = jsondata['version']
        model = jsondata['model']
        self.model = model
        availability_topic = 'video_intercom/state'
        # Voicemail config = switch
        # https://www.home-assistant.io/integrations/switch.mqtt/
        t_config = 'homeassistant/switch/intercom/voicemail/config'
        m_config_json = {
            "name": "Voicemail",
            "unique_id": self.r + "_intercom_voicemail",
            "command_topic": "video_intercom/voicemail/set",
            "state_topic": "video_intercom/voicemail/state",
            "availability_topic": availability_topic,
            "payload_available": "online",
            "payload_not_available": "offline",
            "state_on": "ON",
            "state_off": "OFF",
            "payload_on": "ON",
            "payload_off": "OFF",
            "icon": "mdi:voicemail",
            "device": {
                "identifiers": [self.id],
                "manufacturer": "Bticino-Legrand",
                "name": "Intercom " + self.id,
                "model": model,
                "sw_version": version,
                "configuration_url": "http://" + self.get_local_ip(),
            },
            "json_attributes_topic": "video_intercom/voicemail/attributes",
            "json_attributes_template": "{{ value_json }}",
            "availability_template": "{{ value_json.availability | to_json }}",
            "platform": "mqtt"
        }
        m_config = json.dumps(m_config_json)
        return (t_config, m_config)

    def sent_mqtt_config_doorbell_sound(self, jsondata):
        """Send mqtt config doorbell sound or not."""
        # Get data
        version = jsondata['version']
        model = jsondata['model']
        self.model = model
        availability_topic = 'video_intercom/state'
        # Doorbellsound config = switch
        # https://www.home-assistant.io/integrations/switch.mqtt/
        t_config = 'homeassistant/switch/intercom/doorbellsound/config'
        m_config_json = {
            "name": "Doorbellsound",
            "unique_id": self.r + "_intercom_doorbellsound",
            "command_topic": "video_intercom/doorbellsound/set",
            "state_topic": "video_intercom/doorbellsound/state",
            "availability_topic": availability_topic,
            "payload_available": "online",
            "payload_not_available": "offline",
            "state_on": "ON",
            "state_off": "OFF",
            "payload_on": "ON",
            "payload_off": "OFF",
            "icon": "mdi:bell",
            "device": {
                "identifiers": [self.id],
                "manufacturer": "Bticino-Legrand",
                "name": "Intercom " + self.id,
                "model": model,
                "sw_version": version,
                "configuration_url": "http://" + self.get_local_ip(),
            },
            "json_attributes_topic": "video_intercom/doorbellsound/attributes",
            "json_attributes_template": "{{ value_json }}",
            "availability_template": "{{ value_json.availability | to_json }}",
            "platform": "mqtt"
        }
        m_config = json.dumps(m_config_json)
        return (t_config, m_config)

    def sent_mqtt_config_doorbell_trigger(self, jsondata):
        """Sent mqtt config doorbell trigger."""
        # Get data
        version = jsondata['version']
        model = jsondata['model']
        self.model = model
        availability_topic = 'video_intercom/state'
        # Doorbell config = trigger
        # https://www.home-assistant.io/integrations/device_trigger.mqtt/
        t_config = 'homeassistant/device_automation/intercom/doorbell/config'
        m_config_json = {
            "name": "Doorbell",
            "unique_id": self.r + "_intercom_doorbell",
            "automation_type": "trigger",
            "type": "action",
            "subtype": "doorbell",
            "state_topic": "video_intercom/doorbell/state",
            "availability_topic": availability_topic,
            "payload_available": "online",
            "payload_not_available": "offline",
            "icon": "mdi:doorbell",
            "topic": "video_intercom/doorbell/state",
            "payload": "DOORBELL",
            "device": {
                "identifiers": [self.id],
                "manufacturer": "Bticino-Legrand",
                "name": "Intercom " + self.id,
                "model": model,
                "sw_version": version,
                "configuration_url": "http://" + self.get_local_ip(),
            },
            "json_attributes_topic": "video_intercom/doorbell/attributes",
            "json_attributes_template": "{{ value_json }}",
            "availability_template": "{{ value_json.availability | to_json }}",
            "platform": "mqtt"
        }
        m_config = json.dumps(m_config_json)
        return (t_config, m_config)

    def sent_mqtt_config_keypad(self, jsondata):
        """Sent mqtt config keypad."""
        # Get data
        version = jsondata['version']
        model = jsondata['model']
        self.model = model
        availability_topic = 'video_intercom/state'
        # keypad config = trigger
        # https://www.home-assistant.io/integrations/trigger.mqtt/
        t_config = 'homeassistant/event/intercom/keypad/config'
        m_config_json = {
            "name": "Keypad",
            "unique_id": self.r + "_intercom_keypad",
            "automation_type": "trigger",
            "type": "action",
            "subtype": "keypad",
            "state_topic": "video_intercom/keypad/state",
            "availability_topic": availability_topic,
            "payload_available": "online",
            "payload_not_available": "offline",
            "icon": "mdi:dialpad",
            "topic": "video_intercom/keypad/state",
            "device": {
                "identifiers": [self.id],
                "manufacturer": "Bticino-Legrand",
                "name": "Intercom " + self.id,
                "model": model,
                "sw_version": version,
                "configuration_url": "http://" + self.get_local_ip(),
            },
            "json_attributes_topic": "video_intercom/keypad/attributes",
            "json_attributes_template": "{{ value_json }}",
            "availability_template": "{{ value_json.availability | to_json }}",
            "platform": "mqtt"
        }
        m_config = json.dumps(m_config_json)
        return (t_config, m_config)

    def is_valid_ip(self, ip):
        """Check if IP is valid."""
        try:
            ipaddress.ip_address(ip)
            return True
        except ValueError:
            return False

    def is_valid_host(self, host):
        """Check if host is valid."""
        try:
            socket.gethostbyname(host)
            return True
        except socket.gaierror:
            return False

    def get_local_ip(self):
        """Get local IP."""
        try:
            # Create a socket object to get local IP address
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

            # Connect to a remote server (doesn't send data)
            s.connect(("8.8.8.8", 80))

            # Get the local IP address
            local_ip = s.getsockname()[0]

            return local_ip
        except socket.error as e:
            self.logger.error("An error occurred: %s", str(e))
            return None

    def get_router_ip(self):
        """Get router IP."""
        local_ip = self.get_local_ip()
        # Get the router IP address
        router_ip = local_ip[:local_ip.rfind(".")] + ".1"
        return router_ip

    def read_xml_file_version(self):
        """Read XML file."""
        file = '/home/bticino/sp/dbfiles_ws.xml'
        xml_content = ElementTree.parse(file)
        root = xml_content.getroot()
        # Access the data
        date_fw = root.find(".//date").text
        model_fw = root.find(".//webserver_type").text
        version_fw = root.find(".//ver_webserver").text
        file_fw = root.find(".//latest_sp").text
        # return in json format
        json_result = {
            'date': date_fw,
            'model': model_fw,
            'version': version_fw,
            'file': file_fw
        }
        return json_result

    def unlock(self):
        """Open door."""
        ok = '*#*1##'
        # nok = '*#*0##'
        data1 = "*8*19*20##"
        r = self.send_data(data1)
        if r == ok:
            time.sleep(1)
            data2 = "*8*20*20##"
            r2 = self.send_data(data2)
            if r2 == ok:
                self.logger.info("Door opened")
            else:
                self.logger.error("Door not opened: %s", str(r2))
        else:
            self.logger.error("Door not opened: %s", str(r))

    def voicemail(self, state):
        """Set voicemail state to ON or OFF."""
        ok = '*#*1##'
        # nok = '*#*0##'
        data0 = "*7*73#1#100*##"
        if state == 'ON':
            data = "*8*91##"
            data2 = "*#8**40*1*0*9815*1*25##"
            data3 = "*8*91*##"
        elif state == 'OFF':
            data = "*8*92##"
            data2 = "*#8**40*0*0*9815*1*25##"
            data3 = "*8*92*##"
        else:
            self.logger.error("Wrong state: %s", state)
        r0 = self.send_data(data0)
        time.sleep(0.31)
        r1 = self.send_data(data)
        time.sleep(0.31)  # 0.31 seconds (not change this value)
        r2 = self.send_data(data2)
        time.sleep(0.31)  # 0.31 seconds (not change this value)
        r3 = self.send_data(data3)
        if r0 == ok:
            r0_txt = 'OK'
        else:
            r0_txt = 'NOK'
        if r1 == ok:
            r1_txt = 'OK'
        else:
            r1_txt = 'NOK'
        if r3 == ok:
            r3_txt = 'OK'
        else:
            r3_txt = 'NOK'
        if r2 == ok:
            self.logger.info(
                'Voicemail %s: r3 %s r1 %s r0 %s',
                state, r3_txt, r1_txt, r0_txt)
            return True
        else:
            self.logger.info(
                'Voicemail not %s: r3 %s r1 %s r0 %s',
                state, r3_txt, r1_txt, r0_txt)
            return False

    def doorbell_sound(self, state):
        """Set doorbell sound state to ON or OFF."""
        ok = '*#*1##'
        # nok = '*#*0##'
        if state == 'ON':
            data = "*#8**33*1##"
        elif state == 'OFF':
            data = "*#8**33*0##"
        else:
            self.logger.error("Wrong state: %s", state)
        r = self.send_data(data)
        if r == ok:
            self.logger.info("Doorbell sound %s", state)
            return True
        else:
            self.logger.error("Doorbell sound not %s: %s", state, str(r))
            return False

    def send_data(self, data):
        """Sent data."""
        host = '127.0.0.1'
        port = 30006
        try:
            # Create a socket object
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            # Connect to the server
            sock.connect((host, port))
            # Send the data
            sock.sendall(data.encode())
            # Receive data from the socket (adjust the buffer size as needed)
            response = sock.recv(1024).decode()
            # Sleep for 1 second
            time.sleep(1)
            # Close the socket
            sock.close()
            return response  # Return the received response
        except socket.error as e:
            self.logger.error("Error occurred: %s", str(e))
            return None  # Return None if an error occurs

    def read_ini_file(self):
        """Read INI file."""
        config = configparser.ConfigParser()
        script_dir = os.path.dirname(__file__)
        rel_path = "./ha_config.ini"
        abs_file_path = os.path.join(script_dir, rel_path)
        config.read(abs_file_path)
        self.logging_level = config['DEFAULT']['logging_level']
        self.localfolder = config['DEFAULT']['localfolder']
        self.enable_tls = config['MQTT']['enableTLS']
        self.host_mqtt = config['MQTT']['host']
        self.port_mqtt = int(config['MQTT']['port'])
        self.u_mqtt = config['MQTT']['username']
        self.p_mqtt = config['MQTT']['password']
        if self.enable_tls == 'True':
            self.ca_cert = config['MQTT']['ca_cert']
            self.certfile = config['MQTT']['client_cert']
            self.keyfile = config['MQTT']['client_key']
            print('TLS enabled: ' + self.enable_tls)
            print('ca_cert: ' + self.ca_cert +
                  ' client_cert: ' + self.certfile +
                  ' client_key: ' + self.keyfile)

    def on_connect(self, client, userdata, flags, rc):
        """MQTT when connect."""
        self.logger.info("Connected with result code %s ", str(rc))

        # Subscribe to topics
        topic = "video_intercom/lock/set"
        client.subscribe(topic)
        self.logger.info("Subscribed to %s", topic)
        topic = "video_intercom/voicemail/set"
        client.subscribe(topic)
        self.logger.info("Subscribed to %s", topic)
        topic = "video_intercom/doorbellsound/set"
        client.subscribe(topic)
        self.logger.info("Subscribed to %s", topic)

        # State online when connect
        availability_topic = 'video_intercom/state'
        topic = availability_topic
        # message = json.dumps({"availability": "online"})
        message = "online"
        client.publish(topic, message)
        self.logger.info('Sent: %s %s', topic, message)

        # get_router_ip = self.get_router_ip()

        state_topic_lock = 'video_intercom/lock/state'
        topic = state_topic_lock
        message = 'LOCKED'
        client.publish(topic, message)

        topic = 'video_intercom/display/state'
        message = json.dumps({"display": "OFF"})
        client.publish(topic, message)

        topic = 'video_intercom/doorbell/state'
        message = json.dumps({"event_type": "RELEASE"})
        client.publish(topic, message)

        # state_topic_keypad = 'video_intercom/keypad/state'
        # topic = state_topic_keypad

    def on_disconnect(self, client, userdata, rc):
        """MQTT when disconnect."""
        if rc != 0:
            self.logger.error("Unexpected disconnection. R. Code %s", str(rc))
            while not client.is_connected():
                try:
                    self.logger.info("Attempting to reconnect...")
                    client.reconnect()
                    time.sleep(10)
                except Exception as e:
                    self.logger.error("Reconnection failed: %s", str(e))
        else:
            self.logger.warning("Disconected with result code %s", str(rc))

    def on_subscribe(self, client, userdata, mid, granted_qos):
        """MQTT when subcribe."""
        self.logger.debug("Subscription from %s", str(mid))

    def on_publish(self, client, userdata, mid):
        """MQTT when publish."""
        self.logger.debug("Publish from mid %s", str(mid))
        # View user data
        # self.logger.info("User data: " + str(userdata))

    def on_message(self, client, userdata, msg):
        """MQTT when message is received."""
        self.logger.info('New msg. Topic: %s %s', msg.topic, msg.payload)

        msg_payload = (msg.payload).decode("utf-8")

        if (msg.topic == 'video_intercom/voicemail/set'):
            message = None
            if msg_payload == 'ON':
                r = self.voicemail('ON')
                if r:
                    message = "ON"
                else:
                    message = "OFF"
            elif msg_payload == 'OFF':
                r = self.voicemail('OFF')
                if r:
                    message = "OFF"
                else:
                    message = "ON"
            else:
                self.logger.error('Wrong payload: %s', msg_payload)
            if message:
                topic = 'video_intercom/voicemail/state'
                client.publish(topic, message)
        elif (msg.topic == 'video_intercom/doorbellsound/set'):
            message = None
            if msg_payload == 'ON':
                r = self.doorbell_sound('ON')
                if r:
                    message = "ON"
                else:
                    message = "OFF"
            elif msg_payload == 'OFF':
                r = self.doorbell_sound('OFF')
                if r:
                    message = "OFF"
                else:
                    message = "ON"
            else:
                self.logger.error('Wrong payload: %s', msg_payload)
            if message:
                topic = 'video_intercom/doorbellsound/state'
                client.publish(topic, message)
        elif (msg.topic == 'video_intercom/lock/set'):
            topic = 'video_intercom/lock/state'
            if msg_payload == 'UNLOCK':
                # sent unlocking
                message = 'UNLOCKING'
                client.publish(topic, message)
                self.unlock()
                # sent unlocked
                message = 'UNLOCKED'
                client.publish(topic, message)
                time.sleep(1)
                # sent locked
                message = 'LOCKED'
                client.publish(topic, message)
            elif msg_payload == 'LOCK':
                # Sent locking
                message = 'LOCKING'
                client.publish(topic, message)
                time.sleep(1)
                # Sent locked
                message = 'LOCKED'
                client.publish(topic, message)
            else:
                self.logger.error('Wrong payload: %s', msg_payload)

    def is_host_available(self, host):
        """Check if host is available."""
        try:
            # Run the ping command and capture the output
            output = subprocess.check_output(["ping", "-c", "1", host])

            # Check if the output contains a successful ping
            if b"1 received" in output:
                return True
            else:
                return False
        except subprocess.CalledProcessError:
            return False

    def get_availability_device(self, ip_device):
        """Get availability of device."""
        result = self.is_host_available(ip_device)
        if result:
            r = "online"
        else:
            r = "offline"
        self.logger.info("Availability are: %s", r)
        return r


if __name__ == "__main__":
    Control()
