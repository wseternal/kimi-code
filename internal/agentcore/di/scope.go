package di

import (
	"errors"
	"fmt"
	"sync"
)

// LifecycleScope represents the lifecycle scope of a service.
type LifecycleScope int

const (
	ScopeApp     LifecycleScope = 0
	ScopeSession LifecycleScope = 1
	ScopeAgent   LifecycleScope = 2
)

// Scope is a DI container that holds services organized by lifecycle scope.
type Scope struct {
	id       string
	kind     LifecycleScope
	parent   *Scope
	children map[string]*Scope
	services map[string]any
	mu       sync.RWMutex
	disposed bool
}

// NewAppScope creates a new app-level scope.
func NewAppScope(id string) *Scope {
	return &Scope{
		id:       id,
		kind:     ScopeApp,
		children: make(map[string]*Scope),
		services: make(map[string]any),
	}
}

// ID returns the scope identifier.
func (s *Scope) ID() string { return s.id }

// Kind returns the lifecycle scope kind.
func (s *Scope) Kind() LifecycleScope { return s.kind }

// Register adds a service to this scope.
func (s *Scope) Register(name string, service any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services[name] = service
}

// Resolve retrieves a service by name, searching up the scope tree.
func (s *Scope) Resolve(name string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if svc, ok := s.services[name]; ok {
		return svc, nil
	}
	if s.parent != nil {
		return s.parent.Resolve(name)
	}
	return nil, fmt.Errorf("service %q not found in scope %q", name, s.id)
}

// MustResolve retrieves a service, panicking if not found.
func (s *Scope) MustResolve(name string) any {
	svc, err := s.Resolve(name)
	if err != nil {
		panic(err)
	}
	return svc
}

// CreateChild creates a child scope with the given kind and id.
func (s *Scope) CreateChild(kind LifecycleScope, id string) (*Scope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.disposed {
		return nil, errors.New("scope has been disposed")
	}
	if kind <= s.kind {
		return nil, fmt.Errorf("child scope kind %d must be greater than parent kind %d", kind, s.kind)
	}
	if _, exists := s.children[id]; exists {
		return nil, fmt.Errorf("child scope %q already exists", id)
	}

	child := &Scope{
		id:       id,
		kind:     kind,
		parent:   s,
		children: make(map[string]*Scope),
		services: make(map[string]any),
	}
	s.children[id] = child
	return child, nil
}

// GetChild returns a child scope by id.
func (s *Scope) GetChild(id string) (*Scope, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	child, ok := s.children[id]
	return child, ok
}

// Dispose disposes this scope and all its children.
func (s *Scope) Dispose() {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return
	}
	s.disposed = true

	// Copy children to avoid deadlock
	kids := make([]*Scope, 0, len(s.children))
	for _, child := range s.children {
		kids = append(kids, child)
	}
	s.children = make(map[string]*Scope)
	parent := s.parent
	s.mu.Unlock()

	// Dispose children
	for _, child := range kids {
		child.Dispose()
	}

	// Remove from parent
	if parent != nil {
		parent.mu.Lock()
		delete(parent.children, s.id)
		parent.mu.Unlock()
	}
}

// ServicesAccessor provides read-only access to scope services.
type ServicesAccessor struct {
	scope *Scope
}

// NewServicesAccessor creates a services accessor for the given scope.
func NewServicesAccessor(scope *Scope) *ServicesAccessor {
	return &ServicesAccessor{scope: scope}
}

// Get retrieves a service by name.
func (a *ServicesAccessor) Get(name string) (any, error) {
	return a.scope.Resolve(name)
}

// MustGet retrieves a service, panicking if not found.
func (a *ServicesAccessor) MustGet(name string) any {
	return a.scope.MustResolve(name)
}
