// Package bticino_commands provides enhanced OpenWebNet command handling
// based on real device analysis from ha_config repository
package bticino_commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/bticino"
	"bticino_bridge/pkg/openwebnet"
)

// BTicinoCommandHandler provides enhanced command handling with real device knowledge
type BTicinoCommandHandler struct {
	client *openwebnet.Client
	logger *logrus.Logger
}

// NewBTicinoCommandHandler creates a new command handler with OpenWebNet client
func NewBTicinoCommandHandler(client *openwebnet.Client, logger *logrus.Logger) *BTicinoCommandHandler {
	return &BTicinoCommandHandler{
		client: client,
		logger: logger,
	}
}

// AnsweringMachine Commands

// EnableAnsweringMachine enables the answering machine using real BTicino commands
func (bch *BTicinoCommandHandler) EnableAnsweringMachine() error {
	bch.logger.Info("Enabling answering machine with BTicino sequence...")

	// Step 1: Turn display on (required for answering machine activation)
	if err := bch.executeCommandWithDelay(bticino.CmdDisplayOn, "Turn display ON"); err != nil {
		return fmt.Errorf("failed to turn display on: %v", err)
	}

	// Step 2: Enable voicemail
	if err := bch.executeCommandWithDelay(bticino.CmdVoicemailOn, "Enable voicemail"); err != nil {
		return fmt.Errorf("failed to enable voicemail: %v", err)
	}

	// Step 3: Enable via app protocol (critical timing)
	if err := bch.executeCommandWithDelay(bticino.CmdVoicemailOnApp, "Enable voicemail via app"); err != nil {
		return fmt.Errorf("failed to enable voicemail via app: %v", err)
	}

	// Step 4: Final confirmation command
	if err := bch.executeCommandWithDelay(bticino.CmdVoicemailOn+"*", "Confirm voicemail ON"); err != nil {
		bch.logger.Warn("Final confirmation failed, but voicemail should be enabled")
	}

	bch.logger.Info("✅ Answering machine enabled successfully")
	return nil
}

// DisableAnsweringMachine disables the answering machine using real BTicino commands
func (bch *BTicinoCommandHandler) DisableAnsweringMachine() error {
	bch.logger.Info("Disabling answering machine with BTicino sequence...")

	// Step 1: Turn display on (required for answering machine deactivation)
	if err := bch.executeCommandWithDelay(bticino.CmdDisplayOn, "Turn display ON"); err != nil {
		return fmt.Errorf("failed to turn display on: %v", err)
	}

	// Step 2: Disable voicemail
	if err := bch.executeCommandWithDelay(bticino.CmdVoicemailOff, "Disable voicemail"); err != nil {
		return fmt.Errorf("failed to disable voicemail: %v", err)
	}

	// Step 3: Disable via app protocol (critical timing)
	if err := bch.executeCommandWithDelay(bticino.CmdVoicemailOffApp, "Disable voicemail via app"); err != nil {
		return fmt.Errorf("failed to disable voicemail via app: %v", err)
	}

	// Step 4: Final confirmation command
	if err := bch.executeCommandWithDelay(bticino.CmdVoicemailOff+"*", "Confirm voicemail OFF"); err != nil {
		bch.logger.Warn("Final confirmation failed, but voicemail should be disabled")
	}

	bch.logger.Info("✅ Answering machine disabled successfully")
	return nil
}

// GetAnsweringMachineStatus queries the current answering machine status
func (bch *BTicinoCommandHandler) GetAnsweringMachineStatus() (bool, error) {
	response, err := bch.client.SendCommand(bticino.CmdVoicemailStatus)
	if err != nil {
		return false, fmt.Errorf("failed to query answering machine status: %v", err)
	}

	// Parse response to determine if enabled
	responseStr := response.Raw
	enabled := strings.Contains(responseStr, "1") || strings.Contains(strings.ToLower(responseStr), "on")
	bch.logger.Debugf("Answering machine status query: %s -> enabled: %v", responseStr, enabled)

	return enabled, nil
}

// Doorbell Sound Commands

// EnableDoorbellSound enables the doorbell sound
func (bch *BTicinoCommandHandler) EnableDoorbellSound() error {
	response, err := bch.client.SendCommand(bticino.CmdBellOn)
	if err != nil {
		return fmt.Errorf("failed to enable doorbell sound: %v", err)
	}

	if !bch.isACK(response.Raw) {
		return fmt.Errorf("doorbell sound enable was not acknowledged: %s", response.Raw)
	}

	bch.logger.Info("✅ Doorbell sound enabled")
	return nil
}

// DisableDoorbellSound disables the doorbell sound
func (bch *BTicinoCommandHandler) DisableDoorbellSound() error {
	response, err := bch.client.SendCommand(bticino.CmdBellOff)
	if err != nil {
		return fmt.Errorf("failed to disable doorbell sound: %v", err)
	}

	if !bch.isACK(response.Raw) {
		return fmt.Errorf("doorbell sound disable was not acknowledged: %s", response.Raw)
	}

	bch.logger.Info("✅ Doorbell sound disabled")
	return nil
}

// Door Lock Commands

// UnlockDoor performs door unlock sequence with proper timing
func (bch *BTicinoCommandHandler) UnlockDoor() error {
	bch.logger.Info("Unlocking door with proper BTicino sequence...")

	// Step 1: Door open button press
	if err := bch.executeCommandWithDelay(bticino.CmdDoorOpenPress, "Door open press"); err != nil {
		return fmt.Errorf("failed to press door open: %v", err)
	}

	// Step 2: Wait 1 second (critical timing from ha_config analysis)
	time.Sleep(1 * time.Second)

	// Step 3: Door open button release
	if err := bch.executeCommandWithDelay(bticino.CmdDoorOpenRelease, "Door open release"); err != nil {
		return fmt.Errorf("failed to release door open: %v", err)
	}

	bch.logger.Info("✅ Door unlock sequence completed")
	return nil
}

// Display Commands

// TurnDisplayOn turns the BTicino display on
func (bch *BTicinoCommandHandler) TurnDisplayOn() error {
	response, err := bch.client.SendCommand(bticino.CmdDisplayOn)
	if err != nil {
		return fmt.Errorf("failed to turn display on: %v", err)
	}

	if !bch.isACK(response.Raw) {
		return fmt.Errorf("display on was not acknowledged: %s", response.Raw)
	}

	bch.logger.Info("✅ Display turned on")
	return nil
}

// TurnDisplayOff turns the BTicino display off
func (bch *BTicinoCommandHandler) TurnDisplayOff() error {
	response, err := bch.client.SendCommand(bticino.CmdDisplayOff)
	if err != nil {
		return fmt.Errorf("failed to turn display off: %v", err)
	}

	if !bch.isACK(response.Raw) {
		return fmt.Errorf("display off was not acknowledged: %s", response.Raw)
	}

	bch.logger.Info("✅ Display turned off")
	return nil
}

// Light Commands

// TurnLightOn activates light with proper button sequence
func (bch *BTicinoCommandHandler) TurnLightOn() error {
	bch.logger.Info("Turning light on with BTicino sequence...")

	// Step 1: Light button press
	if err := bch.executeCommandWithDelay(bticino.CmdLightOnPress, "Light button press"); err != nil {
		return fmt.Errorf("failed to press light button: %v", err)
	}

	// Step 2: Small delay for button press simulation
	time.Sleep(200 * time.Millisecond)

	// Step 3: Light button release
	if err := bch.executeCommandWithDelay(bticino.CmdLightOnRelease, "Light button release"); err != nil {
		return fmt.Errorf("failed to release light button: %v", err)
	}

	bch.logger.Info("✅ Light activation sequence completed")
	return nil
}

// System Status Commands

// GetSystemStatus requests general system status
func (bch *BTicinoCommandHandler) GetSystemStatus() (string, error) {
	response, err := bch.client.SendCommand(bticino.CmdStatusRequest)
	if err != nil {
		return "", fmt.Errorf("failed to get system status: %v", err)
	}

	bch.logger.Debugf("System status response: %s", response.Raw)
	return response.Raw, nil
}

// Command Analysis and Parsing

// ParseCommand analyzes a received OpenWebNet command and returns its meaning
func (bch *BTicinoCommandHandler) ParseCommand(command string) string {
	if description, exists := bticino.CommandResponses[command]; exists {
		return description
	}

	// Additional pattern matching for complex commands
	command = strings.TrimSpace(command)

	switch {
	case strings.HasPrefix(command, "*7*73#1#"):
		if strings.Contains(command, "100") {
			return "display ON"
		} else if strings.Contains(command, "10") {
			return "display OFF"
		}
	case strings.HasPrefix(command, "*#8**"):
		if strings.Contains(command, "*33*1") {
			return "bell ON"
		} else if strings.Contains(command, "*33*0") {
			return "bell OFF"
		} else if strings.Contains(command, "*40*1*") {
			return "voicemail ON using App"
		} else if strings.Contains(command, "*40*0*") {
			return "voicemail OFF using App"
		}
	case strings.HasPrefix(command, "*8*"):
		switch {
		case strings.Contains(command, "91"):
			return "voicemail ON"
		case strings.Contains(command, "92"):
			return "voicemail OFF"
		case strings.Contains(command, "19*20"):
			return "door open button press"
		case strings.Contains(command, "20*20"):
			return "door open button release"
		case strings.Contains(command, "21*16"):
			return "light ON button press"
		case strings.Contains(command, "22*16"):
			return "light ON button release"
		case strings.Contains(command, "1#1#4#21*16"):
			return "doorbell activation"
		}
	}

	return fmt.Sprintf("unknown command: %s", command)
}

// Helper Methods

// executeCommandWithDelay executes a command with the critical 310ms delay
func (bch *BTicinoCommandHandler) executeCommandWithDelay(command, description string) error {
	bch.logger.Debugf("Executing %s: %s", description, command)

	response, err := bch.client.SendCommand(command)
	if err != nil {
		return fmt.Errorf("command failed: %v", err)
	}

	if !bch.isACK(response.Raw) {
		bch.logger.Warnf("Command %s not acknowledged: %s", description, response.Raw)
	}

	// Critical delay - DO NOT CHANGE (from ha_config analysis)
	time.Sleep(bticino.CommandRetryDelay)

	return nil
}

// isACK checks if response is an acknowledgment
func (bch *BTicinoCommandHandler) isACK(response string) bool {
	return strings.TrimSpace(response) == bticino.CmdACK
}

// isNACK checks if response is a negative acknowledgment
func (bch *BTicinoCommandHandler) isNACK(response string) bool {
	return strings.TrimSpace(response) == bticino.CmdNACK
}

// CommandBuilder provides fluent interface for building complex command sequences

// CommandBuilder helps build complex command sequences
type CommandBuilder struct {
	commands []commandStep
	handler  *BTicinoCommandHandler
}

type commandStep struct {
	command     string
	description string
	delay       time.Duration
}

// NewCommandBuilder creates a new command builder
func (bch *BTicinoCommandHandler) NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{
		commands: make([]commandStep, 0),
		handler:  bch,
	}
}

// AddCommand adds a command to the sequence
func (cb *CommandBuilder) AddCommand(command, description string) *CommandBuilder {
	cb.commands = append(cb.commands, commandStep{
		command:     command,
		description: description,
		delay:       bticino.CommandRetryDelay,
	})
	return cb
}

// AddCommandWithDelay adds a command with custom delay
func (cb *CommandBuilder) AddCommandWithDelay(command, description string, delay time.Duration) *CommandBuilder {
	cb.commands = append(cb.commands, commandStep{
		command:     command,
		description: description,
		delay:       delay,
	})
	return cb
}

// Execute runs all commands in sequence
func (cb *CommandBuilder) Execute() error {
	cb.handler.logger.Infof("Executing command sequence with %d steps", len(cb.commands))

	for i, step := range cb.commands {
		cb.handler.logger.Debugf("Step %d/%d: %s", i+1, len(cb.commands), step.description)

		if err := cb.handler.executeCommandWithDelay(step.command, step.description); err != nil {
			return fmt.Errorf("step %d failed (%s): %v", i+1, step.description, err)
		}

		// Use custom delay if specified
		if step.delay != bticino.CommandRetryDelay {
			time.Sleep(step.delay - bticino.CommandRetryDelay) // Subtract already applied delay
		}
	}

	cb.handler.logger.Info("✅ Command sequence completed successfully")
	return nil
}
