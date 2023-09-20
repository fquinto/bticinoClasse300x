
# Write File

topic = Bticino/rx

{
    "command": "write_file",
    "file_path": "/path/to/your/file.txt",
    "data": "This is the content to write to the file."
}

# Write LED's ON

topic = Bticino/rx

echo '255' > /sys/class/leds/led_ans_machine/brightness
echo '255' > /sys/class/leds/led_exc_call/brightness
echo '255' > /sys/class/leds/led_gwifi/brightness
echo '255' > /sys/class/leds/led_lock/brightness
echo '255' > /sys/class/leds/led_memo/brightness
echo '255' > /sys/class/leds/led_vct_green/brightness
echo '255' > /sys/class/leds/led_vct_red/brightness

{
    "command": "write_file",
    "file_path": "/sys/class/leds/led_ans_machine/brightness",
    "data": "255"
}

# Write LED's OFF

topic = Bticino/rx

{
    "command": "write_file",
    "file_path": "/sys/class/leds/led_ans_machine/brightness",
    "data": "0"
}

# Write LED's BLINK

topic = Bticino/rx

{
    "command": "write_file",
    "file_path": "/sys/class/leds/led_ans_machine/brightness",
    "data": "127"
}

# Read File

topic = Bticino/rx

{
    "command": "read_file",
    "file_path": "/etc/tcpdump2mqtt/TcpDump2Mqtt.conf"
}

read topic = Bticino/file_content_topic

# Execute Command

topic = Bticino/rx

{
    "command": "execute_command",
    "data": "ls -la /etc/tcpdump2mqtt/"
}

read topic = Bticino/command_result_topic

# Key management JSON response

Reading topic: Bticino/key

value can be: pressed/released
key can be: KEY_1, KEY_2, KEY_3, KEY_4

{
    "key": "KEY_1",
    "draw": "key",
    "value": "pressed"
}

{
    "key": "KEY_2",
    "draw": "star",
    "value": "pressed"
}

{
    "key": "KEY_3",
    "draw": "eye",
    "value": "pressed"
}

{
    "key": "KEY_4",
    "draw": "phone",
    "value": "pressed"
}
