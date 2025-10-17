### Steps for modify firmware before flash

- Open firmware ZIP using password: `C300X`
- Note for C100X use password: `C100X`

- unGZ file: btweb_only.ext4.gz to btweb_only.ext4

- Mount root filesystem:
  `sudo mount -o loop btweb_only.ext4 /media/mounted/`

- Select your password, example: `pwned123`
- See the salt of your selected password:

```sh
openssl passwd -1 -salt root pwned123
$1$root$0i6hbFPn3JOGMeEF0LgEV1
```

```sh
cd /media/mounted/etc/
sudo vim shadow
```

- Set to:

```sh
root2:$1$root$0i6hbFPn3JOGMeEF0LgEV1:18033:0:99999:7:::
bticino2:$1$root$0i6hbFPn3JOGMeEF0LgEV1:18033:0:99999:7:::
```

```sh
sudo vim passwd
```

- Set to:

```sh
 root2:x:0:0:root:/home/root:/bin/sh
 bticino2:x:1000:1000::/home/bticino:/bin/sh
 ```

- Setup dropbear (is a SSH server)

```sh
cd /media/mounted/etc/rc5.d/
sudo ln -s ../init.d/dropbear S98dropbear
```

### Access SSH and scripts for open door

In this examples, we are using next:
<p>
UNIT = yyyyyyy<br>
MAC_ADDRESS = 00-03-50-xx-xx-xx<br>
IP = 192.168.1.97
PASSWORD = pwned123
</p>

**Replace with your needs.**

#### Change password for user root2

  ```
  mount -oremount,rw /
  passwd root2
  mount -oremount,ro /
  ```

#### Adapt your SSH access

- In terminal 1: access inside bticino and setup access for RW:
  - `mount -oremount,rw /`
  - **Not close this terminal**

- In terminal 2: create SSH key:

  `ssh-keygen -o -b 4096 -t rsa -f ./keys/bticinokey`

  Touch: [INTRO]+[INTRO]

- In terminal 2: copy SSH key inside the device:

  `ssh-copy-id -i ./keys/bticinokey.pub root2@192.168.1.97`

- In terminal 2: setup SSH key with rights and in your user home:

  `cp ./keys/bticinokey ~/.ssh/bticinokey`
  `chmod 600 ~/.ssh/bticinokey`

- In terminal 2: config SSH easy access:

  `$ cat ~/.ssh/config`

  ```sh
  Host bticino
    HostName 192.168.1.97
    User root2
    StrictHostKeyChecking no
    IdentityFile ~/.ssh/bticinokey
  ```

- In terminal 1: (inside your Bticino)

  ```sh
  cd
  mkdir .ssh
  cp /etc/dropbear/authorized_keys .ssh/authorized_keys
  ```

- In terminal 2: test:

  `ssh bticino`

- In terminal 1: (inside your Bticino) change to RO access:
  - `mount -oremount,ro /`
