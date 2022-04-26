#! /bin/bash

dir="$1"
#dir="./C300X_010711"

if [ -z "$dir" ]; then
    echo "Use: sudo $0 <firmware dir>"
    exit 0
fi

if [[ $EUID -eq 0 ]]; then
   echo "This script do not need to be run as root"
fi

firmware="${dir}-new.fwz"

pass="C300X"
cd "$dir"
zip -r -Z deflate -P "${pass}" "../${firmware}" .
