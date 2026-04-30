// Package health provides health checking functionality for AgentOS
package health

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// Status represents health status
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
	StatusUnknown   Status = "unknown"
)

// CheckResult contains the result of a health check
type CheckResult struct {
	Component   string
	Status      Status
	Message     string
	Latency     time.Duration
	CheckedAt   time.Time
	Metadata    map[string]interface{}
}

// ComponentHealth contains health information for a component
type ComponentHealth struct {
	Name        string
	Status      Status
	LastCheck   time.Time
	LastSuccess time.Time
	LastFailure time.Time
	Failures    int
	Successes   int
}

// Checker performs health checks
type Checker struct {
	checks     map[string]Check
	results    map[string]*CheckResult
	components map[string]*ComponentHealth
	mu         sync.RWMutex
	interval   time.Duration
	stop       chan bool
	wg         sync.WaitGroup
	db         *sql.DB
}

// Check is a health check function
type Check func(ctx context.Context) CheckResult

// NewChecker creates a new health checker
func NewChecker(interval time.Duration, db *sql.DB) *Checker {
	return &Checker{
		checks:     make(map[string]Check),
		results:    make(map[string]*CheckResult),
		components: make(map[string]*ComponentHealth),
		interval:   interval,
		stop:       make(chan bool),
		db:         db,
	}
}

// Register registers a health check
func (c *Checker) Register(name string, check Check) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = check
	c.components[name] = &ComponentHealth{
		Name:   name,
		Status: StatusUnknown,
	}
}

// Unregister removes a health check
func (c *Checker) Unregister(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.checks, name)
	delete(c.results, name)
	delete(c.components, name)
}

// Check runs a specific health check
func (c *Checker) Check(ctx context.Context, name string) (*CheckResult, error) {
	c.mu.RLock()
	check, exists := c.checks[name]
	c.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("health check not found: %s", name)
	}

	result := check(ctx)
	result.Component = name
	result.CheckedAt = time.Now()

	c.mu.Lock()
	c.results[name] = &result

	// Update component health
	if comp, ok := c.components[name]; ok {
		comp.LastCheck = result.CheckedAt
		if result.Status == StatusHealthy {
			comp.LastSuccess = result.CheckedAt
			comp.Successes++
		} else {
			comp.LastFailure = result.CheckedAt
			comp.Failures++
		}
		comp.Status = result.Status
	}
	c.mu.Unlock()

	// Store in database if available
	if c.db != nil {
		c.storeResult(ctx, &result)
	}

	return &result, nil
}

// CheckAll runs all health checks
func (c *Checker) CheckAll(ctx context.Context) map[string]*CheckResult {
	c.mu.RLock()
	checks := make(map[string]Check)
	for name, check := range c.checks {
		checks[name] = check
	}
	c.mu.RUnlock()

	results := make(map[string]*CheckResult)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, check := range checks {
		wg.Add(1)
		go func(n string, ch Check) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			result := ch(ctx)
			result.Component = n
			result.CheckedAt = time.Now()

			mu.Lock()
			results[n] = &result
			mu.Unlock()

			c.mu.Lock()
			c.results[n] = &result
			if comp, ok := c.components[n]; ok {
				comp.LastCheck = result.CheckedAt
				if result.Status == StatusHealthy {
					comp.LastSuccess = result.CheckedAt
					comp.Successes++
				} else {
					comp.LastFailure = result.CheckedAt
					comp.Failures++
				}
				comp.Status = result.Status
			}
			c.mu.Unlock()

			if c.db != nil {
				c.storeResult(ctx, &result)
			}
		}(name, check)
	}

	wg.Wait()
	return results
}

// GetResult returns the last result for a component
func (c *Checker) GetResult(name string) (*CheckResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result, exists := c.results[name]
	if !exists {
		return nil, fmt.Errorf("no results for component: %s", name)
	}

	return result, nil
}

// GetAllResults returns all results
func (c *Checker) GetAllResults() map[string]*CheckResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	results := make(map[string]*CheckResult)
	for name, result := range c.results {
		results[name] = result
	}

	return results
}

// GetComponentHealth returns health info for a component
func (c *Checker) GetComponentHealth(name string) (*ComponentHealth, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	comp, exists := c.components[name]
	if !exists {
		return nil, fmt.Errorf("component not found: %s", name)
	}

	// Return a copy
	compCopy := *comp
	return &compCopy, nil
}

// GetOverallStatus returns the overall system status
func (c *Checker) GetOverallStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hasUnhealthy := false
	hasDegraded := false

	for _, comp := range c.components {
		switch comp.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}

	// Check if we have any healthy components
	hasHealthy := false
	for _, comp := range c.components {
		if comp.Status == StatusHealthy {
			hasHealthy = true
			break
		}
	}

	if hasHealthy {
		return StatusHealthy
	}

	return StatusUnknown
}

// Start starts the background health check loop
func (c *Checker) Start(ctx context.Context) {
	c.wg.Add(1)
	go c.loop(ctx)
}

// Stop stops the background health check loop
func (c *Checker) Stop() {
	close(c.stop)
	c.wg.Wait()
}

// loop runs health checks periodically
func (c *Checker) loop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Run immediately on start
	c.CheckAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stop:
			return
		case <-ticker.C:
			c.CheckAll(ctx)
		}
	}
}

// storeResult stores a result in the database
func (c *Checker) storeResult(ctx context.Context, result *CheckResult) error {
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO health_checks (component, status, latency_ms, message, checked_at)
		VALUES (?, ?, ?, ?, ?)
	`, result.Component, string(result.Status), int(result.Latency.Milliseconds()),
		result.Message, result.CheckedAt)

	return err
}

// Common health checks

// DatabaseHealthCheck creates a health check for a database
func DatabaseHealthCheck(db *sql.DB) Check {
	return func(ctx context.Context) CheckResult {
		start := time.Now()
		err := db.PingContext(ctx)
		latency := time.Since(start)

		if err != nil {
			return CheckResult{
				Status:  StatusUnhealthy,
				Message: fmt.Sprintf("database ping failed: %v", err),
				Latency: latency,
			}
		}

		if latency > 1*time.Second {
			return CheckResult{
				Status:  StatusDegraded,
				Message: "slow response",
				Latency: latency,
			}
		}

		return CheckResult{
			Status:  StatusHealthy,
			Message: "database responsive",
			Latency: latency,
		}
	}
}

// DiskHealthCheck creates a health check for disk space
func DiskHealthCheck(path string, threshold int64) Check {
	return func(ctx context.Context) CheckResult {
		// This is a placeholder - implement actual disk check
		return CheckResult{
			Status:  StatusHealthy,
			Message: "disk check placeholder",
			Latency: 0,
		}
	}
}

// MemoryHealthCheck creates a health check for memory usage
func MemoryHealthCheck(threshold float64) Check {
	return func(ctx context.Context) CheckResult {
		// This is a placeholder - implement actual memory check
		return CheckResult{
			Status:  StatusHealthy,
			Message: "memory check placeholder",
			Latency: 0,
		}
	}
}

// LLMProviderHealthCheck creates a health check for an LLM provider
func LLMProviderHealthCheck(providerName string, checkFunc func() error) Check {
	return func(ctx context.Context) CheckResult {
		start := time.Now()
		err := checkFunc()
		latency := time.Since(start)

		if err != nil {
			return CheckResult{
				Status:  StatusUnhealthy,
				Message: fmt.Sprintf("%s provider error: %v", providerName, err),
				Latency: latency,
			}
		}

		if latency > 5*time.Second {
			return CheckResult{
				Status:  StatusDegraded,
				Message: fmt.Sprintf("%s provider slow", providerName),
				Latency: latency,
			}
		}

		return CheckResult{
			Status:  StatusHealthy,
			Message: fmt.Sprintf("%s provider healthy", providerName),
			Latency: latency,
		}
	}
}

// SystemHealthCheck creates a health check for an AgentOS system
func SystemHealthCheck(systemPath string) Check {
	return func(ctx context.Context) CheckResult {
		start := time.Now()
		// Implement actual system health check
		latency := time.Since(start)

		return CheckResult{
			Status:  StatusHealthy,
			Message: "system healthy",
			Latency: latency,
		}
	}
}

// CompositeCheck creates a composite health check
func CompositeCheck(checks ...Check) Check {
	return func(ctx context.Context) CheckResult {
		start := time.Now()
		var messages []string
		hasUnhealthy := false
		hasDegraded := false

		for _, check := range checks {
			result := check(ctx)
			if result.Status == StatusUnhealthy {
				hasUnhealthy = true
			}
			if result.Status == StatusDegraded {
				hasDegraded = true
			}
			if result.Status != StatusHealthy {
				messages = append(messages, result.Message)
			}
		}

		latency := time.Since(start)

		if hasUnhealthy {
			return CheckResult{
				Status:  StatusUnhealthy,
				Message: fmt.Sprintf("some checks failed: %v", messages),
				Latency: latency,
			}
		}

		if hasDegraded {
			return CheckResult{
				Status:  StatusDegraded,
				Message: "some checks degraded",
				Latency: latency,
			}
		}

		return CheckResult{
			Status:  StatusHealthy,
			Message: "all checks passed",
			Latency: latency,
		}
	}
}
