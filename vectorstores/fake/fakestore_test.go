package fake_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/vectorstores/fake"
)

func TestDeleteCollection(t *testing.T) {
	s := fake.New()
	err := s.DeleteCollection(context.Background(), "test-collection")
	require.NoError(t, err)
}
