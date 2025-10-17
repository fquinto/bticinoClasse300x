#!/usr/bin/env python
# -*- coding: utf-8 -*-

# Author   Version  Date        Comments
# FQuinto  0.0.1    2023-09-28  First version

"""Home Assistant API."""


__version__ = '0.0.1'

import time
import socket
import logging
import os
import json
import configparser
import base64
from logging.handlers import RotatingFileHandler
# Better is using lxml but difficult to install
from xml.etree import ElementTree
# from lxml import html, etree
from flask import Flask, jsonify, send_from_directory


class HomeAssistantAPI:
    """Home Assistant API for Bticino."""
    def __init__(self):
        self.app = Flask(__name__)
        self.setuplogging()
        # Define routes
        self.app.add_url_rule('/', 'main_menu', self.main_menu)
        self.app.add_url_rule('/load', 'load', self.load)
        self.app.add_url_rule('/unlock', 'unlock', self.unlock)
        self.app.add_url_rule('/reboot', 'reboot', self.reboot)
        self.app.add_url_rule('/fwupgrade', 'fwupgrade',
                              self.read_xml_fwupgrade)
        self.app.add_url_rule('/fwversion', 'fwversion', self.fwversion)
        self.app.add_url_rule('/leds', 'leds', self.leds)
        self.app.add_url_rule('/messages', 'messages',
                              self.get_message_numbers)
        self.app.add_url_rule('/messages_html', 'messages_html',
                              self.get_messages_html)
        self.app.add_url_rule('/messages/<int:m_num>', 'get_message',
                              self.get_message)
        self.app.add_url_rule('/messages/<int:m_num>/video',
                              'get_videomessage',
                              self.get_videomessage)
        self.app.add_url_rule('/messages/<int:m_num>/image',
                              'get_imagemessage',
                              self.get_imagemessage)
        # conf.xml file or stack_open.xml file
        self.app.add_url_rule('/conf/<string:conf_file>', 'get_conf',
                              self.get_conf)
        self.app.add_url_rule('/conf/<string:conf_file>/download',
                              'get_conf_download',
                              self.get_conf_download)

    def setuplogging(self):
        """Setup logging."""
        self.loggingLEVEL = None
        self.readINIfile()
        # Setup LOGGING
        switcher = {
            'error': logging.ERROR,
            'info': logging.INFO,
            'warning': logging.WARNING,
            'critical': logging.CRITICAL,
            'debug': logging.DEBUG
        }
        LOGGER_LEVEL = switcher.get(self.loggingLEVEL)
        f = ('%(asctime)s - %(name)s - [%(levelname)s] '
             + '- %(funcName)s - %(message)s')
        logging.basicConfig(level=LOGGER_LEVEL, format=f)
        self.logger = logging.getLogger(__name__)
        self.logger.setLevel(LOGGER_LEVEL)

        # Añadiendo logging rotativo
        logpath = '/var/log/ha_api.log'
        handler = RotatingFileHandler(logpath, maxBytes=10485760,
                                      backupCount=3)
        formatter = logging.Formatter(f)
        # Añadiendo el formato al handler
        handler.setFormatter(formatter)
        # Añadiendo el handler al logger
        self.logger.addHandler(handler)
        message = "Log saving in: " + logpath
        self.logger.info(message + ' version: ' + __version__)

    def readINIfile(self):
        """Read INI file."""
        config = configparser.ConfigParser()
        script_dir = os.path.dirname(__file__)
        rel_path = "./ha_config.ini"
        abs_file_path = os.path.join(script_dir, rel_path)
        config.read(abs_file_path)
        self.loggingLEVEL = config['DEFAULT']['loggingLEVEL']
        self.localfolder = config['DEFAULT']['localfolder']

    def leds(self):
        """Read leds status."""

        # Define the directory path
        leds_dir = '/sys/class/leds'

        # Initialize a dictionary to store the LED data
        led_data = {}

        # List all LED directories in the directory
        led_dirs = [d for d in os.listdir(leds_dir) if d.startswith('led_')]

        # Iterate through the LED directories
        for led_directory in led_dirs:
            brightness_f = os.path.join(leds_dir, led_directory, 'brightness')
            try:
                with open(brightness_f, 'r') as file:
                    brightness_value = file.read().strip()
                led_data[led_directory] = brightness_value
            except Exception as e:
                self.logger.error("An error occurred: " + str(e))

        # Convert the dictionary to JSON
        led_data_json = json.dumps(led_data, indent=4)
        return led_data_json

    def get_conf(self, conf_file, download=False):
        """Get conf.xml file (show)."""
        folder = '/var/tmp/'
        conf_files = ['stack_open.xml', 'conf.xml']
        if conf_file not in conf_files:
            self.logger.error("File not found: stack_open.xml or conf.xml "
                              "but " + conf_file + " found")
            return jsonify({'error': (
                'File not found: '
                'stack_open.xml or conf.xml')}), 404
        conf_file_dir = os.path.join(folder, conf_file)
        if not os.path.exists(conf_file_dir):
            return jsonify({'error': 'File not found'}), 404
        if download:
            return send_from_directory(folder, conf_file, as_attachment=True)
        else:
            return self.read_file_content(conf_file_dir)

    def get_conf_download(self, conf_file):
        """Get conf.xml file (download)."""
        return self.get_conf(conf_file, download=True)

    def get_message_numbers(self):
        """Read video messages."""
        m_dir = '/home/bticino/cfg/extra/47/messages'
        file_list = os.listdir(m_dir)
        # Sort the file list by modification timestamp (most recent first)
        file_list.sort(key=lambda x: os.path.getmtime(os.path.join(m_dir, x)),
                       reverse=True)
        m_numbers = [d for d in file_list if d.startswith('message_')]
        # add url image for every message
        for i in range(len(m_numbers)):
            num_message = m_numbers[i].split('_')[1]
            unixtime_message = self.get_message_info_param(
                num_message, 'unixtime')
            dt_unixtime = time.strftime(
                '%Y-%m-%d %H:%M:%S', time.localtime(int(unixtime_message)))
            m_folder = os.path.join(m_dir, m_numbers[i])
            image_file = os.path.join(m_folder, 'aswm.jpg')
            if os.path.exists(image_file):
                m_numbers[i] = {
                    'number': str(i),
                    'image': '/messages/' + num_message + '/image',
                    'detail': '/messages/' + num_message,
                    'date': dt_unixtime,
                    'unixtime': unixtime_message
                }
            else:
                m_numbers[i] = {
                    'number': str(i),
                    'image': '',
                    'detail': '/messages/' + num_message,
                    'date': dt_unixtime,
                    'unixtime': unixtime_message
                }
            video_file = os.path.join(m_folder, 'aswm.avi')
            if os.path.exists(video_file):
                m_numbers[i]['video'] = (
                    '/messages/' + num_message + '/video')
            else:
                m_numbers[i]['video'] = ''
        # order m_numbers by unixtime
        m_numbers = self.sort_messages(m_numbers)
        return jsonify(m_numbers)

    def get_messages_html(self):
        """Get messages in HTML format."""
        m_dir = '/home/bticino/cfg/extra/47/messages'
        file_list = os.listdir(m_dir)
        # Sort the file list by modification timestamp (most recent first)
        # Sort the file list by modification timestamp (most recent first)
        file_list.sort(key=lambda x: os.path.getmtime(os.path.join(m_dir, x)),
                       reverse=True)
        m_numbers = [d for d in file_list if d.startswith('message_')]
        # add url image for every message
        for i in range(len(m_numbers)):
            num_message = m_numbers[i].split('_')[1]
            r_message = self.get_message_info_param(num_message, 'read')
            # r_message = 0 -> unread
            # r_message = 1 -> read
            if r_message == '0':
                read_status = 'Unread'
            else:
                read_status = 'Read'
            unixtime_message = self.get_message_info_param(
                num_message, 'unixtime')
            dt_unixtime = time.strftime(
                '%Y-%m-%d %H:%M:%S', time.localtime(int(unixtime_message)))
            m_folder = os.path.join(m_dir, m_numbers[i])
            image_file = os.path.join(m_folder, 'aswm.jpg')
            if os.path.exists(image_file):
                with open(image_file, "rb") as ifile:
                    e_string = base64.b64encode(
                        ifile.read()).decode('utf-8')
                m_numbers[i] = {
                    'number': num_message,
                    'image': '/messages/' + num_message + '/image',
                    'detail': '/messages/' + num_message,
                    'base64': e_string,
                    'date': dt_unixtime,
                    'unixtime': unixtime_message,
                    'read_status': read_status
                }
            else:
                m_numbers[i] = {
                    'number': num_message,
                    'image': '',
                    'detail': '/messages/' + num_message,
                    'base64': '',
                    'date': dt_unixtime,
                    'unixtime': unixtime_message,
                    'read_status': read_status
                }
            video_file = os.path.join(m_folder, 'aswm.avi')
            if os.path.exists(video_file):
                m_numbers[i]['video'] = (
                    '/messages/' + num_message + '/video')
            else:
                m_numbers[i]['video'] = ''
        # Prepare response in HTML format
        response_html = (
            "<html><head>"
            "<title>Messages</title>"
            "</head><body><h1>Messages</h1>"
            "<ul>")
        # order m_numbers by unixtime
        m_numbers = self.sort_messages(m_numbers)
        for m in m_numbers:
            response_html += (
                "<li>Message " + m['number'] + " - "
                + m['read_status'] + " - "
                "<a href=\"" + m['detail'] + "\">Detail</a>")
            if m['image'] != '':
                response_html += (
                    " - <a href=\"" + m['image'] + "\">"
                    "<img src=\"data:image/jpg;base64," + m['base64'] + "\">"
                    "</a>")
            else:
                response_html += ' - No image'
            if m['video'] != '':
                response_html += (
                    " - <a href=\"" + m['video'] + "\">Video</a></li>")
            else:
                response_html += ' - No video</li>'
        response_html += (
            "</ul>"
            "</body></html>")
        return response_html

    def sort_messages(self, m_numbers):
        """Sort messages."""
        # sort using unixtime from date
        m_numbers.sort(key=lambda x: x['unixtime'], reverse=True)
        return m_numbers

    def get_message_info(self, m_num):
        """Read info message."""
        message_info = {}
        e_string = ''
        m_dir = '/home/bticino/cfg/extra/47/messages'
        m_folder = os.path.join(m_dir, 'message_' + str(m_num))
        if not os.path.exists(m_folder):
            return (message_info, e_string)
        info_file = os.path.join(m_folder, 'msg_info.ini')
        if os.path.exists(info_file):
            config = configparser.ConfigParser()
            config.read(info_file)
            if 'Message Information' in config:
                message_info = dict(config['Message Information'])
        # Assuming the image file is named aswm.jpg
        image_file = os.path.join(m_folder, 'aswm.jpg')
        if not os.path.exists(image_file):
            return (message_info, e_string)
        # convert image file to data base64
        with open(image_file, "rb") as ifile:
            e_string = base64.b64encode(ifile.read()).decode('utf-8')
        # check video file
        video_file = os.path.join(m_folder, 'aswm.avi')
        if not os.path.exists(video_file):
            message_info['video'] = ''
        else:
            message_info['video'] = '/messages/' + str(m_num) + '/video'
        return (message_info, e_string)

    def get_message_info_param(self, m_num, param):
        """Get message info param."""
        (message_info, e_string) = self.get_message_info(m_num)
        aux = 'N/A'
        if param in message_info:
            aux = message_info.get(param)
        return aux

    def get_message(self, m_num):
        """View video message html."""
        (message_info, e_string) = self.get_message_info(m_num)
        # Prepare response in HTML format
        # video_file_api = '/messages/' + str(m_num) + '/video'
        video_file_api = message_info.get('video', '')
        response_html = (
            "<html><head>"
            "<title>Message " + str(m_num) + "</title>"
            "</head><body><h1>Message " + str(m_num) + "</h1>")
        if e_string != '':
            response_html += (
                "<img src=\"data:image/jpg;base64," +
                e_string + "\">")
        else:
            response_html += 'No image'
        response_html += (
            "<h2>Message Information</h2>"
            "<ul>"
            "<li>Date: " + message_info.get('date', 'N/A') + "</li>"
            "<li>MediaType: " + message_info.get('mediatype', 'N/A') + "</li>"
            "<li>EuAddr: " + message_info.get('euaddr', 'N/A') + "</li>"
            "<li>Cause: " + message_info.get('cause', 'N/A') + "</li>"
            "<li>Status: " + message_info.get('status', 'N/A') + "</li>"
            "<li>UnixTime: " + message_info.get('unixtime', 'N/A') + "</li>"
            "<li>Read: " + message_info.get('read', 'N/A') + "</li>"
            "<li>Duration: " + message_info.get('duration', 'N/A') + "</li>")
        if video_file_api != '':
            response_html += (
                "<li>Video: <a href=\"" + video_file_api +
                "\">Download</a></li>")
        else:
            response_html += "<li>Video not found</li>"
        response_html += (
            "</ul>"
            "</body></html>")
        return response_html

    def get_videomessage(self, m_num):
        """Get video message."""
        m_dir = '/home/bticino/cfg/extra/47/messages'
        m_folder = os.path.join(m_dir, 'message_' + str(m_num))
        if not os.path.exists(m_folder):
            return jsonify({'error': 'Message not found'}), 404
        # Assuming the video file is named aswm.avi
        video_file = os.path.join(m_folder, 'aswm.avi')
        if not os.path.exists(video_file):
            return jsonify({'error': 'Video not found'}), 404
        return send_from_directory(m_folder, 'aswm.avi', as_attachment=True)

    def get_imagemessage(self, m_num):
        """Get image message."""
        m_dir = '/home/bticino/cfg/extra/47/messages'
        m_folder = os.path.join(m_dir, 'message_' + str(m_num))
        if not os.path.exists(m_folder):
            return jsonify({'error': 'Message not found'}), 404
        # Assuming the image file is named aswm.jpg
        image_file = os.path.join(m_folder, 'aswm.jpg')
        if not os.path.exists(image_file):
            return jsonify({'error': 'Image not found'}), 404
        return send_from_directory(m_folder, 'aswm.jpg', as_attachment=True)

    def read_xml_fwupgrade(self):
        """Read XML file."""
        file = '/home/bticino/cfg/extra/FW/meta.xml'
        xml_content = ElementTree.parse(file)
        root = xml_content.getroot()
        # Access the data
        portal_version_fe = root.find(".//front_end").text
        portal_version_scheduler = root.find(".//scheduler").text
        binary = root.find(".//binary").text
        url = root.find(".//url").text
        md5 = root.find(".//checksum").text
        ref = root.find(".//ref").text
        brand = root.find(".//brand").text
        platform = root.find(".//platform").text
        label = root.find(".//label").text
        description = root.find(".//description").text
        # return in json format
        json_result = {
            'portal_version_front_end': portal_version_fe,
            'portal_version_scheduler': portal_version_scheduler,
            'binary': binary,
            'url': url,
            'md5': md5,
            'ref': ref,
            'brand': brand,
            'platform': platform,
            'label': label,
            'description': description
        }
        return json_result

    def fwversion(self):
        """Read actual firmware version."""
        fn = '/home/bticino/cfg/extra/.license_ver'
        with open(fn, 'r') as f:
            version = f.read()
        json_result = {
            'version': version.rstrip('\n')
        }
        return json_result

    def read_file_content(self, file_path):
        """Read file content."""
        try:
            with open(file_path, 'r') as file:
                return file.read()
        except Exception as e:
            return str(e)

    def get_cpu_temperature(self):
        """Get CPU temperature."""
        temp_file_path = "/sys/class/thermal/thermal_zone0/temp"
        try:
            temp = self.read_file_content(temp_file_path)
            temp_c = int(temp) / 1000
            return temp_c
        except ValueError:
            return None

    def get_load_average(self):
        """Get load average."""
        load_file_path = "/proc/loadavg"
        x = self.read_file_content(load_file_path)
        return x.rstrip('\n')

    def main_menu(self):
        """Main menu."""
        # Show the main menu options API
        response = (
            "<h1>Home Assistant API</h1>"
            "<small>Version: " + __version__ + "</small><br><br>"
            "<a href=\"/load\">Load</a><br>"
            "<a href=\"/unlock\">Unlock</a><br>"
            "<a href=\"/reboot\">Reboot</a><br>"
            "<a href=\"/fwupgrade\">Firmware Upgrade</a><br>"
            "<a href=\"/fwversion\">Firmware Version</a><br>"
            "<a href=\"/leds\">Leds</a><br>"
            "<a href=\"/messages\">JSON Messages</a> - "
            "<a href=\"/messages_html\">HTML Messages</a><br>"
            "<h2>Configuration files</h2>"
            "<a href=\"/conf/conf.xml\">View conf.xml</a>"
            " - <a href=\"/conf/conf.xml/download\">Download conf.xml</a><br>"
            "<a href=\"/conf/stack_open.xml\">View stack_open.xml</a>"
            " - <a href=\"/conf/stack_open.xml/download\">Download "
            "stack_open.xml</a><br>"
        )
        return response

    def load(self):
        """Read load average and CPU temperature."""
        temp_c = self.get_cpu_temperature()
        load = self.get_load_average()
        # Prepare response in JSON format
        response = "{\n"
        response += "\"cpu_temperature\": " + str(temp_c) + ",\n"
        response += "\"load\": \"" + load + "\"\n"
        response += "}"
        return response

    def unlock(self):
        """Open door."""
        json_result = None
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
                json_result = {
                    'result': 'ok'
                }
            else:
                self.logger.error("Door not opened: " + str(r2))
                json_result = {
                    'result': 'nok'
                }
        else:
            self.logger.error("Door not opened: " + str(r))
            json_result = {
                    'result': 'nok'
                }
        return json_result

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
        except Exception as e:
            self.logger.error("An error occurred: " + str(e))
            return None  # Return None if an error occurs

    def reboot(self):
        """Reboot device."""
        self.logger.info("Rebooting device")
        os.system("/sbin/shutdown -r now")

    def run(self):
        """Run API."""
        self.app.run(host='0.0.0.0', port=5000)


if __name__ == '__main__':
    app_instance = HomeAssistantAPI()
    app_instance.run()

# flask --app hello run
