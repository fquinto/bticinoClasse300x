#!/usr/bin/env python
# -*- coding: utf-8 -*-

"""Prepare firmware update."""

__version__ = "0.0.13"

import shutil
import os
import sys
import time
import ipaddress
import logging
import zipfile
import tempfile
import subprocess
import gzip
import pyminizip
import wget
import re

class PrepareFirmware():
    """Firmware prepare class."""

    # Last known firmware version for C300X and C100X models
    # C300X_010717.fwz
    url_c300x_010717 = ('https://www.homesystems-legrandgroup.com/MatrixENG/liferay/'
        'bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName='
        'C300X_010717.fwz&fileId=58107.23188.15908.12349')
    # C300X_010719.fwz
    url_c300x_010719 = ('https://prodlegrandressourcespkg.blob.core.'
                 'windows.net/binarycontainer/bt_344642_3_0_0-'
                 'c300x_010719_1_7_19.bin')
    # C100X_010501.fwz
    url_c100x_010501 = ('https://www.homesystems-legrandgroup.com/MatrixENG/'
                 'liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&'
                 'fileName=C100X_010501.fwz&fileId='
                 '58107.23188.46381.34528')
    # C100X_010505.fwz
    url_c100x_010505 = ('https://www.homesystems-legrandgroup.com/MatrixENG/'
                 'liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&'
                 'fileName=C100X_010505.fwz&fileId='
                 '58107.23188.62332.48840')
    password = 'C300X'
    password2 = 'C100X'
    password3 = 'SMARTDES'

    def __init__(self):
        """First init class."""
        self.logging_level = 'debug'
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
        formatter = logging.Formatter(f)
        self.logger = logging.getLogger('prepare_firmware')
        self.logger.setLevel(logger_level)

        # Create a file handler
        fh = logging.FileHandler('prepare_firmware.log')
        fh.setLevel(logger_level)
        fh.setFormatter(formatter)
        self.logger.addHandler(fh)

        # Variables
        self.filename = None
        self.fileout = None
        self.workingdir = None
        self.prt_frmw = None
        self.use_web_firmware = None
        self.root_password = None
        self.ssh_creation = None
        self.remove_sig = None
        self.install_mqtt = None
        self.notify_new_firmware = None
        self.url = None
        self.model = None
        self.mnt_loc = '/media/mounted'

    def main(self):
        """Main function."""
        self.logger.info('Starting PrepareFirmware using version %s', __version__)
        step = 0
        while True:
            if step == 0:
                # Ask for model: C300X or C100X
                model = input('Insert model (C300X or C100X, default C300X [ENTER]): ')
                if model in ('C300X', 'c300x', ''):
                    self.model = 'c300x'
                    step = 1
                elif model in ('C100X', 'c100x'):
                    self.model = model.lower()
                    step = 1
                else:
                    print('Wrong model ❌', flush=True)
                    time.sleep(1)
            elif step == 1:
                self.logger.info('State 0 done: using model %s', self.model)
                # choose version of the firmware
                if self.model == 'c300x':
                    version = input('Insert version (1.7.17 or 1.7.19, default 1.7.19 [ENTER]): ')
                    if version in ('010717', '1.7.17'):
                        self.url = PrepareFirmware.url_c300x_010717
                        step = 2
                    elif version in ('010719', '1.7.19', ''):
                        self.url = PrepareFirmware.url_c300x_010719
                        step = 2
                    else:
                        print('Wrong version ❌', flush=True)
                        time.sleep(1)
                elif self.model == 'c100x':
                    version = input('Insert version (1.5.1 or 1.5.5, default 1.5.5 [ENTER]): ')
                    if version in ('010501', '1.5.1', ''):
                        self.url = PrepareFirmware.url_c100x_010501
                        step = 2
                    elif version in ('010505', '1.5.5', ''):
                        self.url = PrepareFirmware.url_c100x_010505
                        step = 2
                    else:
                        print('Wrong version ❌', flush=True)
                        time.sleep(1)
            elif step == 2:
                self.logger.info('State 1 done: using version %s', version)
                # Ask for firmware file
                ask = input(
                    'Do you want to download the firmware [y/Y/download] or '
                    'use an available firmware [n/N/available]? default download [ENTER]: ')
                if ask in ('y', 'Y', 'download', ''):
                    self.use_web_firmware = 'y'
                    version = self.get_version_from_url()
                    self.logger.info('Version from URL: %s', version)
                    self.filename = f'{self.model}_{version}.fwz'
                    print('The program will download the firmware: '
                        f'{self.filename}', flush=True)
                    step = 3
                elif ask in ('n', 'N', 'available'):
                    self.use_web_firmware = 'n'
                    self.filename = f'{self.model}_{version}.fwz'
                    print('We use the firmware called: '
                        f'{self.filename}', flush=True)
                    step = 3
                else:
                    print('Wrong answer ❌', flush=True)
                    time.sleep(1)
            elif step == 3:
                self.logger.info('State 2 done: using firmware on %s', self.filename)
                # Ask for root password
                self.root_password = input(
                    'Enter the BTICINO root password (pwned123): ')
                if not self.root_password:
                    self.root_password = 'pwned123'
                    print('The program will use this root password: '
                        f'{self.root_password}', flush=True)
                ask = input(
                    'Do you want to create an SSH key [y/Y/create] or '
                    'use your SSH key [n/N/me]? default is use your SSH key [ENTER]: ')
                if ask in ('y', 'Y', 'create'):
                    self.ssh_creation = 'y'
                    print('The program will create SSH key for you.', flush=True)
                    step = 4
                elif ask in ('n', 'N', 'me', ''):
                    self.ssh_creation = 'n'
                    print('We use SSH on this folder called: bticinokey and '
                        'bticinokey.pub', flush=True)
                    step = 4
                else:
                    print('Wrong answer ❌', flush=True)
                    time.sleep(1)
            elif step == 4:
                self.logger.info('State 3 done: using SSH creation: %s', self.ssh_creation)
                # Ask for sig files removal
                ask = input(
                    'Do you want to remove Sig files [y/Y/remove] or keep '
                    'them [n/N]? default remove [ENTER]: ')
                if ask in ('y', 'Y', 'remove', ''):
                    self.remove_sig = 'y'
                    print('The program will remove Sig files.', flush=True)
                    step = 5
                elif ask in ('n', 'N'):
                    self.remove_sig = ask.lower()
                    print('The program will keep Sig files.', flush=True)
                    step = 5
                else:
                    print('Wrong answer ❌', flush=True)
                    time.sleep(1)
            elif step == 5:
                self.logger.info('State 4 done: using remove sig: %s', self.remove_sig)
                # Ask for MQTT installation
                ask = input(
                    'Do you want to install MQTT [y/Y] or no [n/N]? default no [ENTER]: ')
                if ask in ('y', 'Y'):
                    self.install_mqtt = 'y'
                    print('The program will install MQTT.', flush=True)
                    step = 6
                elif ask in ('n', 'N', ''):
                    self.install_mqtt = 'n'
                    print('The program will NOT install MQTT.', flush=True)
                    step = 6
                else:
                    print('Wrong answer ❌', flush=True)
                    time.sleep(1)
            elif step == 6:
                self.logger.info('State 5 done: using install MQTT: %s', self.install_mqtt)
                # Ask for notification when new firmware is available
                ask = input(
                    'Do you want to be notified when a new firmware is available '
                    '[y/Y] or not [n/N]? default yes [ENTER]: ')
                if ask in ('y', 'Y', ''):
                    self.notify_new_firmware = 'y'
                    print('App will notify you when a new firmware is '
                        'available.', flush=True)
                    step = 7
                elif ask in ('n', 'N'):
                    self.notify_new_firmware = 'n'
                    print('App will not notify you when a new firmware is '
                        'available.', flush=True)
                    step = 7
                else:
                    print('Wrong answer ❌', flush=True)
                    time.sleep(1)
            elif step == 7:
                self.logger.info('State 6 done: notify new firmware: %s', self.notify_new_firmware)
                dt = time.strftime('%Y%m%d_%H%M%S')
                if self.install_mqtt == 'y':
                    self.fileout = f'NEW_{self.model}_{version}_MQTT_{dt}.fwz'
                else:
                    self.fileout = f'NEW_{self.model}_{version}_{dt}.fwz'
                cwd = self.process_firmware()
                # move inside folder custom_firmware
                orig = f'{cwd}/{self.fileout}'
                dest = f'custom_firmware/{self.fileout}'
                subprocess.run(['sudo', 'mv', orig, dest], check=False)
                break
            time.sleep(1)
        self.logger.info('End PrepareFirmware using version %s', __version__)

    def process_firmware(self):
        """Process firmware."""
        # Get the current working directory
        cwd = os.getcwd()

        r = self.create_temp_folder()
        self.logger.info('Created temporary folder: %s', r)
        if self.use_web_firmware == 'y':
            outfile = self.download_firmware(cwd)
            self.logger.info('Downloaded firmware: %s', outfile)
            orig = outfile
        else:
            orig = f'{cwd}/original_firmware/{self.filename}'
        dest = f'{self.workingdir}/{self.filename}'
        subprocess.run(['sudo', 'cp', orig, dest], check=False)
        self.logger.info('Copied firmware from %s to %s', orig, dest)
        filesinsidelist = self.list_files_zip()
        self.select_firmware_file(filesinsidelist)
        self.logger.info('Selected firmware file: %s', self.prt_frmw)
        self.unzip_file()
        self.logger.info('Unzipped firmware')
        self.ungz_firmware()
        if self.remove_sig == 'y':
            self.remove_sig_files()
        self.logger.info('Removed Sig files')
        self.umount_firmware()
        self.mount_firmware()
        self.logger.info('Mounted firmware')
        if self.root_password:
            root_seed = self.create_root_password(self.root_password)
            self.set_shadow_file(root_seed)
            self.set_passwd_file()
            self.logger.info('Created root password')
        if self.ssh_creation == 'y':
            self.create_ssh_key()
        elif self.ssh_creation == 'n':
            self.get_ssh_key(cwd)
        self.set_ssh_key()
        self.logger.info('Set SSH key')
        self.setup_ssh_key_rights()
        self.enable_dropbear()
        self.logger.info('Enabled dropbear')
        self.save_version(cwd, __version__)
        if self.install_mqtt == 'y':
            ok = self.prepare_mqtt(cwd)
            if ok:
                self.enable_mqtt()
            else:
                print('MQTT not installed ❌')
            self.logger.info('MQTT installed')
        if self.notify_new_firmware == 'n':
            self.disable_notify_new_firmware()
        self.umount_firmware()
        self.logger.info('Unmounted firmware')
        self.gz_firmware()
        self.logger.info('GZed firmware')
        self.zip_file_firmware(filesinsidelist)
        self.logger.info('Ziped firmware')
        self.move_ssh_key_file_firmware(cwd)
        self.logger.info('Moved SSH key file')
        self.delete_temp_folder()
        self.logger.info('Deleted temporary folder')
        self.setup_firmware_rights(cwd)
        self.logger.info('Set firmware rights')

        # return to init folder
        os.chdir(cwd)
        self.logger.info('Returned to init folder')
        return cwd

    def get_version_from_url(self, human=True):
        """Get version from URL."""
        # url in lowercase
        url = self.url.lower()
        # get version
        vtxt = url.split(self.model)[1].split('_')[1]
        if not human:
            return vtxt[0:6]
        # major version
        major = (vtxt[0:2]).lstrip('0')
        # minor version
        minor = (vtxt[2:4]).lstrip('0')
        # patch version
        patch = (vtxt[4:6]).lstrip('0')
        # version
        version = major + '.' + minor + '.' + patch
        return version

    def create_temp_folder(self):
        """Create temporary folder."""
        print('Creating temporary folder... ', end='', flush=True)
        tempdir = tempfile.mkdtemp(prefix="bticino-")
        self.workingdir = tempdir
        # Change the current working directory
        os.chdir(self.workingdir)
        print(f'created {self.workingdir} ✅')
        return tempdir

    def download_firmware(self, cwd):
        """Main function."""
        print('Downloading firmware... ', flush=True)
        # save to cwd/original_firmware/
        output = f'{cwd}/original_firmware/{self.filename}'
        # Using wget to download the file
        wget.download(self.url, output)

        # Using httpx to download the file
        # with open(self.filename, 'wb') as f:
        #     with httpx.stream("GET", url) as r:
        #         for datachunk in r.iter_bytes():
        #             f.write(datachunk)
        print(f' downloaded {self.filename} inside original_firmware ✅')
        return output

    def list_files_zip(self):
        """List of files."""
        print('Reading files inside firmware... ', end='', flush=True)
        # zip file handler
        zipfn = zipfile.ZipFile(f'{self.workingdir}/{self.filename}')
        # list available files in the container
        filesinsidelist = []
        if self.remove_sig == 'y':
            for part_firm in zipfn.namelist():
                if 'sig' not in part_firm:
                    filesinsidelist.append(part_firm)
        elif self.remove_sig == 'n':
            filesinsidelist = zipfn.namelist()
        print('done ✅')
        return filesinsidelist

    def select_firmware_file(self, filesinsidelist):
        """Select firmware file."""
        print('Selecting firmware file... ', end='', flush=True)
        # Select the firmware file
        for part_firm in filesinsidelist:
            if 'gz' in part_firm and 'recovery' not in part_firm:
                self.prt_frmw = part_firm
        print(f'important file is {self.prt_frmw} ✅')

    def unzip_file(self):
        """Un zip function."""
        print('Unzipping firmware... ', end='', flush=True)
        password = None
        zip_file = f'{self.workingdir}/{self.filename}'
        if self.model == 'c300x':
            password = PrepareFirmware.password
        elif self.model == 'c100x':
            password = PrepareFirmware.password2
        elif PrepareFirmware.password3 in zip_file:
            password = PrepareFirmware.password3
        else:
            print('No password found ❌')
            return
        if password:
            print(f'Trying to unzip with password: {password} '
                  '... (please wait arround 95 seconds) ', end='', flush=True)
            try:
                with zipfile.ZipFile(zip_file) as zf:
                    zf.extractall(pwd=bytes(password, 'utf-8'))
            except RuntimeError:
                print('Wrong password ❌')
                sys.exit(1)
        # 7z l -slt C300X_010717.fwz check is "Method = ZipCrypto Deflate"
        if self.remove_sig == 'y':
            subprocess.run(['rm', '-rf', f'{self.workingdir}/*.sig'], check=False)
        print(f'unzipped {self.filename} ✅')

    def remove_sig_files(self):
        """Will remove sig files."""
        print('Removing Sig files... ', end='', flush=True)
        subprocess.call(f'rm -rf {self.workingdir}/*.sig', shell=True)
        print('done ✅')

    def ungz_firmware(self):
        """UnGZ firmware."""
        print('UnGZ firmware... ', end='', flush=True)
        # From btweb_only.ext4.gz to btweb_only.ext4
        try:
            with gzip.open(f'{self.workingdir}/{self.prt_frmw}', 'rb') as f_in:
                with open(f'{self.workingdir}/{self.prt_frmw[:-3]}', 'wb') as f_out:
                    shutil.copyfileobj(f_in, f_out)
            print(f'unGZed {self.prt_frmw} ✅')
        except FileNotFoundError:
            print(f'file {self.prt_frmw} not found ❌')
            sys.exit(1)

    def mount_firmware(self):
        """Mount firmware."""
        print('Mounting firmware... ', end='', flush=True)
        # sudo mount -o loop btweb_only.ext4 /media/mounted/
        # Make directory mounted
        subprocess.run(['sudo', 'mkdir', '-p', self.mnt_loc], check=False)
        subprocess.call(['sudo', 'mount', '-t', 'ext4', '-o', 'loop',
                         f'{self.workingdir}/{self.prt_frmw[:-3]}',
                         self.mnt_loc])
        print(f'mounted on {self.mnt_loc} ✅')

    def create_root_password(self, password_root):
        """Create root password."""
        print('Creating root password... ', end='', flush=True)
        # openssl passwd -1 -salt root pwned123
        # r = $1$root$0i6hbFPn3JOGMeEF0LgEV1
        output = subprocess.run(['openssl', 'passwd', '-1', '-salt', 'root',
                                 password_root], check=False, capture_output=True)
        r = str((output.stdout).decode('utf-8'))
        # remove last character because it is a newline
        result = r[:-1]
        # result = r.rstrip()
        print(f'created {result} ✅')
        return result

    def append_to_file(self, filename, line1, line2):
        """Append to filename."""
        try:
            with open(filename, 'a', encoding='utf-8') as f:
                f.write(line1)
                f.write(line2)
            print('modified ✅')
            return
        except FileNotFoundError:
            print('failed because of file not found ❌')
            return
        except PermissionError:
            print('failed because of permission ❌')
            return

    def set_shadow_file(self, root_seed):
        """Set shadow file."""
        print('Setting shadow file... ', end='', flush=True)
        filename = f'{self.mnt_loc}/etc/shadow'
        line1 = f'root2:{root_seed}:18033:0:99999:7:::\n'
        line2 = f'bticino2:{root_seed}:18033:0:99999:7:::\n'
        self.append_to_file(filename, line1, line2)

    def set_passwd_file(self):
        """Set passwd file."""
        print('Setting passwd file... ', end='', flush=True)
        filename = f'{self.mnt_loc}/etc/passwd'
        line1 = 'root2:x:0:0:root:/home/root:/bin/sh\n'
        line2 = 'bticino2:x:1000:1000::/home/bticino:/bin/sh\n'
        self.append_to_file(filename, line1, line2)

    def create_ssh_key(self):
        """Create SSH key."""
        print('Creating SSH key... ', end='', flush=True)
        # ssh-keygen -t rsa -b 4096 -f /tmp/bticinokey -N ""
        savedkeyfile = f'{self.workingdir}/bticinokey'
        subprocess.run(['ssh-keygen', '-t', 'rsa', '-b', '4096',
                        '-f', savedkeyfile, '-N', ''], check=False)
        print('created ✅')

    def get_ssh_key(self, cwd):
        """Get SSH key."""
        print('Getting SSH key... ', end='', flush=True)
        fles = ['bticinokey.pub', 'bticinokey']
        for f in fles:
            subprocess.run(['sudo', 'cp', f'{cwd}/{f}',
                            f'{self.workingdir}/{f}'], check=False)
        print('files moved ✅')

    def set_ssh_key(self):
        """Set SSH key."""
        print('Setting SSH key... ', end='', flush=True)
        # sudo cp /tmp/bticinokey.pub to
        # /media/mounted/etc/dropbear/authorized_keys
        subprocess.run(['sudo', 'cp', f'{self.workingdir}/bticinokey.pub',
                        f'{self.mnt_loc}/etc/dropbear/authorized_keys'], check=False)
        # Add public file to .ssh/authorized_keys
        subprocess.run(['sudo', 'mkdir', '-p',
                        f'{self.mnt_loc}/home/root/.ssh'], check=False)
        subprocess.run(['sudo', 'cp', f'{self.workingdir}/bticinokey.pub',
                        f'{self.mnt_loc}/home/root/.ssh/authorized_keys'], check=False)
        print('set done ✅')

    def is_valid_ip(self, ip):
        """Check if IP is valid."""
        try:
            ipaddress.ip_address(ip)
            return True
        except ValueError:
            return False

    def prepare_mqtt(self, cwd):
        """Prepare MQTT."""
        result = False
        value = None
        print('Preparing MQTT... ', end='', flush=True)
        # Check .conf file if MQTT_HOST is and IP address or a domain name
        is_ip = False
        with open(f'{cwd}/mqtt_scripts/TcpDump2Mqtt.conf', 'r', encoding='utf-8') as f:
            contents = f.readlines()
        i = 0
        for i, line in enumerate(contents):
            if 'MQTT_HOST' in line:
                value = line.split('=')[1].strip()
                # check if value is empty or None
                if not value:
                    print('MQTT_HOST is empty ❌ check TcpDump2Mqtt.conf on '
                          f'line {i}')
                    return result
                r = self.is_valid_ip(value)
                if r:
                    is_ip = True
                break
        # If is_ip is True then value is an IP address and we can continue
        # if not is IP then value is a domain name and we need to get the IP
        # address from the domain name and add it to the hosts file
        if not is_ip:
            hstnme = value
            # Get ip from the hostname
            # ip = socket.gethostbyname(hstnme)
            if len(hstnme) > 255:
                print('MQTT_HOST is too long (> 255 chars) ❌ check TcpDump2Mqtt.conf on '
                      f'line {i}')
                return result
            pattern = r'^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-\_]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-\_]*[A-Za-z0-9])(\.[A-Za-z]{2,})?$'
            if not re.match(pattern, hstnme):
                print('MQTT_HOST has illegal characters ❌ check TcpDump2Mqtt.conf on '
                      f'line {i}')
                return result
            ip = input(f'Enter IP address for hostname "{hstnme}": ')
            while not self.is_valid_ip(ip):
                ip = input(f'Not valid. Enter IP address for hostname "{hstnme}": ')
                time.sleep(1)
            # add hostname to the end of the hosts file
            self.add_host_and_ip(hstnme, ip)

        # Copy file to mounted folder
        dirm = '/etc/tcpdump2mqtt'
        # Create tcpdump2mqtt directory
        subprocess.run(['sudo', 'mkdir', '-p',
                        f'{self.mnt_loc}{dirm}'], check=False)
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/TcpDump2Mqtt',
                        f'{self.mnt_loc}{dirm}/TcpDump2Mqtt'], check=False)
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mnt_loc}{dirm}/TcpDump2Mqtt'], check=False)
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/TcpDump2Mqtt.conf',
                        f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.conf'], check=False)
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/TcpDump2Mqtt.sh',
                        f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.sh'], check=False)
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.sh'], check=False)
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/StartMqttSend',
                        f'{self.mnt_loc}{dirm}/StartMqttSend'], check=False)
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mnt_loc}{dirm}/StartMqttSend'], check=False)
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/StartMqttReceive',
                        f'{self.mnt_loc}{dirm}/StartMqttReceive'], check=False)
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mnt_loc}{dirm}/StartMqttReceive'], check=False)
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/filter.py',
                        f'{self.mnt_loc}/home/root/filter.py'], check=False)
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mnt_loc}/home/root/filter.py'], check=False)
        subprocess.run(['sudo', 'cp', f'{self.mnt_loc}/etc/init.d/flexisipsh',
                        f'{self.mnt_loc}/etc/init.d/flexisipsh_bak'], check=False)

        # If extis file m2mqtt_ca.crt copy it
        if os.path.isfile(f'{cwd}/certs/m2mqtt_ca.crt'):
            subprocess.run(['sudo', 'cp', f'{cwd}/certs/m2mqtt_ca.crt',
                            f'{self.mnt_loc}/etc/ssl/certs/m2mqtt_ca.crt'], check=False)
        # If extis file m2mqtt_srv_bticino.crt copy it
        if os.path.isfile(f'{cwd}/certs/m2mqtt_srv_bticino.crt'):
            subprocess.run([
                'sudo', 'cp', f'{cwd}/certs/m2mqtt_srv_bticino.crt',
                f'{self.mnt_loc}{dirm}/m2mqtt_srv_bticino.crt'], check=False)
        # If extis file m2mqtt_srv_bticino.key copy it
        if os.path.isfile(f'{cwd}/certs/m2mqtt_srv_bticino.key'):
            subprocess.run([
                'sudo', 'cp', f'{cwd}/certs/m2mqtt_srv_bticino.key',
                f'{self.mnt_loc}{dirm}/m2mqtt_srv_bticino.key'], check=False)

        # Copy jq to /usr/bin
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/jq-linux-armhf',
                        f'{self.mnt_loc}/usr/bin/jq'], check=False)
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mnt_loc}/usr/bin/jq'], check=False)

        # Copy evtest to /usr/bin
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/evtest',
                        f'{self.mnt_loc}/usr/bin/evtest'], check=False)
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mnt_loc}/usr/bin/evtest'], check=False)

        with open(f'{self.mnt_loc}/etc/init.d/flexisipsh', 'r', encoding='utf-8') as f:
            contents = f.readlines()

        contents.insert(24, '\t/bin/touch /tmp/flexisip_restarted\n')

        with open(f'{self.mnt_loc}/etc/init.d/flexisipsh', 'w', encoding='utf-8') as f:
            contents = ''.join(contents)
            f.write(contents)

        print('done ✅')
        result = True
        return result

    def enable_mqtt(self):
        """Enable MQTT."""
        print('Enabling MQTT... ', end='', flush=True)
        os.chdir(f'{self.mnt_loc}/etc/rc5.d')
        # create symbolic link
        subprocess.call(['sudo', 'ln', '-s', '../tcpdump2mqtt/TcpDump2Mqtt.sh',
                         'S99TcpDump2Mqtt'])
        # return to temporary folder
        os.chdir(self.workingdir)
        print('done ✅')

    def setup_ssh_key_rights(self):
        """Setup SSH key rights."""
        print('Setting up SSH key rights... ', end='', flush=True)
        subprocess.run(['sudo', 'chmod', '600',
                        f'{self.mnt_loc}/etc/dropbear/authorized_keys'], check=False)
        subprocess.run(['sudo', 'chmod', '600',
                        f'{self.mnt_loc}/home/root/.ssh/authorized_keys'], check=False)
        print('set to 600 ✅')

    def enable_dropbear(self):
        """Enable dropbear."""
        print('Enabling dropbear... ', end='', flush=True)
        # change to mounted folder
        os.chdir(f'{self.mnt_loc}/etc/rc5.d')
        # create symbolic link
        subprocess.call(['sudo', 'ln', '-s', '../init.d/dropbear',
                         'S98dropbear'])
        # return to temporary folder
        os.chdir(self.workingdir)
        print('enabled ✅')

    def save_version(self, cwd, version):
        """Save version."""
        print('Saving version... ', end='', flush=True)
        destination_path = '/home/bticino/sp/patch_github.xml'
        # Copy file patch_github.xml to mounted folder
        # in /home/bticino/sp/patch_github.xml
        from_file = f'{cwd}/patch_github.xml'
        to_file = f'{self.mnt_loc}{destination_path}'
        input_file = open(from_file, 'r', encoding='utf-8')
        lines = input_file.readlines()
        input_file.close()
        output_file = open(to_file, 'w', encoding='utf-8')
        for line in lines:
            if '<version>' in line:
                line = f'      <version>{version}</version>\n'
            output_file.write(line)
        output_file.close()
        print(f'saved in {destination_path} ✅')

    def add_host_and_ip(self, host, ip):
        """Add host and IP."""
        line1 = f'/bin/bt_hosts.sh add {host} {ip}'
        with open(f'{self.mnt_loc}/etc/init.d/bt_daemon-apps.sh', 'r', encoding='utf-8') as f:
            contents = f.readlines()
        for i, line in enumerate(contents):
            if 'openserver' in line:
                contents.insert(i + 1, f'\t{line1}\n')
                break
        with open(f'{self.mnt_loc}/etc/init.d/bt_daemon-apps.sh', 'w', encoding='utf-8') as f:
            contents = ''.join(contents)
            f.write(contents)
        print('Editing "/etc/init.d/bt_daemon-apps.sh" done '
              f'for host {host}:{ip} ✅')

    def disable_notify_new_firmware(self):
        """Disable notify new firmware."""
        print('Disabling notifications when new '
              'firmware... ', end='', flush=True)
        # Preparing lines
        host1 = 'prodlegrandressourcespkg.blob.core.windows.net'
        host2 = 'blob.ams25prdstr02a.store.core.windows.net'
        ip1 = ip2 = '127.0.0.1'
        self.add_host_and_ip(host1, ip1)
        self.add_host_and_ip(host2, ip2)

    def umount_firmware(self):
        """Unmount firmware."""
        print('Unmounting firmware... ', end='', flush=True)
        subprocess.call(['sudo', 'umount', self.mnt_loc])
        print('unmounted ✅')

    def gz_firmware(self):
        """GZ firmware."""
        print('GZ firmware... ', end='', flush=True)
        # From btweb_only.ext4 to btweb_only.ext4.gz
        with open(f'{self.workingdir}/{self.prt_frmw[:-3]}', 'rb') as f_in:
            with gzip.open(f'{self.workingdir}/{self.prt_frmw}', 'wb') as f_out:
                shutil.copyfileobj(f_in, f_out)
        print(f'new GZed {self.prt_frmw} ✅')

    def zip_file_firmware(self, filesinsidelist):
        """Adding files in the zip archive."""
        print('Adding files in the zip archive... ', end='', flush=True)
        password = None
        output = self.fileout
        zip_file = f'{self.workingdir}/{output}'
        if self.model == 'c300x':
            password = PrepareFirmware.password
        elif self.model == 'c100x':
            password = PrepareFirmware.password2
        elif PrepareFirmware.password3 in zip_file:
            password = PrepareFirmware.password3
        else:
            print('No password found ❌')
            return
        if password:
            pathlist = []
            for fil in filesinsidelist:
                pathlist.append(f'{self.workingdir}/{fil}')
            level_compression = 9
            pyminizip.compress_multiple(
                pathlist, [], zip_file, password, level_compression)
            print(f'firmware new ziped on {zip_file} ✅')

    def move_ssh_key_file_firmware(self, cwd):
        """Move SSH key file."""
        print('Moving SSH key file... ', end='', flush=True)
        output = self.fileout
        fles = ['bticinokey.pub', 'bticinokey', output]
        for f in fles:
            subprocess.run(['sudo', 'mv',
                            f'{self.workingdir}/{f}', f'{cwd}/{f}'], check=False)
        print('files moved ✅')

    def delete_temp_folder(self):
        """Delete temporary folder."""
        print('Deleting temporary folder... ', end='', flush=True)
        shutil.rmtree(self.workingdir)
        print(f'deleted {self.workingdir} ✅')

    def setup_firmware_rights(self, cwd):
        """Setup firmware rights."""
        print('Setting up firmware rights... ', end='', flush=True)
        output = self.fileout
        fles = ['bticinokey.pub', 'bticinokey', output]
        for f in fles:
            subprocess.run(['sudo', 'chown', '-R', '1000:1000', f'{cwd}/{f}'], check=False)
            subprocess.run(['sudo', 'chmod', '-R', '755', f'{cwd}/{f}'], check=False)
        print('rights set ✅')


if __name__ == '__main__':
    c = PrepareFirmware()
    c.main()
