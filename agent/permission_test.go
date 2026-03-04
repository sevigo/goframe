package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermissionBuilder(t *testing.T) {
	t.Run("default permissions", func(t *testing.T) {
		perm := NewPermissions().Build()
		assert.Equal(t, PermissionAsk, perm.Bash)
		assert.Equal(t, PermissionAsk, perm.Edit)
		assert.Equal(t, PermissionAsk, perm.Webfetch)
	})

	t.Run("allow all bash", func(t *testing.T) {
		perm := NewPermissions().AllowBash().Build()
		assert.Equal(t, PermissionAllow, perm.Bash)
	})

	t.Run("allow specific bash patterns", func(t *testing.T) {
		perm := NewPermissions().
			AllowBash("go test", "go build").
			Build()
		assert.Equal(t, PermissionAsk, perm.Bash)
		assert.Equal(t, PermissionAllow, perm.BashMap["go test"])
		assert.Equal(t, PermissionAllow, perm.BashMap["go build"])
	})

	t.Run("deny bash patterns", func(t *testing.T) {
		perm := NewPermissions().
			DenyBash("rm *", "git push").
			Build()
		assert.Equal(t, PermissionDeny, perm.BashMap["rm *"])
		assert.Equal(t, PermissionDeny, perm.BashMap["git push"])
	})

	t.Run("mixed bash permissions", func(t *testing.T) {
		perm := NewPermissions().
			AllowBash("go test", "go build").
			AskBash("rm *").
			DenyBash("git push").
			Build()
		assert.Equal(t, PermissionAllow, perm.BashMap["go test"])
		assert.Equal(t, PermissionAsk, perm.BashMap["rm *"])
		assert.Equal(t, PermissionDeny, perm.BashMap["git push"])
	})

	t.Run("edit permissions", func(t *testing.T) {
		perm := NewPermissions().AllowEdit().Build()
		assert.Equal(t, PermissionAllow, perm.Edit)

		perm = NewPermissions().DenyEdit().Build()
		assert.Equal(t, PermissionDeny, perm.Edit)
	})

	t.Run("webfetch permissions", func(t *testing.T) {
		perm := NewPermissions().AllowWebfetch().Build()
		assert.Equal(t, PermissionAllow, perm.Webfetch)

		perm = NewPermissions().DenyWebfetch().Build()
		assert.Equal(t, PermissionDeny, perm.Webfetch)
	})

	t.Run("chained permissions", func(t *testing.T) {
		perm := NewPermissions().
			AllowBash("go test", "go build").
			AllowEdit().
			DenyWebfetch().
			Build()

		assert.Equal(t, PermissionAsk, perm.Bash)
		assert.Equal(t, PermissionAllow, perm.BashMap["go test"])
		assert.Equal(t, PermissionAllow, perm.Edit)
		assert.Equal(t, PermissionDeny, perm.Webfetch)
	})
}

func TestPermissionHandlers(t *testing.T) {
	t.Run("default handler", func(t *testing.T) {
		handler := DefaultPermissionHandler()
		resp, err := handler(nil, &PermissionRequest{
			Type: PermissionTypeBash,
		})
		assert.NoError(t, err)
		assert.False(t, resp.Allow)
		assert.Contains(t, resp.Reason, "default")
	})

	t.Run("allow all handler", func(t *testing.T) {
		handler := AllowAllPermissionHandler()
		resp, err := handler(nil, &PermissionRequest{
			Type: PermissionTypeEdit,
		})
		assert.NoError(t, err)
		assert.True(t, resp.Allow)
	})

	t.Run("deny all handler", func(t *testing.T) {
		handler := DenyAllPermissionHandler()
		resp, err := handler(nil, &PermissionRequest{
			Type: PermissionTypeWebfetch,
		})
		assert.NoError(t, err)
		assert.False(t, resp.Allow)
	})
}
