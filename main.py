#!/usr/bin/env python
# -*- coding: utf-8 -*-

"""Prepare firmware update."""

__version__ = "0.0.3"

import wget
import zipfile
import tempfile
import shutil
import os
import subprocess
import gzip
import pyminizip


class PrepareFirmware():
    """Firmware prepare class."""

    def __init__(self):
        """First init class."""
        # Variables to adjust
        self.filename = 'C300X_010717.fwz'
        self.url = f'https://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName={self.filename}&fileId=58107.23188.15908.12349'

        # Contants
        self.password = 'C300X'
        self.password2 = 'C100X'
        self.password3 = 'SMARTDES'
        self.workingdir = None
        self.partFirmware = None
        self.rootPassword = None
        self.mountLocation = '/media/mounted'

    def main(self):
        """Main function."""
        # Get the current working directory
        cwd = os.getcwd()

        # Ask for root password
        self.rootPassword = input('Enter the BTICINO root '
                                  'password (pwned123): ')
        self.SSHcreation = input('Do you want to create an SSH key [y] or use your SSH key [n]? (y/n): ')
        if self.SSHcreation == 'y' or self.SSHcreation == 'Y':
            print('The program will create SSH key for you.', flush=True)
        elif self.SSHcreation == 'n' or self.SSHcreation == 'N':
            print('We use SSH on this folder called: bticinokey and bticinokey.pub', flush=True)
        else:
            print('Please use y or n', flush=True)
            exit(1)

        self.createTempFolder()
        self.downloadFirmware()
        filesinsidelist = self.listFilesZIP()
        self.selectFirmwareFile(filesinsidelist)
        self.unzipFile()
        self.unGZfirmware()
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

        self.umountFirmware()
        self.GZfirmware()
        self.zipFileFirmware(filesinsidelist)
        self.moveSSHkeyFileFirmware(cwd)
        self.deleteTempFolder()
        self.setupFirmwareRights(cwd)

        # return to init folder
        os.chdir(cwd)

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
        filesinsidelist = zip.namelist()
        print('done ✅')
        return filesinsidelist

    def selectFirmwareFile(self, filesinsidelist):
        """Select firmware file."""
        print('Selecting firmware file... ', end='', flush=True)
        # Select the firmware file
        for partFirm in filesinsidelist:
            if 'gz' in partFirm and 'recovery' not in partFirm:
                self.partFirmware = partFirm
        print(f'important file is {self.partFirmware} ✅')

    def unzipFile(self):
        """Un zip function."""
        print('Unzipping firmware... ', end='', flush=True)
        zip_file = f'{self.workingdir}/{self.filename}'
        if self.password in zip_file:
            password = self.password
        elif self.password2 in zip_file:
            password = self.password2
        elif self.password3 in zip_file:
            password = self.password3
        else:
            password = False
            print('No password found ❌')
            return
        if password:
            with zipfile.ZipFile(zip_file) as zf:
                zf.extractall(pwd=bytes(password, 'utf-8'))
        # 7z l -slt C300X_010717.fwz check is "Method = ZipCrypto Deflate"
        print(f'unzipped {self.filename} ✅')

    def unGZfirmware(self):
        """UnGZ firmware."""
        print('UnGZ firmware... ', end='', flush=True)
        # From btweb_only.ext4.gz to btweb_only.ext4
        with gzip.open(f'{self.workingdir}/{self.partFirmware}', 'rb') as f_in:
            with open(f'{self.workingdir}/{self.partFirmware[:-3]}', 'wb') as f_out:
                shutil.copyfileobj(f_in, f_out)
        print(f'unGZed {self.partFirmware} ✅')

    def mountFirmware(self):
        """Mount firmware."""
        print('Mounting firmware... ', end='', flush=True)
        # sudo mount -o loop btweb_only.ext4 /media/mounted/
        # Make directory mounted
        subprocess.run(['sudo', 'mkdir', '-p', self.mountLocation])
        subprocess.call(['sudo', 'mount', '-t', 'ext4', '-o', 'loop', f'{self.workingdir}/{self.partFirmware[:-3]}', self.mountLocation])
        print(f'mounted on {self.mountLocation} ✅')

    def createRootPassword(self):
        """Create root password."""
        print('Creating root password... ', end='', flush=True)
        # openssl passwd -1 -salt root pwned123
        # r = $1$root$0i6hbFPn3JOGMeEF0LgEV1
        output = subprocess.run(['openssl', 'passwd', '-1', '-salt', 'root', self.rootPassword], capture_output=True)
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
            file_object = open(f'{self.mountLocation}/etc/shadow', 'a')
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
        file_object = open(f'{self.mountLocation}/etc/passwd', 'a')
        file_object.write(line1)
        file_object.write(line2)
        file_object.close()
        print('modified ✅')

    def createSSHkey(self):
        """Create SSH key."""
        print('Creating SSH key... ', end='', flush=True)
        # ssh-keygen -t rsa -b 4096 -f /tmp/bticinokey -N ""
        savedkeyfile = f'{self.workingdir}/bticinokey'
        subprocess.run(['ssh-keygen', '-t', 'rsa', '-b', '4096', '-f', savedkeyfile, '-N', ''])
        print('created ✅')

    def getSSHkey(self, cwd):
        """Get SSH key."""
        print('Getting SSH key... ', end='', flush=True)
        fles = ['bticinokey.pub', 'bticinokey']
        for f in fles:
            subprocess.run(['sudo', 'cp', f'{cwd}/{f}', f'{self.workingdir}/{f}'])
        print('files moved ✅')

    def setSSHkey(self):
        """Set SSH key."""
        print('Setting SSH key... ', end='', flush=True)
        # sudo cp /tmp/bticinokey.pub /media/mounted/etc/dropbear/authorized_keys
        subprocess.run(['sudo', 'cp', f'{self.workingdir}/bticinokey.pub', f'{self.mountLocation}/etc/dropbear/authorized_keys'])
        # Add public file to .ssh/authorized_keys
        subprocess.run(['sudo', 'mkdir', '-p', f'{self.mountLocation}/home/root/.ssh'])
        subprocess.run(['sudo', 'cp', f'{self.workingdir}/bticinokey.pub', f'{self.mountLocation}/home/root/.ssh/authorized_keys'])
        print('set done ✅')

    def setupSSHkeyRights(self):
        """Setup SSH key rights."""
        print('Setting up SSH key rights... ', end='', flush=True)
        subprocess.run(['sudo', 'chmod', '600', f'{self.mountLocation}/etc/dropbear/authorized_keys'])
        subprocess.run(['sudo', 'chmod', '600', f'{self.mountLocation}/home/root/.ssh/authorized_keys'])
        print('set to 600 ✅')

    def enableDropbear(self):
        """Enable dropbear."""
        print('Enabling dropbear... ', end='', flush=True)
        # change to mounted folder
        os.chdir(f'{self.mountLocation}/etc/rc5.d')
        # create symbolic link
        subprocess.call(['sudo', 'ln', '-s', '../init.d/dropbear', 'S98dropbear'])
        # return to temporary folder
        os.chdir(self.workingdir)
        print('enabled ✅')

    def umountFirmware(self):
        """Unmount firmware."""
        print('Unmounting firmware... ', end='', flush=True)
        subprocess.call(['sudo', 'umount', self.mountLocation])
        print('unmounted ✅')

    def GZfirmware(self):
        """GZ firmware."""
        print('GZ firmware... ', end='', flush=True)
        # From btweb_only.ext4 to btweb_only.ext4.gz
        with open(f'{self.workingdir}/{self.partFirmware[:-3]}', 'rb') as f_in:
            with gzip.open(f'{self.workingdir}/{self.partFirmware}', 'wb') as f_out:
                shutil.copyfileobj(f_in, f_out)
        print(f'new GZed {self.partFirmware} ✅')

    def zipFileFirmware(self, filesinsidelist):
        """Adding files in the zip archive."""
        print('Adding files in the zip archive... ', end='', flush=True)
        a = self.filename
        output = a[:-4] + '_new' + a[-4:]
        zip_file = f'{self.workingdir}/{output}'
        if self.password in zip_file:
            password = self.password
        elif self.password2 in zip_file:
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
            subprocess.run(['sudo', 'mv', f'{self.workingdir}/{f}', f'{cwd}/{f}'])
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
