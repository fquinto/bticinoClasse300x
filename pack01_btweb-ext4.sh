#! /bin/bash


fwzdir="$1"
#fwzdir="./C300X_010711"

if [ -z "$fwzdir" ]; then
    echo "Use: sudo $0 <extracted fwz dir>"
    exit 0
fi

if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root"
   exit 1
fi

image="${fwzdir}/btweb_only.ext4"
workdir="./tmp"

#mkdir -p "${workdir}/mnt_btweb"

function create_image() {
    # ext4 image size 1073741824

    dd if=/dev/zero of="$image" bs=1073741824 count=1
    mkfs.ext4 "$image"
    mount -t ext4 -o loop "${fwzdir}/btweb_only.ext4" "${workdir}/mnt_btweb"

}


#cp -vRa "${workdir}/mnt_btweb" "${workdir}/btweb_ext4"
umount "${workdir}/mnt_btweb"

gzip ${image}

