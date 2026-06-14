#!/bin/bash
# Script to configure local flexisip for SIP streaming
# This enables bticino_bridge to connect to flexisip on the device

set -e

DEVICE_IP="192.168.1.38"
BTICINO_USER="baresip"
DOMAIN="1754162.bs.iotleg.com"

echo "=== BTicino Flexisip Local Configuration ==="
echo "Device IP: $DEVICE_IP"
echo ""

# Check if running as root
if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: This script must be run as root"
    echo "Run: ssh bticino 'sudo bash /tmp/setup_flexisip_local.sh'"
    exit 1
fi

echo "Step 1: Remounting root filesystem as writable..."
mount -o remount,rw /

echo ""
echo "Step 2: Backup original flexisip configuration..."
cp /etc/init.d/flexisipsh /home/bticino/cfg/extra/flexisipsh.bak
cp /etc/flexisip/flexisip.conf /home/bticino/cfg/extra/flexisip.conf.bak
cp /etc/flexisip/users/users.db.txt /home/bticino/cfg/extra/users.db.txt.bak
cp /etc/flexisip/users/route.conf /home/bticino/cfg/extra/route.conf.bak
cp /etc/flexisip/users/route_int.conf /home/bticino/cfg/extra/route_int.conf.bak

echo ""
echo "Step 3: Modifying flexisipsh to listen on network IP (TCP 5060 + TLS 5061)..."
# Replace the entire flexisipsh file with corrected version
cat > /etc/init.d/flexisipsh << 'EOF'
#! /bin/sh

[ $# -le 3 ] || exit 0

PATH=/sbin:/usr/sbin:/bin:/usr/bin:/usr/bin
NAME=flexisip
PIDFILE=/var/run/$NAME.pid
DAEMON=/usr/bin/$NAME
DAEMON_ARGS="--daemon --syslog --pidfile $PIDFILE"
SCRIPTNAME=/etc/init.d/$NAME

[ -x "$DAEMON" ] || exit 0

[ -z "$3" ] || { DAEMON_ARGS+=" --p12-passphrase-file $3"; }

. /lib/lsb/init-functions

case "$1" in
  start)
    start-stop-daemon --start --quiet --exec $DAEMON -- $DAEMON_ARGS --transports "sip:$2:5060;maddr=$2 sip:127.0.0.1:5060;maddr=127.0.0.1 sips:$2:5061;maddr=$2;require-peer-certificate=1"
    /bin/touch /tmp/flexisip_restarted
    ;;
  stop)
    start-stop-daemon --stop --quiet --oknodo --retry=TERM/3/KILL/2 --exec $DAEMON
    killall -KILL $NAME 2>/dev/null
    rm -fr $PIDFILE
    rm -fr /var/tmp/flexisip-proxy*
    ;;
  *)
    echo "Usage: $SCRIPTNAME {start|stop}" >&2
    exit 3
    ;;
esac
EOF
chmod +x /etc/init.d/flexisipsh
echo "Created new flexisipsh with TCP 5060 (no cert) + TLS 5061 (cert required)"

echo ""
echo "Step 4: Adding baresip user to users.db.txt..."
# Check if user already exists
if ! grep -q "$BTICINO_USER@$DOMAIN" /etc/flexisip/users/users.db.txt; then
    # Copy the c300x user line and replace username
    C300X_LINE=$(grep "^c300x@" /etc/flexisip/users/users.db.txt | head -1)
    if [ -n "$C300X_LINE" ]; then
        BARESIP_LINE=$(echo "$C300X_LINE" | sed "s/c300x@/$BTICINO_USER@/g")
        echo "$BARESIP_LINE" >> /etc/flexisip/users/users.db.txt
        echo "Added: $BARESIP_LINE"
    else
        # Try fquinto line as fallback
        FQ_LINE=$(grep "^fquinto-" /etc/flexisip/users/users.db.txt | head -1)
        if [ -n "$FQ_LINE" ]; then
            HASH=$(echo "$FQ_LINE" | sed 's/.*md5:\([^ ]*\).*/\1/')
            echo "$BTICINO_USER@$DOMAIN md5:$HASH ;" >> /etc/flexisip/users/users.db.txt
            echo "Added: $BTICINO_USER@$DOMAIN md5:$HASH"
        fi
    fi
else
    echo "User $BTICINO_USER@$DOMAIN already exists"
fi

echo ""
echo "Step 5: Adding route for baresip user..."
if ! grep -q "sip:$BTICINO_USER@" /etc/flexisip/users/route.conf; then
    echo "<sip:$BTICINO_USER@$DOMAIN> sip:$DEVICE_IP" >> /etc/flexisip/users/route.conf
    echo "Added route: <sip:$BTICINO_USER@$DOMAIN> sip:$DEVICE_IP"
else
    echo "Route for $BTICINO_USER already exists"
fi

echo ""
echo "Step 6: Updating trusted-hosts in flexisip.conf..."
if ! grep -q "$DEVICE_IP" /etc/flexisip/flexisip.conf; then
    sed -i "s/trusted-hosts=127.0.0.1/trusted-hosts=127.0.0.1 $DEVICE_IP/g" /etc/flexisip/flexisip.conf
    echo "Added $DEVICE_IP to trusted-hosts"
else
    echo "$DEVICE_IP already in trusted-hosts"
fi

echo ""
echo "Step 7: Adding baresip to route_int.conf (receive doorbell)..."
if ! grep -q "$BTICINO_USER@" /etc/flexisip/users/route_int.conf; then
    sed -i "s/<sip:alluser@/<sip:$BTICINO_USER@$DOMAIN>, <sip:alluser@/g" /etc/flexisip/users/route_int.conf
    echo "Added $BTICINO_USER to doorbell group"
else
    echo "$BTICINO_USER already in doorbell group"
fi

echo ""
echo "=== Configuration Complete ==="
echo ""
echo "IMPORTANT: You need to reboot the device for changes to take effect."
echo ""
echo "After reboot:"
echo "1. Check flexisip is listening: ps aux | grep flexisip"
echo "2. Test SIP connection from bticino_bridge"
echo ""
echo "To verify configuration:"
echo "  cat /etc/init.d/flexisipsh | grep transports"
echo "  cat /etc/flexisip/users/users.db.txt"
echo "  cat /etc/flexisip/users/route.conf"
echo ""
