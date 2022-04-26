#! /bin/bash

firmware="$1"
if [ -z "$firmware" ]; then
    echo "Use: sudo $0 <firmware file fwz>"
    exit 0
fi

if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root"
   exit 1
fi

dir=$(basename -s ".fwz" "$firmware")

pass="C300X"
unzip -XKb -P "${pass}" -d "${dir}" "${firmware}"
