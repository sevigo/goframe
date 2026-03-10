package agent

import (
	"context"
	"errors"
	"testing"
)

func TestGovernance_Validate(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&mockTool{name: "read", description: "read file"})
	_ = registry.Register(&mockTool{name: "write", description: "write file"})
	_ = registry.Register(&mockTool{name: "delete", description: "delete file"})

	tests := []struct {
		name       string
		governance *Governance
		toolName   string
		wantErr    bool
	}{
		{
			name:       "empty governance allows all",
			governance: NewGovernance(),
			toolName:   "delete",
			wantErr:    false,
		},
		{
			name:       "permission check allows tool",
			governance: NewGovernance(NewPermissionCheck().Allow("read", "write")),
			toolName:   "read",
			wantErr:    false,
		},
		{
			name:       "permission check denies unlisted tool",
			governance: NewGovernance(NewPermissionCheck().Allow("read", "write")),
			toolName:   "delete",
			wantErr:    true,
		},
		{
			name:       "permission check explicitly denies tool",
			governance: NewGovernance(NewPermissionCheck().Deny("delete")),
			toolName:   "delete",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.governance.Validate(context.Background(), tt.toolName, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPermissionCheck(t *testing.T) {
	tests := []struct {
		name     string
		check    *PermissionCheck
		toolName string
		wantErr  bool
	}{
		{
			name:     "empty permission allows all",
			check:    NewPermissionCheck(),
			toolName: "any",
			wantErr:  false,
		},
		{
			name:     "allowed tool",
			check:    NewPermissionCheck().Allow("read", "write"),
			toolName: "read",
			wantErr:  false,
		},
		{
			name:     "non-allowed tool",
			check:    NewPermissionCheck().Allow("read", "write"),
			toolName: "delete",
			wantErr:  true,
		},
		{
			name:     "denied tool",
			check:    NewPermissionCheck().Deny("delete"),
			toolName: "delete",
			wantErr:  true,
		},
		{
			name:     "denied overrides allowed",
			check:    NewPermissionCheck().Allow("delete").Deny("delete"),
			toolName: "delete",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.check.Validate(context.Background(), tt.toolName, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParameterCheck(t *testing.T) {
	tests := []struct {
		name     string
		check    *ParameterCheck
		toolName string
		params   map[string]any
		wantErr  bool
	}{
		{
			name:     "no requirements",
			check:    NewParameterCheck(),
			toolName: "test",
			params:   map[string]any{},
			wantErr:  false,
		},
		{
			name:     "required parameter present",
			check:    NewParameterCheck().Require("test", "path"),
			toolName: "test",
			params:   map[string]any{"path": "/tmp"},
			wantErr:  false,
		},
		{
			name:     "required parameter missing",
			check:    NewParameterCheck().Require("test", "path"),
			toolName: "test",
			params:   map[string]any{},
			wantErr:  true,
		},
		{
			name:     "forbidden parameter present",
			check:    NewParameterCheck().Forbid("test", "admin"),
			toolName: "test",
			params:   map[string]any{"admin": true},
			wantErr:  true,
		},
		{
			name:     "forbidden parameter absent",
			check:    NewParameterCheck().Forbid("test", "admin"),
			toolName: "test",
			params:   map[string]any{"user": "john"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.check.Validate(context.Background(), tt.toolName, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRateLimitCheck(t *testing.T) {
	check := NewRateLimitCheck().SetLimit("test", 2)

	// First call should succeed
	err := check.Validate(context.Background(), "test", nil)
	if err != nil {
		t.Errorf("first call failed: %v", err)
	}

	// Second call should succeed
	err = check.Validate(context.Background(), "test", nil)
	if err != nil {
		t.Errorf("second call failed: %v", err)
	}

	// Third call should fail
	err = check.Validate(context.Background(), "test", nil)
	if err == nil {
		t.Error("expected error on third call")
	}

	// Reset should allow calls again
	check.Reset("test")
	err = check.Validate(context.Background(), "test", nil)
	if err != nil {
		t.Errorf("call after reset failed: %v", err)
	}
}

func TestContentSafetyCheck(t *testing.T) {
	check := NewContentSafetyCheck().
		BlockPattern("execute", "rm -rf", "DROP TABLE")

	tests := []struct {
		name     string
		toolName string
		params   map[string]any
		wantErr  bool
	}{
		{
			name:     "safe content",
			toolName: "execute",
			params:   map[string]any{"query": "SELECT * FROM users"},
			wantErr:  false,
		},
		{
			name:     "blocked content",
			toolName: "execute",
			params:   map[string]any{"query": "DROP TABLE users"},
			wantErr:  true,
		},
		{
			name:     "blocked command",
			toolName: "execute",
			params:   map[string]any{"command": "rm -rf /"},
			wantErr:  true,
		},
		{
			name:     "non-string parameter",
			toolName: "execute",
			params:   map[string]any{"count": 42},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := check.Validate(context.Background(), tt.toolName, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompositeCheck(t *testing.T) {
	composite := NewCompositeCheck(
		NewPermissionCheck().Allow("read", "write"),
		NewParameterCheck().Require("write", "path"),
	)

	tests := []struct {
		name     string
		toolName string
		params   map[string]any
		wantErr  bool
	}{
		{
			name:     "passing all checks",
			toolName: "write",
			params:   map[string]any{"path": "/tmp"},
			wantErr:  false,
		},
		{
			name:     "failing permission check",
			toolName: "delete",
			params:   map[string]any{"path": "/tmp"},
			wantErr:  true,
		},
		{
			name:     "failing parameter check",
			toolName: "write",
			params:   map[string]any{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := composite.Validate(context.Background(), tt.toolName, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIntegrityCheckFunc(t *testing.T) {
	called := false
	check := IntegrityCheckFunc(func(ctx context.Context, toolName string, params map[string]any) error {
		called = true
		if toolName == "forbidden" {
			return errors.New("forbidden tool")
		}
		return nil
	})

	err := check.Validate(context.Background(), "allowed", nil)
	if !called {
		t.Error("check was not called")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = check.Validate(context.Background(), "forbidden", nil)
	if err == nil {
		t.Error("expected error for forbidden tool")
	}
}

func TestGovernance_AddCheck(t *testing.T) {
	g := NewGovernance()
	g.AddCheck(NewPermissionCheck().Allow("read"))

	err := g.Validate(context.Background(), "read", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = g.Validate(context.Background(), "write", nil)
	if err == nil {
		t.Error("expected error for write tool")
	}
}

func TestGovernance_RemoveAllChecks(t *testing.T) {
	g := NewGovernance(NewPermissionCheck().Allow("read"))

	err := g.Validate(context.Background(), "read", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	g.RemoveAllChecks()

	// After removal, all tools should be allowed
	err = g.Validate(context.Background(), "write", nil)
	if err != nil {
		t.Errorf("unexpected error after removing checks: %v", err)
	}
}
