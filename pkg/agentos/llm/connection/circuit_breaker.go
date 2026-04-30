// Package connection provides connection management for LLM providers
package connection

import (
	"sync"
	"time"
)

// State represents the state of a circuit breaker
type State int

const (
	// StateClosed - normal operation, requests pass through
	StateClosed State = iota
	// StateOpen - failing fast, requests rejected
	StateOpen
	// StateHalfOpen - testing if service recovered
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	failures       int
	successes      int
	lastFailure    time.Time
	lastSuccess    time.Time
	state          State
	threshold      int           // failures before opening
	resetTimeout   time.Duration // time before attempting reset
	halfOpenMax    int           // max requests in half-open state
	mu             sync.RWMutex
}

// Config for circuit breaker
type CircuitBreakerConfig struct {
	Threshold    int
	ResetTimeout time.Duration
	HalfOpenMax  int
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Threshold:    5,
		ResetTimeout: 30 * time.Second,
		HalfOpenMax:  3,
	}
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    config.Threshold,
		resetTimeout: config.ResetTimeout,
		halfOpenMax:  config.HalfOpenMax,
		state:        StateClosed,
	}
}

// State returns the current state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	cb.transitionIfNeeded()
	return cb.state
}

// Allow checks if a request should be allowed
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transitionIfNeeded()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		return false
	case StateHalfOpen:
		if cb.successes+cb.failures >= cb.halfOpenMax {
			return false
		}
		return true
	}

	return false
}

// RecordSuccess records a successful request
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastSuccess = time.Now()

	switch cb.state {
	case StateClosed:
		cb.successes++
		cb.failures = 0

	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.halfOpenMax {
			// Transition to closed
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
		}
	}
}

// RecordFailure records a failed request
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailure = time.Now()
	cb.failures++

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.threshold {
			cb.state = StateOpen
		}

	case StateHalfOpen:
		// Immediately open on failure in half-open
		cb.state = StateOpen
		cb.failures = 1
		cb.successes = 0
	}
}

// transitionIfNeeded checks if we should transition states
func (cb *CircuitBreaker) transitionIfNeeded() {
	if cb.state != StateOpen {
		return
	}

	// Check if enough time has passed to try half-open
	if time.Since(cb.lastFailure) >= cb.resetTimeout {
		cb.state = StateHalfOpen
		cb.failures = 0
		cb.successes = 0
	}
}

// Stats returns circuit breaker statistics
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:       cb.state.String(),
		Failures:    cb.failures,
		Successes:   cb.successes,
		LastFailure: cb.lastFailure,
		LastSuccess: cb.lastSuccess,
	}
}

// Reset manually resets the circuit breaker
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
}

// CircuitBreakerStats contains circuit breaker statistics
type CircuitBreakerStats struct {
	State       string
	Failures    int
	Successes   int
	LastFailure time.Time
	LastSuccess time.Time
}

// CircuitBreakerRegistry manages circuit breakers for multiple providers
type CircuitBreakerRegistry struct {
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
	mu       sync.RWMutex
}

// NewCircuitBreakerRegistry creates a new registry
func NewCircuitBreakerRegistry(config CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// Get returns a circuit breaker for a provider, creating if needed
func (r *CircuitBreakerRegistry) Get(provider string) *CircuitBreaker {
	r.mu.RLock()
	cb, exists := r.breakers[provider]
	r.mu.RUnlock()

	if exists {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	cb, exists = r.breakers[provider]
	if exists {
		return cb
	}

	cb = NewCircuitBreaker(r.config)
	r.breakers[provider] = cb
	return cb
}

// GetStats returns stats for all circuit breakers
func (r *CircuitBreakerRegistry) GetStats() map[string]CircuitBreakerStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats)
	for name, cb := range r.breakers {
		stats[name] = cb.Stats()
	}

	return stats
}

// Reset resets a specific circuit breaker
func (r *CircuitBreakerRegistry) Reset(provider string) {
	r.mu.RLock()
	cb, exists := r.breakers[provider]
	r.mu.RUnlock()

	if exists {
		cb.Reset()
	}
}

// ResetAll resets all circuit breakers
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	breakers := make([]*CircuitBreaker, 0, len(r.breakers))
	for _, cb := range r.breakers {
		breakers = append(breakers, cb)
	}
	r.mu.RUnlock()

	for _, cb := range breakers {
		cb.Reset()
	}
}
