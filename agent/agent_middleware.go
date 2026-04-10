package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

var (
	// ErrHumanInterventionRequired signals that the agent needs human approval.
	ErrHumanInterventionRequired = errors.New("agent: human intervention required for high-risk operation")
	// ErrActionFailedVerification signals that an action did not achieve its intended goal.
	ErrActionFailedVerification = errors.New("agent: action failed verification")
	// ErrApprovalTimedOut signals that human approval was not received within the timeout.
	ErrApprovalTimedOut = errors.New("agent: human approval timed out")
)

// RiskLevel categorizes tool execution risk levels.
type RiskLevel int

const (
	RiskLow      RiskLevel = iota // Safe operations (read, search)
	RiskMedium                    // Moderate operations (write, update)
	RiskHigh                      // Destructive operations (delete, navigate external)
	RiskCritical                  // Irreversible operations (format disk, publish)
)

// RiskAssessor evaluates the risk level of tool executions.
type RiskAssessor interface {
	// AssessRisk returns the risk level for a tool execution.
	AssessRisk(ctx context.Context, toolName string, params map[string]any) RiskLevel
}

// HumanApprovalRequest represents a request for human approval.
type HumanApprovalRequest struct {
	// ToolName is the tool being executed.
	ToolName string
	// Params are the parameters for the tool call.
	Params map[string]any
	// Reason explains why this action is necessary.
	Reason string
	// Impact describes the potential impact.
	Impact string
	// RiskLevel is the assessed risk level.
	RiskLevel RiskLevel
	// Timeout is the maximum wait time for approval.
	Timeout time.Duration
}

// HumanApprovalHandler handles human approval requests.
type HumanApprovalHandler interface {
	// RequestApproval requests human approval and blocks until received.
	// Returns true if approved, false if rejected, or error on timeout/failure.
	RequestApproval(ctx context.Context, req HumanApprovalRequest) (bool, error)
}

// VerifierResult represents the outcome of action verification.
type VerifierResult struct {
	// Verified indicates if the action achieved its intended goal.
	Verified bool
	// Reason explains why verification failed (if not verified).
	Reason string
	// Correction proposes a fix for failed actions.
	Correction string
	// Screenshot hints for UI (optional).
	ScreenshotHint string
}

// ActionVerifier verifies that actions achieved their intended goals.
type ActionVerifier interface {
	// VerifyAction checks if a tool execution achieved its goal.
	VerifyAction(ctx context.Context, toolName string, params map[string]any, result any) (VerifierResult, error)
}

// LLMAssistedVerifier uses an LLM to verify action results.
type LLMAssistedVerifier struct {
	model      SimpleModel
	logger     *slog.Logger
	maxRetries int
}

// LLMAssistedVerifierOption configures the verifier.
type LLMAssistedVerifierOption func(*LLMAssistedVerifier)

// NewLLMAssistedVerifier creates a verifier that uses an LLM for verification.
func NewLLMAssistedVerifier(model SimpleModel, opts ...LLMAssistedVerifierOption) *LLMAssistedVerifier {
	v := &LLMAssistedVerifier{
		model:      model,
		logger:     slog.Default(),
		maxRetries: 3,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// WithVerifierLogger sets the logger.
func WithVerifierLogger(logger *slog.Logger) LLMAssistedVerifierOption {
	return func(v *LLMAssistedVerifier) {
		if logger != nil {
			v.logger = logger
		}
	}
}

// WithVerifierMaxRetries sets the maximum verification retries.
func WithVerifierMaxRetries(n int) LLMAssistedVerifierOption {
	return func(v *LLMAssistedVerifier) {
		if n > 0 {
			v.maxRetries = n
		}
	}
}

// VerifyAction verifies that a tool execution achieved its intended goal.
func (v *LLMAssistedVerifier) VerifyAction(ctx context.Context, toolName string, params map[string]any, result any) (VerifierResult, error) {
	// Build verification prompt
	prompt := v.buildVerificationPrompt(toolName, params, result)

	// Call LLM for verification
	response, err := v.model.Call(ctx, prompt)
	if err != nil {
		return VerifierResult{}, fmt.Errorf("verification LLM call failed: %w", err)
	}

	// Parse verification result
	return v.parseVerificationResult(response), nil
}

// buildVerificationPrompt constructs the verification prompt.
func (v *LLMAssistedVerifier) buildVerificationPrompt(toolName string, params map[string]any, result any) string {
	var paramsStr string
	for k, val := range params {
		paramsStr += fmt.Sprintf("  - %s: %v\n", k, val)
	}

	return fmt.Sprintf(`You just executed: %s

Parameters:
%s

Tool returned: %v

CRITICAL VERIFICATION STEP:

Analyze the action result and determine:

1. Did the action achieve the intended goal?
2. If the page/state did not change as expected, identify the root cause:
   - Element blocked by modal or overlay?
   - Selector became stale or incorrect?
   - Insufficient wait time for dynamic content?
   - Permission or authentication issue?

3. If the action failed:
   - Describe specifically what went wrong
   - Propose a correction to the tool call arguments
   
4. If the action succeeded:
   - Confirm the expected state change occurred
   - End with: ACTION_VERIFIED

Response format:
VERIFICATION_STATUS: [SUCCESS|FAILURE]
REASON: [explanation]
CORRECTION: [if failed, specific fix]`,
		toolName, paramsStr, result)
}

// parseVerificationResult parses the LLM's verification response.
func (v *LLMAssistedVerifier) parseVerificationResult(response string) VerifierResult {
	result := VerifierResult{
		Verified: strings.Contains(response, "ACTION_VERIFIED") ||
			strings.Contains(response, "VERIFICATION_STATUS: SUCCESS"),
	}

	// Extract reason
	if idx := strings.Index(response, "REASON:"); idx >= 0 {
		reasonStart := idx + 7
		reasonEnd := strings.Index(response[reasonStart:], "\n")
		if reasonEnd < 0 {
			reasonEnd = len(response[reasonStart:])
		}
		result.Reason = strings.TrimSpace(response[reasonStart : reasonStart+reasonEnd])
	}

	// Extract correction
	if idx := strings.Index(response, "CORRECTION:"); idx >= 0 {
		corrStart := idx + 11
		result.Correction = strings.TrimSpace(response[corrStart:])
	}

	return result
}

// SimpleModel is a minimal LLM interface for verification.
// It provides a simpler API than llms.Model, suitable for single-prompt use cases.
type SimpleModel interface {
	Call(ctx context.Context, prompt string) (string, error)
}

// DefaultRiskAssessor provides basic risk assessment.
// It is safe for concurrent use once fully configured.
type DefaultRiskAssessor struct {
	highRiskTools     map[string]bool
	criticalRiskTools map[string]bool
	mu                sync.RWMutex
}

// NewDefaultRiskAssessor creates a risk assessor with sensible defaults.
func NewDefaultRiskAssessor() *DefaultRiskAssessor {
	return &DefaultRiskAssessor{
		highRiskTools: map[string]bool{
			"delete":   true,
			"remove":   true,
			"drop":     true,
			"navigate": true,
			"submit":   true,
			"publish":  true,
			"deploy":   true,
			"execute":  true,
			"format":   true,
			"wipe":     true,
			"destroy":  true,
		},
		criticalRiskTools: map[string]bool{
			"format_disk":   true,
			"delete_all":    true,
			"drop_database": true,
			"deploy_prod":   true,
			"publish_all":   true,
		},
	}
}

// AssessRisk evaluates the risk level of a tool execution.
func (r *DefaultRiskAssessor) AssessRisk(ctx context.Context, toolName string, params map[string]any) RiskLevel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check critical tools
	if r.criticalRiskTools[toolName] {
		return RiskCritical
	}

	// Check high-risk tools
	if r.highRiskTools[toolName] {
		return RiskHigh
	}

	// Check params for dangerous patterns
	if r.hasDangerousParams(params) {
		return RiskHigh
	}

	// Check for external navigation
	if toolName == "navigate" || toolName == "goto" {
		if url, ok := params["url"].(string); ok {
			if r.isExternalDomain(url) {
				return RiskHigh
			}
		}
	}

	// Default to medium risk for write operations
	if r.isWriteOperation(toolName) {
		return RiskMedium
	}

	return RiskLow
}

func (r *DefaultRiskAssessor) hasDangerousParams(params map[string]any) bool {
	// Check for dangerous patterns in parameters
	for key, val := range params {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "password") ||
			strings.Contains(lowerKey, "secret") ||
			strings.Contains(lowerKey, "token") {
			return true
		}

		// Check for dangerous values
		if strVal, ok := val.(string); ok {
			lower := strings.ToLower(strVal)
			if strings.Contains(lower, "rm -rf") ||
				strings.Contains(lower, "format") ||
				strings.Contains(lower, "delete all") {
				return true
			}
		}
	}
	return false
}

func (r *DefaultRiskAssessor) isExternalDomain(url string) bool {
	// Simple check - in production, use proper URL parsing
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func (r *DefaultRiskAssessor) isWriteOperation(toolName string) bool {
	writeOps := map[string]bool{
		"write":  true,
		"update": true,
		"create": true,
		"modify": true,
		"edit":   true,
		"patch":  true,
		"put":    true,
		"post":   true,
	}
	return writeOps[toolName]
}

// AddHighRiskTool marks a tool as high-risk.
func (r *DefaultRiskAssessor) AddHighRiskTool(toolName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.highRiskTools[toolName] = true
}

// AddCriticalRiskTool marks a tool as critical-risk.
func (r *DefaultRiskAssessor) AddCriticalRiskTool(toolName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.criticalRiskTools[toolName] = true
}

// ChannelApprovalHandler implements HumanApprovalHandler using channels.
type ChannelApprovalHandler struct {
	// ApprovalChan receives approval decisions from UI.
	ApprovalChan <-chan bool
	// RequestChan sends approval requests to UI.
	RequestChan chan<- HumanApprovalRequest
	// Timeout is the default approval timeout.
	Timeout time.Duration
	// Logger records approval events.
	Logger *slog.Logger
}

// NewChannelApprovalHandler creates a channel-based approval handler.
func NewChannelApprovalHandler(approvalChan <-chan bool, requestChan chan<- HumanApprovalRequest, timeout time.Duration) *ChannelApprovalHandler {
	return &ChannelApprovalHandler{
		ApprovalChan: approvalChan,
		RequestChan:  requestChan,
		Timeout:      timeout,
		Logger:       slog.Default(),
	}
}

// RequestApproval sends a request and waits for approval.
func (h *ChannelApprovalHandler) RequestApproval(ctx context.Context, req HumanApprovalRequest) (bool, error) {
	// Use request timeout or default
	timeout := req.Timeout
	if timeout == 0 {
		timeout = h.Timeout
	}
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Send request to UI
	select {
	case h.RequestChan <- req:
		h.Logger.Debug("approval request sent", "tool", req.ToolName, "risk", req.RiskLevel)
	case <-ctx.Done():
		return false, ErrApprovalTimedOut
	}

	// Wait for approval
	select {
	case approved := <-h.ApprovalChan:
		if approved {
			h.Logger.Info("action approved by human", "tool", req.ToolName)
		} else {
			h.Logger.Warn("action rejected by human", "tool", req.ToolName)
		}
		return approved, nil
	case <-ctx.Done():
		h.Logger.Warn("approval timed out", "tool", req.ToolName, "timeout", timeout)
		return false, ErrApprovalTimedOut
	}
}

// MockApprovalHandler is a simple approval handler for testing.
type MockApprovalHandler struct {
	AutoApprove bool
	AutoReject  bool
	Delay       time.Duration
	Logger      *slog.Logger
}

// NewMockApprovalHandler creates a mock handler for testing.
func NewMockApprovalHandler(autoApprove bool) *MockApprovalHandler {
	return &MockApprovalHandler{
		AutoApprove: autoApprove,
		Logger:      slog.Default(),
	}
}

// RequestApproval returns a configured response for testing.
func (h *MockApprovalHandler) RequestApproval(ctx context.Context, req HumanApprovalRequest) (bool, error) {
	if h.Delay > 0 {
		select {
		case <-time.After(h.Delay):
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	if h.AutoApprove {
		h.Logger.Debug("auto-approving action", "tool", req.ToolName)
		return true, nil
	}

	if h.AutoReject {
		h.Logger.Debug("auto-rejecting action", "tool", req.ToolName)
		return false, nil
	}

	// Default: require human approval
	return false, errors.New("mock handler: human approval required but not configured")
}
