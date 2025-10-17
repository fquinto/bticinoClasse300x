#!/bin/bash

typeset -r THIS="$(realpath "$0")"

dir="$1"
#dir="./C300X_010711"

if [ -z "$dir" ]; then
	echo "Use: $THIS <firmware dir>"
	exit 0
fi

if [[ $EUID -eq 0 ]]; then
	echo "This script does not need to be run as root"
fi

firmware="${dir}-new.fwz"

pass="C300X"
cd "$dir"
zip -r -Z deflate -P "${pass}" "../${firmware}" .
