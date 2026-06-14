package openwebnet

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// SafetyManager handles dangerous command protection and auditing
type SafetyManager struct {
	logger               *logrus.Logger
	auditLog             *AuditLogger
	confirmationRequired bool
	enabledCritical      bool
}

// AuditLogger tracks all dangerous command execution
type AuditLogger struct {
	logger *logrus.Logger
}

// AuditEntry represents a logged security event
type AuditEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	Command   string      `json:"command"`
	Safety    SafetyLevel `json:"safety_level"`
	Source    string      `json:"source_ip,omitempty"`
	User      string      `json:"user,omitempty"`
	Action    string      `json:"action"` // "executed", "blocked", "confirmed"
	Result    string      `json:"result,omitempty"`
}

// CommandValidationResult contains the result of safety validation
type CommandValidationResult struct {
	Allowed              bool        `json:"allowed"`
	Safety               SafetyLevel `json:"safety_level"`
	Warning              string      `json:"warning,omitempty"`
	RequiresConfirmation bool        `json:"requires_confirmation"`
	AuditEntry           *AuditEntry `json:"audit_entry,omitempty"`
}

// NewSafetyManager creates a new safety manager
func NewSafetyManager(logger *logrus.Logger) *SafetyManager {
	return &SafetyManager{
		logger:               logger,
		auditLog:             NewAuditLogger(logger),
		confirmationRequired: true,  // Default: require confirmation for dangerous commands
		enabledCritical:      false, // Default: critical commands disabled
	}
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger *logrus.Logger) *AuditLogger {
	return &AuditLogger{
		logger: logger,
	}
}

// EnableCriticalCommands enables execution of critical safety commands
// This should only be called with explicit user authorization
func (sm *SafetyManager) EnableCriticalCommands(enable bool) {
	sm.enabledCritical = enable
	if enable {
		sm.logger.Warn("CRITICAL COMMANDS ENABLED - Physical security at risk!")
		sm.auditLog.Log("CRITICAL_ENABLED", "", SafetyCritical, "", "", "Critical command execution enabled")
	} else {
		sm.logger.Info("Critical commands disabled")
		sm.auditLog.Log("CRITICAL_DISABLED", "", SafetyCritical, "", "", "Critical command execution disabled")
	}
}

// SetConfirmationRequired sets whether dangerous commands require confirmation
func (sm *SafetyManager) SetConfirmationRequired(required bool) {
	sm.confirmationRequired = required
	sm.logger.Infof("Confirmation required for dangerous commands: %t", required)
}

// ValidateCommand checks if a command is safe to execute
func (sm *SafetyManager) ValidateCommand(cmdStr string, db *CommandDatabase, sourceIP, user string) (*CommandValidationResult, error) {
	result := &CommandValidationResult{
		Allowed: true,
		Safety:  SafetyLow,
	}

	// Look up command in database
	cmdInfo, exists := db.FindCommand(cmdStr)
	if !exists {
		// Unknown command - treat as medium risk
		result.Safety = SafetyMedium
		result.Warning = "Unknown command - executing with caution"
		sm.auditLog.Log(cmdStr, "UNKNOWN_COMMAND", SafetyMedium, sourceIP, user, "Unknown command executed")
		return result, nil
	}

	result.Safety = cmdInfo.Safety

	// Apply safety policies based on command safety level
	switch cmdInfo.Safety {
	case SafetyLow:
		// Safe commands - allow execution
		result.Allowed = true

	case SafetyMedium:
		// Medium risk - warn but allow
		result.Warning = fmt.Sprintf("CAUTION: %s", cmdInfo.Description)
		sm.auditLog.Log(cmdStr, "MEDIUM_RISK", SafetyMedium, sourceIP, user, "Medium risk command executed")

	case SafetyHigh:
		// High risk - require confirmation if enabled
		if sm.confirmationRequired {
			result.RequiresConfirmation = true
			result.Warning = fmt.Sprintf("HIGH RISK: %s - Confirmation required", cmdInfo.Description)
		} else {
			result.Warning = fmt.Sprintf("HIGH RISK: %s", cmdInfo.Description)
		}
		sm.auditLog.Log(cmdStr, "HIGH_RISK", SafetyHigh, sourceIP, user, "High risk command attempted")

	case SafetyCritical:
		// Critical commands - check if enabled
		if !sm.enabledCritical {
			result.Allowed = false
			result.Warning = fmt.Sprintf("CRITICAL COMMAND BLOCKED: %s - Enable critical commands first", cmdInfo.Description)
			sm.auditLog.Log(cmdStr, "CRITICAL_BLOCKED", SafetyCritical, sourceIP, user, "Critical command blocked")
			return result, fmt.Errorf("critical command blocked: %s", cmdStr)
		}

		// Critical commands always require confirmation
		result.RequiresConfirmation = true
		result.Warning = fmt.Sprintf("CRITICAL: %s - PHYSICAL SECURITY RISK", cmdInfo.Description)
		sm.auditLog.Log(cmdStr, "CRITICAL_ATTEMPTED", SafetyCritical, sourceIP, user, "Critical command attempted")
	}

	return result, nil
}

// LogCommandExecution logs successful command execution
func (sm *SafetyManager) LogCommandExecution(cmdStr string, safety SafetyLevel, sourceIP, user, result string) {
	action := "EXECUTED"
	if safety >= SafetyHigh {
		action = "CRITICAL_EXECUTED"
	}
	sm.auditLog.Log(cmdStr, action, safety, sourceIP, user, result)
}

// Log creates an audit log entry
func (al *AuditLogger) Log(command, action string, safety SafetyLevel, sourceIP, user, result string) {
	// Log to structured logger
	logEntry := al.logger.WithFields(logrus.Fields{
		"command":   command,
		"safety":    getSafetyLevelString(safety),
		"action":    action,
		"source_ip": sourceIP,
		"user":      user,
		"result":    result,
		"audit":     true,
	})

	switch safety {
	case SafetyCritical:
		logEntry.Error("CRITICAL COMMAND AUDIT")
	case SafetyHigh:
		logEntry.Warn("HIGH RISK COMMAND AUDIT")
	case SafetyMedium:
		logEntry.Info("MEDIUM RISK COMMAND AUDIT")
	default:
		logEntry.Debug("COMMAND AUDIT")
	}
}

// getSafetyLevelString converts SafetyLevel to string
func getSafetyLevelString(level SafetyLevel) string {
	switch level {
	case SafetyLow:
		return "LOW"
	case SafetyMedium:
		return "MEDIUM"
	case SafetyHigh:
		return "HIGH"
	case SafetyCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// IsDangerousCommand checks if a command is considered dangerous (High or Critical)
func (sm *SafetyManager) IsDangerousCommand(cmdStr string, db *CommandDatabase) bool {
	cmdInfo, exists := db.FindCommand(cmdStr)
	if !exists {
		// Unknown commands are treated as medium risk
		return true
	}
	return cmdInfo.Safety >= SafetyHigh
}

// GetCommandSafety returns the safety level of a command
func (sm *SafetyManager) GetCommandSafety(cmdStr string, db *CommandDatabase) SafetyLevel {
	cmdInfo, exists := db.FindCommand(cmdStr)
	if !exists {
		return SafetyMedium // Unknown commands default to medium risk
	}
	return cmdInfo.Safety
}

// IsDoorControlCommand checks if command controls physical doors
func IsDoorControlCommand(cmdStr string) bool {
	// Check for door control patterns
	doorPatterns := []string{
		"*8*19*20##", // Main door open
		"*8*19*11##", // Secondary door open
		"*8*20*20##", // Main door close
		"*8*20*11##", // Secondary door close
	}

	for _, pattern := range doorPatterns {
		if strings.EqualFold(cmdStr, pattern) {
			return true
		}
	}
	return false
}

// IsUserEnumerationCommand checks if command attempts user enumeration
func IsUserEnumerationCommand(cmdStr string) bool {
	return strings.Contains(cmdStr, "*#8**37*") || strings.Contains(cmdStr, "*13*35*")
}

// IsVideoManipulationCommand checks if command manipulates video system
func IsVideoManipulationCommand(cmdStr string) bool {
	return strings.Contains(cmdStr, "*7*77#") ||
		strings.Contains(cmdStr, "*7*300#") ||
		strings.Contains(cmdStr, "*7*220#")
}

// GetSecurityWarning returns appropriate security warning for command
func GetSecurityWarning(cmdStr string, safety SafetyLevel) string {
	switch {
	case IsDoorControlCommand(cmdStr):
		return "⚠️  PHYSICAL SECURITY: This command controls door locks"
	case IsUserEnumerationCommand(cmdStr):
		return "🔍 PRIVACY RISK: This command may expose user account information"
	case IsVideoManipulationCommand(cmdStr):
		return "📹 VIDEO SYSTEM: This command modifies video stream parameters"
	case safety == SafetyCritical:
		return "🚨 CRITICAL: This command poses significant security risks"
	case safety == SafetyHigh:
		return "⚡ HIGH RISK: This command affects sensitive system functions"
	case safety == SafetyMedium:
		return "⚠️  CAUTION: This command modifies system settings"
	default:
		return ""
	}
}
