package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"bticino_bridge/pkg/events"
)

// OpenWebNetHandler handles OpenWebNet commands received via multicast
// Based on slyoldfox's openwebnet-handler.js implementation
type OpenWebNetHandler struct {
	eventBus events.EventBus
	logger   *logrus.Logger
}

// OpenWebNetEvent represents a parsed OpenWebNet event
type OpenWebNetEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Command   string    `json:"command"`
	EventType string    `json:"event_type"`
	Who       string    `json:"who,omitempty"`       // System identifier
	What      string    `json:"what,omitempty"`      // Command/action
	Where     string    `json:"where,omitempty"`     // Location/address
	When      string    `json:"when,omitempty"`      // Time parameter
	Level     string    `json:"level,omitempty"`     // Level parameter
	Interface string    `json:"interface,omitempty"` // Interface parameter
	Raw       string    `json:"raw"`
}

// Command patterns based on slyoldfox's analysis of BTicino Class 300X
var (
	// Door lock/unlock events: *8*19*0## (unlock) and *8*20*0## (lock)
	doorLockPattern = regexp.MustCompile(`\*8\*(\d+)\*(\d+)##`)

	// Doorbell press: *8*1#1#4# (based on slyoldfox observations)
	doorbellPattern = regexp.MustCompile(`\*8\*1#1#4#`)

	// Floor/landing doorbell (timbre de rellano): *7*59#...
	floorRingPattern = regexp.MustCompile(`\*7\*59#`)

	// Video stream events: *7*300#127#0#0#1#5007#0*##
	videoStreamPattern = regexp.MustCompile(`\*7\*(\d+)#(.+)##`)

	// Mute/unmute events: *#8**33*0## (mute) and *#8**33*1## (unmute)
	mutePattern = regexp.MustCompile(`\*#8\*\*33\*([01])##`)

	// General OpenWebNet command pattern: *WHO*WHAT*WHERE## or *#WHO*WHERE*PARAMETER##
	openwebnetPattern = regexp.MustCompile(`\*(?:#?)(\d+)\*(?:(\d+)\*)?(.*)##`)
)

// NewOpenWebNetHandler creates a new OpenWebNet handler
func NewOpenWebNetHandler(eventBus events.EventBus, logger *logrus.Logger) *OpenWebNetHandler {
	if logger == nil {
		logger = logrus.New()
	}

	return &OpenWebNetHandler{
		eventBus: eventBus,
		logger:   logger,
	}
}

// GetSystemName returns the system name this handler manages
func (h *OpenWebNetHandler) GetSystemName() string {
	return "OPEN"
}

// Handle processes OpenWebNet messages from multicast
func (h *OpenWebNetHandler) Handle(listener interface{}, system string, message string) bool {
	if system != "OPEN" {
		return false
	}

	// Parse the OpenWebNet command
	event := h.parseOpenWebNetCommand(message)
	if event == nil {
		h.logger.Debugf("Failed to parse OpenWebNet command: %s", message)
		return false
	}

	h.logger.Debugf("Parsed OpenWebNet event: %s -> %s", event.Command, event.EventType)

	// Publish specific event based on type
	switch event.EventType {
	case "door.unlock":
		h.eventBus.PublishWithSource("door.unlocked", event, "openwebnet")
		h.eventBus.PublishWithSource("door.state.changed", map[string]interface{}{
			"state":     "unlocked",
			"timestamp": event.Timestamp,
			"source":    "openwebnet",
		}, "openwebnet")

	case "door.lock":
		h.eventBus.PublishWithSource("door.locked", event, "openwebnet")
		h.eventBus.PublishWithSource("door.state.changed", map[string]interface{}{
			"state":     "locked",
			"timestamp": event.Timestamp,
			"source":    "openwebnet",
		}, "openwebnet")

	case "doorbell.pressed":
		h.eventBus.PublishWithSource("doorbell.pressed", event, "openwebnet")
		h.eventBus.PublishWithSource("doorbell.ring", map[string]interface{}{
			"timestamp": event.Timestamp,
			"source":    "openwebnet",
		}, "openwebnet")

	case "doorbell.floor":
		// Timbre de rellano (boton del descansillo), distinto de la placa de calle
		h.eventBus.PublishWithSource("doorbell.floor.pressed", event, "openwebnet")

	case "video.stream":
		h.eventBus.PublishWithSource("video.stream.event", event, "openwebnet")

	case "audio.mute":
		h.eventBus.PublishWithSource("audio.muted", event, "openwebnet")

	case "audio.unmute":
		h.eventBus.PublishWithSource("audio.unmuted", event, "openwebnet")

	default:
		// Publish generic OpenWebNet event
		h.eventBus.PublishWithSource("openwebnet.command", event, "openwebnet")
	}

	// Always publish raw OpenWebNet event for debugging/advanced usage
	h.eventBus.PublishWithSource("openwebnet.raw", event, "openwebnet")

	return true
}

// parseOpenWebNetCommand parses an OpenWebNet command and determines its type
func (h *OpenWebNetHandler) parseOpenWebNetCommand(command string) *OpenWebNetEvent {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	event := &OpenWebNetEvent{
		Timestamp: time.Now(),
		Command:   command,
		Raw:       command,
	}

	// Check for door lock/unlock events
	if matches := doorLockPattern.FindStringSubmatch(command); matches != nil {
		event.Who = "8" // Auxiliary system
		event.What = matches[1]
		event.Where = matches[2]

		switch matches[1] {
		case "19":
			event.EventType = "door.unlock"
		case "20":
			event.EventType = "door.lock"
		default:
			event.EventType = "door.unknown"
		}
		return event
	}

	// Check for doorbell press
	if doorbellPattern.MatchString(command) {
		event.EventType = "doorbell.pressed"
		event.Who = "8"
		event.What = "1"
		return event
	}

	// Check for floor/landing doorbell (antes que el patron generico *7*...)
	if floorRingPattern.MatchString(command) {
		event.EventType = "doorbell.floor"
		event.Who = "7"
		event.What = "59"
		return event
	}

	// Check for video stream events
	if matches := videoStreamPattern.FindStringSubmatch(command); matches != nil {
		event.Who = "7" // Video system
		event.What = matches[1]
		event.EventType = "video.stream"

		// Parse additional parameters
		params := strings.Split(matches[2], "#")
		if len(params) > 0 {
			event.Where = params[0]
		}
		return event
	}

	// Check for mute/unmute events
	if matches := mutePattern.FindStringSubmatch(command); matches != nil {
		event.Who = "8" // Auxiliary system
		event.What = "33"
		event.Where = matches[1]

		switch matches[1] {
		case "0":
			event.EventType = "audio.mute"
		case "1":
			event.EventType = "audio.unmute"
		}
		return event
	}

	// Try to parse as general OpenWebNet command
	if matches := openwebnetPattern.FindStringSubmatch(command); matches != nil {
		event.Who = matches[1]
		if len(matches) > 2 && matches[2] != "" {
			event.What = matches[2]
		}
		if len(matches) > 3 && matches[3] != "" {
			// Parse WHERE and additional parameters
			whereParts := strings.Split(matches[3], "*")
			if len(whereParts) > 0 {
				event.Where = whereParts[0]
			}
			if len(whereParts) > 1 {
				event.When = whereParts[1]
			}
			if len(whereParts) > 2 {
				event.Level = whereParts[2]
			}
			if len(whereParts) > 3 {
				event.Interface = whereParts[3]
			}
		}

		event.EventType = fmt.Sprintf("openwebnet.who%s", event.Who)
		return event
	}

	// Unknown command format
	event.EventType = "openwebnet.unknown"
	return event
}

// GetKnownCommands returns a map of known OpenWebNet command patterns
func (h *OpenWebNetHandler) GetKnownCommands() map[string]string {
	return map[string]string{
		"*8*19*0##":    "Door unlock",
		"*8*20*0##":    "Door lock",
		"*8*1#1#4#":    "Doorbell press",
		"*7*300#...##": "Video stream event",
		"*#8**33*0##":  "Audio mute",
		"*#8**33*1##":  "Audio unmute",
	}
}

// TimeLog logs a message with timestamp (compatible with slyoldfox interface)
func (h *OpenWebNetHandler) TimeLog(message string) {
	h.logger.Infof("= %s => %s", time.Now().Format("2006-01-02 15:04:05"), message)
}
