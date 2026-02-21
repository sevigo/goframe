package vectorstores_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/schema"
	fakeretriever "github.com/sevigo/goframe/schema/fake"
	"github.com/sevigo/goframe/vectorstores"
)

func TestHyDERetriever_SingleGeneration(t *testing.T) {
	ctx := context.Background()

	t.Run("uses hypothetical doc for retrieval", func(t *testing.T) {
		expectedDocs := []schema.Document{
			{PageContent: "actual code snippet", Metadata: map[string]any{"source": "main.go"}},
		}

		fr := fakeretriever.NewRetriever()
		fr.DocsToReturn = expectedDocs

		generator := func(_ context.Context, query string) (string, error) {
			return "hypothetical answer for: " + query, nil
		}

		retriever := vectorstores.NewHyDERetriever(fr, generator)

		docs, err := retriever.GetRelevantDocuments(ctx, "how does auth work?")
		require.NoError(t, err)
		assert.Equal(t, expectedDocs, docs)
	})

	t.Run("falls back to original query on generator failure", func(t *testing.T) {
		expectedDocs := []schema.Document{
			{PageContent: "fallback result"},
		}

		fr := fakeretriever.NewRetriever()
		fr.DocsToReturn = expectedDocs

		generator := func(_ context.Context, _ string) (string, error) {
			return "", errors.New("LLM unavailable")
		}

		retriever := vectorstores.NewHyDERetriever(fr, generator)

		docs, err := retriever.GetRelevantDocuments(ctx, "test query")
		require.NoError(t, err)
		assert.Equal(t, expectedDocs, docs)
	})

	t.Run("propagates base retriever error", func(t *testing.T) {
		fr := fakeretriever.NewRetriever()
		fr.ErrToReturn = errors.New("vector store down")

		generator := func(_ context.Context, _ string) (string, error) {
			return "hypothetical doc", nil
		}

		retriever := vectorstores.NewHyDERetriever(fr, generator)

		_, err := retriever.GetRelevantDocuments(ctx, "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vector store down")
	})
}

func TestHyDERetriever_MultiGeneration(t *testing.T) {
	ctx := context.Background()

	t.Run("deduplicates results from multiple generations", func(t *testing.T) {
		// The fake retriever always returns the same docs regardless of query,
		// so all generations will return identical results that get deduplicated.
		docs := []schema.Document{
			{PageContent: "shared result", Metadata: map[string]any{"source": "utils.go"}},
		}

		fr := fakeretriever.NewRetriever()
		fr.DocsToReturn = docs

		callCount := 0
		generator := func(_ context.Context, _ string) (string, error) {
			callCount++
			return "hypothetical doc", nil
		}

		retriever := vectorstores.NewHyDERetriever(fr, generator, vectorstores.WithNumGenerations(3))

		result, err := retriever.GetRelevantDocuments(ctx, "test")
		require.NoError(t, err)

		// Should be deduplicated to 1 doc even though 3 generations ran
		assert.Len(t, result, 1)
		assert.Equal(t, 3, callCount)
	})

	t.Run("falls back when all generations fail", func(t *testing.T) {
		expectedDocs := []schema.Document{
			{PageContent: "fallback doc"},
		}

		fr := fakeretriever.NewRetriever()
		fr.DocsToReturn = expectedDocs

		generator := func(_ context.Context, _ string) (string, error) {
			return "", errors.New("generation failed")
		}

		retriever := vectorstores.NewHyDERetriever(fr, generator, vectorstores.WithNumGenerations(3))

		docs, err := retriever.GetRelevantDocuments(ctx, "test")
		require.NoError(t, err)
		assert.Equal(t, expectedDocs, docs)
	})
}

func TestHyDERetriever_NumGenerationsDefaults(t *testing.T) {
	fr := fakeretriever.NewRetriever()
	generator := func(_ context.Context, _ string) (string, error) {
		return "doc", nil
	}

	t.Run("defaults to 1 generation", func(t *testing.T) {
		retriever := vectorstores.NewHyDERetriever(fr, generator)
		assert.Equal(t, 1, retriever.NumGenerations)
	})

	t.Run("clamps negative values to 1", func(t *testing.T) {
		retriever := vectorstores.NewHyDERetriever(fr, generator, vectorstores.WithNumGenerations(-5))
		assert.Equal(t, 1, retriever.NumGenerations)
	})
}
