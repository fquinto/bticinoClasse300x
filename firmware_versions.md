### Firmware BTICINO

## C300X

Search for new firmware in this website: https://www.homesystems-legrandgroup.com/ and write the next "model number" in search bar.

Model 344642 = CLASSE 300X13E Touch Screen handsfree video intern
https://www.homesystems-legrandgroup.com/home/-/productsheets/2486279

Model 344643 = CLASSE 300X13E Touch Screen handsfree video intern
https://www.homesystems-legrandgroup.com/home/-/productsheets/2486306

- [Version 1.7.17](https://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName=C300X_010717.fwz&fileId=58107.23188.15908.12349)

- [Version 1.7.17](https://prodlegrandressourcespkg.blob.core.windows.net/packagecontainer/package_343bb0abacf05a27c6c146848e85d1de2425700e_h.tar.gz)

- [Version 1.7.19](https://prodlegrandressourcespkg.blob.core.windows.net/binarycontainer/bt_344642_3_0_0-c300x_010719_1_7_19.bin)

## C100X

Model 344682 = Classe100 X16E 2 WIRES / Wi-Fi handsfree video internal unit with inductive loop
https://www.homesystems-legrandgroup.com/home/-/productsheets/2595814

- [Version 1.5.1](https://www.homesystems-legrandgroup.com/MatrixENG/liferay/bt_mxLiferayCheckout.jsp?fileFormat=generic&fileName=C100X_010501.fwz&fileId=58107.23188.46381.34528)


## Download new firmware from unlocked unit

For the C100X, there has been newer 1.5.4 firmware for some time. Unfortunately this new firmware isn't available for download on the Legrand support page. From what I've been told by Legrand support, new firmware wil not be made available anymore via the website but only using the mobile app notification. In case the new firmware gets released on the legrand site, this file can be updated.

In any case, here is a workaround if you already have an 'unlocked' device.

When a new firmware is available, you'll receive a notification in the mobile app.
At this time, the new firmware file has already been downloaded by the unit for installation.
Don't install the new firmware using the app. Instead SCP to the unit, download the firmware, edit it using the scripts in this repo and update the firmware using MyHomeSuite.

- The firmwarefile can be found at /home/bticino/cfg/extra/FW/UPDATE.fwz
