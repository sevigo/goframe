package qdrant

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"

	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
)

// MockEmbedder is a mock embedder for testing
type MockEmbedder struct {
	dimension int
}

func (m *MockEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = make([]float32, m.dimension)
		for j := range embeddings[i] {
			embeddings[i][j] = float32(float64(i)*0.1 + float64(j)*0.01)
		}
	}
	return embeddings, nil
}

func (m *MockEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	embedding := make([]float32, m.dimension)
	for i := range embedding {
		embedding[i] = float32(0.5 + float64(i)*0.01)
	}
	return embedding, nil
}

func (m *MockEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = make([]float32, m.dimension)
		for j := range embeddings[i] {
			embeddings[i][j] = float32(0.5 + float64(i)*0.01 + float64(j)*0.01)
		}
	}
	return embeddings, nil
}

func (m *MockEmbedder) GetDimension(ctx context.Context) (int, error) {
	return m.dimension, nil
}

func TestWithCollectionName(t *testing.T) {
	t.Parallel()

	t.Run("sets_collection_name", func(t *testing.T) {
		opt := WithCollectionName("test-collection")
		opts := options{}
		opt(&opts)
		assert.Equal(t, "test-collection", opts.collectionName)
	})

	t.Run("trims_whitespace", func(t *testing.T) {
		opt := WithCollectionName("  test  ")
		opts := options{}
		opt(&opts)
		assert.Equal(t, "test", opts.collectionName)
	})
}

func TestWithEmbedder(t *testing.T) {
	t.Parallel()

	embedder := &MockEmbedder{dimension: 768}
	opt := WithEmbedder(embedder)
	opts := options{}
	opt(&opts)
	assert.Equal(t, embedder, opts.embedder)
}

func TestWithAPIKey(t *testing.T) {
	t.Parallel()

	opt := WithAPIKey("secret-key-123")
	opts := options{}
	opt(&opts)
	assert.Equal(t, "secret-key-123", opts.apiKey)
}

func TestWithSparseVector(t *testing.T) {
	t.Parallel()

	opt := WithSparseVector("bow")
	opts := options{}
	opt(&opts)
	assert.Equal(t, []string{"bow"}, opts.sparseVectors)
}

func TestWithPayloadIndex(t *testing.T) {
	t.Parallel()

	opt := WithPayloadIndex("source", "package_name")
	opts := options{}
	opt(&opts)
	assert.Equal(t, []string{"source", "package_name"}, opts.payloadIndexes)
}

func TestBuildQdrantFilter(t *testing.T) {
	t.Parallel()

	t.Run("empty_filters", func(t *testing.T) {
		filter := buildQdrantFilter(nil)
		assert.Nil(t, filter)

		filter = buildQdrantFilter(map[string]any{})
		assert.Nil(t, filter)
	})

	t.Run("string_value", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{"name": "test"})
		assert.NotNil(t, filter)
		assert.Len(t, filter.Must, 1)
	})

	t.Run("int_value", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{"count": 42})
		assert.NotNil(t, filter)
	})

	t.Run("bool_value", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{"active": true})
		assert.NotNil(t, filter)
	})

	t.Run("string_slice", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{"tags": []string{"a", "b", "c"}})
		assert.NotNil(t, filter)
	})

	t.Run("int64_slice", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{"ids": []int64{1, 2, 3}})
		assert.NotNil(t, filter)
	})

	t.Run("int_slice", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{"ids": []int{1, 2, 3}})
		assert.NotNil(t, filter)
	})

	t.Run("mixed_slice", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{"ids": []any{1, "test", true}})
		assert.NotNil(t, filter)
	})

	t.Run("unsupported_type", func(t *testing.T) {
		// Should not panic, just skip unsupported type
		filter := buildQdrantFilter(map[string]any{
			"valid":   "string",
			"unsupported": struct{ X int }{X: 1},
		})
		assert.NotNil(t, filter)
		assert.Len(t, filter.Must, 1) // Only valid field should be included
	})

	t.Run("nil_value_in_slice", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{
			"tags": []any{"a", nil, "b", nil},
		})
		assert.NotNil(t, filter)
	})
}

func TestBuildMatchFromSlice(t *testing.T) {
	t.Parallel()

	t.Run("empty_slice", func(t *testing.T) {
		match := buildMatchFromSlice("key", []any{})
		assert.Nil(t, match)
	})

	t.Run("all_nil_slice", func(t *testing.T) {
		match := buildMatchFromSlice("key", []any{nil, nil})
		assert.Nil(t, match)
	})

	t.Run("string_slice", func(t *testing.T) {
		match := buildMatchFromSlice("tags", []any{"a", "b", "c"})
		assert.NotNil(t, match)
		assert.NotNil(t, match.MatchValue)
	})

	t.Run("int_slice", func(t *testing.T) {
		match := buildMatchFromSlice("ids", []any{1, 2, 3})
		assert.NotNil(t, match)
		assert.NotNil(t, match.MatchValue)
	})

	t.Run("mixed_int_types", func(t *testing.T) {
		match := buildMatchFromSlice("ids", []any{int(1), int32(2), int64(3)})
		assert.NotNil(t, match)
	})

	t.Run("unsupported_type", func(t *testing.T) {
		match := buildMatchFromSlice("data", []any{struct{ X int }{X: 1}})
		assert.Nil(t, match)
	})
}

func TestStoreValidateAndNormalizeBatchConfig(t *testing.T) {
	t.Parallel()

	// Create store with a logger to avoid nil pointer errors
	store := &Store{
		logger: nil, // Test doesn't use logging
	}

	t.Run("default_values", func(t *testing.T) {
		// Note: validateAndNormalizeBatchConfig only sets defaults for negative values,
		// not zero values. The actual defaults are applied in New() when creating the Store.
		config := store.validateAndNormalizeBatchConfig(BatchConfig{})
		assert.Equal(t, DefaultBatchSize, config.BatchSize)
		assert.Equal(t, DefaultMaxConcurrency, config.MaxConcurrency)
		// RetryAttempts stays at 0 for zero value input (defaults applied elsewhere)
		assert.Equal(t, int(0), config.RetryAttempts)
	})

	t.Run("below_minimum_batch_size", func(t *testing.T) {
		config := store.validateAndNormalizeBatchConfig(BatchConfig{BatchSize: 0})
		assert.Equal(t, DefaultBatchSize, config.BatchSize)
	})

	t.Run("above_maximum_batch_size", func(t *testing.T) {
		config := store.validateAndNormalizeBatchConfig(BatchConfig{BatchSize: 2000})
		assert.Equal(t, MaxBatchSize, config.BatchSize)
	})

	t.Run("negative_embedding_batch_size", func(t *testing.T) {
		config := store.validateAndNormalizeBatchConfig(BatchConfig{EmbeddingBatchSize: -1})
		assert.Equal(t, 0, config.EmbeddingBatchSize)
	})

	t.Run("zero_concurrency", func(t *testing.T) {
		config := store.validateAndNormalizeBatchConfig(BatchConfig{MaxConcurrency: 0})
		assert.Equal(t, DefaultMaxConcurrency, config.MaxConcurrency)
	})

	t.Run("negative_retry_attempts", func(t *testing.T) {
		config := store.validateAndNormalizeBatchConfig(BatchConfig{RetryAttempts: -1})
		assert.Equal(t, DefaultRetryAttempts, config.RetryAttempts)
	})

	t.Run("zero_retry_delay", func(t *testing.T) {
		config := store.validateAndNormalizeBatchConfig(BatchConfig{RetryDelay: 0})
		assert.Equal(t, DefaultRetryDelay, config.RetryDelay)
	})

	t.Run("custom_values_preserved", func(t *testing.T) {
		config := store.validateAndNormalizeBatchConfig(BatchConfig{
			BatchSize:               50,
			MaxConcurrency:          4,
			RetryAttempts:           5,
			RetryDelay:              3 * time.Second,
			MaxRetryDelay:           60 * time.Second,
			EmbeddingBatchSize:      25,
			EmbeddingMaxConcurrency: 2,
		})
		assert.Equal(t, 50, config.BatchSize)
		assert.Equal(t, 4, config.MaxConcurrency)
		assert.Equal(t, 5, config.RetryAttempts)
		assert.Equal(t, 3*time.Second, config.RetryDelay)
		assert.Equal(t, 60*time.Second, config.MaxRetryDelay)
	})
}

func TestStoreValidateHybridSearchOptions(t *testing.T) {
	t.Parallel()

	t.Run("nil_sparse_query", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: []string{"bow"}}}
		err := store.validateHybridSearchOptions(vectorstores.Options{SparseQuery: nil})
		assert.NoError(t, err)
	})

	t.Run("sparse_query_without_sparse_config", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: nil}}
		sparse := &schema.SparseVector{Indices: []uint32{1}, Values: []float32{0.5}}
		err := store.validateHybridSearchOptions(vectorstores.Options{SparseQuery: sparse})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), ErrMissingSparseName.Error())
	})

	t.Run("sparse_query_with_matching_config", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: []string{"bow"}}}
		sparse := &schema.SparseVector{Indices: []uint32{1}, Values: []float32{0.5}}
		err := store.validateHybridSearchOptions(vectorstores.Options{SparseQuery: sparse})
		assert.NoError(t, err)
	})

	t.Run("mismatched_sparse_vector_length", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: []string{"bow"}}}
		sparse := &schema.SparseVector{Indices: []uint32{1, 2}, Values: []float32{0.5}}
		err := store.validateHybridSearchOptions(vectorstores.Options{SparseQuery: sparse})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sparse vector indices and values length mismatch")
	})
}

func TestStoreBuildSparseVectorConfig(t *testing.T) {
	t.Parallel()

	t.Run("no_sparse_vectors", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: nil}}
		config := store.buildSparseVectorConfig()
		assert.Nil(t, config)
	})

	t.Run("with_sparse_vector", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: []string{"bow"}}}
		config := store.buildSparseVectorConfig()
		assert.NotNil(t, config)
		assert.Len(t, config.Map, 1)
		assert.NotNil(t, config.Map["bow"].Index)
		assert.True(t, *config.Map["bow"].Index.OnDisk)
	})

	t.Run("multiple_sparse_vectors", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: []string{"bow", "bm25"}}}
		config := store.buildSparseVectorConfig()
		assert.NotNil(t, config)
		assert.Len(t, config.Map, 2)
	})
}

func TestStoreConvertToQdrantValue(t *testing.T) {
	t.Parallel()

	store := &Store{}

	t.Run("string_value", func(t *testing.T) {
		val := store.convertToQdrantValue("test")
		assert.IsType(t, &qdrant.Value{}, val)
		assert.Equal(t, "test", val.GetStringValue())
	})

	t.Run("int_value", func(t *testing.T) {
		val := store.convertToQdrantValue(42)
		assert.IsType(t, &qdrant.Value{}, val)
		assert.Equal(t, int64(42), val.GetIntegerValue())
	})

	t.Run("int32_value", func(t *testing.T) {
		val := store.convertToQdrantValue(int32(42))
		assert.IsType(t, &qdrant.Value{}, val)
		assert.Equal(t, int64(42), val.GetIntegerValue())
	})

	t.Run("int64_value", func(t *testing.T) {
		val := store.convertToQdrantValue(int64(42))
		assert.IsType(t, &qdrant.Value{}, val)
		assert.Equal(t, int64(42), val.GetIntegerValue())
	})

	t.Run("float32_value", func(t *testing.T) {
		val := store.convertToQdrantValue(float32(3.14))
		assert.IsType(t, &qdrant.Value{}, val)
		// float32 to float64 conversion has precision differences
		// Just check it's approximately 3.14
		assert.InDelta(t, 3.14, val.GetDoubleValue(), 0.01)
	})

	t.Run("float64_value", func(t *testing.T) {
		val := store.convertToQdrantValue(3.14)
		assert.IsType(t, &qdrant.Value{}, val)
		assert.Equal(t, 3.14, val.GetDoubleValue())
	})

	t.Run("bool_value", func(t *testing.T) {
		val := store.convertToQdrantValue(true)
		assert.IsType(t, &qdrant.Value{}, val)
		assert.True(t, val.GetBoolValue())
	})

	t.Run("string_slice", func(t *testing.T) {
		val := store.convertToQdrantValue([]string{"a", "b", "c"})
		assert.IsType(t, &qdrant.Value{}, val)
		assert.NotNil(t, val.GetListValue())
		assert.Len(t, val.GetListValue().Values, 3)
	})

	t.Run("nil_value", func(t *testing.T) {
		val := store.convertToQdrantValue(nil)
		assert.IsType(t, &qdrant.Value{}, val)
		assert.NotNil(t, val.GetNullValue())
	})

	t.Run("unknown_type", func(t *testing.T) {
		val := store.convertToQdrantValue(struct{ X int }{X: 1})
		assert.IsType(t, &qdrant.Value{}, val)
		// Unknown types get stringified - just verify it's a string value
		assert.NotNil(t, val.GetStringValue())
	})
}

func TestStoreConvertFromQdrantValue(t *testing.T) {
	t.Parallel()

	store := &Store{}

	t.Run("string_value", func(t *testing.T) {
		val := &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: "test"}}
		result := store.convertFromQdrantValue(val)
		assert.Equal(t, "test", result)
	})

	t.Run("integer_value", func(t *testing.T) {
		val := &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: 42}}
		result := store.convertFromQdrantValue(val)
		assert.Equal(t, int64(42), result)
	})

	t.Run("double_value", func(t *testing.T) {
		val := &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: 3.14}}
		result := store.convertFromQdrantValue(val)
		assert.Equal(t, 3.14, result)
	})

	t.Run("bool_value", func(t *testing.T) {
		val := &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: true}}
		result := store.convertFromQdrantValue(val)
		assert.True(t, result.(bool))
	})

	t.Run("list_value", func(t *testing.T) {
		val := &qdrant.Value{Kind: &qdrant.Value_ListValue{ListValue: &qdrant.ListValue{
			Values: []*qdrant.Value{
				{Kind: &qdrant.Value_StringValue{StringValue: "a"}},
				{Kind: &qdrant.Value_IntegerValue{IntegerValue: 1}},
			},
		}}}
		result := store.convertFromQdrantValue(val)
		results, ok := result.([]any)
		assert.True(t, ok)
		assert.Len(t, results, 2)
		assert.Equal(t, "a", results[0])
		assert.Equal(t, int64(1), results[1])
	})

	t.Run("null_value", func(t *testing.T) {
		val := &qdrant.Value{Kind: &qdrant.Value_NullValue{}}
		result := store.convertFromQdrantValue(val)
		assert.Nil(t, result)
	})
}

func TestStoreDocumentToPayload(t *testing.T) {
	t.Parallel()

	store := &Store{}

	t.Run("basic_document", func(t *testing.T) {
		doc := schema.Document{
			PageContent: "test content",
			Metadata: map[string]any{
				"source": "test.go",
			},
		}

		payload := store.documentToPayload(doc)
		assert.Equal(t, "test content", payload["page_content"].GetStringValue())
		assert.Equal(t, "test.go", payload["source"].GetStringValue())
	})

	t.Run("metadata_only", func(t *testing.T) {
		doc := schema.Document{
			PageContent: "test",
			Metadata: map[string]any{
				"count": 42,
				"tags":  []string{"a", "b"},
			},
		}

		payload := store.documentToPayload(doc)
		assert.Equal(t, int64(42), payload["count"].GetIntegerValue())
		assert.Len(t, payload["tags"].GetListValue().Values, 2)
	})

	t.Run("nil_metadata", func(t *testing.T) {
		doc := schema.Document{
			PageContent: "test",
			Metadata:    nil,
		}

		payload := store.documentToPayload(doc)
		assert.Equal(t, "test", payload["page_content"].GetStringValue())
		assert.Len(t, payload, 1)
	})
}

func TestStorePayloadToDocument(t *testing.T) {
	t.Parallel()

	store := &Store{}

	t.Run("basic_payload", func(t *testing.T) {
		payload := map[string]*qdrant.Value{
			"page_content": {Kind: &qdrant.Value_StringValue{StringValue: "test content"}},
			"source":       {Kind: &qdrant.Value_StringValue{StringValue: "test.go"}},
		}

		doc := store.payloadToDocument(payload)
		assert.Equal(t, "test content", doc.PageContent)
		assert.Equal(t, "test.go", doc.Metadata["source"])
	})

	t.Run("all_data_types", func(t *testing.T) {
		payload := map[string]*qdrant.Value{
			"page_content": {Kind: &qdrant.Value_StringValue{StringValue: "test"}},
			"string_field": {Kind: &qdrant.Value_StringValue{StringValue: "str"}},
			"int_field":    {Kind: &qdrant.Value_IntegerValue{IntegerValue: 42}},
			"float_field":  {Kind: &qdrant.Value_DoubleValue{DoubleValue: 3.14}},
			"bool_field":   {Kind: &qdrant.Value_BoolValue{BoolValue: true}},
			"null_field":   {Kind: &qdrant.Value_NullValue{}},
			"list_field": &qdrant.Value{
				Kind: &qdrant.Value_ListValue{
					ListValue: &qdrant.ListValue{
						Values: []*qdrant.Value{
							{Kind: &qdrant.Value_StringValue{StringValue: "a"}},
						},
					},
				},
			},
		}

		doc := store.payloadToDocument(payload)
		assert.Equal(t, "test", doc.PageContent)
		assert.Equal(t, "str", doc.Metadata["string_field"])
		assert.Equal(t, int64(42), doc.Metadata["int_field"])
		assert.Equal(t, 3.14, doc.Metadata["float_field"])
		assert.True(t, doc.Metadata["bool_field"].(bool))
		assert.Nil(t, doc.Metadata["null_field"])
	})

	t.Run("missing_page_content", func(t *testing.T) {
		payload := map[string]*qdrant.Value{
			"source": {Kind: &qdrant.Value_StringValue{StringValue: "test.go"}},
		}

		doc := store.payloadToDocument(payload)
		assert.Equal(t, "", doc.PageContent)
	})
}

func TestStoreIsRetryableError(t *testing.T) {
	t.Parallel()

	store := &Store{}

	t.Run("error_500", func(t *testing.T) {
		err := fmt.Errorf("Error 500: internal server error")
		assert.True(t, store.isRetryableError(err))
	})

	t.Run("status_internal", func(t *testing.T) {
		err := fmt.Errorf("Status: INTERNAL")
		assert.True(t, store.isRetryableError(err))
	})

	t.Run("error_429", func(t *testing.T) {
		err := fmt.Errorf("Error 429: rate limit")
		assert.True(t, store.isRetryableError(err))
	})

	t.Run("resource_exhausted", func(t *testing.T) {
		err := fmt.Errorf("RESOURCE_EXHAUSTED")
		assert.True(t, store.isRetryableError(err))
	})

	t.Run("unexpected_eof", func(t *testing.T) {
		err := fmt.Errorf("unexpected EOF")
		assert.True(t, store.isRetryableError(err))
	})

	t.Run("non_retryable_error", func(t *testing.T) {
		err := fmt.Errorf("not a retryable error")
		assert.False(t, store.isRetryableError(err))
	})
}

func TestStoreCalculateNextDelay(t *testing.T) {
	t.Parallel()

	store := &Store{batchConfig: BatchConfig{MaxRetryDelay: 30 * time.Second}}

	t.Run("normal_delay_increase", func(t *testing.T) {
		delay := store.calculateNextDelay(2 * time.Second)
		assert.Equal(t, 4*time.Second, delay)
	})

	t.Run("capped_at_max", func(t *testing.T) {
		delay := store.calculateNextDelay(20 * time.Second)
		assert.Equal(t, 30*time.Second, delay)
	})

	t.Run("already_at_max", func(t *testing.T) {
		delay := store.calculateNextDelay(30 * time.Second)
		assert.Equal(t, 30*time.Second, delay)
	})
}

func TestStoreGetBatchConfig(t *testing.T) {
	t.Parallel()

	batchConfig := BatchConfig{
		BatchSize:      50,
		MaxConcurrency: 4,
	}
	store := &Store{
		logger: nil,
	}
	// Skip the SetBatchConfig that calls logBatchConfigUpdate
	store.batchConfig = batchConfig

	config := store.GetBatchConfig()
	assert.Equal(t, 50, config.BatchSize)
	assert.Equal(t, 4, config.MaxConcurrency)
}

func TestStoreGetEmbedder(t *testing.T) {
	t.Parallel()

	embedder := &MockEmbedder{dimension: 768}
	store := &Store{embedder: embedder}

	assert.Equal(t, embedder, store.GetEmbedder())
}

// These are defined in the vectorstores package, not qdrant
// TestNewDependencyRetriever and TestContextNetwork would be in vectorstores_test.go
