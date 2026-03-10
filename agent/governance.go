package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

var (
	ErrGovernanceDenied = errors.New("agent: governance check denied tool execution")
)

// IntegrityCheck validates tool executions before they run.
// Implementations can enforce policies, validate inputs, or prevent dangerous operations.
type IntegrityCheck interface {
	// Validate checks if a tool execution should be allowed.
	// Returns nil to allow execution, or an error to deny it.
	// The error message will be provided to the agent as observation for self-correction.
	Validate(ctx context.Context, toolName string, params map[string]any) error
}

// IntegrityCheckFunc is an adapter to use a function as an IntegrityCheck.
type IntegrityCheckFunc func(ctx context.Context, toolName string, params map[string]any) error

func (f IntegrityCheckFunc) Validate(ctx context.Context, toolName string, params map[string]any) error {
	return f(ctx, toolName, params)
}

// Governance manages integrity checks for tool execution.
type Governance struct {
	checks []IntegrityCheck
	logger *slog.Logger
	mu     sync.RWMutex
}

// NewGovernance creates a new governance manager with optional checks.
func NewGovernance(checks ...IntegrityCheck) *Governance {
	return &Governance{
		checks: checks,
		logger: slog.Default(),
	}
}

// AddCheck appends a new integrity check.
func (g *Governance) AddCheck(check IntegrityCheck) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checks = append(g.checks, check)
}

// RemoveAllChecks clears all integrity checks.
func (g *Governance) RemoveAllChecks() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checks = nil
}

// SetLogger configures the governance logger.
func (g *Governance) SetLogger(logger *slog.Logger) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.logger = logger
}

// Validate runs all integrity checks for a tool execution.
// Returns nil if all checks pass, or an error from the first failing check.
func (g *Governance) Validate(ctx context.Context, toolName string, params map[string]any) error {
	g.mu.RLock()
	checks := g.checks
	logger := g.logger
	g.mu.RUnlock()

	for _, check := range checks {
		if err := check.Validate(ctx, toolName, params); err != nil {
			logger.Warn("governance check denied tool execution",
				"tool", toolName,
				"error", err,
			)
			return fmt.Errorf("%w: %w", ErrGovernanceDenied, err)
		}
	}

	logger.Debug("governance checks passed",
		"tool", toolName,
		"check_count", len(checks),
	)

	return nil
}

// Common IntegrityCheck implementations

// PermissionCheck validates tool execution based on a permission map.
type PermissionCheck struct {
	// Allowed is a set of tool names that are permitted.
	Allowed map[string]bool
	// Denied is a set of tool names that are explicitly blocked.
	Denied map[string]bool
}

func NewPermissionCheck() *PermissionCheck {
	return &PermissionCheck{
		Allowed: make(map[string]bool),
		Denied:  make(map[string]bool),
	}
}

func (p *PermissionCheck) Allow(tools ...string) *PermissionCheck {
	for _, tool := range tools {
		p.Allowed[tool] = true
		delete(p.Denied, tool)
	}
	return p
}

func (p *PermissionCheck) Deny(tools ...string) *PermissionCheck {
	for _, tool := range tools {
		p.Denied[tool] = true
		delete(p.Allowed, tool)
	}
	return p
}

func (p *PermissionCheck) Validate(ctx context.Context, toolName string, params map[string]any) error {
	if len(p.Denied) > 0 && p.Denied[toolName] {
		return fmt.Errorf("tool %q is explicitly denied", toolName)
	}

	if len(p.Allowed) > 0 && !p.Allowed[toolName] {
		return fmt.Errorf("tool %q is not in the allowed list", toolName)
	}

	return nil
}

// ParameterCheck validates tool parameters against required fields.
type ParameterCheck struct {
	// Required maps tool names to their required parameter keys.
	Required map[string][]string
	// Forbidden maps tool names to parameters that must not be present.
	Forbidden map[string][]string
}

func NewParameterCheck() *ParameterCheck {
	return &ParameterCheck{
		Required:  make(map[string][]string),
		Forbidden: make(map[string][]string),
	}
}

func (p *ParameterCheck) Require(tool string, params ...string) *ParameterCheck {
	p.Required[tool] = append(p.Required[tool], params...)
	return p
}

func (p *ParameterCheck) Forbid(tool string, params ...string) *ParameterCheck {
	p.Forbidden[tool] = append(p.Forbidden[tool], params...)
	return p
}

func (p *ParameterCheck) Validate(ctx context.Context, toolName string, params map[string]any) error {
	if required, ok := p.Required[toolName]; ok {
		for _, key := range required {
			if _, exists := params[key]; !exists {
				return fmt.Errorf("tool %q missing required parameter: %s", toolName, key)
			}
		}
	}

	if forbidden, ok := p.Forbidden[toolName]; ok {
		for _, key := range forbidden {
			if _, exists := params[key]; exists {
				return fmt.Errorf("tool %q has forbidden parameter: %s", toolName, key)
			}
		}
	}

	return nil
}

// RateLimitCheck enforces rate limits on tool execution.
type RateLimitCheck struct {
	limits map[string]*rateLimiter
	mu     sync.RWMutex
}

type rateLimiter struct {
	count int
	max   int
}

func NewRateLimitCheck() *RateLimitCheck {
	return &RateLimitCheck{
		limits: make(map[string]*rateLimiter),
	}
}

func (r *RateLimitCheck) SetLimit(tool string, maxPerSession int) *RateLimitCheck {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limits[tool] = &rateLimiter{max: maxPerSession}
	return r
}

func (r *RateLimitCheck) Validate(ctx context.Context, toolName string, params map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	limiter, ok := r.limits[toolName]
	if !ok {
		return nil
	}

	limiter.count++
	if limiter.count > limiter.max {
		return fmt.Errorf("tool %q exceeded rate limit of %d calls", toolName, limiter.max)
	}

	return nil
}

func (r *RateLimitCheck) Reset(tool string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limiter, ok := r.limits[tool]; ok {
		limiter.count = 0
	}
}

// ContentSafetyCheck validates content for safety concerns.
// This is a simple implementation - production systems should use more sophisticated checks.
type ContentSafetyCheck struct {
	// BlockedPatterns maps tool names to patterns that should block execution.
	BlockedPatterns map[string][]string
}

func NewContentSafetyCheck() *ContentSafetyCheck {
	return &ContentSafetyCheck{
		BlockedPatterns: make(map[string][]string),
	}
}

func (c *ContentSafetyCheck) BlockPattern(tool string, patterns ...string) *ContentSafetyCheck {
	c.BlockedPatterns[tool] = append(c.BlockedPatterns[tool], patterns...)
	return c
}

func (c *ContentSafetyCheck) Validate(ctx context.Context, toolName string, params map[string]any) error {
	patterns, ok := c.BlockedPatterns[toolName]
	if !ok {
		return nil
	}

	for key, value := range params {
		strVal, ok := value.(string)
		if !ok {
			continue
		}

		for _, pattern := range patterns {
			if containsBlockedPattern(strVal, pattern) {
				return fmt.Errorf("tool %q contains blocked content in parameter %q", toolName, key)
			}
		}
	}

	return nil
}

func containsBlockedPattern(content, pattern string) bool {
	return len(pattern) > 0 && len(content) >= len(pattern) &&
		(content == pattern || containsSubstring(content, pattern))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CompositeCheck combines multiple checks into one.
type CompositeCheck struct {
	checks []IntegrityCheck
}

func NewCompositeCheck(checks ...IntegrityCheck) *CompositeCheck {
	return &CompositeCheck{checks: checks}
}

func (c *CompositeCheck) Add(check IntegrityCheck) {
	c.checks = append(c.checks, check)
}

func (c *CompositeCheck) Validate(ctx context.Context, toolName string, params map[string]any) error {
	for _, check := range c.checks {
		if err := check.Validate(ctx, toolName, params); err != nil {
			return err
		}
	}
	return nil
}
