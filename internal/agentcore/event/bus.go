package event

import (
	"sync"
)

// Handler is a function that handles an event.
type Handler[T any] func(T)

// Bus is a typed event bus that supports publish/subscribe.
type Bus[T any] struct {
	handlers []Handler[T]
	mu       sync.RWMutex
}

// NewBus creates a new event bus.
func NewBus[T any]() *Bus[T] {
	return &Bus[T]{}
}

// Subscribe adds a handler to the bus.
func (b *Bus[T]) Subscribe(h Handler[T]) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Publish sends an event to all handlers.
func (b *Bus[T]) Publish(event T) {
	b.mu.RLock()
	handlers := make([]Handler[T], len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
}

// Unsubscribe removes all handlers.
func (b *Bus[T]) Unsubscribe() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = nil
}

// MultiBus manages multiple named event buses.
type MultiBus struct {
	buses map[string]any
	mu    sync.RWMutex
}

// NewMultiBus creates a new multi-bus.
func NewMultiBus() *MultiBus {
	return &MultiBus{buses: make(map[string]any)}
}

// Get returns a bus by name, or nil if not found.
func (m *MultiBus) Get(name string) any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.buses[name]
}

// Set stores a bus by name.
func (m *MultiBus) Set(name string, bus any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buses[name] = bus
}
