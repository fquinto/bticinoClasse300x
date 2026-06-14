package events

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Event represents a generic event with metadata
type Event struct {
	Name      string      `json:"name"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	Source    string      `json:"source"`
}

// HandlerFunc defines the signature for event handlers
type HandlerFunc func(event *Event)

// PatternHandlerFunc defines the signature for pattern-based handlers
type PatternHandlerFunc func(eventName string, event *Event)

// Subscriber represents an event subscription
type Subscriber struct {
	ID             string
	Pattern        string
	Handler        HandlerFunc
	PatternHandler PatternHandlerFunc
	Active         bool
	IsPattern      bool
}

// EventBus interface defines the contract for event bus implementations
type EventBus interface {
	// Publish sends an event to all matching subscribers
	Publish(name string, data interface{})
	PublishWithSource(name string, data interface{}, source string)

	// Subscribe registers a handler for specific event names
	Subscribe(name string, handler HandlerFunc) string
	SubscribePattern(pattern string, handler PatternHandlerFunc) string

	// Unsubscribe removes a subscription by ID
	Unsubscribe(subscriptionID string) bool

	// GetStats returns event bus statistics
	GetStats() BusStats

	// Close shuts down the event bus
	Close()
}

// BusStats contains event bus statistics
type BusStats struct {
	TotalEvents       int64            `json:"total_events"`
	ActiveSubscribers int              `json:"active_subscribers"`
	EventCounts       map[string]int64 `json:"event_counts"`
	LastEventTime     time.Time        `json:"last_event_time"`
}

// DefaultEventBus is a simple, lightweight implementation of EventBus
type DefaultEventBus struct {
	subscribers  map[string][]*Subscriber
	patternSubs  map[string]*Subscriber
	mutex        sync.RWMutex
	stats        BusStats
	logger       *logrus.Logger
	eventChannel chan *Event
	workers      int
	shutdownChan chan struct{}
	wg           sync.WaitGroup
}

// NewEventBus creates a new default event bus with specified number of workers
func NewEventBus(workers int, logger *logrus.Logger) EventBus {
	if logger == nil {
		logger = logrus.New()
	}

	if workers <= 0 {
		workers = 2 // Default to 2 workers
	}

	bus := &DefaultEventBus{
		subscribers:  make(map[string][]*Subscriber),
		patternSubs:  make(map[string]*Subscriber),
		logger:       logger,
		eventChannel: make(chan *Event, 1000), // Buffer up to 1000 events
		workers:      workers,
		shutdownChan: make(chan struct{}),
		stats: BusStats{
			EventCounts: make(map[string]int64),
		},
	}

	// Start worker goroutines
	for i := 0; i < workers; i++ {
		bus.wg.Add(1)
		go bus.worker(i)
	}

	return bus
}

// worker processes events from the channel
func (b *DefaultEventBus) worker(id int) {
	defer b.wg.Done()
	b.logger.Debugf("EventBus worker %d started", id)

	for {
		select {
		case event := <-b.eventChannel:
			b.processEvent(event)
		case <-b.shutdownChan:
			b.logger.Debugf("EventBus worker %d shutting down", id)
			return
		}
	}
}

// processEvent handles a single event by calling all matching subscribers
func (b *DefaultEventBus) processEvent(event *Event) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	// Update statistics
	b.stats.TotalEvents++
	b.stats.EventCounts[event.Name]++
	b.stats.LastEventTime = event.Timestamp

	// Find direct subscribers
	if subscribers, exists := b.subscribers[event.Name]; exists {
		for _, sub := range subscribers {
			if sub.Active {
				go b.safeCallHandler(sub.Handler, event)
			}
		}
	}

	// Find pattern subscribers
	for _, sub := range b.patternSubs {
		if sub.Active && b.matchPattern(sub.Pattern, event.Name) {
			if sub.PatternHandler != nil {
				go b.safeCallPatternHandler(sub.PatternHandler, event.Name, event)
			}
		}
	}
}

// safeCallHandler calls an event handler with panic recovery
func (b *DefaultEventBus) safeCallHandler(handler HandlerFunc, event *Event) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Errorf("Event handler panicked for event '%s': %v", event.Name, r)
		}
	}()

	handler(event)
}

// safeCallPatternHandler calls a pattern handler with panic recovery
func (b *DefaultEventBus) safeCallPatternHandler(handler PatternHandlerFunc, eventName string, event *Event) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Errorf("Pattern handler panicked for event '%s': %v", eventName, r)
		}
	}()

	handler(eventName, event)
}

// Publish sends an event with default source
func (b *DefaultEventBus) Publish(name string, data interface{}) {
	b.PublishWithSource(name, data, "unknown")
}

// PublishWithSource sends an event with specified source
func (b *DefaultEventBus) PublishWithSource(name string, data interface{}, source string) {
	event := &Event{
		Name:      name,
		Data:      data,
		Timestamp: time.Now(),
		Source:    source,
	}

	select {
	case b.eventChannel <- event:
		b.logger.Debugf("Published event: %s from %s", name, source)
	default:
		b.logger.Warnf("Event channel full, dropping event: %s", name)
	}
}

// Subscribe registers a handler for specific event names
func (b *DefaultEventBus) Subscribe(name string, handler HandlerFunc) string {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	subscriptionID := fmt.Sprintf("%s_%d", name, time.Now().UnixNano())
	subscriber := &Subscriber{
		ID:        subscriptionID,
		Pattern:   name,
		Handler:   handler,
		Active:    true,
		IsPattern: false,
	}

	if _, exists := b.subscribers[name]; !exists {
		b.subscribers[name] = make([]*Subscriber, 0)
	}

	b.subscribers[name] = append(b.subscribers[name], subscriber)
	b.stats.ActiveSubscribers++

	b.logger.Debugf("Subscribed to event '%s' with ID: %s", name, subscriptionID)
	return subscriptionID
}

// SubscribePattern registers a handler for event name patterns
func (b *DefaultEventBus) SubscribePattern(pattern string, handler PatternHandlerFunc) string {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	subscriptionID := fmt.Sprintf("%s_%d", pattern, time.Now().UnixNano())

	subscriber := &Subscriber{
		ID:             subscriptionID,
		Pattern:        pattern,
		PatternHandler: handler,
		Active:         true,
		IsPattern:      true,
	}

	b.patternSubs[subscriptionID] = subscriber
	b.stats.ActiveSubscribers++

	b.logger.Debugf("Subscribed to pattern '%s' with ID: %s", pattern, subscriptionID)
	return subscriptionID
}

// Unsubscribe removes a subscription by ID
func (b *DefaultEventBus) Unsubscribe(subscriptionID string) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Check pattern subscriptions first
	if sub, exists := b.patternSubs[subscriptionID]; exists {
		sub.Active = false
		delete(b.patternSubs, subscriptionID)
		b.stats.ActiveSubscribers--
		b.logger.Debugf("Unsubscribed pattern subscription: %s", subscriptionID)
		return true
	}

	// Check direct subscriptions
	for eventName, subscribers := range b.subscribers {
		for i, sub := range subscribers {
			if sub.ID == subscriptionID {
				sub.Active = false
				// Remove from slice
				b.subscribers[eventName] = append(subscribers[:i], subscribers[i+1:]...)
				b.stats.ActiveSubscribers--
				b.logger.Debugf("Unsubscribed from event '%s': %s", eventName, subscriptionID)
				return true
			}
		}
	}

	return false
}

// matchPattern checks if an event name matches a pattern
// Supports simple wildcard matching with * and **
func (b *DefaultEventBus) matchPattern(pattern, eventName string) bool {
	// Simple cases
	if pattern == "*" || pattern == "**" {
		return true
	}
	if pattern == eventName {
		return true
	}

	// Wildcard matching
	if strings.Contains(pattern, "*") {
		return b.wildcardMatch(pattern, eventName)
	}

	return false
}

// wildcardMatch performs wildcard pattern matching
func (b *DefaultEventBus) wildcardMatch(pattern, str string) bool {
	// Simple implementation: support * as wildcard
	if pattern == "*" {
		return true
	}

	// If pattern ends with *, check if string starts with the prefix
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(str, prefix)
	}

	// If pattern starts with *, check if string ends with the suffix
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(str, suffix)
	}

	// Exact match if no wildcards
	return pattern == str
}

// GetStats returns current event bus statistics
func (b *DefaultEventBus) GetStats() BusStats {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	// Create a copy to avoid race conditions
	statsCopy := BusStats{
		TotalEvents:       b.stats.TotalEvents,
		ActiveSubscribers: b.stats.ActiveSubscribers,
		EventCounts:       make(map[string]int64),
		LastEventTime:     b.stats.LastEventTime,
	}

	for k, v := range b.stats.EventCounts {
		statsCopy.EventCounts[k] = v
	}

	return statsCopy
}

// Close shuts down the event bus
func (b *DefaultEventBus) Close() {
	b.logger.Info("Shutting down EventBus")

	close(b.shutdownChan)
	b.wg.Wait()
	close(b.eventChannel)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Deactivate all subscribers
	for _, subscribers := range b.subscribers {
		for _, sub := range subscribers {
			sub.Active = false
		}
	}

	for _, sub := range b.patternSubs {
		sub.Active = false
	}

	b.logger.Info("EventBus shut down complete")
}
