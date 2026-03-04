package agent

import (
	"context"
)

type PermissionLevel string

const (
	PermissionAsk   PermissionLevel = "ask"
	PermissionAllow PermissionLevel = "allow"
	PermissionDeny  PermissionLevel = "deny"
)

type PermissionConfig struct {
	Bash     PermissionLevel            `json:"bash"`
	BashMap  map[string]PermissionLevel `json:"bashMap,omitempty"`
	Edit     PermissionLevel            `json:"edit"`
	Webfetch PermissionLevel            `json:"webfetch"`
}

type PermissionBuilder struct {
	config *PermissionConfig
}

func NewPermissions() *PermissionBuilder {
	return &PermissionBuilder{
		config: &PermissionConfig{
			Bash:     PermissionAsk,
			Edit:     PermissionAsk,
			Webfetch: PermissionAsk,
			BashMap:  make(map[string]PermissionLevel),
		},
	}
}

func (b *PermissionBuilder) AllowBash(patterns ...string) *PermissionBuilder {
	if len(patterns) == 0 {
		b.config.Bash = PermissionAllow
	} else {
		for _, pattern := range patterns {
			b.config.BashMap[pattern] = PermissionAllow
		}
	}
	return b
}

func (b *PermissionBuilder) DenyBash(patterns ...string) *PermissionBuilder {
	if len(patterns) == 0 {
		b.config.Bash = PermissionDeny
	} else {
		for _, pattern := range patterns {
			b.config.BashMap[pattern] = PermissionDeny
		}
	}
	return b
}

func (b *PermissionBuilder) AskBash(patterns ...string) *PermissionBuilder {
	if len(patterns) == 0 {
		b.config.Bash = PermissionAsk
	} else {
		for _, pattern := range patterns {
			b.config.BashMap[pattern] = PermissionAsk
		}
	}
	return b
}

func (b *PermissionBuilder) AllowEdit() *PermissionBuilder {
	b.config.Edit = PermissionAllow
	return b
}

func (b *PermissionBuilder) DenyEdit() *PermissionBuilder {
	b.config.Edit = PermissionDeny
	return b
}

func (b *PermissionBuilder) AskEdit() *PermissionBuilder {
	b.config.Edit = PermissionAsk
	return b
}

func (b *PermissionBuilder) AllowWebfetch() *PermissionBuilder {
	b.config.Webfetch = PermissionAllow
	return b
}

func (b *PermissionBuilder) DenyWebfetch() *PermissionBuilder {
	b.config.Webfetch = PermissionDeny
	return b
}

func (b *PermissionBuilder) AskWebfetch() *PermissionBuilder {
	b.config.Webfetch = PermissionAsk
	return b
}

func (b *PermissionBuilder) Build() *PermissionConfig {
	return b.config
}

type PermissionType string

const (
	PermissionTypeBash     PermissionType = "bash"
	PermissionTypeEdit     PermissionType = "edit"
	PermissionTypeWebfetch PermissionType = "webfetch"
)

type PermissionRequest struct {
	ID      string
	Session string
	Type    PermissionType
	Details map[string]interface{}
}

type PermissionResponse struct {
	Allow  bool
	Reason string
}

type PermissionHandler func(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error)

func DefaultPermissionHandler() PermissionHandler {
	return func(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error) {
		return &PermissionResponse{
			Allow:  false,
			Reason: "default policy: ask for permission",
		}, nil
	}
}

func AllowAllPermissionHandler() PermissionHandler {
	return func(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error) {
		return &PermissionResponse{
			Allow:  true,
			Reason: "allow all policy",
		}, nil
	}
}

func DenyAllPermissionHandler() PermissionHandler {
	return func(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error) {
		return &PermissionResponse{
			Allow:  false,
			Reason: "deny all policy",
		}, nil
	}
}
