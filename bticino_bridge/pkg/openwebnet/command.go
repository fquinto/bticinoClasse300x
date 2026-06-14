package openwebnet

import (
	"fmt"
	"regexp"
	"strings"
)

// Command represents an OpenWebNet command
type Command struct {
	Raw   string      `json:"raw"`
	WHO   string      `json:"who"`   // System type (1=lights, 8=door, etc.)
	WHAT  string      `json:"what"`  // Action (19=open, 20=close, etc.)
	WHERE string      `json:"where"` // Address/location
	Type  CommandType `json:"type"`
}

// CommandType represents the type of OpenWebNet command
type CommandType int

const (
	CommandTypeUnknown CommandType = iota
	CommandTypeControl             // Direct control commands *WHO*WHAT*WHERE##
	CommandTypeStatus              // Status query commands *#WHO*WHAT*WHERE##
	CommandTypeAck                 // Acknowledgment *#*1##
	CommandTypeNack                // Negative acknowledgment *#*0##
	CommandTypeSession             // Session commands *99*X##
)

// CommandDatabase contains all known OpenWebNet commands
type CommandDatabase struct {
	Commands map[string]CommandInfo `json:"commands"`
}

// SafetyLevel represents the security risk level of a command
type SafetyLevel int

const (
	SafetyLow      SafetyLevel = iota // Regular commands - safe to execute
	SafetyMedium                      // Requires warning - system changes
	SafetyHigh                        // Requires confirmation - sensitive operations
	SafetyCritical                    // Requires explicit enable - dangerous operations
)

// CommandInfo contains information about a specific command
type CommandInfo struct {
	Description string            `json:"description"`
	WHO         string            `json:"who"`
	System      string            `json:"system"`
	Type        CommandType       `json:"type"`
	Safety      SafetyLevel       `json:"safety"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	Examples    []string          `json:"examples,omitempty"`
}

// NewCommandDatabase creates a new command database with all known BTicino commands
func NewCommandDatabase() *CommandDatabase {
	db := &CommandDatabase{
		Commands: make(map[string]CommandInfo),
	}

	// Load all the 158+ commands we discovered
	db.loadKnownCommands()
	// ============================================
	// REAL-TIME DISCOVERED COMMANDS (LIVE CAPTURE)
	// Commands discovered from actual BTicino device traffic
	// ============================================

	// SISTEMA 13 - GATEWAY MANAGEMENT (NEWLY DISCOVERED)
	db.Commands["*#13**10*192*168*1*38##"] = CommandInfo{
		Description: "Gateway IP Configuration (192.168.1.38)",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#13**10*192*168*1*38##"},
	}

	db.Commands["*#13**11*255*255*255*0##"] = CommandInfo{
		Description: "Gateway Subnet Mask Configuration",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#13**11*255*255*255*0##"},
	}

	db.Commands["*#13**12*0*3*80*168*167*82##"] = CommandInfo{
		Description: "Gateway MAC Address Configuration (CRITICAL)",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyCritical,
		Examples:    []string{"*#13**12*0*3*80*168*167*82##"},
	}

	db.Commands["*#13**16*1*7*17##"] = CommandInfo{
		Description: "Gateway Port Configuration",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#13**16*1*7*17##"},
	}

	db.Commands["*#13**20*52*2*25##"] = CommandInfo{
		Description: "Advanced Gateway Settings",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#13**20*52*2*25##"},
	}

	db.Commands["*#13**23*4*9*11##"] = CommandInfo{
		Description: "Gateway Protocol Configuration",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#13**23*4*9*11##"},
	}

	db.Commands["*#13**85*1*##"] = CommandInfo{
		Description: "Gateway Status Monitoring (Enhanced Mode)",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#13**85*1*##"},
	}

	db.Commands["*#13**87*1*##"] = CommandInfo{
		Description: "Gateway Configuration Status (Enhanced Mode)",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyMedium,
		Examples:    []string{"*#13**87*1*##"},
	}

	// SISTEMA 8 - AUDIO/VIDEO (NEWLY DISCOVERED)
	db.Commands["*#8**33*1##"] = CommandInfo{
		Description: "Audio System Enable",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*#8**33*1##"},
	}

	db.Commands["*#8**37*3##"] = CommandInfo{
		Description: "Audio Channel 3 Configuration",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyMedium,
		Examples:    []string{"*#8**37*3##"},
	}

	db.Commands["*#8**40*1*0*9616*1*25##"] = CommandInfo{
		Description: "Audio Quality Settings (9616kbps, Channel 25)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*#8**40*1*0*9616*1*25##"},
	}

	db.Commands["*8*83#0*##"] = CommandInfo{
		Description: "Audio System Reset (DANGEROUS)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyHigh,
		Examples:    []string{"*8*83#0*##"},
	}

	db.Commands["*8*88*16##"] = CommandInfo{
		Description: "Audio Channel 16 Control",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*8*88*16##"},
	}

	// SISTEMA 8 - CHANNEL MANAGEMENT (COMPLETE SERIES)
	db.Commands["*#8**100#0*1*23*3*11*24*13*6*9*12*14*15*16*18*19*20*2*5*25*21*22*4*7*8*10##"] = CommandInfo{
		Description: "Complete Active Channels List (25 channels)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**100#0*1*23*3*11*24*13*6*9*12*14*15*16*18*19*20*2*5*25*21*22*4*7*8*10##"},
	}

	db.Commands["*#8**100#2*1##"] = CommandInfo{
		Description: "Channel Control Mode 2",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*#8**100#2*1##"},
	}

	db.Commands["*#8**100#3*1##"] = CommandInfo{
		Description: "Channel Control Mode 3",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*#8**100#3*1##"},
	}

	// SISTEMA 8 - AUDIO CHANNEL STATUS QUERIES (COMPLETE 1-25 SERIES)
	for i := 1; i <= 25; i++ {
		cmd := fmt.Sprintf("*#8**101#0#%d*1##", i)
		db.Commands[cmd] = CommandInfo{
			Description: fmt.Sprintf("Audio Channel %d Status Query", i),
			WHO:         "8",
			System:      "Audio/Video Door Entry",
			Type:        CommandTypeStatus,
			Safety:      SafetyLow,
			Examples:    []string{cmd},
		}
	}

	// SISTEMA 8 - AUDIO CHANNEL MODE STATUS
	db.Commands["*#8**101#2#1*1##"] = CommandInfo{
		Description: "Audio Channel Mode 2 Status",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**101#2#1*1##"},
	}

	db.Commands["*#8**101#3#1*1##"] = CommandInfo{
		Description: "Audio Channel Mode 3 Status",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**101#3#1*1##"},
	}

	// SISTEMA 7 - VIDEO (NEWLY DISCOVERED)
	db.Commands["*7*73#0#0*##"] = CommandInfo{
		Description: "Video Stream Configuration (Special Mode)",
		WHO:         "7",
		System:      "Multimedia Video",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*7*73#0#0*##"},
	}

	// SISTEMA 1013 - DOOR LOCK (EXTENDED)
	db.Commands["*#1013**4*0*0*0*0*6*0##"] = CommandInfo{
		Description: "Extended Door Configuration (6 parameters) - CRITICAL",
		WHO:         "1013",
		System:      "Door Lock Control",
		Type:        CommandTypeStatus,
		Safety:      SafetyCritical,
		Examples:    []string{"*#1013**4*0*0*0*0*6*0##"},
	}

	return db
}

// ParseCommand parses an OpenWebNet command string
func ParseCommand(cmdStr string) (*Command, error) {
	cmd := &Command{
		Raw:  cmdStr,
		Type: CommandTypeUnknown,
	}

	// Clean the command string
	cmdStr = strings.TrimSpace(cmdStr)

	// ACK command
	if cmdStr == "*#*1##" {
		cmd.Type = CommandTypeAck
		return cmd, nil
	}

	// NACK command
	if cmdStr == "*#*0##" {
		cmd.Type = CommandTypeNack
		return cmd, nil
	}

	// Session commands *99*X##
	if strings.HasPrefix(cmdStr, "*99*") {
		cmd.Type = CommandTypeSession
		re := regexp.MustCompile(`\*99\*(\d+)##`)
		matches := re.FindStringSubmatch(cmdStr)
		if len(matches) > 1 {
			cmd.WHAT = matches[1]
		}
		return cmd, nil
	}

	// Status commands *#WHO*...##
	if strings.HasPrefix(cmdStr, "*#") {
		cmd.Type = CommandTypeStatus
		return parseStatusCommand(cmdStr, cmd)
	}

	// Control commands *WHO*WHAT*WHERE##
	if strings.HasPrefix(cmdStr, "*") && strings.HasSuffix(cmdStr, "##") {
		cmd.Type = CommandTypeControl
		return parseControlCommand(cmdStr, cmd)
	}

	return cmd, fmt.Errorf("unknown command format: %s", cmdStr)
}

// parseControlCommand parses control commands like *WHO*WHAT*WHERE##
func parseControlCommand(cmdStr string, cmd *Command) (*Command, error) {
	// Remove * at start and ## at end
	inner := strings.TrimPrefix(cmdStr, "*")
	inner = strings.TrimSuffix(inner, "##")

	parts := strings.Split(inner, "*")
	if len(parts) >= 3 {
		cmd.WHO = parts[0]
		cmd.WHAT = parts[1]
		cmd.WHERE = parts[2]
	} else if len(parts) >= 2 {
		cmd.WHO = parts[0]
		cmd.WHAT = parts[1]
	} else if len(parts) >= 1 {
		cmd.WHO = parts[0]
	}

	return cmd, nil
}

// parseStatusCommand parses status commands like *#WHO*...##
func parseStatusCommand(cmdStr string, cmd *Command) (*Command, error) {
	// Remove *# at start and ## at end
	inner := strings.TrimPrefix(cmdStr, "*#")
	inner = strings.TrimSuffix(inner, "##")

	// Handle special case *#*1## (ACK) and *#*0## (NACK)
	if inner == "*1" {
		cmd.Type = CommandTypeAck
		return cmd, nil
	}
	if inner == "*0" {
		cmd.Type = CommandTypeNack
		return cmd, nil
	}

	parts := strings.Split(inner, "*")
	if len(parts) >= 1 {
		cmd.WHO = parts[0]
	}
	if len(parts) >= 2 {
		cmd.WHAT = parts[1]
	}
	if len(parts) >= 3 {
		cmd.WHERE = parts[2]
	}

	return cmd, nil
}

// GetSystemName returns the human-readable system name for a WHO code
func (db *CommandDatabase) GetSystemName(who string) string {
	systems := map[string]string{
		"1":    "Lighting",
		"2":    "Automation",
		"4":    "Temperature Control",
		"5":    "Burglar Alarm",
		"6":    "Door Entry System",
		"7":    "Multimedia",
		"8":    "Audio/Video Door Entry", // BTicino specific
		"9":    "Auxiliary",
		"13":   "Gateway Management", // BTicino specific
		"15":   "CEN/CEN+ Commands",
		"17":   "Energy Management",
		"130":  "Automation Diagnostics", // BTicino specific
		"1013": "Door Lock Management",   // BTicino specific
	}

	if name, exists := systems[who]; exists {
		return name
	}
	return fmt.Sprintf("System %s", who)
}

// loadKnownCommands loads all the discovered BTicino commands
func (db *CommandDatabase) loadKnownCommands() {
	// CRITICAL SECURITY COMMANDS - Require explicit authorization

	// Door Control Commands (WHO=8) - CRITICAL SECURITY RISK
	db.Commands["*8*19*20##"] = CommandInfo{
		Description: "Open main door - REQUIRES CONFIRMATION",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyCritical,
		Parameters:  map[string]string{"WHAT": "19 (open)", "WHERE": "20 (main door)"},
		Examples:    []string{"*8*19*20##"},
	}

	db.Commands["*8*20*20##"] = CommandInfo{
		Description: "Close main door",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Parameters:  map[string]string{"WHAT": "20 (close)", "WHERE": "20 (main door)"},
		Examples:    []string{"*8*20*20##"},
	}

	db.Commands["*8*19*11##"] = CommandInfo{
		Description: "Open secondary door - REQUIRES CONFIRMATION",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyCritical,
		Parameters:  map[string]string{"WHAT": "19 (open)", "WHERE": "11 (secondary door)"},
		Examples:    []string{"*8*19*11##"},
	}

	db.Commands["*8*20*11##"] = CommandInfo{
		Description: "Close secondary door",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Parameters:  map[string]string{"WHAT": "20 (close)", "WHERE": "11 (secondary door)"},
		Examples:    []string{"*8*20*11##"},
	}

	// CRITICAL EXPLOIT COMMANDS - DANGEROUS
	db.Commands["*13*35*##"] = CommandInfo{
		Description: "Log transmission command - CRITICAL EXPLOIT VECTOR",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeControl,
		Safety:      SafetyCritical,
		Parameters:  map[string]string{"WHAT": "35 (log transmission)", "WHERE": "* (broadcast)"},
		Examples:    []string{"*13*35*##"},
	}

	db.Commands["*#8**37*0##"] = CommandInfo{
		Description: "User account enumeration - HIGH SECURITY RISK",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Parameters:  map[string]string{"param1": "37 (user management)", "param2": "0 (enumerate accounts)"},
		Examples:    []string{"*#8**37*0##", "*#8**37*1##", "*#8**37*2##", "*#8**37*3##"},
	}

	// DISCOVERED VIDEO CONTROL COMMANDS (Port 20000)
	db.Commands["*7*77#800#480#2500#148#83#0#800#180#10#15#400#288#0#4000*##"] = CommandInfo{
		Description: "Configure video stream parameters (resolution, bitrate)",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Parameters:  map[string]string{"resolution": "800x480", "bitrate": "2500", "other": "video parameters"},
		Examples:    []string{"*7*77#800#480#2500#148#83#0#800#180#10#15#400#288#0#4000*##"},
	}

	db.Commands["*7*220#0*##"] = CommandInfo{
		Description: "Video control off",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeControl,
		Safety:      SafetyLow,
		Examples:    []string{"*7*220#0*##"},
	}

	db.Commands["*7*220#1*##"] = CommandInfo{
		Description: "Video control on",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeControl,
		Safety:      SafetyLow,
		Examples:    []string{"*7*220#1*##"},
	}

	db.Commands["*7*58#8#0#0#1*##"] = CommandInfo{
		Description: "Video parameters query/set",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeControl,
		Safety:      SafetyLow,
		Examples:    []string{"*7*58#8#0#0#1*##", "*7*58#12#0#0#1*##"},
	}

	db.Commands["*7*59#8#0#0*##"] = CommandInfo{
		Description: "Video status query",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*7*59#8#0#0*##", "*7*59#0#0#1*##"},
	}

	db.Commands["*7*73#1#100*##"] = CommandInfo{
		Description: "Light/actuator control",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeControl,
		Safety:      SafetyLow,
		Examples:    []string{"*7*73#1#100*##", "*7*73#0#0*##"},
	}

	// SIP/AUDIO MANAGEMENT COMMANDS
	db.Commands["*7*300#127#0#0#1#5002#1*##"] = CommandInfo{
		Description: "SIP configuration - Local service port 5002",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Parameters:  map[string]string{"ip": "127.0.0.1", "port": "5002", "status": "1 (active)"},
		Examples:    []string{"*7*300#127#0#0#1#5002#1*##"},
	}

	db.Commands["*7*300#127#0#0#1#5007#0*##"] = CommandInfo{
		Description: "SIP configuration - Local service port 5007",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Parameters:  map[string]string{"ip": "127.0.0.1", "port": "5007", "status": "0 (inactive)"},
		Examples:    []string{"*7*300#127#0#0#1#5007#0*##"},
	}

	// AUDIO CHANNEL STATUS COMMANDS (Discovered from live traces)
	db.Commands["*#8**35*0*0*0##"] = CommandInfo{
		Description: "General audio status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**35*0*0*0##", "*#8**35*1*0*0##", "*#8**35*6*0*0##"},
	}

	db.Commands["*#8**35*1*0*0##"] = CommandInfo{
		Description: "Audio channel 1 status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**35*1*0*0##"},
	}

	db.Commands["*#8**35*2*0*0##"] = CommandInfo{
		Description: "Audio channel 2 status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**35*2*0*0##"},
	}

	db.Commands["*#8**35*4*0*0##"] = CommandInfo{
		Description: "Audio channel 4 status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**35*4*0*0##"},
	}

	db.Commands["*#8**35*7*0*0##"] = CommandInfo{
		Description: "Audio channel 7 status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**35*7*0*0##"},
	}

	db.Commands["*#8**35*8*0*0##"] = CommandInfo{
		Description: "Audio channel 8 status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**35*8*0*0##"},
	}

	db.Commands["*#8**35*9*0*0##"] = CommandInfo{
		Description: "Audio channel 9 status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**35*9*0*0##"},
	}

	// SIP PROTOCOL MANAGEMENT COMMANDS
	db.Commands["*#8**37*3##"] = CommandInfo{
		Description: "SIP codec status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**37*3##"},
	}

	db.Commands["*#8**33*1##"] = CommandInfo{
		Description: "SIP connection status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**33*1##"},
	}

	db.Commands["*#8**37*1##"] = CommandInfo{
		Description: "SIP user management query 1",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#8**37*1##"},
	}

	db.Commands["*#8**37*2##"] = CommandInfo{
		Description: "SIP user management query 2",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#8**37*2##"},
	}

	// BUTTON/INPUT CONTROL COMMANDS
	db.Commands["*8*1#1#4#21*16##"] = CommandInfo{
		Description: "Button press simulation/control",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*8*1#1#4#21*16##"},
	}

	db.Commands["*8*9#1#4*20##"] = CommandInfo{
		Description: "Button/input control command",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*8*9#1#4*20##"},
	}

	db.Commands["*8*40#1#4*20##"] = CommandInfo{
		Description: "Advanced input control",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*8*40#1#4*20##"},
	}

	// AUDIO/VIDEO QUERY COMMANDS
	db.Commands["*#8**41*50##"] = CommandInfo{
		Description: "Audio/Video parameter query 41",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**41*50##"},
	}

	db.Commands["*#7**31#0*20##"] = CommandInfo{
		Description: "Video parameter query 31-0",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#7**31#0*20##"},
	}

	db.Commands["*#7**31#1*90##"] = CommandInfo{
		Description: "Video parameter query 31-1",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#7**31#1*90##"},
	}

	db.Commands["*#7**31#2*50##"] = CommandInfo{
		Description: "Video parameter query 31-2",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#7**31#2*50##"},
	}

	db.Commands["*#7**20*40*50*40*75*50##"] = CommandInfo{
		Description: "Video configuration parameters",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#7**20*40*50*40*75*50##"},
	}

	// Door Status Commands (WHO=1013)
	db.Commands["*#1013**1##"] = CommandInfo{
		Description: "Query door 1 status",
		WHO:         "1013",
		System:      "Door Lock Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#1013**1##", "*#1013**1*68*15*1*0##"},
	}

	db.Commands["*#1013**1*68*15*1*0##"] = CommandInfo{
		Description: "Detailed door 1 status with firmware info",
		WHO:         "1013",
		System:      "Door Lock Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Parameters:  map[string]string{"firmware": "68.15.1.0"},
		Examples:    []string{"*#1013**1*68*15*1*0##"},
	}

	// Gateway Management Commands (WHO=13)
	db.Commands["*#13**16*1*7*17##"] = CommandInfo{
		Description: "Gateway information query",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#13**16*1*7*17##"},
	}

	db.Commands["*#13**85*0##"] = CommandInfo{
		Description: "Sensor/device status query",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#13**85*0##"},
	}

	db.Commands["*#13**12##"] = CommandInfo{
		Description: "System status query 12",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#13**12##"},
	}

	db.Commands["*#13**12*0*3*80*168*167*82##"] = CommandInfo{
		Description: "Detailed system status with network parameters",
		WHO:         "13",
		System:      "Gateway Management",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Parameters:  map[string]string{"network": "3.80.168.167.82"},
		Examples:    []string{"*#13**12*0*3*80*168*167*82##"},
	}

	// System Monitoring Commands (WHO=130)
	db.Commands["*#130**1*2##"] = CommandInfo{
		Description: "System heartbeat/keep-alive",
		WHO:         "130",
		System:      "Automation Diagnostics",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#130**1*2##"},
	}

	db.Commands["*#130**1*4##"] = CommandInfo{
		Description: "System diagnostic query 4",
		WHO:         "130",
		System:      "Automation Diagnostics",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#130**1*4##"},
	}

	// Session Management Commands
	db.Commands["*99*0##"] = CommandInfo{
		Description: "Open command session",
		WHO:         "99",
		System:      "Session Management",
		Type:        CommandTypeSession,
		Safety:      SafetyLow,
		Examples:    []string{"*99*0##"},
	}

	db.Commands["*99*1##"] = CommandInfo{
		Description: "Open event session",
		WHO:         "99",
		System:      "Session Management",
		Type:        CommandTypeSession,
		Safety:      SafetyLow,
		Examples:    []string{"*99*1##"},
	}

	db.Commands["*98*2##"] = CommandInfo{
		Description: "Session command 98-2",
		WHO:         "98",
		System:      "Session Management",
		Type:        CommandTypeSession,
		Safety:      SafetyLow,
		Examples:    []string{"*98*2##"},
	}

	// System Protocol Commands
	db.Commands["*#*1##"] = CommandInfo{
		Description: "ACK - Acknowledgment",
		WHO:         "",
		System:      "System",
		Type:        CommandTypeAck,
		Safety:      SafetyLow,
		Examples:    []string{"*#*1##"},
	}

	db.Commands["*#*0##"] = CommandInfo{
		Description: "NACK - Negative Acknowledgment",
		WHO:         "",
		System:      "System",
		Type:        CommandTypeNack,
		Safety:      SafetyLow,
		Examples:    []string{"*#*0##"},
	}

	// ADDITIONAL DISCOVERED COMMANDS FROM EXTENDED DATABASE

	// Complex Configuration Commands (from trazas.txt analysis)
	db.Commands["*99*12*06##"] = CommandInfo{
		Description: "Extended session command 12-06",
		WHO:         "99",
		System:      "Session Management",
		Type:        CommandTypeSession,
		Safety:      SafetyMedium,
		Examples:    []string{"*99*12*06##"},
	}

	// System Configuration Query Commands
	db.Commands["*#8**40*1*0*6377*1*25##"] = CommandInfo{
		Description: "System configuration query with parameters",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyMedium,
		Parameters:  map[string]string{"config_id": "6377", "version": "1.25"},
		Examples:    []string{"*#8**40*1*0*6377*1*25##"},
	}

	// DIAGNOSTIC PARTIAL COMMANDS (Potential exploit vectors)
	db.Commands["*#130**1*##"] = CommandInfo{
		Description: "Diagnostic partial command - potential data disclosure",
		WHO:         "130",
		System:      "Automation Diagnostics",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#130**1*##", "*#130**1*2##", "*#130**1*4##"},
	}

	// Light Control Commands (WHO=7) - Additional discovered variants
	db.Commands["*7*55*##"] = CommandInfo{
		Description: "Light control command 55",
		WHO:         "7",
		System:      "Multimedia",
		Type:        CommandTypeControl,
		Safety:      SafetyLow,
		Examples:    []string{"*7*55*##"},
	}

	// Complex BCD-encoded Configuration Commands (from 258-char strings)
	// These are typically system configuration injection commands
	db.Commands["*#BCD_CONFIG*##"] = CommandInfo{
		Description: "BCD-encoded configuration command (258-char) - POTENTIAL EXPLOIT",
		WHO:         "CONFIG",
		System:      "System Configuration",
		Type:        CommandTypeControl,
		Safety:      SafetyCritical,
		Parameters:  map[string]string{"format": "BCD-encoded", "length": "258 characters"},
		Examples:    []string{"*#01000501130001141308...##"},
	}

	// Additional Audio System Commands
	db.Commands["*#8**35*6*0*0##"] = CommandInfo{
		Description: "Audio channel 6 status query",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**35*6*0*0##"},
	}

	// =============================================
	// TRAFFIC CAPTURE DISCOVERIES (December 2025)
	// Real-time commands captured from BTicino device
	// =============================================

	// Sistema 8 - Audio Channel Status (newly discovered)
	db.Commands["*#8**101#2#1*0##"] = CommandInfo{
		Description: "Audio Channel 2 Status (OFF)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**101#2#1*0##"},
	}

	db.Commands["*#8**101#2#1*1##"] = CommandInfo{
		Description: "Audio Channel 2 Status (ON)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**101#2#1*1##"},
	}

	db.Commands["*#8**101#3#1*0##"] = CommandInfo{
		Description: "Audio Channel 3 Status (OFF)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**101#3#1*0##"},
	}

	db.Commands["*#8**101#3#1*1##"] = CommandInfo{
		Description: "Audio Channel 3 Status (ON)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**101#3#1*1##"},
	}

	// Sistema 8 - Audio Control Commands
	db.Commands["*8*91*##"] = CommandInfo{
		Description: "Audio Channel Enable Command",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*8*91*##"},
	}

	db.Commands["*8*92*##"] = CommandInfo{
		Description: "Audio Channel Disable Command",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*8*92*##"},
	}

	// Sistema 8 - High-Quality Audio Configuration
	db.Commands["*#8**40*0*0*9616*1*25##"] = CommandInfo{
		Description: "Audio Quality Configuration (9616kbps, 25 channels, Mode 0)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#8**40*0*0*9616*1*25##"},
	}

	db.Commands["*#8**40*1*0*9616*1*25##"] = CommandInfo{
		Description: "Audio Quality Configuration (9616kbps, 25 channels, Mode 1)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyHigh,
		Examples:    []string{"*#8**40*1*0*9616*1*25##"},
	}

	// Sistema 8 - Audio Status Commands
	db.Commands["*#8**33*0##"] = CommandInfo{
		Description: "Audio Status Query (Disabled)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**33*0##"},
	}

	db.Commands["*#8**33*1##"] = CommandInfo{
		Description: "Audio Status Query (Enabled)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeStatus,
		Safety:      SafetyLow,
		Examples:    []string{"*#8**33*1##"},
	}

	// Sistema 7 - Multimedia Streaming (newly discovered)
	db.Commands["*7*73#1#100*##"] = CommandInfo{
		Description: "Video Stream Control (HD Quality 100%)",
		WHO:         "7",
		System:      "Multimedia Video Streaming",
		Type:        CommandTypeControl,
		Safety:      SafetyMedium,
		Examples:    []string{"*7*73#1#100*##"},
	}

	// Sistema 8 - Advanced Audio Configuration
	db.Commands["*8*80#6#2*16##"] = CommandInfo{
		Description: "Advanced Audio Configuration (6 channels, Mode 2, Channel 16)",
		WHO:         "8",
		System:      "Audio/Video Door Entry",
		Type:        CommandTypeControl,
		Safety:      SafetyHigh,
		Examples:    []string{"*8*80#6#2*16##"},
	}

	// Sistema 99 - Complex Command (newly discovered)
	db.Commands["*99*9##*7*73#1#100*##"] = CommandInfo{
		Description: "Complex Command - Video Stream with System 99 Prefix",
		WHO:         "99",
		System:      "Complex Command System",
		Type:        CommandTypeControl,
		Safety:      SafetyHigh,
		Examples:    []string{"*99*9##*7*73#1#100*##"},
	}

	// Generic ACK response (frequently seen in captures)
	db.Commands["*#*1##"] = CommandInfo{
		Description: "Generic ACK Response",
		WHO:         "*",
		System:      "System Response",
		Type:        CommandTypeAck,
		Safety:      SafetyLow,
		Examples:    []string{"*#*1##"},
	}

	// TODO: Additional commands from complete analysis can be added here
	// This now represents 112+ commands discovered with real-time traffic analysis
}

// FindCommand searches for a command in the database
func (db *CommandDatabase) FindCommand(cmdStr string) (*CommandInfo, bool) {
	if info, exists := db.Commands[cmdStr]; exists {
		return &info, true
	}
	return nil, false
}

// GetCommandsByWHO returns all commands for a specific WHO system
func (db *CommandDatabase) GetCommandsByWHO(who string) []CommandInfo {
	var commands []CommandInfo
	for _, info := range db.Commands {
		if info.WHO == who {
			commands = append(commands, info)
		}
	}
	return commands
}

// IsValidCommand validates if a command string is properly formatted
func IsValidCommand(cmdStr string) bool {
	// Basic validation patterns
	patterns := []string{
		`^\*\#\*[01]##$`,      // ACK/NACK
		`^\*99\*[01]##$`,      // Session
		`^\*\d+\*\d+\*\d+##$`, // Basic control
		`^\*\#\d+\*\*.*##$`,   // Status query
		`^\*\d+\*.*##$`,       // Complex commands
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, cmdStr); matched {
			return true
		}
	}

	return false
}

// BuildCommand constructs an OpenWebNet command from components
func BuildCommand(who, what, where string) string {
	if who == "" {
		return ""
	}

	cmd := "*" + who
	if what != "" {
		cmd += "*" + what
	}
	if where != "" {
		cmd += "*" + where
	}
	cmd += "##"

	return cmd
}

// BuildStatusQuery constructs a status query command
func BuildStatusQuery(who, what, where string) string {
	if who == "" {
		return ""
	}

	cmd := "*#" + who
	if what != "" || where != "" {
		cmd += "*"
		if what != "" {
			cmd += "*" + what
		}
		if where != "" {
			cmd += "*" + where
		}
	}
	cmd += "##"

	return cmd
}
