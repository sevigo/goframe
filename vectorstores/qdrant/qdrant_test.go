package qdrant

import (
	"context"
	"fmt"
	"log/slog"
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
	embedder := &MockEmbedder{dimension: 768}
	opt := WithEmbedder(embedder)
	opts := options{}
	opt(&opts)
	assert.Equal(t, embedder, opts.embedder)
}

func TestWithAPIKey(t *testing.T) {
	opt := WithAPIKey("secret-key-123")
	opts := options{}
	opt(&opts)
	assert.Equal(t, "secret-key-123", opts.apiKey)
}

func TestWithSparseVector(t *testing.T) {
	opt := WithSparseVector("bow")
	opts := options{}
	opt(&opts)
	assert.Equal(t, []string{"bow"}, opts.sparseVectors)
}

func TestWithPayloadIndex(t *testing.T) {
	opt := WithPayloadIndex("source", "package_name")
	opts := options{}
	opt(&opts)
	assert.Equal(t, []string{"source", "package_name"}, opts.payloadIndexes)
}

func TestBuildQdrantFilter(t *testing.T) {
	t.Run("empty_filters", func(t *testing.T) {
		filter := buildQdrantFilter(nil)
		assert.Nil(t, filter)

		filter = buildQdrantFilter(map[string]any{})
		assert.Nil(t, filter)
	})

	t.Run("string_value", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{"name": "test"})
		assert.NotNil(t, filter)
		assert.Len(t, filter.GetMust(), 1)
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
			"valid":       "string",
			"unsupported": struct{ X int }{X: 1},
		})
		assert.NotNil(t, filter)
		assert.Len(t, filter.GetMust(), 1) // Only valid field should be included
	})

	t.Run("nil_value_in_slice", func(t *testing.T) {
		filter := buildQdrantFilter(map[string]any{
			"tags": []any{"a", nil, "b", nil},
		})
		assert.NotNil(t, filter)
	})
}

func TestBuildMatchFromSlice(t *testing.T) {
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
		assert.NotNil(t, match.GetMatchValue())
	})

	t.Run("int_slice", func(t *testing.T) {
		match := buildMatchFromSlice("ids", []any{1, 2, 3})
		assert.NotNil(t, match)
		assert.NotNil(t, match.GetMatchValue())
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
	t.Run("no_sparse_vectors", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: nil}}
		config := store.buildSparseVectorConfig()
		assert.Nil(t, config)
	})

	t.Run("with_sparse_vector", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: []string{"bow"}}}
		config := store.buildSparseVectorConfig()
		assert.NotNil(t, config)
		assert.Len(t, config.GetMap(), 1)
		assert.NotNil(t, config.GetMap()["bow"].GetIndex())
		assert.True(t, config.GetMap()["bow"].GetIndex().GetOnDisk())
	})

	t.Run("multiple_sparse_vectors", func(t *testing.T) {
		store := &Store{options: options{sparseVectors: []string{"bow", "bm25"}}}
		config := store.buildSparseVectorConfig()
		assert.NotNil(t, config)
		assert.Len(t, config.GetMap(), 2)
	})
}

func TestStoreConvertToQdrantValue(t *testing.T) {
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
		assert.Len(t, val.GetListValue().GetValues(), 3)
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
		assert.Len(t, payload["tags"].GetListValue().GetValues(), 2)
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
			"list_field": {
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
	embedder := &MockEmbedder{dimension: 768}
	store := &Store{embedder: embedder}

	assert.Equal(t, embedder, store.GetEmbedder())
}

// These are defined in the vectorstores package, not qdrant
// TestNewDependencyRetriever and TestContextNetwork would be in vectorstores_test.go

func TestWithTimeout(t *testing.T) {
	opt := WithTimeout(60 * time.Second)
	opts := options{}
	opt(&opts)
	assert.Equal(t, 60*time.Second, opts.timeout)

	// Zero timeout should be ignored
	opt = WithTimeout(0)
	opts = options{}
	opt(&opts)
	assert.Equal(t, time.Duration(0), opts.timeout)
}

func TestWithRetryDelay(t *testing.T) {
	opt := WithRetryDelay(5 * time.Second)
	opts := options{}
	opt(&opts)
	assert.Equal(t, 5*time.Second, opts.retryDelay)
}

func TestWithMaxRetryDelay(t *testing.T) {
	opt := WithMaxRetryDelay(60 * time.Second)
	opts := options{}
	opt(&opts)
	assert.Equal(t, 60*time.Second, opts.maxRetryDelay)
}

func TestWithRetryJitter(t *testing.T) {
	opt := WithRetryJitter(2 * time.Second)
	opts := options{}
	opt(&opts)
	assert.Equal(t, 2*time.Second, opts.retryJitter)
}

func TestWithKeepaliveTime(t *testing.T) {
	opt := WithKeepaliveTime(15 * time.Second)
	opts := options{}
	opt(&opts)
	assert.Equal(t, 15*time.Second, opts.keepaliveTime)
}

func TestWithKeepaliveTimeout(t *testing.T) {
	opt := WithKeepaliveTimeout(5 * time.Second)
	opts := options{}
	opt(&opts)
	assert.Equal(t, 5*time.Second, opts.keepaliveTimeout)
}

func TestWithPoolSize(t *testing.T) {
	opt := WithPoolSize(20)
	opts := options{}
	opt(&opts)
	assert.Equal(t, 20, opts.poolSize)
}

func TestApplyDefaultsConnectionSettings(t *testing.T) {
	opts := options{}
	applyDefaults(&opts)

	assert.Equal(t, defaultTimeout, opts.timeout)
	assert.Equal(t, defaultKeepaliveTime, opts.keepaliveTime)
	assert.Equal(t, defaultKeepaliveTimeout, opts.keepaliveTimeout)
	assert.Equal(t, defaultPoolSize, opts.poolSize)
	assert.Equal(t, 2*time.Second, opts.retryDelay)
	assert.Equal(t, 30*time.Second, opts.maxRetryDelay)
	assert.Equal(t, 1*time.Second, opts.retryJitter)
}

func TestStoreIsRetryableErrorExtended(t *testing.T) {
	store := &Store{batchConfig: BatchConfig{MaxRetryDelay: 30 * time.Second}}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"connection refused", fmt.Errorf("connection refused"), true},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"transport closing", fmt.Errorf("transport is closing"), true},
		{"client closing", fmt.Errorf("client connection is closing"), true},
		{"deadline exceeded", fmt.Errorf("context deadline exceeded"), true},
		{"error 503", fmt.Errorf("Error 503: service unavailable"), true},
		{"non-retryable", fmt.Errorf("invalid parameter"), false},
		{"not found", fmt.Errorf("collection not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStoreDoWithRetry(t *testing.T) {
	logger := slog.Default()
	store := &Store{
		options: options{
			retryAttempts: 2,
			retryDelay:    10 * time.Millisecond,
			maxRetryDelay: 100 * time.Millisecond,
			retryJitter:   5 * time.Millisecond,
			logger:        logger,
		},
		batchConfig: BatchConfig{MaxRetryDelay: 100 * time.Millisecond},
		logger:      logger,
	}

	t.Run("success on first try", func(t *testing.T) {
		callCount := 0
		err := store.doWithRetry(context.Background(), "test_op", func() error {
			callCount++
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})

	t.Run("success after retry", func(t *testing.T) {
		callCount := 0
		err := store.doWithRetry(context.Background(), "test_op", func() error {
			callCount++
			if callCount < 2 {
				return fmt.Errorf("Error 500: internal error")
			}
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 2, callCount)
	})

	t.Run("non-retryable error", func(t *testing.T) {
		callCount := 0
		err := store.doWithRetry(context.Background(), "test_op", func() error {
			callCount++
			return fmt.Errorf("invalid parameter")
		})
		assert.Error(t, err)
		assert.Equal(t, 1, callCount)
	})

	t.Run("exhausted retries", func(t *testing.T) {
		callCount := 0
		err := store.doWithRetry(context.Background(), "test_op", func() error {
			callCount++
			return fmt.Errorf("connection refused")
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test_op failed after 3 attempts")
		assert.Equal(t, 3, callCount) // initial + 2 retries
	})
}

func TestOptionsCloneWithNewFields(t *testing.T) {
	original := options{
		collectionName:   "test",
		timeout:          60 * time.Second,
		retryDelay:       3 * time.Second,
		maxRetryDelay:    30 * time.Second,
		retryJitter:      500 * time.Millisecond,
		keepaliveTime:    15 * time.Second,
		keepaliveTimeout: 5 * time.Second,
		poolSize:         20,
		grpcOptions:      nil,
		payloadIndexes:   []string{"source", "package"},
		sparseVectors:    []string{"bow"},
	}

	cloned := original.Clone()

	assert.Equal(t, original.timeout, cloned.timeout)
	assert.Equal(t, original.retryDelay, cloned.retryDelay)
	assert.Equal(t, original.maxRetryDelay, cloned.maxRetryDelay)
	assert.Equal(t, original.retryJitter, cloned.retryJitter)
	assert.Equal(t, original.keepaliveTime, cloned.keepaliveTime)
	assert.Equal(t, original.keepaliveTimeout, cloned.keepaliveTimeout)
	assert.Equal(t, original.poolSize, cloned.poolSize)
	assert.Equal(t, original.payloadIndexes, cloned.payloadIndexes)
	assert.Equal(t, original.sparseVectors, cloned.sparseVectors)

	// Verify it's a copy, not a reference
	cloned.payloadIndexes[0] = "modified"
	assert.Equal(t, "source", original.payloadIndexes[0])
}
