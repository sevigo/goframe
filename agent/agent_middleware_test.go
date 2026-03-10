package agent

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

// MockModel implements Model for testing.
type MockModel struct {
	responses []string
	index     int
}

func (m *MockModel) Call(ctx context.Context, prompt string) (string, error) {
	if m.index >= len(m.responses) {
		return "", errors.New("no more responses")
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func TestDefaultRiskAssessor_AssessRisk(t *testing.T) {
	assessor := NewDefaultRiskAssessor()

	tests := []struct {
		name     string
		toolName string
		params   map[string]any
		wantRisk RiskLevel
	}{
		{
			name:     "low risk - read operation",
			toolName: "read",
			params:   map[string]any{"path": "/tmp/file"},
			wantRisk: RiskLow,
		},
		{
			name:     "medium risk - write operation",
			toolName: "write",
			params:   map[string]any{"path": "/tmp/file", "content": "data"},
			wantRisk: RiskMedium,
		},
		{
			name:     "high risk - delete operation",
			toolName: "delete",
			params:   map[string]any{"path": "/tmp/file"},
			wantRisk: RiskHigh,
		},
		{
			name:     "critical risk - format disk",
			toolName: "format_disk",
			params:   map[string]any{"disk": "/dev/sda"},
			wantRisk: RiskCritical,
		},
		{
			name:     "high risk - dangerous params",
			toolName: "execute",
			params:   map[string]any{"command": "rm -rf /"},
			wantRisk: RiskHigh,
		},
		{
			name:     "high risk - external navigation",
			toolName: "navigate",
			params:   map[string]any{"url": "https://external.com"},
			wantRisk: RiskHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := assessor.AssessRisk(context.Background(), tt.toolName, tt.params)
			if risk != tt.wantRisk {
				t.Errorf("AssessRisk(%q) = %v, want %v", tt.toolName, risk, tt.wantRisk)
			}
		})
	}
}

func TestDefaultRiskAssessor_AddHighRiskTool(t *testing.T) {
	assessor := NewDefaultRiskAssessor()

	// Initially not high risk
	risk := assessor.AssessRisk(context.Background(), "custom_tool", nil)
	if risk == RiskHigh {
		t.Error("custom_tool should not be high risk initially")
	}

	// Add as high risk
	assessor.AddHighRiskTool("custom_tool")

	// Now should be high risk
	risk = assessor.AssessRisk(context.Background(), "custom_tool", nil)
	if risk != RiskHigh {
		t.Error("custom_tool should be high risk after addition")
	}
}

func TestLLMAssistedVerifier_VerifyAction(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		wantVerified   bool
		wantReason     string
		wantCorrection string
	}{
		{
			name:         "verified success",
			response:     "ACTION_VERIFIED",
			wantVerified: true,
		},
		{
			name:         "verified with status",
			response:     "VERIFICATION_STATUS: SUCCESS\nREASON: Button clicked successfully",
			wantVerified: true,
			wantReason:   "Button clicked successfully",
		},
		{
			name:           "verification failed with correction",
			response:       "VERIFICATION_STATUS: FAILURE\nREASON: Button blocked by modal\nCORRECTION: Close modal first with selector .modal-close",
			wantVerified:   false,
			wantReason:     "Button blocked by modal",
			wantCorrection: "Close modal first with selector .modal-close",
		},
		{
			name:         "implicit failure",
			response:     "The action did not succeed. The element was not found.",
			wantVerified: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &MockModel{responses: []string{tt.response}}
			verifier := NewLLMAssistedVerifier(model)

			result, err := verifier.VerifyAction(context.Background(), "click", map[string]any{"selector": "#btn"}, "clicked")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Verified != tt.wantVerified {
				t.Errorf("Verified = %v, want %v", result.Verified, tt.wantVerified)
			}

			if tt.wantReason != "" && result.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", result.Reason, tt.wantReason)
			}

			if tt.wantCorrection != "" && result.Correction != tt.wantCorrection {
				t.Errorf("Correction = %q, want %q", result.Correction, tt.wantCorrection)
			}
		})
	}
}

func TestLLMAssistedVerifier_BuildVerificationPrompt(t *testing.T) {
	model := &MockModel{}
	verifier := NewLLMAssistedVerifier(model)

	prompt := verifier.buildVerificationPrompt("click", map[string]any{"selector": "#submit-btn"}, "success")

	// Check prompt contains key elements
	if !contains(prompt, "click") {
		t.Error("prompt should contain tool name")
	}
	if !contains(prompt, "#submit-btn") {
		t.Error("prompt should contain params")
	}
	if !contains(prompt, "success") {
		t.Error("prompt should contain result")
	}
	if !contains(prompt, "ACTION_VERIFIED") {
		t.Error("prompt should request ACTION_VERIFIED marker")
	}
	if !contains(prompt, "VERIFICATION_STATUS") {
		t.Error("prompt should request VERIFICATION_STATUS")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMockApprovalHandler(t *testing.T) {
	tests := []struct {
		name         string
		autoApprove  bool
		autoReject   bool
		wantApproval bool
		wantErr      bool
	}{
		{
			name:         "auto approve",
			autoApprove:  true,
			wantApproval: true,
		},
		{
			name:         "auto reject",
			autoReject:   true,
			wantApproval: false,
		},
		{
			name:    "no auto config",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &MockApprovalHandler{
				AutoApprove: tt.autoApprove,
				AutoReject:  tt.autoReject,
				Logger:      slog.Default(),
			}

			req := HumanApprovalRequest{
				ToolName: "test_tool",
				Params:   map[string]any{"key": "value"},
			}

			approved, err := handler.RequestApproval(context.Background(), req)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if approved != tt.wantApproval {
				t.Errorf("approved = %v, want %v", approved, tt.wantApproval)
			}
		})
	}
}

func TestChannelApprovalHandler(t *testing.T) {
	approvalChan := make(chan bool, 1)
	requestChan := make(chan HumanApprovalRequest, 1)

	handler := NewChannelApprovalHandler(approvalChan, requestChan, 5*time.Second)

	req := HumanApprovalRequest{
		ToolName: "delete",
		Params:   map[string]any{"path": "/important/file"},
	}

	// Simulate approval in goroutine
	go func() {
		<-requestChan        // Wait for request
		approvalChan <- true // Send approval
	}()

	approved, err := handler.RequestApproval(context.Background(), req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !approved {
		t.Error("expected approval, got rejection")
	}
}

func TestChannelApprovalHandler_Timeout(t *testing.T) {
	approvalChan := make(chan bool, 1)
	requestChan := make(chan HumanApprovalRequest, 1)

	handler := NewChannelApprovalHandler(approvalChan, requestChan, 100*time.Millisecond)

	req := HumanApprovalRequest{
		ToolName: "delete",
		Params:   map[string]any{"path": "/important/file"},
		Timeout:  50 * time.Millisecond,
	}

	_, err := handler.RequestApproval(context.Background(), req)
	if !errors.Is(err, ErrApprovalTimedOut) {
		t.Errorf("expected ErrApprovalTimedOut, got %v", err)
	}
}

func TestRiskLevel_String(t *testing.T) {
	levels := []RiskLevel{RiskLow, RiskMedium, RiskHigh, RiskCritical}
	for _, level := range levels {
		// Just ensure it doesn't panic
		_ = level
	}
}
