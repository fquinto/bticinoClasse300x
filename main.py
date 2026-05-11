#!/usr/bin/env python
# -*- coding: utf-8 -*-

"""Prepare firmware update."""

__version__ = "0.0.13"

from collections import namedtuple
from collections.abc import Iterable
from urllib.parse import urlparse, urlunparse, ParseResult
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

# Helper Functions

def ask(
    prompt: str,
    options: Iterable[str],
    extras: Iterable[str] = (),
    default: str = '',
    display_as_list: bool = False,
) -> str:
    """Prompt user for `input()` with specified options.

    ---
    Parameters
    ----------
    prompt : str
        The prompt message to display
    options : Iterable[str]
        Valid options that the user can choose from
    extras : Iterable[str] = ()
        Other considerable options that otherwise do not show up
    default : str = ''
        Default value that must be in options
    display_as_list : bool = False
        If False, display as [opt1/opt2/opt3]. If True, display as a list.

    Raises
    ------
    ValueError
        * If `options` are not provided;
        * If `default` provided cannot be found in `options`.

    Returns
    -------
    str : The selected option
    ---
    """

    # Arg checking
    if not options:
        raise ValueError("Options cannot be empty")
    elif default and default not in options:
        raise ValueError(f"Default value '{default}' not found in options")

    all_options = [*options, *extras]

    # Display options:
    # If specified, display as a list
    if display_as_list:
        # Append text '[default]' if default is defined
        print(*(f"- {x if not default or x.casefold() != default.casefold() else x+' [default]'}" for x in options), sep='\n')
    # Otherwise, display as [a/b/c/...]
    else:
        # Copy options, while applying uppercase to default value
        displayed_opts = [x if not default or x.casefold() != default.casefold() else x.upper() for x in options]

        # Prepend space if prompt is not empty
        prompt += (' ' if prompt else '') + f'[{'/'.join(displayed_opts)}]'

    # Preparing prompt tail
    prompt += ': ' if prompt else '> '

    answer = ''
    while not answer:
        # Grab contents, setting result to default if empty
        try:
            reply = input(prompt).strip()
        except KeyboardInterrupt:
            print('\nKeyboardInterrupt issued. Aborting', file=sys.stderr, flush=True)
            sys.exit(1)
        except EOFError:
            print('EOF found.', end=' ')
            if default:
                print(f'Assuming default "{default}"', flush=True)
                reply = ''
            else:
                print('No default defined. Repeating input prompt...', file=sys.stderr, flush=True)

        if not reply and default:
            answer = default
            break

        # Find reply in options, case-insensitive
        for opt in all_options:
            if reply.casefold() == opt.casefold():
                answer = opt
                break

        # If we reached here, reply was invalidated
        if not answer:
            print('Invalid answer ❌', flush=True)

    return answer

def ask_yn(prompt: str, default: str | bool):
    """Prompt user with a yes or no question.

    Wrapper for `ask(prompt, ['y', 'n'], ['ye', 'yes', 'no'], default)`.
    """
    # Arg checking
    if isinstance(default, bool):
        default = 'y' if default else 'n'

    # Preparing parameters
    options = ('y', 'n')
    extras_yes = ('ye', 'yes')
    extras_no = ('no')

    reply = ask(prompt, options, [*extras_yes, *extras_no], default).lower()[0]
    return reply in ('y', *extras_yes), reply

# Helper types

SSHKeyPair = namedtuple('SSHKeyPair', ['public', 'private'])
LegrandURLQuery = namedtuple('LegrandURLQuery', ['fileFormat', 'fileName', 'fileId'], defaults=['generic'])

# Base URL: https://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp
URL_LEGRAND = ParseResult('https', 'www.homesystems-legrandgroup.com', '/MatrixENG/liferay/bt_mxLiferayCheckout.jsp', '', '', '')

class PrepareFirmware():
    """Firmware prepare class."""

    # Last known firmware version for C300X and C100X models
    firmwares = {
        'c100x': {
            'versions':    ['1.5.1', '1.5.5', '1.5.7', '1.5.8'],
            'version_ids': ['010501', '010505', '010507', '010508'],
            'default': '1.5.8',
            '010501': '58107.23188.46381.34528',
            '010505': '58107.23188.62332.48840',
            '010507': '58107.23188.5954.54078' ,
            '010508': '58107.23188.17611.32784',
        },
        'c300x': {
            'versions':    ['1.7.17', '1.7.19'],
            'version_ids': ['010717', '010719'],
            'default': '1.7.19',
            '010717': '58107.23188.15908.12349',
            '010719': 'https://prodlegrandressourcespkg.blob.core.windows.net/binarycontainer/bt_344642_3_0_0-c300x_010719_1_7_19.bin',
        },
    }

    password_zip = 'SMARTDES'

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
        fh = logging.FileHandler('tmp/prepare_firmware.log')
        fh.setLevel(logger_level)
        fh.setFormatter(formatter)
        self.logger.addHandler(fh)

        # Variables
        self.fileout = None
        self.workingdir = None
        self.prt_frmw = None
        self.mnt_loc = '/media/mounted'
        self.ssh_keys = SSHKeyPair('bticinokey.pub', 'bticinokey')

        # Data from input, in order
        self.model = None
        self.version = None
        self.version_id = None
        self.url = None
        self.use_web_firmware = None
        self.filename = None
        self.root_password = None
        self.ssh_creation = None
        self.remove_sig = None
        self.install_mqtt = None
        self.notify_new_firmware = None

    def main(self):
        """Main function."""
        self.logger.info('Starting PrepareFirmware using version %s', __version__)

        # Ask for device model
        self.model = ask('Enter model', ['C100X', 'C300X'], default='C100X', display_as_list=True).lower()
        self.logger.info('State 0 done: using model %s', self.model)

        # Ask for firmware version
        self.version = ask('Enter version',
            PrepareFirmware.firmwares[self.model]['versions'], PrepareFirmware.firmwares[self.model]['version_ids'],
            PrepareFirmware.firmwares[self.model]['default'],
            display_as_list=True
        )
        self.version_id = self.format_version(self.version)
        self.filename = f'{self.model.upper()}_{self.version_id}.fwz'
        self.url = self.prepare_url(self.model, self.version_id)

        self.logger.info('State 1 done: using version %s', self.version)

        # Ask for firmware file
        result, self.use_web_firmware = ask_yn('Do you want to download the firmware?', 'y')
        print(f'The program will {'download' if result else 'use'} this firmware: {self.filename}', flush=True)
        self.logger.info('State 2 done: using firmware on %s', self.filename)

        # Ask for root password
        self.root_password = input('Enter the BTICINO root password [pwned123]: ').strip()
        if not self.root_password:
            self.root_password = 'pwned123'
            print(f'The program will use this root password: {self.root_password}', flush=True)

        # Ask for SSH key
        result, self.ssh_creation = ask_yn('Do you want to create an SSH key-pair?', 'n')
        if result:
            print('The program will create SSH key for you.', flush=True)
        else:
            print(f'Make sure to name your SSH keys accordingly: {self.ssh_keys.private} and {self.ssh_keys.public}', flush=True)
        self.logger.info('State 3 done: using SSH creation: %s', self.ssh_creation)

        # Ask for sig files removal
        result, self.remove_sig = ask_yn('Do you want to remove Sig files?', 'y')
        print(f'The program will {'remove' if result else 'keep'} Sig files.', flush=True)
        self.logger.info('State 4 done: using remove sig: %s', self.remove_sig)

        # Ask for MQTT installation
        result, self.install_mqtt = ask_yn('Do you want to install MQTT?', 'n')
        print(f'The program will{'' if result else ' NOT'} install MQTT.', flush=True)
        self.logger.info('State 5 done: using install MQTT: %s', self.install_mqtt)

        # Ask for notification when new firmware is available
        result, self.notify_new_firmware = ask_yn('Do you want to be notified when a new firmware is available?', 'y')
        print(f'App will{'' if result else ' not'} notify you when a new firmware is available.', flush=True)
        self.logger.info('State 6 done: notify new firmware: %s', self.notify_new_firmware)

        # Process firmware
        dt = time.strftime('%Y%m%d_%H%M%S')
        self.fileout = f'NEW_{self.model}_{self.version_id}{'_MQTT' if result else ''}_{dt}.fwz'
        cwd = self.process_firmware()

        # Move it inside folder fw/custom
        src = f'{cwd}/{self.fileout}'
        dst = f'fw/custom/{self.fileout}'
        subprocess.run(['mv', src, dst], check=False)

        self.logger.info('End PrepareFirmware using version %s', __version__)

    def process_firmware(self):
        """Process firmware."""
        # Get the current working directory
        cwd = os.getcwd()

        tempdir = self.create_temp_folder()
        # Change the current working directory to tempdir
        os.chdir(tempdir)
        self.workingdir = os.getcwd()

        if self.use_web_firmware == 'y':
            src = self.download_firmware(cwd)
            self.logger.info('Downloaded firmware: %s', src)
        else:
            src = f'{cwd}/fw/original/{self.filename}'
        dst = f'{self.workingdir}/{self.filename}'
        subprocess.run(['cp', src, dst], check=False)
        self.logger.info('Copied firmware from %s to %s', src, dst)

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
            if self.prepare_mqtt(cwd):
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

    def format_version(self, version: str) -> str:
        parts = version.split('.')
        # Pad with zeros to ensure we have 3 parts
        while len(parts) < 3:
            parts.append('0')
        return ''.join(f'{int(part):02d}' for part in parts)

    def unformat_version(self, version: str) -> str:
        major = (version[0:2]).lstrip('0')
        minor = (version[2:4]).lstrip('0')
        patch = (version[4:6]).lstrip('0')
        retval = major + '.' + minor + '.' + patch
        return retval

    def prepare_url(self, model: str, version_id: str) -> str:
        result = self.firmwares[model][version_id]
        if not result:
            raise ValueError(f"Version {version_id} for model {model} not found")
        # if the value found is a hardcoded URL
        elif result.startswith('http'):
            url_firmware = result
        # otherwise, it's the fileId for a query
        else:
            query_tuple = LegrandURLQuery('generic', self.filename, result)
            query = "&".join([f"{k}={v}" for k, v in query_tuple._asdict().items()])
            url_with_query = URL_LEGRAND._replace(query=query)
            url_firmware = urlunparse(url_with_query)
        return url_firmware

    def create_temp_folder(self):
        """Create temporary folder."""
        print('Creating temporary folder... ', end='', flush=True)
        tempdir = tempfile.mkdtemp(prefix="bticino-", dir="tmp")
        print(f'Created {tempdir} ✅')
        return tempdir

    def download_firmware(self, cwd):
        """Main function."""
        print('Downloading firmware... ', flush=True)
        # save to cwd/fw/original/
        output = f'{cwd}/fw/original/{self.filename}'
        # Using wget to download the file
        wget.download(self.url, output)

        # Using httpx to download the file
        # with open(self.filename, 'wb') as f:
        #     with httpx.stream("GET", url) as r:
        #         for datachunk in r.iter_bytes():
        #             f.write(datachunk)
        print(f' downloaded {self.filename} inside fw/original ✅')
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
        """Unzip function."""
        print('Unzipping firmware... ', end='', flush=True)
        password = None
        zip_file = f'{self.workingdir}/{self.filename}'
        if self.model in PrepareFirmware.firmwares:
            password = self.model.upper() # password is model name, uppercase
        elif password_zip in zip_file:
            password = password_zip
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
        """Mount firmware with robust error handling."""
        print('Mounting firmware... ', end='', flush=True)
        
        # Create mount point directory
        result = subprocess.run(['sudo', 'mkdir', '-p', self.mnt_loc], 
                               capture_output=True, text=True)
        if result.returncode != 0:
            print(f'❌')
            print(f'ERROR: Failed to create mount directory {self.mnt_loc}')
            print(f'Error: {result.stderr.strip()}')
            sys.exit(1)
        
        # Verify filesystem image exists
        firmware_image = f'{self.workingdir}/{self.prt_frmw[:-3]}'
        if not os.path.exists(firmware_image):
            print(f'❌')
            print(f'ERROR: Firmware image not found: {firmware_image}')
            sys.exit(1)
        
        # Mount filesystem
        result = subprocess.run(['sudo', 'mount', '-t', 'ext4', '-o', 'loop',
                                firmware_image, self.mnt_loc], 
                               capture_output=True, text=True)
        if result.returncode != 0:
            print(f'❌')
            print(f'ERROR: Failed to mount {firmware_image} to {self.mnt_loc}')
            print(f'Error: {result.stderr.strip()}')
            sys.exit(1)
        
        # Verify mount is accessible and writable
        try:
            # Test if mount point is accessible
            if not os.path.isdir(self.mnt_loc):
                raise Exception(f"Mount point {self.mnt_loc} is not accessible")
            
            # Test writability
            test_file = f'{self.mnt_loc}/.write_test_{os.getpid()}'
            with open(test_file, 'w') as f:
                f.write('mount_test')
            os.remove(test_file)
            
        except Exception as e:
            print(f'❌')
            print(f'ERROR: Mount point not writable: {e}')
            print('Attempting to unmount...')
            subprocess.run(['sudo', 'umount', self.mnt_loc], capture_output=True)
            sys.exit(1)
        
        print(f'mounted on {self.mnt_loc} ✅')
        self.logger.info(f'Firmware mounted successfully at {self.mnt_loc}')

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
        # ssh-keygen -t rsa -b 4096 -f keyfile -N ""
        savedkeyfile = f'{self.workingdir}/{self.ssh_keys.private}'
        subprocess.run(['ssh-keygen', '-t', 'rsa', '-b', '4096',
                        '-f', savedkeyfile, '-N', ''], check=False)
        print('created ✅')

    def get_ssh_key(self, cwd):
        """Get SSH key."""
        print('Getting SSH key... ', end='', flush=True)
        for f in self.ssh_keys:
            subprocess.run(['cp', f'{cwd}/{f}',
                            f'{self.workingdir}/{f}'], check=False)
        print('files moved ✅')

    def set_ssh_key(self):
        """Set SSH key."""
        print('Setting SSH key... ', end='', flush=True)
        # sudo cp keyfile /media/mounted/etc/dropbear/authorized_keys
        subprocess.run(['sudo', 'cp', f'{self.workingdir}/{self.ssh_keys.public}',
                        f'{self.mnt_loc}/etc/dropbear/authorized_keys'], check=False)
        # Add public file to .ssh/authorized_keys
        subprocess.run(['sudo', 'mkdir', '-p',
                        f'{self.mnt_loc}/home/root/.ssh'], check=False)
        subprocess.run(['sudo', 'cp', f'{self.workingdir}/{self.ssh_keys.public}',
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
        with open(f'{cwd}/scripts/mqtt/TcpDump2Mqtt.conf', 'r', encoding='utf-8') as f:
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

        # Copy file to mounted folder with robust error handling
        dirm = '/etc/tcpdump2mqtt'
        
        def run_command_with_check(cmd, description):
            """Run command with error checking and detailed logging."""
            self.logger.info(f'Executing: {" ".join(cmd)}')
            result = subprocess.run(cmd, capture_output=True, text=True)
            if result.returncode != 0:
                print(f'❌')
                print(f'ERROR: {description} failed')
                print(f'Command: {" ".join(cmd)}')
                print(f'Error: {result.stderr.strip()}')
                return False
            return True
        
        def verify_file_exists(file_path, description):
            """Verify source file exists before copying."""
            if not os.path.exists(file_path):
                print(f'❌')
                print(f'ERROR: {description} source file not found: {file_path}')
                return False
            return True
        
        # Create tcpdump2mqtt directory
        if not run_command_with_check(['sudo', 'mkdir', '-p', f'{self.mnt_loc}{dirm}'], 
                                     'Creating /etc/tcpdump2mqtt directory'):
            return False
        
        # Verify directory was created
        if not os.path.exists(f'{self.mnt_loc}{dirm}'):
            print(f'❌')
            print(f'ERROR: Directory {self.mnt_loc}{dirm} was not created successfully')
            return False
        
        self.logger.info(f'Successfully created directory: {self.mnt_loc}{dirm}')
        
        # Define files to copy with their destinations and permissions
        mqtt_files = [
            # (source, destination, permissions, required)
            (f'{cwd}/mqtt_scripts/TcpDump2Mqtt', f'{self.mnt_loc}{dirm}/TcpDump2Mqtt', '775', True),
            (f'{cwd}/mqtt_scripts/TcpDump2Mqtt.conf', f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.conf', None, True),
            (f'{cwd}/mqtt_scripts/TcpDump2Mqtt.sh', f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.sh', '775', True),
            (f'{cwd}/mqtt_scripts/StartMqttSend', f'{self.mnt_loc}{dirm}/StartMqttSend', '775', True),
            (f'{cwd}/mqtt_scripts/StartMqttReceive', f'{self.mnt_loc}{dirm}/StartMqttReceive', '775', True),
            (f'{cwd}/mqtt_scripts/filter.py', f'{self.mnt_loc}/home/root/filter.py', '775', True),
            (f'{cwd}/mqtt_scripts/jq-linux-armhf', f'{self.mnt_loc}/usr/bin/jq', '775', True),
            (f'{cwd}/mqtt_scripts/evtest', f'{self.mnt_loc}/usr/bin/evtest', '775', True),
        ]
        
        # Copy required MQTT files
        for source, dest, perm, required in mqtt_files:
            if required and not verify_file_exists(source, f'MQTT file {os.path.basename(source)}'):
                return False
                
            if not run_command_with_check(['sudo', 'cp', source, dest], 
                                         f'Copying {os.path.basename(source)}'):
                return False
            
            if perm and not run_command_with_check(['sudo', 'chmod', perm, dest], 
                                                  f'Setting permissions for {os.path.basename(dest)}'):
                return False
        
        # Backup flexisipsh before modification
        if not run_command_with_check(['sudo', 'cp', f'{self.mnt_loc}/etc/init.d/flexisipsh',
                                      f'{self.mnt_loc}/etc/init.d/flexisipsh_bak'], 
                                     'Backing up flexisipsh'):
            return False

        # Copy optional certificate files
        cert_files = [
            (f'{cwd}/certs/m2mqtt_ca.crt', f'{self.mnt_loc}/etc/ssl/certs/m2mqtt_ca.crt', 'CA certificate'),
            (f'{cwd}/certs/m2mqtt_srv_bticino.crt', f'{self.mnt_loc}{dirm}/m2mqtt_srv_bticino.crt', 'Client certificate'),
            (f'{cwd}/certs/m2mqtt_srv_bticino.key', f'{self.mnt_loc}{dirm}/m2mqtt_srv_bticino.key', 'Private key'),
        ]
        
        for cert_source, cert_dest, cert_desc in cert_files:
            if os.path.isfile(cert_source):
                if not run_command_with_check(['sudo', 'cp', cert_source, cert_dest], 
                                             f'Copying {cert_desc}'):
                    print(f'WARNING: Failed to copy {cert_desc}, continuing...')
                else:
                    self.logger.info(f'Successfully copied {cert_desc}')
        
        # Verify critical files were copied successfully
        critical_files = [
            f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.conf',
            f'{self.mnt_loc}{dirm}/TcpDump2Mqtt',
            f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.sh',
        ]
        
        for critical_file in critical_files:
            if not os.path.exists(critical_file):
                print(f'❌')
                print(f'ERROR: Critical MQTT file not found after copy: {critical_file}')
                return False
        
        self.logger.info('All critical MQTT files copied successfully')

        with open(f'{self.mnt_loc}/etc/init.d/flexisipsh', 'r', encoding='utf-8') as f:
            contents = f.readlines()

        contents.insert(24, '\t/bin/touch /tmp/flexisip_restarted\n')

        with open(f'{self.mnt_loc}/etc/init.d/flexisipsh', 'w', encoding='utf-8') as f:
            contents = ''.join(contents)
            f.write(contents)

        # Final verification of MQTT installation
        if not self.verify_mqtt_installation():
            return False
        
        print('done ✅')
        result = True
        return result

    def verify_mqtt_installation(self):
        """Verify that MQTT installation was successful."""
        print('Verifying MQTT installation... ', end='', flush=True)
        
        # Directory and files to verify
        dirm = '/etc/tcpdump2mqtt'
        verification_items = [
            # (path, type, description)
            (f'{self.mnt_loc}{dirm}', 'directory', '/etc/tcpdump2mqtt directory'),
            (f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.conf', 'file', 'MQTT configuration file'),
            (f'{self.mnt_loc}{dirm}/TcpDump2Mqtt', 'file', 'MQTT main executable'),
            (f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.sh', 'file', 'MQTT startup script'),
            (f'{self.mnt_loc}{dirm}/StartMqttSend', 'file', 'MQTT send script'),
            (f'{self.mnt_loc}{dirm}/StartMqttReceive', 'file', 'MQTT receive script'),
            (f'{self.mnt_loc}/home/root/filter.py', 'file', 'MQTT filter script'),
            (f'{self.mnt_loc}/usr/bin/jq', 'file', 'jq utility'),
            (f'{self.mnt_loc}/usr/bin/evtest', 'file', 'evtest utility'),
        ]
        
        missing_items = []
        for item_path, item_type, description in verification_items:
            if item_type == 'directory':
                if not os.path.isdir(item_path):
                    missing_items.append((description, item_path))
            elif item_type == 'file':
                if not os.path.isfile(item_path):
                    missing_items.append((description, item_path))
        
        if missing_items:
            print('❌')
            print('ERROR: MQTT installation verification failed!')
            print('Missing items:')
            for desc, path in missing_items:
                print(f'  - {desc}: {path}')
            self.logger.error(f'MQTT verification failed - missing {len(missing_items)} items')
            return False
        
        # Verify MQTT configuration file contains required settings
        try:
            mqtt_conf_path = f'{self.mnt_loc}{dirm}/TcpDump2Mqtt.conf'
            with open(mqtt_conf_path, 'r', encoding='utf-8') as f:
                conf_content = f.read()
                
            if 'MQTT_HOST=' not in conf_content:
                print('❌')
                print('ERROR: MQTT_HOST not found in configuration file')
                return False
                
            # Check if MQTT_HOST has a value
            for line in conf_content.splitlines():
                if line.startswith('MQTT_HOST='):
                    mqtt_host = line.split('=')[1].strip()
                    if not mqtt_host:
                        print('❌')
                        print('ERROR: MQTT_HOST is empty in configuration file')
                        return False
                    break
                    
        except Exception as e:
            print('❌')
            print(f'ERROR: Could not verify MQTT configuration file: {e}')
            return False
        
        print('verified ✅')
        self.logger.info('MQTT installation verification completed successfully')
        return True

    def enable_mqtt(self):
        """Enable MQTT with robust error handling."""
        print('Enabling MQTT... ', end='', flush=True)
        
        # Store current working directory
        original_dir = os.getcwd()
        
        try:
            # Verify rc5.d directory exists
            rc5_dir = f'{self.mnt_loc}/etc/rc5.d'
            if not os.path.exists(rc5_dir):
                print('❌')
                print(f'ERROR: rc5.d directory not found: {rc5_dir}')
                return False
            
            # Change to rc5.d directory
            os.chdir(rc5_dir)
            
            # Verify target script exists
            target_script = '../tcpdump2mqtt/TcpDump2Mqtt.sh'
            target_full_path = f'{self.mnt_loc}/etc/tcpdump2mqtt/TcpDump2Mqtt.sh'
            if not os.path.exists(target_full_path):
                print('❌')
                print(f'ERROR: Target script not found: {target_full_path}')
                return False
            
            # Remove existing symlink if it exists
            symlink_name = 'S99TcpDump2Mqtt'
            if os.path.exists(symlink_name) or os.path.islink(symlink_name):
                result = subprocess.run(['sudo', 'rm', symlink_name], 
                                       capture_output=True, text=True)
                if result.returncode != 0:
                    print('❌')
                    print(f'ERROR: Failed to remove existing symlink: {result.stderr.strip()}')
                    return False
            
            # Create symbolic link
            result = subprocess.run(['sudo', 'ln', '-s', target_script, symlink_name], 
                                   capture_output=True, text=True)
            if result.returncode != 0:
                print('❌')
                print(f'ERROR: Failed to create symbolic link: {result.stderr.strip()}')
                return False
            
            # Verify symlink was created successfully
            if not os.path.islink(symlink_name):
                print('❌')
                print(f'ERROR: Symbolic link was not created: {symlink_name}')
                return False
            
            print('done ✅')
            self.logger.info(f'MQTT service enabled successfully with symlink: {rc5_dir}/{symlink_name}')
            return True
            
        except Exception as e:
            print('❌')
            print(f'ERROR: Failed to enable MQTT service: {e}')
            self.logger.error(f'Failed to enable MQTT service: {e}')
            return False
            
        finally:
            # Always return to original working directory
            os.chdir(original_dir)

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
        from_file = f'{cwd}/rsrc/patch_github.xml'
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
        """Unmount firmware with robust error handling."""
        print('Unmounting firmware... ', end='', flush=True)
        
        # First attempt normal unmount
        result = subprocess.run(['sudo', 'umount', self.mnt_loc], 
                               capture_output=True, text=True)
        
        if result.returncode == 0:
            print('unmounted ✅')
            self.logger.info(f'Firmware successfully unmounted from {self.mnt_loc}')
            return True
        
        # If normal unmount failed, try lazy unmount
        print('retrying with lazy unmount... ', end='', flush=True)
        result = subprocess.run(['sudo', 'umount', '-l', self.mnt_loc], 
                               capture_output=True, text=True)
        
        if result.returncode == 0:
            print('unmounted (lazy) ✅')
            self.logger.info(f'Firmware lazily unmounted from {self.mnt_loc}')
            return True
        
        # If lazy unmount failed, try force unmount
        print('retrying with force unmount... ', end='', flush=True)
        result = subprocess.run(['sudo', 'umount', '-f', self.mnt_loc], 
                               capture_output=True, text=True)
        
        if result.returncode == 0:
            print('unmounted (forced) ✅')
            self.logger.info(f'Firmware forcibly unmounted from {self.mnt_loc}')
            return True
        
        # All unmount attempts failed
        print('❌')
        print(f'WARNING: Failed to unmount {self.mnt_loc}')
        print(f'Error: {result.stderr.strip()}')
        print(f'You may need to manually unmount: sudo umount {self.mnt_loc}')
        self.logger.error(f'Failed to unmount {self.mnt_loc}: {result.stderr.strip()}')
        return False

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
        fles = self.ssh_keys + [output]
        for f in fles:
            subprocess.run(['mv',
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
        fles = self.ssh_keys + [output]
        for f in fles:
            subprocess.run(['chown', '-R', '1000:1000', f'{cwd}/{f}'], check=False)
            subprocess.run(['chmod', '-R', '755', f'{cwd}/{f}'], check=False)
        print('rights set ✅')


if __name__ == '__main__':
    c = PrepareFirmware()
    c.main()
