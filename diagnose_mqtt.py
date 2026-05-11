#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
BTicino MQTT Diagnostic Tool

This script helps diagnose issues with MQTT preparation and installation
in BTicino Classe300x firmware modification process.
"""

import os
import sys
import subprocess
import argparse
from datetime import datetime

class MQTTDiagnostic:
    """MQTT Diagnostic and verification tool."""
    
    def __init__(self):
        self.issues_found = []
        self.warnings = []
        
    def log_issue(self, issue):
        """Log a critical issue."""
        self.issues_found.append(issue)
        print(f"❌ ISSUE: {issue}")
    
    def log_warning(self, warning):
        """Log a warning."""
        self.warnings.append(warning)
        print(f"⚠️  WARNING: {warning}")
    
    def log_success(self, message):
        """Log a success message."""
        print(f"✅ {message}")
    
    def check_mqtt_source_files(self, base_path='.'):
        """Check if all required MQTT source files exist."""
        print("\n🔍 Checking MQTT source files...")
        
        required_files = [
            'mqtt_scripts/TcpDump2Mqtt.conf',
            'mqtt_scripts/TcpDump2Mqtt',
            'mqtt_scripts/TcpDump2Mqtt.sh',
            'mqtt_scripts/StartMqttSend',
            'mqtt_scripts/StartMqttReceive',
            'mqtt_scripts/filter.py',
            'mqtt_scripts/jq-linux-armhf',
            'mqtt_scripts/evtest',
        ]
        
        for file_path in required_files:
            full_path = os.path.join(base_path, file_path)
            if os.path.exists(full_path):
                self.log_success(f"Found: {file_path}")
            else:
                self.log_issue(f"Missing required file: {file_path}")
    
    def check_mqtt_config(self, base_path='.'):
        """Check MQTT configuration file."""
        print("\n🔍 Checking MQTT configuration...")
        
        config_path = os.path.join(base_path, 'mqtt_scripts/TcpDump2Mqtt.conf')
        if not os.path.exists(config_path):
            self.log_issue(f"MQTT config file not found: {config_path}")
            return
        
        try:
            with open(config_path, 'r', encoding='utf-8') as f:
                content = f.read()
            
            # Check for MQTT_HOST
            mqtt_host_found = False
            mqtt_host_value = None
            
            for line_num, line in enumerate(content.splitlines(), 1):
                if line.startswith('MQTT_HOST='):
                    mqtt_host_found = True
                    mqtt_host_value = line.split('=', 1)[1].strip()
                    
                    if mqtt_host_value:
                        self.log_success(f"MQTT_HOST configured: '{mqtt_host_value}' (line {line_num})")
                    else:
                        self.log_issue(f"MQTT_HOST is empty on line {line_num}")
                    break
            
            if not mqtt_host_found:
                self.log_issue("MQTT_HOST parameter not found in configuration file")
            
            # Check other important parameters
            important_params = ['MQTT_PORT', 'TOPIC_RX', 'TOPIC_DUMP']
            for param in important_params:
                if f'{param}=' in content:
                    self.log_success(f"Found parameter: {param}")
                else:
                    self.log_warning(f"Parameter {param} not found in config")
                    
        except Exception as e:
            self.log_issue(f"Could not read MQTT config file: {e}")
    
    def check_certificates(self, base_path='.'):
        """Check certificate files."""
        print("\n🔍 Checking certificate files...")
        
        cert_files = [
            'certs/m2mqtt_ca.crt',
            'certs/m2mqtt_srv_bticino.crt', 
            'certs/m2mqtt_srv_bticino.key'
        ]
        
        found_certs = 0
        for cert_file in cert_files:
            cert_path = os.path.join(base_path, cert_file)
            if os.path.exists(cert_path):
                self.log_success(f"Found certificate: {cert_file}")
                found_certs += 1
            else:
                print(f"ℹ️  Optional certificate not found: {cert_file}")
        
        if found_certs == 0:
            print("ℹ️  No certificates found (this is OK for non-TLS MQTT)")
        else:
            print(f"ℹ️  Found {found_certs} certificate files")
    
    def check_system_requirements(self):
        """Check system requirements for firmware preparation."""
        print("\n🔍 Checking system requirements...")
        
        # Check if running as root or with sudo access
        try:
            result = subprocess.run(['sudo', '-n', 'true'], 
                                   capture_output=True, text=True, timeout=5)
            if result.returncode == 0:
                self.log_success("Sudo access available")
            else:
                self.log_issue("No sudo access - firmware preparation requires sudo")
        except subprocess.TimeoutExpired:
            self.log_warning("Sudo check timed out")
        except Exception as e:
            self.log_issue(f"Could not check sudo access: {e}")
        
        # Check for required commands
        required_commands = ['mount', 'umount', 'mkdir', 'cp', 'chmod', 'ln']
        for cmd in required_commands:
            try:
                result = subprocess.run(['which', cmd], 
                                       capture_output=True, text=True)
                if result.returncode == 0:
                    self.log_success(f"Found required command: {cmd}")
                else:
                    self.log_issue(f"Missing required command: {cmd}")
            except Exception as e:
                self.log_issue(f"Could not check command {cmd}: {e}")
    
    def check_mount_point(self, mount_point='/media/mounted'):
        """Check mount point directory."""
        print(f"\n🔍 Checking mount point: {mount_point}")
        
        if os.path.exists(mount_point):
            if os.path.isdir(mount_point):
                self.log_success(f"Mount point directory exists: {mount_point}")
                
                # Check if currently mounted
                try:
                    result = subprocess.run(['mount'], capture_output=True, text=True)
                    if mount_point in result.stdout:
                        self.log_warning(f"Something is currently mounted at {mount_point}")
                        print("  You may need to unmount before firmware preparation")
                    else:
                        self.log_success(f"Mount point is available")
                except Exception as e:
                    self.log_warning(f"Could not check mount status: {e}")
            else:
                self.log_issue(f"Mount point exists but is not a directory: {mount_point}")
        else:
            print(f"ℹ️  Mount point will be created during preparation: {mount_point}")
    
    def verify_firmware_structure(self, firmware_dir):
        """Verify firmware directory structure after preparation."""
        print(f"\n🔍 Verifying prepared firmware structure: {firmware_dir}")
        
        if not os.path.exists(firmware_dir):
            self.log_issue(f"Firmware directory not found: {firmware_dir}")
            return
        
        # Check if it's mounted
        try:
            result = subprocess.run(['mount'], capture_output=True, text=True)
            if firmware_dir in result.stdout:
                self.log_success(f"Firmware is currently mounted at: {firmware_dir}")
            else:
                self.log_warning(f"Firmware directory exists but is not mounted")
        except Exception as e:
            self.log_warning(f"Could not check mount status: {e}")
        
        # Check MQTT files in mounted firmware
        mqtt_files = [
            'etc/tcpdump2mqtt/TcpDump2Mqtt.conf',
            'etc/tcpdump2mqtt/TcpDump2Mqtt',
            'etc/tcpdump2mqtt/TcpDump2Mqtt.sh',
            'etc/tcpdump2mqtt/StartMqttSend',
            'etc/tcpdump2mqtt/StartMqttReceive',
            'home/root/filter.py',
            'usr/bin/jq',
            'usr/bin/evtest',
            'etc/rc5.d/S99TcpDump2Mqtt'
        ]
        
        for mqtt_file in mqtt_files:
            file_path = os.path.join(firmware_dir, mqtt_file)
            if os.path.exists(file_path):
                self.log_success(f"Found in firmware: {mqtt_file}")
            else:
                self.log_issue(f"Missing from firmware: {mqtt_file}")
    
    def generate_report(self):
        """Generate diagnostic report."""
        print("\n" + "="*60)
        print("📋 DIAGNOSTIC REPORT")
        print("="*60)
        print(f"🕐 Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        print()
        
        if not self.issues_found and not self.warnings:
            print("🎉 No issues found! MQTT configuration appears to be correct.")
        else:
            if self.issues_found:
                print(f"❌ Critical Issues Found: {len(self.issues_found)}")
                for i, issue in enumerate(self.issues_found, 1):
                    print(f"   {i}. {issue}")
                print()
            
            if self.warnings:
                print(f"⚠️  Warnings: {len(self.warnings)}")
                for i, warning in enumerate(self.warnings, 1):
                    print(f"   {i}. {warning}")
                print()
        
        print("💡 RECOMMENDATIONS:")
        if self.issues_found:
            print("   • Address all critical issues before attempting firmware preparation")
            print("   • Ensure all required MQTT files are present in mqtt_scripts/ directory")
            print("   • Verify MQTT_HOST is properly configured in TcpDump2Mqtt.conf")
        
        if any("sudo" in issue.lower() for issue in self.issues_found):
            print("   • Run firmware preparation with proper sudo privileges")
        
        print("   • Check the improved error messages in main.py during firmware preparation")
        print("   • Use 'sudo python3 main.py' to see detailed error output")
        print()

def main():
    parser = argparse.ArgumentParser(description='BTicino MQTT Diagnostic Tool')
    parser.add_argument('--base-path', default='.', 
                       help='Base path to BTicino project directory (default: current directory)')
    parser.add_argument('--verify-firmware', 
                       help='Path to mounted firmware directory to verify')
    parser.add_argument('--mount-point', default='/media/mounted',
                       help='Mount point to check (default: /media/mounted)')
    
    args = parser.parse_args()
    
    diagnostic = MQTTDiagnostic()
    
    print("🔧 BTicino MQTT Diagnostic Tool")
    print("=" * 40)
    
    # Run all checks
    diagnostic.check_system_requirements()
    diagnostic.check_mqtt_source_files(args.base_path)
    diagnostic.check_mqtt_config(args.base_path)
    diagnostic.check_certificates(args.base_path)
    diagnostic.check_mount_point(args.mount_point)
    
    if args.verify_firmware:
        diagnostic.verify_firmware_structure(args.verify_firmware)
    
    # Generate final report
    diagnostic.generate_report()
    
    # Exit with appropriate code
    if diagnostic.issues_found:
        sys.exit(1)
    else:
        sys.exit(0)

if __name__ == '__main__':
    main()