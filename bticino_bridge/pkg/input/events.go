package input

import (
	"encoding/json"
	"time"
)

// EventType represents the type of input event
type EventType string

const (
	EventTypeButton EventType = "button"
	EventTypeTouch  EventType = "touch"
	EventTypeScreen EventType = "screen"
	EventTypeGPIO   EventType = "gpio"
	EventTypeSystem EventType = "system"
)

// InputEvent is a generic interface for all input events
type InputEvent interface {
	GetType() EventType
	GetTimestamp() time.Time
	GetDevice() string
	String() string
}

// BaseEvent provides common functionality for all events
type BaseEvent struct {
	Type      EventType `json:"type"`
	Device    string    `json:"device"`
	Timestamp time.Time `json:"timestamp"`
}

func (be BaseEvent) GetType() EventType      { return be.Type }
func (be BaseEvent) GetTimestamp() time.Time { return be.Timestamp }
func (be BaseEvent) GetDevice() string       { return be.Device }

// ButtonPressEvent represents button press/release with OpenWebNet command mapping
type ButtonPressEvent struct {
	BaseEvent
	Key           int    `json:"key"`
	KeyName       string `json:"key_name"`
	Action        string `json:"action"`
	Value         int32  `json:"value"`
	OpenWebNetCmd string `json:"openwebnet_cmd,omitempty"` // Mapped OpenWebNet command
}

func (bpe ButtonPressEvent) String() string {
	data, _ := json.Marshal(bpe)
	return string(data)
}

// TouchScreenEvent represents touchscreen interaction
type TouchScreenEvent struct {
	BaseEvent
	X        int32  `json:"x"`
	Y        int32  `json:"y"`
	Pressure int32  `json:"pressure"`
	Action   string `json:"action"`
}

func (tse TouchScreenEvent) String() string {
	data, _ := json.Marshal(tse)
	return string(data)
}

// ScreenStateEvent represents screen on/off state changes
type ScreenStateEvent struct {
	BaseEvent
	GPIO     int  `json:"gpio"`
	State    bool `json:"state"`    // true = on, false = off
	Previous bool `json:"previous"` // previous state
}

func (sse ScreenStateEvent) String() string {
	data, _ := json.Marshal(sse)
	return string(data)
}

// SystemEvent represents system-level events (startup, shutdown, etc.)
type SystemEvent struct {
	BaseEvent
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

func (se SystemEvent) String() string {
	data, _ := json.Marshal(se)
	return string(data)
}

// EventHandler is a function type for handling input events
type EventHandler func(InputEvent)

// EventBus manages event distribution to multiple handlers
type EventBus struct {
	handlers map[EventType][]EventHandler
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[EventType][]EventHandler),
	}
}

// Subscribe adds an event handler for a specific event type
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// Publish sends an event to all registered handlers
func (eb *EventBus) Publish(event InputEvent) {
	if handlers, exists := eb.handlers[event.GetType()]; exists {
		for _, handler := range handlers {
			go handler(event) // Execute handlers asynchronously
		}
	}

	// Also send to wildcard handlers (if any)
	if handlers, exists := eb.handlers["*"]; exists {
		for _, handler := range handlers {
			go handler(event)
		}
	}
}

// Unsubscribe removes all handlers for an event type
func (eb *EventBus) Unsubscribe(eventType EventType) {
	delete(eb.handlers, eventType)
}

// GetSubscriberCount returns the number of handlers for an event type
func (eb *EventBus) GetSubscriberCount(eventType EventType) int {
	return len(eb.handlers[eventType])
}

// ButtonToOpenWebNetMapper maps physical buttons to OpenWebNet commands
type ButtonToOpenWebNetMapper struct {
	buttonMappings map[int]string // key code -> OpenWebNet command
}

// NewButtonToOpenWebNetMapper creates a new button mapper
func NewButtonToOpenWebNetMapper() *ButtonToOpenWebNetMapper {
	mapper := &ButtonToOpenWebNetMapper{
		buttonMappings: make(map[int]string),
	}

	// Default BTicino button mappings based on device analysis
	mapper.SetDefaultBTicinoMappings()
	return mapper
}

// SetDefaultBTicinoMappings configures default button-to-command mappings
func (mapper *ButtonToOpenWebNetMapper) SetDefaultBTicinoMappings() {
	// Based on BTicino Classe 300X button layout and discoveries
	mapper.buttonMappings[KEY_1] = "*8*19*20##"      // Button 1: Open main door
	mapper.buttonMappings[KEY_2] = "*#130**1*2##"    // Button 2: System status
	mapper.buttonMappings[KEY_3] = "*#8**35*1*0*0##" // Button 3: Audio channel 1 status
	mapper.buttonMappings[KEY_4] = "*#1013**1##"     // Button 4: Door status query
}

// MapButtonToCommand maps a button press to an OpenWebNet command
func (mapper *ButtonToOpenWebNetMapper) MapButtonToCommand(keyCode int) (string, bool) {
	cmd, exists := mapper.buttonMappings[keyCode]
	return cmd, exists
}

// SetButtonMapping sets a custom mapping for a button
func (mapper *ButtonToOpenWebNetMapper) SetButtonMapping(keyCode int, openWebNetCmd string) {
	mapper.buttonMappings[keyCode] = openWebNetCmd
}

// GetAllMappings returns all current button mappings
func (mapper *ButtonToOpenWebNetMapper) GetAllMappings() map[int]string {
	mappings := make(map[int]string)
	for key, cmd := range mapper.buttonMappings {
		mappings[key] = cmd
	}
	return mappings
}

// ClearMapping removes a button mapping
func (mapper *ButtonToOpenWebNetMapper) ClearMapping(keyCode int) {
	delete(mapper.buttonMappings, keyCode)
}

// CreateButtonEvent creates a ButtonPressEvent with OpenWebNet command mapping
func CreateButtonEvent(device string, keyCode int, action string, value int32) *ButtonPressEvent {
	mapper := NewButtonToOpenWebNetMapper()
	cmd, _ := mapper.MapButtonToCommand(keyCode)

	return &ButtonPressEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeButton,
			Device:    device,
			Timestamp: time.Now(),
		},
		Key:           keyCode,
		KeyName:       getKeyName(keyCode),
		Action:        action,
		Value:         value,
		OpenWebNetCmd: cmd,
	}
}

// CreateTouchEvent creates a TouchScreenEvent
func CreateTouchEvent(device string, x, y, pressure int32, action string) *TouchScreenEvent {
	return &TouchScreenEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeTouch,
			Device:    device,
			Timestamp: time.Now(),
		},
		X:        x,
		Y:        y,
		Pressure: pressure,
		Action:   action,
	}
}

// CreateScreenEvent creates a ScreenStateEvent
func CreateScreenEvent(device string, gpio int, state, previous bool) *ScreenStateEvent {
	return &ScreenStateEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeScreen,
			Device:    device,
			Timestamp: time.Now(),
		},
		GPIO:     gpio,
		State:    state,
		Previous: previous,
	}
}

// CreateSystemEvent creates a SystemEvent
func CreateSystemEvent(device, message string, data map[string]interface{}) *SystemEvent {
	return &SystemEvent{
		BaseEvent: BaseEvent{
			Type:      EventTypeSystem,
			Device:    device,
			Timestamp: time.Now(),
		},
		Message: message,
		Data:    data,
	}
}
