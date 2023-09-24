#!/usr/bin/env python
# -*- coding: utf-8 -*-

"""Prepare firmware update."""

__version__ = "0.0.12"

import wget
import zipfile
import tempfile
import shutil
import os
import subprocess
import gzip
import pyminizip
import time
import ipaddress
import logging


class PrepareFirmware():
    """Firmware prepare class."""

    def __init__(self):
        """First init class."""
        self.loggingLEVEL = 'debug'
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
        formatter = logging.Formatter(f)
        self.logger = logging.getLogger('PrepareFirmware')
        self.logger.setLevel(LOGGER_LEVEL)

        # Create a console handler
        ch = logging.StreamHandler()
        ch.setLevel(LOGGER_LEVEL)
        ch.setFormatter(formatter)
        self.logger.addHandler(ch)

        # Create a file handler
        fh = logging.FileHandler('PrepareFirmware.log')
        fh.setLevel(LOGGER_LEVEL)
        fh.setFormatter(formatter)
        self.logger.addHandler(fh)

        # Last known firmware version for C300X and C100X models
        self.urlC300X = ('https://prodlegrandressourcespkg.blob.core.'
                         'windows.net/binarycontainer/bt_344642_3_0_0-'
                         'c300x_010719_1_7_19.bin')
        self.urlC100X = ('https://www.homesystems-legrandgroup.com/MatrixENG/'
                         'liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&'
                         'fileName=C100X_010501.fwz&fileId='
                         '58107.23188.46381.34528')
        # Variables
        self.filename = None
        # Contants
        self.password = 'C300X'
        self.password2 = 'C100X'
        self.password3 = 'SMARTDES'
        self.workingdir = None
        self.prtFrmw = None
        self.useWebFirmware = None
        self.rootPassword = None
        self.SSHcreation = None
        self.removeSig = None
        self.installMQTT = None
        self.notifyNewFirmware = None
        self.mntLoc = '/media/mounted'
        self.step = 0

    def main(self):
        """Main function."""
        self.logger.info('Starting PrepareFirmware')
        while self.step == 0:
            # Ask for model: C300X or C100X
            self.model = input('Insert model (C300X or C100X): ')
            if self.model == 'C300X':
                self.url = self.urlC300X
                self.step = 1
            elif self.model == 'C100X':
                self.url = self.urlC100X
                self.step = 1
            else:
                print('Wrong model ❌')
                time.sleep(1)

        while self.step == 1:
            # Ask for firmware file
            ask = input(
                'Do you want to download the firmware [y] or '
                'use an available firmware [n]? (y/n): ')
            if ask == 'y' or ask == 'Y':
                self.useWebFirmware = ask.lower()
                version = self.getVersionFromURL()
                self.filename = f'{self.model}_{version}.fwz'
                print('The program will download the firmware: '
                      f'{self.filename}', flush=True)
                self.step = 2
            elif ask == 'n' or ask == 'N':
                self.useWebFirmware = ask.lower()
                self.filename = input(
                    'Enter the filename in the root directory: ')
                print('We use the firmware on this folder called: '
                      f'{self.filename}', flush=True)
                self.step = 2
            else:
                print('Please use y or n', flush=True)
                time.sleep(1)

        while self.step == 2:
            # Ask for root password
            self.rootPassword = input(
                'Enter the BTICINO root password (pwned123): ')
            if not self.rootPassword:
                self.rootPassword = 'pwned123'
                print('The program will use this root password: '
                      f'{self.rootPassword}', flush=True)
            ask = input(
                'Do you want to create an SSH key [y] or '
                'use your SSH key [n]? (y/n): ')
            if ask == 'y' or ask == 'Y':
                self.SSHcreation = ask.lower()
                print('The program will create SSH key for you.', flush=True)
                self.step = 3
            elif ask == 'n' or ask == 'N':
                self.SSHcreation = ask.lower()
                print('We use SSH on this folder called: bticinokey and '
                      'bticinokey.pub', flush=True)
                self.step = 3
            else:
                print('Please use y or n', flush=True)
                time.sleep(1)

        while self.step == 3:
            # Ask for sig files removal
            ask = input(
                'Do you want to remove Sig files [y] or keep '
                'them [n]? (y/n): ')
            if ask == 'y' or ask == 'Y':
                self.removeSig = ask.lower()
                print('The program will remove Sig files.', flush=True)
                self.step = 4
            elif ask == 'n' or ask == 'N':
                self.removeSig = ask.lower()
                print('The program will keep Sig files.', flush=True)
                self.step = 4
            else:
                print('Please use y or n', flush=True)
                time.sleep(1)

        while self.step == 4:
            # Ask for MQTT installation
            ask = input(
                'Do you want to install MQTT [y] or no [n]? (y/n): ')
            if ask == 'y' or ask == 'Y':
                self.installMQTT = ask.lower()
                print('The program will install MQTT.', flush=True)
                self.step = 5
            elif ask == 'n' or ask == 'N':
                self.installMQTT = ask.lower()
                print('The program will NOT install MQTT.', flush=True)
                self.step = 5
            else:
                print('Please use y or n', flush=True)
                time.sleep(1)

        while self.step == 5:
            # Ask for notification when new firmware is available
            ask = input(
                'Do you want to be notified when a new firmware is available '
                '[y] or not [n]? (y/n): ')
            if ask == 'y' or ask == 'Y':
                self.notifyNewFirmware = ask.lower()
                print('App will notify you when a new firmware is '
                      'available.', flush=True)
                self.step = 6
            elif ask == 'n' or ask == 'N':
                self.notifyNewFirmware = ask.lower()
                print('App will not notify you when a new firmware is '
                      'available.', flush=True)
                self.step = 6
            else:
                print('Please use y or n', flush=True)
                time.sleep(1)

        if self.step == 6:
            self.processFirmware()

    def processFirmware(self):
        """Process firmware."""
        # Get the current working directory
        cwd = os.getcwd()

        self.createTempFolder()
        if self.useWebFirmware == 'y':
            self.downloadFirmware()
        else:
            orig = f'{cwd}/{self.filename}'
            dest = f'{self.workingdir}/{self.filename}'
            subprocess.run(['sudo', 'cp', orig, dest])
        filesinsidelist = self.listFilesZIP()
        self.selectFirmwareFile(filesinsidelist)
        self.unzipFile()
        self.unGZfirmware()
        if self.removeSig == 'y':
            self.removeSigFiles()
        self.umountFirmware()
        self.mountFirmware()

        rootSeed = self.createRootPassword()
        self.setShadowFile(rootSeed)
        self.setPasswdFile()
        if self.SSHcreation == 'y':
            self.createSSHkey()
        elif self.SSHcreation == 'n':
            self.getSSHkey(cwd)
        self.setSSHkey()
        self.setupSSHkeyRights()
        self.enableDropbear()
        self.saveVersion(cwd, __version__)
        if self.installMQTT == 'y':
            ok = self.prepareMQTT(cwd)
            if ok:
                self.enableMQTT()
            else:
                print('MQTT not installed ❌')
        if self.notifyNewFirmware == 'n':
            self.disableNotifyNewFirmware()
        self.umountFirmware()
        self.GZfirmware()
        self.zipFileFirmware(filesinsidelist)
        self.moveSSHkeyFileFirmware(cwd)
        self.deleteTempFolder()
        self.setupFirmwareRights(cwd)

        # return to init folder
        os.chdir(cwd)

    def getVersionFromURL(self, human=False):
        """Get version from URL."""
        # url in lowercase
        url = self.url.lower()
        # search model in url
        if self.model == 'C300X':
            model = 'c300x'
        elif self.model == 'C100X':
            model = 'c100x'
        # get version
        vtxt = url.split(model)[1].split('_')[1]
        if not human:
            return vtxt[0:6]
        else:
            # major version
            major = (vtxt[0:2]).lstrip('0')
            # minor version
            minor = (vtxt[2:4]).lstrip('0')
            # patch version
            patch = (vtxt[4:6]).lstrip('0')
            # version
            version = major + '.' + minor + '.' + patch
            return version

    def createTempFolder(self):
        """Create temporary folder."""
        print('Creating temporary folder... ', end='', flush=True)
        tempdir = tempfile.mkdtemp(prefix="bticino-")
        self.workingdir = tempdir
        # Change the current working directory
        os.chdir(self.workingdir)
        print(f'created {self.workingdir} ✅')
        return tempdir

    def downloadFirmware(self):
        """Main function."""
        print('Downloading firmware... ', flush=True)
        # Using wget to download the file
        wget.download(self.url, f'{self.workingdir}/{self.filename}')

        # Using httpx to download the file
        # with open(self.filename, 'wb') as f:
        #     with httpx.stream("GET", url) as r:
        #         for datachunk in r.iter_bytes():
        #             f.write(datachunk)
        print(f' downloaded {self.filename} ✅')

    def listFilesZIP(self):
        """List of files."""
        print('Reading files inside firmware... ', end='', flush=True)
        # zip file handler
        zip = zipfile.ZipFile(f'{self.workingdir}/{self.filename}')
        # list available files in the container
        filesinsidelist = []
        if self.removeSig == 'y' or self.removeSig == 'Y':
            for partFirm in zip.namelist():
                if 'sig' not in partFirm:
                    filesinsidelist.append(partFirm)
        elif self.removeSig == 'n' or self.removeSig == 'N':
            filesinsidelist = zip.namelist()
        print('done ✅')
        return filesinsidelist

    def selectFirmwareFile(self, filesinsidelist):
        """Select firmware file."""
        print('Selecting firmware file... ', end='', flush=True)
        # Select the firmware file
        for partFirm in filesinsidelist:
            if 'gz' in partFirm and 'recovery' not in partFirm:
                self.prtFrmw = partFirm
        print(f'important file is {self.prtFrmw} ✅')

    def unzipFile(self):
        """Un zip function."""
        print('Unzipping firmware... ', end='', flush=True)
        zip_file = f'{self.workingdir}/{self.filename}'
        if self.model == 'C300X':
            password = self.password
        elif self.model == 'C100X':
            password = self.password2
        elif self.password3 in zip_file:
            password = self.password3
        else:
            password = False
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
                exit(1)
        # 7z l -slt C300X_010717.fwz check is "Method = ZipCrypto Deflate"
        if self.removeSig == 'y' or self.removeSig == 'Y':
            subprocess.run(['rm', '-rf', f'{self.workingdir}/*.sig'])
        print(f'unzipped {self.filename} ✅')

    def removeSigFiles(self):
        """Will remove sig files."""
        print('Removing Sig files... ', end='', flush=True)
        subprocess.call(f'rm -rf {self.workingdir}/*.sig', shell=True)
        print('done ✅')

    def unGZfirmware(self):
        """UnGZ firmware."""
        print('UnGZ firmware... ', end='', flush=True)
        # From btweb_only.ext4.gz to btweb_only.ext4
        with gzip.open(f'{self.workingdir}/{self.prtFrmw}', 'rb') as f_in:
            with open(f'{self.workingdir}/{self.prtFrmw[:-3]}', 'wb') as f_out:
                shutil.copyfileobj(f_in, f_out)
        print(f'unGZed {self.prtFrmw} ✅')

    def mountFirmware(self):
        """Mount firmware."""
        print('Mounting firmware... ', end='', flush=True)
        # sudo mount -o loop btweb_only.ext4 /media/mounted/
        # Make directory mounted
        subprocess.run(['sudo', 'mkdir', '-p', self.mntLoc])
        subprocess.call(['sudo', 'mount', '-t', 'ext4', '-o', 'loop',
                         f'{self.workingdir}/{self.prtFrmw[:-3]}',
                         self.mntLoc])
        print(f'mounted on {self.mntLoc} ✅')

    def createRootPassword(self):
        """Create root password."""
        print('Creating root password... ', end='', flush=True)
        # openssl passwd -1 -salt root pwned123
        # r = $1$root$0i6hbFPn3JOGMeEF0LgEV1
        output = subprocess.run(['openssl', 'passwd', '-1', '-salt', 'root',
                                 self.rootPassword], capture_output=True)
        r = str((output.stdout).decode('utf-8'))
        # remove last character because it is a newline
        result = r[:-1]
        # result = r.rstrip()
        print(f'created {result} ✅')
        return result

    def setShadowFile(self, rootSeed):
        """Set shadow file."""
        print('Setting shadow file... ', end='', flush=True)
        if self.rootPassword:
            line1 = f'root2:{rootSeed}:18033:0:99999:7:::\n'
            line2 = f'bticino2:{rootSeed}:18033:0:99999:7:::\n'
            file_object = open(f'{self.mntLoc}/etc/shadow', 'a')
            file_object.write(line1)
            file_object.write(line2)
            file_object.close()
            print('modified ✅')
            return
        else:
            print('failed ❌')
            return

    def setPasswdFile(self):
        """Set passwd file."""
        print('Setting passwd file... ', end='', flush=True)
        line1 = 'root2:x:0:0:root:/home/root:/bin/sh\n'
        line2 = 'bticino2:x:1000:1000::/home/bticino:/bin/sh\n'
        file_object = open(f'{self.mntLoc}/etc/passwd', 'a')
        file_object.write(line1)
        file_object.write(line2)
        file_object.close()
        print('modified ✅')

    def createSSHkey(self):
        """Create SSH key."""
        print('Creating SSH key... ', end='', flush=True)
        # ssh-keygen -t rsa -b 4096 -f /tmp/bticinokey -N ""
        savedkeyfile = f'{self.workingdir}/bticinokey'
        subprocess.run(['ssh-keygen', '-t', 'rsa', '-b', '4096',
                        '-f', savedkeyfile, '-N', ''])
        print('created ✅')

    def getSSHkey(self, cwd):
        """Get SSH key."""
        print('Getting SSH key... ', end='', flush=True)
        fles = ['bticinokey.pub', 'bticinokey']
        for f in fles:
            subprocess.run(['sudo', 'cp', f'{cwd}/{f}',
                            f'{self.workingdir}/{f}'])
        print('files moved ✅')

    def setSSHkey(self):
        """Set SSH key."""
        print('Setting SSH key... ', end='', flush=True)
        # sudo cp /tmp/bticinokey.pub to
        # /media/mounted/etc/dropbear/authorized_keys
        subprocess.run(['sudo', 'cp', f'{self.workingdir}/bticinokey.pub',
                        f'{self.mntLoc}/etc/dropbear/authorized_keys'])
        # Add public file to .ssh/authorized_keys
        subprocess.run(['sudo', 'mkdir', '-p',
                        f'{self.mntLoc}/home/root/.ssh'])
        subprocess.run(['sudo', 'cp', f'{self.workingdir}/bticinokey.pub',
                        f'{self.mntLoc}/home/root/.ssh/authorized_keys'])
        print('set done ✅')

    def is_valid_ip(self, ip):
        """Check if IP is valid."""
        try:
            ipaddress.ip_address(ip)
            return True
        except ValueError:
            return False

    def prepareMQTT(self, cwd):
        """Prepare MQTT."""
        result = False
        value = None
        print('Preparing MQTT... ', end='', flush=True)
        # Check .conf file if MQTT_HOST is and IP address or a domain name
        isIP = False
        with open(f'{cwd}/mqtt_scripts/TcpDump2Mqtt.conf', 'r') as f:
            contents = f.readlines()
        for i, line in enumerate(contents):
            if 'MQTT_HOST' in line:
                value = line.split('=')[1]
                # check if value is empty or None
                if not value:
                    print('MQTT_HOST is empty ❌ check TcpDump2Mqtt.conf')
                    return result
                else:
                    r = self.is_valid_ip(value)
                    if r:
                        isIP = True
                        break
                    else:
                        break
        # If isIP is True then value is an IP address and we can continue
        # if not is IP then value is a domain name and we need to get the IP
        # address from the domain name and add it to the hosts file
        if not isIP:
            hstnme = value
            # Get ip from the hostname
            # ip = socket.gethostbyname(hstnme)
            ip = input(f'Enter IP address for {hstnme}: ')
            while not self.is_valid_ip(ip):
                ip = input(f'Not valid. Enter IP address for {hstnme}: ')
                time.sleep(1)
            # add hostname to the end of the hosts file
            self.addHostandIP(hstnme, ip)

        # Copy file to mounted folder
        dirm = '/etc/tcpdump2mqtt'
        # Create tcpdump2mqtt directory
        subprocess.run(['sudo', 'mkdir', '-p',
                        f'{self.mntLoc}{dirm}'])
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/TcpDump2Mqtt',
                        f'{self.mntLoc}{dirm}/TcpDump2Mqtt'])
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mntLoc}{dirm}/TcpDump2Mqtt'])
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/TcpDump2Mqtt.conf',
                        f'{self.mntLoc}{dirm}/TcpDump2Mqtt.conf'])
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/TcpDump2Mqtt.sh',
                        f'{self.mntLoc}{dirm}/TcpDump2Mqtt.sh'])
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mntLoc}{dirm}/TcpDump2Mqtt.sh'])
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/StartMqttSend',
                        f'{self.mntLoc}{dirm}/StartMqttSend'])
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mntLoc}{dirm}/StartMqttSend'])
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/StartMqttReceive',
                        f'{self.mntLoc}{dirm}/StartMqttReceive'])
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mntLoc}{dirm}/StartMqttReceive'])
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/filter.py',
                        f'{self.mntLoc}/home/root/filter.py'])
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mntLoc}/home/root/filter.py'])
        subprocess.run(['sudo', 'cp', f'{self.mntLoc}/etc/init.d/flexisipsh',
                        f'{self.mntLoc}/etc/init.d/flexisipsh_bak'])

        # If extis file m2mqtt_ca.crt copy it
        if os.path.isfile(f'{cwd}/certs/m2mqtt_ca.crt'):
            subprocess.run(['sudo', 'cp', f'{cwd}/certs/m2mqtt_ca.crt',
                            f'{self.mntLoc}/etc/ssl/certs/m2mqtt_ca.crt'])
        # If extis file m2mqtt_srv_bticino.crt copy it
        if os.path.isfile(f'{cwd}/certs/m2mqtt_srv_bticino.crt'):
            subprocess.run([
                'sudo', 'cp', f'{cwd}/certs/m2mqtt_srv_bticino.crt',
                f'{self.mntLoc}{dirm}/m2mqtt_srv_bticino.crt'])
        # If extis file m2mqtt_srv_bticino.key copy it
        if os.path.isfile(f'{cwd}/certs/m2mqtt_srv_bticino.key'):
            subprocess.run([
                'sudo', 'cp', f'{cwd}/certs/m2mqtt_srv_bticino.key',
                f'{self.mntLoc}{dirm}/m2mqtt_srv_bticino.key'])

        # Copy jq to /usr/bin
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/jq-linux-armhf',
                        f'{self.mntLoc}/usr/bin/jq'])
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mntLoc}/usr/bin/jq'])

        # Copy evtest to /usr/bin
        subprocess.run(['sudo', 'cp', f'{cwd}/mqtt_scripts/evtest',
                        f'{self.mntLoc}/usr/bin/evtest'])
        subprocess.run(['sudo', 'chmod', '775',
                        f'{self.mntLoc}/usr/bin/evtest'])

        with open(f'{self.mntLoc}/etc/init.d/flexisipsh', 'r') as f:
            contents = f.readlines()

        contents.insert(24, '\t/bin/touch /tmp/flexisip_restarted\n')

        with open(f'{self.mntLoc}/etc/init.d/flexisipsh', 'w') as f:
            contents = ''.join(contents)
            f.write(contents)

        print('done ✅')
        result = True
        return result

    def enableMQTT(self):
        """Enable MQTT."""
        print('Enabling MQTT... ', end='', flush=True)
        os.chdir(f'{self.mntLoc}/etc/rc5.d')
        # create symbolic link
        subprocess.call(['sudo', 'ln', '-s', '../tcpdump2mqtt/TcpDump2Mqtt.sh',
                         'S99TcpDump2Mqtt'])
        # return to temporary folder
        os.chdir(self.workingdir)
        print('done ✅')

    def setupSSHkeyRights(self):
        """Setup SSH key rights."""
        print('Setting up SSH key rights... ', end='', flush=True)
        subprocess.run(['sudo', 'chmod', '600',
                        f'{self.mntLoc}/etc/dropbear/authorized_keys'])
        subprocess.run(['sudo', 'chmod', '600',
                        f'{self.mntLoc}/home/root/.ssh/authorized_keys'])
        print('set to 600 ✅')

    def enableDropbear(self):
        """Enable dropbear."""
        print('Enabling dropbear... ', end='', flush=True)
        # change to mounted folder
        os.chdir(f'{self.mntLoc}/etc/rc5.d')
        # create symbolic link
        subprocess.call(['sudo', 'ln', '-s', '../init.d/dropbear',
                         'S98dropbear'])
        # return to temporary folder
        os.chdir(self.workingdir)
        print('enabled ✅')

    def saveVersion(self, cwd, version):
        """Save version."""
        print('Saving version... ', end='', flush=True)
        destinationPath = '/home/bticino/sp/patch_github.xml'
        # Copy file patch_github.xml to mounted folder
        # in /home/bticino/sp/patch_github.xml
        fromFile = f'{cwd}/patch_github.xml'
        toFile = f'{self.mntLoc}{destinationPath}'
        inputFile = open(fromFile, 'r')
        lines = inputFile.readlines()
        inputFile.close()
        outputFile = open(toFile, 'w')
        for line in lines:
            if '<version>' in line:
                line = f'      <version>{version}</version>\n'
            outputFile.write(line)
        outputFile.close()
        print(f'saved in {destinationPath} ✅')

    def addHostandIP(self, host, ip):
        """Add host and IP."""
        line1 = f'/bin/bt_hosts.sh add {host} {ip}'
        with open(f'{self.mntLoc}/etc/init.d/bt_daemon-apps.sh', 'r') as f:
            contents = f.readlines()
        for i, line in enumerate(contents):
            if 'openserver' in line:
                contents.insert(i + 1, f'\t{line1}\n')
                break
        with open(f'{self.mntLoc}/etc/init.d/bt_daemon-apps.sh', 'w') as f:
            contents = ''.join(contents)
            f.write(contents)
        print('Editing "/etc/init.d/bt_daemon-apps.sh" done '
              f'for host {host}:{ip} ✅')

    def disableNotifyNewFirmware(self):
        """Disable notify new firmware."""
        print('Disabling notifications when new '
              'firmware... ', end='', flush=True)
        # Preparing lines
        host1 = 'prodlegrandressourcespkg.blob.core.windows.net'
        host2 = 'blob.ams25prdstr02a.store.core.windows.net'
        ip1 = ip2 = '127.0.0.1'
        self.addHostandIP(host1, ip1)
        self.addHostandIP(host2, ip2)

    def umountFirmware(self):
        """Unmount firmware."""
        print('Unmounting firmware... ', end='', flush=True)
        subprocess.call(['sudo', 'umount', self.mntLoc])
        print('unmounted ✅')

    def GZfirmware(self):
        """GZ firmware."""
        print('GZ firmware... ', end='', flush=True)
        # From btweb_only.ext4 to btweb_only.ext4.gz
        with open(f'{self.workingdir}/{self.prtFrmw[:-3]}', 'rb') as f_in:
            with gzip.open(f'{self.workingdir}/{self.prtFrmw}', 'wb') as f_out:
                shutil.copyfileobj(f_in, f_out)
        print(f'new GZed {self.prtFrmw} ✅')

    def zipFileFirmware(self, filesinsidelist):
        """Adding files in the zip archive."""
        print('Adding files in the zip archive... ', end='', flush=True)
        a = self.filename
        output = a[:-4] + '_new' + a[-4:]
        zip_file = f'{self.workingdir}/{output}'
        if self.model == 'C300X':
            password = self.password
        elif self.model == 'C100X':
            password = self.password2
        elif self.password3 in zip_file:
            password = self.password3
        else:
            password = False
            print('No password found ❌')
            return
        if password:
            pathlist = []
            for fil in filesinsidelist:
                pathlist.append(f'{self.workingdir}/{fil}')
            pyminizip.compress_multiple(pathlist, [], zip_file, password, 5)
            print(f'firmware new ziped on {zip_file} ✅')

    def moveSSHkeyFileFirmware(self, cwd):
        """Move SSH key file."""
        print('Moving SSH key file... ', end='', flush=True)
        a = self.filename
        output = a[:-4] + '_new' + a[-4:]
        fles = ['bticinokey.pub', 'bticinokey', output]
        for f in fles:
            subprocess.run(['sudo', 'mv',
                            f'{self.workingdir}/{f}', f'{cwd}/{f}'])
        print('files moved ✅')

    def deleteTempFolder(self):
        """Delete temporary folder."""
        print('Deleting temporary folder... ', end='', flush=True)
        shutil.rmtree(self.workingdir)
        print(f'deleted {self.workingdir} ✅')

    def setupFirmwareRights(self, cwd):
        """Setup firmware rights."""
        print('Setting up firmware rights... ', end='', flush=True)
        a = self.filename
        output = a[:-4] + '_new' + a[-4:]
        fles = ['bticinokey.pub', 'bticinokey', output]
        for f in fles:
            subprocess.run(['sudo', 'chown', '-R', '1000:1000', f'{cwd}/{f}'])
            subprocess.run(['sudo', 'chmod', '-R', '755', f'{cwd}/{f}'])
        print('rights set ✅')


if __name__ == '__main__':
    c = PrepareFirmware()
    c.main()
