package vectorstores_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
)

type mockFilterStore struct {
	docs []schema.Document
}

func (m *mockFilterStore) AddDocuments(ctx context.Context, docs []schema.Document, opts ...vectorstores.Option) ([]string, error) {
	return nil, nil
}
func (m *mockFilterStore) SimilaritySearch(ctx context.Context, query string, numDocuments int, opts ...vectorstores.Option) ([]schema.Document, error) {
	options := vectorstores.ParseOptions(opts...)
	var results []schema.Document
	for _, doc := range m.docs {
		match := true
		for k, v := range options.Filters {
			if docV, ok := doc.Metadata[k]; !ok || docV != v {
				match = false
				break
			}
		}
		if match {
			results = append(results, doc)
			if len(results) >= numDocuments {
				break
			}
		}
	}
	return results, nil
}
func (m *mockFilterStore) SimilaritySearchBatch(ctx context.Context, queries []string, numDocs int, opts ...vectorstores.Option) ([][]schema.Document, error) {
	return nil, nil
}
func (m *mockFilterStore) SimilaritySearchWithScores(ctx context.Context, query string, numDocs int, opts ...vectorstores.Option) ([]vectorstores.DocumentWithScore, error) {
	return nil, nil
}
func (m *mockFilterStore) ListCollections(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockFilterStore) DeleteCollection(ctx context.Context, name string) error {
	return nil
}
func (m *mockFilterStore) DeleteDocumentsByFilter(ctx context.Context, filters map[string]any, opts ...vectorstores.Option) error {
	return nil
}

func TestNewDefinitionRetriever_Validation(t *testing.T) {
	t.Run("nil store returns error", func(t *testing.T) {
		retriever, err := vectorstores.NewDefinitionRetriever(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "store cannot be nil")
		assert.Nil(t, retriever)
	})
}

func TestDefinitionRetriever_GetDefinition(t *testing.T) {
	// Setup mock store with filtering
	mockStore := &mockFilterStore{
		docs: []schema.Document{
			{
				PageContent: "type User struct { Name string }",
				Metadata: map[string]any{
					"identifier":    "User",
					"is_definition": true,
					"symbol_type":   "struct",
					"source":        "models/user.go",
				},
			},
			{
				PageContent: "func (u *User) Save() error { return nil }",
				Metadata: map[string]any{
					"identifier":    "User.Save",
					"is_definition": true,
					"symbol_type":   "method",
					"source":        "models/user.go",
				},
			},
			{
				PageContent: "func main() { fmt.Println(\"hello\") }",
				Metadata: map[string]any{
					"identifier":    "main",
					"is_definition": true,
					"symbol_type":   "function",
					"source":        "main.go",
				},
			},
		},
	}

	retriever, err := vectorstores.NewDefinitionRetriever(mockStore)
	require.NoError(t, err)

	t.Run("LookupExistingStruct", func(t *testing.T) {
		docs, err := retriever.GetDefinition(context.Background(), "User")
		require.NoError(t, err)
		assert.Len(t, docs, 1)
		assert.Equal(t, "User", docs[0].Metadata["identifier"])
		assert.Equal(t, "struct", docs[0].Metadata["symbol_type"])
	})

	t.Run("LookupExistingFunction", func(t *testing.T) {
		docs, err := retriever.GetDefinition(context.Background(), "main")
		require.NoError(t, err)
		assert.Len(t, docs, 1)
		assert.Equal(t, "main", docs[0].Metadata["identifier"])
		assert.Equal(t, "function", docs[0].Metadata["symbol_type"])
	})

	t.Run("LookupNonExistentSymbol", func(t *testing.T) {
		docs, err := retriever.GetDefinition(context.Background(), "NonExistent")
		require.NoError(t, err)
		assert.Len(t, docs, 0)
	})
}
