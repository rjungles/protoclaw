// Package connection provides connection management for LLM providers
package connection

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Pool manages shared HTTP connections with rate limiting and circuit breaking
type Pool struct {
	client   *http.Client
	breakers *CircuitBreakerRegistry
	limits   map[string]*rate.Limiter
	config   PoolConfig
	mu       sync.RWMutex
	stats    *PoolStats
}

// PoolConfig configures the connection pool
type PoolConfig struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
	ResponseHeaderTimeout time.Duration
	ExpectContinueTimeout time.Duration
	Timeout             time.Duration
	RateLimitPerSecond  rate.Limit
	RateBurst           int
}

// DefaultPoolConfig returns sensible defaults
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		Timeout:               60 * time.Second,
		RateLimitPerSecond:    rate.Limit(10), // 10 req/sec
		RateBurst:             20,
	}
}

// NewPool creates a connection pool
func NewPool(config PoolConfig) *Pool {
	transport := &http.Transport{
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		ExpectContinueTimeout: config.ExpectContinueTimeout,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	return &Pool{
		client:   client,
		breakers: NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig()),
		limits:   make(map[string]*rate.Limiter),
		config:   config,
		stats:    &PoolStats{},
	}
}

// Execute performs an HTTP request with rate limiting and circuit breaking
func (p *Pool) Execute(ctx context.Context, req *http.Request, provider string) (*http.Response, error) {
	start := time.Now()

	// Check circuit breaker
	breaker := p.breakers.Get(provider)
	if !breaker.Allow() {
		p.recordFailure(provider, time.Since(start))
		return nil, fmt.Errorf("circuit breaker open for provider %s", provider)
	}

	// Check rate limit
	limiter := p.getLimiter(provider)
	if !limiter.Allow() {
		p.recordFailure(provider, time.Since(start))
		return nil, fmt.Errorf("rate limit exceeded for provider %s", provider)
	}

	// Execute request
	resp, err := p.client.Do(req)
	latency := time.Since(start)

	// Update circuit breaker and stats
	if err != nil {
		breaker.RecordFailure()
		p.recordFailure(provider, latency)
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// HTTP errors >= 500 are considered failures
	if resp.StatusCode >= 500 {
		breaker.RecordFailure()
		p.recordFailure(provider, latency)
	} else {
		breaker.RecordSuccess()
		p.recordSuccess(provider, latency)
	}

	return resp, nil
}

// ExecuteWithRetry executes with retry logic
func (p *Pool) ExecuteWithRetry(ctx context.Context, req *http.Request, provider string, maxRetries int) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := p.Execute(ctx, req, provider)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on certain errors
		if !shouldRetry(err) {
			return nil, err
		}

		// Wait before retry with exponential backoff
		if attempt < maxRetries {
			delay := calculateBackoff(attempt)
			select {
			case <-time.After(delay):
				// Continue to next attempt
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ExecuteWithTrace executes with detailed tracing
func (p *Pool) ExecuteWithTrace(ctx context.Context, req *http.Request, provider string) (*http.Response, *TraceInfo, error) {
	trace := &TraceInfo{Start: time.Now()}

	traceClient := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			trace.DNSStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			trace.DNSDone = time.Now()
		},
		ConnectStart: func(network, addr string) {
			trace.ConnectStart = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			trace.ConnectDone = time.Now()
			trace.ConnectError = err
		},
		TLSHandshakeStart: func() {
			trace.TLSStart = time.Now()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			trace.TLSDone = time.Now()
			trace.TLSError = err
		},
		GotFirstResponseByte: func() {
			trace.FirstByte = time.Now()
		},
	}

	ctx = httptrace.WithClientTrace(ctx, traceClient)
	resp, err := p.Execute(ctx, req, provider)
	trace.End = time.Now()

	return resp, trace, err
}

// getLimiter returns a rate limiter for a provider
func (p *Pool) getLimiter(provider string) *rate.Limiter {
	p.mu.RLock()
	limiter, exists := p.limits[provider]
	p.mu.RUnlock()

	if exists {
		return limiter
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check
	limiter, exists = p.limits[provider]
	if exists {
		return limiter
	}

	limiter = rate.NewLimiter(p.config.RateLimitPerSecond, p.config.RateBurst)
	p.limits[provider] = limiter
	return limiter
}

// SetRateLimit sets a custom rate limit for a provider
func (p *Pool) SetRateLimit(provider string, limit rate.Limit, burst int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.limits[provider] = rate.NewLimiter(limit, burst)
}

// recordSuccess records a successful request
func (p *Pool) recordSuccess(provider string, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.Successes++
	p.stats.TotalLatency += latency

	if latency > p.stats.MaxLatency {
		p.stats.MaxLatency = latency
	}
	if p.stats.MinLatency == 0 || latency < p.stats.MinLatency {
		p.stats.MinLatency = latency
	}
}

// recordFailure records a failed request
func (p *Pool) recordFailure(provider string, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.Failures++
}

// GetStats returns pool statistics
func (p *Pool) GetStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := *p.stats
	if stats.Successes > 0 {
		stats.AvgLatency = stats.TotalLatency / time.Duration(stats.Successes)
	}
	return stats
}

// GetCircuitBreakerStats returns circuit breaker stats
func (p *Pool) GetCircuitBreakerStats() map[string]CircuitBreakerStats {
	return p.breakers.GetStats()
}

// ResetCircuitBreaker resets a provider's circuit breaker
func (p *Pool) ResetCircuitBreaker(provider string) {
	p.breakers.Reset(provider)
}

// Close closes the pool
func (p *Pool) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

// shouldRetry determines if an error should be retried
func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry context errors
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Retry on temporary errors
	if isTemporary(err) {
		return true
	}

	return true // Conservative: retry by default
}

// isTemporary checks if an error is temporary
func isTemporary(err error) bool {
	type temporary interface {
		Temporary() bool
	}

	if te, ok := err.(temporary); ok {
		return te.Temporary()
	}

	return false
}

// calculateBackoff calculates exponential backoff with jitter
func calculateBackoff(attempt int) time.Duration {
	base := 100 * time.Millisecond
	max := 30 * time.Second

	// Exponential: 100ms, 200ms, 400ms, 800ms, 1.6s...
	delay := base * (1 << attempt)
	if delay > max {
		delay = max
	}

	// Add jitter (±25%)
	jitter := time.Duration(float64(delay) * 0.25 * (2*rand.Float64() - 1))
	return delay + jitter
}

// TraceInfo contains detailed timing information
type TraceInfo struct {
	Start        time.Time
	DNSStart     time.Time
	DNSDone      time.Time
	ConnectStart time.Time
	ConnectDone  time.Time
	TLSStart     time.Time
	TLSDone      time.Time
	FirstByte    time.Time
	End          time.Time

	ConnectError error
	TLSError     error
}

// DNSDuration returns DNS lookup duration
func (t *TraceInfo) DNSDuration() time.Duration {
	if t.DNSDone.IsZero() || t.DNSStart.IsZero() {
		return 0
	}
	return t.DNSDone.Sub(t.DNSStart)
}

// ConnectDuration returns connection establishment duration
func (t *TraceInfo) ConnectDuration() time.Duration {
	if t.ConnectDone.IsZero() || t.ConnectStart.IsZero() {
		return 0
	}
	return t.ConnectDone.Sub(t.ConnectStart)
}

// TLSDuration returns TLS handshake duration
func (t *TraceInfo) TLSDuration() time.Duration {
	if t.TLSDone.IsZero() || t.TLSStart.IsZero() {
		return 0
	}
	return t.TLSDone.Sub(t.TLSStart)
}

// TotalDuration returns total request duration
func (t *TraceInfo) TotalDuration() time.Duration {
	return t.End.Sub(t.Start)
}

// PoolStats contains pool statistics
type PoolStats struct {
	Successes    int64
	Failures     int64
	TotalLatency time.Duration
	AvgLatency   time.Duration
	MaxLatency   time.Duration
	MinLatency   time.Duration
}
