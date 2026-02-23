package vectorstores_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/vectorstores"
)

func TestNewDependencyRetriever_Validation(t *testing.T) {
	t.Run("nil store returns error", func(t *testing.T) {
		retriever, err := vectorstores.NewDependencyRetriever(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "store cannot be nil")
		assert.Nil(t, retriever)
	})
}
