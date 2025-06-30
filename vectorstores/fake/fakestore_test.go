package fake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeleteCollection(t *testing.T) {
	s := New()
	err := s.DeleteCollection(context.Background(), "test-collection")
	assert.NoError(t, err)
}
