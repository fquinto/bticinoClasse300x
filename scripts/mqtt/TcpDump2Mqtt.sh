#!/bin/sh

/etc/tcpdump2mqtt/TcpDump2Mqtt > /tmp/tcp_log.txt 2>&1 &
sleep 10

exit 0
