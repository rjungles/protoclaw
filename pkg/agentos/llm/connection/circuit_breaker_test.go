package connection

import (
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	if cb == nil {
		t.Fatal("NewCircuitBreaker() returned nil")
	}

	if cb.threshold != config.Threshold {
		t.Errorf("threshold = %d, want %d", cb.threshold, config.Threshold)
	}

	if cb.resetTimeout != config.ResetTimeout {
		t.Errorf("resetTimeout = %v, want %v", cb.resetTimeout, config.ResetTimeout)
	}

	if cb.state != StateClosed {
		t.Errorf("initial state = %v, want %v", cb.state, StateClosed)
	}
}

func TestCircuitBreaker_State(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:    3,
		ResetTimeout: 100 * time.Millisecond,
		HalfOpenMax:  1,
	}
	cb := NewCircuitBreaker(config)

	// Initial state should be closed
	if cb.State() != StateClosed {
		t.Errorf("initial state = %v, want %v", cb.State(), StateClosed)
	}

	// Record failures to open circuit
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("state after failures = %v, want %v", cb.State(), StateOpen)
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Should transition to half-open
	if cb.State() != StateHalfOpen {
		t.Errorf("state after timeout = %v, want %v", cb.State(), StateHalfOpen)
	}
}

func TestCircuitBreaker_Allow(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:    2,
		ResetTimeout: 100 * time.Millisecond,
		HalfOpenMax:  1,
	}
	cb := NewCircuitBreaker(config)

	// Closed state - should allow
	if !cb.Allow() {
		t.Error("Allow() should be true when closed")
	}

	// Open circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Open state - should not allow
	if cb.Allow() {
		t.Error("Allow() should be false when open")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Half-open - should allow limited requests
	if !cb.Allow() {
		t.Error("Allow() should be true in half-open")
	}

	// Should not allow more than halfOpenMax
	if cb.Allow() {
		t.Error("Allow() should be false when half-open max reached")
	}
}

func TestCircuitBreaker_RecordSuccess(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:    3,
		ResetTimeout: 100 * time.Millisecond,
		HalfOpenMax:  1,
	}
	cb := NewCircuitBreaker(config)

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	// Success should reset failure count
	if cb.failures != 0 {
		t.Errorf("failures = %d, want 0", cb.failures)
	}

	// Record more failures to open
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for half-open
	time.Sleep(150 * time.Millisecond)
	_ = cb.State() // Trigger transition

	// Success in half-open should close circuit
	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Errorf("state after success in half-open = %v, want %v", cb.State(), StateClosed)
	}
}

func TestCircuitBreaker_RecordFailure(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:    3,
		ResetTimeout: 100 * time.Millisecond,
		HalfOpenMax:  1,
	}
	cb := NewCircuitBreaker(config)

	// Record failures
	cb.RecordFailure()
	if cb.failures != 1 {
		t.Errorf("failures = %d, want 1", cb.failures)
	}

	cb.RecordFailure()
	cb.RecordFailure()

	// Should be open
	if cb.State() != StateOpen {
		t.Errorf("state = %v, want %v", cb.State(), StateOpen)
	}

	// Wait for half-open
	time.Sleep(150 * time.Millisecond)
	_ = cb.State()

	// Failure in half-open should open again
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("state after failure in half-open = %v, want %v", cb.State(), StateOpen)
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	// Record some activity
	cb.RecordSuccess()
	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()

	if stats.Successes != 1 {
		t.Errorf("Stats.Successes = %d, want 1", stats.Successes)
	}

	if stats.Failures != 2 {
		t.Errorf("Stats.Failures = %d, want 2", stats.Failures)
	}

	if stats.State != "closed" {
		t.Errorf("Stats.State = %q, want %q", stats.State, "closed")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:    2,
		ResetTimeout: 100 * time.Millisecond,
		HalfOpenMax:  1,
	}
	cb := NewCircuitBreaker(config)

	// Open circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatal("Circuit should be open")
	}

	// Reset
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("state after reset = %v, want %v", cb.State(), StateClosed)
	}

	if cb.failures != 0 {
		t.Errorf("failures after reset = %d, want 0", cb.failures)
	}

	if cb.successes != 0 {
		t.Errorf("successes after reset = %d, want 0", cb.successes)
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestNewCircuitBreakerRegistry(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	reg := NewCircuitBreakerRegistry(config)

	if reg == nil {
		t.Fatal("NewCircuitBreakerRegistry() returned nil")
	}

	if len(reg.breakers) != 0 {
		t.Errorf("initial breakers count = %d, want 0", len(reg.breakers))
	}
}

func TestCircuitBreakerRegistry_Get(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	reg := NewCircuitBreakerRegistry(config)

	// Get new circuit breaker
	cb1 := reg.Get("provider-a")
	if cb1 == nil {
		t.Fatal("Get() returned nil")
	}

	// Get same circuit breaker
	cb2 := reg.Get("provider-a")
	if cb1 != cb2 {
		t.Error("Get() should return same circuit breaker")
	}

	// Get different circuit breaker
	cb3 := reg.Get("provider-b")
	if cb1 == cb3 {
		t.Error("Get() should return different circuit breaker for different provider")
	}
}

func TestCircuitBreakerRegistry_GetStats(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	reg := NewCircuitBreakerRegistry(config)

	// Create some circuit breakers
	cb1 := reg.Get("provider-a")
	cb2 := reg.Get("provider-b")

	// Record some activity
	cb1.RecordSuccess()
	cb1.RecordFailure()
	cb2.RecordFailure()

	stats := reg.GetStats()

	if len(stats) != 2 {
		t.Errorf("stats count = %d, want 2", len(stats))
	}

	if stats["provider-a"].Successes != 1 {
		t.Errorf("provider-a successes = %d, want 1", stats["provider-a"].Successes)
	}

	if stats["provider-b"].Failures != 1 {
		t.Errorf("provider-b failures = %d, want 1", stats["provider-b"].Failures)
	}
}

func TestCircuitBreakerRegistry_Reset(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	reg := NewCircuitBreakerRegistry(config)

	cb := reg.Get("provider")
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatal("Circuit should be open")
	}

	reg.Reset("provider")

	if cb.State() != StateClosed {
		t.Errorf("state after reset = %v, want %v", cb.State(), StateClosed)
	}
}

func TestCircuitBreakerRegistry_ResetNonExistent(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	reg := NewCircuitBreakerRegistry(config)

	// Should not panic
	reg.Reset("non-existent")
}

func TestCircuitBreakerRegistry_ResetAll(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	reg := NewCircuitBreakerRegistry(config)

	cb1 := reg.Get("provider-a")
	cb2 := reg.Get("provider-b")

	cb1.RecordFailure()
	cb1.RecordFailure()
	cb2.RecordFailure()
	cb2.RecordFailure()

	reg.ResetAll()

	if cb1.State() != StateClosed {
		t.Error("provider-a should be reset")
	}

	if cb2.State() != StateClosed {
		t.Error("provider-b should be reset")
	}
}

func BenchmarkCircuitBreakerRecord(b *testing.B) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			cb.RecordSuccess()
		} else {
			cb.RecordFailure()
		}
	}
}

func BenchmarkCircuitBreakerState(b *testing.B) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.State()
	}
}
