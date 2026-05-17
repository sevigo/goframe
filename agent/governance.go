package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

var (
	// ErrGovernanceDenied is returned when a governance check blocks a tool execution.
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

// Validate calls the underlying function.
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

// NewPermissionCheck creates a permission check with empty allow/deny lists.
func NewPermissionCheck() *PermissionCheck {
	return &PermissionCheck{
		Allowed: make(map[string]bool),
		Denied:  make(map[string]bool),
	}
}

// Allow adds tools to the allowed list and removes them from denied.
func (p *PermissionCheck) Allow(tools ...string) *PermissionCheck {
	for _, tool := range tools {
		p.Allowed[tool] = true
		delete(p.Denied, tool)
	}
	return p
}

// Deny adds tools to the denied list and removes them from allowed.
func (p *PermissionCheck) Deny(tools ...string) *PermissionCheck {
	for _, tool := range tools {
		p.Denied[tool] = true
		delete(p.Allowed, tool)
	}
	return p
}

// Validate checks if a tool is permitted by the allow/deny lists.
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

// NewParameterCheck creates a parameter check with empty required/forbidden maps.
func NewParameterCheck() *ParameterCheck {
	return &ParameterCheck{
		Required:  make(map[string][]string),
		Forbidden: make(map[string][]string),
	}
}

// Require adds required parameter keys for a tool.
func (p *ParameterCheck) Require(tool string, params ...string) *ParameterCheck {
	p.Required[tool] = append(p.Required[tool], params...)
	return p
}

// Forbid adds forbidden parameter keys for a tool.
func (p *ParameterCheck) Forbid(tool string, params ...string) *ParameterCheck {
	p.Forbidden[tool] = append(p.Forbidden[tool], params...)
	return p
}

// Validate checks if a tool's parameters satisfy required and forbidden constraints.
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
// Use RecordCall after a successful tool execution to track usage.
// Validate only checks if the limit has been reached without mutating state.
type RateLimitCheck struct {
	limits map[string]*rateLimiter
	mu     sync.RWMutex
}

type rateLimiter struct {
	count int
	max   int
}

// NewRateLimitCheck creates a new rate-limit checker.
func NewRateLimitCheck() *RateLimitCheck {
	return &RateLimitCheck{
		limits: make(map[string]*rateLimiter),
	}
}

// SetLimit configures the max calls allowed for a tool.
func (r *RateLimitCheck) SetLimit(tool string, maxPerSession int) *RateLimitCheck {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limits[tool] = &rateLimiter{max: maxPerSession}
	return r
}

// Validate checks if a tool execution would exceed the rate limit.
// It does NOT increment the counter — call RecordCall after a successful execution.
func (r *RateLimitCheck) Validate(ctx context.Context, toolName string, params map[string]any) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limiter, ok := r.limits[toolName]
	if !ok {
		return nil
	}

	if limiter.count >= limiter.max {
		return fmt.Errorf("tool %q exceeded rate limit of %d calls", toolName, limiter.max)
	}

	return nil
}

// RecordCall increments the call counter for a tool.
// Call this after a tool has been successfully executed.
func (r *RateLimitCheck) RecordCall(toolName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limiter, ok := r.limits[toolName]; ok {
		limiter.count++
	}
}

// Reset resets the call counter for a tool.
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

// NewContentSafetyCheck creates a content safety check with empty blocked patterns.
func NewContentSafetyCheck() *ContentSafetyCheck {
	return &ContentSafetyCheck{
		BlockedPatterns: make(map[string][]string),
	}
}

// BlockPattern adds blocked content patterns for a tool.
func (c *ContentSafetyCheck) BlockPattern(tool string, patterns ...string) *ContentSafetyCheck {
	c.BlockedPatterns[tool] = append(c.BlockedPatterns[tool], patterns...)
	return c
}

// Validate checks if a tool's parameters contain blocked content.
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
			if pattern != "" && strings.Contains(strVal, pattern) {
				return fmt.Errorf("tool %q contains blocked content in parameter %q", toolName, key)
			}
		}
	}

	return nil
}

// CompositeCheck combines multiple checks into one.
type CompositeCheck struct {
	checks []IntegrityCheck
}

// NewCompositeCheck creates a check that combines multiple integrity checks.
func NewCompositeCheck(checks ...IntegrityCheck) *CompositeCheck {
	return &CompositeCheck{checks: checks}
}

// Add appends an integrity check to the composite.
func (c *CompositeCheck) Add(check IntegrityCheck) {
	c.checks = append(c.checks, check)
}

// Validate runs all checks and returns the first error.
func (c *CompositeCheck) Validate(ctx context.Context, toolName string, params map[string]any) error {
	for _, check := range c.checks {
		if err := check.Validate(ctx, toolName, params); err != nil {
			return err
		}
	}
	return nil
}
