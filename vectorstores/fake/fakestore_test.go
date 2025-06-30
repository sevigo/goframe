package fake_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sevigo/goframe/vectorstores/fake"
)

func TestDeleteCollection(t *testing.T) {
	s := fake.New()
	err := s.DeleteCollection(context.Background(), "test-collection")
	assert.NoError(t, err)
}
