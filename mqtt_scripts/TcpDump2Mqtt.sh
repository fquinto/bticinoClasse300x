#!/bin/sh

# Prevent duplicate instances — if TcpDump2Mqtt is already running, do nothing.
# This can happen if the init system calls this script more than once.
if /usr/bin/pgrep -f "TcpDump2Mqtt$" > /dev/null 2>&1; then
	echo "TcpDump2Mqtt already running, skipping"
	exit 0
fi

/etc/tcpdump2mqtt/TcpDump2Mqtt > /tmp/tcp_log.txt 2>&1 &
sleep 10

exit 0
