# Compare firmware versions of two devices

sudo mkdir /media/C300X_010717
sudo mkdir /media/C300X_010719

sudo mount -o loop btweb_only.ext4 /media/C300X_010719
sudo mount -o loop btweb_only.ext4 /media/C300X_010717

sudo diffoscope /media/C300X_010717 /media/C300X_010719

sudo diffoscope /media/C300X_010717 /media/C300X_010719
sudo diffoscope /media/C300X_010717/usr /media/C300X_010719/usr

sudo diffoscope /media/C300X_010717/home/bticino /media/C300X_010719/home/bticino
sudo diffoscope /media/C300X_010717/usr/local/bin /media/C300X_010719/usr/local/bin
