package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// MiddlewareConfig configures agentic middleware for the agent loop.
type MiddlewareConfig struct {
	// HumanApprovalHandler handles high-risk tool approval requests.
	HumanApprovalHandler HumanApprovalHandler
	// RiskAssessor evaluates the risk level of tool calls.
	RiskAssessor RiskAssessor
	// ActionVerifier verifies that actions achieved their goals.
	ActionVerifier ActionVerifier
	// VerificationEnabled enables action verification after tool calls.
	VerificationEnabled bool
	// HighRiskThreshold is the minimum risk level requiring human approval.
	HighRiskThreshold RiskLevel
	// ApprovalTimeout is the maximum wait time for human approval.
	ApprovalTimeout time.Duration
	// MaxSelfHealingAttempts is the maximum retry attempts for self-healing.
	MaxSelfHealingAttempts int
}

// MiddlewareOption configures the middleware.
type MiddlewareOption func(*MiddlewareConfig)

// WithHumanApprovalHandler sets the human approval handler.
func WithHumanApprovalHandler(handler HumanApprovalHandler) MiddlewareOption {
	return func(c *MiddlewareConfig) {
		c.HumanApprovalHandler = handler
	}
}

// WithRiskAssessor sets the risk assessor.
func WithRiskAssessor(assessor RiskAssessor) MiddlewareOption {
	return func(c *MiddlewareConfig) {
		c.RiskAssessor = assessor
	}
}

// WithActionVerifier sets the action verifier.
func WithActionVerifier(verifier ActionVerifier) MiddlewareOption {
	return func(c *MiddlewareConfig) {
		c.ActionVerifier = verifier
	}
}

// WithVerification enables or disables action verification.
func WithVerification(enabled bool) MiddlewareOption {
	return func(c *MiddlewareConfig) {
		c.VerificationEnabled = enabled
	}
}

// WithHighRiskThreshold sets the risk level that requires human approval.
func WithHighRiskThreshold(level RiskLevel) MiddlewareOption {
	return func(c *MiddlewareConfig) {
		c.HighRiskThreshold = level
	}
}

// WithApprovalTimeout sets the human approval timeout.
func WithApprovalTimeout(timeout time.Duration) MiddlewareOption {
	return func(c *MiddlewareConfig) {
		c.ApprovalTimeout = timeout
	}
}

// WithMaxSelfHealingAttempts sets the maximum retry attempts for self-healing.
func WithMaxSelfHealingAttempts(n int) MiddlewareOption {
	return func(c *MiddlewareConfig) {
		c.MaxSelfHealingAttempts = n
	}
}

// DefaultMiddlewareConfig returns sensible defaults.
func DefaultMiddlewareConfig() MiddlewareConfig {
	return MiddlewareConfig{
		RiskAssessor:           NewDefaultRiskAssessor(),
		VerificationEnabled:    false,
		HighRiskThreshold:      RiskHigh,
		ApprovalTimeout:        60 * time.Second,
		MaxSelfHealingAttempts: 3,
	}
}

// AgentLoopWithMiddleware extends AgentLoop with agentic middleware.
type AgentLoopWithMiddleware struct {
	*AgentLoop
	middleware *MiddlewareConfig
}

// NewAgentLoopWithMiddleware creates an agent loop with middleware support.
func NewAgentLoopWithMiddleware(model llms.Model, registry *Registry, opts ...MiddlewareOption) (*AgentLoopWithMiddleware, error) {
	loop, err := NewAgentLoop(model, registry)
	if err != nil {
		return nil, err
	}

	mwConfig := DefaultMiddlewareConfig()
	for _, opt := range opts {
		opt(&mwConfig)
	}

	return &AgentLoopWithMiddleware{
		AgentLoop:  loop,
		middleware: &mwConfig,
	}, nil
}

// Run executes the autonomous loop with middleware interception.
//
// Middleware Flow:
//  1. THINK: Call LLM with task + tool definitions
//  2. ASSESS_RISK: If high-risk tool call, assess risk level
//  3. REQUEST_APPROVAL: If above threshold, request human approval
//  4. AWAIT_APPROVAL: Block until human responds
//  5. ACT: Execute tool if approved, abort if rejected
//  6. VERIFY: Check if action achieved intended goal
//  7. SELF_HEAL: If verification failed, inject failure and retry
//  8. OBSERVE: Record result in session history
//  9. REPEAT until LLM produces final answer
func (l *AgentLoopWithMiddleware) Run(ctx context.Context, task Task, history []schema.MessageContent) (*LoopResult, error) {
	result := &LoopResult{
		State: StateThinking,
	}

	messages := l.buildInitialHistory(task)
	messages = append(messages, history...)

	selfHealingAttempts := 0

	for i := range l.maxIterations {
		select {
		case <-ctx.Done():
			result.State = StateError
			return result, ErrLoopCancelled
		default:
		}

		l.logger.Debug("starting iteration with middleware",
			"iteration", i+1,
			"state", "thinking",
		)

		// THINK: Call LLM
		response, toolCalls, err := l.think(ctx, messages)
		if err != nil {
			result.State = StateError
			return result, fmt.Errorf("think phase failed: %w", err)
		}

		// Add AI response to history
		messages = append(messages, schema.NewAIMessage(response))

		// If no tool calls, we have a final answer
		if len(toolCalls) == 0 {
			result.Response = response
			result.State = StateComplete
			result.Iterations = i + 1
			l.logger.Info("loop completed with middleware",
				"iterations", result.Iterations,
				"response_length", len(response),
			)
			return result, nil
		}

		// ACT & OBSERVE with middleware interception
		observations, toolRecords, abort := l.actAndObserveWithMiddleware(ctx, toolCalls, &selfHealingAttempts)
		if abort {
			result.State = StateError
			result.Iterations = i + 1
			return result, ErrHumanInterventionRequired
		}

		// Add observations to history
		messages = append(messages, observations...)
		result.ToolCalls = append(result.ToolCalls, toolRecords...)
		result.Iterations = i + 1
	}

	result.State = StateError
	return result, fmt.Errorf("%w (max: %d)", ErrMaxIterations, l.maxIterations)
}

// actAndObserveWithMiddleware executes tools with risk assessment, approval, and verification.
func (l *AgentLoopWithMiddleware) actAndObserveWithMiddleware(ctx context.Context, toolCalls []llms.ToolCall, selfHealingAttempts *int) ([]schema.MessageContent, []ToolCallRecord, bool) {
	toolRecords := make([]ToolCallRecord, 0, len(toolCalls))
	observations := make([]schema.MessageContent, 0, len(toolCalls))

	for _, tc := range toolCalls {
		toolName := tc.Function.Name
		params := tc.Function.Arguments

		l.logger.Debug("processing tool call with middleware",
			"tool", toolName,
			"params", params,
		)

		record := ToolCallRecord{
			Name:   toolName,
			Params: params,
		}

		// Step 1: Risk Assessment
		if l.middleware.RiskAssessor != nil {
			riskLevel := l.middleware.RiskAssessor.AssessRisk(ctx, toolName, params)
			l.logger.Debug("risk assessment",
				"tool", toolName,
				"risk_level", riskLevel,
			)

			// Step 2: Request Human Approval if High Risk
			if riskLevel >= l.middleware.HighRiskThreshold && l.middleware.HumanApprovalHandler != nil {
				approval, err := l.requestHumanApproval(ctx, toolName, params, riskLevel)
				if err != nil {
					l.logger.Error("human approval failed", "tool", toolName, "error", err)
					record.Error = err
					toolRecords = append(toolRecords, record)
					observations = append(observations, schema.NewToolResultMessage(toolName, fmt.Sprintf("HUMAN_APPROVAL_ERROR: %s", err.Error())))
					continue
				}

				if !approval {
					l.logger.Warn("action rejected by human", "tool", toolName)
					record.Error = ErrHumanInterventionRequired
					toolRecords = append(toolRecords, record)
					observations = append(observations, schema.NewToolResultMessage(toolName, "ACTION_ABORTED: Human rejected the operation"))
					continue
				}
			}
		}

		// Step 3: Governance Checks (from base AgentLoop)
		if l.governance != nil {
			if err := l.governance.Validate(ctx, toolName, params); err != nil {
				l.logger.Warn("governance blocked tool execution",
					"tool", toolName,
					"error", err,
				)
				record.Error = err
				toolRecords = append(toolRecords, record)
				observations = append(observations, schema.NewToolResultMessage(toolName, fmt.Sprintf("GOVERNANCE_BLOCKED: %s", err.Error())))
				continue
			}
		}

		// Step 4: Execute Tool
		result, err := l.registry.Execute(ctx, toolName, params)
		record.Result = result
		record.Error = err

		if err != nil {
			l.logger.Error("tool execution failed", "tool", toolName, "error", err)
			toolRecords = append(toolRecords, record)
			observations = append(observations, schema.NewToolResultMessage(toolName, fmt.Sprintf("EXECUTION_FAILED: %s", err.Error())))
			continue
		}

		// Step 5: Action Verification
		if l.middleware.VerificationEnabled && l.middleware.ActionVerifier != nil {
			verified, healAttempted := l.verifyAndSelfHeal(ctx, toolName, params, result, selfHealingAttempts)
			if !verified && !healAttempted {
				// Verification failed and max self-healing attempts reached
				record.Error = ErrActionFailedVerification
				toolRecords = append(toolRecords, record)
				observations = append(observations, schema.NewToolResultMessage(toolName, "VERIFICATION_FAILED: Action did not achieve intended goal"))
				continue
			}
		}

		l.logger.Debug("tool execution succeeded with middleware", "tool", toolName)
		toolRecords = append(toolRecords, record)

		// Create observation message
		var obsContent string
		obsJSON, jsonErr := json.Marshal(result)
		if jsonErr != nil {
			obsContent = fmt.Sprintf("Tool '%s' returned: %v", toolName, result)
		} else {
			obsContent = fmt.Sprintf("Tool '%s' returned: %s", toolName, string(obsJSON))
		}
		observations = append(observations, schema.NewToolResultMessage(toolName, obsContent))
	}

	return observations, toolRecords, false
}

// requestHumanApproval requests approval from the human operator.
func (l *AgentLoopWithMiddleware) requestHumanApproval(ctx context.Context, toolName string, params map[string]any, riskLevel RiskLevel) (bool, error) {
	req := HumanApprovalRequest{
		ToolName:  toolName,
		Params:    params,
		RiskLevel: riskLevel,
		Reason:    fmt.Sprintf("Tool '%s' is classified as high-risk operation", toolName),
		Impact:    "May cause irreversible changes or access external resources",
		Timeout:   l.middleware.ApprovalTimeout,
	}

	l.logger.Info("requesting human approval",
		"tool", toolName,
		"risk", riskLevel,
		"timeout", req.Timeout,
	)

	return l.middleware.HumanApprovalHandler.RequestApproval(ctx, req)
}

// verifyAndSelfHeal verifies the action and attempts self-healing if failed.
func (l *AgentLoopWithMiddleware) verifyAndSelfHeal(ctx context.Context, toolName string, params map[string]any, result any, attempts *int) (verified bool, healAttempted bool) {
	if *attempts >= l.middleware.MaxSelfHealingAttempts {
		l.logger.Warn("max self-healing attempts reached",
			"tool", toolName,
			"attempts", *attempts,
		)
		return false, false
	}

	vr, err := l.middleware.ActionVerifier.VerifyAction(ctx, toolName, params, result)
	if err != nil {
		l.logger.Error("verification failed", "tool", toolName, "error", err)
		return false, false
	}

	if vr.Verified {
		l.logger.Info("action verified successfully", "tool", toolName)
		return true, false
	}

	l.logger.Warn("action verification failed, attempting self-healing",
		"tool", toolName,
		"reason", vr.Reason,
		"correction", vr.Correction,
	)

	(*attempts)++
	return false, true
}
